package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	sm "github.com/Kyaxris-Labs/Noctaxris/internal/services/secretsmanager"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	secretsJSONContentType = "application/x-amz-json-1.1"
	secretsEventSource     = "secretsmanager.amazonaws.com"
)

func (s *Server) handleSecretsManager(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = secretsAction(action)

	switch action {
	case catalog.ActionSecretsCreateSecret:
		s.secretsCreateSecret(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsGetSecretValue:
		s.secretsGetSecretValue(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsPutSecretValue:
		s.secretsPutSecretValue(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsDeleteSecret:
		s.secretsDeleteSecret(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsDescribeSecret:
		s.secretsDescribeSecret(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsListSecrets:
		s.secretsListSecrets(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsPutResourcePolicy:
		s.secretsPutResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsGetResourcePolicy:
		s.secretsGetResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsDeleteResourcePolicy:
		s.secretsDeleteResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeSecretsError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Secrets Manager action is not implemented.", readOnly, eventID, verified)
	}
}

func secretsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateSecret":
		return catalog.ActionSecretsCreateSecret
	case "GetSecretValue":
		return catalog.ActionSecretsGetSecretValue
	case "PutSecretValue":
		return catalog.ActionSecretsPutSecretValue
	case "DeleteSecret":
		return catalog.ActionSecretsDeleteSecret
	case "DescribeSecret":
		return catalog.ActionSecretsDescribeSecret
	case "ListSecrets":
		return catalog.ActionSecretsListSecrets
	case "PutResourcePolicy":
		return catalog.ActionSecretsPutResourcePolicy
	case "GetResourcePolicy":
		return catalog.ActionSecretsGetResourcePolicy
	case "DeleteResourcePolicy":
		return catalog.ActionSecretsDeleteResourcePolicy
	default:
		return action
	}
}

func (s *Server) secretsRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultSecretsRegion
}

func (s *Server) authorizeSecretsManager(verified *authn.Verified, action, resource, resourcePolicy string) bool {
	return s.authorizeDataplaneOR(verified, action, resource, func(caller authz.RequestContext, identityDocs []string) authz.Decision {
		return authz.EvaluateDynamoDB(authz.DynamoDBRequest{
			Caller:            caller,
			IdentityDocs:      identityDocs,
			ResourcePolicyDoc: resourcePolicy,
		})
	})
}

func (s *Server) secretMetaOrErr(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	secretID string,
) (store.Secret, bool) {
	if strings.TrimSpace(secretID) == "" {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"SecretId is required.", readOnly, eventID, verified)
		return store.Secret{}, false
	}
	sec, err := s.store.DescribeSecret(verified.AccountID, secretID)
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return store.Secret{}, false
	}
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load secret.", readOnly, eventID, verified)
		return store.Secret{}, false
	}
	return sec, true
}

func secretsSecretID(params map[string]any) string {
	if id, _ := params["SecretId"].(string); strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if name, _ := params["Name"].(string); strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return ""
}

func secretsBinaryParam(v any) ([]byte, error) {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

func (s *Server) secretsCreateSecret(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name, _ := params["Name"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	secretString, _ := params["SecretString"].(string)
	secretBinary, err := secretsBinaryParam(params["SecretBinary"])
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"SecretBinary is not valid base64.", readOnly, eventID, verified)
		return
	}
	if secretString == "" && len(secretBinary) == 0 {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"You must provide SecretString or SecretBinary.", readOnly, eventID, verified)
		return
	}
	keyID, _ := params["KmsKeyId"].(string)
	description, _ := params["Description"].(string)

	arn := store.SecretARN(s.secretsRegion(verified), verified.AccountID, name, "000000")
	if !s.authorize(verified, catalog.ActionSecretsCreateSecret, arn) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:CreateSecret.", readOnly, eventID, verified)
		return
	}

	sec, err := s.store.CreateSecret(
		verified.AccountID, s.secretsRegion(verified), name, secretString, secretBinary, keyID, description,
	)
	if errors.Is(err, store.ErrSecretAlreadyExists) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceExistsException",
			"The secret already exists.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create secret.", readOnly, eventID, verified)
		return
	}

	payload, err := sm.CreateSecretJSON(sec)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "CreateSecret", readOnly)
}

