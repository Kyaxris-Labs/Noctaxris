package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	cbsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/codebuild"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	codebuildJSONContentType = "application/x-amz-json-1.1"
	codebuildEventSource     = "codebuild.amazonaws.com"
	defaultCodeBuildEndpoint = "http://host.docker.internal:4566"
	codebuildSession         = "noctaxris-codebuild"
)

func (s *Server) handleCodeBuild(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = codebuildAction(action)

	switch action {
	case catalog.ActionCodeBuildCreateProject:
		s.codebuildCreateProject(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeBuildUpdateProject:
		s.codebuildUpdateProject(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeBuildDeleteProject:
		s.codebuildDeleteProject(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeBuildListProjects:
		s.codebuildListProjects(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeBuildBatchGetProjects:
		s.codebuildBatchGetProjects(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeBuildStartBuild:
		s.codebuildStartBuild(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeBuildStartBuildBatch:
		s.codebuildStartBuildBatch(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeBuildStopBuild:
		s.codebuildStopBuild(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeBuildBatchGetBuilds:
		s.codebuildBatchGetBuilds(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeBuildListBuilds:
		s.codebuildListBuilds(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCodeBuildError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This CodeBuild action is not implemented.", readOnly, eventID, verified)
	}
}

func codebuildAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateProject":
		return catalog.ActionCodeBuildCreateProject
	case "UpdateProject":
		return catalog.ActionCodeBuildUpdateProject
	case "DeleteProject":
		return catalog.ActionCodeBuildDeleteProject
	case "ListProjects":
		return catalog.ActionCodeBuildListProjects
	case "BatchGetProjects":
		return catalog.ActionCodeBuildBatchGetProjects
	case "StartBuild":
		return catalog.ActionCodeBuildStartBuild
	case "StartBuildBatch":
		return catalog.ActionCodeBuildStartBuildBatch
	case "StopBuild":
		return catalog.ActionCodeBuildStopBuild
	case "BatchGetBuilds":
		return catalog.ActionCodeBuildBatchGetBuilds
	case "ListBuilds":
		return catalog.ActionCodeBuildListBuilds
	default:
		return action
	}
}

func (s *Server) codebuildRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultCodeBuildRegion
}

func parseCodeBuildEnvVars(v any) []store.CodeBuildEnvVar {
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]store.CodeBuildEnvVar, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := stringParam(m["name"])
		if name == "" {
			name = stringParam(m["Name"])
		}
		if name == "" {
			continue
		}
		value := stringParam(m["value"])
		if value == "" {
			value = stringParam(m["Value"])
		}
		out = append(out, store.CodeBuildEnvVar{Name: name, Value: value})
	}
	return out
}

func parseCodeBuildAllowOverride(source map[string]any, params map[string]any) *bool {
	if source != nil {
		if v, ok := source["allowOverride"]; ok {
			b := anyToBool(v, true)
			return &b
		}
		if v, ok := source["overrideAllowed"]; ok {
			b := anyToBool(v, true)
			return &b
		}
	}
	if params != nil {
		if v, ok := params["overrideAllowed"]; ok {
			b := anyToBool(v, true)
			return &b
		}
	}
	return nil
}

func parseCodeBuildConfigStubs(params map[string]any) (vpc, cache, fleet map[string]any, secondary []any, reportArns []string) {
	if params == nil {
		return nil, nil, nil, nil, nil
	}
	vpc, _ = params["vpcConfig"].(map[string]any)
	cache, _ = params["cache"].(map[string]any)
	fleet, _ = params["fleet"].(map[string]any)
	if fleet == nil {
		fleet, _ = params["projectFleet"].(map[string]any)
	}
	if raw, ok := params["secondarySources"].([]any); ok {
		secondary = raw
	}
	reportArns = stringSliceParam(params["reportGroupArns"])
	return vpc, cache, fleet, secondary, reportArns
}

func decodeCodeBuildStubObject(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	return m
}

func decodeCodeBuildStubArray(raw string) []any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var a []any
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil
	}
	return a
}

func decodeCodeBuildStubStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var a []string
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil
	}
	return a
}

func anyToBool(v any, defaultVal bool) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		default:
			return defaultVal
		}
	case float64:
		return t != 0
	default:
		return defaultVal
	}
}

