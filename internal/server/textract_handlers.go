package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	textractsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/textract"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	textractJSONContentType = "application/x-amz-json-1.1"
	textractEventSource     = "textract.amazonaws.com"
)

func (s *Server) handleTextract(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = textractAction(action)

	switch action {
	case catalog.ActionTextractDetectDocumentText:
		s.textractDetect(w, r, body, requestID, eventID, verified, readOnly, params, false)
	case catalog.ActionTextractAnalyzeDocument:
		s.textractDetect(w, r, body, requestID, eventID, verified, readOnly, params, true)
	default:
		s.writeTextractError(w, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Textract action is not implemented.")
	}
}

func textractAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "DetectDocumentText":
		return catalog.ActionTextractDetectDocumentText
	case "AnalyzeDocument":
		return catalog.ActionTextractAnalyzeDocument
	default:
		return action
	}
}

func (s *Server) textractDetect(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any, analyze bool,
) {
	action := catalog.ActionTextractDetectDocumentText
	eventName := "DetectDocumentText"
	if analyze {
		action = catalog.ActionTextractAnalyzeDocument
		eventName = "AnalyzeDocument"
	}
	if !s.authorize(verified, action, "*") {
		s.writeTextractError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform "+action+".")
		return
	}
	hasBytes, bucket, key := parseTextractDocument(params)
	var (
		det store.TextractDetection
		err error
	)
	if analyze {
		var features []string
		if raw, ok := params["FeatureTypes"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					features = append(features, s)
				}
			}
		}
		det, err = s.store.AnalyzeDocumentStub(verified.AccountID, hasBytes, bucket, key, features)
	} else {
		det, err = s.store.DetectDocumentTextStub(verified.AccountID, hasBytes, bucket, key)
	}
	if errors.Is(err, store.ErrTextractInvalidParameter) {
		s.writeTextractError(w, requestID, http.StatusBadRequest, "InvalidParameterException", err.Error())
		return
	}
	if errors.Is(err, store.ErrTextractInvalidS3Object) {
		s.writeTextractError(w, requestID, http.StatusBadRequest, "InvalidS3ObjectException", err.Error())
		return
	}
	if err != nil {
		s.writeTextractError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to detect document text.")
		return
	}
	payload, _ := textractsvc.DetectDocumentTextJSON(det)
	s.writeTextractOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, textractEventSource, eventName, readOnly)
}

func parseTextractDocument(params map[string]any) (hasBytes bool, bucket, key string) {
	doc, _ := params["Document"].(map[string]any)
	if doc == nil {
		return false, "", ""
	}
	if _, ok := doc["Bytes"]; ok {
		hasBytes = true
	}
	if s3, ok := doc["S3Object"].(map[string]any); ok {
		bucket, _ = s3["Bucket"].(string)
		key, _ = s3["Name"].(string)
	}
	return hasBytes, bucket, key
}

func (s *Server) writeTextractOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", textractJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeTextractError(w http.ResponseWriter, requestID string, status int, code, message string) {
	w.Header().Set("Content-Type", textractJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + message + `"}`))
}
