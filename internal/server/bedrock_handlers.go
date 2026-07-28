package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	bedrockJSONContentType = "application/json"
	bedrockEventSource     = "bedrock.amazonaws.com"
)

func isBedrockRuntimePath(path string) bool {
	if !strings.HasPrefix(path, "/model/") {
		return false
	}
	p := strings.TrimSuffix(path, "/")
	return strings.HasSuffix(p, "/invoke") ||
		strings.HasSuffix(p, "/converse") ||
		strings.HasSuffix(p, "/converse-stream")
}

// resolveBedrockRuntimeREST maps POST /model/{modelId}/{invoke|converse|converse-stream} to catalog actions.
func resolveBedrockRuntimeREST(r *http.Request) (action, modelID string) {
	if r.Method != http.MethodPost {
		return "", ""
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if !strings.HasPrefix(path, "/model/") {
		return "", ""
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "model" {
		return "", ""
	}
	suffix := parts[len(parts)-1]
	var act string
	switch suffix {
	case "invoke":
		act = catalog.ActionBedrockInvokeModel
	case "converse":
		act = catalog.ActionBedrockConverse
	case "converse-stream":
		act = "ConverseStream"
	default:
		return "", ""
	}
	raw := strings.Join(parts[1:len(parts)-1], "/")
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		decoded = raw
	}
	return act, decoded
}

func (s *Server) handleBedrockRuntime(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
	modelIDFromPath string,
) {
	params := jsonBodyMap(body)
	action = bedrockAction(action)

	switch action {
	case catalog.ActionBedrockInvokeModel:
		s.bedrockInvokeModel(w, r, body, requestID, eventID, verified, readOnly, params, modelIDFromPath)
	case catalog.ActionBedrockConverse:
		s.bedrockConverse(w, r, body, requestID, eventID, verified, readOnly, params, modelIDFromPath)
	case "ConverseStream":
		s.bedrockConverseStream(w, r, requestID, verified, modelIDFromPath, params)
	default:
		s.writeBedrockError(w, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Bedrock Runtime action is not implemented.")
	}
}

func bedrockAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "InvokeModel":
		return catalog.ActionBedrockInvokeModel
	case "Converse":
		return catalog.ActionBedrockConverse
	case "ConverseStream":
		return "ConverseStream"
	default:
		return action
	}
}

func (s *Server) bedrockInvokeModel(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any, modelIDFromPath string,
) {
	modelID := bedrockModelID(modelIDFromPath, params)
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	if !s.authorize(verified, catalog.ActionBedrockInvokeModel, "*") {
		s.writeBedrockError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform bedrock:InvokeModel.")
		return
	}
	inv, err := s.store.InvokeBedrockModel(verified.AccountID, modelID, contentType, body)
	if errors.Is(err, store.ErrBedrockValidation) {
		s.writeBedrockError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrBedrockResourceNotFound) {
		s.writeBedrockError(w, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Model id is not allowlisted for this lab stub.")
		return
	}
	if err != nil {
		s.writeBedrockError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to invoke model.")
		return
	}
	w.Header().Set("Content-Type", bedrockJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(inv.ResponseBody))
	s.writeSuccessAudit(r, requestID, eventID, verified, bedrockEventSource, "InvokeModel", readOnly)
}

func (s *Server) bedrockConverse(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any, modelIDFromPath string,
) {
	modelID := bedrockModelID(modelIDFromPath, params)
	if !s.authorize(verified, catalog.ActionBedrockConverse, "*") {
		s.writeBedrockError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform bedrock:Converse.")
		return
	}
	inv, err := s.store.ConverseBedrockModel(verified.AccountID, modelID, body)
	if errors.Is(err, store.ErrBedrockValidation) {
		s.writeBedrockError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrBedrockResourceNotFound) {
		s.writeBedrockError(w, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Model id is not allowlisted for this lab stub.")
		return
	}
	if err != nil {
		s.writeBedrockError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to converse with model.")
		return
	}
	w.Header().Set("Content-Type", bedrockJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(inv.ResponseBody))
	s.writeSuccessAudit(r, requestID, eventID, verified, bedrockEventSource, "Converse", readOnly)
}

func (s *Server) bedrockConverseStream(
	w http.ResponseWriter, r *http.Request, requestID string,
	verified *authn.Verified, modelIDFromPath string, params map[string]any,
) {
	modelID := bedrockModelID(modelIDFromPath, params)
	if modelID == "" {
		s.writeBedrockError(w, requestID, http.StatusBadRequest, "ValidationException",
			"modelId is required")
		return
	}
	if !s.authorize(verified, catalog.ActionBedrockConverse, "*") {
		s.writeBedrockError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform bedrock:Converse.")
		return
	}
	s.writeBedrockError(w, requestID, http.StatusNotImplemented, "UnsupportedOperationException",
		"ConverseStream is not supported by the Noctaxris stub. Use Converse instead.")
}

func bedrockModelID(modelIDFromPath string, params map[string]any) string {
	modelID := modelIDFromPath
	if modelID == "" {
		modelID, _ = params["modelId"].(string)
		if modelID == "" {
			modelID, _ = params["ModelId"].(string)
		}
	}
	return modelID
}

func (s *Server) writeBedrockError(w http.ResponseWriter, requestID string, status int, code, message string) {
	w.Header().Set("Content-Type", bedrockJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"message":"` + message + `"}`))
}
