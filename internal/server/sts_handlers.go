package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/federation"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const defaultSessionDuration = time.Hour

// lookupGetSessionTokenSession returns the access-key row when verified is a
// GetSessionToken-minted user/root session (not AssumeRole / federation).
func (s *Server) lookupGetSessionTokenSession(verified *authn.Verified) (store.AccessKey, bool) {
	if verified == nil || verified.SessionToken == "" {
		return store.AccessKey{}, false
	}
	if verified.Principal.Kind == identity.KindRole || verified.Principal.Kind == identity.KindFederated {
		return store.AccessKey{}, false
	}
	ak, err := s.store.LookupAccessKeyRecord(verified.AccessKeyID)
	if err != nil || ak.RoleARN != "" || ak.FederatedUser != "" {
		return store.AccessKey{}, false
	}
	return ak, true
}

// getSessionTokenSessionBlocksIAM is true when a GetSessionToken session lacks MFA
// (AWS: those credentials cannot call IAM without MFA).
func (s *Server) getSessionTokenSessionBlocksIAM(verified *authn.Verified) bool {
	ak, ok := s.lookupGetSessionTokenSession(verified)
	if !ok {
		return false
	}
	return !ak.MFAAuthenticated
}

// getSessionTokenSessionBlocksSTS is true when a GetSessionToken session cannot
// call the given STS action (AWS allows only AssumeRole and GetCallerIdentity).
func (s *Server) getSessionTokenSessionBlocksSTS(verified *authn.Verified, action string) bool {
	if _, ok := s.lookupGetSessionTokenSession(verified); !ok {
		return false
	}
	switch action {
	case catalog.ActionSTSAssumeRole, "AssumeRole",
		catalog.ActionSTSGetCallerIdentity, "GetCallerIdentity":
		return false
	default:
		return true
	}
}

