package server

import (
	"bytes"
	"context"
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
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/google/uuid"
)

const (
	eventVersion    = "1.11"
	healthPath      = "/_noctaxris/health"
	requestIDHeader = "x-amz-request-id"
	maxBodyBytes    = 1 << 20  // 1 MiB
	maxS3BodyBytes  = 16 << 20 // 16 MiB lab PutObject
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

	bodyLimit := bodyLimitForRequest(r)
	body, err := readBody(r, bodyLimit)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "InvalidRequest",
			"Unable to read request body.", readOnly, r, eventID, "", "", false)
		return
	}

	action := resolveAction(r, body)
	var verified *authn.Verified
	if isUnauthenticatedSTSAction(action) {
		// AWS STS federation APIs authenticate via SAML/OIDC token, not SigV4.
		verified = &authn.Verified{Region: federationRegion(r)}
	} else {
		var err error
		verified, err = authn.Verify(r, body, s.now(), sigv4Skew, s.lookupKey)
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
	}

	if action == "" && (verified.Service == "s3" || isS3PathStyleRequest(r, body, action)) {
		s.handleS3(w, r, body, requestID, eventID, verified, readOnly)
		return
	}

	switch action {
	case catalog.ActionSTSGetCallerIdentity, "GetCallerIdentity":
		s.handleGetCallerIdentity(w, r, requestID, eventID, verified)
	case catalog.ActionOrgsCreateAccount, "CreateAccount":
		s.handleCreateAccount(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsDescribeCreateAccountStatus, "DescribeCreateAccountStatus":
		s.handleDescribeCreateAccountStatus(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRole, "AssumeRole":
		s.handleAssumeRole(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetSessionToken, "GetSessionToken":
		s.handleGetSessionToken(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetFederationToken, "GetFederationToken":
		s.handleGetFederationToken(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetAccessKeyInfo, "GetAccessKeyInfo":
		s.handleGetAccessKeyInfo(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSDecodeAuthorizationMessage, "DecodeAuthorizationMessage":
		s.handleDecodeAuthorizationMessage(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRoot, "AssumeRoot":
		s.handleAssumeRoot(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRoleWithSAML, "AssumeRoleWithSAML":
		s.handleAssumeRoleWithSAML(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRoleWithWebIdentity, "AssumeRoleWithWebIdentity":
		s.handleAssumeRoleWithWebIdentity(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetDelegatedAccessToken, "GetDelegatedAccessToken":
		s.handleGetDelegatedAccessToken(w, r, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetWebIdentityToken, "GetWebIdentityToken":
		s.handleGetWebIdentityToken(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionIAMCreateUser, "CreateUser",
		catalog.ActionIAMGetUser, "GetUser",
		catalog.ActionIAMListUsers, "ListUsers",
		catalog.ActionIAMDeleteUser, "DeleteUser",
		catalog.ActionIAMCreateAccessKey, "CreateAccessKey",
		catalog.ActionIAMDeleteAccessKey, "DeleteAccessKey",
		catalog.ActionIAMListAccessKeys, "ListAccessKeys",
		catalog.ActionIAMUpdateAccessKey, "UpdateAccessKey",
		catalog.ActionIAMCreatePolicy, "CreatePolicy",
		catalog.ActionIAMGetPolicy, "GetPolicy",
		catalog.ActionIAMListPolicies, "ListPolicies",
		catalog.ActionIAMDeletePolicy, "DeletePolicy",
		catalog.ActionIAMAttachUserPolicy, "AttachUserPolicy",
		catalog.ActionIAMDetachUserPolicy, "DetachUserPolicy",
		catalog.ActionIAMAttachRolePolicy, "AttachRolePolicy",
		catalog.ActionIAMDetachRolePolicy, "DetachRolePolicy",
		catalog.ActionIAMListAttachedUserPolicies, "ListAttachedUserPolicies",
		catalog.ActionIAMListAttachedRolePolicies, "ListAttachedRolePolicies",
		catalog.ActionIAMPutUserPolicy, "PutUserPolicy",
		catalog.ActionIAMGetUserPolicy, "GetUserPolicy",
		catalog.ActionIAMDeleteUserPolicy, "DeleteUserPolicy",
		catalog.ActionIAMListUserPolicies, "ListUserPolicies",
		catalog.ActionIAMPutRolePolicy, "PutRolePolicy",
		catalog.ActionIAMGetRolePolicy, "GetRolePolicy",
		catalog.ActionIAMDeleteRolePolicy, "DeleteRolePolicy",
		catalog.ActionIAMListRolePolicies, "ListRolePolicies",
		catalog.ActionIAMCreateRole, "CreateRole",
		catalog.ActionIAMGetRole, "GetRole",
		catalog.ActionIAMListRoles, "ListRoles",
		catalog.ActionIAMDeleteRole, "DeleteRole",
		catalog.ActionIAMUpdateAssumeRolePolicy, "UpdateAssumeRolePolicy":
		s.handleIAM(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionKMSCreateKey, "CreateKey",
		catalog.ActionKMSDescribeKey, "DescribeKey",
		catalog.ActionKMSListKeys, "ListKeys",
		catalog.ActionKMSEnableKey, "EnableKey",
		catalog.ActionKMSDisableKey, "DisableKey",
		catalog.ActionKMSGetKeyPolicy, "GetKeyPolicy",
		catalog.ActionKMSPutKeyPolicy, "PutKeyPolicy",
		catalog.ActionKMSEncrypt, "Encrypt",
		catalog.ActionKMSDecrypt, "Decrypt",
		catalog.ActionKMSGenerateDataKey, "GenerateDataKey",
		catalog.ActionKMSGenerateDataKeyWithoutPlaintext, "GenerateDataKeyWithoutPlaintext",
		catalog.ActionKMSCreateGrant, "CreateGrant",
		catalog.ActionKMSListGrants, "ListGrants",
		catalog.ActionKMSRetireGrant, "RetireGrant",
		catalog.ActionKMSRevokeGrant, "RevokeGrant",
		catalog.ActionKMSCreateAlias, "CreateAlias",
		catalog.ActionKMSListAliases, "ListAliases",
		catalog.ActionKMSDeleteAlias, "DeleteAlias",
		catalog.ActionKMSUpdateAlias, "UpdateAlias":
		s.handleKMS(w, r, body, requestID, eventID, action, verified, readOnly)
	default:
		s.writeAWSError(w, requestID, http.StatusNotImplemented, "NotImplemented",
			"This API action is not implemented in Noctaxris Phase 5.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
	}
}

func bodyLimitForRequest(r *http.Request) int64 {
	if isLikelyNonS3Protocol(r) {
		return maxBodyBytes
	}
	return maxS3BodyBytes
}

func isLikelyNonS3Protocol(r *http.Request) bool {
	if r.Header.Get("X-Amz-Target") != "" {
		return true
	}
	if r.URL.Query().Get("Action") != "" {
		return true
	}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/x-amz-json") {
		return true
	}
	if strings.Contains(ct, "application/x-www-form-urlencoded") {
		return true
	}
	return false
}

// isS3PathStyleRequest detects path-style S3 when Action/X-Amz-Target are absent.
func isS3PathStyleRequest(r *http.Request, body []byte, action string) bool {
	if action != "" {
		return false
	}
	if r.Header.Get("X-Amz-Target") != "" {
		return false
	}
	if r.URL.Query().Get("Action") != "" {
		return false
	}
	if len(body) > 0 {
		vals, err := url.ParseQuery(string(body))
		if err == nil && vals.Get("Action") != "" {
			return false
		}
	}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/x-amz-json") {
		return false
	}
	return true
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
	switch action {
	case "GetCallerIdentity":
		return catalog.ActionSTSGetCallerIdentity
	case "AssumeRole":
		return catalog.ActionSTSAssumeRole
	case "GetSessionToken":
		return catalog.ActionSTSGetSessionToken
	case "GetFederationToken":
		return catalog.ActionSTSGetFederationToken
	case "AssumeRoleWithSAML":
		return catalog.ActionSTSAssumeRoleWithSAML
	case "AssumeRoleWithWebIdentity":
		return catalog.ActionSTSAssumeRoleWithWebIdentity
	case "AssumeRoot":
		return catalog.ActionSTSAssumeRoot
	case "DecodeAuthorizationMessage":
		return catalog.ActionSTSDecodeAuthorizationMessage
	case "GetAccessKeyInfo":
		return catalog.ActionSTSGetAccessKeyInfo
	case "GetDelegatedAccessToken":
		return catalog.ActionSTSGetDelegatedAccessToken
	case "GetWebIdentityToken":
		return catalog.ActionSTSGetWebIdentityToken
	case "CreateAccount":
		return catalog.ActionOrgsCreateAccount
	case "DescribeCreateAccountStatus":
		return catalog.ActionOrgsDescribeCreateAccountStatus
	case "CreateUser":
		return catalog.ActionIAMCreateUser
	case "GetUser":
		return catalog.ActionIAMGetUser
	case "ListUsers":
		return catalog.ActionIAMListUsers
	case "DeleteUser":
		return catalog.ActionIAMDeleteUser
	case "CreateAccessKey":
		return catalog.ActionIAMCreateAccessKey
	case "DeleteAccessKey":
		return catalog.ActionIAMDeleteAccessKey
	case "ListAccessKeys":
		return catalog.ActionIAMListAccessKeys
	case "UpdateAccessKey":
		return catalog.ActionIAMUpdateAccessKey
	case "CreatePolicy":
		return catalog.ActionIAMCreatePolicy
	case "GetPolicy":
		return catalog.ActionIAMGetPolicy
	case "ListPolicies":
		return catalog.ActionIAMListPolicies
	case "DeletePolicy":
		return catalog.ActionIAMDeletePolicy
	case "AttachUserPolicy":
		return catalog.ActionIAMAttachUserPolicy
	case "DetachUserPolicy":
		return catalog.ActionIAMDetachUserPolicy
	case "AttachRolePolicy":
		return catalog.ActionIAMAttachRolePolicy
	case "DetachRolePolicy":
		return catalog.ActionIAMDetachRolePolicy
	case "ListAttachedUserPolicies":
		return catalog.ActionIAMListAttachedUserPolicies
	case "ListAttachedRolePolicies":
		return catalog.ActionIAMListAttachedRolePolicies
	case "PutUserPolicy":
		return catalog.ActionIAMPutUserPolicy
	case "GetUserPolicy":
		return catalog.ActionIAMGetUserPolicy
	case "DeleteUserPolicy":
		return catalog.ActionIAMDeleteUserPolicy
	case "ListUserPolicies":
		return catalog.ActionIAMListUserPolicies
	case "PutRolePolicy":
		return catalog.ActionIAMPutRolePolicy
	case "GetRolePolicy":
		return catalog.ActionIAMGetRolePolicy
	case "DeleteRolePolicy":
		return catalog.ActionIAMDeleteRolePolicy
	case "ListRolePolicies":
		return catalog.ActionIAMListRolePolicies
	case "CreateRole":
		return catalog.ActionIAMCreateRole
	case "GetRole":
		return catalog.ActionIAMGetRole
	case "ListRoles":
		return catalog.ActionIAMListRoles
	case "DeleteRole":
		return catalog.ActionIAMDeleteRole
	case "UpdateAssumeRolePolicy":
		return catalog.ActionIAMUpdateAssumeRolePolicy
	case "CreateKey":
		return catalog.ActionKMSCreateKey
	case "DescribeKey":
		return catalog.ActionKMSDescribeKey
	case "ListKeys":
		return catalog.ActionKMSListKeys
	case "EnableKey":
		return catalog.ActionKMSEnableKey
	case "DisableKey":
		return catalog.ActionKMSDisableKey
	case "GetKeyPolicy":
		return catalog.ActionKMSGetKeyPolicy
	case "PutKeyPolicy":
		return catalog.ActionKMSPutKeyPolicy
	case "Encrypt":
		return catalog.ActionKMSEncrypt
	case "Decrypt":
		return catalog.ActionKMSDecrypt
	case "GenerateDataKey":
		return catalog.ActionKMSGenerateDataKey
	case "GenerateDataKeyWithoutPlaintext":
		return catalog.ActionKMSGenerateDataKeyWithoutPlaintext
	case "CreateGrant":
		return catalog.ActionKMSCreateGrant
	case "ListGrants":
		return catalog.ActionKMSListGrants
	case "RetireGrant":
		return catalog.ActionKMSRetireGrant
	case "RevokeGrant":
		return catalog.ActionKMSRevokeGrant
	case "CreateAlias":
		return catalog.ActionKMSCreateAlias
	case "ListAliases":
		return catalog.ActionKMSListAliases
	case "DeleteAlias":
		return catalog.ActionKMSDeleteAlias
	case "UpdateAlias":
		return catalog.ActionKMSUpdateAlias
	default:
		return action
	}
}

func isUnauthenticatedSTSAction(action string) bool {
	switch action {
	case catalog.ActionSTSAssumeRoleWithSAML, "AssumeRoleWithSAML",
		catalog.ActionSTSAssumeRoleWithWebIdentity, "AssumeRoleWithWebIdentity":
		return true
	default:
		return false
	}
}

func federationRegion(r *http.Request) string {
	_ = r
	return "us-east-1"
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
	return uuid.NewString()
}
