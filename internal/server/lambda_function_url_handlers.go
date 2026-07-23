package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	lambdasvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/lambda"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func (s *Server) lambdaCreateFunctionUrlConfig(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	fnName, _ := params["FunctionName"].(string)
	fnName = strings.TrimSpace(fnName)
	if fnName == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return
	}
	authType, _ := params["AuthType"].(string)
	base, _ := store.ParseFunctionQualifier(fnName)
	fn, err := s.store.GetFunction(verified.AccountID, base)
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
	if !s.authorizeLambda(verified, catalog.ActionLambdaCreateFunctionUrlConfig, fn.FunctionARN, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:CreateFunctionUrlConfig.", readOnly, eventID, verified)
		return
	}
	if strings.EqualFold(strings.TrimSpace(authType), store.FunctionURLAuthNone) && !s.cfg.OpenDataPlaneAllowed() {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"AuthType NONE requires NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1 when listen is non-loopback.",
			readOnly, eventID, verified)
		return
	}
	host := strings.TrimPrefix(strings.TrimSpace(s.cfg.ListenAddr), ":")
	if host == "" || strings.HasPrefix(host, "0.0.0.0") || strings.HasPrefix(host, "[::]") {
		host = "127.0.0.1:4566"
	} else if !strings.Contains(host, ":") {
		host = "127.0.0.1" + host
	} else if strings.HasPrefix(s.cfg.ListenAddr, ":") {
		host = "127.0.0.1" + s.cfg.ListenAddr
	}
	corsOrigins := parseFunctionURLCorsAllowOrigins(params)
	if len(corsOrigins) == 0 {
		corsOrigins = append([]string{}, s.cfg.FunctionURLCORSOrigins...)
	}
	u, err := s.store.CreateFunctionURLConfig(store.CreateFunctionURLInput{
		AccountID:        verified.AccountID,
		FunctionName:     base,
		AuthType:         authType,
		EndpointHost:     host,
		CorsAllowOrigins: corsOrigins,
	})
	if errors.Is(err, store.ErrFunctionURLExists) {
		s.writeLambdaError(w, r, body, requestID, http.StatusConflict, "ResourceConflictException",
			"Function URL already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrInvalidFunctionURLAuthType) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to create function URL config.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.FunctionURLConfigJSON(u)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "CreateFunctionUrlConfig", readOnly)
}

func (s *Server) lambdaGetFunctionUrlConfig(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	fnName, _ := params["FunctionName"].(string)
	base, _ := store.ParseFunctionQualifier(fnName)
	fn, err := s.store.GetFunction(verified.AccountID, base)
	if errors.Is(err, store.ErrNoSuchFunction) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to load function.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaGetFunctionUrlConfig, fn.FunctionARN, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:GetFunctionUrlConfig.", readOnly, eventID, verified)
		return
	}
	u, err := s.store.GetFunctionURLConfig(verified.AccountID, base)
	if errors.Is(err, store.ErrNoSuchFunctionURL) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function URL config not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to get function URL config.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.FunctionURLConfigJSON(u)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "GetFunctionUrlConfig", readOnly)
}

func (s *Server) lambdaDeleteFunctionUrlConfig(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	fnName, _ := params["FunctionName"].(string)
	base, _ := store.ParseFunctionQualifier(fnName)
	fn, err := s.store.GetFunction(verified.AccountID, base)
	if errors.Is(err, store.ErrNoSuchFunction) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to load function.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaDeleteFunctionUrlConfig, fn.FunctionARN, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:DeleteFunctionUrlConfig.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteFunctionURLConfig(verified.AccountID, base); err != nil {
		if errors.Is(err, store.ErrNoSuchFunctionURL) {
			s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
				"Function URL config not found.", readOnly, eventID, verified)
			return
		}
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to delete function URL config.", readOnly, eventID, verified)
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusNoContent)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "DeleteFunctionUrlConfig", readOnly)
}

func (s *Server) lambdaListFunctionUrlConfigs(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	resource := "arn:aws:lambda:" + store.DefaultLambdaRegion + ":" + verified.AccountID + ":function:*"
	if !s.authorizeLambda(verified, catalog.ActionLambdaListFunctionUrlConfigs, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:ListFunctionUrlConfigs.", readOnly, eventID, verified)
		return
	}
	fnName, _ := params["FunctionName"].(string)
	urls, err := s.store.ListFunctionURLConfigs(verified.AccountID, fnName)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to list function URL configs.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.ListFunctionURLConfigsJSON(urls)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "ListFunctionUrlConfigs", readOnly)
}

