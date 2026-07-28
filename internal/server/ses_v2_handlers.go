package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	sessvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ses"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const sesV2JSONContentType = "application/json"

func isSESV2RESTPath(path string) bool {
	return strings.HasPrefix(path, "/v2/email")
}

func resolveSESV2REST(r *http.Request) (action, identity string) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if !strings.HasPrefix(path, "/v2/email") {
		return "", ""
	}
	rest := strings.TrimPrefix(path, "/v2/email")
	switch r.Method {
	case http.MethodPost:
		switch rest {
		case "/identities":
			return catalog.ActionSESCreateEmailIdentity, ""
		case "/outbound-emails":
			return catalog.ActionSESSendEmail, ""
		}
	case http.MethodGet:
		switch rest {
		case "/identities":
			return catalog.ActionSESListEmailIdentities, ""
		case "/account":
			return catalog.ActionSESGetAccount, ""
		}
		if strings.HasPrefix(rest, "/identities/") {
			raw := strings.TrimPrefix(rest, "/identities/")
			if raw == "" || strings.Contains(raw, "/") {
				return "", ""
			}
			id, err := url.PathUnescape(raw)
			if err != nil || id == "" {
				return "", ""
			}
			return catalog.ActionSESGetEmailIdentity, id
		}
	case http.MethodDelete:
		if strings.HasPrefix(rest, "/identities/") {
			raw := strings.TrimPrefix(rest, "/identities/")
			if raw == "" || strings.Contains(raw, "/") {
				return "", ""
			}
			id, err := url.PathUnescape(raw)
			if err != nil || id == "" {
				return "", ""
			}
			return catalog.ActionSESDeleteEmailIdentity, id
		}
	}
	return "", ""
}

