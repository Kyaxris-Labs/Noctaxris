package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
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
	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

const (
	defaultAssumeRoleDuration = time.Hour
	minAssumeRoleSeconds      = 900
	maxAssumeRoleSeconds      = 43200
	maxRoleChainSeconds       = 3600
)

func (s *Server) lookupKey(accessKeyID string) (authn.ResolvedKey, error) {
	ak, err := s.store.LookupAccessKeyRecord(accessKeyID)
	if err != nil {
		return authn.ResolvedKey{}, err
	}
	return authn.ResolvedKey{
		AccountID:     ak.AccountID,
		Secret:        ak.Secret,
		IsRoot:        ak.IsRoot,
		UserName:      ak.UserName,
		Status:        ak.Status,
		SessionToken:  ak.SessionToken,
		RoleARN:       ak.RoleARN,
		SessionName:   ak.SessionName,
		FederatedUser: ak.FederatedUser,
		SessionPolicy: ak.SessionPolicy,
		ExpiresAt:     ak.ExpiresAt,
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
	} else if verified.Principal.Kind == identity.KindFederated {
		arn = verified.Principal.ARN()
		userID = fmt.Sprintf("%s:%s", verified.AccountID, verified.Principal.SessionName)
		identityType = "FederatedUser"
	} else if !verified.Principal.IsRoot {
		userID = verified.AccessKeyID
		if ak, err := s.store.LookupAccessKeyRecord(verified.AccessKeyID); err == nil && ak.UserName != "" {
			if u, err := s.store.GetUser(verified.AccountID, ak.UserName); err == nil && u.UserID != "" {
				userID = u.UserID
			}
		}
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
		if validate.IsInvalid(err) {
			s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
				err.Error(), readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
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

	// Gate caller with EvaluateFull (SCP / boundary / session) before trust eval.
	if !s.authorize(verified, catalog.ActionSTSAssumeRole, roleARN) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:AssumeRole on the specified resource.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	callerDocs := s.identityDocs(verified.Principal)
	trustKeys := s.conditionKeys(verified)
	if ext := strings.TrimSpace(params.Get("ExternalId")); ext != "" {
		trustKeys["sts:ExternalId"] = ext
	}
	if src := strings.TrimSpace(params.Get("SourceIdentity")); src != "" {
		trustKeys["sts:SourceIdentity"] = src
		trustKeys["aws:SourceIdentity"] = src
	}
	decision := authz.EvaluateCrossAccount(authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal:     verified.Principal,
			Action:        catalog.ActionSTSAssumeRole,
			Resource:      roleARN,
			Region:        verified.Region,
			ConditionKeys: trustKeys,
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

	sessionPolicy := ""
	if policy := params.Get("Policy"); policy != "" {
		if unesc, err := url.QueryUnescape(policy); err == nil && unesc != "" {
			sessionPolicy = unesc
		} else {
			sessionPolicy = policy
		}
	} else if policyARN := params.Get("PolicyArns.member.1.arn"); policyARN != "" {
		p, err := s.store.GetManagedPolicy(policyARN)
		if err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Policy ARN not found.", readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		sessionPolicy = p.Document
	}

	roleRec, roleErr := s.store.GetRoleRecord(accountID, roleName)
	maxRoleSecs := int64(3600)
	if roleErr == nil && roleRec.MaxSessionDuration > 0 {
		maxRoleSecs = int64(roleRec.MaxSessionDuration)
	}
	maxAllowed := maxRoleSecs
	if maxAllowed > maxAssumeRoleSeconds {
		maxAllowed = maxAssumeRoleSeconds
	}
	if s.isRoleChainingCaller(verified) && maxAllowed > maxRoleChainSeconds {
		maxAllowed = maxRoleChainSeconds
	}

	duration := defaultAssumeRoleDuration
	if raw := strings.TrimSpace(params.Get("DurationSeconds")); raw != "" {
		secs, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || secs < minAssumeRoleSeconds || secs > maxAssumeRoleSeconds {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"DurationSeconds must be between 900 and 43200.", readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		if secs > maxAllowed {
			msg := fmt.Sprintf("The requested DurationSeconds exceeds the %d second session limit for this role.", maxAllowed)
			if s.isRoleChainingCaller(verified) && secs > maxRoleChainSeconds {
				msg = "The requested DurationSeconds exceeds the 1 hour session limit for roles assumed by role chaining."
			}
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				msg, readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		duration = time.Duration(secs) * time.Second
	} else if int64(duration/time.Second) > maxAllowed {
		duration = time.Duration(maxAllowed) * time.Second
	}
	expires := s.now().UTC().Add(duration)
	accessKeyID, err := s.store.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:     accountID,
		RoleARN:       roleARN,
		SessionName:   sessionName,
		Secret:        secret,
		SessionToken:  sessionToken,
		Expires:       expires,
		SessionPolicy: sessionPolicy,
	})
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
	return s.authorize(verified, action, resource)
}

// isRoleChainingCaller reports whether AssumeRole is called with temporary
// credentials (role session, federated session, or any SessionToken).
func (s *Server) isRoleChainingCaller(verified *authn.Verified) bool {
	if verified == nil {
		return false
	}
	if strings.TrimSpace(verified.SessionToken) != "" {
		return true
	}
	switch verified.Principal.Kind {
	case identity.KindRole, identity.KindFederated:
		return true
	default:
		return false
	}
}

func (s *Server) conditionKeys(verified *authn.Verified) map[string]string {
	now := s.now().UTC()
	keys := map[string]string{
		"aws:PrincipalAccount": verified.AccountID,
		"aws:RequestedRegion":  verified.Region,
		"aws:CurrentTime":      now.Format(time.RFC3339),
		"aws:EpochTime":        fmt.Sprintf("%d", now.Unix()),
		"aws:SecureTransport":  s.secureTransportValue(),
		"aws:PrincipalType":    principalTypeForCondition(verified.Principal),
	}
	if arn := principalArnForCondition(verified.Principal); arn != "" {
		keys["aws:PrincipalArn"] = arn
	}
	if verified.Principal.Kind == identity.KindUser && verified.Principal.UserName != "" {
		keys["aws:username"] = verified.Principal.UserName
		if u, err := s.store.GetUser(verified.AccountID, verified.Principal.UserName); err == nil && u.UserID != "" {
			keys["aws:userid"] = u.UserID
		}
	}
	if verified.Principal.Kind == identity.KindRole && verified.Principal.RoleName != "" {
		if r, err := s.store.GetRoleRecord(verified.AccountID, verified.Principal.RoleName); err == nil && r.RoleID != "" {
			if verified.Principal.SessionName != "" {
				keys["aws:userid"] = r.RoleID + ":" + verified.Principal.SessionName
			} else {
				keys["aws:userid"] = r.RoleID
			}
		}
	}
	if ip := strings.TrimSpace(verified.SourceIP); ip != "" {
		keys["aws:SourceIp"] = ip
	}
	if ak, err := s.store.LookupAccessKeyRecord(verified.AccessKeyID); err == nil && ak.MFAAuthenticated {
		keys["aws:MultiFactorAuthPresent"] = "true"
		if !ak.MFAAuthenticatedAt.IsZero() {
			age := int(s.now().UTC().Sub(ak.MFAAuthenticatedAt.UTC()).Seconds())
			if age < 0 {
				age = 0
			}
			keys["aws:MultiFactorAuthAge"] = fmt.Sprintf("%d", age)
		}
	}
	return keys
}

func (s *Server) secureTransportValue() string {
	if strings.TrimSpace(s.cfg.TLSCertFile) != "" && strings.TrimSpace(s.cfg.TLSKeyFile) != "" {
		return "true"
	}
	return "false"
}

func principalTypeForCondition(p identity.Principal) string {
	switch {
	case p.IsRoot || p.Kind == identity.KindRoot:
		return "Account"
	case p.Kind == identity.KindUser:
		return "User"
	case p.Kind == identity.KindRole:
		return "AssumedRole"
	case p.Kind == identity.KindFederated:
		return "FederatedUser"
	default:
		return "Unknown"
	}
}

// principalArnForCondition returns aws:PrincipalArn. For IAM roles AWS documents
// the role ARN (not the assumed-role session ARN).
func principalArnForCondition(p identity.Principal) string {
	if p.Kind == identity.KindRole && p.RoleName != "" {
		return fmt.Sprintf("arn:aws:iam::%s:role/%s", p.AccountID, p.RoleName)
	}
	return p.ARN()
}

func (s *Server) mergeResourceTagConditionKeys(keys map[string]string, accountID, resource string) {
	resource = strings.TrimSpace(resource)
	if keys == nil || resource == "" || resource == "*" {
		return
	}
	tagAccount := resourceAccountIDFromARN(resource)
	if tagAccount == "" {
		tagAccount = accountID
	}
	if tagAccount == "" {
		return
	}
	tags, err := s.store.ListResourceTags(tagAccount, resource)
	if err != nil || len(tags) == 0 {
		return
	}
	svcPrefix := serviceResourceTagKeyPrefix(resource)
	for _, tag := range tags {
		k := strings.TrimSpace(tag.Key)
		if k == "" {
			continue
		}
		keys["aws:ResourceTag/"+k] = tag.Value
		if svcPrefix != "" {
			keys[svcPrefix+k] = tag.Value
		}
	}
}

// serviceResourceTagKeyPrefix returns a service-specific ResourceTag key prefix
// for cataloged templates. Empty when none.
func serviceResourceTagKeyPrefix(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 3 {
		return ""
	}
	switch strings.ToLower(parts[2]) {
	case "ecr":
		return "ecr:ResourceTag/"
	case "ssm":
		return "ssm:resourceTag/"
	case "secretsmanager":
		return "secretsmanager:ResourceTag/"
	case "iam":
		return "iam:ResourceTag/"
	case "ecs":
		return "ecs:ResourceTag/"
	default:
		return ""
	}
}

func (s *Server) evalInputs(verified *authn.Verified) (authz.EvalInputs, bool) {
	docs := s.identityDocs(verified.Principal)
	sessionDocs := s.sessionPolicyDocs(verified.AccessKeyID)

	boundaryDoc := ""
	boundaryUser := ""
	switch verified.Principal.Kind {
	case identity.KindUser:
		boundaryUser = verified.Principal.UserName
	case identity.KindRole:
		if verified.Principal.RoleName != "" {
			doc, ok, err := s.store.PermissionsBoundaryDoc(verified.AccountID, "role", verified.Principal.RoleName)
			if err != nil {
				return authz.EvalInputs{}, false
			}
			if ok {
				boundaryDoc = doc
			}
		}
	case identity.KindFederated:
		// GetFederationToken: identity ∩ session from calling IAM user; empty session Denies.
		if ak, err := s.store.LookupAccessKeyRecord(verified.AccessKeyID); err == nil && ak.FederatedUser != "" {
			if ak.UserName != "" {
				docs = s.identityDocs(identity.UserPrincipal(ak.AccountID, ak.UserName, verified.AccessKeyID))
				boundaryUser = ak.UserName
			} else if ak.IsRoot {
				docs = []string{`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`}
			} else {
				docs = nil
			}
			if ak.SessionPolicy == "" {
				sessionDocs = []string{`{"Version":"2012-10-17","Statement":[]}`}
			}
		}
	}
	if boundaryUser != "" {
		doc, ok, err := s.store.PermissionsBoundaryDoc(verified.AccountID, "user", boundaryUser)
		if err != nil {
			return authz.EvalInputs{}, false
		}
		if ok {
			boundaryDoc = doc
		}
	}

	scpDocs, err := s.store.SCPDocsForAccount(verified.AccountID)
	if err != nil {
		return authz.EvalInputs{}, false
	}
	rcpDocs, err := s.store.RCPDocsForAccount(verified.AccountID)
	if err != nil {
		return authz.EvalInputs{}, false
	}

	return authz.EvalInputs{
		IdentityDocs:        docs,
		BoundaryDoc:         boundaryDoc,
		SessionDocs:         sessionDocs,
		SCPDocs:             scpDocs,
		RCPDocs:             rcpDocs,
		IsManagementAccount: s.store.IsManagementAccount(verified.AccountID),
	}, true
}

func (s *Server) requestContext(verified *authn.Verified, action, resource string) authz.RequestContext {
	keys := s.conditionKeys(verified)
	s.mergeResourceTagConditionKeys(keys, verified.AccountID, resource)
	return authz.RequestContext{
		Principal:     verified.Principal,
		Action:        action,
		Resource:      resource,
		Region:        verified.Region,
		ConditionKeys: keys,
	}
}

func (s *Server) authorize(verified *authn.Verified, action, resource string) bool {
	in, ok := s.evalInputs(verified)
	if !ok {
		return false
	}
	return authz.EvaluateFull(s.requestContext(verified, action, resource), in) == authz.Allow
}

// authorizeDataplaneOR applies SCP/RCP, then a resource evaluator (S3/SQS/SNS/DynamoDB).
// Same-account: identity Allow OR resource policy Allow. Cross-account (resourceAccountID
// differs from caller): identity Allow AND resource policy Allow via EvaluateResourceAccess.
// Boundary and session intersect only when identity Allows (ADR-0005 §8 resource-policy-only
// path skips boundary/session).
func (s *Server) authorizeDataplaneOR(
	verified *authn.Verified,
	action, resource, resourceAccountID string,
	eval func(caller authz.RequestContext, identityDocs []string, resourceAccountID string) authz.Decision,
) bool {
	in, ok := s.evalInputs(verified)
	if !ok {
		return false
	}
	ctx := s.requestContext(verified, action, resource)
	if authz.OrgFiltersDeny(ctx, in) {
		return false
	}
	if eval(ctx, in.IdentityDocs, resourceAccountID) != authz.Allow {
		return false
	}
	if authz.Evaluate(ctx, in.IdentityDocs) != authz.Allow {
		return true
	}
	if !ctx.Principal.IsRoot && strings.TrimSpace(in.BoundaryDoc) != "" {
		if authz.Evaluate(ctx, []string{in.BoundaryDoc}) != authz.Allow {
			return false
		}
	}
	if len(in.SessionDocs) > 0 {
		if authz.Evaluate(ctx, in.SessionDocs) != authz.Allow {
			return false
		}
	}
	return true
}

func mergeEncryptionContextKeys(keys map[string]string, encCtx map[string]string) {
	if keys == nil || len(encCtx) == 0 {
		return
	}
	sorted := make([]string, 0, len(encCtx))
	for k, v := range encCtx {
		keys["kms:EncryptionContext:"+k] = v
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	keys["kms:EncryptionContextKeys"] = strings.Join(sorted, ",")
}

// authorizeDataplaneKMS applies SCP/RCP, EvaluateKMS, then always intersects
// boundary (when set, non-root) and session (when present).
// encCtx populates kms:EncryptionContext:* and kms:EncryptionContextKeys.
func (s *Server) authorizeDataplaneKMS(
	verified *authn.Verified,
	action, resource, keyPolicy string,
	grantSatisfied bool,
	encCtx map[string]string,
) bool {
	in, ok := s.evalInputs(verified)
	if !ok {
		return false
	}
	ctx := s.requestContext(verified, action, resource)
	mergeEncryptionContextKeys(ctx.ConditionKeys, encCtx)
	if authz.OrgFiltersDeny(ctx, in) {
		return false
	}
	if authz.EvaluateKMS(authz.KMSRequest{
		Caller:         ctx,
		IdentityDocs:   in.IdentityDocs,
		KeyPolicyDoc:   keyPolicy,
		GrantSatisfied: grantSatisfied,
	}) != authz.Allow {
		return false
	}
	if !ctx.Principal.IsRoot && strings.TrimSpace(in.BoundaryDoc) != "" {
		if authz.Evaluate(ctx, []string{in.BoundaryDoc}) != authz.Allow {
			return false
		}
	}
	if len(in.SessionDocs) > 0 {
		if authz.Evaluate(ctx, in.SessionDocs) != authz.Allow {
			return false
		}
	}
	return true
}

func (s *Server) identityDocs(principal identity.Principal) []string {
	if principal.Kind == identity.KindUser && principal.UserName != "" {
		docs, err := s.store.IdentityPolicyDocsForUser(principal.AccountID, principal.UserName)
		if err == nil {
			return docs
		}
		log.Printf("identity docs user load failed account=%s user=%s err=%v", principal.AccountID, principal.UserName, err)
		return nil
	}
	// Assumed-role sessions use sts:assumed-role/... ARNs; IAM attachments live on the role ARN.
	arn := principal.ARN()
	if principal.Kind == identity.KindRole && principal.RoleName != "" {
		arn = principalArnForCondition(principal)
	}
	docs, err := s.store.ListAttachedPolicyDocuments(arn)
	if err != nil {
		log.Printf("identity docs attached load failed arn=%s err=%v", arn, err)
		return nil
	}
	inline, err := s.store.ListInlinePolicies(arn)
	if err != nil {
		log.Printf("identity docs inline load failed arn=%s err=%v", arn, err)
		return nil
	}
	for _, p := range inline {
		docs = append(docs, p.Document)
	}
	return docs
}

func (s *Server) sessionPolicyDocs(accessKeyID string) []string {
	ak, err := s.store.LookupAccessKeyRecord(accessKeyID)
	if err != nil || ak.SessionPolicy == "" {
		return nil
	}
	return []string{ak.SessionPolicy}
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
	if verified.Principal.Kind == identity.KindFederated {
		uid["type"] = "FederatedUser"
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