func isFunctionURLPath(path string) bool {
	return strings.HasPrefix(path, "/lambda-url/")
}

func parseFunctionURLPath(path string) (accountID, functionName string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "lambda-url" {
		return "", "", false
	}
	if parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// handleFunctionURLInvoke serves POST/GET/OPTIONS on /lambda-url/{account}/{function}.
func (s *Server) handleFunctionURLInvoke(w http.ResponseWriter, r *http.Request) {
	requestID := newRequestID()
	eventID := newRequestID()
	accountID, functionName, ok := parseFunctionURLPath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	u, err := s.store.GetFunctionURLConfig(accountID, functionName)
	if errors.Is(err, store.ErrNoSuchFunctionURL) {
		http.Error(w, "function URL not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !s.enforceAssociatedWAF(w, accountID, []string{u.FunctionARN}) {
		return
	}

	// CORS lite for AuthType NONE (lab browser invoke). IAM URLs stay SigV4-only.
	if u.AuthType == store.FunctionURLAuthNone {
		if !s.cfg.OpenDataPlaneAllowed() {
			http.Error(w, "open data plane disabled (set NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1)", http.StatusForbidden)
			return
		}
		setFunctionURLCORSHeaders(w, r.Header.Get("Origin"), u.CorsAllowOrigins)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	var verified *authn.Verified
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if u.AuthType == store.FunctionURLAuthIAM {
		verified, err = authn.Verify(r, body, s.now(), sigv4Skew, s.lookupKey)
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !strings.EqualFold(verified.Service, "lambda") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		fn, err := s.store.GetFunction(accountID, functionName)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		urlKeys := map[string]string{
			"lambda:FunctionUrlAuthType":   u.AuthType,
			"lambda:InvokedViaFunctionUrl": "true",
		}
		if !s.authorizeLambdaWithKeys(verified, catalog.ActionLambdaInvokeFunctionUrl, fn.FunctionARN, fn.ResourcePolicy, urlKeys) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	} else {
		verified = &authn.Verified{AccountID: accountID, Region: store.DefaultLambdaRegion}
	}

	eventJSON := "{}"
	if len(body) > 0 {
		if json.Valid(body) {
			eventJSON = string(body)
		} else {
			raw, _ := json.Marshal(map[string]any{
				"version": "2.0",
				"rawPath": r.URL.Path,
				"body":    string(body),
			})
			eventJSON = string(raw)
		}
	}
	fn, executedVersion, err := s.store.ResolveFunction(accountID, functionName, "$LATEST")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	result, err := s.executeLambdaInvoke(r.Context(), accountID, functionName, fn, executedVersion, eventJSON)
	if err != nil {
		if strings.Contains(err.Error(), "compute unavailable") {
			http.Error(w, "compute unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "invoke failed", http.StatusInternalServerError)
		return
	}
	if u.AuthType == store.FunctionURLAuthNone {
		setFunctionURLCORSHeaders(w, r.Header.Get("Origin"), u.CorsAllowOrigins)
	}
	s.writeLambdaInvokeREST(w, requestID, result, executedVersion)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "InvokeFunctionUrl", false)
}

func parseFunctionURLCorsAllowOrigins(params map[string]any) []string {
	cors, _ := params["Cors"].(map[string]any)
	if cors == nil {
		cors, _ = params["cors"].(map[string]any)
	}
	if cors == nil {
		return nil
	}
	raw, ok := cors["AllowOrigins"]
	if !ok {
		raw = cors["allowOrigins"]
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

func setFunctionURLCORSHeaders(w http.ResponseWriter, requestOrigin string, allowOrigins []string) {
	acao := "*"
	if len(allowOrigins) > 0 {
		acao = ""
		req := strings.TrimSpace(requestOrigin)
		for _, o := range allowOrigins {
			if o == "*" {
				acao = "*"
				break
			}
			if req != "" && strings.EqualFold(o, req) {
				acao = req
				break
			}
		}
		if acao == "" {
			// Allowlist configured but Origin missing/not matched: omit ACAO.
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Max-Age", "86400")
			return
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", acao)
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Max-Age", "86400")
}
