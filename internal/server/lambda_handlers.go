package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	lambdasvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/lambda"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/google/uuid"
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
	case catalog.ActionLambdaPublishVersion:
		s.lambdaPublishVersion(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaListVersionsByFunction:
		s.lambdaListVersionsByFunction(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaCreateAlias:
		s.lambdaCreateAlias(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaUpdateAlias:
		s.lambdaUpdateAlias(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaDeleteAlias:
		s.lambdaDeleteAlias(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaGetAlias:
		s.lambdaGetAlias(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaListAliases:
		s.lambdaListAliases(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaPublishLayerVersion:
		s.lambdaPublishLayerVersion(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaGetLayerVersion:
		s.lambdaGetLayerVersion(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaListLayerVersions:
		s.lambdaListLayerVersions(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaDeleteLayerVersion:
		s.lambdaDeleteLayerVersion(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaAddPermission:
		s.lambdaAddPermission(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaRemovePermission:
		s.lambdaRemovePermission(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaGetPolicy:
		s.lambdaGetPolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaCreateEventSourceMapping:
		s.lambdaCreateEventSourceMapping(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaGetEventSourceMapping:
		s.lambdaGetEventSourceMapping(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaListEventSourceMappings:
		s.lambdaListEventSourceMappings(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaUpdateEventSourceMapping:
		s.lambdaUpdateEventSourceMapping(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaDeleteEventSourceMapping:
		s.lambdaDeleteEventSourceMapping(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaCreateFunctionUrlConfig:
		s.lambdaCreateFunctionUrlConfig(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaGetFunctionUrlConfig:
		s.lambdaGetFunctionUrlConfig(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaDeleteFunctionUrlConfig:
		s.lambdaDeleteFunctionUrlConfig(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaListFunctionUrlConfigs:
		s.lambdaListFunctionUrlConfigs(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaListTags:
		s.lambdaListTags(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaGetFunctionCodeSigningConfig:
		s.lambdaGetFunctionCodeSigningConfig(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaPutFunctionEventInvokeConfig:
		s.lambdaPutFunctionEventInvokeConfig(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaGetFunctionEventInvokeConfig:
		s.lambdaGetFunctionEventInvokeConfig(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLambdaDeleteFunctionEventInvokeConfig:
		s.lambdaDeleteFunctionEventInvokeConfig(w, r, body, requestID, eventID, verified, readOnly, params)
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
	case "PublishVersion":
		return catalog.ActionLambdaPublishVersion
	case "ListVersionsByFunction":
		return catalog.ActionLambdaListVersionsByFunction
	case "CreateAlias":
		return catalog.ActionLambdaCreateAlias
	case "UpdateAlias":
		return catalog.ActionLambdaUpdateAlias
	case "DeleteAlias":
		return catalog.ActionLambdaDeleteAlias
	case "GetAlias":
		return catalog.ActionLambdaGetAlias
	case "ListAliases":
		return catalog.ActionLambdaListAliases
	case "PublishLayerVersion":
		return catalog.ActionLambdaPublishLayerVersion
	case "GetLayerVersion":
		return catalog.ActionLambdaGetLayerVersion
	case "ListLayerVersions":
		return catalog.ActionLambdaListLayerVersions
	case "DeleteLayerVersion":
		return catalog.ActionLambdaDeleteLayerVersion
	case "AddPermission":
		return catalog.ActionLambdaAddPermission
	case "RemovePermission":
		return catalog.ActionLambdaRemovePermission
	case "GetPolicy":
		return catalog.ActionLambdaGetPolicy
	case "CreateEventSourceMapping":
		return catalog.ActionLambdaCreateEventSourceMapping
	case "GetEventSourceMapping":
		return catalog.ActionLambdaGetEventSourceMapping
	case "ListEventSourceMappings":
		return catalog.ActionLambdaListEventSourceMappings
	case "UpdateEventSourceMapping":
		return catalog.ActionLambdaUpdateEventSourceMapping
	case "DeleteEventSourceMapping":
		return catalog.ActionLambdaDeleteEventSourceMapping
	case "CreateFunctionUrlConfig":
		return catalog.ActionLambdaCreateFunctionUrlConfig
	case "GetFunctionUrlConfig":
		return catalog.ActionLambdaGetFunctionUrlConfig
	case "DeleteFunctionUrlConfig":
		return catalog.ActionLambdaDeleteFunctionUrlConfig
	case "ListFunctionUrlConfigs":
		return catalog.ActionLambdaListFunctionUrlConfigs
	case "ListTags":
		return catalog.ActionLambdaListTags
	case "GetFunctionCodeSigningConfig":
		return catalog.ActionLambdaGetFunctionCodeSigningConfig
	case "PutFunctionEventInvokeConfig":
		return catalog.ActionLambdaPutFunctionEventInvokeConfig
	case "GetFunctionEventInvokeConfig":
		return catalog.ActionLambdaGetFunctionEventInvokeConfig
	case "DeleteFunctionEventInvokeConfig":
		return catalog.ActionLambdaDeleteFunctionEventInvokeConfig
	default:
		return action
	}
}

func (s *Server) authorizeLambda(verified *authn.Verified, action, resource, resourcePolicy string) bool {
	resourceAccountID := resourceAccountIDFromARN(resource)
	if resourceAccountID == "" {
		resourceAccountID = verified.AccountID
	}
	return s.authorizeDataplaneOR(verified, action, resource, resourceAccountID, func(caller authz.RequestContext, identityDocs []string, resourceAccountID string) authz.Decision {
		return authz.EvaluateDynamoDB(authz.DynamoDBRequest{
			Caller:            caller,
			IdentityDocs:      identityDocs,
			ResourcePolicyDoc: resourcePolicy,
			ResourceAccountID: resourceAccountID,
		})
	})
}

// resourceAccountIDFromARN extracts the account segment from standard AWS ARNs
// (arn:aws:service:region:account:...). Returns empty for non-ARNs or S3-style
// ARNs without an account id.
func resourceAccountIDFromARN(arn string) string {
	arn = strings.TrimSpace(arn)
	parts := strings.Split(arn, ":")
	if len(parts) < 5 || parts[0] != "arn" {
		return ""
	}
	return parts[4]
}

func functionNameParam(params map[string]any) string {
	raw, _ := params["FunctionName"].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	name, _ := store.ParseFunctionQualifier(raw)
	return name
}

func functionQualifierParam(params map[string]any) string {
	raw, _ := params["FunctionName"].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if q, ok := params["Qualifier"].(string); ok && strings.TrimSpace(q) != "" {
			return strings.TrimSpace(q)
		}
		return "$LATEST"
	}
	_, qual := store.ParseFunctionQualifier(raw)
	if q, ok := params["Qualifier"].(string); ok && strings.TrimSpace(q) != "" {
		return strings.TrimSpace(q)
	}
	return qual
}

func functionNameAndQualifier(params map[string]any) (name, qualifier string) {
	raw, _ := params["FunctionName"].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "$LATEST"
	}
	name, qual := store.ParseFunctionQualifier(raw)
	if q, ok := params["Qualifier"].(string); ok && strings.TrimSpace(q) != "" {
		qual = strings.TrimSpace(q)
	}
	return name, qual
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

func lambdaDLQFromParams(params map[string]any) (deadLetterTarget, onFailure string) {
	deadLetterTarget, onFailure, _ = lambdaDestinationsFromParams(params)
	return deadLetterTarget, onFailure
}

// lambdaDestinationsFromParams parses DeadLetterConfig and DestinationConfig.
// AWS shape: DestinationConfig.OnFailure/OnSuccess are objects with Destination ARN.
// Bare-string OnFailure is accepted as a lab alias for Create/UpdateFunctionConfiguration.
func lambdaDestinationsFromParams(params map[string]any) (deadLetterTarget, onFailure, onSuccess string) {
	if dlc, ok := params["DeadLetterConfig"].(map[string]any); ok {
		deadLetterTarget, _ = dlc["TargetArn"].(string)
	}
	if dc, ok := params["DestinationConfig"].(map[string]any); ok {
		onFailure = destinationARNFromConfig(dc["OnFailure"])
		onSuccess = destinationARNFromConfig(dc["OnSuccess"])
	}
	return strings.TrimSpace(deadLetterTarget), strings.TrimSpace(onFailure), strings.TrimSpace(onSuccess)
}

func destinationARNFromConfig(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if dest, ok := v["Destination"].(string); ok {
			return strings.TrimSpace(dest)
		}
	}
	return ""
}

func lambdaLayersFromParams(params map[string]any) []string {
	raw, ok := params["Layers"]
	if !ok {
		return nil
	}
	switch t := raw.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func layerNameParam(params map[string]any) string {
	raw, _ := params["LayerName"].(string)
	return strings.TrimSpace(raw)
}

func layerVersionNumberParam(params map[string]any) (int, error) {
	switch v := params["VersionNumber"].(type) {
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n <= 0 {
			return 0, store.ErrNoSuchLayer
		}
		return n, nil
	case float64:
		n := int(v)
		if n <= 0 {
			return 0, store.ErrNoSuchLayer
		}
		return n, nil
	case int:
		if v <= 0 {
			return 0, store.ErrNoSuchLayer
		}
		return v, nil
	default:
		return 0, store.ErrNoSuchLayer
	}
}

func (s *Server) lambdaZipFromContent(params map[string]any) ([]byte, error) {
	content, _ := params["Content"].(map[string]any)
	if content == nil {
		return nil, errors.New("Content.ZipFile is required")
	}
	if z, ok := content["ZipFile"]; ok {
		return lambdasvc.DecodeZipFile(z)
	}
	return nil, errors.New("Content.ZipFile is required")
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

func lambdaPackageTypeFromParams(params map[string]any) string {
	raw, _ := params["PackageType"].(string)
	switch strings.TrimSpace(raw) {
	case store.LambdaPackageTypeImage:
		return store.LambdaPackageTypeImage
	default:
		return store.LambdaPackageTypeZip
	}
}

func lambdaImageURIFromCode(params map[string]any) (string, error) {
	code, _ := params["Code"].(map[string]any)
	if code == nil {
		if uri, ok := params["ImageUri"].(string); ok && strings.TrimSpace(uri) != "" {
			return strings.TrimSpace(uri), nil
		}
		return "", errors.New("Code.ImageUri is required")
	}
	if uri, ok := code["ImageUri"].(string); ok && strings.TrimSpace(uri) != "" {
		return strings.TrimSpace(uri), nil
	}
	return "", errors.New("Code.ImageUri is required")
}

func (s *Server) checkLambdaPassRole(verified *authn.Verified, roleARN, sourceARN string) error {
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
		SourceArn:        sourceARN,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Lambda")
	}
	return nil
}

func lambdaVpcConfigRejectMessage() string {
	return "VpcConfig is not supported until ENI attachment exists; omit VpcConfig."
}

func lambdaHasNonEmptyVpcConfig(params map[string]any) bool {
	vc, ok := params["VpcConfig"].(map[string]any)
	if !ok || vc == nil {
		return false
	}
	if subs, ok := vc["SubnetIds"].([]any); ok && len(subs) > 0 {
		return true
	}
	if sgs, ok := vc["SecurityGroupIds"].([]any); ok && len(sgs) > 0 {
		return true
	}
	// Any other non-empty VpcConfig object is also rejected (no silent drop).
	return len(vc) > 0
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
	if lambdaHasNonEmptyVpcConfig(params) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			lambdaVpcConfigRejectMessage(), readOnly, eventID, verified)
		return
	}
	name := functionNameParam(params)
	roleARN, _ := params["Role"].(string)
	runtime, _ := params["Runtime"].(string)
	handler, _ := params["Handler"].(string)
	desc, _ := params["Description"].(string)
	packageType := lambdaPackageTypeFromParams(params)
	if name == "" || roleARN == "" || handler == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName, Role, and Handler are required.", readOnly, eventID, verified)
		return
	}
	if packageType == store.LambdaPackageTypeZip {
		if err := store.ValidateLambdaRuntime(runtime); err != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				store.LambdaRuntimeValidationMessage(), readOnly, eventID, verified)
			return
		}
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultLambdaRegion
	}
	resource := store.FunctionARN(verified.AccountID, region, name)
	if !s.authorizeLambda(verified, catalog.ActionLambdaCreateFunction, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:CreateFunction.", readOnly, eventID, verified)
		return
	}
	if err := s.checkLambdaPassRole(verified, roleARN, resource); err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	var zipBytes []byte
	var imageURI string
	var err error
	if packageType == store.LambdaPackageTypeImage {
		imageURI, err = lambdaImageURIFromCode(params)
		if err != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Code.ImageUri is required for PackageType Image.", readOnly, eventID, verified)
			return
		}
	} else {
		zipBytes, err = s.lambdaZipFromCode(params, verified.AccountID)
		if err != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Code.ZipFile is required and must be valid base64.", readOnly, eventID, verified)
			return
		}
	}
	timeout := clampLambdaTimeout(intParam(params["Timeout"], defaultLambdaTimeout))
	memory := clampLambdaMemory(intParam(params["MemorySize"], defaultLambdaMemory))
	env := lambdaEnvFromParams(params)
	if err := validateLambdaEnvKeys(env); err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	deadLetter, onFailure, onSuccess := lambdaDestinationsFromParams(params)
	fn, err := s.store.CreateFunction(store.CreateFunctionMeta{
		AccountID:               verified.AccountID,
		Region:                  region,
		FunctionName:            name,
		RoleARN:                 roleARN,
		Runtime:                 runtime,
		Handler:                 handler,
		Timeout:                 timeout,
		Memory:                  memory,
		Env:                     env,
		Description:             desc,
		PackageType:             packageType,
		Zip:                     zipBytes,
		ImageURI:                imageURI,
		Layers:                  lambdaLayersFromParams(params),
		DeadLetterTargetArn:     deadLetter,
		DestinationOnFailureArn: onFailure,
		DestinationOnSuccessArn: onSuccess,
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
	if errors.Is(err, store.ErrInvalidRuntime) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			store.LambdaRuntimeValidationMessage(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrTooManyLayers) || errors.Is(err, store.ErrInvalidLayerARN) || errors.Is(err, store.ErrNoSuchLayer) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid Layers.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to create function.", readOnly, eventID, verified)
		return
	}
	s.store.EnrichFunctionLayerSizes(&fn)
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
	name, qualifier := functionNameAndQualifier(params)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return
	}
	base, err := s.store.GetFunction(verified.AccountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaGetFunction, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:GetFunction.", readOnly, eventID, verified)
		return
	}
	qf, err := s.store.GetFunctionByQualifier(verified.AccountID, name, qualifier)
	if errors.Is(err, store.ErrNoSuchVersion) || errors.Is(err, store.ErrNoSuchAlias) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to get function.", readOnly, eventID, verified)
		return
	}
	s.store.EnrichFunctionLayerSizes(&qf.LambdaFunction)
	payload, err := lambdasvc.GetFunctionQualifiedJSON(qf)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaDeleteFunction, fn.FunctionARN, fn.ResourcePolicy) {
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaListFunctions, "*", "") {
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
	for i := range fns {
		s.store.EnrichFunctionLayerSizes(&fns[i])
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaUpdateFunctionCode, fn.FunctionARN, fn.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:UpdateFunctionCode.", readOnly, eventID, verified)
		return
	}
	var updated store.LambdaFunction
	if fn.PackageType == store.LambdaPackageTypeImage {
		imageURI, imgErr := lambdaImageURIFromCode(params)
		if imgErr != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"ImageUri is required and must be a pullable container image.", readOnly, eventID, verified)
			return
		}
		updated, err = s.store.UpdateFunctionImageCode(verified.AccountID, name, imageURI)
	} else {
		zipBytes, zipErr := s.lambdaZipFromCode(params, verified.AccountID)
		if zipErr != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"ZipFile is required and must be valid base64.", readOnly, eventID, verified)
			return
		}
		updated, err = s.store.UpdateFunctionCode(verified.AccountID, name, zipBytes)
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to update function code.", readOnly, eventID, verified)
		return
	}
	s.store.EnrichFunctionLayerSizes(&updated)
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
	if lambdaHasNonEmptyVpcConfig(params) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			lambdaVpcConfigRejectMessage(), readOnly, eventID, verified)
		return
	}
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaUpdateFunctionConfiguration, fn.FunctionARN, fn.ResourcePolicy) {
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
		if err := s.checkLambdaPassRole(verified, role, fn.FunctionARN); err != nil {
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
		if fn.PackageType == store.LambdaPackageTypeZip {
			if err := store.ValidateLambdaRuntime(runtime); err != nil {
				s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					store.LambdaRuntimeValidationMessage(), readOnly, eventID, verified)
				return
			}
		}
		meta.Runtime = runtime
	}
	if _, ok := params["Timeout"]; ok {
		meta.Timeout = clampLambdaTimeout(intParam(params["Timeout"], fn.Timeout))
	}
	if _, ok := params["MemorySize"]; ok {
		meta.Memory = clampLambdaMemory(intParam(params["MemorySize"], fn.Memory))
	}
	if _, ok := params["Environment"]; ok {
		env := lambdaEnvFromParams(params)
		if err := validateLambdaEnvKeys(env); err != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		meta.Env = env
	}
	if _, ok := params["Layers"]; ok {
		layers := lambdaLayersFromParams(params)
		meta.Layers = &layers
	}
	if _, ok := params["DeadLetterConfig"]; ok {
		deadLetter, _, _ := lambdaDestinationsFromParams(params)
		meta.DeadLetterTargetArn = &deadLetter
	}
	if _, ok := params["DestinationConfig"]; ok {
		_, onFailure, onSuccess := lambdaDestinationsFromParams(params)
		meta.DestinationOnFailureArn = &onFailure
		meta.DestinationOnSuccessArn = &onSuccess
	}
	updated, err := s.store.UpdateFunctionConfiguration(verified.AccountID, name, meta)
	if errors.Is(err, store.ErrInvalidRuntime) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			store.LambdaRuntimeValidationMessage(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrTooManyLayers) || errors.Is(err, store.ErrInvalidLayerARN) || errors.Is(err, store.ErrNoSuchLayer) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid Layers.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to update function configuration.", readOnly, eventID, verified)
		return
	}
	s.store.EnrichFunctionLayerSizes(&updated)
	payload, err := lambdasvc.CreateFunctionJSON(updated)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "UpdateFunctionConfiguration", readOnly)
}

func (s *Server) lambdaPublishVersion(
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
	base, err := s.store.GetFunction(verified.AccountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaPublishVersion, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:PublishVersion.", readOnly, eventID, verified)
		return
	}
	version, err := s.store.PublishVersion(verified.AccountID, name)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to publish version.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.PublishVersionJSON(version)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "PublishVersion", readOnly)
}

func (s *Server) lambdaGetFunctionCodeSigningConfig(
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
	base, err := s.store.GetFunction(verified.AccountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaGetFunctionCodeSigningConfig, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:GetFunctionCodeSigningConfig.", readOnly, eventID, verified)
		return
	}
	// Lab has no code signing. Return success without CodeSigningConfigArn so Terraform
	// provider v5 refresh does not treat CodeSigningConfigNotFoundException as hard fail.
	payload, err := json.Marshal(map[string]any{"FunctionName": base.FunctionName})
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "GetFunctionCodeSigningConfig", readOnly)
}

func (s *Server) lambdaPutFunctionEventInvokeConfig(
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaPutFunctionEventInvokeConfig, fn.FunctionARN, fn.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:PutFunctionEventInvokeConfig.", readOnly, eventID, verified)
		return
	}
	_, onFailure, onSuccess := lambdaDestinationsFromParams(params)
	updated, err := s.store.PutFunctionEventInvokeConfig(verified.AccountID, name, onFailure, onSuccess)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to put event invoke config.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.EventInvokeConfigJSON(updated)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "PutFunctionEventInvokeConfig", readOnly)
}

func (s *Server) lambdaGetFunctionEventInvokeConfig(
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaGetFunctionEventInvokeConfig, fn.FunctionARN, fn.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:GetFunctionEventInvokeConfig.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.EventInvokeConfigJSON(fn)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "GetFunctionEventInvokeConfig", readOnly)
}

func (s *Server) lambdaDeleteFunctionEventInvokeConfig(
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaDeleteFunctionEventInvokeConfig, fn.FunctionARN, fn.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:DeleteFunctionEventInvokeConfig.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteFunctionEventInvokeConfig(verified.AccountID, name); err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to delete event invoke config.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, []byte("{}"))
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "DeleteFunctionEventInvokeConfig", readOnly)
}

