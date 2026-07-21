package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ssmsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ssm"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	ssmJSONContentType = "application/x-amz-json-1.1"
	ssmEventSource     = "ssm.amazonaws.com"
)

func (s *Server) handleSSM(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = ssmAction(action)

	switch action {
	case catalog.ActionSSMPutParameter:
		s.ssmPutParameter(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSSMGetParameter:
		s.ssmGetParameter(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSSMGetParameters:
		s.ssmGetParameters(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSSMGetParametersByPath:
		s.ssmGetParametersByPath(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSSMDeleteParameter:
		s.ssmDeleteParameter(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSSMDescribeParameters:
		s.ssmDescribeParameters(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeSSMError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This SSM action is not implemented.", readOnly, eventID, verified)
	}
}

func ssmAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "PutParameter":
		return catalog.ActionSSMPutParameter
	case "GetParameter":
		return catalog.ActionSSMGetParameter
	case "GetParameters":
		return catalog.ActionSSMGetParameters
	case "GetParametersByPath":
		return catalog.ActionSSMGetParametersByPath
	case "DeleteParameter":
		return catalog.ActionSSMDeleteParameter
	case "DescribeParameters":
		return catalog.ActionSSMDescribeParameters
	default:
		return action
	}
}

func (s *Server) ssmRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultSSMRegion
}

func (s *Server) ssmParameterARN(verified *authn.Verified, name string) string {
	return store.ParameterARN(s.ssmRegion(verified), verified.AccountID, name)
}

func (s *Server) authorizeSSM(verified *authn.Verified, action, resource string) bool {
	return s.authorize(verified, action, resource)
}

// ssmAuthorizeKMS resolves the SecureString CMK and evaluates KMS Encrypt/Decrypt.
func (s *Server) ssmAuthorizeKMS(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	keyIDOrAlias, kmsAction string,
) bool {
	keyID, err := s.store.ResolveSSMKeyID(verified.AccountID, keyIDOrAlias)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid KeyId.", readOnly, eventID, verified)
		return false
	}
	key, err := s.store.GetKey(keyID)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid KeyId.", readOnly, eventID, verified)
		return false
	}
	if !s.authorizeKMSOp(verified, kmsAction, key, nil) {
		s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform "+kmsAction+" on the parameter KMS key.", readOnly, eventID, verified)
		return false
	}
	return true
}

func (s *Server) ssmPutParameter(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name, _ := params["Name"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	value, _ := params["Value"].(string)
	paramType, _ := params["Type"].(string)
	if paramType == "" {
		paramType = store.ParamTypeString
	}
	keyID, _ := params["KeyId"].(string)
	overwrite := ssmBoolParam(params["Overwrite"], false)

	arn := s.ssmParameterARN(verified, name)
	if !s.authorizeSSM(verified, catalog.ActionSSMPutParameter, arn) {
		s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ssm:PutParameter.", readOnly, eventID, verified)
		return
	}
	if paramType == store.ParamTypeSecureString {
		if !s.ssmAuthorizeKMS(w, r, body, requestID, eventID, verified, readOnly, keyID, catalog.ActionKMSEncrypt) {
			return
		}
	}

	p, err := s.store.PutParameter(
		verified.AccountID, s.ssmRegion(verified), name, paramType, value, keyID, overwrite,
	)
	if errors.Is(err, store.ErrParameterAlreadyExists) {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ParameterAlreadyExists",
			"The parameter already exists. To overwrite this value, set the overwrite option in the request to true.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put parameter.", readOnly, eventID, verified)
		return
	}

	payload, err := ssmsvc.PutParameterJSON(p.Version)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSSMOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ssmEventSource, "PutParameter", readOnly)
}

func (s *Server) ssmGetParameter(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name, _ := params["Name"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	withDecryption := ssmBoolParam(params["WithDecryption"], false)

	arn := s.ssmParameterARN(verified, name)
	if !s.authorizeSSM(verified, catalog.ActionSSMGetParameter, arn) {
		s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ssm:GetParameter.", readOnly, eventID, verified)
		return
	}

	p, err := s.store.GetParameter(verified.AccountID, name, false)
	if errors.Is(err, store.ErrParameterNotFound) {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ParameterNotFound",
			"Parameter not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get parameter.", readOnly, eventID, verified)
		return
	}
	if withDecryption && p.Type == store.ParamTypeSecureString {
		if !s.ssmAuthorizeKMS(w, r, body, requestID, eventID, verified, readOnly, p.KeyID, catalog.ActionKMSDecrypt) {
			return
		}
		p, err = s.store.GetParameter(verified.AccountID, name, true)
		if err != nil {
			s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to get parameter.", readOnly, eventID, verified)
			return
		}
	}

	includeValue := p.Type == store.ParamTypeString || withDecryption
	payload, err := ssmsvc.GetParameterJSON(p, includeValue)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSSMOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ssmEventSource, "GetParameter", readOnly)
}

