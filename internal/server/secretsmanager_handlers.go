package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
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
	case catalog.ActionSecretsRestoreSecret:
		s.secretsRestoreSecret(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsRotateSecret:
		s.secretsRotateSecret(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsUpdateSecretVersionStage:
		s.secretsUpdateSecretVersionStage(w, r, body, requestID, eventID, verified, readOnly, params)
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
	case catalog.ActionSecretsListTagsForResource:
		s.secretsListTagsForResource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsTagResource:
		s.secretsTagResource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecretsUntagResource:
		s.secretsUntagResource(w, r, body, requestID, eventID, verified, readOnly, params)
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
	case "RestoreSecret":
		return catalog.ActionSecretsRestoreSecret
	case "RotateSecret":
		return catalog.ActionSecretsRotateSecret
	case "UpdateSecretVersionStage":
		return catalog.ActionSecretsUpdateSecretVersionStage
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
	case "ListTagsForResource":
		return catalog.ActionSecretsListTagsForResource
	case "TagResource":
		return catalog.ActionSecretsTagResource
	case "UntagResource":
		return catalog.ActionSecretsUntagResource
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
	resourceAccountID := resourceAccountIDFromARN(resource)
	if resourceAccountID == "" {
		resourceAccountID = verified.AccountID
	}
	return s.authorizeDataplaneOR(verified, action, resource, resourceAccountID, func(caller authz.RequestContext, identityDocs []string, resourceAccountID string) authz.Decision {
		return authz.EvaluateDynamoDB(authz.DynamoDBRequest{
			Caller:            caller,
			IdentityDocs:      identityDocs,
			ResourcePolicyDoc: resourcePolicy,
			ResourceAccountID: resourceAccountID,
		})
	})
}

// secretsAuthorizeKMS resolves the secret CMK (or alias/aws/secretsmanager) and
// evaluates KMS Encrypt/Decrypt for Secrets Manager plaintext paths.
func (s *Server) secretsAuthorizeKMS(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	accountID, keyIDOrAlias, kmsAction string,
	encCtx map[string]string,
) bool {
	keyID, err := s.store.ResolveSecretsManagerKeyID(accountID, keyIDOrAlias)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid KmsKeyId.", readOnly, eventID, verified)
		return false
	}
	key, err := s.store.GetKey(keyID)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid KmsKeyId.", readOnly, eventID, verified)
		return false
	}
	if !s.authorizeKMSOp(verified, kmsAction, key, encCtx) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform "+kmsAction+" on the secret KMS key.", readOnly, eventID, verified)
		return false
	}
	return true
}