func (s *Server) secretsGetSecretValue(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsGetSecretValue, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:GetSecretValue.", readOnly, eventID, verified)
		return
	}

	sec, err := s.store.GetSecretValue(verified.AccountID, secretID)
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get secret value.", readOnly, eventID, verified)
		return
	}

	payload, err := sm.GetSecretValueJSON(sec)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "GetSecretValue", readOnly)
}

func (s *Server) secretsPutSecretValue(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsPutSecretValue, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:PutSecretValue.", readOnly, eventID, verified)
		return
	}

	secretString, _ := params["SecretString"].(string)
	secretBinary, err := secretsBinaryParam(params["SecretBinary"])
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"SecretBinary is not valid base64.", readOnly, eventID, verified)
		return
	}
	if secretString == "" && len(secretBinary) == 0 {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"You must provide SecretString or SecretBinary.", readOnly, eventID, verified)
		return
	}

	sec, err := s.store.PutSecretValue(verified.AccountID, secretID, secretString, secretBinary)
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put secret value.", readOnly, eventID, verified)
		return
	}

	payload, err := sm.PutSecretValueJSON(sec)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "PutSecretValue", readOnly)
}

func (s *Server) secretsDeleteSecret(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsDeleteSecret, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:DeleteSecret.", readOnly, eventID, verified)
		return
	}

	if err := s.store.DeleteSecret(verified.AccountID, secretID); errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete secret.", readOnly, eventID, verified)
		return
	}

	deletionDate := time.Now().UTC().Format(time.RFC3339)
	payload, err := sm.DeleteSecretJSON(meta, deletionDate)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "DeleteSecret", readOnly)
}

func (s *Server) secretsDescribeSecret(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsDescribeSecret, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:DescribeSecret.", readOnly, eventID, verified)
		return
	}

	payload, err := sm.DescribeSecretJSON(meta)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "DescribeSecret", readOnly)
}

func (s *Server) secretsListSecrets(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionSecretsListSecrets, "*") {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:ListSecrets.", readOnly, eventID, verified)
		return
	}

	secrets, err := s.store.ListSecrets(verified.AccountID)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list secrets.", readOnly, eventID, verified)
		return
	}

	payload, err := sm.ListSecretsJSON(secrets)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "ListSecrets", readOnly)
}

func (s *Server) secretsPutResourcePolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsPutResourcePolicy, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:PutResourcePolicy.", readOnly, eventID, verified)
		return
	}
	policy, _ := params["ResourcePolicy"].(string)
	if strings.TrimSpace(policy) == "" {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"ResourcePolicy is required.", readOnly, eventID, verified)
		return
	}
	if err := s.store.PutSecretResourcePolicy(verified.AccountID, secretID, policy); err != nil {
		if errors.Is(err, store.ErrSecretNotFound) {
			s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
				"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
			return
		}
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put resource policy.", readOnly, eventID, verified)
		return
	}

	payload, err := sm.PutResourcePolicyJSON(meta)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "PutResourcePolicy", readOnly)
}

func (s *Server) secretsGetResourcePolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsGetResourcePolicy, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:GetResourcePolicy.", readOnly, eventID, verified)
		return
	}
	policy, err := s.store.GetSecretResourcePolicy(verified.AccountID, secretID)
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrNoSuchResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret value for ResourcePolicy.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get resource policy.", readOnly, eventID, verified)
		return
	}

	payload, err := sm.GetResourcePolicyJSON(meta, policy)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "GetResourcePolicy", readOnly)
}

func (s *Server) secretsDeleteResourcePolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsDeleteResourcePolicy, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:DeleteResourcePolicy.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteSecretResourcePolicy(verified.AccountID, secretID); errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete resource policy.", readOnly, eventID, verified)
		return
	}

	payload, err := sm.DeleteResourcePolicyJSON(meta)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "DeleteResourcePolicy", readOnly)
}

func (s *Server) writeSecretsOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", secretsJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeSecretsError(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID string,
	status int,
	code, message string,
	readOnly bool,
	eventID string,
	verified *authn.Verified,
) {
	_ = body
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", secretsJSONContentType)
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{
		"__type":  code,
		"message": message,
	})
	_, _ = w.Write(payload)

	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}