func parseCodeBuildBatchChildren(params map[string]any) []store.CodeBuildBatchChild {
	if raw, ok := params["matrix"].([]any); ok && len(raw) > 0 {
		out := make([]store.CodeBuildBatchChild, 0, len(raw))
		for i, item := range raw {
			switch t := item.(type) {
			case []any:
				out = append(out, store.CodeBuildBatchChild{
					Identifier: fmt.Sprintf("MATRIX_%d", i),
					EnvVars:    parseCodeBuildEnvVars(t),
				})
			case map[string]any:
				id := stringParam(t["identifier"])
				if id == "" {
					id = fmt.Sprintf("MATRIX_%d", i)
				}
				var envs []store.CodeBuildEnvVar
				if ev, ok := t["environmentVariables"]; ok {
					envs = parseCodeBuildEnvVars(ev)
				} else if ev, ok := t["env"]; ok {
					if m, ok := ev.(map[string]any); ok {
						if vars, ok := m["variables"].(map[string]any); ok {
							for k, raw := range vars {
								envs = append(envs, store.CodeBuildEnvVar{Name: k, Value: stringParam(raw)})
							}
						}
					}
				} else {
					envs = parseCodeBuildEnvVars(t["environmentVariablesOverride"])
				}
				out = append(out, store.CodeBuildBatchChild{Identifier: id, EnvVars: envs})
			}
		}
		return out
	}
	if raw, ok := params["buildList"].([]any); ok && len(raw) > 0 {
		out := make([]store.CodeBuildBatchChild, 0, len(raw))
		for i, item := range raw {
			m, _ := item.(map[string]any)
			id := stringParam(m["identifier"])
			if id == "" {
				id = fmt.Sprintf("BUILD_%d", i)
			}
			out = append(out, store.CodeBuildBatchChild{
				Identifier: id,
				EnvVars:    parseCodeBuildEnvVars(m["environmentVariables"]),
			})
		}
		return out
	}
	return nil
}

func (s *Server) checkCodeBuildPassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("serviceRole must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("serviceRole must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("serviceRole not found")
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to CodeBuild")
	}
	decision := authz.CheckPassRole(authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal:     verified.Principal,
			Resource:      roleARN,
			Region:        verified.Region,
			ConditionKeys: s.conditionKeys(verified),
		},
		EvalInputs:       in,
		RoleARN:          roleARN,
		TrustPolicyDoc:   trust,
		ServicePrincipal: authz.ServicePrincipalCodeBuild,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to CodeBuild")
	}
	return nil
}

func (s *Server) codebuildCreateProject(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["name"])
	serviceRole := stringParam(params["serviceRole"])
	source, _ := params["source"].(map[string]any)
	env, _ := params["environment"].(map[string]any)
	artifacts, _ := params["artifacts"].(map[string]any)

	sourceType := stringParam(source["type"])
	sourceLoc := stringParam(source["location"])
	buildspec := stringParam(source["buildspec"])
	image := stringParam(env["image"])
	envVars := parseCodeBuildEnvVars(env["environmentVariables"])
	vpc, cache, fleet, secondary, reportArns := parseCodeBuildConfigStubs(params)

	resource := store.CodeBuildProjectARN(s.codebuildRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeBuildCreateProject, resource) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codebuild:CreateProject.", readOnly, eventID, verified)
		return
	}
	if err := s.checkCodeBuildPassRole(verified, serviceRole); err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}

	p, err := s.store.CreateCodeBuildProject(verified.AccountID, s.codebuildRegion(verified), store.CreateCodeBuildProjectInput{
		Name:             name,
		Description:      stringParam(params["description"]),
		ServiceRole:      serviceRole,
		SourceType:       sourceType,
		SourceLoc:        sourceLoc,
		Buildspec:        buildspec,
		Image:            image,
		Artifacts:        artifacts,
		EnvVars:          envVars,
		OverrideAllowed:  parseCodeBuildAllowOverride(source, params),
		VpcConfig:        vpc,
		Cache:            cache,
		SecondarySources: secondary,
		Fleet:            fleet,
		ReportGroupArns:  reportArns,
	})
	if errors.Is(err, store.ErrCodeBuildProjectExists) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceAlreadyExistsException",
			"Project already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeBuildInvalidInput) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create project.", readOnly, eventID, verified)
		return
	}
	payload, err := cbsvc.CreateProjectJSON(p)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "CreateProject", readOnly)
}

