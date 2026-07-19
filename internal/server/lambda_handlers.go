package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	lambdasvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/lambda"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	lambdaJSONContentType = "application/x-amz-json-1.1"
	lambdaEventSource     = "lambda.amazonaws.com"
	defaultLambdaTimeout  = 3
	defaultLambdaMemory   = 128
	defaultLambdaEndpoint = "http://host.docker.internal:4566"
	lambdaInvokeSession   = "noctaxris-lambda"
)

func (s *Server) handleLambda(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = lambdaAction(action)

	switch action {
	case catalog.ActionLambdaCreateFunction:
		s.lambdaCreateFunction(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaGetFunction:
		s.lambdaGetFunction(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaDeleteFunction:
		s.lambdaDeleteFunction(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaListFunctions:
		s.lambdaListFunctions(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLambdaUpdateFunctionCode:
		s.lambdaUpdateFunctionCode(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaUpdateFunctionConfiguration:
		s.lambdaUpdateFunctionConfiguration(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaInvoke:
		s.lambdaInvoke(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeLambdaError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Lambda action is not implemented.", readOnly, eventID, verified)
	}
}

func lambdaAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateFunction":
		return catalog.ActionLambdaCreateFunction
	case "GetFunction":
		return catalog.ActionLambdaGetFunction
	case "DeleteFunction":
		return catalog.ActionLambdaDeleteFunction
	case "ListFunctions":
		return catalog.ActionLambdaListFunctions
	case "UpdateFunctionCode":
		return catalog.ActionLambdaUpdateFunctionCode
	case "UpdateFunctionConfiguration":
		return catalog.ActionLambdaUpdateFunctionConfiguration
	case "Invoke":
		return catalog.ActionLambdaInvoke
	default:
		return action
	}
}

func (s *Server) authorizeLambda(verified *authn.Verified, action, resource string) bool {
	return s.authorize(verified, action, resource)
}

func functionNameParam(params map[string]any) string {
	name, _ := params["FunctionName"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if i := strings.LastIndex(name, ":function:"); i >= 0 {
		return name[i+len(":function:"):]
	}
	return name
}

func lambdaEnvFromParams(params map[string]any) map[string]string {
	out := map[string]string{}
	envObj, _ := params["Environment"].(map[string]any)
	if envObj == nil {
		return out
	}
	vars, _ := envObj["Variables"].(map[string]any)
	for k, v := range vars {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func (s *Server) lambdaZipFromCode(params map[string]any, accountID string) ([]byte, error) {
	code, _ := params["Code"].(map[string]any)
	if code == nil {
		// UpdateFunctionCode puts ZipFile at the top level.
		if z, ok := params["ZipFile"]; ok {
			return lambdasvc.DecodeZipFile(z)
		}
		return nil, errors.New("Code.ZipFile is required")
	}
	if z, ok := code["ZipFile"]; ok {
		return lambdasvc.DecodeZipFile(z)
	}
	bucket, _ := code["S3Bucket"].(string)
	key, _ := code["S3Key"].(string)
	if bucket != "" && key != "" {
		_, data, err := s.store.GetObject(accountID, bucket, key)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return nil, errors.New("empty S3 object")
		}
		return data, nil
	}
	return nil, errors.New("Code.ZipFile or Code.S3Bucket/S3Key is required")
}

func (s *Server) checkLambdaPassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("Role must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("Role must be in the same account")
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
		return errors.New("not authorized to pass role to Lambda")
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
		ServicePrincipal: authz.ServicePrincipalLambda,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Lambda")
	}
	return nil
}

func (s *Server) lambdaCreateFunction(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	roleARN, _ := params["Role"].(string)
	runtime, _ := params["Runtime"].(string)
	handler, _ := params["Handler"].(string)
	desc, _ := params["Description"].(string)
	if name == "" || roleARN == "" || handler == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName, Role, and Handler are required.", readOnly, eventID, verified)
		return
	}
	if runtime != store.LambdaRuntimePython312 {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Runtime must be python3.12.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultLambdaRegion
	}
	resource := store.FunctionARN(verified.AccountID, region, name)
	if !s.authorizeLambda(verified, catalog.ActionLambdaCreateFunction, resource) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:CreateFunction.", readOnly, eventID, verified)
		return
	}
	if err := s.checkLambdaPassRole(verified, roleARN); err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	zipBytes, err := s.lambdaZipFromCode(params, verified.AccountID)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Code.ZipFile is required and must be valid base64.", readOnly, eventID, verified)
		return
	}
	timeout := intParam(params["Timeout"], defaultLambdaTimeout)
	if timeout <= 0 {
		timeout = defaultLambdaTimeout
	}
	memory := intParam(params["MemorySize"], defaultLambdaMemory)
	if memory <= 0 {
		memory = defaultLambdaMemory
	}
	fn, err := s.store.CreateFunction(store.CreateFunctionMeta{
		AccountID:    verified.AccountID,
		Region:       region,
		FunctionName: name,
		RoleARN:      roleARN,
		Runtime:      runtime,
		Handler:      handler,
		Timeout:      timeout,
		Memory:       memory,
		Env:          lambdaEnvFromParams(params),
		Description:  desc,
		Zip:          zipBytes,
	})
	if errors.Is(err, store.ErrFunctionAlreadyExists) {
		s.writeLambdaError(w, r, body, requestID, http.StatusConflict, "ResourceConflictException",
			"Function already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrInvalidFunctionName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid FunctionName.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to create function.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.CreateFunctionJSON(fn)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "CreateFunction", readOnly)
}

func (s *Server) lambdaGetFunction(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return
	}
	fn, err := s.store.GetFunction(verified.AccountID, name)
	if errors.Is(err, store.ErrNoSuchFunction) || errors.Is(err, store.ErrInvalidFunctionName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to get function.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaGetFunction, fn.FunctionARN) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:GetFunction.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.GetFunctionJSON(fn)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "GetFunction", readOnly)
}

func (s *Server) lambdaDeleteFunction(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return
	}
	fn, err := s.store.GetFunction(verified.AccountID, name)
	if errors.Is(err, store.ErrNoSuchFunction) || errors.Is(err, store.ErrInvalidFunctionName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to load function.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaDeleteFunction, fn.FunctionARN) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:DeleteFunction.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteFunction(verified.AccountID, name); err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to delete function.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.EmptyOKJSON()
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "DeleteFunction", readOnly)
}

func (s *Server) lambdaListFunctions(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeLambda(verified, catalog.ActionLambdaListFunctions, "*") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:ListFunctions.", readOnly, eventID, verified)
		return
	}
	fns, err := s.store.ListFunctions(verified.AccountID)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to list functions.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.ListFunctionsJSON(fns)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "ListFunctions", readOnly)
}