func (s *Server) secretMetaOrErr(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	secretID string,
) (store.Secret, string, bool) {
	if strings.TrimSpace(secretID) == "" {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"SecretId is required.", readOnly, eventID, verified)
		return store.Secret{}, "", false
	}
	accountID := verified.AccountID
	if acct := resourceAccountIDFromARN(secretID); acct != "" {
		accountID = acct
	}
	sec, err := s.store.DescribeSecret(accountID, secretID)
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return store.Secret{}, "", false
	}
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load secret.", readOnly, eventID, verified)
		return store.Secret{}, "", false
	}
	if owner := resourceAccountIDFromARN(sec.ARN); owner != "" {
		accountID = owner
	}
	return sec, accountID, true
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
	// AWS allows CreateSecret with neither SecretString nor SecretBinary (metadata-only;
	// Terraform aws_secretsmanager_secret then PutSecretValue via aws_secretsmanager_secret_version).
	keyID, _ := params["KmsKeyId"].(string)
	description, _ := params["Description"].(string)
	hasValue := secretString != "" || len(secretBinary) > 0

	suffix, err := store.NewSecretARNSuffix()
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create secret.", readOnly, eventID, verified)
		return
	}
	arn := store.SecretARN(s.secretsRegion(verified), verified.AccountID, name, suffix)
	if !s.authorize(verified, catalog.ActionSecretsCreateSecret, arn) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:CreateSecret.", readOnly, eventID, verified)
		return
	}
	if hasValue {
		encCtx := store.SecretsEncryptionContext(arn, "1")
		if !s.secretsAuthorizeKMS(w, r, body, requestID, eventID, verified, readOnly, verified.AccountID, keyID, catalog.ActionKMSEncrypt, encCtx) {
			return
		}
	}

	sec, err := s.store.CreateSecret(
		verified.AccountID, s.secretsRegion(verified), name, secretString, secretBinary, keyID, description, suffix,
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
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsGetSecretValue, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:GetSecretValue.", readOnly, eventID, verified)
		return
	}
	versionStage, _ := params["VersionStage"].(string)
	versionID, _ := params["VersionId"].(string)
	lookupVersionID := strings.TrimSpace(versionID)
	if lookupVersionID == "" {
		stage := strings.TrimSpace(versionStage)
		if stage != "" && stage != store.SecretVersionStageCurrent {
			for vid, stages := range meta.VersionIdsToStages {
				if secretsStagesContain(stages, stage) {
					lookupVersionID = vid
					break
				}
			}
		}
	}
	if lookupVersionID == "" {
		lookupVersionID = meta.VersionID
	}
	encCtx := store.SecretsEncryptionContext(meta.ARN, lookupVersionID)
	if !s.secretsAuthorizeKMS(w, r, body, requestID, eventID, verified, readOnly, secretAccountID, meta.KmsKeyID, catalog.ActionKMSDecrypt, encCtx) {
		return
	}

	sec, err := s.store.GetSecretValueByStage(secretAccountID, secretID, versionStage, versionID)
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrSecretScheduledDeletion) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"Secret is scheduled for deletion.", readOnly, eventID, verified)
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
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsPutSecretValue, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:PutSecretValue.", readOnly, eventID, verified)
		return
	}
	clientToken, _ := params["ClientRequestToken"].(string)
	clientToken = strings.TrimSpace(clientToken)
	versionStages := secretsStringSliceParam(params["VersionStages"])
	nextVersionID := clientToken
	if nextVersionID == "" {
		nextVersionID = fmt.Sprintf("%d", meta.Version+1)
	}
	encCtx := store.SecretsEncryptionContext(meta.ARN, nextVersionID)
	if !s.secretsAuthorizeKMS(w, r, body, requestID, eventID, verified, readOnly, secretAccountID, meta.KmsKeyID, catalog.ActionKMSEncrypt, encCtx) {
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

	var sec store.Secret
	if len(versionStages) > 0 {
		sec, err = s.store.PutSecretValueWithStages(secretAccountID, secretID, secretString, secretBinary, clientToken, versionStages)
	} else {
		sec, err = s.store.PutSecretValue(secretAccountID, secretID, secretString, secretBinary)
	}
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrSecretScheduledDeletion) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"Secret is scheduled for deletion.", readOnly, eventID, verified)
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
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsDeleteSecret, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:DeleteSecret.", readOnly, eventID, verified)
		return
	}

	force := secretsBoolParam(params["ForceDeleteWithoutRecovery"], false)
	recoveryDays := intFromJSONNumber(params["RecoveryWindowInDays"])
	sec, deletion, err := s.store.DeleteSecretWithRecovery(secretAccountID, secretID, recoveryDays, force)
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete secret.", readOnly, eventID, verified)
		return
	}

	deletionDate := deletion.UTC().Format(time.RFC3339)
	if force {
		deletionDate = time.Now().UTC().Format(time.RFC3339)
	}
	payload, err := sm.DeleteSecretJSON(sec, deletionDate)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "DeleteSecret", readOnly)
}

func (s *Server) secretsRestoreSecret(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsRestoreSecret, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:RestoreSecret.", readOnly, eventID, verified)
		return
	}
	if err := s.store.RestoreSecret(secretAccountID, secretID); errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to restore secret.", readOnly, eventID, verified)
		return
	}
	meta, err := s.store.DescribeSecret(secretAccountID, secretID)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to restore secret.", readOnly, eventID, verified)
		return
	}
	payload, err := sm.RestoreSecretJSON(meta)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "RestoreSecret", readOnly)
}

