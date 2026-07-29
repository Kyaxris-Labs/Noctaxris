package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	appsyncsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/appsync"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	appsyncJSONContentType = "application/x-amz-json-1.1"
	appsyncEventSource     = "appsync.amazonaws.com"
)

func isAppSyncGraphQLPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return len(parts) == 3 && parts[0] == "appsync" && parts[2] == "graphql"
}

func appSyncAPIIDFromPath(path string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 3 {
		return ""
	}
	return parts[1]
}

func (s *Server) handleAppSync(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = appsyncAction(action)

	switch action {
	case catalog.ActionAppSyncCreateGraphqlApi:
		s.appsyncCreateAPI(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncDeleteGraphqlApi:
		s.appsyncDeleteAPI(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncGetGraphqlApi:
		s.appsyncGetAPI(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncListGraphqlApis:
		s.appsyncListAPIs(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncStartSchemaCreation:
		s.appsyncStartSchema(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncGetSchemaCreationStatus:
		s.appsyncGetSchemaStatus(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncCreateApiKey:
		s.appsyncCreateAPIKey(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncListApiKeys:
		s.appsyncListAPIKeys(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncDeleteApiKey:
		s.appsyncDeleteAPIKey(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncCreateDataSource:
		s.appsyncCreateDataSource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncUpdateDataSource:
		s.appsyncUpdateDataSource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncDeleteDataSource:
		s.appsyncDeleteDataSource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncGetDataSource:
		s.appsyncGetDataSource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncListDataSources:
		s.appsyncListDataSources(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncCreateResolver:
		s.appsyncCreateResolver(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncUpdateResolver:
		s.appsyncUpdateResolver(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncDeleteResolver:
		s.appsyncDeleteResolver(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncGetResolver:
		s.appsyncGetResolver(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncListResolvers:
		s.appsyncListResolvers(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotImplemented, "BadRequestException",
			"This AppSync action is not implemented.", readOnly, eventID, verified)
	}
}

func appsyncAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateGraphqlApi":
		return catalog.ActionAppSyncCreateGraphqlApi
	case "DeleteGraphqlApi":
		return catalog.ActionAppSyncDeleteGraphqlApi
	case "GetGraphqlApi":
		return catalog.ActionAppSyncGetGraphqlApi
	case "ListGraphqlApis":
		return catalog.ActionAppSyncListGraphqlApis
	case "StartSchemaCreation":
		return catalog.ActionAppSyncStartSchemaCreation
	case "GetSchemaCreationStatus":
		return catalog.ActionAppSyncGetSchemaCreationStatus
	case "CreateApiKey":
		return catalog.ActionAppSyncCreateApiKey
	case "ListApiKeys":
		return catalog.ActionAppSyncListApiKeys
	case "DeleteApiKey":
		return catalog.ActionAppSyncDeleteApiKey
	case "CreateDataSource":
		return catalog.ActionAppSyncCreateDataSource
	case "UpdateDataSource":
		return catalog.ActionAppSyncUpdateDataSource
	case "DeleteDataSource":
		return catalog.ActionAppSyncDeleteDataSource
	case "GetDataSource":
		return catalog.ActionAppSyncGetDataSource
	case "ListDataSources":
		return catalog.ActionAppSyncListDataSources
	case "CreateResolver":
		return catalog.ActionAppSyncCreateResolver
	case "UpdateResolver":
		return catalog.ActionAppSyncUpdateResolver
	case "DeleteResolver":
		return catalog.ActionAppSyncDeleteResolver
	case "GetResolver":
		return catalog.ActionAppSyncGetResolver
	case "ListResolvers":
		return catalog.ActionAppSyncListResolvers
	default:
		return action
	}
}

func (s *Server) appsyncCreateAPI(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["name"].(string)
	if name == "" {
		name, _ = params["Name"].(string)
	}
	authType, _ := params["authenticationType"].(string)
	if authType == "" {
		authType, _ = params["AuthenticationType"].(string)
	}
	pool := store.AppSyncUserPoolConfig{}
	if cfg, ok := params["userPoolConfig"].(map[string]any); ok {
		pool.UserPoolID, _ = cfg["userPoolId"].(string)
		if pool.UserPoolID == "" {
			pool.UserPoolID, _ = cfg["UserPoolId"].(string)
		}
		pool.AwsRegion, _ = cfg["awsRegion"].(string)
		if pool.AwsRegion == "" {
			pool.AwsRegion, _ = cfg["AwsRegion"].(string)
		}
		pool.ClientID, _ = cfg["clientId"].(string)
		if pool.ClientID == "" {
			pool.ClientID, _ = cfg["appClientId"].(string)
		}
		if pool.ClientID == "" {
			pool.ClientID, _ = cfg["ClientId"].(string)
		}
		pool.Issuer, _ = cfg["issuer"].(string)
		if pool.Issuer == "" {
			pool.Issuer, _ = cfg["Issuer"].(string)
		}
	}
	if !s.authorize(verified, catalog.ActionAppSyncCreateGraphqlApi, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:CreateGraphqlApi.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultAppSyncRegion
	}
	api, err := s.store.CreateAppSyncGraphqlAPIWithConfig(verified.AccountID, region, name, authType, pool)
	if errors.Is(err, store.ErrAppSyncBadRequest) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create GraphQL API.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.CreateGraphqlApiJSON(api)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "CreateGraphqlApi", readOnly)
}

func (s *Server) appsyncDeleteAPI(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	if apiID == "" {
		apiID, _ = params["ApiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAppSyncDeleteGraphqlApi, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:DeleteGraphqlApi.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteAppSyncGraphqlAPI(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete GraphQL API.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.DeleteGraphqlApiJSON()
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "DeleteGraphqlApi", readOnly)
}

func (s *Server) appsyncGetAPI(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	if apiID == "" {
		apiID, _ = params["ApiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAppSyncGetGraphqlApi, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:GetGraphqlApi.", readOnly, eventID, verified)
		return
	}
	api, err := s.store.GetAppSyncGraphqlAPI(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get GraphQL API.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.GetGraphqlApiJSON(api)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "GetGraphqlApi", readOnly)
}

func (s *Server) appsyncListAPIs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionAppSyncListGraphqlApis, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:ListGraphqlApis.", readOnly, eventID, verified)
		return
	}
	apis, err := s.store.ListAppSyncGraphqlAPIs(verified.AccountID)
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list GraphQL APIs.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.ListGraphqlApisJSON(apis)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "ListGraphqlApis", readOnly)
}

func (s *Server) appsyncStartSchema(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	def, _ := params["definition"].(string)
	if !s.authorize(verified, catalog.ActionAppSyncStartSchemaCreation, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:StartSchemaCreation.", readOnly, eventID, verified)
		return
	}
	err := s.store.StartAppSyncSchemaCreation(verified.AccountID, apiID, def)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.StartSchemaCreationJSON()
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "StartSchemaCreation", readOnly)
}

func (s *Server) appsyncGetSchemaStatus(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	if apiID == "" {
		apiID, _ = params["ApiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAppSyncGetSchemaCreationStatus, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:GetSchemaCreationStatus.", readOnly, eventID, verified)
		return
	}
	st, err := s.store.GetAppSyncSchemaCreationStatus(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get schema creation status.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.GetSchemaCreationStatusJSON(st)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "GetSchemaCreationStatus", readOnly)
}

func (s *Server) appsyncCreateAPIKey(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	var expires int64
	switch v := params["expires"].(type) {
	case float64:
		expires = int64(v)
	}
	if !s.authorize(verified, catalog.ActionAppSyncCreateApiKey, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:CreateApiKey.", readOnly, eventID, verified)
		return
	}
	key, err := s.store.CreateAppSyncAPIKey(verified.AccountID, apiID, expires)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAppSyncBadRequest) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create API key.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.CreateApiKeyJSON(key)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "CreateApiKey", readOnly)
}

func (s *Server) appsyncListAPIKeys(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	if apiID == "" {
		apiID, _ = params["ApiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAppSyncListApiKeys, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:ListApiKeys.", readOnly, eventID, verified)
		return
	}
	keys, err := s.store.ListAppSyncAPIKeys(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list API keys.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.ListApiKeysJSON(keys)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "ListApiKeys", readOnly)
}

func (s *Server) appsyncDeleteAPIKey(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	if apiID == "" {
		apiID, _ = params["ApiId"].(string)
	}
	id, _ := params["id"].(string)
	if id == "" {
		id, _ = params["Id"].(string)
	}
	if !s.authorize(verified, catalog.ActionAppSyncDeleteApiKey, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:DeleteApiKey.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteAppSyncAPIKey(verified.AccountID, apiID, id)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"API key not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAppSyncBadRequest) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete API key.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.DeleteApiKeyJSON()
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "DeleteApiKey", readOnly)
}

func (s *Server) checkAppSyncPassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	return s.checkEdgePassRole(verified, roleARN, sourceARN, authz.ServicePrincipalAppSync, "AppSync")
}

func appsyncParseDataSourceParams(params map[string]any) (apiID, name, dsType, lambdaARN, serviceRoleArn string) {
	apiID, _ = params["apiId"].(string)
	if apiID == "" {
		apiID, _ = params["ApiId"].(string)
	}
	name, _ = params["name"].(string)
	if name == "" {
		name, _ = params["Name"].(string)
	}
	dsType, _ = params["type"].(string)
	if dsType == "" {
		dsType, _ = params["Type"].(string)
	}
	if cfg, ok := params["lambdaConfig"].(map[string]any); ok {
		lambdaARN, _ = cfg["lambdaFunctionArn"].(string)
		if lambdaARN == "" {
			lambdaARN, _ = cfg["LambdaFunctionArn"].(string)
		}
	}
	if lambdaARN == "" {
		lambdaARN, _ = params["lambdaFunctionArn"].(string)
	}
	serviceRoleArn, _ = params["serviceRoleArn"].(string)
	if serviceRoleArn == "" {
		serviceRoleArn, _ = params["ServiceRoleArn"].(string)
	}
	return apiID, name, dsType, lambdaARN, serviceRoleArn
}

func (s *Server) appsyncCreateDataSource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, name, dsType, lambdaARN, serviceRoleArn := appsyncParseDataSourceParams(params)
	if !s.authorize(verified, catalog.ActionAppSyncCreateDataSource, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:CreateDataSource.", readOnly, eventID, verified)
		return
	}
	api, err := s.store.GetAppSyncGraphqlAPI(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get GraphQL API.", readOnly, eventID, verified)
		return
	}
	if strings.TrimSpace(serviceRoleArn) != "" {
		if err := s.checkAppSyncPassRole(verified, serviceRoleArn, api.ARN); err != nil {
			s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	ds, err := s.store.CreateAppSyncDataSource(verified.AccountID, apiID, name, dsType, lambdaARN, serviceRoleArn)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.CreateDataSourceJSON(ds)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "CreateDataSource", readOnly)
}

func (s *Server) appsyncUpdateDataSource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, name, dsType, lambdaARN, serviceRoleArn := appsyncParseDataSourceParams(params)
	if !s.authorize(verified, catalog.ActionAppSyncUpdateDataSource, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:UpdateDataSource.", readOnly, eventID, verified)
		return
	}
	api, err := s.store.GetAppSyncGraphqlAPI(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get GraphQL API.", readOnly, eventID, verified)
		return
	}
	if strings.TrimSpace(serviceRoleArn) != "" {
		if err := s.checkAppSyncPassRole(verified, serviceRoleArn, api.ARN); err != nil {
			s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	ds, err := s.store.UpdateAppSyncDataSource(verified.AccountID, apiID, name, dsType, lambdaARN, serviceRoleArn)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Data source not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.UpdateDataSourceJSON(ds)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "UpdateDataSource", readOnly)
}

func (s *Server) appsyncGetDataSource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, name, _, _, _ := appsyncParseDataSourceParams(params)
	if !s.authorize(verified, catalog.ActionAppSyncGetDataSource, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:GetDataSource.", readOnly, eventID, verified)
		return
	}
	ds, err := s.store.GetAppSyncDataSource(verified.AccountID, apiID, name)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Data source not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get data source.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.GetDataSourceJSON(ds)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "GetDataSource", readOnly)
}

func (s *Server) appsyncListDataSources(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	if apiID == "" {
		apiID, _ = params["ApiId"].(string)
	}
	if !s.authorize(verified, catalog.ActionAppSyncListDataSources, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:ListDataSources.", readOnly, eventID, verified)
		return
	}
	sources, err := s.store.ListAppSyncDataSources(verified.AccountID, apiID)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list data sources.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.ListDataSourcesJSON(sources)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "ListDataSources", readOnly)
}

func (s *Server) appsyncDeleteDataSource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, name, _, _, _ := appsyncParseDataSourceParams(params)
	if !s.authorize(verified, catalog.ActionAppSyncDeleteDataSource, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:DeleteDataSource.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteAppSyncDataSource(verified.AccountID, apiID, name)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Data source not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAppSyncBadRequest) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete data source.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.DeleteDataSourceJSON()
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "DeleteDataSource", readOnly)
}