func (s *Server) lambdaUpdateFunctionCode(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return
	}
	fn, err := s.store.GetFunction(verified.AccountID, name)
	if errors.Is(err, store.ErrNoSuchFunction) || errors.Is(err, store.ErrInvalidFunctionName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to load function.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaUpdateFunctionCode, fn.FunctionARN) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:UpdateFunctionCode.", readOnly, eventID, verified)
		return
	}
	zipBytes, err := s.lambdaZipFromCode(params, verified.AccountID)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"ZipFile is required and must be valid base64.", readOnly, eventID, verified)
		return
	}
	updated, err := s.store.UpdateFunctionCode(verified.AccountID, name, zipBytes)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to update function code.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.CreateFunctionJSON(updated)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "UpdateFunctionCode", readOnly)
}

func (s *Server) lambdaUpdateFunctionConfiguration(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return
	}
	fn, err := s.store.GetFunction(verified.AccountID, name)
	if errors.Is(err, store.ErrNoSuchFunction) || errors.Is(err, store.ErrInvalidFunctionName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to load function.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaUpdateFunctionConfiguration, fn.FunctionARN) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:UpdateFunctionConfiguration.", readOnly, eventID, verified)
		return
	}
	meta := store.UpdateFunctionConfigurationMeta{
		RoleARN: fn.RoleARN,
		Timeout: fn.Timeout,
		Memory:  fn.Memory,
		Handler: fn.Handler,
		Env:     fn.Env,
		Runtime: fn.Runtime,
	}
	if role, ok := params["Role"].(string); ok && role != "" {
		if err := s.checkLambdaPassRole(verified, role); err != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		meta.RoleARN = role
	}
	if handler, ok := params["Handler"].(string); ok && handler != "" {
		meta.Handler = handler
	}
	if runtime, ok := params["Runtime"].(string); ok && runtime != "" {
		if runtime != store.LambdaRuntimePython312 {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Runtime must be python3.12.", readOnly, eventID, verified)
			return
		}
		meta.Runtime = runtime
	}
	if _, ok := params["Timeout"]; ok {
		meta.Timeout = intParam(params["Timeout"], fn.Timeout)
	}
	if _, ok := params["MemorySize"]; ok {
		meta.Memory = intParam(params["MemorySize"], fn.Memory)
	}
	if _, ok := params["Environment"]; ok {
		meta.Env = lambdaEnvFromParams(params)
	}
	updated, err := s.store.UpdateFunctionConfiguration(verified.AccountID, name, meta)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to update function configuration.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.CreateFunctionJSON(updated)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "UpdateFunctionConfiguration", readOnly)
}