func (s *Server) lambdaListVersionsByFunction(
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
	base, err := s.store.GetFunction(verified.AccountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaListVersionsByFunction, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:ListVersionsByFunction.", readOnly, eventID, verified)
		return
	}
	versions, err := s.store.ListVersionsByFunction(verified.AccountID, name)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to list versions.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.ListVersionsByFunctionJSON(base, versions)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "ListVersionsByFunction", readOnly)
}

func (s *Server) lambdaListTags(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	resource, _ := params["Resource"].(string)
	resource = strings.TrimSpace(resource)
	if resource == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			"Resource is required.", readOnly, eventID, verified)
		return
	}
	name, _ := store.ParseFunctionQualifier(resource)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			"Invalid Resource ARN.", readOnly, eventID, verified)
		return
	}
	accountID := verified.AccountID
	if acct := resourceAccountIDFromARN(resource); acct != "" {
		accountID = acct
	}
	base, err := s.store.GetFunction(accountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaListTags, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:ListTags.", readOnly, eventID, verified)
		return
	}
	tags, err := s.store.ListResourceTags(accountID, base.FunctionARN)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to list tags.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.ListTagsJSON(resourceTagsToMap(tags))
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "ListTags", readOnly)
}

func lambdaAliasVersionParam(params map[string]any) (int, error) {
	switch v := params["FunctionVersion"].(type) {
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n <= 0 {
			return 0, store.ErrInvalidFunctionVersion
		}
		return n, nil
	case float64:
		n := int(v)
		if n <= 0 {
			return 0, store.ErrInvalidFunctionVersion
		}
		return n, nil
	case int:
		if v <= 0 {
			return 0, store.ErrInvalidFunctionVersion
		}
		return v, nil
	default:
		return 0, store.ErrInvalidFunctionVersion
	}
}

