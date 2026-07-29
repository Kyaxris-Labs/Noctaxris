package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	apigw "github.com/Kyaxris-Labs/Noctaxris/internal/services/apigateway"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func (s *Server) apigwRESTCreateAuthorizer(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	name := paramString(params, "name", "Name")
	authType := paramString(params, "type", "Type", "authorizerType", "AuthorizerType")
	uri := paramString(params, "authorizerUri", "AuthorizerUri")
	identity := paramString(params, "identitySource", "IdentitySource")
	cred := paramString(params, "authorizerCredentials", "AuthorizerCredentials", "authorizerCredentialsArn", "AuthorizerCredentialsArn")
	if !s.authorize(verified, catalog.ActionAPIGatewayCreateAuthorizer, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:CreateAuthorizer.", readOnly, eventID, verified)
		return
	}
	if cred != "" {
		if err := s.checkAPIGatewayPassRole(verified, cred, store.RestAPIControlPlaneARN(verified.Region, apiID)); err != nil {
			s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	a, err := s.store.CreateRestAuthorizer(store.CreateRestAuthorizerInput{
		AccountID: verified.AccountID, APIID: apiID, Name: name, Type: authType,
		AuthorizerURI: uri, IdentitySource: identity, AuthorizerCredentials: cred,
	})
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API identifier specified", readOnly, eventID, verified)
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
	payload, _ := apigw.AuthorizerJSON(a)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateAuthorizer", readOnly)
}

func (s *Server) apigwRESTGetAuthorizer(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	authorizerID := paramString(params, "authorizerId", "AuthorizerId")
	if !s.authorize(verified, catalog.ActionAPIGatewayGetAuthorizer, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetAuthorizer.", readOnly, eventID, verified)
		return
	}
	a, err := s.store.GetRestAuthorizer(verified.AccountID, apiID, authorizerID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Authorizer identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get authorizer.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.AuthorizerJSON(a)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetAuthorizer", readOnly)
}

func (s *Server) apigwRESTGetAuthorizers(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	if !s.authorize(verified, catalog.ActionAPIGatewayGetAuthorizers, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetAuthorizers.", readOnly, eventID, verified)
		return
	}
	items, err := s.store.ListRestAuthorizers(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list authorizers.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.AuthorizersJSON(items)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetAuthorizers", readOnly)
}

func (s *Server) apigwRESTDeleteAuthorizer(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID := paramString(params, "restApiId", "RestApiId")
	authorizerID := paramString(params, "authorizerId", "AuthorizerId")
	if !s.authorize(verified, catalog.ActionAPIGatewayDeleteAuthorizer, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:DeleteAuthorizer.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteRestAuthorizer(verified.AccountID, apiID, authorizerID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Authorizer identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete authorizer.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "DeleteAuthorizer", readOnly)
}

func (s *Server) apigwRESTCreateApiKey(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := paramString(params, "name", "Name")
	enabled := true
	if v, ok := params["enabled"].(bool); ok {
		enabled = v
	} else if v, ok := params["Enabled"].(bool); ok {
		enabled = v
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayCreateApiKey, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:CreateApiKey.", readOnly, eventID, verified)
		return
	}
	key, err := s.store.CreateRestAPIKey(verified.AccountID, name, enabled)
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create API key.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.ApiKeyJSON(key)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateApiKey", readOnly)
}

func (s *Server) apigwRESTGetApiKey(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiKeyID := paramString(params, "apiKey", "ApiKey", "apiKeyId", "ApiKeyId")
	if !s.authorize(verified, catalog.ActionAPIGatewayGetApiKey, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetApiKey.", readOnly, eventID, verified)
		return
	}
	key, err := s.store.GetRestAPIKey(verified.AccountID, apiKeyID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API Key identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get API key.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.ApiKeyJSON(key)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetApiKey", readOnly)
}

func (s *Server) apigwRESTGetApiKeys(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionAPIGatewayGetApiKeys, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetApiKeys.", readOnly, eventID, verified)
		return
	}
	keys, err := s.store.ListRestAPIKeys(verified.AccountID)
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list API keys.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.ApiKeysJSON(keys)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetApiKeys", readOnly)
}

func (s *Server) apigwRESTDeleteApiKey(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiKeyID := paramString(params, "apiKey", "ApiKey", "apiKeyId", "ApiKeyId")
	if !s.authorize(verified, catalog.ActionAPIGatewayDeleteApiKey, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:DeleteApiKey.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteRestAPIKey(verified.AccountID, apiKeyID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API Key identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete API key.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "DeleteApiKey", readOnly)
}

func (s *Server) apigwRESTCreateUsagePlan(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := paramString(params, "name", "Name")
	desc := paramString(params, "description", "Description")
	rawStages := params["apiStages"]
	if rawStages == nil {
		rawStages = params["ApiStages"]
	}
	stages, err := store.ParseRestAPIStagesJSON(rawStages)
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayCreateUsagePlan, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:CreateUsagePlan.", readOnly, eventID, verified)
		return
	}
	plan, err := s.store.CreateRestUsagePlan(verified.AccountID, name, desc, stages)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid API or stage identifier specified", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create usage plan.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.UsagePlanJSON(plan)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateUsagePlan", readOnly)
}

func (s *Server) apigwRESTGetUsagePlan(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	planID := paramString(params, "usagePlanId", "UsagePlanId")
	if !s.authorize(verified, catalog.ActionAPIGatewayGetUsagePlan, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetUsagePlan.", readOnly, eventID, verified)
		return
	}
	plan, err := s.store.GetRestUsagePlan(verified.AccountID, planID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Usage Plan ID specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get usage plan.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.UsagePlanJSON(plan)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetUsagePlan", readOnly)
}

func (s *Server) apigwRESTGetUsagePlans(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionAPIGatewayGetUsagePlans, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetUsagePlans.", readOnly, eventID, verified)
		return
	}
	plans, err := s.store.ListRestUsagePlans(verified.AccountID)
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list usage plans.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.UsagePlansJSON(plans)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetUsagePlans", readOnly)
}

func (s *Server) apigwRESTDeleteUsagePlan(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	planID := paramString(params, "usagePlanId", "UsagePlanId")
	if !s.authorize(verified, catalog.ActionAPIGatewayDeleteUsagePlan, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:DeleteUsagePlan.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteRestUsagePlan(verified.AccountID, planID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Usage Plan ID specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete usage plan.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "DeleteUsagePlan", readOnly)
}

func (s *Server) apigwRESTCreateUsagePlanKey(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	planID := paramString(params, "usagePlanId", "UsagePlanId")
	keyID := paramString(params, "keyId", "KeyId")
	keyType := strings.ToUpper(paramString(params, "keyType", "KeyType"))
	if keyType == "" {
		keyType = "API_KEY"
	}
	if !s.authorize(verified, catalog.ActionAPIGatewayCreateUsagePlanKey, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:CreateUsagePlanKey.", readOnly, eventID, verified)
		return
	}
	if keyType != "API_KEY" {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"keyType must be API_KEY", readOnly, eventID, verified)
		return
	}
	k, err := s.store.CreateRestUsagePlanKey(verified.AccountID, planID, keyID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Usage Plan or API Key identifier specified", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAPIGatewayBadRequest) || errors.Is(err, store.ErrAPIGatewayConflict) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create usage plan key.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.UsagePlanKeyJSON(k)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "CreateUsagePlanKey", readOnly)
}

func (s *Server) apigwRESTGetUsagePlanKeys(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	planID := paramString(params, "usagePlanId", "UsagePlanId")
	if !s.authorize(verified, catalog.ActionAPIGatewayGetUsagePlanKeys, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:GetUsagePlanKeys.", readOnly, eventID, verified)
		return
	}
	keys, err := s.store.ListRestUsagePlanKeys(verified.AccountID, planID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Usage Plan ID specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list usage plan keys.", readOnly, eventID, verified)
		return
	}
	payload, _ := apigw.UsagePlanKeysJSON(keys)
	s.writeAPIGatewayOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetUsagePlanKeys", readOnly)
}

func (s *Server) apigwRESTDeleteUsagePlanKey(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	planID := paramString(params, "usagePlanId", "UsagePlanId")
	keyID := paramString(params, "keyId", "KeyId", "apiKeyId", "ApiKeyId")
	if !s.authorize(verified, catalog.ActionAPIGatewayDeleteUsagePlanKey, "*") {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform apigateway:DeleteUsagePlanKey.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteRestUsagePlanKey(verified.AccountID, planID, keyID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Invalid Usage Plan Key identifier specified", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAPIGatewayError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete usage plan key.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "DeleteUsagePlanKey", readOnly)
}

// invokeRestAPILambdaAuthorizer calls a TOKEN or REQUEST authorizer Lambda (fail closed).
func (s *Server) invokeRestAPILambdaAuthorizer(
	ctx context.Context,
	r *http.Request,
	accountID, apiID, stage, routePath, resourcePath, resourceID, requestID string,
	authzRow store.RestAuthorizer,
) (allowed bool, errMsg string) {
	region := store.DefaultAPIGatewayRegion
	fnAccount, fnName, ok := store.ParseLambdaARNFromSFNResource(authzRow.AuthorizerURI)
	if !ok || fnName == "" {
		return false, "invalid authorizer"
	}
	if fnAccount == "" {
		fnAccount = accountID
	}
	fn, executedVersion, err := s.store.ResolveFunction(fnAccount, fnName, "$LATEST")
	if err != nil {
		return false, "authorizer lambda not found"
	}
	methodARN := store.RestAPIMethodARN(region, accountID, apiID, stage, r.Method, routePath)
	if credARN := strings.TrimSpace(authzRow.AuthorizerCredentials); credARN != "" {
		if !s.store.RoleSessionAllows(
			accountID, credARN, catalog.ActionLambdaInvoke, fn.FunctionARN,
			"apigateway-authorizer-credentials", region,
		) {
			return false, "Forbidden"
		}
	} else if !s.store.DeliveryTargetResourcePolicyAllows(
		fnAccount, fn.FunctionARN, catalog.ActionLambdaInvoke, authz.ServicePrincipalAPIGateway, methodARN,
	) {
		return false, "Forbidden"
	}

	eventJSON, err := buildRestAPIAuthorizerEvent(r, accountID, apiID, stage, routePath, resourcePath, resourceID, requestID, authzRow, methodARN)
	if err != nil {
		return false, "authorizer event"
	}
	result, err := s.executeLambdaInvoke(ctx, fnAccount, fnName, fn, executedVersion, string(eventJSON))
	if err != nil {
		if strings.Contains(err.Error(), "compute unavailable") {
			return false, "compute unavailable"
		}
		return false, "authorizer invoke failed"
	}
	if parseHTTPAPIAuthorizerResponse(result, false) {
		return true, ""
	}
	return false, "Unauthorized"
}

func buildRestAPIAuthorizerEvent(
	r *http.Request,
	accountID, apiID, stage, routePath, resourcePath, resourceID, requestID string,
	authzRow store.RestAuthorizer,
	methodARN string,
) ([]byte, error) {
	token := resolveRestAuthorizerIdentityToken(r, authzRow.IdentitySource)
	if authzRow.Type == store.APIGatewayAuthorizerTOKEN {
		return json.Marshal(map[string]any{
			"type":               "TOKEN",
			"authorizationToken": token,
			"methodArn":          methodARN,
		})
	}
	return json.Marshal(map[string]any{
		"type":                  "REQUEST",
		"methodArn":             methodARN,
		"resource":              resourcePath,
		"path":                  routePath,
		"httpMethod":            r.Method,
		"headers":               flattenHeaders(r.Header),
		"queryStringParameters": flattenQuery(r),
		"pathParameters":        restPathParameters(resourcePath, routePath),
		"stageVariables":        nil,
		"requestContext": map[string]any{
			"accountId":    accountID,
			"apiId":        apiID,
			"stage":        stage,
			"requestId":    requestID,
			"resourceId":   resourceID,
			"resourcePath": resourcePath,
			"httpMethod":   r.Method,
			"path":         "/" + stage + routePath,
			"identity":     map[string]any{},
		},
	})
}

func resolveRestAuthorizerIdentityToken(r *http.Request, identitySource string) string {
	src := strings.TrimSpace(identitySource)
	switch {
	case strings.HasPrefix(src, "method.request.header."):
		return r.Header.Get(strings.TrimPrefix(src, "method.request.header."))
	case strings.HasPrefix(src, "$request.header."):
		return r.Header.Get(strings.TrimPrefix(src, "$request.header."))
	case strings.HasPrefix(src, "method.request.querystring."):
		return r.URL.Query().Get(strings.TrimPrefix(src, "method.request.querystring."))
	case strings.HasPrefix(src, "$request.querystring."):
		return r.URL.Query().Get(strings.TrimPrefix(src, "$request.querystring."))
	case src != "" && !strings.Contains(src, "."):
		return r.Header.Get(src)
	default:
		return r.Header.Get("Authorization")
	}
}