func (s *Server) codebuildUpdateProject(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["name"])
	resource := store.CodeBuildProjectARN(s.codebuildRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeBuildUpdateProject, resource) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codebuild:UpdateProject.", readOnly, eventID, verified)
		return
	}

	existing, err := s.store.GetCodeBuildProject(verified.AccountID, name)
	if errors.Is(err, store.ErrCodeBuildProjectNotFound) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Project not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load project.", readOnly, eventID, verified)
		return
	}

	serviceRole := stringParam(params["serviceRole"])
	if serviceRole == "" {
		serviceRole = existing.ServiceRole
	}
	if serviceRole != existing.ServiceRole {
		if err := s.checkCodeBuildPassRole(verified, serviceRole); err != nil {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}

	source, _ := params["source"].(map[string]any)
	env, _ := params["environment"].(map[string]any)
	artifacts, _ := params["artifacts"].(map[string]any)

	sourceType := stringParam(source["type"])
	if sourceType == "" {
		sourceType = existing.SourceType
	}
	sourceLoc := stringParam(source["location"])
	if source == nil {
		sourceLoc = existing.SourceLoc
	}
	buildspec := stringParam(source["buildspec"])
	if source == nil {
		buildspec = existing.Buildspec
	}
	image := stringParam(env["image"])
	if image == "" {
		image = existing.Image
	}
	desc := stringParam(params["description"])
	if _, ok := params["description"]; !ok {
		desc = existing.Description
	}

	var envVars []store.CodeBuildEnvVar
	if env != nil {
		if _, ok := env["environmentVariables"]; ok {
			envVars = parseCodeBuildEnvVars(env["environmentVariables"])
		} else {
			_ = json.Unmarshal([]byte(existing.EnvVarsJSON), &envVars)
		}
	} else {
		_ = json.Unmarshal([]byte(existing.EnvVarsJSON), &envVars)
	}
	if artifacts == nil && existing.Artifacts != "" {
		_ = json.Unmarshal([]byte(existing.Artifacts), &artifacts)
	}

	vpc, cache, fleet, secondary, reportArns := parseCodeBuildConfigStubs(params)
	if _, ok := params["vpcConfig"]; !ok {
		vpc = decodeCodeBuildStubObject(existing.VpcConfigJSON)
	}
	if _, ok := params["cache"]; !ok {
		cache = decodeCodeBuildStubObject(existing.CacheJSON)
	}
	if _, ok := params["fleet"]; !ok {
		if _, ok2 := params["projectFleet"]; !ok2 {
			fleet = decodeCodeBuildStubObject(existing.FleetJSON)
		}
	}
	if _, ok := params["secondarySources"]; !ok {
		secondary = decodeCodeBuildStubArray(existing.SecondarySourcesJSON)
	}
	if _, ok := params["reportGroupArns"]; !ok {
		reportArns = decodeCodeBuildStubStringSlice(existing.ReportArnsJSON)
	}

	p, err := s.store.UpdateCodeBuildProject(verified.AccountID, s.codebuildRegion(verified), store.UpdateCodeBuildProjectInput{
		Name:             name,
		Description:      desc,
		ServiceRole:      serviceRole,
		SourceType:       sourceType,
		SourceLoc:        sourceLoc,
		Buildspec:        buildspec,
		Image:            image,
		Artifacts:        artifacts,
		EnvVars:          envVars,
		OverrideAllowed:  parseCodeBuildAllowOverride(source, params),
		VpcConfig:        vpc,
		Cache:            cache,
		SecondarySources: secondary,
		Fleet:            fleet,
		ReportGroupArns:  reportArns,
	})
	if errors.Is(err, store.ErrCodeBuildProjectNotFound) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Project not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeBuildInvalidInput) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update project.", readOnly, eventID, verified)
		return
	}
	payload, err := cbsvc.UpdateProjectJSON(p)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "UpdateProject", readOnly)
}

