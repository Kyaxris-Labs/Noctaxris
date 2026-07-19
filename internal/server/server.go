package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	eventVersion    = "1.11"
	healthPath      = "/_noctaxris/health"
	requestIDHeader = "x-amz-request-id"
	maxBodyBytes    = 1 << 20 // 1 MiB
	sigv4Skew       = 15 * time.Minute
)

type Server struct {
	cfg   config.Config
	store *store.Store
	audit *audit.Writer
	now   func() time.Time
}

type awsError struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
	Type    string   `xml:"Type"`
}

type awsErrorResponse struct {
	XMLName   xml.Name `xml:"ErrorResponse"`
	XMLNS     string   `xml:"xmlns,attr"`
	Error     awsError `xml:"Error"`
	RequestID string   `xml:"RequestId"`
}

func New(cfg config.Config, st *store.Store, aud *audit.Writer) *Server {
	return &Server{
		cfg:   cfg,
		store: st,
		audit: aud,
		now:   time.Now,
	}
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:    s.cfg.ListenAddr,
		Handler: s.Handler(),
	}
	if s.cfg.TLSCertFile != "" && s.cfg.TLSKeyFile != "" {
		return srv.ListenAndServeTLS(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
	}
	return srv.ListenAndServe()
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == healthPath {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	requestID := newRequestID()
	eventID := newRequestID()
	readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead

	body, err := readBody(r, maxBodyBytes)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "InvalidRequest",
			"Unable to read request body.", readOnly, r, eventID, "", "", false)
		return
	}

	verified, err := authn.Verify(r, body, s.now(), sigv4Skew, s.lookupKey)
	if err != nil {
		code := authn.Code(err)
		if code == "" {
			code = authn.CodeInvalidClientTokenId
		}
		msg := defaultAuthnMessage(code)
		accessKeyID := ""
		if ak, ok := parseAccessKeyID(r.Header.Get("Authorization")); ok {
			accessKeyID = ak
		}
		s.writeAWSError(w, requestID, http.StatusForbidden, code, msg, readOnly, r, eventID, accessKeyID, "", false)
		return
	}

	action := resolveAction(r, body)
	if action == catalog.ActionSTSGetCallerIdentity || action == "GetCallerIdentity" {
		s.handleGetCallerIdentity(w, r, requestID, eventID, verified)
		return
	}

	s.writeAWSError(w, requestID, http.StatusNotImplemented, "NotImplemented",
		"This API action is not implemented in Noctaxris Phase 1.", readOnly, r, eventID,
		verified.AccessKeyID, verified.AccountID, true)
}

func (s *Server) handleGetCallerIdentity(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
) {
	// AWS STS GetCallerIdentity requires no IAM permissions after authentication.
	userID, arn := sts.RootCallerIDs(verified.AccountID)
	if !verified.Principal.IsRoot {
		userID = verified.AccessKeyID
		arn = verified.Principal.ARN()
		if arn == "" {
			arn = fmt.Sprintf("arn:aws:iam::%s:user/%s", verified.AccountID, verified.AccessKeyID)
		}
	}

	payload, err := sts.GetCallerIdentityXML(verified.AccountID, userID, arn, requestID)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build GetCallerIdentity response.", true, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)

	ev := audit.Event{
		EventVersion:       eventVersion,
		EventTime:          s.now().UTC().Format(time.RFC3339),
		EventSource:        "sts.amazonaws.com",
		EventName:          "GetCallerIdentity",
		AWSRegion:          verified.Region,
		SourceIPAddress:    clientIP(r),
		UserAgent:          r.UserAgent(),
		RequestID:          requestID,
		EventID:            eventID,
		EventType:          "AwsApiCall",
		RecipientAccountID: verified.AccountID,
		ReadOnly:           true,
		UserIdentity: map[string]any{
			"type":        "Root",
			"accountId":   verified.AccountID,
			"accessKeyId": verified.AccessKeyID,
			"arn":         arn,
		},
		RequestParameters: map[string]any{
			"httpMethod": r.Method,
			"path":       r.URL.Path,
		},
	}
	if !verified.Principal.IsRoot {
		ev.UserIdentity = map[string]any{
			"type":        "IAMUser",
			"accountId":   verified.AccountID,
			"accessKeyId": verified.AccessKeyID,
			"arn":         arn,
		}
	}
	_ = s.audit.Write(context.Background(), ev)
}

