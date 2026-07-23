package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	apigwv2 "github.com/Kyaxris-Labs/Noctaxris/internal/services/apigatewayv2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	apiGatewayJSONContentType = "application/x-amz-json-1.1"
	apiGatewayEventSource     = "apigateway.amazonaws.com"
)

func (s *Server) handleAPIGatewayV2(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = apiGatewayV2Action(action)

	switch action {
	case catalog.ActionAPIGatewayV2CreateApi:
		s.apigwCreateApi(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2GetApi:
		s.apigwGetApi(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2UpdateApi:
		s.apigwUpdateApi(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2DeleteApi:
		s.apigwDeleteApi(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2GetApis:
		s.apigwGetApis(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2CreateIntegration:
		s.apigwCreateIntegration(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2GetIntegrations:
		s.apigwGetIntegrations(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2CreateAuthorizer:
		s.apigwCreateAuthorizer(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2GetAuthorizers:
		s.apigwGetAuthorizers(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2CreateRoute:
		s.apigwCreateRoute(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2GetRoutes:
		s.apigwGetRoutes(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAPIGatewayV2CreateStage:
		s.apigwCreateStage(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotImplemented, "BadRequestException",
			"This API Gateway action is not implemented.", readOnly, eventID, verified)
	}
}

func apiGatewayV2Action(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateApi":
		return catalog.ActionAPIGatewayV2CreateApi
	case "GetApi":
		return catalog.ActionAPIGatewayV2GetApi
	case "UpdateApi":
		return catalog.ActionAPIGatewayV2UpdateApi
	case "DeleteApi":
		return catalog.ActionAPIGatewayV2DeleteApi
	case "GetApis":
		return catalog.ActionAPIGatewayV2GetApis
	case "CreateIntegration":
		return catalog.ActionAPIGatewayV2CreateIntegration
	case "GetIntegrations":
		return catalog.ActionAPIGatewayV2GetIntegrations
	case "CreateAuthorizer":
		return catalog.ActionAPIGatewayV2CreateAuthorizer
	case "GetAuthorizers":
		return catalog.ActionAPIGatewayV2GetAuthorizers
	case "CreateRoute":
		return catalog.ActionAPIGatewayV2CreateRoute
	case "GetRoutes":
		return catalog.ActionAPIGatewayV2GetRoutes
	case "CreateStage":
		return catalog.ActionAPIGatewayV2CreateStage
	default:
		return action
	}
}

// resolveAPIGatewayV2REST maps /v2/apis/... control-plane paths used by aws apigatewayv2.
func resolveAPIGatewayV2REST(r *http.Request, body []byte) (action string, outBody []byte) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// parts: ["v2", "apis"] or ["v2", "apis", apiId, ...]
	if len(parts) < 2 || parts[0] != "v2" || parts[1] != "apis" {
		return "", body
	}
	switch r.Method {
	case http.MethodPost:
		if len(parts) == 2 {
			return catalog.ActionAPIGatewayV2CreateApi, body
		}
		if len(parts) == 4 {
			apiID := parts[2]
			switch parts[3] {
			case "integrations":
				return catalog.ActionAPIGatewayV2CreateIntegration, injectJSONStringField(body, "ApiId", apiID)
			case "routes":
				return catalog.ActionAPIGatewayV2CreateRoute, injectJSONStringField(body, "ApiId", apiID)
			case "authorizers":
				return catalog.ActionAPIGatewayV2CreateAuthorizer, injectJSONStringField(body, "ApiId", apiID)
			case "stages":
				return catalog.ActionAPIGatewayV2CreateStage, injectJSONStringField(body, "ApiId", apiID)
			}
		}
	case http.MethodGet:
		if len(parts) == 2 {
			return catalog.ActionAPIGatewayV2GetApis, body
		}
		if len(parts) == 3 {
			return catalog.ActionAPIGatewayV2GetApi, injectJSONStringField(body, "ApiId", parts[2])
		}
		if len(parts) == 4 {
			apiID := parts[2]
			switch parts[3] {
			case "integrations":
				return catalog.ActionAPIGatewayV2GetIntegrations, injectJSONStringField(body, "ApiId", apiID)
			case "routes":
				return catalog.ActionAPIGatewayV2GetRoutes, injectJSONStringField(body, "ApiId", apiID)
			case "authorizers":
				return catalog.ActionAPIGatewayV2GetAuthorizers, injectJSONStringField(body, "ApiId", apiID)
			}
		}
	case http.MethodDelete:
		if len(parts) == 3 {
			return catalog.ActionAPIGatewayV2DeleteApi, injectJSONStringField(body, "ApiId", parts[2])
		}
	case http.MethodPatch:
		if len(parts) == 3 {
			return catalog.ActionAPIGatewayV2UpdateApi, injectJSONStringField(body, "ApiId", parts[2])
		}
	}
	return "", body
}

func (s *Server) apigwCreateApi(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	if name == "" {
		name, _ = params["name"].(string)
	}
	proto, _ := params["ProtocolType"].(string)
	if proto == "" {
		proto, _ = params["protocolType"].(string)
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2CreateApi, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:CreateApi.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultAPIGatewayRegion
	}
	cors, _, corsErr := store.ParseAPIGatewayCORSFromParams(params)
	if corsErr != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			corsErr.Error(), readOnly, eventID, verified)
		return
	}
	api, err := s.store.CreateAPIGatewayAPIWithCORS(verified.AccountID, region, name, proto, cors)
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create API.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.CreateApiJSON(api)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateApi", readOnly)
}

func (s *Server) apigwUpdateApi(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	if apiID == "" {
		apiID, _ = params["apiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2UpdateApi, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:UpdateApi.", readOnly, eventID, verified)
		return
	}
	cors, hasCORS, corsErr := store.ParseAPIGatewayCORSFromParams(params)
	if corsErr != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			corsErr.Error(), readOnly, eventID, verified)
		return
	}
	if !hasCORS {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"CorsConfiguration is required for lab UpdateApi", readOnly, eventID, verified)
		return
	}
	api, err := s.store.UpdateAPIGatewayAPICORS(verified.AccountID, apiID, cors)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API not found", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update API.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.CreateApiJSON(api)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "UpdateApi", readOnly)
}

func (s *Server) apigwGetApi(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	if apiID == "" {
		apiID, _ = params["apiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2GetApi, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:GetApi.", readOnly, eventID, verified)
		return
	}
	api, err := s.store.GetAPIGatewayAPI(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get API.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.GetApiJSON(api)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetApi", readOnly)
}

func (s *Server) apigwDeleteApi(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	if apiID == "" {
		apiID, _ = params["apiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2DeleteApi, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:DeleteApi.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteAPIGatewayAPI(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete API.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.DeleteApiJSON()
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "DeleteApi", readOnly)
}

func (s *Server) apigwGetApis(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionAPIGatewayV2GetApis, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:GetApis.", readOnly, eventID, verified)
		return
	}
	apis, err := s.store.ListAPIGatewayAPIs(verified.AccountID)
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list APIs.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.GetApisJSON(apis)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetApis", readOnly)
}

func (s *Server) apigwGetIntegrations(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	if apiID == "" {
		apiID, _ = params["apiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2GetIntegrations, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:GetIntegrations.", readOnly, eventID, verified)
		return
	}
	items, err := s.store.ListAPIGatewayIntegrations(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list integrations.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.GetIntegrationsJSON(items)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetIntegrations", readOnly)
}

func (s *Server) apigwGetRoutes(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	if apiID == "" {
		apiID, _ = params["apiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2GetRoutes, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:GetRoutes.", readOnly, eventID, verified)
		return
	}
	items, err := s.store.ListAPIGatewayRoutes(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list routes.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.GetRoutesJSON(items)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetRoutes", readOnly)
}

func (s *Server) apigwGetAuthorizers(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	if apiID == "" {
		apiID, _ = params["apiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2GetAuthorizers, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:GetAuthorizers.", readOnly, eventID, verified)
		return
	}
	items, err := s.store.ListAPIGatewayAuthorizers(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list authorizers.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.GetAuthorizersJSON(items)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetAuthorizers", readOnly)
}

func (s *Server) apiGatewayAPIARN(verified *authn.Verified, apiID string) string {
	region := ""
	if verified != nil {
		region = verified.Region
	}
	if region == "" {
		region = store.DefaultAPIGatewayRegion
	}
	return "arn:aws:apigateway:" + region + "::/apis/" + apiID
}

func (s *Server) apigwCreateIntegration(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	intType, _ := params["IntegrationType"].(string)
	uri, _ := params["IntegrationUri"].(string)
	payloadFmt, _ := params["PayloadFormatVersion"].(string)
	credentialsArn, _ := params["CredentialsArn"].(string)
	if credentialsArn == "" {
		credentialsArn, _ = params["credentialsArn"].(string)
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2CreateIntegration, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:CreateIntegration.", readOnly, eventID, verified)
		return
	}
	if credentialsArn != "" {
		if err := s.checkAPIGatewayPassRole(verified, credentialsArn, s.apiGatewayAPIARN(verified, apiID)); err != nil {
			s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	in, err := s.store.CreateAPIGatewayIntegration(verified.AccountID, apiID, intType, uri, payloadFmt, credentialsArn)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create integration.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.CreateIntegrationJSON(in)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateIntegration", readOnly)
}

func (s *Server) apigwCreateAuthorizer(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	name, _ := params["Name"].(string)
	authType, _ := params["AuthorizerType"].(string)
	identitySource := ""
	if srcs, ok := params["IdentitySource"].([]any); ok && len(srcs) > 0 {
		identitySource, _ = srcs[0].(string)
	}
	if identitySource == "" {
		identitySource, _ = params["IdentitySource"].(string)
	}
	issuer := ""
	var audience []string
	if cfg, ok := params["JwtConfiguration"].(map[string]any); ok {
		issuer, _ = cfg["Issuer"].(string)
		switch aud := cfg["Audience"].(type) {
		case []any:
			for _, a := range aud {
				if s, ok := a.(string); ok && s != "" {
					audience = append(audience, s)
				}
			}
		case []string:
			audience = aud
		}
	}
	authorizerURI, _ := params["AuthorizerUri"].(string)
	if authorizerURI == "" {
		authorizerURI, _ = params["authorizerUri"].(string)
	}
	credARN, _ := params["AuthorizerCredentialsArn"].(string)
	if credARN == "" {
		credARN, _ = params["authorizerCredentialsArn"].(string)
	}
	payloadVer, _ := params["AuthorizerPayloadFormatVersion"].(string)
	if payloadVer == "" {
		payloadVer, _ = params["authorizerPayloadFormatVersion"].(string)
	}
	var enableSimple *bool
	if v, ok := params["EnableSimpleResponses"].(bool); ok {
		enableSimple = &v
	} else if v, ok := params["enableSimpleResponses"].(bool); ok {
		enableSimple = &v
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2CreateAuthorizer, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:CreateAuthorizer.", readOnly, eventID, verified)
		return
	}
	if credARN != "" {
		if err := s.checkAPIGatewayPassRole(verified, credARN, s.apiGatewayAPIARN(verified, apiID)); err != nil {
			s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	a, err := s.store.CreateAPIGatewayAuthorizer(store.CreateAPIGatewayAuthorizerInput{
		AccountID:                      verified.AccountID,
		APIID:                          apiID,
		Name:                           name,
		AuthorizerType:                 authType,
		IdentitySource:                 identitySource,
		JWTIssuer:                      issuer,
		JWTAudience:                    audience,
		AuthorizerURI:                  authorizerURI,
		AuthorizerCredentialsArn:       credARN,
		AuthorizerPayloadFormatVersion: payloadVer,
		EnableSimpleResponses:          enableSimple,
	})
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create authorizer.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.CreateAuthorizerJSON(a)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateAuthorizer", readOnly)
}

func (s *Server) apigwCreateRoute(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	routeKey, _ := params["RouteKey"].(string)
	target, _ := params["Target"].(string)
	authType, _ := params["AuthorizationType"].(string)
	authorizerID, _ := params["AuthorizerId"].(string)
	if !s.authorize(verified, catalog.ActionAPIGatewayV2CreateRoute, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:CreateRoute.", readOnly, eventID, verified)
		return
	}
	effectiveAuth := strings.ToUpper(strings.TrimSpace(authType))
	if effectiveAuth == "" {
		effectiveAuth = store.APIGatewayAuthNone
	}
	if effectiveAuth == store.APIGatewayAuthNone && !s.cfg.OpenDataPlaneAllowed() {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"AuthorizationType NONE requires NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1 when listen is non-loopback.",
			readOnly, eventID, verified)
		return
	}
	route, err := s.store.CreateAPIGatewayRoute(verified.AccountID, apiID, routeKey, target, authType, authorizerID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API, integration, or authorizer not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) || errors.Is(err, store.ErrAPIGatewayConflict) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create route.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.CreateRouteJSON(route)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateRoute", readOnly)
}

func (s *Server) apigwCreateStage(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["ApiId"].(string)
	stageName, _ := params["StageName"].(string)
	autoDeploy := true
	if v, ok := params["AutoDeploy"].(bool); ok {
		autoDeploy = v
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayV2CreateStage, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigatewayv2:CreateStage.", readOnly, eventID, verified)
		return
	}
	st, err := s.store.CreateAPIGatewayStage(verified.AccountID, apiID, stageName, autoDeploy)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) || errors.Is(err, store.ErrAPIGatewayConflict) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create stage.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigwv2.CreateStageJSON(st)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateStage", readOnly)
}

func (s *Server) writeAPIGatewayOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", apiGatewayJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeAPIGatewayError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", apiGatewayJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, code, readOnly)
}