func (s *Server) codebuildDeleteProject(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["name"])
	resource := store.CodeBuildProjectARN(s.codebuildRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeBuildDeleteProject, resource) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codebuild:DeleteProject.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteCodeBuildProject(verified.AccountID, name)
	if errors.Is(err, store.ErrCodeBuildProjectNotFound) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Project not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeBuildInvalidInput) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete project.", readOnly, eventID, verified)
		return
	}
	payload, err := cbsvc.DeleteProjectJSON()
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "DeleteProject", readOnly)
}

func (s *Server) codebuildListProjects(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionCodeBuildListProjects, "*") {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codebuild:ListProjects.", readOnly, eventID, verified)
		return
	}
	projects, err := s.store.ListCodeBuildProjects(verified.AccountID)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list projects.", readOnly, eventID, verified)
		return
	}
	payload, err := cbsvc.ListProjectsJSON(projects)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "ListProjects", readOnly)
}

func (s *Server) codebuildBatchGetProjects(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	names := stringSliceParam(params["names"])
	if len(names) == 0 {
		if !s.authorize(verified, catalog.ActionCodeBuildBatchGetProjects, "*") {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform codebuild:BatchGetProjects.", readOnly, eventID, verified)
			return
		}
	} else {
		for _, name := range names {
			resource := store.CodeBuildProjectARN(s.codebuildRegion(verified), verified.AccountID, name)
			if !s.authorize(verified, catalog.ActionCodeBuildBatchGetProjects, resource) {
				s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
					"User is not authorized to perform codebuild:BatchGetProjects.", readOnly, eventID, verified)
				return
			}
		}
	}
	projects, err := s.store.BatchGetCodeBuildProjects(verified.AccountID, names)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get projects.", readOnly, eventID, verified)
		return
	}
	found := make(map[string]struct{}, len(projects))
	for _, p := range projects {
		found[p.Name] = struct{}{}
	}
	var notFound []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := found[name]; !ok {
			notFound = append(notFound, name)
		}
	}
	payload, err := cbsvc.BatchGetProjectsJSON(projects, notFound)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "BatchGetProjects", readOnly)
}

func (s *Server) codebuildStartBuild(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	projectName := stringParam(params["projectName"])
	resource := store.CodeBuildProjectARN(s.codebuildRegion(verified), verified.AccountID, projectName)
	if projectName == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeBuildStartBuild, resource) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codebuild:StartBuild.", readOnly, eventID, verified)
		return
	}

	opts := store.StartCodeBuildBuildOpts{
		ProjectName:       projectName,
		BuildspecOverride: stringParam(params["buildspecOverride"]),
		EnvOverride:       parseCodeBuildEnvVars(params["environmentVariablesOverride"]),
	}

	// Validate project + override-lock before the compute gate so InvalidInputException
	// is observable without Docker.
	if projectName != "" {
		p, err := s.store.GetCodeBuildProject(verified.AccountID, projectName)
		if errors.Is(err, store.ErrCodeBuildProjectNotFound) {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
				"Project not found.", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to load project.", readOnly, eventID, verified)
			return
		}
		if !p.OverrideAllowed && (strings.TrimSpace(opts.BuildspecOverride) != "" || len(opts.EnvOverride) > 0) {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
				"project does not allow buildspecOverride or environmentVariablesOverride", readOnly, eventID, verified)
			return
		}
		if strings.EqualFold(p.SourceType, "CODECOMMIT") {
			if _, err := s.store.EnsureCodeBuildCodeCommitSource(verified.AccountID, p.SourceLoc); err != nil {
				s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
					err.Error(), readOnly, eventID, verified)
				return
			}
		}
	}

	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
			"compute unavailable", readOnly, eventID, verified)
		return
	}

	b, err := s.store.StartCodeBuildBuild(verified.AccountID, s.codebuildRegion(verified), opts)
	if errors.Is(err, store.ErrCodeBuildProjectNotFound) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Project not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeBuildInvalidInput) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to start build.", readOnly, eventID, verified)
		return
	}

	if err := s.startCodeBuildContainer(r.Context(), verified.AccountID, b); err != nil {
		_ = s.store.SetCodeBuildBuildRuntime(verified.AccountID, b.ID, "", store.CodeBuildStatusFailed, time.Now().UTC().Format(time.RFC3339))
		if strings.Contains(err.Error(), "compute unavailable") {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
				"compute unavailable", readOnly, eventID, verified)
			return
		}
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			err.Error(), readOnly, eventID, verified)
		return
	}

	builds, err := s.store.BatchGetCodeBuildBuilds(verified.AccountID, []string{b.ID})
	if err != nil || len(builds) == 0 {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load build.", readOnly, eventID, verified)
		return
	}
	payload, err := cbsvc.StartBuildJSON(builds[0])
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "StartBuild", readOnly)
}