func (s *Server) lookupKey(accessKeyID string) (accountID, secret string, isRoot bool, err error) {
	return s.store.LookupAccessKey(accessKeyID)
}

func (s *Server) writeAWSError(
	w http.ResponseWriter,
	requestID string,
	status int,
	code string,
	message string,
	readOnly bool,
	r *http.Request,
	eventID string,
	accessKeyID string,
	accountID string,
	knownKey bool,
) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)

	resp := awsErrorResponse{
		XMLNS: "http://noctaxris.amazonaws.com/doc/2026-07-18/",
		Error: awsError{
			Code:    code,
			Message: message,
			Type:    "Sender",
		},
		RequestID: requestID,
	}
	data, err := xml.Marshal(resp)
	if err == nil {
		_, _ = w.Write([]byte(xml.Header))
		_, _ = w.Write(data)
	}

	eventSource := "noctaxris.amazonaws.com"
	eventName := eventNameForRequest(r)
	if strings.Contains(strings.ToLower(eventName), "getcalleridentity") ||
		strings.Contains(r.Header.Get("X-Amz-Target"), "GetCallerIdentity") {
		eventSource = "sts.amazonaws.com"
		eventName = "GetCallerIdentity"
	}

	ev := audit.Event{
		EventVersion:       eventVersion,
		EventTime:          s.now().UTC().Format(time.RFC3339),
		EventSource:        eventSource,
		EventName:          eventName,
		SourceIPAddress:    clientIP(r),
		UserAgent:          r.UserAgent(),
		RequestID:          requestID,
		EventID:            eventID,
		EventType:          "AwsApiCall",
		RecipientAccountID: s.cfg.AccountID,
		ReadOnly:           readOnly,
		ErrorCode:          code,
		ErrorMessage:       message,
		RequestParameters: map[string]any{
			"httpMethod": r.Method,
			"path":       r.URL.Path,
		},
	}

	if knownKey {
		recipient := s.cfg.AccountID
		if accountID != "" {
			recipient = accountID
		}
		ev.RecipientAccountID = recipient
		ev.UserIdentity = map[string]any{
			"type":        "IAMUser",
			"accountId":   recipient,
			"accessKeyId": accessKeyID,
		}
	}

	_ = s.audit.Write(context.Background(), ev)
}

func readBody(r *http.Request, limit int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("body too large")
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func resolveAction(r *http.Request, body []byte) string {
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		if i := strings.LastIndex(target, "."); i >= 0 && i+1 < len(target) {
			return target[i+1:]
		}
		return target
	}
	if v := r.URL.Query().Get("Action"); v != "" {
		return normalizeAction(v)
	}
	if len(body) > 0 {
		vals, err := url.ParseQuery(string(body))
		if err == nil {
			if v := vals.Get("Action"); v != "" {
				return normalizeAction(v)
			}
		}
	}
	return ""
}

func normalizeAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	if action == "GetCallerIdentity" {
		return catalog.ActionSTSGetCallerIdentity
	}
	return action
}

func defaultAuthnMessage(code string) string {
	switch code {
	case authn.CodeMissingAuthenticationToken:
		return "The request must contain a valid signature or access key."
	case authn.CodeInvalidClientTokenId:
		return "The security token included in the request is invalid."
	case authn.CodeSignatureDoesNotMatch:
		return "The request signature we calculated does not match the signature you provided."
	case authn.CodeRequestTimeTooSkewed:
		return "The difference between the request time and the server time is too large."
	default:
		return "The request was rejected because authentication failed."
	}
}

func parseAccessKeyID(authorization string) (string, bool) {
	const prefix = "Credential="
	idx := strings.Index(authorization, prefix)
	if idx < 0 {
		return "", false
	}
	rest := authorization[idx+len(prefix):]
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return "", false
	}
	return rest[:slash], true
}

func eventNameForRequest(r *http.Request) string {
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		if i := strings.LastIndex(target, "."); i >= 0 && i+1 < len(target) {
			return target[i+1:]
		}
		return target
	}
	if v := r.URL.Query().Get("Action"); v != "" {
		return v
	}
	return r.Method + " " + r.URL.Path
}

func clientIP(r *http.Request) string {
	if host, _, ok := strings.Cut(r.RemoteAddr, ":"); ok && host != "" {
		return host
	}
	return r.RemoteAddr
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