func (s *Server) handleSESV2(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
	identityFromPath string,
) {
	switch action {
	case catalog.ActionSESCreateEmailIdentity:
		s.sesV2CreateEmailIdentity(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSESListEmailIdentities:
		s.sesV2ListEmailIdentities(w, r, requestID, eventID, verified, readOnly)
	case catalog.ActionSESGetEmailIdentity:
		s.sesV2GetEmailIdentity(w, r, requestID, eventID, verified, readOnly, identityFromPath)
	case catalog.ActionSESDeleteEmailIdentity:
		s.sesV2DeleteEmailIdentity(w, r, requestID, eventID, verified, readOnly, identityFromPath)
	case catalog.ActionSESSendEmail:
		s.sesV2SendEmail(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSESGetAccount:
		s.sesV2GetAccount(w, r, requestID, eventID, verified, readOnly)
	default:
		s.writeSESV2Error(w, requestID, http.StatusNotFound, "NotFoundException", "Unknown operation.")
	}
}

func (s *Server) sesV2CreateEmailIdentity(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionSESCreateEmailIdentity, "*") {
		s.writeSESV2Error(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ses:CreateEmailIdentity.")
		return
	}
	params := jsonBodyMap(body)
	emailIdentity, _ := params["EmailIdentity"].(string)
	emailIdentity = strings.TrimSpace(emailIdentity)
	if emailIdentity == "" {
		s.writeSESV2Error(w, requestID, http.StatusBadRequest, "BadRequestException",
			"EmailIdentity is required.")
		return
	}
	id, err := s.store.CreateSESIdentityV2(verified.AccountID, emailIdentity)
	if errors.Is(err, store.ErrSESIdentityAlreadyExists) {
		s.writeSESV2Error(w, requestID, http.StatusBadRequest, "AlreadyExistsException",
			"Email identity "+emailIdentity+" already exist.")
		return
	}
	if err != nil {
		s.writeSESV2Error(w, requestID, http.StatusBadRequest, "BadRequestException", err.Error())
		return
	}
	payload, _ := sessvc.CreateEmailIdentityJSON(id)
	s.writeSESV2OK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "CreateEmailIdentity", readOnly)
}

func (s *Server) sesV2ListEmailIdentities(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionSESListEmailIdentities, "*") {
		s.writeSESV2Error(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ses:ListEmailIdentities.")
		return
	}
	ids, err := s.store.ListSESIdentities(verified.AccountID, "")
	if err != nil {
		s.writeSESV2Error(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list identities.")
		return
	}
	payload, _ := sessvc.ListEmailIdentitiesJSON(ids)
	s.writeSESV2OK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "ListEmailIdentities", readOnly)
}

func (s *Server) sesV2GetEmailIdentity(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, identity string,
) {
	if !s.authorize(verified, catalog.ActionSESGetEmailIdentity, "*") {
		s.writeSESV2Error(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ses:GetEmailIdentity.")
		return
	}
	id, err := s.store.GetSESIdentity(verified.AccountID, identity)
	if errors.Is(err, store.ErrSESIdentityNotFoundV2) {
		s.writeSESV2Error(w, requestID, http.StatusNotFound, "NotFoundException",
			"Identity "+identity+" does not exist.")
		return
	}
	if err != nil {
		s.writeSESV2Error(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get identity.")
		return
	}
	payload, _ := sessvc.GetEmailIdentityJSON(id)
	s.writeSESV2OK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "GetEmailIdentity", readOnly)
}

func (s *Server) sesV2DeleteEmailIdentity(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, identity string,
) {
	if !s.authorize(verified, catalog.ActionSESDeleteEmailIdentity, "*") {
		s.writeSESV2Error(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ses:DeleteEmailIdentity.")
		return
	}
	err := s.store.DeleteSESIdentity(verified.AccountID, identity)
	if errors.Is(err, store.ErrSESIdentityNotFoundV2) {
		s.writeSESV2Error(w, requestID, http.StatusNotFound, "NotFoundException",
			"Email identity "+identity+" does not exist.")
		return
	}
	if err != nil {
		s.writeSESV2Error(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete identity.")
		return
	}
	s.writeSESV2OK(w, requestID, sessvc.EmptyJSON())
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "DeleteEmailIdentity", readOnly)
}

func (s *Server) sesV2SendEmail(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionSESSendEmail, "*") {
		s.writeSESV2Error(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ses:SendEmail.")
		return
	}
	params := jsonBodyMap(body)
	from, _ := params["FromEmailAddress"].(string)
	from = strings.TrimSpace(strings.ToLower(from))

	dest := sesV2Destinations(params)
	content, _ := params["Content"].(map[string]any)
	if content == nil {
		s.writeSESV2Error(w, requestID, http.StatusBadRequest, "BadRequestException",
			"Content must contain Raw, Simple, or Template.")
		return
	}

	var msgID string
	var err error

	if raw, ok := content["Raw"].(map[string]any); ok {
		rawData, _ := raw["Data"].(string)
		if strings.TrimSpace(rawData) == "" {
			s.writeSESV2Error(w, requestID, http.StatusBadRequest, "BadRequestException",
				"Content.Raw.Data is required.")
			return
		}
		if len(dest) == 0 {
			s.writeSESV2Error(w, requestID, http.StatusBadRequest, "BadRequestException",
				"At least one destination address is required.")
			return
		}
		if from == "" {
			s.writeSESV2Error(w, requestID, http.StatusBadRequest, "BadRequestException",
				"FromEmailAddress is required.")
			return
		}
		msgID, err = s.store.SendSESRawEmail(verified.AccountID, from, dest, rawData)
	} else if simple, ok := content["Simple"].(map[string]any); ok {
		if from == "" {
			s.writeSESV2Error(w, requestID, http.StatusBadRequest, "BadRequestException",
				"FromEmailAddress is required.")
			return
		}
		subject := nestedString(simple, "Subject", "Data")
		bodyText := nestedString(simple, "Body", "Text", "Data")
		bodyHTML := nestedString(simple, "Body", "Html", "Data")
		msgID, err = s.store.SendSESEmail(verified.AccountID, from, dest, subject, bodyText, bodyHTML)
	} else {
		s.writeSESV2Error(w, requestID, http.StatusBadRequest, "BadRequestException",
			"Content must contain Raw, Simple, or Template.")
		return
	}

	if errors.Is(err, store.ErrSESIdentityNotFound) {
		s.writeSESV2Error(w, requestID, http.StatusBadRequest, "MessageRejected",
			"Email address is not verified.")
		return
	}
	if err != nil {
		s.writeSESV2Error(w, requestID, http.StatusBadRequest, "BadRequestException", err.Error())
		return
	}
	payload, _ := sessvc.SendEmailV2JSON(msgID)
	s.writeSESV2OK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "SendEmail", readOnly)
}

func (s *Server) sesV2GetAccount(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionSESGetAccount, "*") {
		s.writeSESV2Error(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ses:GetAccount.")
		return
	}
	n, err := s.store.CountSESMessages(verified.AccountID)
	if err != nil {
		s.writeSESV2Error(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get account.")
		return
	}
	payload, _ := sessvc.GetAccountJSON(n)
	s.writeSESV2OK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sesEventSource, "GetAccount", readOnly)
}

func sesV2Destinations(params map[string]any) []string {
	destNode, _ := params["Destination"].(map[string]any)
	if destNode == nil {
		return nil
	}
	var out []string
	for _, key := range []string{"ToAddresses", "CcAddresses", "BccAddresses"} {
		out = append(out, stringSlice(destNode[key])...)
	}
	return out
}

func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range raw {
		s, _ := item.(string)
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func nestedString(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		node, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = node[k]
	}
	s, _ := cur.(string)
	return s
}

func (s *Server) writeSESV2OK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", sesV2JSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeSESV2Error(w http.ResponseWriter, requestID string, status int, code, message string) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", sesV2JSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{"__type": code, "message": message})
	_, _ = w.Write(payload)
}
