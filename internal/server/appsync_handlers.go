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
	case catalog.ActionAppSyncCreateApiKey:
		s.appsyncCreateAPIKey(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncCreateDataSource:
		s.appsyncCreateDataSource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppSyncCreateResolver:
		s.appsyncCreateResolver(w, r, body, requestID, eventID, verified, readOnly, params)
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
	case "CreateApiKey":
		return catalog.ActionAppSyncCreateApiKey
	case "CreateDataSource":
		return catalog.ActionAppSyncCreateDataSource
	case "CreateResolver":
		return catalog.ActionAppSyncCreateResolver
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

func (s *Server) checkAppSyncPassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	return s.checkEdgePassRole(verified, roleARN, sourceARN, authz.ServicePrincipalAppSync, "AppSync")
}

func (s *Server) appsyncCreateDataSource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	name, _ := params["name"].(string)
	dsType, _ := params["type"].(string)
	lambdaARN := ""
	if cfg, ok := params["lambdaConfig"].(map[string]any); ok {
		lambdaARN, _ = cfg["lambdaFunctionArn"].(string)
	}
	if lambdaARN == "" {
		lambdaARN, _ = params["lambdaFunctionArn"].(string)
	}
	serviceRoleArn, _ := params["serviceRoleArn"].(string)
	if serviceRoleArn == "" {
		serviceRoleArn, _ = params["ServiceRoleArn"].(string)
	}
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
	payload, _ := appsyncCreateDataSourceJSON(ds)
	s.writeAppSyncOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appsyncEventSource, "CreateDataSource", readOnly)
}

func appsyncCreateDataSourceJSON(ds store.AppSyncDataSource) ([]byte, error) {
	src := map[string]any{
		"name":          ds.Name,
		"type":          ds.Type,
		"lambdaConfig":  map[string]any{"lambdaFunctionArn": ds.LambdaFunctionARN},
		"dataSourceArn": "arn:aws:appsync:us-east-1:000000000001:apis/" + ds.APIID + "/datasources/" + ds.Name,
	}
	if ds.ServiceRoleArn != "" {
		src["serviceRoleArn"] = ds.ServiceRoleArn
	}
	return json.Marshal(map[string]any{"dataSource": src})
}

func (s *Server) appsyncCreateResolver(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	apiID, _ := params["apiId"].(string)
	typeName, _ := params["typeName"].(string)
	fieldName, _ := params["fieldName"].(string)
	dsName, _ := params["dataSourceName"].(string)
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
	fields, err := store.ParseAppSyncQueryFields(query)
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
	data := make(map[string]any, len(fields))
	var gqlErrs []map[string]any
	for _, field := range fields {
		value, fieldErr := s.appsyncResolveInvokeField(r, accountID, apiID, api.ARN, region, field)
		if fieldErr != "" {
			data[field] = nil
			gqlErrs = append(gqlErrs, map[string]any{
				"message": fieldErr,
				"path":    []any{field},
			})
			continue
		}
		data[field] = value
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

// appsyncResolveInvokeField resolves one Query field and Invokes its Lambda data source.
// When the data source has serviceRoleArn, requires role-session Allow on lambda:InvokeFunction;
// otherwise requires a Lambda resource policy Allow for appsync.amazonaws.com.
func (s *Server) appsyncResolveInvokeField(
	r *http.Request, accountID, apiID, apiARN, region, field string,
) (value any, errMsg string) {
	_, ds, err := s.store.ResolveAppSyncQueryField(accountID, apiID, field)
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
	eventJSON, _ := json.Marshal(map[string]any{
		"field":     field,
		"arguments": map[string]any{},
		"info":      map[string]any{"fieldName": field, "parentTypeName": "Query"},
	})
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