func (s *Server) ssmGetParameters(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	names := stringSliceParam(params["Names"])
	if len(names) == 0 {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Names is required.", readOnly, eventID, verified)
		return
	}
	withDecryption := ssmBoolParam(params["WithDecryption"], false)

	for _, name := range names {
		arn := s.ssmParameterARN(verified, name)
		if !s.authorizeSSM(verified, catalog.ActionSSMGetParameters, arn) {
			s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform ssm:GetParameters.", readOnly, eventID, verified)
			return
		}
	}

	found, err := s.store.GetParameters(verified.AccountID, names, false)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get parameters.", readOnly, eventID, verified)
		return
	}
	if withDecryption {
		for _, p := range found {
			if p.Type != store.ParamTypeSecureString {
				continue
			}
			if !s.ssmAuthorizeKMS(w, r, body, requestID, eventID, verified, readOnly, p.KeyID, catalog.ActionKMSDecrypt) {
				return
			}
		}
		found, err = s.store.GetParameters(verified.AccountID, names, true)
		if err != nil {
			s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to get parameters.", readOnly, eventID, verified)
			return
		}
	}

	foundSet := map[string]struct{}{}
	for _, p := range found {
		foundSet[p.Name] = struct{}{}
	}
	invalid := make([]string, 0)
	for _, name := range names {
		if _, ok := foundSet[ssmNormalizeName(name)]; !ok {
			invalid = append(invalid, name)
		}
	}

	payload, err := ssmsvc.GetParametersJSON(found, invalid, withDecryption)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSSMOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ssmEventSource, "GetParameters", readOnly)
}

func (s *Server) ssmGetParametersByPath(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	path, _ := params["Path"].(string)
	if strings.TrimSpace(path) == "" {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Path is required.", readOnly, eventID, verified)
		return
	}
	recursive := ssmBoolParam(params["Recursive"], false)
	withDecryption := ssmBoolParam(params["WithDecryption"], false)

	arn := s.ssmParameterARN(verified, path)
	if !s.authorizeSSM(verified, catalog.ActionSSMGetParametersByPath, arn) {
		s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ssm:GetParametersByPath.", readOnly, eventID, verified)
		return
	}

	found, err := s.store.GetParametersByPath(verified.AccountID, path, recursive, false)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get parameters by path.", readOnly, eventID, verified)
		return
	}
	if withDecryption {
		for _, p := range found {
			if p.Type != store.ParamTypeSecureString {
				continue
			}
			if !s.ssmAuthorizeKMS(w, r, body, requestID, eventID, verified, readOnly, p.KeyID, catalog.ActionKMSDecrypt) {
				return
			}
		}
		found, err = s.store.GetParametersByPath(verified.AccountID, path, recursive, true)
		if err != nil {
			s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to get parameters by path.", readOnly, eventID, verified)
			return
		}
	}

	payload, err := ssmsvc.GetParametersByPathJSON(found, withDecryption)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSSMOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ssmEventSource, "GetParametersByPath", readOnly)
}

func (s *Server) ssmDeleteParameter(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name, _ := params["Name"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}

	arn := s.ssmParameterARN(verified, name)
	if !s.authorizeSSM(verified, catalog.ActionSSMDeleteParameter, arn) {
		s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ssm:DeleteParameter.", readOnly, eventID, verified)
		return
	}

	if err := s.store.DeleteParameter(verified.AccountID, name); errors.Is(err, store.ErrParameterNotFound) {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ParameterNotFound",
			"Parameter not found.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete parameter.", readOnly, eventID, verified)
		return
	}

	payload, err := ssmsvc.EmptyOKJSON()
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSSMOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ssmEventSource, "DeleteParameter", readOnly)
}

func (s *Server) ssmDescribeParameters(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorizeSSM(verified, catalog.ActionSSMDescribeParameters, "*") {
		s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ssm:DescribeParameters.", readOnly, eventID, verified)
		return
	}

	prefix := ssmNamePrefixFromFilters(params["ParameterFilters"])
	paramsList, err := s.store.DescribeParameters(verified.AccountID, prefix)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe parameters.", readOnly, eventID, verified)
		return
	}

	payload, err := ssmsvc.DescribeParametersJSON(paramsList)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSSMOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ssmEventSource, "DescribeParameters", readOnly)
}

func ssmNamePrefixFromFilters(v any) string {
	filters, ok := v.([]any)
	if !ok {
		return ""
	}
	for _, raw := range filters {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key, _ := m["Key"].(string)
		if !strings.EqualFold(key, "Name") {
			continue
		}
		opt, _ := m["Option"].(string)
		if opt != "" && !strings.EqualFold(opt, "BeginsWith") {
			continue
		}
		vals := stringSliceParam(m["Values"])
		if len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func ssmBoolParam(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

func ssmNormalizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	return name
}

func (s *Server) writeSSMOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", ssmJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeSSMError(
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
	w.Header().Set("Content-Type", ssmJSONContentType)
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
