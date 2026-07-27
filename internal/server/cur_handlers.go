package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	cursvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cur"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	curJSONContentType = "application/x-amz-json-1.1"
	curEventSource     = "cur.amazonaws.com"
)

func (s *Server) handleCUR(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = curAction(action)

	switch action {
	case catalog.ActionCURPutReportDefinition:
		s.curPut(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCURModifyReportDefinition:
		s.curModify(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCURDescribeReportDefinitions:
		s.curDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCURDeleteReportDefinition:
		s.curDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCURTagResource, catalog.ActionCURUntagResource:
		s.writeCUROK(w, []byte(`{}`))
		s.writeSuccessAudit(r, requestID, eventID, verified, curEventSource, "TagResource", readOnly)
	case catalog.ActionCURListTagsForResource:
		s.writeCUROK(w, []byte(`{"Tags":[]}`))
		s.writeSuccessAudit(r, requestID, eventID, verified, curEventSource, "ListTagsForResource", readOnly)
	default:
		s.writeCURError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This CUR action is not implemented.", readOnly, eventID, verified)
	}
}

func curAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "PutReportDefinition":
		return catalog.ActionCURPutReportDefinition
	case "ModifyReportDefinition":
		return catalog.ActionCURModifyReportDefinition
	case "DescribeReportDefinitions":
		return catalog.ActionCURDescribeReportDefinitions
	case "DeleteReportDefinition":
		return catalog.ActionCURDeleteReportDefinition
	case "TagResource":
		return catalog.ActionCURTagResource
	case "UntagResource":
		return catalog.ActionCURUntagResource
	case "ListTagsForResource":
		return catalog.ActionCURListTagsForResource
	default:
		return action
	}
}

func parseCURReportDefinition(params map[string]any) store.CURReportDefinition {
	var d store.CURReportDefinition
	raw, ok := params["ReportDefinition"].(map[string]any)
	if !ok {
		return d
	}
	d.ReportName, _ = raw["ReportName"].(string)
	d.TimeUnit, _ = raw["TimeUnit"].(string)
	d.Format, _ = raw["Format"].(string)
	d.Compression, _ = raw["Compression"].(string)
	d.S3Bucket, _ = raw["S3Bucket"].(string)
	d.S3Prefix, _ = raw["S3Prefix"].(string)
	d.S3Region, _ = raw["S3Region"].(string)
	d.ReportVersioning, _ = raw["ReportVersioning"].(string)
	if v, ok := raw["RefreshClosedReports"].(bool); ok {
		d.RefreshClosedReports = v
	}
	d.AdditionalSchemaElements = stringSliceFromAny(raw["AdditionalSchemaElements"])
	d.AdditionalArtifacts = stringSliceFromAny(raw["AdditionalArtifacts"])
	return d
}

func stringSliceFromAny(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (s *Server) curRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return "us-east-1"
}

func (s *Server) curPut(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCURPutReportDefinition, "*") {
		s.writeCURError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cur:PutReportDefinition.", readOnly, eventID, verified)
		return
	}
	def := parseCURReportDefinition(params)
	if def.ReportName == "" && params["ReportDefinition"] == nil {
		s.writeCURError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"ReportDefinition is required.", readOnly, eventID, verified)
		return
	}
	created, err := s.store.PutCURReportDefinition(verified.AccountID, s.curRegion(verified), def)
	if errors.Is(err, store.ErrCURDuplicate) {
		s.writeCURError(w, r, body, requestID, http.StatusBadRequest, "DuplicateReportNameException",
			"A report with that name already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCURLimitReached) {
		s.writeCURError(w, r, body, requestID, http.StatusBadRequest, "ReportLimitReachedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCURBadRequest) {
		s.writeCURError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCURError(w, r, body, requestID, http.StatusInternalServerError, "InternalErrorException",
			"Unable to put report definition.", readOnly, eventID, verified)
		return
	}
	// Re-read so ReportStatus reflects emission outcome.
	if fresh, err := s.store.GetCURReportDefinition(verified.AccountID, created.Region, created.ReportName); err == nil {
		created = fresh
	}
	payload, _ := cursvc.PutReportDefinitionJSON(created)
	s.writeCUROK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, curEventSource, "PutReportDefinition", readOnly)
}

func (s *Server) curModify(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCURModifyReportDefinition, "*") {
		s.writeCURError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cur:ModifyReportDefinition.", readOnly, eventID, verified)
		return
	}
	reportName, _ := params["ReportName"].(string)
	def := parseCURReportDefinition(params)
	updated, err := s.store.ModifyCURReportDefinition(verified.AccountID, s.curRegion(verified), reportName, def)
	if errors.Is(err, store.ErrCURNotFound) {
		s.writeCURError(w, r, body, requestID, http.StatusBadRequest, "ReportNotFoundException",
			"Report not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCURBadRequest) {
		s.writeCURError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCURError(w, r, body, requestID, http.StatusInternalServerError, "InternalErrorException",
			"Unable to modify report definition.", readOnly, eventID, verified)
		return
	}
	if fresh, err := s.store.GetCURReportDefinition(verified.AccountID, updated.Region, updated.ReportName); err == nil {
		updated = fresh
	}
	payload, _ := cursvc.ModifyReportDefinitionJSON(updated)
	s.writeCUROK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, curEventSource, "ModifyReportDefinition", readOnly)
}

func (s *Server) curDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionCURDescribeReportDefinitions, "*") {
		s.writeCURError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cur:DescribeReportDefinitions.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.DescribeCURReportDefinitions(verified.AccountID)
	if err != nil {
		s.writeCURError(w, r, body, requestID, http.StatusInternalServerError, "InternalErrorException",
			"Unable to describe report definitions.", readOnly, eventID, verified)
		return
	}
	payload, _ := cursvc.DescribeReportDefinitionsJSON(list)
	s.writeCUROK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, curEventSource, "DescribeReportDefinitions", readOnly)
}

func (s *Server) curDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCURDeleteReportDefinition, "*") {
		s.writeCURError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cur:DeleteReportDefinition.", readOnly, eventID, verified)
		return
	}
	reportName, _ := params["ReportName"].(string)
	region := s.curRegion(verified)
	_, getErr := s.store.GetCURReportDefinition(verified.AccountID, region, reportName)
	deleted := getErr == nil
	if err := s.store.DeleteCURReportDefinition(verified.AccountID, region, reportName); err != nil {
		s.writeCURError(w, r, body, requestID, http.StatusInternalServerError, "InternalErrorException",
			"Unable to delete report definition.", readOnly, eventID, verified)
		return
	}
	payload, _ := cursvc.DeleteReportDefinitionJSON(reportName, deleted)
	s.writeCUROK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, curEventSource, "DeleteReportDefinition", readOnly)
}

func (s *Server) writeCUROK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", curJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCURError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", curJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, curEventSource, code, readOnly)
}
