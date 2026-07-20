package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
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
	case catalog.ActionCognitoSignUp:
		s.cognitoSignUp(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoConfirmSignUp:
		s.cognitoConfirmSignUp(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoInitiateAuth:
		s.cognitoInitiateAuth(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCognitoAdminInitiateAuth:
		s.cognitoAdminInitiateAuth(w, r, body, requestID, eventID, verified, readOnly, params)
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
	case "SignUp":
		return catalog.ActionCognitoSignUp
	case "ConfirmSignUp":
		return catalog.ActionCognitoConfirmSignUp
	case "InitiateAuth":
		return catalog.ActionCognitoInitiateAuth
	case "AdminInitiateAuth":
		return catalog.ActionCognitoAdminInitiateAuth
	default:
		return action
	}
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
	payload, _ := cognitosvc.CreateUserPoolJSON(pool)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "CreateUserPool", readOnly)
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

func cognitoAuthParams(params map[string]any) (username, password string) {
	raw, _ := params["AuthParameters"].(map[string]any)
	if raw == nil {
		return "", ""
	}
	username, _ = raw["USERNAME"].(string)
	password, _ = raw["PASSWORD"].(string)
	return username, password
}

func (s *Server) cognitoInitiateAuth(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	clientID, _ := params["ClientId"].(string)
	flow, _ := params["AuthFlow"].(string)
	username, password := cognitoAuthParams(params)
	if !s.authorize(verified, catalog.ActionCognitoInitiateAuth, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:InitiateAuth.", readOnly, eventID, verified)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(flow), "USER_PASSWORD_AUTH") {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Only USER_PASSWORD_AUTH is supported.", readOnly, eventID, verified)
		return
	}
	result, err := s.store.InitiateCognitoAuth(clientID, username, password)
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			"Incorrect username or password.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Client not found.", readOnly, eventID, verified)
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
	payload, _ := cognitosvc.AuthResultJSON(result)
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
	username, password := cognitoAuthParams(params)
	if !s.authorize(verified, catalog.ActionCognitoAdminInitiateAuth, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:AdminInitiateAuth.", readOnly, eventID, verified)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(flow), "USER_PASSWORD_AUTH") &&
		!strings.EqualFold(strings.TrimSpace(flow), "ADMIN_USER_PASSWORD_AUTH") &&
		!strings.EqualFold(strings.TrimSpace(flow), "ADMIN_NO_SRP_AUTH") {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Only USER_PASSWORD_AUTH or ADMIN_USER_PASSWORD_AUTH is supported.", readOnly, eventID, verified)
		return
	}
	result, err := s.store.AdminInitiateCognitoAuth(verified.AccountID, poolID, clientID, username, password)
	if errors.Is(err, store.ErrCognitoUnauthorized) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "NotAuthorizedException",
			"Incorrect username or password.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Pool or client not found.", readOnly, eventID, verified)
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
	payload, _ := cognitosvc.AuthResultJSON(result)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "AdminInitiateAuth", readOnly)
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