func (s *Server) lambdaCreateAlias(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	aliasName, _ := params["Name"].(string)
	aliasName = strings.TrimSpace(aliasName)
	desc, _ := params["Description"].(string)
	if name == "" || aliasName == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName and Name are required.", readOnly, eventID, verified)
		return
	}
	version, err := lambdaAliasVersionParam(params)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionVersion must be a published version number.", readOnly, eventID, verified)
		return
	}
	base, err := s.store.GetFunction(verified.AccountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaCreateAlias, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:CreateAlias.", readOnly, eventID, verified)
		return
	}
	alias, err := s.store.CreateLambdaAlias(verified.AccountID, name, aliasName, version, desc)
	if errors.Is(err, store.ErrAliasAlreadyExists) {
		s.writeLambdaError(w, r, body, requestID, http.StatusConflict, "ResourceConflictException",
			"Alias already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrInvalidAliasName) || errors.Is(err, store.ErrInvalidFunctionVersion) || errors.Is(err, store.ErrNoSuchVersion) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Unable to create alias.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to create alias.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.CreateAliasJSON(alias)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "CreateAlias", readOnly)
}

func (s *Server) lambdaUpdateAlias(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	aliasName, _ := params["Name"].(string)
	aliasName = strings.TrimSpace(aliasName)
	desc, _ := params["Description"].(string)
	if name == "" || aliasName == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName and Name are required.", readOnly, eventID, verified)
		return
	}
	version, err := lambdaAliasVersionParam(params)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionVersion must be a published version number.", readOnly, eventID, verified)
		return
	}
	base, err := s.store.GetFunction(verified.AccountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaUpdateAlias, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:UpdateAlias.", readOnly, eventID, verified)
		return
	}
	alias, err := s.store.UpdateLambdaAlias(verified.AccountID, name, aliasName, version, desc)
	if errors.Is(err, store.ErrNoSuchAlias) || errors.Is(err, store.ErrNoSuchVersion) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Alias not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to update alias.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.CreateAliasJSON(alias)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "UpdateAlias", readOnly)
}

