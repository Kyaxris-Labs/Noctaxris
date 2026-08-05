package server

import (
	"errors"
	"net/http"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	cognitosvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cognito"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func (s *Server) cognitoAdminGetUser(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	username, _ := params["Username"].(string)
	if !s.authorize(verified, catalog.ActionCognitoAdminGetUser, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:AdminGetUser.", readOnly, eventID, verified)
		return
	}
	user, err := s.store.AdminGetCognitoUser(verified.AccountID, poolID, username)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get user.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.AdminGetUserJSON(user)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "AdminGetUser", readOnly)
}

func (s *Server) cognitoAdminSetUserPassword(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	username, _ := params["Username"].(string)
	password, _ := params["Password"].(string)
	permanent, _ := params["Permanent"].(bool)
	if !s.authorize(verified, catalog.ActionCognitoAdminSetUserPassword, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:AdminSetUserPassword.", readOnly, eventID, verified)
		return
	}
	err := s.store.AdminSetCognitoUserPassword(verified.AccountID, poolID, username, password, permanent)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) || errors.Is(err, store.ErrCognitoInvalidPass) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to set user password.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.AdminSetUserPasswordJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "AdminSetUserPassword", readOnly)
}

func (s *Server) cognitoAdminDeleteUser(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	username, _ := params["Username"].(string)
	if !s.authorize(verified, catalog.ActionCognitoAdminDeleteUser, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:AdminDeleteUser.", readOnly, eventID, verified)
		return
	}
	err := s.store.AdminDeleteCognitoUser(verified.AccountID, poolID, username)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete user.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.AdminDeleteUserJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "AdminDeleteUser", readOnly)
}

func (s *Server) cognitoAdminDisableUser(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	username, _ := params["Username"].(string)
	if !s.authorize(verified, catalog.ActionCognitoAdminDisableUser, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:AdminDisableUser.", readOnly, eventID, verified)
		return
	}
	err := s.store.AdminDisableCognitoUser(verified.AccountID, poolID, username)
	if errors.Is(err, store.ErrCognitoNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User pool not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoUserNotFound) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "UserNotFoundException",
			"User does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCognitoBadRequest) {
		s.writeCognitoError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCognitoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to disable user.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.AdminDisableUserJSON()
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "AdminDisableUser", readOnly)
}

func (s *Server) cognitoListUsers(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	poolID, _ := params["UserPoolId"].(string)
	if !s.authorize(verified, catalog.ActionCognitoListUsers, "*") {
		s.writeCognitoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cognito-idp:ListUsers.", readOnly, eventID, verified)
		return
	}
	users, err := s.store.ListCognitoUsers(verified.AccountID, poolID)
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
			"Unable to list users.", readOnly, eventID, verified)
		return
	}
	payload, _ := cognitosvc.ListUsersJSON(users)
	s.writeCognitoOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cognitoEventSource, "ListUsers", readOnly)
}
