package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/Kyaxris-Labs/Noctaxris/internal/services/organizations"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const defaultAssumeRoleDuration = time.Hour

func (s *Server) lookupKey(accessKeyID string) (authn.ResolvedKey, error) {
	ak, err := s.store.LookupAccessKeyRecord(accessKeyID)
	if err != nil {
		return authn.ResolvedKey{}, err
	}
	return authn.ResolvedKey{
		AccountID:    ak.AccountID,
		Secret:       ak.Secret,
		IsRoot:       ak.IsRoot,
		SessionToken: ak.SessionToken,
		RoleARN:      ak.RoleARN,
		SessionName:  ak.SessionName,
		ExpiresAt:    ak.ExpiresAt,
	}, nil
}

func (s *Server) handleGetCallerIdentity(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
) {
	userID, arn := sts.RootCallerIDs(verified.AccountID)
	identityType := "Root"
	if verified.Principal.Kind == identity.KindRole {
		arn = verified.Principal.ARN()
		userID = fmt.Sprintf("%s:%s:%s", verified.AccountID, verified.Principal.RoleName, verified.Principal.SessionName)
		identityType = "AssumedRole"
	} else if !verified.Principal.IsRoot {
		userID = verified.AccessKeyID
		arn = verified.Principal.ARN()
		if arn == "" {
			arn = fmt.Sprintf("arn:aws:iam::%s:user/%s", verified.AccountID, verified.AccessKeyID)
		}
		identityType = "IAMUser"
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
			"type":        identityType,
			"accountId":   verified.AccountID,
			"accessKeyId": verified.AccessKeyID,
			"arn":         arn,
		},
		RequestParameters: map[string]any{
			"httpMethod": r.Method,
			"path":       r.URL.Path,
		},
	}
	_ = s.audit.Write(context.Background(), ev)
}