func appsyncParseResolverParams(params map[string]any) (apiID, typeName, fieldName, dsName string) {
	apiID, _ = params["apiId"].(string)
	if apiID == "" {
		apiID, _ = params["ApiId"].(string)
	}
	typeName, _ = params["typeName"].(string)
	if typeName == "" {
		typeName, _ = params["TypeName"].(string)
	}
	fieldName, _ = params["fieldName"].(string)
	if fieldName == "" {
		fieldName, _ = params["FieldName"].(string)
	}
	dsName, _ = params["dataSourceName"].(string)
	if dsName == "" {
		dsName, _ = params["DataSourceName"].(string)
	}
	return apiID, typeName, fieldName, dsName
}

func (s *Server) appsyncCreateResolver(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, typeName, fieldName, dsName := appsyncParseResolverParams(params)
	if !s.authorize(verified, catalog.ActionAppSyncCreateResolver, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:CreateResolver.", readOnly, eventID, verified)
		return
	}
	res, err := s.store.CreateAppSyncResolver(verified.AccountID, apiID, typeName, fieldName, dsName)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Data source or API not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.CreateResolverJSON(res)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "CreateResolver", readOnly)
}

func (s *Server) appsyncUpdateResolver(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, typeName, fieldName, dsName := appsyncParseResolverParams(params)
	if !s.authorize(verified, catalog.ActionAppSyncUpdateResolver, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:UpdateResolver.", readOnly, eventID, verified)
		return
	}
	res, err := s.store.UpdateAppSyncResolver(verified.AccountID, apiID, typeName, fieldName, dsName)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Resolver or data source not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.UpdateResolverJSON(res)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "UpdateResolver", readOnly)
}

