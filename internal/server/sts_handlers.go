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

func (s *Server) handleGetSessionToken(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionSTSGetSessionToken, "*") {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:GetSessionToken.", readOnly, r, eventID,
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
	userName := verified.Principal.UserName
	accessKeyID, err := s.store.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    verified.AccountID,
		SessionName:  "session",
		UserName:     userName,
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      expires,
		IsRoot:       verified.Principal.IsRoot,
	})
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
	params := formParams(r, body)
	name := params.Get("Name")
	policy := params.Get("Policy")
	if policy == "" {
		policy = params.Get("PolicyArns.member.1.arn")
	}
	if name == "" || policy == "" {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
			"Name and Policy (or PolicyArns) are required.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if !s.authorize(verified, catalog.ActionSTSGetFederationToken, "*") {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to perform sts:GetFederationToken.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	sessionPolicy := policy
	if strings.HasPrefix(policy, "arn:") {
		if p, err := s.store.GetManagedPolicy(policy); err == nil {
			sessionPolicy = p.Document
		}
	} else {
		if unesc, err := url.QueryUnescape(policy); err == nil && unesc != "" {
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
	accessKeyID, err := s.store.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:     verified.AccountID,
		SessionName:   name,
		FederatedUser: name,
		SessionPolicy: sessionPolicy,
		Secret:        secret,
		SessionToken:  sessionToken,
		Expires:       expires,
	})
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
	// Dual-eval: identity of IdP principal + trust. Use federated-like principal for trust evaluation.
	fedPrincipal := identity.FederatedUserPrincipal(accountID, "saml", verified.AccessKeyID)
	callerDocs := s.identityDocs(fedPrincipal)
	decision := authz.EvaluateCrossAccount(authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: fedPrincipal,
			Action:    catalog.ActionSTSAssumeRoleWithSAML,
			Resource:  roleARN,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount": accountID,
				"aws:RequestedRegion":  verified.Region,
			},
		},
		CallerIdentityDocs: callerDocs,
		TrustPolicyDoc:     trust,
	})
	if decision != authz.Allow {
		// Root-like allow via trust only if trust permits the SAML provider principal.
		trustOnly := authz.EvaluateCrossAccount(authz.CrossAccountRequest{
			Caller: authz.RequestContext{
				Principal: identity.Principal{Kind: identity.KindFederated, AccountID: accountID, SessionName: "saml"},
				Action:    catalog.ActionSTSAssumeRoleWithSAML,
				Resource:  roleARN,
				Region:    verified.Region,
			},
			CallerIdentityDocs: []string{fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:AssumeRoleWithSAML","Resource":"%s"}]}`, roleARN)},
			TrustPolicyDoc:     trust,
		})
		if trustOnly != authz.Allow {
			s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
				"Not authorized to perform sts:AssumeRoleWithSAML on the specified resource.", readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
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
	fedPrincipal := identity.FederatedUserPrincipal(accountID, "oidc", verified.AccessKeyID)
	callerDocs := []string{fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:AssumeRoleWithWebIdentity","Resource":"%s"}]}`, roleARN)}
	decision := authz.EvaluateCrossAccount(authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: fedPrincipal,
			Action:    catalog.ActionSTSAssumeRoleWithWebIdentity,
			Resource:  roleARN,
			Region:    verified.Region,
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