func (s *Server) lambdaDeleteAlias(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	aliasName, _ := params["Name"].(string)
	aliasName = strings.TrimSpace(aliasName)
	if name == "" || aliasName == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName and Name are required.", readOnly, eventID, verified)
		return
	}
	base, err := s.store.GetFunction(verified.AccountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaDeleteAlias, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:DeleteAlias.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteLambdaAlias(verified.AccountID, name, aliasName); errors.Is(err, store.ErrNoSuchAlias) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Alias not found.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to delete alias.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.EmptyOKJSON()
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "DeleteAlias", readOnly)
}

func (s *Server) lambdaGetAlias(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	aliasName, _ := params["Name"].(string)
	aliasName = strings.TrimSpace(aliasName)
	if name == "" || aliasName == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName and Name are required.", readOnly, eventID, verified)
		return
	}
	base, err := s.store.GetFunction(verified.AccountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaGetAlias, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:GetAlias.", readOnly, eventID, verified)
		return
	}
	alias, err := s.store.GetLambdaAlias(verified.AccountID, name, aliasName)
	if errors.Is(err, store.ErrNoSuchAlias) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Alias not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to get alias.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.GetAliasJSON(alias)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "GetAlias", readOnly)
}

func (s *Server) lambdaListAliases(
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
	base, err := s.store.GetFunction(verified.AccountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaListAliases, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:ListAliases.", readOnly, eventID, verified)
		return
	}
	aliases, err := s.store.ListLambdaAliases(verified.AccountID, name)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to list aliases.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.ListAliasesJSON(aliases)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "ListAliases", readOnly)
}