func (s *Server) checkSecretsManagerPassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("rotation role must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("rotation role must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("rotation role not found")
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to Secrets Manager")
	}
	decision := authz.CheckPassRole(authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal:     verified.Principal,
			Resource:      roleARN,
			Region:        verified.Region,
			ConditionKeys: s.conditionKeys(verified),
		},
		EvalInputs:       in,
		RoleARN:          roleARN,
		TrustPolicyDoc:   trust,
		ServicePrincipal: authz.ServicePrincipalSecretsManager,
		SourceArn:        sourceARN,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Secrets Manager")
	}
	return nil
}

func (s *Server) secretsRotateSecret(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsRotateSecret, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:RotateSecret.", readOnly, eventID, verified)
		return
	}
	clientToken, _ := params["ClientRequestToken"].(string)
	clientToken = strings.TrimSpace(clientToken)
	nextVersionID := clientToken
	if nextVersionID == "" {
		nextVersionID = fmt.Sprintf("%d", meta.Version+1)
	}
	encCtx := store.SecretsEncryptionContext(meta.ARN, nextVersionID)
	if !s.secretsAuthorizeKMS(w, r, body, requestID, eventID, verified, readOnly, secretAccountID, meta.KmsKeyID, catalog.ActionKMSEncrypt, encCtx) {
		return
	}

	rotationLambdaARN, _ := params["RotationLambdaARN"].(string)
	rotationLambdaARN = strings.TrimSpace(rotationLambdaARN)
	rotationRoleARN, _ := params["RotationRoleARN"].(string)
	rotationRoleARN = strings.TrimSpace(rotationRoleARN)
	if rotationLambdaARN != "" || rotationRoleARN != "" {
		persistLambda := rotationLambdaARN
		if persistLambda == "" {
			persistLambda = meta.RotationLambdaARN
		}
		persistRole := rotationRoleARN
		if persistRole == "" {
			persistRole = meta.RotationRoleARN
		}
		if err := s.store.SetSecretRotationConfig(secretAccountID, secretID, persistLambda, persistRole); err != nil {
			s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to configure secret rotation.", readOnly, eventID, verified)
			return
		}
	}

	rotateImmediately := true
	if v, ok := params["RotateImmediately"]; ok {
		switch t := v.(type) {
		case bool:
			rotateImmediately = t
		case string:
			rotateImmediately = !strings.EqualFold(strings.TrimSpace(t), "false")
		}
	}
	rules, hasRules, rulesErr := parseSecretRotationRules(params["RotationRules"])
	if rulesErr != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			rulesErr.Error(), readOnly, eventID, verified)
		return
	}
	now := s.now().UTC()
	if hasRules {
		if err := s.store.SetSecretRotationRules(secretAccountID, secretID, rules, now, rotateImmediately); err != nil {
			if strings.Contains(err.Error(), "ValidationException") {
				s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					err.Error(), readOnly, eventID, verified)
				return
			}
			s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to configure rotation rules.", readOnly, eventID, verified)
			return
		}
		s.StartSecretsRotationTicker()
	}

	described, err := s.store.DescribeSecret(secretAccountID, secretID)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to rotate secret.", readOnly, eventID, verified)
		return
	}

	var sec store.Secret
	if !rotateImmediately {
		// Configure-only: return current AWSCURRENT without rotating (lab deferral).
		sec = described
		if cur, getErr := s.store.GetSecretValue(secretAccountID, secretID); getErr == nil {
			sec.VersionID = cur.VersionID
			sec.Version = cur.Version
		}
	} else if strings.TrimSpace(described.RotationLambdaARN) == "" {
		sec, err = s.store.RotateSecret(secretAccountID, secretID)
	} else {
		sec, err = s.secretsRotateViaLambda(r.Context(), verified, secretAccountID, secretID, described, clientToken)
	}
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrSecretScheduledDeletion) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"Secret is scheduled for deletion.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrSecretRotationLambdaInvalid) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"RotationLambdaARN is invalid or the function was not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrSecretRotationInProgress) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"A previous rotation is still in progress.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "not authorized to pass role") {
			s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to rotate secret.", readOnly, eventID, verified)
		return
	}
	if rotateImmediately && (hasRules || described.RotationEnabled) {
		_ = s.store.MarkSecretRotated(secretAccountID, secretID, now)
	}
	payload, err := sm.RotateSecretJSON(sec)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "RotateSecret", readOnly)
}

