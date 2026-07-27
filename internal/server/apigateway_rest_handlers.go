package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	apigw "github.com/Kyaxris-Labs/Noctaxris/internal/services/apigateway"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func (s *Server) handleAPIGatewayREST(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = apiGatewayRESTAction(action)

	switch action {
	case catalog.ActionAPIGatewayCreateRestApi:
		s.apigwRESTCreateRestApi(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayGetRestApi:
		s.apigwRESTGetRestApi(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayGetRestApis:
		s.apigwRESTGetRestApis(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayDeleteRestApi:
		s.apigwRESTDeleteRestApi(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayCreateResource:
		s.apigwRESTCreateResource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayGetResources:
		s.apigwRESTGetResources(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayDeleteResource:
		s.apigwRESTDeleteResource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayPutMethod:
		s.apigwRESTPutMethod(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayGetMethod:
		s.apigwRESTGetMethod(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayDeleteMethod:
		s.apigwRESTDeleteMethod(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayPutIntegration:
		s.apigwRESTPutIntegration(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayGetIntegration:
		s.apigwRESTGetIntegration(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayCreateDeployment:
		s.apigwRESTCreateDeployment(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayCreateStage:
		s.apigwRESTCreateStage(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayGetStage:
		s.apigwRESTGetStage(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotImplemented, "BadRequestException",
			"This API Gateway REST action is not implemented.", readOnly, eventID, verified)
	}
}

func apiGatewayRESTAction(action string) string {
	if strings.HasPrefix(action, "apigateway:") {
		return action
	}
	switch action {
	case "CreateRestApi":
		return catalog.ActionAPIGatewayCreateRestApi
	case "GetRestApi":
		return catalog.ActionAPIGatewayGetRestApi
	case "GetRestApis":
		return catalog.ActionAPIGatewayGetRestApis
	case "DeleteRestApi":
		return catalog.ActionAPIGatewayDeleteRestApi
	case "CreateResource":
		return catalog.ActionAPIGatewayCreateResource
	case "GetResources":
		return catalog.ActionAPIGatewayGetResources
	case "DeleteResource":
		return catalog.ActionAPIGatewayDeleteResource
	case "PutMethod":
		return catalog.ActionAPIGatewayPutMethod
	case "GetMethod":
		return catalog.ActionAPIGatewayGetMethod
	case "DeleteMethod":
		return catalog.ActionAPIGatewayDeleteMethod
	case "PutIntegration":
		return catalog.ActionAPIGatewayPutIntegration
	case "GetIntegration":
		return catalog.ActionAPIGatewayGetIntegration
	case "CreateDeployment":
		return catalog.ActionAPIGatewayCreateDeployment
	case "CreateStage":
		return catalog.ActionAPIGatewayCreateStage
	case "GetStage":
		return catalog.ActionAPIGatewayGetStage
	default:
		return action
	}
}

func isAPIGatewayRESTAction(action string) bool {
	a := apiGatewayRESTAction(action)
	switch a {
	case catalog.ActionAPIGatewayCreateRestApi,
		catalog.ActionAPIGatewayGetRestApi,
		catalog.ActionAPIGatewayGetRestApis,
		catalog.ActionAPIGatewayDeleteRestApi,
		catalog.ActionAPIGatewayCreateResource,
		catalog.ActionAPIGatewayGetResources,
		catalog.ActionAPIGatewayDeleteResource,
		catalog.ActionAPIGatewayPutMethod,
		catalog.ActionAPIGatewayGetMethod,
		catalog.ActionAPIGatewayDeleteMethod,
		catalog.ActionAPIGatewayPutIntegration,
		catalog.ActionAPIGatewayGetIntegration,
		catalog.ActionAPIGatewayCreateDeployment,
		catalog.ActionAPIGatewayCreateStage,
		catalog.ActionAPIGatewayGetStage:
		return true
	default:
		return false
	}
}

// isAPIGatewayRESTMgmtPath reports REST API v1 control-plane paths (not execute).
func isAPIGatewayRESTMgmtPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 || parts[0] != "restapis" {
		return false
	}
	if len(parts) >= 4 && parts[3] == "_user_request_" {
		return false
	}
	return true
}

// resolveAPIGatewayREST maps /restapis/... control-plane paths used by aws apigateway.
func resolveAPIGatewayREST(r *http.Request, body []byte) (action string, outBody []byte) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 1 || parts[0] != "restapis" {
		return "", body
	}
	if len(parts) >= 4 && parts[3] == "_user_request_" {
		return "", body
	}

	switch r.Method {
	case http.MethodPost:
		if len(parts) == 1 {
			return catalog.ActionAPIGatewayCreateRestApi, body
		}
		if len(parts) == 3 && parts[2] == "deployments" {
			return catalog.ActionAPIGatewayCreateDeployment, injectJSONStringField(body, "restApiId", parts[1])
		}
		if len(parts) == 3 && parts[2] == "stages" {
			return catalog.ActionAPIGatewayCreateStage, injectJSONStringField(body, "restApiId", parts[1])
		}
		if len(parts) == 4 && parts[2] == "resources" {
			body = injectJSONStringField(body, "restApiId", parts[1])
			return catalog.ActionAPIGatewayCreateResource, injectJSONStringField(body, "parentId", parts[3])
		}
	case http.MethodGet:
		if len(parts) == 1 {
			return catalog.ActionAPIGatewayGetRestApis, body
		}
		if len(parts) == 2 {
			return catalog.ActionAPIGatewayGetRestApi, injectJSONStringField(body, "restApiId", parts[1])
		}
		if len(parts) == 3 && parts[2] == "resources" {
			return catalog.ActionAPIGatewayGetResources, injectJSONStringField(body, "restApiId", parts[1])
		}
		if len(parts) == 4 && parts[2] == "stages" {
			body = injectJSONStringField(body, "restApiId", parts[1])
			return catalog.ActionAPIGatewayGetStage, injectJSONStringField(body, "stageName", parts[3])
		}
		if len(parts) == 5 && parts[2] == "resources" && parts[4] != "methods" {
			// unused
		}
		if len(parts) == 5 && parts[2] == "resources" && parts[4] == "methods" {
			// GET /restapis/{id}/resources/{rid}/methods — not a single method
		}
		if len(parts) == 6 && parts[2] == "resources" && parts[4] == "methods" {
			body = injectJSONStringField(body, "restApiId", parts[1])
			body = injectJSONStringField(body, "resourceId", parts[3])
			return catalog.ActionAPIGatewayGetMethod, injectJSONStringField(body, "pathHttpMethod", parts[5])
		}
		if len(parts) == 7 && parts[2] == "resources" && parts[4] == "methods" && parts[6] == "integration" {
			body = injectJSONStringField(body, "restApiId", parts[1])
			body = injectJSONStringField(body, "resourceId", parts[3])
			return catalog.ActionAPIGatewayGetIntegration, injectJSONStringField(body, "pathHttpMethod", parts[5])
		}
	case http.MethodPut:
		if len(parts) == 6 && parts[2] == "resources" && parts[4] == "methods" {
			body = injectJSONStringField(body, "restApiId", parts[1])
			body = injectJSONStringField(body, "resourceId", parts[3])
			return catalog.ActionAPIGatewayPutMethod, injectJSONStringField(body, "pathHttpMethod", parts[5])
		}
		if len(parts) == 7 && parts[2] == "resources" && parts[4] == "methods" && parts[6] == "integration" {
			body = injectJSONStringField(body, "restApiId", parts[1])
			body = injectJSONStringField(body, "resourceId", parts[3])
			return catalog.ActionAPIGatewayPutIntegration, injectJSONStringField(body, "pathHttpMethod", parts[5])
		}
	case http.MethodDelete:
		if len(parts) == 2 {
			return catalog.ActionAPIGatewayDeleteRestApi, injectJSONStringField(body, "restApiId", parts[1])
		}
		if len(parts) == 4 && parts[2] == "resources" {
			body = injectJSONStringField(body, "restApiId", parts[1])
			return catalog.ActionAPIGatewayDeleteResource, injectJSONStringField(body, "resourceId", parts[3])
		}
		if len(parts) == 6 && parts[2] == "resources" && parts[4] == "methods" {
			body = injectJSONStringField(body, "restApiId", parts[1])
			body = injectJSONStringField(body, "resourceId", parts[3])
			return catalog.ActionAPIGatewayDeleteMethod, injectJSONStringField(body, "pathHttpMethod", parts[5])
		}
	}
	return "", body
}

func paramString(params map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := params[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (s *Server) apigwRESTCreateRestApi(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := paramString(params, "name", "Name")
	desc := paramString(params, "description", "Description")
	if !s.authorize(verified, catalog.ActionAPIGatewayCreateRestApi, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:CreateRestApi.", readOnly, eventID, verified)
		return
	}
	api, err := s.store.CreateRestAPI(verified.AccountID, name, desc)
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create REST API.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.RestApiJSON(api)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateRestApi", readOnly)
}

func (s *Server) apigwRESTGetRestApi(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId", "apiId", "ApiId")
	if !s.authorize(verified, catalog.ActionAPIGatewayGetRestApi, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetRestApi.", readOnly, eventID, verified)
		return
	}
	api, err := s.store.GetRestAPI(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get REST API.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.RestApiJSON(api)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetRestApi", readOnly)
}

func (s *Server) apigwRESTGetRestApis(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionAPIGatewayGetRestApis, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetRestApis.", readOnly, eventID, verified)
		return
	}
	apis, err := s.store.ListRestAPIs(verified.AccountID)
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list REST APIs.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.RestApisJSON(apis)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetRestApis", readOnly)
}

func (s *Server) apigwRESTDeleteRestApi(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId", "apiId", "ApiId")
	if !s.authorize(verified, catalog.ActionAPIGatewayDeleteRestApi, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:DeleteRestApi.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteRestAPI(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete REST API.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "DeleteRestApi", readOnly)
}

func (s *Server) apigwRESTCreateResource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	parentID := paramString(params, "parentId", "ParentId")
	pathPart := paramString(params, "pathPart", "PathPart")
	if !s.authorize(verified, catalog.ActionAPIGatewayCreateResource, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:CreateResource.", readOnly, eventID, verified)
		return
	}
	res, err := s.store.CreateRestResource(verified.AccountID, apiID, parentID, pathPart)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid resource identifier specified", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) || errors.Is(err, store.ErrAPIGatewayConflict) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create resource.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.ResourceJSON(res)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateResource", readOnly)
}

func (s *Server) apigwRESTGetResources(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	if !s.authorize(verified, catalog.ActionAPIGatewayGetResources, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetResources.", readOnly, eventID, verified)
		return
	}
	items, err := s.store.ListRestResources(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list resources.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.ResourcesJSON(items)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetResources", readOnly)
}

func (s *Server) apigwRESTDeleteResource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	resourceID := paramString(params, "resourceId", "ResourceId")
	if !s.authorize(verified, catalog.ActionAPIGatewayDeleteResource, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:DeleteResource.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteRestResource(verified.AccountID, apiID, resourceID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid resource identifier specified", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete resource.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "DeleteResource", readOnly)
}

func restResourceHTTPMethod(params map[string]any) string {
	if v := paramString(params, "pathHttpMethod"); v != "" {
		return v
	}
	return paramString(params, "httpMethod", "HttpMethod")
}

func restIntegrationHTTPMethod(params map[string]any) string {
	if v := paramString(params, "integrationHttpMethod", "IntegrationHttpMethod"); v != "" {
		return v
	}
	// AWS REST body uses httpMethod for the integration HTTP method when path carries the resource method.
	if paramString(params, "pathHttpMethod") != "" {
		return paramString(params, "httpMethod", "HttpMethod")
	}
	return ""
}

func (s *Server) apigwRESTPutMethod(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	resourceID := paramString(params, "resourceId", "ResourceId")
	httpMethod := restResourceHTTPMethod(params)
	authType := paramString(params, "authorizationType", "AuthorizationType")
	authorizerID := paramString(params, "authorizerId", "AuthorizerId")
	apiKeyRequired := false
	if v, ok := params["apiKeyRequired"].(bool); ok {
		apiKeyRequired = v
	} else if v, ok := params["ApiKeyRequired"].(bool); ok {
		apiKeyRequired = v
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayPutMethod, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:PutMethod.", readOnly, eventID, verified)
		return
	}
	effectiveAuth := strings.ToUpper(strings.TrimSpace(authType))
	if effectiveAuth == "" {
		effectiveAuth = store.APIGatewayRESTAuthNone
	}
	if effectiveAuth == store.APIGatewayRESTAuthNone && !s.cfg.OpenDataPlaneAllowed() {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"authorizationType NONE requires NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1 when listen is non-loopback.",
			readOnly, eventID, verified)
		return
	}
	method, err := s.store.PutRestMethod(verified.AccountID, apiID, resourceID, httpMethod, authType, authorizerID, apiKeyRequired)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid resource identifier specified", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put method.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.MethodJSON(method)
	w.Header().Set("Content-Type", apiGatewayJSONContentType)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "PutMethod", readOnly)
}

func (s *Server) apigwRESTGetMethod(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	resourceID := paramString(params, "resourceId", "ResourceId")
	httpMethod := restResourceHTTPMethod(params)
	if !s.authorize(verified, catalog.ActionAPIGatewayGetMethod, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetMethod.", readOnly, eventID, verified)
		return
	}
	method, err := s.store.GetRestMethod(verified.AccountID, apiID, resourceID, httpMethod)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Method identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get method.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.MethodJSON(method)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetMethod", readOnly)
}

func (s *Server) apigwRESTDeleteMethod(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	resourceID := paramString(params, "resourceId", "ResourceId")
	httpMethod := restResourceHTTPMethod(params)
	if !s.authorize(verified, catalog.ActionAPIGatewayDeleteMethod, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:DeleteMethod.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteRestMethod(verified.AccountID, apiID, resourceID, httpMethod)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Method identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete method.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "DeleteMethod", readOnly)
}

func parseRequestTemplates(params map[string]any) map[string]string {
	out := map[string]string{}
	raw, ok := params["requestTemplates"]
	if !ok {
		raw, ok = params["RequestTemplates"]
	}
	if !ok {
		return out
	}
	switch m := raw.(type) {
	case map[string]any:
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	case map[string]string:
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func (s *Server) apigwRESTPutIntegration(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	resourceID := paramString(params, "resourceId", "ResourceId")
	httpMethod := restResourceHTTPMethod(params)
	intType := paramString(params, "type", "Type")
	uri := paramString(params, "uri", "Uri")
	intHTTP := restIntegrationHTTPMethod(params)
	credentials := paramString(params, "credentials", "Credentials")
	templates := parseRequestTemplates(params)

	if !s.authorize(verified, catalog.ActionAPIGatewayPutIntegration, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:PutIntegration.", readOnly, eventID, verified)
		return
	}
	in, err := s.store.PutRestIntegration(
		verified.AccountID, apiID, resourceID, httpMethod, intType, uri, intHTTP, credentials, templates,
	)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Method identifier specified", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put integration.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.IntegrationJSON(in)
	w.Header().Set("Content-Type", apiGatewayJSONContentType)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "PutIntegration", readOnly)
}

func (s *Server) apigwRESTGetIntegration(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	resourceID := paramString(params, "resourceId", "ResourceId")
	httpMethod := restResourceHTTPMethod(params)
	if !s.authorize(verified, catalog.ActionAPIGatewayGetIntegration, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetIntegration.", readOnly, eventID, verified)
		return
	}
	in, err := s.store.GetRestIntegration(verified.AccountID, apiID, resourceID, httpMethod)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Integration identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get integration.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.IntegrationJSON(in)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetIntegration", readOnly)
}

func (s *Server) apigwRESTCreateDeployment(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	desc := paramString(params, "description", "Description")
	stageName := paramString(params, "stageName", "StageName")
	if !s.authorize(verified, catalog.ActionAPIGatewayCreateDeployment, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:CreateDeployment.", readOnly, eventID, verified)
		return
	}
	dep, err := s.store.CreateRestDeployment(verified.AccountID, apiID, desc, stageName)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create deployment.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.DeploymentJSON(dep)
	w.Header().Set("Content-Type", apiGatewayJSONContentType)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateDeployment", readOnly)
}

func (s *Server) apigwRESTCreateStage(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	stageName := paramString(params, "stageName", "StageName")
	deploymentID := paramString(params, "deploymentId", "DeploymentId")
	if !s.authorize(verified, catalog.ActionAPIGatewayCreateStage, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:CreateStage.", readOnly, eventID, verified)
		return
	}
	st, err := s.store.CreateRestStage(verified.AccountID, apiID, stageName, deploymentID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API or deployment identifier specified", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create stage.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.StageJSON(st)
	w.Header().Set("Content-Type", apiGatewayJSONContentType)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateStage", readOnly)
}

func (s *Server) apigwRESTGetStage(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	stageName := paramString(params, "stageName", "StageName")
	if !s.authorize(verified, catalog.ActionAPIGatewayGetStage, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetStage.", readOnly, eventID, verified)
		return
	}
	st, err := s.store.GetRestStage(verified.AccountID, apiID, stageName)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid stage identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get stage.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.StageJSON(st)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetStage", readOnly)
}