func (s *Server) appsyncGetResolver(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, typeName, fieldName, _ := appsyncParseResolverParams(params)
	if !s.authorize(verified, catalog.ActionAppSyncGetResolver, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:GetResolver.", readOnly, eventID, verified)
		return
	}
	res, err := s.store.GetAppSyncResolver(verified.AccountID, apiID, typeName, fieldName)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Resolver not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get resolver.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.GetResolverJSON(res)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "GetResolver", readOnly)
}

func (s *Server) appsyncListResolvers(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, typeName, _, _ := appsyncParseResolverParams(params)
	if !s.authorize(verified, catalog.ActionAppSyncListResolvers, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:ListResolvers.", readOnly, eventID, verified)
		return
	}
	resolvers, err := s.store.ListAppSyncResolvers(verified.AccountID, apiID, typeName)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"GraphQL API not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAppSyncBadRequest) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list resolvers.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.ListResolversJSON(resolvers)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "ListResolvers", readOnly)
}

func (s *Server) appsyncDeleteResolver(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, typeName, fieldName, _ := appsyncParseResolverParams(params)
	if !s.authorize(verified, catalog.ActionAppSyncDeleteResolver, "*") {
		s.writeAppSyncError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appsync:DeleteResolver.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteAppSyncResolver(verified.AccountID, apiID, typeName, fieldName)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Resolver not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAppSyncBadRequest) {
		s.writeAppSyncError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppSyncError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete resolver.", readOnly, eventID, verified)
		return
	}
	payload, _ := appsyncsvc.DeleteResolverJSON()
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "DeleteResolver", readOnly)
}

