package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	transcribesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/transcribe"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	transcribeJSONContentType = "application/x-amz-json-1.1"
	transcribeEventSource     = "transcribe.amazonaws.com"
)

func (s *Server) handleTranscribe(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = transcribeAction(action)

	switch action {
	case catalog.ActionTranscribeStartTranscriptionJob:
		s.transcribeStart(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTranscribeGetTranscriptionJob:
		s.transcribeGet(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTranscribeListTranscriptionJobs:
		s.transcribeList(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeTranscribeError(w, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Transcribe action is not implemented.")
	}
}

func transcribeAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "StartTranscriptionJob":
		return catalog.ActionTranscribeStartTranscriptionJob
	case "GetTranscriptionJob":
		return catalog.ActionTranscribeGetTranscriptionJob
	case "ListTranscriptionJobs":
		return catalog.ActionTranscribeListTranscriptionJobs
	default:
		return action
	}
}

func (s *Server) transcribeRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultTranscribeRegion
}

func (s *Server) transcribeStart(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionTranscribeStartTranscriptionJob, "*") {
		s.writeTranscribeError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transcribe:StartTranscriptionJob.")
		return
	}
	name, _ := params["TranscriptionJobName"].(string)
	lang, _ := params["LanguageCode"].(string)
	mediaURI := ""
	if media, ok := params["Media"].(map[string]any); ok {
		mediaURI, _ = media["MediaFileUri"].(string)
	}
	job, err := s.store.StartTranscriptionJobStub(verified.AccountID, s.transcribeRegion(verified), name, mediaURI, lang)
	if errors.Is(err, store.ErrTranscribeConflict) {
		s.writeTranscribeError(w, requestID, http.StatusBadRequest, "ConflictException",
			"Transcription job already exists.")
		return
	}
	if errors.Is(err, store.ErrTranscribeBadRequest) {
		s.writeTranscribeError(w, requestID, http.StatusBadRequest, "BadRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeTranscribeError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to start transcription job.")
		return
	}
	payload, _ := transcribesvc.StartTranscriptionJobJSON(job)
	s.writeTranscribeOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transcribeEventSource, "StartTranscriptionJob", readOnly)
}

func (s *Server) transcribeGet(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionTranscribeGetTranscriptionJob, "*") {
		s.writeTranscribeError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transcribe:GetTranscriptionJob.")
		return
	}
	name, _ := params["TranscriptionJobName"].(string)
	job, err := s.store.GetTranscriptionJob(verified.AccountID, name)
	if errors.Is(err, store.ErrTranscribeNotFound) {
		s.writeTranscribeError(w, requestID, http.StatusBadRequest, "NotFoundException",
			"Transcription job not found.")
		return
	}
	if err != nil {
		s.writeTranscribeError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get transcription job.")
		return
	}
	payload, _ := transcribesvc.GetTranscriptionJobJSON(job)
	s.writeTranscribeOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transcribeEventSource, "GetTranscriptionJob", readOnly)
}

func (s *Server) transcribeList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionTranscribeListTranscriptionJobs, "*") {
		s.writeTranscribeError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transcribe:ListTranscriptionJobs.")
		return
	}
	jobs, err := s.store.ListTranscriptionJobs(verified.AccountID)
	if err != nil {
		s.writeTranscribeError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list transcription jobs.")
		return
	}
	payload, _ := transcribesvc.ListTranscriptionJobsJSON(jobs)
	s.writeTranscribeOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transcribeEventSource, "ListTranscriptionJobs", readOnly)
}

func (s *Server) writeTranscribeOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", transcribeJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeTranscribeError(w http.ResponseWriter, requestID string, status int, code, message string) {
	w.Header().Set("Content-Type", transcribeJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + message + `"}`))
}