func parseSecretRotationRules(raw any) (store.SecretRotationRules, bool, error) {
	if raw == nil {
		return store.SecretRotationRules{}, false, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return store.SecretRotationRules{}, false, fmt.Errorf("ValidationException: RotationRules must be an object")
	}
	var rules store.SecretRotationRules
	switch v := m["AutomaticallyAfterDays"].(type) {
	case float64:
		rules.AutomaticallyAfterDays = int64(v)
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return store.SecretRotationRules{}, false, fmt.Errorf("ValidationException: AutomaticallyAfterDays is invalid")
		}
		rules.AutomaticallyAfterDays = n
	case int:
		rules.AutomaticallyAfterDays = int64(v)
	case int64:
		rules.AutomaticallyAfterDays = v
	}
	if s, ok := m["ScheduleExpression"].(string); ok {
		rules.ScheduleExpression = strings.TrimSpace(s)
	}
	if s, ok := m["Duration"].(string); ok {
		rules.Duration = strings.TrimSpace(s)
	}
	if rules.AutomaticallyAfterDays == 0 && rules.ScheduleExpression == "" && rules.Duration == "" {
		return store.SecretRotationRules{}, false, nil
	}
	if err := store.ValidateSecretRotationRules(rules); err != nil {
		return store.SecretRotationRules{}, false, err
	}
	return rules, true, nil
}

// secretsRotateViaLambda enforces PassRole for secretsmanager.amazonaws.com, then runs
// createSecret → setSecret → testSecret → finishSecret invokes. AWSPENDING is created
// before the first invoke; finishSecret promote runs only after all four succeed.
func (s *Server) secretsRotateViaLambda(
	ctx context.Context,
	verified *authn.Verified,
	secretAccountID, secretID string,
	meta store.Secret,
	clientRequestToken string,
) (store.Secret, error) {
	fnAccount, functionName, qualifier, passRoleARN, err := s.store.ResolveSecretRotationLambda(secretAccountID, secretID)
	if err != nil {
		return store.Secret{}, err
	}
	if functionName == "" || passRoleARN == "" {
		return store.Secret{}, store.ErrSecretRotationLambdaInvalid
	}
	if err := s.checkSecretsManagerPassRole(verified, passRoleARN, meta.ARN); err != nil {
		return store.Secret{}, err
	}
	return s.store.RotateSecretFourStep(secretAccountID, secretID, clientRequestToken, func(eventJSON string) error {
		job, err := s.store.EnqueueAsyncInvokeQuiet(fnAccount, functionName, qualifier, eventJSON)
		if err != nil {
			return err
		}
		if err := s.store.ProcessAsyncInvocation(job.InvocationID, 0, func() error {
			fn, resolvedVersion, resolveErr := s.store.ResolveFunction(fnAccount, functionName, qualifier)
			if resolveErr != nil {
				return resolveErr
			}
			_, execErr := s.executeLambdaInvoke(ctx, fnAccount, functionName, fn, resolvedVersion, job.EventJSON)
			return execErr
		}); err != nil {
			return fmt.Errorf("rotation lambda invoke failed: %w", err)
		}
		done, err := s.store.GetAsyncInvocation(job.InvocationID)
		if err != nil {
			return err
		}
		if done.Status != "succeeded" {
			msg := strings.TrimSpace(done.LastError)
			if msg == "" {
				msg = "rotation lambda invoke failed"
			}
			return fmt.Errorf("%s", msg)
		}
		return nil
	})
}

