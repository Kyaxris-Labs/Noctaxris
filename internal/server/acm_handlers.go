package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	acmsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/acm"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	acmJSONContentType = "application/x-amz-json-1.1"
	acmEventSource     = "acm.amazonaws.com"
)

func (s *Server) handleACM(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = acmAction(action)

	switch action {
	case catalog.ActionACMRequestCertificate:
		s.acmRequestCertificate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionACMDescribeCertificate:
		s.acmDescribeCertificate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionACMListCertificates:
		s.acmListCertificates(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionACMDeleteCertificate:
		s.acmDeleteCertificate(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeACMError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This ACM action is not implemented.", readOnly, eventID, verified)
	}
}

func acmAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "RequestCertificate":
		return catalog.ActionACMRequestCertificate
	case "DescribeCertificate":
		return catalog.ActionACMDescribeCertificate
	case "ListCertificates":
		return catalog.ActionACMListCertificates
	case "DeleteCertificate":
		return catalog.ActionACMDeleteCertificate
	default:
		return action
	}
}

func (s *Server) acmRequestCertificate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	domain, _ := params["DomainName"].(string)
	if !s.authorize(verified, catalog.ActionACMRequestCertificate, "*") {
		s.writeACMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform acm:RequestCertificate.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultACMRegion
	}
	cert, err := s.store.RequestACMCertificate(verified.AccountID, region, domain)
	if errors.Is(err, store.ErrACMBadRequest) {
		s.writeACMError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeACMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to request certificate.", readOnly, eventID, verified)
		return
	}
	payload, _ := acmsvc.RequestCertificateJSON(cert)
	s.writeACMOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, acmEventSource, "RequestCertificate", readOnly)
}

func (s *Server) acmDescribeCertificate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["CertificateArn"].(string)
	if !s.authorize(verified, catalog.ActionACMDescribeCertificate, arn) {
		s.writeACMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform acm:DescribeCertificate.", readOnly, eventID, verified)
		return
	}
	cert, err := s.store.DescribeACMCertificate(verified.AccountID, arn)
	if errors.Is(err, store.ErrACMNotFound) {
		s.writeACMError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Certificate not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeACMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe certificate.", readOnly, eventID, verified)
		return
	}
	payload, _ := acmsvc.DescribeCertificateJSON(cert)
	s.writeACMOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, acmEventSource, "DescribeCertificate", readOnly)
}

func (s *Server) acmListCertificates(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionACMListCertificates, "*") {
		s.writeACMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform acm:ListCertificates.", readOnly, eventID, verified)
		return
	}
	certs, err := s.store.ListACMCertificates(verified.AccountID)
	if err != nil {
		s.writeACMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list certificates.", readOnly, eventID, verified)
		return
	}
	payload, _ := acmsvc.ListCertificatesJSON(certs)
	s.writeACMOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, acmEventSource, "ListCertificates", readOnly)
}

func (s *Server) acmDeleteCertificate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["CertificateArn"].(string)
	if !s.authorize(verified, catalog.ActionACMDeleteCertificate, arn) {
		s.writeACMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform acm:DeleteCertificate.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteACMCertificate(verified.AccountID, arn)
	if errors.Is(err, store.ErrACMNotFound) {
		s.writeACMError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Certificate not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeACMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete certificate.", readOnly, eventID, verified)
		return
	}
	payload, _ := acmsvc.DeleteCertificateJSON()
	s.writeACMOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, acmEventSource, "DeleteCertificate", readOnly)
}

func (s *Server) writeACMOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", acmJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeACMError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", acmJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, acmEventSource, code, readOnly)
}
