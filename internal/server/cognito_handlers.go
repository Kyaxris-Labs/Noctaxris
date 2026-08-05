package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	cognitosvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cognito"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	cognitoJSONContentType = "application/x-amz-json-1.1"
	cognitoEventSource     = "cognito-idp.amazonaws.com"
)

func isCognitoJWKSPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// cognito-idp / {region} / {poolId} / .well-known / jwks.json
	return len(parts) == 5 &&
		parts[0] == "cognito-idp" &&
		parts[3] == ".well-known" &&
		parts[4] == "jwks.json"
}

func cognitoPoolIDFromJWKSPath(path string) (region, poolID string) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 5 {
		return "", ""
	}
	return parts[1], parts[2]
}

func (s *Server) handleCognitoJWKS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, poolID := cognitoPoolIDFromJWKSPath(r.URL.Path)
	jwks, err := s.store.CognitoJWKSJSON(poolID)
	if errors.Is(err, store.ErrCognitoNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(jwks)
	}
}

func (s *Server) handleCognito(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = cognitoAction(action)

	switch action {
	case catalog.ActionCognitoCreateUserPool:
		s.cognitoCreateUserPool(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoDescribeUserPool:
		s.cognitoDescribeUserPool(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoUpdateUserPool:
		s.cognitoUpdateUserPool(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoListUserPools:
		s.cognitoListUserPools(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoDeleteUserPool:
		s.cognitoDeleteUserPool(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoCreateUserPoolClient:
		s.cognitoCreateUserPoolClient(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoDescribeUserPoolClient:
		s.cognitoDescribeUserPoolClient(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoListUserPoolClients:
		s.cognitoListUserPoolClients(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoDeleteUserPoolClient:
		s.cognitoDeleteUserPoolClient(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoAdminCreateUser:
		s.cognitoAdminCreateUser(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoAdminGetUser:
		s.cognitoAdminGetUser(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoAdminSetUserPassword:
		s.cognitoAdminSetUserPassword(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoAdminDeleteUser:
		s.cognitoAdminDeleteUser(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoAdminDisableUser:
		s.cognitoAdminDisableUser(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoListUsers:
		s.cognitoListUsers(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoSignUp:
		s.cognitoSignUp(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoConfirmSignUp:
		s.cognitoConfirmSignUp(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoForgotPassword:
		s.cognitoForgotPassword(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoConfirmForgotPassword:
		s.cognitoConfirmForgotPassword(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoResendConfirmationCode:
		s.cognitoResendConfirmationCode(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoUpdateUserAttributes:
		s.cognitoUpdateUserAttributes(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoGetUserAttributeVerificationCode:
		s.cognitoGetUserAttributeVerificationCode(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoVerifyUserAttribute:
		s.cognitoVerifyUserAttribute(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoInitiateAuth:
		s.cognitoInitiateAuth(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoAdminInitiateAuth:
		s.cognitoAdminInitiateAuth(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoRevokeToken:
		s.cognitoRevokeToken(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoAssociateSoftwareToken:
		s.cognitoAssociateSoftwareToken(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoVerifySoftwareToken:
		s.cognitoVerifySoftwareToken(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoRespondToAuthChallenge:
		s.cognitoRespondToAuthChallenge(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCognitoError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Cognito action is not implemented.", readOnly, eventID, verified)
	}
}

func cognitoAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateUserPool":
		return catalog.ActionCognitoCreateUserPool
	case "DescribeUserPool":
		return catalog.ActionCognitoDescribeUserPool
	case "UpdateUserPool":
		return catalog.ActionCognitoUpdateUserPool
	case "ListUserPools":
		return catalog.ActionCognitoListUserPools
	case "DeleteUserPool":
		return catalog.ActionCognitoDeleteUserPool
	case "CreateUserPoolClient":
		return catalog.ActionCognitoCreateUserPoolClient
	case "DescribeUserPoolClient":
		return catalog.ActionCognitoDescribeUserPoolClient
	case "ListUserPoolClients":
		return catalog.ActionCognitoListUserPoolClients
	case "DeleteUserPoolClient":
		return catalog.ActionCognitoDeleteUserPoolClient
	case "AdminCreateUser":
		return catalog.ActionCognitoAdminCreateUser
	case "AdminGetUser":
		return catalog.ActionCognitoAdminGetUser
	case "AdminSetUserPassword":
		return catalog.ActionCognitoAdminSetUserPassword
	case "AdminDeleteUser":
		return catalog.ActionCognitoAdminDeleteUser
	case "AdminDisableUser":
		return catalog.ActionCognitoAdminDisableUser
	case "ListUsers":
		return catalog.ActionCognitoListUsers
	case "SignUp":
		return catalog.ActionCognitoSignUp
	case "ConfirmSignUp":
		return catalog.ActionCognitoConfirmSignUp
	case "ForgotPassword":
		return catalog.ActionCognitoForgotPassword
	case "ConfirmForgotPassword":
		return catalog.ActionCognitoConfirmForgotPassword
	case "ResendConfirmationCode":
		return catalog.ActionCognitoResendConfirmationCode
	case "UpdateUserAttributes":
		return catalog.ActionCognitoUpdateUserAttributes
	case "GetUserAttributeVerificationCode":
		return catalog.ActionCognitoGetUserAttributeVerificationCode
	case "VerifyUserAttribute":
		return catalog.ActionCognitoVerifyUserAttribute
	case "InitiateAuth":
		return catalog.ActionCognitoInitiateAuth
	case "AdminInitiateAuth":
		return catalog.ActionCognitoAdminInitiateAuth
	case "RevokeToken":
		return catalog.ActionCognitoRevokeToken
	case "AssociateSoftwareToken":
		return catalog.ActionCognitoAssociateSoftwareToken
	case "VerifySoftwareToken":
		return catalog.ActionCognitoVerifySoftwareToken
	case "RespondToAuthChallenge":
		return catalog.ActionCognitoRespondToAuthChallenge
	default:
		return action
	}
}

func cognitoRoleArnFromParams(params map[string]any) string {
	roleARN, _ := params["RoleArn"].(string)
	return strings.TrimSpace(roleARN)
}

func cognitoLambdaConfigFromParams(params map[string]any) store.CognitoLambdaConfig {
	raw, _ := params["LambdaConfig"].(map[string]any)
	return store.CognitoLambdaConfigFromAPI(raw)
}

func (s *Server) checkCognitoPassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	return s.checkServicePassRole(verified, roleARN, sourceARN, authz.ServicePrincipalCognitoIDP, "Cognito", passRoleMsgs{
		InvalidARN:   "RoleArn must be a valid IAM role ARN",
		WrongAccount: "RoleArn must be in the same account",
		NotFound:     "RoleArn not found",
	})
}

func (s *Server) cognitoCreateUserPool(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["PoolName"].(string)
	if !s.authorize(verified, catalog.ActionCognitoCreateUserPool, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:CreateUserPool.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultCognitoRegion
	}
	roleARN := cognitoRoleArnFromParams(params)
	lambdaCfg := cognitoLambdaConfigFromParams(params)
	pool, err := s.store.CreateCognitoUserPool(verified.AccountID, region, name)
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create user pool.", readOnly, eventID, verified)
		return
	}
	if roleARN != "" {
		if err := s.checkCognitoPassRole(verified, roleARN, pool.ARN); err != nil {
			_ = s.store.DeleteCognitoUserPool(verified.AccountID, pool.PoolID)
			s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	if roleARN != "" || lambdaCfg.HasTriggers() {
		pool, err = s.store.SetCognitoUserPoolTriggers(verified.AccountID, pool.PoolID, roleARN, lambdaCfg)
		if err != nil {
			_ = s.store.DeleteCognitoUserPool(verified.AccountID, pool.PoolID)
			s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to configure user pool triggers.", readOnly, eventID, verified)
			return
		}
	}
	payload, _ := cognitosvc.CreateUserPoolJSON(pool)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "CreateUserPool", readOnly)
}

func (s *Server) cognitoUpdateUserPool(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	if !s.authorize(verified, catalog.ActionCognitoUpdateUserPool, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:UpdateUserPool.", readOnly, eventID, verified)
		return
	}
	pool, err := s.store.DescribeCognitoUserPool(verified.AccountID, poolID)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update user pool.", readOnly, eventID, verified)
		return
	}
	roleARN := cognitoRoleArnFromParams(params)
	lambdaCfg := cognitoLambdaConfigFromParams(params)
	// AWS UpdateUserPool replace semantics for LambdaConfig: omitted/empty clears triggers.
	if _, hasLC := params["LambdaConfig"]; !hasLC {
		lambdaCfg = store.CognitoLambdaConfig{}
	}
	if _, hasRole := params["RoleArn"]; !hasRole {
		roleARN = ""
	}
	if roleARN != "" {
		if err := s.checkCognitoPassRole(verified, roleARN, pool.ARN); err != nil {
			s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	if _, err := s.store.SetCognitoUserPoolTriggers(verified.AccountID, pool.PoolID, roleARN, lambdaCfg); err != nil {
		if errors.Is(err, store.ErrCognitoNotFound) {
			s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
				"User pool not found.", readOnly, eventID, verified)
			return
		}
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update user pool.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.UpdateUserPoolJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "UpdateUserPool", readOnly)
}

func (s *Server) cognitoDescribeUserPool(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	if !s.authorize(verified, catalog.ActionCognitoDescribeUserPool, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:DescribeUserPool.", readOnly, eventID, verified)
		return
	}
	pool, err := s.store.DescribeCognitoUserPool(verified.AccountID, poolID)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe user pool.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.DescribeUserPoolJSON(pool)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "DescribeUserPool", readOnly)
}

func (s *Server) cognitoListUserPools(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionCognitoListUserPools, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:ListUserPools.", readOnly, eventID, verified)
		return
	}
	pools, err := s.store.ListCognitoUserPools(verified.AccountID)
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list user pools.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.ListUserPoolsJSON(pools)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "ListUserPools", readOnly)
}

func (s *Server) cognitoDeleteUserPool(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	if !s.authorize(verified, catalog.ActionCognitoDeleteUserPool, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:DeleteUserPool.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteCognitoUserPool(verified.AccountID, poolID)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete user pool.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.DeleteUserPoolJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "DeleteUserPool", readOnly)
}

func (s *Server) cognitoCreateUserPoolClient(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	name, _ := params["ClientName"].(string)
	if !s.authorize(verified, catalog.ActionCognitoCreateUserPoolClient, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:CreateUserPoolClient.", readOnly, eventID, verified)
		return
	}
	client, err := s.store.CreateCognitoUserPoolClient(verified.AccountID, poolID, name)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create user pool client.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.CreateUserPoolClientJSON(client)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "CreateUserPoolClient", readOnly)
}

func (s *Server) cognitoDescribeUserPoolClient(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	clientID, _ := params["ClientId"].(string)
	if !s.authorize(verified, catalog.ActionCognitoDescribeUserPoolClient, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:DescribeUserPoolClient.", readOnly, eventID, verified)
		return
	}
	client, err := s.store.DescribeCognitoUserPoolClient(verified.AccountID, poolID, clientID)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool client not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe user pool client.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.DescribeUserPoolClientJSON(client)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "DescribeUserPoolClient", readOnly)
}

func (s *Server) cognitoListUserPoolClients(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	if !s.authorize(verified, catalog.ActionCognitoListUserPoolClients, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:ListUserPoolClients.", readOnly, eventID, verified)
		return
	}
	clients, err := s.store.ListCognitoUserPoolClients(verified.AccountID, poolID)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list user pool clients.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.ListUserPoolClientsJSON(clients)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "ListUserPoolClients", readOnly)
}

func (s *Server) cognitoDeleteUserPoolClient(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	clientID, _ := params["ClientId"].(string)
	if !s.authorize(verified, catalog.ActionCognitoDeleteUserPoolClient, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:DeleteUserPoolClient.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteCognitoUserPoolClient(verified.AccountID, poolID, clientID)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool client not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete user pool client.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.DeleteUserPoolClientJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "DeleteUserPoolClient", readOnly)
}

func (s *Server) cognitoAdminCreateUser(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	username, _ := params["Username"].(string)
	password, _ := params["TemporaryPassword"].(string)
	if !s.authorize(verified, catalog.ActionCognitoAdminCreateUser, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:AdminCreateUser.", readOnly, eventID, verified)
		return
	}
	user, err := s.store.AdminCreateCognitoUser(verified.AccountID, poolID, username, password)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserExists) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UsernameExistsException",
			"User already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) || errors.Is(err, store.ErrCognitoInvalidPass) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create user.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.AdminCreateUserJSON(user)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "AdminCreateUser", readOnly)
}

func (s *Server) cognitoSignUp(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	clientID, _ := params["ClientId"].(string)
	username, _ := params["Username"].(string)
	password, _ := params["Password"].(string)
	// SignUp is public in AWS; lab still requires SigV4 identity for management surface consistency.
	if !s.authorize(verified, catalog.ActionCognitoSignUp, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:SignUp.", readOnly, eventID, verified)
		return
	}
	user, _, err := s.store.SignUpCognitoUser(verified.AccountID, clientID, username, password)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Client not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserExists) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UsernameExistsException",
			"User already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoTriggerFailed) {
		s.writeCognitoTriggerError(w, r, body, requestID, eventID, verified, readOnly, err)
		return
	}
	if errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidLambdaResponseException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) || errors.Is(err, store.ErrCognitoInvalidPass) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to sign up.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.SignUpJSON(user)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "SignUp", readOnly)
}

func (s *Server) cognitoConfirmSignUp(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	clientID, _ := params["ClientId"].(string)
	username, _ := params["Username"].(string)
	code, _ := params["ConfirmationCode"].(string)
	if !s.authorize(verified, catalog.ActionCognitoConfirmSignUp, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:ConfirmSignUp.", readOnly, eventID, verified)
		return
	}
	err := s.store.ConfirmSignUpCognitoUser(clientID, username, code)
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoTriggerFailed) {
		s.writeCognitoTriggerError(w, r, body, requestID, eventID, verified, readOnly, err)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to confirm sign up.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.ConfirmSignUpJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "ConfirmSignUp", readOnly)
}

func (s *Server) cognitoForgotPassword(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	clientID, _ := params["ClientId"].(string)
	username, _ := params["Username"].(string)
	if !s.authorize(verified, catalog.ActionCognitoForgotPassword, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:ForgotPassword.", readOnly, eventID, verified)
		return
	}
	details, err := s.store.ForgotPasswordCognitoUser(clientID, username)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Client not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoTriggerFailed) {
		s.writeCognitoTriggerError(w, r, body, requestID, eventID, verified, readOnly, err)
		return
	}
	if errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidLambdaResponseException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to forgot password.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.CodeDeliveryDetailsJSON(details)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "ForgotPassword", readOnly)
}

func (s *Server) cognitoConfirmForgotPassword(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	clientID, _ := params["ClientId"].(string)
	username, _ := params["Username"].(string)
	code, _ := params["ConfirmationCode"].(string)
	password, _ := params["Password"].(string)
	// ConfirmForgotPassword is a public Cognito IdP API (no IAM evaluation on AWS).
	err := s.store.ConfirmForgotPasswordCognitoUser(clientID, username, code, password)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Client not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoCodeMismatch) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "CodeMismatchException",
			"Invalid verification code provided, please try again.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) || errors.Is(err, store.ErrCognitoInvalidPass) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to confirm forgot password.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.ConfirmForgotPasswordJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "ConfirmForgotPassword", readOnly)
}

func cognitoUserAttributesMap(params map[string]any) map[string]string {
	raw, _ := params["UserAttributes"].([]any)
	if raw == nil {
		return nil
	}
	out := make(map[string]string, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["Name"].(string)
		value, _ := m["Value"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[name] = value
	}
	return out
}

func (s *Server) cognitoUpdateUserAttributes(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	accessToken, _ := params["AccessToken"].(string)
	attrs := cognitoUserAttributesMap(params)
	// Public IdP: access token authorizes (no IAM evaluation).
	details, err := s.store.UpdateUserAttributesCognitoUser(accessToken, attrs)
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			"Invalid access token.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoTriggerFailed) {
		s.writeCognitoTriggerError(w, r, body, requestID, eventID, verified, readOnly, err)
		return
	}
	if errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidLambdaResponseException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update user attributes.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.UpdateUserAttributesJSON(details)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "UpdateUserAttributes", readOnly)
}

func (s *Server) cognitoGetUserAttributeVerificationCode(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	accessToken, _ := params["AccessToken"].(string)
	attrName, _ := params["AttributeName"].(string)
	// Public IdP: access token authorizes (no IAM evaluation).
	details, err := s.store.GetUserAttributeVerificationCodeCognitoUser(accessToken, attrName)
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			"Invalid access token.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoTriggerFailed) {
		s.writeCognitoTriggerError(w, r, body, requestID, eventID, verified, readOnly, err)
		return
	}
	if errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidLambdaResponseException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get user attribute verification code.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.CodeDeliveryDetailsJSON(details)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "GetUserAttributeVerificationCode", readOnly)
}

func (s *Server) cognitoVerifyUserAttribute(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	accessToken, _ := params["AccessToken"].(string)
	attrName, _ := params["AttributeName"].(string)
	code, _ := params["Code"].(string)
	// Public IdP: access token authorizes (no IAM evaluation).
	err := s.store.VerifyUserAttributeCognitoUser(accessToken, attrName, code)
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			"Invalid access token.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoCodeMismatch) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "CodeMismatchException",
			"Invalid verification code provided, please try again.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to verify user attribute.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.VerifyUserAttributeJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "VerifyUserAttribute", readOnly)
}

func (s *Server) cognitoResendConfirmationCode(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	clientID, _ := params["ClientId"].(string)
	username, _ := params["Username"].(string)
	if !s.authorize(verified, catalog.ActionCognitoResendConfirmationCode, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:ResendConfirmationCode.", readOnly, eventID, verified)
		return
	}
	details, err := s.store.ResendConfirmationCodeCognitoUser(clientID, username)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Client not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoTriggerFailed) {
		s.writeCognitoTriggerError(w, r, body, requestID, eventID, verified, readOnly, err)
		return
	}
	if errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidLambdaResponseException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to resend confirmation code.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.CodeDeliveryDetailsJSON(details)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "ResendConfirmationCode", readOnly)
}

func (s *Server) writeCognitoTriggerError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, err error,
) {
	if err != nil {
		log.Printf("cognito trigger failed: %v", err)
	}
	s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UnexpectedLambdaException",
		"Configured Lambda trigger failed.", readOnly, eventID, verified)
}

func cognitoAuthParams(params map[string]any) (username, password, refreshToken, srpA string) {
	raw, _ := params["AuthParameters"].(map[string]any)
	if raw == nil {
		return "", "", "", ""
	}
	username, _ = raw["USERNAME"].(string)
	password, _ = raw["PASSWORD"].(string)
	refreshToken, _ = raw["REFRESH_TOKEN"].(string)
	srpA, _ = raw["SRP_A"].(string)
	return username, password, refreshToken, srpA
}

func cognitoIsRefreshFlow(flow string) bool {
	flow = strings.ToUpper(strings.TrimSpace(flow))
	return flow == "REFRESH_TOKEN_AUTH" || flow == "REFRESH_TOKEN"
}

func (s *Server) cognitoInitiateAuth(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	clientID, _ := params["ClientId"].(string)
	flow, _ := params["AuthFlow"].(string)
	username, password, refreshToken, srpA := cognitoAuthParams(params)
	// InitiateAuth is a public Cognito IdP API (no IAM / SigV4 on AWS).
	flowNorm := strings.ToUpper(strings.TrimSpace(flow))
	var outcome store.CognitoAuthOutcome
	var err error
	switch {
	case flowNorm == "USER_PASSWORD_AUTH":
		outcome, err = s.store.InitiateCognitoAuth(clientID, username, password)
	case flowNorm == "USER_SRP_AUTH":
		outcome, err = s.store.InitiateCognitoSRPAuth(clientID, username, srpA)
	case flowNorm == "CUSTOM_AUTH":
		outcome, err = s.store.InitiateCognitoCustomAuth(clientID, username, srpA)
	case cognitoIsRefreshFlow(flow):
		result, refreshErr := s.store.RefreshCognitoTokens("", clientID, refreshToken)
		err = refreshErr
		outcome = store.CognitoAuthOutcome{CognitoAuthResult: result}
	default:
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Only USER_PASSWORD_AUTH, USER_SRP_AUTH, CUSTOM_AUTH, REFRESH_TOKEN_AUTH, or REFRESH_TOKEN is supported.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		msg := "Incorrect username or password."
		if cognitoIsRefreshFlow(flow) {
			msg = "Invalid refresh token."
		}
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			msg, readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Client not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoTriggerFailed) {
		s.writeCognitoTriggerError(w, r, body, requestID, eventID, verified, readOnly, err)
		return
	}
	if errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidLambdaResponseException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to initiate auth.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.AuthOutcomeJSON(outcome)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "InitiateAuth", readOnly)
}

func (s *Server) cognitoAdminInitiateAuth(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	clientID, _ := params["ClientId"].(string)
	flow, _ := params["AuthFlow"].(string)
	username, password, refreshToken, _ := cognitoAuthParams(params)
	if !s.authorize(verified, catalog.ActionCognitoAdminInitiateAuth, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:AdminInitiateAuth.", readOnly, eventID, verified)
		return
	}
	flowNorm := strings.ToUpper(strings.TrimSpace(flow))
	var outcome store.CognitoAuthOutcome
	var err error
	switch {
	case flowNorm == "USER_PASSWORD_AUTH" || flowNorm == "ADMIN_USER_PASSWORD_AUTH" || flowNorm == "ADMIN_NO_SRP_AUTH":
		outcome, err = s.store.AdminInitiateCognitoAuth(verified.AccountID, poolID, clientID, username, password)
	case cognitoIsRefreshFlow(flow):
		result, refreshErr := s.store.RefreshCognitoTokens(verified.AccountID, clientID, refreshToken)
		err = refreshErr
		outcome = store.CognitoAuthOutcome{CognitoAuthResult: result}
	default:
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Only USER_PASSWORD_AUTH, ADMIN_USER_PASSWORD_AUTH, REFRESH_TOKEN_AUTH, or REFRESH_TOKEN is supported.",
			readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		msg := "Incorrect username or password."
		if cognitoIsRefreshFlow(flow) {
			msg = "Invalid refresh token."
		}
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			msg, readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Pool or client not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoTriggerFailed) {
		s.writeCognitoTriggerError(w, r, body, requestID, eventID, verified, readOnly, err)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to initiate auth.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.AuthOutcomeJSON(outcome)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "AdminInitiateAuth", readOnly)
}

func (s *Server) cognitoRevokeToken(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	clientID, _ := params["ClientId"].(string)
	token, _ := params["Token"].(string)
	// RevokeToken is a public Cognito IdP API (no IAM evaluation on AWS).
	err := s.store.RevokeCognitoToken(clientID, token)
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UnauthorizedException",
			"Invalid refresh token.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to revoke token.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.RevokeTokenJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "RevokeToken", readOnly)
}

func (s *Server) cognitoAssociateSoftwareToken(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	accessToken, _ := params["AccessToken"].(string)
	session, _ := params["Session"].(string)
	sessionOrAccess := strings.TrimSpace(accessToken)
	if sessionOrAccess == "" {
		sessionOrAccess = strings.TrimSpace(session)
	}
	// Public IdP: no IAM evaluation (access token or session authorizes).
	secretCode, outSession, err := s.store.AssociateSoftwareTokenMFA("", "", "", sessionOrAccess)
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			"Invalid access token or session.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to associate software token.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.AssociateSoftwareTokenJSON(secretCode, outSession)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "AssociateSoftwareToken", readOnly)
}

func (s *Server) cognitoVerifySoftwareToken(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	session, _ := params["Session"].(string)
	userCode, _ := params["UserCode"].(string)
	accessToken, _ := params["AccessToken"].(string)
	// Lab: Session from AssociateSoftwareToken is required; AccessToken alone is not enough
	// without a pending associate session (AWS allows either).
	if strings.TrimSpace(session) == "" && strings.TrimSpace(accessToken) != "" {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Session from AssociateSoftwareToken is required.", readOnly, eventID, verified)
		return
	}
	err := s.store.VerifySoftwareTokenMFA("", "", "", session, userCode)
	if errors.Is(err, store.ErrCognitoCodeMismatch) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "CodeMismatchException",
			"Invalid code provided, please request a code again.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			"Invalid session.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to verify software token.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.VerifySoftwareTokenJSON("SUCCESS")
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "VerifySoftwareToken", readOnly)
}

func cognitoChallengeResponses(params map[string]any) map[string]string {
	raw, _ := params["ChallengeResponses"].(map[string]any)
	if raw == nil {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func (s *Server) cognitoRespondToAuthChallenge(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	clientID, _ := params["ClientId"].(string)
	challengeName, _ := params["ChallengeName"].(string)
	session, _ := params["Session"].(string)
	responses := cognitoChallengeResponses(params)
	// Public IdP: no IAM evaluation.
	challengeNorm := strings.ToUpper(strings.TrimSpace(challengeName))
	var outcome store.CognitoAuthOutcome
	var err error
	switch challengeNorm {
	case "SOFTWARE_TOKEN_MFA":
		username := responses["USERNAME"]
		totpCode := responses["SOFTWARE_TOKEN_MFA_CODE"]
		result, mfaErr := s.store.RespondToSOFTWARETokenMFAChallenge(clientID, username, session, totpCode)
		err = mfaErr
		outcome = store.CognitoAuthOutcome{CognitoAuthResult: result}
	case "PASSWORD_VERIFIER":
		outcome, err = s.store.RespondToCognitoPASSWORDVerifierChallenge(clientID, session, responses)
	case "CUSTOM_CHALLENGE":
		outcome, err = s.store.RespondToCognitoCUSTOMChallenge(clientID, session, responses)
	default:
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Only PASSWORD_VERIFIER, SOFTWARE_TOKEN_MFA, or CUSTOM_CHALLENGE is supported.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoCodeMismatch) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "CodeMismatchException",
			"Invalid code provided, please request a code again.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			"Invalid session or challenge response.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoTriggerFailed) {
		s.writeCognitoTriggerError(w, r, body, requestID, eventID, verified, readOnly, err)
		return
	}
	if errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidLambdaResponseException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to respond to auth challenge.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.AuthOutcomeJSON(outcome)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "RespondToAuthChallenge", readOnly)
}

func (s *Server) writeCognitoOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", cognitoJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCognitoError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", cognitoJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, code, readOnly)
}