func (s *Server) lambdaPublishLayerVersion(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := layerNameParam(params)
	desc, _ := params["Description"].(string)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"LayerName is required.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultLambdaRegion
	}
	resource := store.LayerVersionARN(verified.AccountID, region, name, 1)
	if !s.authorizeLambda(verified, catalog.ActionLambdaPublishLayerVersion, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:PublishLayerVersion.", readOnly, eventID, verified)
		return
	}
	zipBytes, err := s.lambdaZipFromContent(params)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Content.ZipFile is required and must be valid base64.", readOnly, eventID, verified)
		return
	}
	layer, err := s.store.PublishLayerVersion(store.PublishLayerVersionMeta{
		AccountID:   verified.AccountID,
		Region:      region,
		LayerName:   name,
		Description: desc,
		Zip:         zipBytes,
	})
	if errors.Is(err, store.ErrInvalidLayerName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid LayerName.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to publish layer version.", readOnly, eventID, verified)
		return
	}
	codeSize := s.store.LayerCodeSizeBytes(layer.AccountID, layer.LayerARN)
	payload, err := lambdasvc.PublishLayerVersionJSON(layer, codeSize)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "PublishLayerVersion", readOnly)
}

func (s *Server) lambdaGetLayerVersion(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := layerNameParam(params)
	version, err := layerVersionNumberParam(params)
	if name == "" || err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"LayerName and VersionNumber are required.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultLambdaRegion
	}
	resource := store.LayerVersionARN(verified.AccountID, region, name, version)
	if !s.authorizeLambda(verified, catalog.ActionLambdaGetLayerVersion, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:GetLayerVersion.", readOnly, eventID, verified)
		return
	}
	layer, err := s.store.GetLayerVersion(verified.AccountID, name, version)
	if errors.Is(err, store.ErrNoSuchLayer) || errors.Is(err, store.ErrInvalidLayerName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Layer version not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to get layer version.", readOnly, eventID, verified)
		return
	}
	codeSize := s.store.LayerCodeSizeBytes(layer.AccountID, layer.LayerARN)
	payload, err := lambdasvc.GetLayerVersionJSON(layer, codeSize)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "GetLayerVersion", readOnly)
}