func (s *Server) codebuildStartBuildBatch(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	projectName := stringParam(params["projectName"])
	resource := store.CodeBuildProjectARN(s.codebuildRegion(verified), verified.AccountID, projectName)
	if projectName == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeBuildStartBuildBatch, resource) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codebuild:StartBuildBatch.", readOnly, eventID, verified)
		return
	}

	opts := store.StartCodeBuildBuildBatchOpts{
		ProjectName:       projectName,
		BuildspecOverride: stringParam(params["buildspecOverride"]),
		EnvOverride:       parseCodeBuildEnvVars(params["environmentVariablesOverride"]),
		Children:          parseCodeBuildBatchChildren(params),
	}

	if projectName != "" {
		p, err := s.store.GetCodeBuildProject(verified.AccountID, projectName)
		if errors.Is(err, store.ErrCodeBuildProjectNotFound) {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
				"Project not found.", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to load project.", readOnly, eventID, verified)
			return
		}
		if !p.OverrideAllowed && (strings.TrimSpace(opts.BuildspecOverride) != "" || len(opts.EnvOverride) > 0) {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
				"project does not allow buildspecOverride or environmentVariablesOverride", readOnly, eventID, verified)
			return
		}
		if strings.EqualFold(p.SourceType, "CODECOMMIT") {
			if _, err := s.store.EnsureCodeBuildCodeCommitSource(verified.AccountID, p.SourceLoc); err != nil {
				s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
					err.Error(), readOnly, eventID, verified)
				return
			}
		}
	}

	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
			"compute unavailable", readOnly, eventID, verified)
		return
	}

	batch, builds, err := s.store.StartCodeBuildBuildBatch(verified.AccountID, s.codebuildRegion(verified), opts)
	if errors.Is(err, store.ErrCodeBuildProjectNotFound) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Project not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeBuildInvalidInput) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to start build batch.", readOnly, eventID, verified)
		return
	}

	var startErr error
	for i := range builds {
		if err := s.startCodeBuildContainer(r.Context(), verified.AccountID, builds[i]); err != nil {
			_ = s.store.SetCodeBuildBuildRuntime(verified.AccountID, builds[i].ID, "", store.CodeBuildStatusFailed, time.Now().UTC().Format(time.RFC3339))
			startErr = err
		}
	}
	if startErr != nil {
		if strings.Contains(startErr.Error(), "compute unavailable") {
			s.writeCodeBuildError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
				"compute unavailable", readOnly, eventID, verified)
			return
		}
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			startErr.Error(), readOnly, eventID, verified)
		return
	}

	payload, err := cbsvc.StartBuildBatchJSON(batch, builds)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "StartBuildBatch", readOnly)
}