// handleAppSyncGraphQLRuntime serves POST /appsync/{apiId}/graphql with API_KEY, AWS_IAM, or Cognito auth.
func (s *Server) handleAppSyncGraphQLRuntime(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool,
) {
	apiID := appSyncAPIIDFromPath(r.URL.Path)
	accountID, api, err := s.store.GetAppSyncGraphqlAPIByID(apiID)
	if errors.Is(err, store.ErrAppSyncNotFound) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		payload, _ := appsyncsvc.GraphQLErrorsJSON("GraphQL API not found")
		_, _ = w.Write(payload)
		return
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		payload, _ := appsyncsvc.GraphQLErrorsJSON("internal error")
		_, _ = w.Write(payload)
		return
	}

	var verified *authn.Verified
	switch api.AuthenticationType {
	case store.AppSyncAuthAPIKey:
		key := strings.TrimSpace(r.Header.Get("x-api-key"))
		if key == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			payload, _ := appsyncsvc.GraphQLErrorsJSON("Missing x-api-key")
			_, _ = w.Write(payload)
			return
		}
		keyAccount, keyAPI, err := s.store.LookupAppSyncAPIKey(key)
		if err != nil || keyAccount != accountID || keyAPI != apiID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			payload, _ := appsyncsvc.GraphQLErrorsJSON("UnauthorizedException")
			_, _ = w.Write(payload)
			return
		}
		verified = &authn.Verified{AccountID: accountID, Region: store.DefaultAppSyncRegion, Service: "appsync"}
	case store.AppSyncAuthIAM:
		v, err := authn.Verify(r, body, s.now(), sigv4Skew, s.lookupKey)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			payload, _ := appsyncsvc.GraphQLErrorsJSON("UnauthorizedException")
			_, _ = w.Write(payload)
			return
		}
		if !strings.EqualFold(v.Service, "appsync") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			payload, _ := appsyncsvc.GraphQLErrorsJSON("UnauthorizedException")
			_, _ = w.Write(payload)
			return
		}
		if v.AccountID != accountID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			payload, _ := appsyncsvc.GraphQLErrorsJSON("UnauthorizedException")
			_, _ = w.Write(payload)
			return
		}
		if !s.authorize(v, catalog.ActionAppSyncGraphQL, api.ARN) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			payload, _ := appsyncsvc.GraphQLErrorsJSON("UnauthorizedException")
			_, _ = w.Write(payload)
			return
		}
		verified = v
	case store.AppSyncAuthCognito:
		token := bearerTokenFromAuthorization(r.Header.Get("Authorization"))
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			payload, _ := appsyncsvc.GraphQLErrorsJSON("UnauthorizedException")
			_, _ = w.Write(payload)
			return
		}
		issuer := api.UserPoolIssuer
		if issuer == "" {
			issuer = store.AppSyncCognitoIssuer(api.UserPoolRegion, api.UserPoolID)
		}
		if err := s.verifyAPIGatewayJWT(token, issuer, []string{api.UserPoolClientID}, s.now(), "id"); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			payload, _ := appsyncsvc.GraphQLErrorsJSON("UnauthorizedException")
			_, _ = w.Write(payload)
			return
		}
		verified = &authn.Verified{AccountID: accountID, Region: store.DefaultAppSyncRegion, Service: "appsync"}
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		payload, _ := appsyncsvc.GraphQLErrorsJSON("unsupported authenticationType")
		_, _ = w.Write(payload)
		return
	}

	if !s.enforceAssociatedWAF(w, r, accountID, appSyncWAFCandidateARNs(store.DefaultAppSyncRegion, accountID, apiID)) {
		return
	}

	params := jsonBodyMap(body)
	query, _ := params["query"].(string)
	sels, err := store.ParseAppSyncQuerySelections(query)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		payload, _ := appsyncsvc.GraphQLErrorsJSON(err.Error())
		_, _ = w.Write(payload)
		return
	}

	region := store.DefaultAppSyncRegion
	if verified != nil && verified.Region != "" {
		region = verified.Region
	}
	fieldTypes := store.ParseAppSyncSchemaFieldReturnTypes(api.SchemaSDL)
	data := make(map[string]any, len(sels))
	var gqlErrs []map[string]any
	for _, sel := range sels {
		value, pathErrs := s.appsyncResolveSelection(
			r, accountID, apiID, api.ARN, region, "Query", nil, sel, fieldTypes, []any{sel.Name},
		)
		data[sel.Name] = value
		gqlErrs = append(gqlErrs, pathErrs...)
	}
	envelope := map[string]any{"data": data}
	if len(gqlErrs) > 0 {
		envelope["errors"] = gqlErrs
	}
	payload, _ := json.Marshal(envelope)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "GraphQL", readOnly)
}