func (s *Server) lambdaListLayerVersions(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := layerNameParam(params)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"LayerName is required.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultLambdaRegion
	}
	resource := store.LayerVersionARN(verified.AccountID, region, name, 1)
	if !s.authorizeLambda(verified, catalog.ActionLambdaListLayerVersions, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:ListLayerVersions.", readOnly, eventID, verified)
		return
	}
	layers, err := s.store.ListLayerVersions(verified.AccountID, name)
	if errors.Is(err, store.ErrInvalidLayerName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid LayerName.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to list layer versions.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.ListLayerVersionsJSON(layers)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "ListLayerVersions", readOnly)
}

func (s *Server) lambdaDeleteLayerVersion(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := layerNameParam(params)
	version, err := layerVersionNumberParam(params)
	if name == "" || err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"LayerName and VersionNumber are required.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultLambdaRegion
	}
	resource := store.LayerVersionARN(verified.AccountID, region, name, version)
	if !s.authorizeLambda(verified, catalog.ActionLambdaDeleteLayerVersion, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:DeleteLayerVersion.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteLayerVersion(verified.AccountID, name, version); errors.Is(err, store.ErrNoSuchLayer) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Layer version not found.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to delete layer version.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.EmptyOKJSON()
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "DeleteLayerVersion", readOnly)
}

func (s *Server) lambdaFunctionOrErr(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	name string,
) (store.LambdaFunction, bool) {
	if strings.TrimSpace(name) == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return store.LambdaFunction{}, false
	}
	fn, err := s.store.GetFunction(verified.AccountID, name)
	if errors.Is(err, store.ErrNoSuchFunction) || errors.Is(err, store.ErrInvalidFunctionName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return store.LambdaFunction{}, false
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to load function.", readOnly, eventID, verified)
		return store.LambdaFunction{}, false
	}
	return fn, true
}