func (s *Server) codebuildStopBuild(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id := stringParam(params["id"])
	resource := "*"
	if id != "" {
		// Prefer project ARN from id shape project:uuid when possible; else "*".
		if strings.Contains(id, ":build/") {
			resource = id
		} else {
			resource = "*"
		}
	}
	if !s.authorize(verified, catalog.ActionCodeBuildStopBuild, resource) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codebuild:StopBuild.", readOnly, eventID, verified)
		return
	}
	b, err := s.store.StopCodeBuildBuild(verified.AccountID, id)
	if errors.Is(err, store.ErrCodeBuildBuildNotFound) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Build not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to stop build.", readOnly, eventID, verified)
		return
	}
	if b.ContainerID != "" {
		if cli, cerr := s.computeClient(); cerr == nil && cli != nil {
			_ = cli.StopECSTask(r.Context(), b.ContainerID)
		}
	}
	payload, err := cbsvc.StopBuildJSON(b)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "StopBuild", readOnly)
}

// startCodeBuildContainer starts the nested container and returns IN_PROGRESS; exit reap is background.
// Nested CodeBuild uses compute.RunECSTask, which honors NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1 so
// containers can resolve host.docker.internal for the lab endpoint.
func (s *Server) startCodeBuildContainer(ctx context.Context, accountID string, b store.CodeBuildBuild) error {
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return errors.New("compute unavailable")
	}
	buildspec, err := s.store.ResolveCodeBuildBuildspec(accountID, b)
	if err != nil {
		return err
	}
	cmds := store.ExtractBuildspecCommands(buildspec)
	if len(cmds) == 0 {
		return fmt.Errorf("%w: no build commands", store.ErrCodeBuildInvalidInput)
	}
	script := strings.Join(cmds, " && ")
	if strings.EqualFold(b.SourceType, "CODECOMMIT") {
		tmp, mkErr := os.MkdirTemp("", "noctaxris-codebuild-cc-*")
		if mkErr != nil {
			return fmt.Errorf("codebuild CODECOMMIT temp dir: %w", mkErr)
		}
		defer os.RemoveAll(tmp)
		if _, matErr := s.store.MaterializeCodeBuildCodeCommitSource(accountID, b.SourceLoc, tmp); matErr != nil {
			return matErr
		}
		script, err = store.BuildCodeBuildCodeCommitShell(tmp, cmds)
		if err != nil {
			return err
		}
	}
	endpoint := strings.TrimSpace(s.cfg.LambdaEndpointURL)
	if endpoint == "" {
		endpoint = defaultCodeBuildEndpoint
	}
	env := map[string]string{
		"CODEBUILD_BUILD_ID":         b.ID,
		"CODEBUILD_PROJECT_NAME":     b.ProjectName,
		"AWS_ENDPOINT_URL_CODEBUILD": endpoint,
	}
	proj, err := s.store.GetCodeBuildProject(accountID, b.ProjectName)
	if err != nil {
		return err
	}
	for _, v := range decodeCodeBuildEnvVarsJSON(proj.EnvVarsJSON) {
		if v.Name != "" {
			env[v.Name] = v.Value
		}
	}
	for _, v := range decodeCodeBuildEnvVarsJSON(b.EnvVarsJSON) {
		if v.Name != "" {
			env[v.Name] = v.Value
		}
	}
	serviceRole := strings.TrimSpace(proj.ServiceRole)
	if serviceRole != "" {
		minted, mintErr := s.mintRoleSessionEnv(serviceRole, codebuildSession, endpoint, store.DefaultCodeBuildRegion)
		if mintErr != nil {
			return mintErr
		}
		for k, v := range minted {
			env[k] = v
		}
	}
	pullRef, useAuth, username, password, err := s.labRegistryPullOpts(accountID, b.Image, serviceRole)
	if err != nil {
		return err
	}
	cid, err := cli.RunECSTask(ctx, compute.ECSRunOpts{
		ImageURI:         pullRef,
		Command:          []string{"/bin/sh", "-c", script},
		EndpointURL:      endpoint,
		Env:              env,
		ListenAddr:       s.cfg.ListenAddr,
		LabRegistryPull:  useAuth,
		RegistryUsername: username,
		RegistryPassword: password,
	})
	if err != nil {
		return err
	}
	if err := s.store.SetCodeBuildBuildRuntime(accountID, b.ID, cid, store.CodeBuildStatusInProgress, ""); err != nil {
		_ = cli.StopECSTask(ctx, cid)
		return err
	}
	go s.reapCodeBuild(accountID, b.ID, b.ProjectName, cid)
	return nil
}