// appsyncResolveSelection resolves one selection (and nested children) via Lambda resolvers or parent projection.
func (s *Server) appsyncResolveSelection(
	r *http.Request,
	accountID, apiID, apiARN, region, parentType string,
	source any,
	sel store.AppSyncSelection,
	fieldTypes map[string]map[string]string,
	path []any,
) (value any, errs []map[string]any) {
	resolved, errMsg := s.appsyncResolveInvokeField(r, accountID, apiID, apiARN, region, parentType, sel.Name, source)
	if errMsg != "" {
		// No unit resolver: GraphQL default field resolver projects from parent object (null if missing).
		if source != nil {
			projected, _ := appsyncProjectField(source, sel.Name)
			if len(sel.Children) == 0 {
				return projected, nil
			}
			if projected == nil {
				return nil, nil
			}
			return s.appsyncApplyNestedSelections(r, accountID, apiID, apiARN, region, parentType, sel, projected, fieldTypes, path)
		}
		return nil, []map[string]any{{"message": errMsg, "path": append([]any(nil), path...)}}
	}
	if len(sel.Children) == 0 {
		return resolved, nil
	}
	return s.appsyncApplyNestedSelections(r, accountID, apiID, apiARN, region, parentType, sel, resolved, fieldTypes, path)
}

