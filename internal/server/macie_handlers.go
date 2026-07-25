package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	maciesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/macie2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const macieEventSource = "macie2.amazonaws.com"

func (s *Server) handleMacie(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = macieAction(action)

	switch action {
	case catalog.ActionMacieEnableMacie:
		s.macieEnable(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionMacieGetMacieSession:
		s.macieGetSession(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionMacieCreateClassificationJob:
		s.macieCreateJob(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMacieDescribeClassificationJob:
		s.macieDescribeJob(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMacieListClassificationJobs:
		s.macieListJobs(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMacieListFindings:
		s.macieListFindings(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMacieGetFindings:
		s.macieGetFindings(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMacieInjectFindings:
		s.macieInjectFindings(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeMacieError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Macie action is not implemented.", readOnly, eventID, verified)
	}
}

func macieAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "EnableMacie":
		return catalog.ActionMacieEnableMacie
	case "GetMacieSession":
		return catalog.ActionMacieGetMacieSession
	case "CreateClassificationJob":
		return catalog.ActionMacieCreateClassificationJob
	case "DescribeClassificationJob":
		return catalog.ActionMacieDescribeClassificationJob
	case "ListClassificationJobs":
		return catalog.ActionMacieListClassificationJobs
	case "ListFindings":
		return catalog.ActionMacieListFindings
	case "GetFindings":
		return catalog.ActionMacieGetFindings
	case "InjectFindings":
		return catalog.ActionMacieInjectFindings
	default:
		return action
	}
}

func (s *Server) macieEnable(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionMacieEnableMacie, "*") {
		s.writeMacieError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform macie2:EnableMacie.", readOnly, eventID, verified)
		return
	}
	if _, err := s.store.EnableMacie(verified.AccountID); err != nil {
		s.writeMacieError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to enable Macie.", readOnly, eventID, verified)
		return
	}
	payload, _ := maciesvc.EnableMacieJSON()
	s.writeMacieOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, macieEventSource, "EnableMacie", false)
}

func (s *Server) macieGetSession(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionMacieGetMacieSession, "*") {
		s.writeMacieError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform macie2:GetMacieSession.", readOnly, eventID, verified)
		return
	}
	sess, err := s.store.GetMacieSession(verified.AccountID)
	if errors.Is(err, store.ErrMacieNotFound) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Macie is not enabled.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMacieError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to get Macie session.", readOnly, eventID, verified)
		return
	}
	payload, _ := maciesvc.GetMacieSessionJSON(sess)
	s.writeMacieOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, macieEventSource, "GetMacieSession", true)
}

func (s *Server) macieCreateJob(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionMacieCreateClassificationJob, "*") {
		s.writeMacieError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform macie2:CreateClassificationJob.", readOnly, eventID, verified)
		return
	}
	name, _ := params["name"].(string)
	if name == "" {
		name, _ = params["Name"].(string)
	}
	jobType, _ := params["jobType"].(string)
	if jobType == "" {
		jobType, _ = params["JobType"].(string)
	}
	var def map[string]any
	if v, ok := params["s3JobDefinition"].(map[string]any); ok {
		def = v
	} else if v, ok := params["S3JobDefinition"].(map[string]any); ok {
		def = v
	}
	region := verified.Region
	job, err := s.store.CreateMacieClassificationJob(verified.AccountID, region, name, jobType, def)
	if errors.Is(err, store.ErrMacieNotFound) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Macie is not enabled.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrMacieBadRequest) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMacieError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create classification job.", readOnly, eventID, verified)
		return
	}
	payload, _ := maciesvc.CreateClassificationJobJSON(job)
	s.writeMacieOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, macieEventSource, "CreateClassificationJob", false,
		WithAuditRequestParameters(map[string]any{"jobId": job.JobID, "name": job.Name}))
}

func (s *Server) macieDescribeJob(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionMacieDescribeClassificationJob, "*") {
		s.writeMacieError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform macie2:DescribeClassificationJob.", readOnly, eventID, verified)
		return
	}
	jobID, _ := params["jobId"].(string)
	if jobID == "" {
		jobID, _ = params["JobId"].(string)
	}
	job, err := s.store.DescribeMacieClassificationJob(verified.AccountID, jobID)
	if errors.Is(err, store.ErrMacieNotFound) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Job not found or Macie is not enabled.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMacieError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to describe classification job.", readOnly, eventID, verified)
		return
	}
	payload, _ := maciesvc.DescribeClassificationJobJSON(job)
	s.writeMacieOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, macieEventSource, "DescribeClassificationJob", true)
}