func decodeCodeBuildEnvVarsJSON(raw string) []store.CodeBuildEnvVar {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []store.CodeBuildEnvVar
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func (s *Server) reapCodeBuild(accountID, buildID, projectName, containerID string) {
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	exitCode, waitErr := cli.WaitECSTaskExit(ctx, containerID)
	status := store.CodeBuildStatusSucceeded
	if waitErr != nil || exitCode != 0 {
		status = store.CodeBuildStatusFailed
	}

	logsText := ""
	if text, logErr := cli.DataPlaneLogs(ctx, containerID); logErr == nil {
		logsText = text
	}
	logGroup := "/aws/codebuild/" + projectName
	logStream := buildID
	_ = s.store.SetCodeBuildBuildLogs(accountID, buildID, logsText, logGroup, logStream)
	s.shipCodeBuildLogsBestEffort(accountID, logGroup, logStream, logsText)

	_ = cli.StopECSTask(context.Background(), containerID)
	_ = s.store.SetCodeBuildBuildRuntime(accountID, buildID, containerID, status, time.Now().UTC().Format(time.RFC3339))

	if status == store.CodeBuildStatusSucceeded {
		builds, err := s.store.BatchGetCodeBuildBuilds(accountID, []string{buildID})
		if err == nil && len(builds) == 1 {
			_, _ = s.store.PublishCodeBuildBuildArtifacts(accountID, builds[0])
		}
	}
}

func (s *Server) shipCodeBuildLogsBestEffort(accountID, group, stream, logsText string) {
	if s.store == nil || strings.TrimSpace(logsText) == "" {
		return
	}
	region := store.DefaultCodeBuildRegion
	if _, err := s.store.EnsureLogGroup(accountID, region, group); err != nil {
		return
	}
	if _, err := s.store.EnsureLogStream(accountID, region, group, stream); err != nil {
		return
	}
	now := time.Now().UTC().UnixMilli()
	var events []store.LogEvent
	for _, line := range strings.Split(logsText, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		events = append(events, store.LogEvent{Timestamp: now, Message: line})
	}
	if len(events) == 0 {
		return
	}
	_, _, _ = s.store.PutLogEvents(accountID, group, stream, "", events)
}

func (s *Server) codebuildBatchGetBuilds(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCodeBuildBatchGetBuilds, "*") {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codebuild:BatchGetBuilds.", readOnly, eventID, verified)
		return
	}
	ids := stringSliceParam(params["ids"])
	builds, err := s.store.BatchGetCodeBuildBuilds(verified.AccountID, ids)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get builds.", readOnly, eventID, verified)
		return
	}
	payload, err := cbsvc.BatchGetBuildsJSON(builds)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "BatchGetBuilds", readOnly)
}

func (s *Server) codebuildListBuilds(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCodeBuildListBuilds, "*") {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codebuild:ListBuilds.", readOnly, eventID, verified)
		return
	}
	projectName := stringParam(params["projectName"])
	ids, err := s.store.ListCodeBuildBuilds(verified.AccountID, projectName)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list builds.", readOnly, eventID, verified)
		return
	}
	payload, err := cbsvc.ListBuildsJSON(ids)
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeBuildOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codebuildEventSource, "ListBuilds", readOnly)
}

func (s *Server) writeCodeBuildOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", codebuildJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCodeBuildError(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID string,
	status int,
	code, message string,
	readOnly bool,
	eventID string,
	verified *authn.Verified,
) {
	_ = body
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", codebuildJSONContentType)
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{
		"__type":  code,
		"message": message,
	})
	_, _ = w.Write(payload)

	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}