func (s *Server) appsyncApplyNestedSelections(
	r *http.Request,
	accountID, apiID, apiARN, region, parentType string,
	sel store.AppSyncSelection,
	parentValue any,
	fieldTypes map[string]map[string]string,
	path []any,
) (value any, errs []map[string]any) {
	childType := ""
	if ft, ok := fieldTypes[parentType]; ok {
		childType = ft[sel.Name]
	}
	if childType == "" {
		return nil, []map[string]any{{
			"message": "unable to resolve nested type for field " + sel.Name,
			"path":    append([]any(nil), path...),
		}}
	}
	switch v := parentValue.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return s.appsyncResolveChildrenOnObject(r, accountID, apiID, apiARN, region, childType, v, sel.Children, fieldTypes, path)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			itemPath := append(append([]any(nil), path...), i)
			obj, ok := item.(map[string]any)
			if !ok {
				if item == nil {
					out[i] = nil
					continue
				}
				errs = append(errs, map[string]any{
					"message": "Cannot query fields on a non-object list item",
					"path":    itemPath,
				})
				out[i] = nil
				continue
			}
			resolved, itemErrs := s.appsyncResolveChildrenOnObject(
				r, accountID, apiID, apiARN, region, childType, obj, sel.Children, fieldTypes, itemPath,
			)
			out[i] = resolved
			errs = append(errs, itemErrs...)
		}
		return out, errs
	default:
		return nil, []map[string]any{{
			"message": "Cannot query fields on a scalar value",
			"path":    append([]any(nil), path...),
		}}
	}
}