func (s *Server) lambdaAddPermission(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	fn, ok := s.lambdaFunctionOrErr(w, r, body, requestID, eventID, verified, readOnly, name)
	if !ok {
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaAddPermission, fn.FunctionARN, fn.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:AddPermission.", readOnly, eventID, verified)
		return
	}
	statementID, _ := params["StatementId"].(string)
	action, _ := params["Action"].(string)
	principal, _ := params["Principal"].(string)
	sourceAccount, _ := params["SourceAccount"].(string)
	sourceARN, _ := params["SourceArn"].(string)
	statement, err := s.store.AddFunctionPermission(verified.AccountID, name, statementID, action, principal, sourceAccount, sourceARN)
	if errors.Is(err, store.ErrLambdaPolicyStatementExists) {
		s.writeLambdaError(w, r, body, requestID, http.StatusConflict, "ResourceConflictException",
			"The statement id specified already exists.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "validation:") {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				strings.TrimPrefix(err.Error(), "validation: "), readOnly, eventID, verified)
			return
		}
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to add permission.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.AddPermissionJSON(statement)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "AddPermission", readOnly)
}

func (s *Server) lambdaRemovePermission(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	fn, ok := s.lambdaFunctionOrErr(w, r, body, requestID, eventID, verified, readOnly, name)
	if !ok {
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaRemovePermission, fn.FunctionARN, fn.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:RemovePermission.", readOnly, eventID, verified)
		return
	}
	statementID, _ := params["StatementId"].(string)
	if err := s.store.RemoveFunctionPermission(verified.AccountID, name, statementID); errors.Is(err, store.ErrLambdaPolicyStatementNotFound) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Statement not found.", readOnly, eventID, verified)
		return
	} else if err != nil {
		if strings.Contains(err.Error(), "validation:") {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				strings.TrimPrefix(err.Error(), "validation: "), readOnly, eventID, verified)
			return
		}
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to remove permission.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.EmptyOKJSON()
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "RemovePermission", readOnly)
}

func (s *Server) lambdaGetPolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := functionNameParam(params)
	fn, ok := s.lambdaFunctionOrErr(w, r, body, requestID, eventID, verified, readOnly, name)
	if !ok {
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaGetPolicy, fn.FunctionARN, fn.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:GetPolicy.", readOnly, eventID, verified)
		return
	}
	policy, err := s.store.GetFunctionPolicy(verified.AccountID, name)
	if errors.Is(err, store.ErrNoSuchResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"The resource you requested does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to get policy.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.GetPolicyJSON(policy)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "GetPolicy", readOnly)
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
	name, qualifier := functionNameAndQualifier(params)
	if name == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return
	}
	rawName, _ := params["FunctionName"].(string)
	accountID := verified.AccountID
	if acct, ok := store.ParseFunctionAccountID(rawName); ok {
		accountID = acct
	}
	base, err := s.store.GetFunction(accountID, name)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaInvoke, base.FunctionARN, base.ResourcePolicy) {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:InvokeFunction.", readOnly, eventID, verified)
		return
	}
	fn, executedVersion, err := s.store.ResolveFunction(accountID, name, qualifier)
	if errors.Is(err, store.ErrNoSuchVersion) || errors.Is(err, store.ErrNoSuchAlias) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to load function.", readOnly, eventID, verified)
		return
	}

	eventJSON, err := invokeEventJSON(params["Payload"])
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Payload must be JSON or base64-encoded JSON.", readOnly, eventID, verified)
		return
	}

	invocationType, _ := params["InvocationType"].(string)
	if invocationType == "" {
		invocationType = "RequestResponse"
	}
	if invocationType == "Event" {
		_, err := s.store.EnqueueAsyncInvoke(accountID, name, qualifier, eventJSON)
		if err != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
				"Unable to enqueue async invoke.", readOnly, eventID, verified)
			return
		}
		// Worker starts via Store.OnAsyncEnqueue (wired in server.New).
		if strings.Contains(r.URL.Path, "/invocations") {
			s.writeLambdaInvokeRESTAccepted(w, requestID, executedVersion)
			s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "Invoke", readOnly)
			return
		}
		payload, err := lambdasvc.InvokeAsyncAcceptedJSON(executedVersion)
		if err != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
				"Unable to build response.", readOnly, eventID, verified)
			return
		}
		s.writeLambdaAccepted(w, requestID, payload)
		s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "Invoke", readOnly)
		return
	}
	if invocationType != "RequestResponse" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"InvocationType must be RequestResponse or Event.", readOnly, eventID, verified)
		return
	}

	result, err := s.executeLambdaInvoke(r.Context(), accountID, name, fn, executedVersion, eventJSON)
	if err != nil {
		if strings.Contains(err.Error(), "compute unavailable") {
			s.writeLambdaError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
				"compute unavailable", readOnly, eventID, verified)
			return
		}
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Invoke failed: "+err.Error(), readOnly, eventID, verified)
		return
	}
	if strings.Contains(r.URL.Path, "/invocations") {
		s.writeLambdaInvokeREST(w, requestID, result, executedVersion)
		s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "Invoke", readOnly)
		return
	}
	payload, err := lambdasvc.InvokeJSON(result, 200, executedVersion)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "Invoke", readOnly)
}

