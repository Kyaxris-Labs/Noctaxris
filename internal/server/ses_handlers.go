package server

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	sessvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ses"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const sesEventSource = "ses.amazonaws.com"

func (s *Server) handleSES(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	action = sesAction(action)

	switch action {
	case catalog.ActionSESVerifyEmailIdentity:
		s.sesVerifyEmailIdentity(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSESSendEmail:
		s.sesSendEmail(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSESSendRawEmail:
		s.sesSendRawEmail(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSESListIdentities:
		s.sesListIdentities(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSESGetSendStatistics:
		s.sesGetSendStatistics(w, r, requestID, eventID, verified, readOnly)
	case catalog.ActionSESSetIdentityNotificationTopic:
		s.sesSetIdentityNotificationTopic(w, r, requestID, eventID, verified, readOnly, params)
	default:
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "InvalidAction",
			"This SES action is not implemented.", readOnly, eventID, verified)
	}
}

func sesAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "VerifyEmailIdentity":
		return catalog.ActionSESVerifyEmailIdentity
	case "SendEmail":
		return catalog.ActionSESSendEmail
	case "SendRawEmail":
		return catalog.ActionSESSendRawEmail
	case "ListIdentities":
		return catalog.ActionSESListIdentities
	case "GetSendStatistics":
		return catalog.ActionSESGetSendStatistics
	case "SetIdentityNotificationTopic":
		return catalog.ActionSESSetIdentityNotificationTopic
	default:
		return action
	}
}

func (s *Server) sesVerifyEmailIdentity(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	email := strings.TrimSpace(params.Get("EmailAddress"))
	if email == "" {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			"EmailAddress is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionSESVerifyEmailIdentity, "*") {
		s.writeSESError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform ses:VerifyEmailIdentity.", readOnly, eventID, verified)
		return
	}
	if err := s.store.VerifySESEmailIdentity(verified.AccountID, email); err != nil {
		s.writeSESError(w, r, requestID, http.StatusInternalServerError, "ServiceUnavailable",
			"Unable to verify identity.", readOnly, eventID, verified)
		return
	}
	payload, _ := sessvc.VerifyEmailIdentityXML(requestID)
	s.writeSESOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "VerifyEmailIdentity", readOnly)
}

func (s *Server) sesSendEmail(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	source := strings.TrimSpace(params.Get("Source"))
	if source == "" {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			"Source is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionSESSendEmail, "*") {
		s.writeSESError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform ses:SendEmail.", readOnly, eventID, verified)
		return
	}
	dest := collectSESDestinations(params)
	subject := params.Get("Message.Subject.Data")
	bodyText := params.Get("Message.Body.Text.Data")
	bodyHTML := params.Get("Message.Body.Html.Data")
	msgID, err := s.store.SendSESEmail(verified.AccountID, source, dest, subject, bodyText, bodyHTML)
	if errors.Is(err, store.ErrSESIdentityNotFound) {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "MessageRejected",
			"Email address is not verified.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := sessvc.SendEmailXML(msgID, requestID)
	s.writeSESOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "SendEmail", readOnly)
}

func (s *Server) sesSendRawEmail(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	source := strings.TrimSpace(params.Get("Source"))
	if source == "" {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			"Source is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionSESSendRawEmail, "*") {
		s.writeSESError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform ses:SendRawEmail.", readOnly, eventID, verified)
		return
	}
	dest := collectSESDestinations(params)
	raw := params.Get("RawMessage.Data")
	if raw == "" {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			"RawMessage.Data is required.", readOnly, eventID, verified)
		return
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil {
		raw = string(decoded)
	}
	msgID, err := s.store.SendSESRawEmail(verified.AccountID, source, dest, raw)
	if errors.Is(err, store.ErrSESIdentityNotFound) {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "MessageRejected",
			"Email address is not verified.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := sessvc.SendRawEmailXML(msgID, requestID)
	s.writeSESOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "SendRawEmail", readOnly)
}

func (s *Server) sesListIdentities(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionSESListIdentities, "*") {
		s.writeSESError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform ses:ListIdentities.", readOnly, eventID, verified)
		return
	}
	ids, err := s.store.ListSESIdentities(verified.AccountID, params.Get("IdentityType"))
	if err != nil {
		s.writeSESError(w, r, requestID, http.StatusInternalServerError, "ServiceUnavailable",
			"Unable to list identities.", readOnly, eventID, verified)
		return
	}
	payload, _ := sessvc.ListIdentitiesXML(ids, requestID)
	s.writeSESOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "ListIdentities", readOnly)
}

func (s *Server) sesGetSendStatistics(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionSESGetSendStatistics, "*") {
		s.writeSESError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform ses:GetSendStatistics.", readOnly, eventID, verified)
		return
	}
	stats, err := s.store.GetSESSendStatistics(verified.AccountID)
	if err != nil {
		s.writeSESError(w, r, requestID, http.StatusInternalServerError, "ServiceUnavailable",
			"Unable to get send statistics.", readOnly, eventID, verified)
		return
	}
	payload, _ := sessvc.GetSendStatisticsXML(stats, requestID)
	s.writeSESOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "GetSendStatistics", readOnly)
}

func (s *Server) sesSetIdentityNotificationTopic(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	identity := strings.TrimSpace(params.Get("Identity"))
	notifType := strings.TrimSpace(params.Get("NotificationType"))
	topicARN := strings.TrimSpace(params.Get("SnsTopic"))
	if identity == "" || notifType == "" {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			"Identity and NotificationType are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionSESSetIdentityNotificationTopic, "*") {
		s.writeSESError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform ses:SetIdentityNotificationTopic.", readOnly, eventID, verified)
		return
	}
	err := s.store.SetSESIdentityNotificationTopic(verified.AccountID, identity, notifType, topicARN)
	if errors.Is(err, store.ErrSESIdentityNotFound) {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "MessageRejected",
			"Email address is not verified.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSESError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := sessvc.SetIdentityNotificationTopicXML(requestID)
	s.writeSESOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "SetIdentityNotificationTopic", readOnly)
}

func collectSESDestinations(params url.Values) []string {
	var dest []string
	for i := 1; i <= 50; i++ {
		key := "Destination.ToAddresses.member." + strconv.Itoa(i)
		v := strings.TrimSpace(params.Get(key))
		if v == "" {
			break
		}
		dest = append(dest, v)
	}
	return dest
}

func (s *Server) writeSESOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeSESError(
	w http.ResponseWriter, r *http.Request, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	payload, _ := sessvc.ErrorXML(code, message, requestID)
	_, _ = w.Write(payload)
	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}