func (s *Server) handleGetSessionToken(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	// AWS: GetSessionToken requires long-term credentials of an IAM user or root.
	// No IAM permission check for sts:GetSessionToken after SigV4.
	if verified.SessionToken != "" ||
		verified.Principal.Kind == identity.KindRole ||
		verified.Principal.Kind == identity.KindFederated {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"GetSessionToken must be called with long-term credentials.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	params := formParams(r, body)
	serial := params.Get("SerialNumber")
	tokenCode := params.Get("TokenCode")
	now := s.now().UTC()
	mfaPresent := false

	if serial != "" || tokenCode != "" {
		if serial == "" || tokenCode == "" {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"SerialNumber and TokenCode must both be provided for MFA.", readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		dev, err := s.store.GetMFADevice(verified.AccountID, serial)
		if err != nil || !dev.Enabled {
			s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
				"MultiFactorAuthentication failed with invalid MFA one time pass code.", readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		callerUser := verified.Principal.UserName
		if !verified.Principal.IsRoot && (callerUser == "" || callerUser != dev.UserName) {
			s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
				"MultiFactorAuthentication failed with invalid MFA one time pass code.", readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		if !sts.ValidateLabTokenCode(dev.Seed, tokenCode, now, 1) {
			s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
				"MultiFactorAuthentication failed with invalid MFA one time pass code.", readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		mfaPresent = true
	}

	secret, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	sessionToken, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	expires := now.Add(defaultSessionDuration)
	userName := verified.Principal.UserName
	mintOpts := store.MintTempOpts{
		AccountID:    verified.AccountID,
		SessionName:  "session",
		UserName:     userName,
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      expires,
		IsRoot:       verified.Principal.IsRoot,
	}
	if mfaPresent {
		mintOpts.MFAAuthenticated = true
		mintOpts.MFAAuthenticatedAt = now
	}
	accessKeyID, err := s.store.MintTempCredentialsOpts(mintOpts)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to store temporary credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	payload, err := sts.GetSessionTokenXML(sts.GetSessionTokenResult{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secret,
		SessionToken:    sessionToken,
		Expiration:      expires,
		RequestID:       requestID,
	})
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build GetSessionToken response.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	s.writeXMLOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "sts.amazonaws.com", "GetSessionToken", false)
}

func (s *Server) handleGetFederationToken(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if s.getSessionTokenSessionBlocksSTS(verified, catalog.ActionSTSGetFederationToken) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Temporary credentials from GetSessionToken cannot call sts:GetFederationToken.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	params := formParams(r, body)
	name := params.Get("Name")
	policy := params.Get("Policy")
	if policy == "" {
		policy = params.Get("PolicyArns.member.1.arn")
	}
	if name == "" {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
			"Name is required.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if !s.authorize(verified, catalog.ActionSTSGetFederationToken, "*") {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:GetFederationToken.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	sessionPolicy := ""
	if policy != "" {
		sessionPolicy = policy
		if strings.HasPrefix(policy, "arn:") {
			p, err := s.store.GetManagedPolicy(policy)
			if err != nil {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Policy ARN not found.", readOnly, r, eventID,
					verified.AccessKeyID, verified.AccountID, true)
				return
			}
			sessionPolicy = p.Document
		} else if unesc, err := url.QueryUnescape(policy); err == nil && unesc != "" {
			sessionPolicy = unesc
		}
	}
	secret, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	sessionToken, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	expires := s.now().UTC().Add(defaultSessionDuration)
	userName := verified.Principal.UserName
	mintOpts := store.MintTempOpts{
		AccountID:     verified.AccountID,
		SessionName:   name,
		UserName:      userName,
		FederatedUser: name,
		SessionPolicy: sessionPolicy,
		Secret:        secret,
		SessionToken:  sessionToken,
		Expires:       expires,
		IsRoot:        verified.Principal.IsRoot,
	}
	accessKeyID, err := s.store.MintTempCredentialsOpts(mintOpts)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to store temporary credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	fedARN := identity.FederatedUserPrincipal(verified.AccountID, name, accessKeyID).ARN()
	fedID := fmt.Sprintf("%s:%s", verified.AccountID, name)
	payload, err := sts.GetFederationTokenXML(sts.GetFederationTokenResult{
		AccessKeyID:      accessKeyID,
		SecretAccessKey:  secret,
		SessionToken:     sessionToken,
		Expiration:       expires,
		FederatedUserARN: fedARN,
		FederatedUserID:  fedID,
		PackedPolicySize: len(sessionPolicy) * 100 / 2048,
		RequestID:        requestID,
	})
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build GetFederationToken response.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	s.writeXMLOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "sts.amazonaws.com", "GetFederationToken", false)
}

func (s *Server) handleGetAccessKeyInfo(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if s.getSessionTokenSessionBlocksSTS(verified, catalog.ActionSTSGetAccessKeyInfo) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Temporary credentials from GetSessionToken cannot call sts:GetAccessKeyInfo.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	params := formParams(r, body)
	akid := params.Get("AccessKeyId")
	if akid == "" {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
			"AccessKeyId is required.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	ak, err := s.store.LookupAccessKeyRecord(akid)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
			"Access key not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	payload, err := sts.GetAccessKeyInfoXML(ak.AccountID, requestID)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build GetAccessKeyInfo response.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	s.writeXMLOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "sts.amazonaws.com", "GetAccessKeyInfo", true)
}

func (s *Server) handleDecodeAuthorizationMessage(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if s.getSessionTokenSessionBlocksSTS(verified, catalog.ActionSTSDecodeAuthorizationMessage) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Temporary credentials from GetSessionToken cannot call sts:DecodeAuthorizationMessage.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if !s.authorize(verified, catalog.ActionSTSDecodeAuthorizationMessage, "*") {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:DecodeAuthorizationMessage.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	params := formParams(r, body)
	encoded := params.Get("EncodedMessage")
	decoded, err := sts.DecodeAuthorizationMessage(encoded)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusBadRequest, sts.CodeInvalidAuthorizationMessageException,
			"The encoded authorization message was invalid.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	payload, err := sts.DecodeAuthorizationMessageXML(decoded, requestID)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build DecodeAuthorizationMessage response.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	s.writeXMLOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "sts.amazonaws.com", "DecodeAuthorizationMessage", true)
}

func (s *Server) handleAssumeRoot(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if s.getSessionTokenSessionBlocksSTS(verified, catalog.ActionSTSAssumeRoot) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Temporary credentials from GetSessionToken cannot call sts:AssumeRoot.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	params := formParams(r, body)
	target := params.Get("TargetAccount")
	if target == "" {
		target = params.Get("TargetPrincipal")
	}
	if !verified.Principal.IsRoot || !s.store.IsOrgMemberAccount(verified.AccountID, target) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:AssumeRoot.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if !s.authorize(verified, catalog.ActionSTSAssumeRoot, "*") {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:AssumeRoot.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	secret, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	sessionToken, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	expires := s.now().UTC().Add(defaultSessionDuration)
	accessKeyID, err := s.store.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    target,
		SessionName:  "AssumeRoot",
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      expires,
		IsRoot:       true,
	})
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to store temporary credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	payload, err := sts.AssumeRootXML(sts.AssumeRootResult{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secret,
		SessionToken:    sessionToken,
		Expiration:      expires,
		RequestID:       requestID,
	})
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build AssumeRoot response.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	s.writeXMLOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "sts.amazonaws.com", "AssumeRoot", false)
}

func (s *Server) handleAssumeRoleWithSAML(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	roleARN := params.Get("RoleArn")
	principalARN := params.Get("PrincipalArn")
	assertion := params.Get("SAMLAssertion")
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok || principalARN == "" || assertion == "" {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
			"RoleArn, PrincipalArn, and SAMLAssertion are required.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	idp, err := s.store.GetSAMLProvider(principalARN)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDeniedException",
				"IdP not configured", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load SAML provider.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if err := federation.VerifySAMLAssertion(idp.MetadataXML, assertion); err != nil {
		code := federation.CodeInvalidIdentityToken
		if fe, ok := err.(*federation.Error); ok {
			code = fe.Code()
		}
		s.writeAWSError(w, requestID, http.StatusForbidden, code,
			err.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:AssumeRoleWithSAML.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	// Trust eval uses Federated provider ARN (not account-root AWS principal over-allow).
	fedPrincipal := identity.FederatedProviderPrincipal(accountID, principalARN, "saml", verified.AccessKeyID)
	callerDocs := []string{fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:AssumeRoleWithSAML","Resource":"%s"}]}`,
		roleARN,
	)}
	decision := authz.EvaluateCrossAccount(authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: fedPrincipal,
			Action:    catalog.ActionSTSAssumeRoleWithSAML,
			Resource:  roleARN,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount":  accountID,
				"aws:RequestedRegion":   verified.Region,
				"aws:FederatedProvider": principalARN,
			},
		},
		CallerIdentityDocs: callerDocs,
		TrustPolicyDoc:     trust,
	})
	if decision != authz.Allow {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:AssumeRoleWithSAML on the specified resource.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	s.mintAndWriteAssumeRole(w, r, requestID, eventID, verified, readOnly, accountID, roleName, roleARN, params.Get("RoleSessionName"))
}

func (s *Server) handleAssumeRoleWithWebIdentity(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	roleARN := params.Get("RoleArn")
	token := params.Get("WebIdentityToken")
	sessionName := params.Get("RoleSessionName")
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok || token == "" {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
			"RoleArn and WebIdentityToken are required.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	iss, _, err := federation.ParseJWTUnverified(token)
	if err != nil || iss == "" {
		providers, _ := s.store.ListOIDCProviders(accountID)
		if len(providers) == 0 {
			s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
				"IdP not configured", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeAWSError(w, requestID, http.StatusForbidden, federation.CodeInvalidIdentityToken,
			"OIDC token is invalid", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	idp, err := s.store.GetOIDCProviderByURL(accountID, iss)
	if err != nil {
		// try trimmed / with https
		idp, err = s.store.GetOIDCProviderByURL(accountID, strings.TrimRight(iss, "/"))
		if err != nil {
			providers, _ := s.store.ListOIDCProviders(accountID)
			if len(providers) == 0 {
				s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
					"IdP not configured", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
			s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
				"IdP not configured", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
	}
	if _, err := federation.VerifyWebIdentityJWT(token, idp.URL, idp.ClientID, nil); err != nil {
		code := federation.CodeInvalidIdentityToken
		if fe, ok := err.(*federation.Error); ok {
			code = fe.Code()
		}
		s.writeAWSError(w, requestID, http.StatusForbidden, code,
			err.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:AssumeRoleWithWebIdentity.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	fedPrincipal := identity.FederatedProviderPrincipal(accountID, idp.ProviderARN, "oidc", verified.AccessKeyID)
	callerDocs := []string{fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:AssumeRoleWithWebIdentity","Resource":"%s"}]}`,
		roleARN,
	)}
	decision := authz.EvaluateCrossAccount(authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: fedPrincipal,
			Action:    catalog.ActionSTSAssumeRoleWithWebIdentity,
			Resource:  roleARN,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount":  accountID,
				"aws:RequestedRegion":   verified.Region,
				"aws:FederatedProvider": idp.ProviderARN,
			},
		},
		CallerIdentityDocs: callerDocs,
		TrustPolicyDoc:     trust,
	})
	if decision != authz.Allow {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:AssumeRoleWithWebIdentity on the specified resource.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if sessionName == "" {
		sessionName = "web-identity"
	}
	s.mintAndWriteAssumeRole(w, r, requestID, eventID, verified, readOnly, accountID, roleName, roleARN, sessionName)
}

func (s *Server) handleGetDelegatedAccessToken(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if s.getSessionTokenSessionBlocksSTS(verified, catalog.ActionSTSGetDelegatedAccessToken) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Temporary credentials from GetSessionToken cannot call sts:GetDelegatedAccessToken.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	err := sts.GetDelegatedAccessToken()
	s.writeAWSError(w, requestID, http.StatusForbidden, sts.CodeOf(err),
		err.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
}

func (s *Server) handleGetWebIdentityToken(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if s.getSessionTokenSessionBlocksSTS(verified, catalog.ActionSTSGetWebIdentityToken) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Temporary credentials from GetSessionToken cannot call sts:GetWebIdentityToken.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	providers, _ := s.store.ListOIDCProviders(verified.AccountID)
	params := formParams(r, body)
	err := sts.GetWebIdentityToken(len(providers) > 0, params.Get("Audience"))
	s.writeAWSError(w, requestID, http.StatusForbidden, sts.CodeOf(err),
		err.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
}

func (s *Server) mintAndWriteAssumeRole(
	w http.ResponseWriter,
	r *http.Request,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	accountID, roleName, roleARN, sessionName string,
) {
	if sessionName == "" {
		sessionName = "session"
	}
	secret, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	sessionToken, err := randomSecret()
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to mint credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	expires := s.now().UTC().Add(defaultAssumeRoleDuration)
	accessKeyID, err := s.store.MintTempCredentials(accountID, roleARN, sessionName, secret, sessionToken, expires)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to store temporary credentials.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
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
			"Unable to build AssumeRole response.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	s.writeXMLOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "sts.amazonaws.com", "AssumeRole", false)
}
