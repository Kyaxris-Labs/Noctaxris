package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	cdsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/codedeploy"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	codeDeployJSONContentType = "application/x-amz-json-1.1"
	codeDeployEventSource     = "codedeploy.amazonaws.com"
)

func (s *Server) handleCodeDeploy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = codeDeployAction(action)

	switch action {
	case catalog.ActionCodeDeployCreateApplication:
		s.cdCreateApplication(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeDeployCreateDeploymentGroup:
		s.cdCreateDeploymentGroup(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeDeployCreateDeployment:
		s.cdCreateDeployment(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeDeployGetDeployment:
		s.cdGetDeployment(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeDeployListDeployments:
		s.cdListDeployments(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCodeDeployError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This CodeDeploy action is not implemented.", readOnly, eventID, verified)
	}
}

func codeDeployAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateApplication":
		return catalog.ActionCodeDeployCreateApplication
	case "CreateDeploymentGroup":
		return catalog.ActionCodeDeployCreateDeploymentGroup
	case "CreateDeployment":
		return catalog.ActionCodeDeployCreateDeployment
	case "GetDeployment":
		return catalog.ActionCodeDeployGetDeployment
	case "ListDeployments":
		return catalog.ActionCodeDeployListDeployments
	default:
		return action
	}
}

func (s *Server) checkCodeDeployPassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("serviceRoleArn must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("serviceRoleArn must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("Role not found")
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to CodeDeploy")
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
		ServicePrincipal: authz.ServicePrincipalCodeDeploy,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to CodeDeploy")
	}
	return nil
}

func jsonString(params map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := params[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func (s *Server) cdCreateApplication(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCodeDeployCreateApplication, "*") {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codedeploy:CreateApplication.", readOnly, eventID, verified)
		return
	}
	name := jsonString(params, "applicationName", "ApplicationName")
	platform := jsonString(params, "computePlatform", "ComputePlatform")
	app, err := s.store.CreateCodeDeployApplication(verified.AccountID, name, platform)
	if errors.Is(err, store.ErrCodeDeployExists) {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusBadRequest, "ApplicationAlreadyExistsException",
			"Application already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeDeployBadRequest) {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusBadRequest, "InvalidApplicationNameException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create application.", readOnly, eventID, verified)
		return
	}
	payload, _ := cdsvc.CreateApplicationJSON(app)
	s.writeCodeDeployOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codeDeployEventSource, "CreateApplication", readOnly)
}

func (s *Server) cdCreateDeploymentGroup(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCodeDeployCreateDeploymentGroup, "*") {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codedeploy:CreateDeploymentGroup.", readOnly, eventID, verified)
		return
	}
	appName := jsonString(params, "applicationName", "ApplicationName")
	dgName := jsonString(params, "deploymentGroupName", "DeploymentGroupName")
	roleARN := jsonString(params, "serviceRoleArn", "ServiceRoleArn")
	ecsService, ecsCluster, lambdaFn := "", "", ""
	if ecsList, ok := params["ecsServices"].([]any); ok && len(ecsList) > 0 {
		if m, ok := ecsList[0].(map[string]any); ok {
			ecsService = jsonString(m, "serviceName", "ServiceName")
			ecsCluster = jsonString(m, "clusterName", "ClusterName")
		}
	}
	if cfg, ok := params["ecsServices"].(map[string]any); ok {
		ecsService = jsonString(cfg, "serviceName", "ServiceName")
		ecsCluster = jsonString(cfg, "clusterName", "ClusterName")
	}
	lambdaFn = jsonString(params, "lambdaFunctionName", "LambdaFunctionName")
	if roleARN != "" {
		if err := s.checkCodeDeployPassRole(verified, roleARN); err != nil {
			s.writeCodeDeployError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	g, err := s.store.CreateCodeDeployDeploymentGroup(verified.AccountID, appName, dgName, roleARN, ecsService, ecsCluster, lambdaFn)
	if errors.Is(err, store.ErrCodeDeployNotFound) {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusBadRequest, "ApplicationDoesNotExistException",
			"Application does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeDeployDGExists) {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusBadRequest, "DeploymentGroupAlreadyExistsException",
			"Deployment group already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeDeployBadRequest) {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create deployment group.", readOnly, eventID, verified)
		return
	}
	payload, _ := cdsvc.CreateDeploymentGroupJSON(g)
	s.writeCodeDeployOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codeDeployEventSource, "CreateDeploymentGroup", readOnly)
}

func (s *Server) cdCreateDeployment(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCodeDeployCreateDeployment, "*") {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codedeploy:CreateDeployment.", readOnly, eventID, verified)
		return
	}
	appName := jsonString(params, "applicationName", "ApplicationName")
	dgName := jsonString(params, "deploymentGroupName", "DeploymentGroupName")
	description := jsonString(params, "description", "Description")
	dg, err := s.store.GetCodeDeployDeploymentGroup(verified.AccountID, appName, dgName)
	if err != nil {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusBadRequest, "DeploymentGroupDoesNotExistException",
			"Deployment group does not exist.", readOnly, eventID, verified)
		return
	}
	// Optional: bump ECS DesiredCount when group references a service (best-effort lab hook).
	if dg.ECSServiceName != "" {
		cluster := dg.ECSClusterName
		if cluster == "" {
			cluster = "default"
		}
		if svc, getErr := s.store.GetService(verified.AccountID, cluster, dg.ECSServiceName); getErr == nil {
			desired := svc.DesiredCount
			if desired < 1 {
				desired = 1
			}
			_, _ = s.store.UpdateService(verified.AccountID, verified.Region, store.UpdateServiceInput{
				Cluster:      cluster,
				Service:      dg.ECSServiceName,
				DesiredCount: &desired,
			})
		}
	}
	dep, err := s.store.CreateCodeDeployDeployment(verified.AccountID, appName, dgName, description)
	if err != nil {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create deployment.", readOnly, eventID, verified)
		return
	}
	payload, _ := cdsvc.CreateDeploymentJSON(dep)
	s.writeCodeDeployOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codeDeployEventSource, "CreateDeployment", readOnly)
}

func (s *Server) cdGetDeployment(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id := jsonString(params, "deploymentId", "DeploymentId")
	if !s.authorize(verified, catalog.ActionCodeDeployGetDeployment, "*") {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codedeploy:GetDeployment.", readOnly, eventID, verified)
		return
	}
	dep, err := s.store.GetCodeDeployDeployment(verified.AccountID, id)
	if errors.Is(err, store.ErrCodeDeployNotFound) {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusBadRequest, "DeploymentDoesNotExistException",
			"Deployment not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get deployment.", readOnly, eventID, verified)
		return
	}
	payload, _ := cdsvc.GetDeploymentJSON(dep)
	s.writeCodeDeployOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codeDeployEventSource, "GetDeployment", readOnly)
}

func (s *Server) cdListDeployments(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCodeDeployListDeployments, "*") {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codedeploy:ListDeployments.", readOnly, eventID, verified)
		return
	}
	appName := jsonString(params, "applicationName", "ApplicationName")
	deps, err := s.store.ListCodeDeployDeployments(verified.AccountID, appName)
	if err != nil {
		s.writeCodeDeployError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list deployments.", readOnly, eventID, verified)
		return
	}
	payload, _ := cdsvc.ListDeploymentsJSON(deps)
	s.writeCodeDeployOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codeDeployEventSource, "ListDeployments", readOnly)
}

func (s *Server) writeCodeDeployOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", codeDeployJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCodeDeployError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", codeDeployJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, codeDeployEventSource, code, readOnly)
}