func (s *Server) secretsUpdateSecretVersionStage(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsUpdateSecretVersionStage, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:UpdateSecretVersionStage.", readOnly, eventID, verified)
		return
	}
	stage, _ := params["VersionStage"].(string)
	moveTo, _ := params["MoveToVersionId"].(string)
	removeFrom, _ := params["RemoveFromVersionId"].(string)
	if err := s.store.UpdateSecretVersionStage(secretAccountID, secretID, moveTo, removeFrom, stage); err != nil {
		if errors.Is(err, store.ErrSecretNotFound) {
			s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
				"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
			return
		}
		if errors.Is(err, store.ErrSecretScheduledDeletion) {
			s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
				"Secret is scheduled for deletion.", readOnly, eventID, verified)
			return
		}
		if strings.Contains(err.Error(), "ValidationException") {
			s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update secret version stage.", readOnly, eventID, verified)
		return
	}
	described, err := s.store.DescribeSecret(secretAccountID, secretID)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe secret after stage update.", readOnly, eventID, verified)
		return
	}
	payload, err := sm.UpdateSecretVersionStageJSON(described)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "UpdateSecretVersionStage", readOnly)
}

func secretsStringSliceParam(v any) []string {
	switch t := v.(type) {
	case []string:
		return append([]string{}, t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func secretsStagesContain(stages []string, want string) bool {
	for _, s := range stages {
		if s == want {
			return true
		}
	}
	return false
}

func secretsBoolParam(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
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
	meta, _, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
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
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
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
	if err := authz.ValidateResourcePolicyDocument(policy); err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "MalformedPolicyDocumentException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.store.PutSecretResourcePolicy(secretAccountID, secretID, policy); err != nil {
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
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsGetResourcePolicy, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:GetResourcePolicy.", readOnly, eventID, verified)
		return
	}
	policy, err := s.store.GetSecretResourcePolicy(secretAccountID, secretID)
	if errors.Is(err, store.ErrSecretNotFound) {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Secrets Manager can't find the specified secret.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrNoSuchResourcePolicy) {
		// Terraform aws_secretsmanager_secret treats policy ResourceNotFoundException as
		// a hard miss ("couldn't find resource"). Return success with empty policy instead.
		payload, buildErr := sm.GetResourcePolicyJSON(meta, "")
		if buildErr != nil {
			s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build response.", readOnly, eventID, verified)
			return
		}
		s.writeSecretsOK(w, requestID, payload)
		s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "GetResourcePolicy", readOnly)
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
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsDeleteResourcePolicy, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:DeleteResourcePolicy.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteSecretResourcePolicy(secretAccountID, secretID); errors.Is(err, store.ErrSecretNotFound) {
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

func (s *Server) secretsListTagsForResource(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsListTagsForResource, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:ListTagsForResource.", readOnly, eventID, verified)
		return
	}
	tags, err := s.store.ListResourceTags(secretAccountID, meta.ARN)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list tags.", readOnly, eventID, verified)
		return
	}
	payload, err := sm.ListTagsForResourceJSON(tags)
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "ListTagsForResource", readOnly)
}

func (s *Server) secretsTagResource(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	tags := parseKeyValueTags(params["Tags"])
	if len(tags) == 0 {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Tags is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsTagResource, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:TagResource.", readOnly, eventID, verified)
		return
	}
	if _, err := s.store.TagResources(secretAccountID, []string{meta.ARN}, tags); err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to tag secret.", readOnly, eventID, verified)
		return
	}
	payload, err := sm.EmptyOKJSON()
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "TagResource", readOnly)
}

func (s *Server) secretsUntagResource(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	secretID := secretsSecretID(params)
	meta, secretAccountID, ok := s.secretMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, secretID)
	if !ok {
		return
	}
	keys := stringSliceParam(params["TagKeys"])
	if len(keys) == 0 {
		s.writeSecretsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TagKeys is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeSecretsManager(verified, catalog.ActionSecretsUntagResource, meta.ARN, meta.ResourcePolicy) {
		s.writeSecretsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform secretsmanager:UntagResource.", readOnly, eventID, verified)
		return
	}
	if _, err := s.store.UntagResources(secretAccountID, []string{meta.ARN}, keys); err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to untag secret.", readOnly, eventID, verified)
		return
	}
	payload, err := sm.EmptyOKJSON()
	if err != nil {
		s.writeSecretsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSecretsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, secretsEventSource, "UntagResource", readOnly)
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