func (s *Server) startAsyncInvoke(job store.LambdaAsyncInvocation, accountID, name, executedVersion string) {
	run := func() {
		ctx := context.Background()
		_ = s.store.ProcessAsyncInvocation(job.InvocationID, store.LambdaAsyncMaxRetries, func() error {
			fn, resolvedVersion, err := s.store.ResolveFunction(accountID, name, job.Qualifier)
			if err != nil {
				return err
			}
			if executedVersion == "" {
				executedVersion = resolvedVersion
			}
			_, err = s.executeLambdaInvoke(ctx, accountID, name, fn, resolvedVersion, job.EventJSON)
			return err
		})
	}
	// Unit tests leave DockerHost empty. Run the worker inline so SQLite writes
	// from retries do not race the next SigV4 key lookup on the same store.
	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		run()
		return
	}
	go run()
}

func (s *Server) executeLambdaInvoke(
	ctx context.Context,
	accountID, name string,
	fn store.LambdaFunction,
	executedVersion, eventJSON string,
) ([]byte, error) {
	if s.lambdaInvokeHook != nil {
		return s.lambdaInvokeHook(ctx, accountID, name, fn, executedVersion, eventJSON)
	}
	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		return nil, errors.New("compute unavailable")
	}
	cli, err := s.lambdaInvoker()
	if err != nil {
		return nil, errors.New("compute unavailable")
	}

	endpoint := strings.TrimSpace(s.cfg.LambdaEndpointURL)
	if endpoint == "" {
		endpoint = defaultLambdaEndpoint
	}
	minted, err := s.mintRoleSessionEnv(fn.RoleARN, lambdaInvokeSession, endpoint, store.DefaultLambdaRegion)
	if err != nil {
		return nil, fmt.Errorf("mint execution role credentials: %w", err)
	}
	env := mergeLambdaInvokeEnv(fn.Env, minted)

	if fn.PackageType == store.LambdaPackageTypeImage {
		eventRel := filepath.Join("lambda", accountID, name, "invoke-events", uuid.NewString())
		eventHostPath := filepath.ToSlash(filepath.Join(s.cfg.DataRoot, eventRel))
		eventAbs := filepath.Join(s.cfg.DataRoot, eventRel)
		if err := os.MkdirAll(eventAbs, store.LabSharedDirMode); err != nil {
			return nil, fmt.Errorf("prepare image invoke event dir: %w", err)
		}
		if err := os.Chmod(eventAbs, store.LabSharedDirMode); err != nil {
			return nil, fmt.Errorf("chmod image invoke event dir: %w", err)
		}
		imageOpts, err := s.prepareLambdaImageRunOpts(accountID, fn, env, eventJSON, endpoint, eventHostPath)
		if err != nil {
			return nil, err
		}
		result, err := cli.RunImageInvoke(ctx, imageOpts)
		if err != nil {
			return nil, err
		}
		return result.Payload, nil
	}

	codePath := store.FunctionCodeDirInContainer(s.cfg.DataRoot, accountID, name)
	if executedVersion != "$LATEST" {
		var version int
		if _, scanErr := fmt.Sscanf(executedVersion, "%d", &version); scanErr == nil && version > 0 {
			codePath = store.VersionCodeDirInContainer(s.cfg.DataRoot, accountID, name, version)
		}
	}
	layerPaths, err := s.store.ResolveLayerCodeDirs(s.cfg.DataRoot, accountID, fn.Layers)
	if err != nil {
		return nil, fmt.Errorf("invalid layer configuration: %w", err)
	}
	result, err := cli.RunInvoke(ctx, compute.RunOpts{
		CodeHostPath:   codePath,
		Runtime:        fn.Runtime,
		Handler:        fn.Handler,
		TimeoutSec:     clampLambdaTimeout(fn.Timeout),
		MemoryMB:       clampLambdaMemory(fn.Memory),
		Env:            env,
		EventJSON:      eventJSON,
		EndpointURL:    endpoint,
		LayerHostPaths: layerPaths,
	})
	if err != nil {
		return nil, err
	}
	return result.Payload, nil
}

func (s *Server) writeLambdaAccepted(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", lambdaJSONContentType)
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write(payload)
}

func (s *Server) writeLambdaInvokeRESTAccepted(w http.ResponseWriter, requestID, executedVersion string) {
	if executedVersion == "" {
		executedVersion = "$LATEST"
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("X-Amz-Executed-Version", executedVersion)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) writeLambdaInvokeREST(w http.ResponseWriter, requestID string, payload []byte, executedVersion string) {
	if executedVersion == "" {
		executedVersion = "$LATEST"
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("X-Amz-Executed-Version", executedVersion)
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
		s.compute, s.computeErr = compute.NewClient(s.cfg.DockerHost, s.cfg.DockerTLSCertPath)
	})
	return s.compute, s.computeErr
}

func (s *Server) lambdaInvoker() (compute.FunctionInvoker, error) {
	s.invokerOnce.Do(func() {
		s.invoker, s.invokerErr = compute.NewFunctionInvoker(compute.InvokerConfig{
			Runtime:           s.cfg.ComputeRuntime,
			DockerHost:        s.cfg.DockerHost,
			DockerTLSCertPath: s.cfg.DockerTLSCertPath,
		})
	})
	return s.invoker, s.invokerErr
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
