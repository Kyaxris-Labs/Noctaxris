package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	bcmexports "github.com/Kyaxris-Labs/Noctaxris/internal/services/bcmexports"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	bcmJSONContentType = "application/x-amz-json-1.1"
	bcmEventSource     = "bcm-data-exports.amazonaws.com"
)

func (s *Server) handleBCMExports(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = bcmExportAction(action)

	switch action {
	case catalog.ActionBCMCreateExport:
		s.bcmCreateExport(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBCMGetExport:
		s.bcmGetExport(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBCMListExports:
		s.bcmListExports(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBCMDeleteExport:
		s.bcmDeleteExport(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeBCMError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This BCM Data Exports action is not implemented.", readOnly, eventID, verified)
	}
}

func bcmExportAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateExport":
		return catalog.ActionBCMCreateExport
	case "GetExport":
		return catalog.ActionBCMGetExport
	case "ListExports":
		return catalog.ActionBCMListExports
	case "DeleteExport":
		return catalog.ActionBCMDeleteExport
	default:
		return action
	}
}

func (s *Server) bcmCreateExport(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBCMCreateExport, "*") {
		s.writeBCMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform bcm-data-exports:CreateExport.", readOnly, eventID, verified)
		return
	}
	name, format, description := "", "CSV", ""
	if exp, ok := params["Export"].(map[string]any); ok {
		name, _ = exp["Name"].(string)
		description, _ = exp["Description"].(string)
		if dest, ok := exp["DestinationConfigurations"].(map[string]any); ok {
			if s3, ok := dest["S3Destination"].(map[string]any); ok {
				if out, ok := s3["S3OutputConfigurations"].(map[string]any); ok {
					if f, ok := out["Format"].(string); ok && f != "" {
						format = f
					}
				}
			}
		}
	}
	if name == "" {
		name, _ = params["Name"].(string)
	}
	region := verified.Region
	exp, err := s.store.CreateBCMExport(verified.AccountID, region, name, description, format)
	if errors.Is(err, store.ErrBCMExportBadRequest) {
		s.writeBCMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBCMError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create export.", readOnly, eventID, verified)
		return
	}
	payload, _ := bcmexports.CreateExportJSON(exp)
	s.writeBCMOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, bcmEventSource, "CreateExport", readOnly)
}

func (s *Server) bcmGetExport(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["ExportArn"].(string)
	if !s.authorize(verified, catalog.ActionBCMGetExport, arn) {
		s.writeBCMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform bcm-data-exports:GetExport.", readOnly, eventID, verified)
		return
	}
	exp, err := s.store.GetBCMExport(verified.AccountID, arn)
	if errors.Is(err, store.ErrBCMExportNotFound) {
		s.writeBCMError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Export not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBCMError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to get export.", readOnly, eventID, verified)
		return
	}
	payload, _ := bcmexports.GetExportJSON(exp)
	s.writeBCMOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, bcmEventSource, "GetExport", readOnly)
}

func (s *Server) bcmListExports(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionBCMListExports, "*") {
		s.writeBCMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform bcm-data-exports:ListExports.", readOnly, eventID, verified)
		return
	}
	exports, err := s.store.ListBCMExports(verified.AccountID)
	if err != nil {
		s.writeBCMError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to list exports.", readOnly, eventID, verified)
		return
	}
	payload, _ := bcmexports.ListExportsJSON(exports)
	s.writeBCMOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, bcmEventSource, "ListExports", readOnly)
}

func (s *Server) bcmDeleteExport(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["ExportArn"].(string)
	if !s.authorize(verified, catalog.ActionBCMDeleteExport, arn) {
		s.writeBCMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform bcm-data-exports:DeleteExport.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteBCMExport(verified.AccountID, arn)
	if errors.Is(err, store.ErrBCMExportNotFound) {
		s.writeBCMError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Export not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBCMError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to delete export.", readOnly, eventID, verified)
		return
	}
	payload, _ := bcmexports.DeleteExportJSON()
	s.writeBCMOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, bcmEventSource, "DeleteExport", readOnly)
}

func (s *Server) writeBCMOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", bcmJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeBCMError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", bcmJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, bcmEventSource, code, readOnly)
}