func (s *Server) macieListJobs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionMacieListClassificationJobs, "*") {
		s.writeMacieError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform macie2:ListClassificationJobs.", readOnly, eventID, verified)
		return
	}
	max := intFromParam(params, "maxResults", 50)
	if max == 50 {
		max = intFromParam(params, "MaxResults", 50)
	}
	jobs, err := s.store.ListMacieClassificationJobs(verified.AccountID, max)
	if errors.Is(err, store.ErrMacieNotFound) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Macie is not enabled.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMacieError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to list classification jobs.", readOnly, eventID, verified)
		return
	}
	payload, _ := maciesvc.ListClassificationJobsJSON(jobs)
	s.writeMacieOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, macieEventSource, "ListClassificationJobs", true)
}

func (s *Server) macieListFindings(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionMacieListFindings, "*") {
		s.writeMacieError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform macie2:ListFindings.", readOnly, eventID, verified)
		return
	}
	max := intFromParam(params, "maxResults", 50)
	if max == 50 {
		max = intFromParam(params, "MaxResults", 50)
	}
	ids, err := s.store.ListMacieFindingIDs(verified.AccountID, max)
	if errors.Is(err, store.ErrMacieNotFound) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Macie is not enabled.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMacieError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to list findings.", readOnly, eventID, verified)
		return
	}
	payload, _ := maciesvc.ListFindingsJSON(ids)
	s.writeMacieOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, macieEventSource, "ListFindings", true)
}

func (s *Server) macieGetFindings(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionMacieGetFindings, "*") {
		s.writeMacieError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform macie2:GetFindings.", readOnly, eventID, verified)
		return
	}
	rawIDs, _ := params["findingIds"].([]any)
	if rawIDs == nil {
		rawIDs, _ = params["FindingIds"].([]any)
	}
	ids := make([]string, 0, len(rawIDs))
	for _, v := range rawIDs {
		if s, ok := v.(string); ok && s != "" {
			ids = append(ids, s)
		}
	}
	findings, err := s.store.GetMacieFindings(verified.AccountID, ids)
	if errors.Is(err, store.ErrMacieNotFound) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Macie is not enabled.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrMacieBadRequest) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMacieError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to get findings.", readOnly, eventID, verified)
		return
	}
	payload, _ := maciesvc.GetFindingsJSON(findings)
	s.writeMacieOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, macieEventSource, "GetFindings", true)
}

func (s *Server) macieInjectFindings(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.cfg.MacieInject {
		s.writeMacieError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"macie2:InjectFindings is disabled. Set NOCTAXRIS_MACIE_INJECT=1.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionMacieInjectFindings, "*") {
		s.writeMacieError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform macie2:InjectFindings.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	var ids []string
	var err error

	if rawObjs, ok := params["S3Objects"].([]any); ok && len(rawObjs) > 0 {
		objects := make([]store.MacieS3ObjectRef, 0, len(rawObjs))
		for _, item := range rawObjs {
			m, ok := item.(map[string]any)
			if !ok {
				s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"S3Objects entries must be objects.", readOnly, eventID, verified)
				return
			}
			bucket, _ := m["Bucket"].(string)
			if bucket == "" {
				bucket, _ = m["bucket"].(string)
			}
			key, _ := m["Key"].(string)
			if key == "" {
				key, _ = m["key"].(string)
			}
			objects = append(objects, store.MacieS3ObjectRef{Bucket: bucket, Key: key})
		}
		jobID, _ := params["JobId"].(string)
		if jobID == "" {
			jobID, _ = params["jobId"].(string)
		}
		ids, err = s.store.InjectMacieFindingsFromS3Objects(verified.AccountID, region, jobID, objects)
	} else {
		var findings []store.MacieFinding
		if raw, ok := params["Findings"].([]any); ok {
			for _, item := range raw {
				b, _ := json.Marshal(item)
				var f store.MacieFinding
				if uErr := json.Unmarshal(b, &f); uErr != nil {
					s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						"Invalid Findings entry.", readOnly, eventID, verified)
					return
				}
				findings = append(findings, f)
			}
		} else if raw, ok := params["Finding"].(map[string]any); ok {
			b, _ := json.Marshal(raw)
			var f store.MacieFinding
			if uErr := json.Unmarshal(b, &f); uErr != nil {
				s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"Invalid Finding.", readOnly, eventID, verified)
				return
			}
			findings = append(findings, f)
		}
		ids, err = s.store.InjectMacieFindings(verified.AccountID, region, findings)
	}

	if errors.Is(err, store.ErrMacieNotFound) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Macie is not enabled.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrMacieBadRequest) {
		s.writeMacieError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMacieError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to inject findings.", readOnly, eventID, verified)
		return
	}
	payload, _ := maciesvc.InjectFindingsJSON(ids)
	s.writeMacieOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, macieEventSource, "InjectFindings", false,
		WithAuditRequestParameters(map[string]any{"count": len(ids)}))
}

func (s *Server) writeMacieOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeMacieError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	ak, acct := "", ""
	known := false
	if verified != nil {
		ak, acct, known = verified.AccessKeyID, verified.AccountID, true
	}
	s.writeAPIError(w, r, body, requestID, status, code, message, readOnly, eventID, ak, acct, known)
}
