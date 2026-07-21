package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	case catalog.ActionCodeBuildStartBuild:
		s.codebuildStartBuild(w, r, body, requestID, eventID, verified, readOnly, params)
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
	case "StartBuild":
		return catalog.ActionCodeBuildStartBuild
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
		Name:        name,
		Description: stringParam(params["description"]),
		ServiceRole: serviceRole,
		SourceType:  sourceType,
		SourceLoc:   sourceLoc,
		Buildspec:   buildspec,
		Image:       image,
		Artifacts:   artifacts,
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
	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
			"compute unavailable", readOnly, eventID, verified)
		return
	}

	b, err := s.store.StartCodeBuildBuild(verified.AccountID, s.codebuildRegion(verified), projectName, stringParam(params["buildspecOverride"]))
	if errors.Is(err, store.ErrCodeBuildProjectNotFound) {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Project not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeBuildError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to start build.", readOnly, eventID, verified)
		return
	}

	if err := s.executeCodeBuild(r.Context(), verified.AccountID, b); err != nil {
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

func (s *Server) executeCodeBuild(ctx context.Context, accountID string, b store.CodeBuildBuild) error {
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
	endpoint := strings.TrimSpace(s.cfg.LambdaEndpointURL)
	if endpoint == "" {
		endpoint = defaultCodeBuildEndpoint
	}
	cid, err := cli.RunECSTask(ctx, compute.ECSRunOpts{
		ImageURI:    b.Image,
		Command:     []string{"/bin/sh", "-c", script},
		EndpointURL: endpoint,
		Env: map[string]string{
			"CODEBUILD_BUILD_ID":      b.ID,
			"CODEBUILD_PROJECT_NAME":  b.ProjectName,
			"AWS_ENDPOINT_URL_CODEBUILD": endpoint,
		},
	})
	if err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	exitCode, waitErr := cli.WaitECSTaskExit(waitCtx, cid)
	end := time.Now().UTC().Format(time.RFC3339)
	status := store.CodeBuildStatusSucceeded
	if waitErr != nil {
		status = store.CodeBuildStatusFailed
	} else if exitCode != 0 {
		status = store.CodeBuildStatusFailed
	} else {
		running, runErr := cli.ContainerRunning(ctx, cid)
		if runErr == nil && running {
			status = store.CodeBuildStatusInProgress
		}
	}
	return s.store.SetCodeBuildBuildRuntime(accountID, b.ID, cid, status, end)
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