func (s *Server) appsyncResolveChildrenOnObject(
	r *http.Request,
	accountID, apiID, apiARN, region, typeName string,
	source map[string]any,
	children []store.AppSyncSelection,
	fieldTypes map[string]map[string]string,
	path []any,
) (map[string]any, []map[string]any) {
	out := make(map[string]any, len(children))
	var errs []map[string]any
	for _, child := range children {
		childPath := append(append([]any(nil), path...), child.Name)
		val, childErrs := s.appsyncResolveSelection(
			r, accountID, apiID, apiARN, region, typeName, source, child, fieldTypes, childPath,
		)
		out[child.Name] = val
		errs = append(errs, childErrs...)
	}
	return out, errs
}

func appsyncProjectField(source any, field string) (any, bool) {
	m, ok := source.(map[string]any)
	if !ok {
		return nil, false
	}
	v, ok := m[field]
	return v, ok
}

// appsyncResolveInvokeField resolves one type.field and Invokes its Lambda data source.
// When the data source has serviceRoleArn, requires role-session Allow on lambda:InvokeFunction;
// otherwise requires a Lambda resource policy Allow for appsync.amazonaws.com.
// source is the parent object (nil for Query root); it is passed as the Lambda event "source".
func (s *Server) appsyncResolveInvokeField(
	r *http.Request, accountID, apiID, apiARN, region, typeName, field string, source any,
) (value any, errMsg string) {
	_, ds, err := s.store.ResolveAppSyncField(accountID, apiID, typeName, field)
	if err != nil {
		return nil, "resolver not found for field " + field
	}
	fnAccount, fnName, ok := store.ParseLambdaARNFromSFNResource(ds.LambdaFunctionARN)
	if !ok {
		return nil, "invalid lambda data source ARN"
	}
	if fnAccount == "" {
		fnAccount = accountID
	}
	fn, executedVersion, err := s.store.ResolveFunction(fnAccount, fnName, "$LATEST")
	if err != nil {
		return nil, "lambda function not found"
	}
	if roleARN := strings.TrimSpace(ds.ServiceRoleArn); roleARN != "" {
		if !s.store.RoleSessionAllows(
			accountID, roleARN, catalog.ActionLambdaInvoke, fn.FunctionARN, "appsync-datasource", region,
		) {
			return nil, "UnauthorizedException"
		}
	} else if !s.store.DeliveryTargetResourcePolicyAllows(
		fnAccount, fn.FunctionARN, catalog.ActionLambdaInvoke, authz.ServicePrincipalAppSync, apiARN,
	) {
		return nil, "UnauthorizedException"
	}
	event := map[string]any{
		"field":     field,
		"arguments": map[string]any{},
		"info":      map[string]any{"fieldName": field, "parentTypeName": typeName},
	}
	if source != nil {
		event["source"] = source
	}
	eventJSON, _ := json.Marshal(event)
	result, err := s.executeLambdaInvoke(r.Context(), fnAccount, fnName, fn, executedVersion, string(eventJSON))
	if err != nil {
		return nil, err.Error()
	}
	if err := json.Unmarshal(result, &value); err != nil {
		value = string(result)
	}
	return value, ""
}

func (s *Server) writeAppSyncOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", appsyncJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeAppSyncError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", appsyncJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, code, readOnly)
}