func (s *Server) lambdaInvoke(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return
	}
	fn, err := s.store.GetFunction(verified.AccountID, name)
	if errors.Is(err, store.ErrNoSuchFunction) || errors.Is(err, store.ErrInvalidFunctionName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to load function.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaInvoke, fn.FunctionARN) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:InvokeFunction.", readOnly, eventID, verified)
		return
	}
	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
			"compute unavailable", readOnly, eventID, verified)
		return
	}
	cli, err := s.computeClient()
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
			"compute unavailable", readOnly, eventID, verified)
		return
	}

	accountID, roleName, ok := sts.ParseRoleARN(fn.RoleARN)
	if !ok {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Function role ARN is invalid.", readOnly, eventID, verified)
		return
	}
	secret, err := randomSecret()
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to mint credentials.", readOnly, eventID, verified)
		return
	}
	sessionToken, err := randomSecret()
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to mint credentials.", readOnly, eventID, verified)
		return
	}
	expires := s.now().UTC().Add(defaultSessionDuration)
	accessKeyID, err := s.store.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    accountID,
		RoleARN:      fn.RoleARN,
		SessionName:  lambdaInvokeSession,
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      expires,
	})
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to mint execution role credentials.", readOnly, eventID, verified)
		return
	}
	_ = roleName

	endpoint := strings.TrimSpace(s.cfg.LambdaEndpointURL)
	if endpoint == "" {
		endpoint = defaultLambdaEndpoint
	}
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     accessKeyID,
		"AWS_SECRET_ACCESS_KEY": secret,
		"AWS_SESSION_TOKEN":     sessionToken,
		"AWS_DEFAULT_REGION":    store.DefaultLambdaRegion,
		"AWS_REGION":            store.DefaultLambdaRegion,
		"AWS_ENDPOINT_URL":      endpoint,
		"AWS_ENDPOINT_URL_STS":  endpoint,
		"AWS_ENDPOINT_URL_IAM":  endpoint,
		"AWS_ENDPOINT_URL_S3":   endpoint,
		"AWS_ENDPOINT_URL_DYNAMODB": endpoint,
		"AWS_ENDPOINT_URL_SQS":  endpoint,
		"AWS_ENDPOINT_URL_LAMBDA": endpoint,
		"AWS_ENDPOINT_URL_KMS":  endpoint,
	}
	for k, v := range fn.Env {
		env[k] = v
	}

	eventJSON, err := invokeEventJSON(params["Payload"])
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Payload must be JSON or base64-encoded JSON.", readOnly, eventID, verified)
		return
	}

	codePath := store.FunctionCodeDirInContainer(s.cfg.DataRoot, verified.AccountID, name)
	result, err := cli.RunInvoke(r.Context(), compute.RunOpts{
		CodeHostPath: codePath,
		Handler:      fn.Handler,
		TimeoutSec:   fn.Timeout,
		Env:          env,
		EventJSON:    eventJSON,
		EndpointURL:  endpoint,
	})
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Invoke failed: "+err.Error(), readOnly, eventID, verified)
		return
	}
	// AWS CLI uses REST Invoke: HTTP body is the raw function payload.
	if strings.Contains(r.URL.Path, "/invocations") {
		s.writeLambdaInvokeREST(w, requestID, result.Payload)
		s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "Invoke", readOnly)
		return
	}
	payload, err := lambdasvc.InvokeJSON(result.Payload, 200)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "Invoke", readOnly)
}

func (s *Server) writeLambdaInvokeREST(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("X-Amz-Executed-Version", "$LATEST")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if len(payload) == 0 {
		_, _ = w.Write([]byte("null"))
		return
	}
	_, _ = w.Write(payload)
}

func invokeEventJSON(v any) (string, error) {
	if v == nil {
		return "{}", nil
	}
	switch t := v.(type) {
	case string:
		if t == "" {
			return "{}", nil
		}
		if json.Valid([]byte(t)) {
			return t, nil
		}
		raw, err := base64.StdEncoding.DecodeString(t)
		if err != nil {
			return "", err
		}
		if !json.Valid(raw) {
			return "", errors.New("decoded payload is not JSON")
		}
		return string(raw), nil
	case map[string]any, []any:
		raw, err := json.Marshal(t)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
}

func (s *Server) computeClient() (*compute.Client, error) {
	s.computeOnce.Do(func() {
		s.compute, s.computeErr = compute.NewClient(s.cfg.DockerHost)
	})
	return s.compute, s.computeErr
}

func (s *Server) writeLambdaOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", lambdaJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeLambdaError(
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
	w.Header().Set("Content-Type", lambdaJSONContentType)
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