func (s *Server) handleCreateAccount(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsCreateAccount, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:CreateAccount.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	email := params["Email"]
	name := params["AccountName"]
	if email == "" || name == "" {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"Email and AccountName are required.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	createID, _, err := s.store.CreateMemberAccount(verified.AccountID, email, name)
	if err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create account.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if wantsJSON(r, body) {
		payload, err := json.Marshal(map[string]any{
			"CreateAccountStatus": map[string]any{
				"Id":          createID,
				"AccountName": name,
				"State":       "SUCCEEDED",
			},
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build CreateAccount response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeJSONOK(w, requestID, payload)
	} else {
		payload, err := organizations.CreateAccountXML(organizations.CreateAccountResult{
			RequestID:   requestID,
			CreateID:    createID,
			AccountName: name,
			State:       "SUCCEEDED",
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build CreateAccount response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "CreateAccount", false)
}

func (s *Server) handleDescribeCreateAccountStatus(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsDescribeCreateAccountStatus, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:DescribeCreateAccountStatus.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	createID := params["CreateAccountRequestId"]
	if createID == "" {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"CreateAccountRequestId is required.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	status, accountID, _, failure, err := s.store.DescribeCreateAccountStatus(createID)
	if err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "CreateAccountStatusNotFoundException",
			"CreateAccount request not found.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if wantsJSON(r, body) {
		statusObj := map[string]any{
			"Id":        createID,
			"State":     status,
			"AccountId": accountID,
		}
		if failure != "" {
			statusObj["FailureReason"] = failure
		}
		payload, err := json.Marshal(map[string]any{"CreateAccountStatus": statusObj})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build DescribeCreateAccountStatus response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeJSONOK(w, requestID, payload)
	} else {
		payload, err := organizations.DescribeCreateAccountStatusXML(organizations.DescribeCreateAccountStatusResult{
			RequestID:     requestID,
			CreateID:      createID,
			State:         status,
			AccountID:     accountID,
			FailureReason: failure,
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build DescribeCreateAccountStatus response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "DescribeCreateAccountStatus", true)
}

func (s *Server) handleAssumeRole(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	roleARN := params.Get("RoleArn")
	sessionName := params.Get("RoleSessionName")
	if roleARN == "" || sessionName == "" {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
			"RoleArn and RoleSessionName are required.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
			"RoleArn is invalid.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:AssumeRole.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if storedARN != "" {
		roleARN = storedARN
	}

	callerDocs, _ := s.store.ListAttachedPolicyDocuments(verified.Principal.ARN())
	decision := authz.EvaluateCrossAccount(authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: verified.Principal,
			Action:    catalog.ActionSTSAssumeRole,
			Resource:  roleARN,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount": verified.AccountID,
				"aws:RequestedRegion":  verified.Region,
			},
		},
		CallerIdentityDocs: callerDocs,
		TrustPolicyDoc:     trust,
	})
	if decision != authz.Allow {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:AssumeRole on the specified resource.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	secret, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	sessionToken, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	expires := s.now().UTC().Add(defaultAssumeRoleDuration)
	accessKeyID, err := s.store.MintTempCredentials(accountID, roleARN, sessionName, secret, sessionToken, expires)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to store temporary credentials.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	assumedARN := identity.RoleSessionPrincipal(accountID, roleName, sessionName, accessKeyID).ARN()
	assumedID := fmt.Sprintf("%s:%s:%s", accountID, roleName, sessionName)
	payload, err := sts.AssumeRoleXML(sts.AssumeRoleResult{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secret,
		SessionToken:    sessionToken,
		Expiration:      expires,
		AssumedRoleARN:  assumedARN,
		AssumedRoleID:   assumedID,
		RequestID:       requestID,
	})
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build AssumeRole response.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	s.writeXMLOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "sts.amazonaws.com", "AssumeRole", false)
}

func (s *Server) authorizeOrgs(verified *authn.Verified, action, resource string) bool {
	docs, _ := s.store.ListAttachedPolicyDocuments(verified.Principal.ARN())
	return authz.Evaluate(authz.RequestContext{
		Principal: verified.Principal,
		Action:    action,
		Resource:  resource,
		Region:    verified.Region,
		ConditionKeys: map[string]string{
			"aws:PrincipalAccount": verified.AccountID,
			"aws:RequestedRegion":  verified.Region,
		},
	}, docs) == authz.Allow
}

func (s *Server) writeXMLOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)
}

func (s *Server) writeSuccessAudit(
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	eventSource, eventName string,
	readOnly bool,
) {
	uid := map[string]any{
		"type":        "Root",
		"accountId":   verified.AccountID,
		"accessKeyId": verified.AccessKeyID,
		"arn":         verified.Principal.ARN(),
	}
	if !verified.Principal.IsRoot {
		uid["type"] = "IAMUser"
	}
	if verified.Principal.Kind == identity.KindRole {
		uid["type"] = "AssumedRole"
	}
	ev := audit.Event{
		EventVersion:       eventVersion,
		EventTime:          s.now().UTC().Format(time.RFC3339),
		EventSource:        eventSource,
		EventName:          eventName,
		AWSRegion:          verified.Region,
		SourceIPAddress:    clientIP(r),
		UserAgent:          r.UserAgent(),
		RequestID:          requestID,
		EventID:            eventID,
		EventType:          "AwsApiCall",
		RecipientAccountID: verified.AccountID,
		ReadOnly:           readOnly,
		UserIdentity:       uid,
		RequestParameters: map[string]any{
			"httpMethod": r.Method,
			"path":       r.URL.Path,
		},
	}
	_ = s.audit.Write(context.Background(), ev)
}

func formParams(r *http.Request, body []byte) url.Values {
	vals := url.Values{}
	for k, v := range r.URL.Query() {
		vals[k] = v
	}
	if len(body) > 0 {
		if parsed, err := url.ParseQuery(string(body)); err == nil {
			for k, v := range parsed {
				vals[k] = v
			}
		}
	}
	return vals
}

func requestParams(r *http.Request, body []byte) map[string]string {
	out := map[string]string{}
	for k, vs := range formParams(r, body) {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	if len(body) > 0 && (strings.Contains(r.Header.Get("Content-Type"), "json") || looksLikeJSON(body)) {
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err == nil {
			for k, v := range obj {
				switch t := v.(type) {
				case string:
					out[k] = t
				case float64:
					out[k] = fmt.Sprintf("%.0f", t)
				}
			}
		}
	}
	return out
}

func looksLikeJSON(body []byte) bool {
	b := strings.TrimSpace(string(body))
	return strings.HasPrefix(b, "{") || strings.HasPrefix(b, "[")
}

func wantsJSON(r *http.Request, body []byte) bool {
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "json") {
		return true
	}
	if strings.Contains(r.Header.Get("X-Amz-Target"), "AWSOrganizations") {
		return true
	}
	return looksLikeJSON(body)
}

func (s *Server) writeJSONOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeAPIError(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID string,
	status int,
	code, message string,
	readOnly bool,
	eventID, accessKeyID, accountID string,
	knownKey bool,
) {
	if wantsJSON(r, body) {
		w.Header().Set(requestIDHeader, requestID)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(status)
		payload, _ := json.Marshal(map[string]string{
			"__type":  code,
			"message": message,
		})
		_, _ = w.Write(payload)
		// still audit via writeAWSError path without rewriting body
		s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, knownKey)
		return
	}
	s.writeAWSError(w, requestID, status, code, message, readOnly, r, eventID, accessKeyID, accountID, knownKey)
}

func (s *Server) auditAPIError(
	r *http.Request,
	requestID, eventID, code, message string,
	readOnly bool,
	accessKeyID, accountID string,
	knownKey bool,
) {
	ev := audit.Event{
		EventVersion:       eventVersion,
		EventTime:          s.now().UTC().Format(time.RFC3339),
		EventSource:        "organizations.amazonaws.com",
		EventName:          eventNameForRequest(r),
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

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var _ = store.OrganizationAccountAccessRoleName
