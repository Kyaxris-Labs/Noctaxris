package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	kmssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kms"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func (s *Server) handleKMS(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	accountID := verified.AccountID

	if action == "ReEncrypt" {
		s.handleKMSReEncrypt(w, r, body, requestID, eventID, params, verified, readOnly)
		return
	}

	resource := "*"
	var (
		key       store.Key
		keyID     string
		keyPolicy string
		needsKey  bool
		err       error
	)

	keyParam, _ := params["KeyId"].(string)
	aliasParam, _ := params["AliasName"].(string)

	switch action {
	case catalog.ActionKMSCreateKey, "CreateKey",
		catalog.ActionKMSListKeys, "ListKeys",
		catalog.ActionKMSListAliases, "ListAliases":
		needsKey = false
		resource = "*"
	case catalog.ActionKMSCreateAlias, "CreateAlias":
		needsKey = true
		targetID, _ := params["TargetKeyId"].(string)
		if targetID == "" {
			targetID = keyParam
		}
		if targetID == "" {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"TargetKeyId is required.", readOnly, eventID, verified)
			return
		}
		keyID, err = s.store.ResolveKeyID(accountID, targetID)
		if err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
	case catalog.ActionKMSDeleteAlias, "DeleteAlias",
		catalog.ActionKMSUpdateAlias, "UpdateAlias":
		needsKey = false
		resource = "*"
	case catalog.ActionKMSDecrypt, "Decrypt":
		needsKey = true
		if keyParam != "" {
			keyID, err = s.store.ResolveKeyID(accountID, keyParam)
			if err != nil {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
					"Key not found.", readOnly, eventID, verified)
				return
			}
		} else {
			blob, decErr := kmssvc.DecodeBinaryField(params["CiphertextBlob"])
			if decErr != nil || len(blob) == 0 {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"CiphertextBlob is required.", readOnly, eventID, verified)
				return
			}
			embedded, kidErr := kmssvc.KeyIDFromCiphertext(blob)
			if kidErr != nil {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"KeyId is required.", readOnly, eventID, verified)
				return
			}
			keyID, err = s.store.ResolveKeyID(accountID, embedded)
			if err != nil {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
					"Key not found.", readOnly, eventID, verified)
				return
			}
		}
	case catalog.ActionKMSRetireGrant, "RetireGrant",
		catalog.ActionKMSRevokeGrant, "RevokeGrant":
		needsKey = false
		if keyParam != "" {
			keyID, err = s.store.ResolveKeyID(accountID, keyParam)
			if err == nil {
				needsKey = true
			}
		}
	default:
		needsKey = true
		if keyParam == "" {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"KeyId is required.", readOnly, eventID, verified)
			return
		}
		keyID, err = s.store.ResolveKeyID(accountID, keyParam)
		if err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
	}

	if needsKey && keyID != "" {
		key, err = s.store.GetKey(keyID)
		if err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		keyPolicy = key.KeyPolicy
		resource = key.ARN
	}

	encCtx, encCtxErr := kmssvc.ParseEncryptionContext(params["EncryptionContext"])
	if encCtxErr != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			encCtxErr.Error(), readOnly, eventID, verified)
		return
	}

	grantSatisfied := false
	if keyID != "" {
		ok, gerr := s.store.FindMatchingGrant(keyID, verified.Principal.ARN(), action)
		if gerr == nil {
			grantSatisfied = ok
		}
	}

	if !s.authorizeDataplaneKMS(verified, normalizeAction(action), resource, keyPolicy, grantSatisfied, encCtx) {
		s.writeKMSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform "+normalizeAction(action)+".", readOnly, eventID, verified)
		return
	}

	var payload []byte
	switch action {
	case catalog.ActionKMSCreateKey, "CreateKey":
		policyOverride, _ := params["Policy"].(string)
		k, createErr := s.store.CreateKey(accountID, verified.Principal.ARN(), policyOverride)
		if createErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to create key.", readOnly, eventID, verified)
			return
		}
		if tags := parseKMSTags(params["Tags"]); len(tags) > 0 {
			if _, tagErr := s.store.TagResources(accountID, []string{k.ARN}, tags); tagErr != nil {
				s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
					"Unable to tag key.", readOnly, eventID, verified)
				return
			}
		}
		payload, err = kmssvc.CreateKeyJSON(k)
	case catalog.ActionKMSDescribeKey, "DescribeKey":
		payload, err = kmssvc.DescribeKeyJSON(key)
	case catalog.ActionKMSListKeys, "ListKeys":
		keys, listErr := s.store.ListKeys(accountID)
		if listErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list keys.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.ListKeysJSON(keys)
	case catalog.ActionKMSEnableKey, "EnableKey":
		if err := s.store.SetKeyState(keyID, store.KeyStateEnabled); err != nil {
			if errors.Is(err, store.ErrInvalidKeyState) {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "KMSInvalidStateException",
					"Key is pending deletion.", readOnly, eventID, verified)
				return
			}
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSDisableKey, "DisableKey":
		if err := s.store.SetKeyState(keyID, store.KeyStateDisabled); err != nil {
			if errors.Is(err, store.ErrInvalidKeyState) {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "KMSInvalidStateException",
					"Key is pending deletion.", readOnly, eventID, verified)
				return
			}
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSScheduleKeyDeletion, "ScheduleKeyDeletion":
		window := intFromJSONNumber(params["PendingWindowInDays"])
		scheduled, schedErr := s.store.ScheduleKeyDeletion(keyID, window)
		if schedErr != nil {
			if errors.Is(schedErr, store.ErrInvalidKeyState) {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "KMSInvalidStateException",
					"Key is not in a valid state for deletion.", readOnly, eventID, verified)
				return
			}
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.ScheduleKeyDeletionJSON(scheduled)
	case catalog.ActionKMSCancelKeyDeletion, "CancelKeyDeletion":
		if err := s.store.CancelKeyDeletion(keyID); err != nil {
			if errors.Is(err, store.ErrInvalidKeyState) {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "KMSInvalidStateException",
					"Key is not pending deletion.", readOnly, eventID, verified)
				return
			}
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.CancelKeyDeletionJSON(key.ARN)
	case catalog.ActionKMSEnableKeyRotation, "EnableKeyRotation":
		if err := s.store.SetKeyRotationEnabled(keyID, true); err != nil {
			if errors.Is(err, store.ErrUnsupportedKeyRotation) {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "UnsupportedOperationException",
					"Key rotation is supported only for symmetric ENCRYPT_DECRYPT keys.", readOnly, eventID, verified)
				return
			}
			if errors.Is(err, store.ErrInvalidKeyState) {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "KMSInvalidStateException",
					"Key is not in a valid state for rotation.", readOnly, eventID, verified)
				return
			}
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSDisableKeyRotation, "DisableKeyRotation":
		if err := s.store.SetKeyRotationEnabled(keyID, false); err != nil {
			if errors.Is(err, store.ErrUnsupportedKeyRotation) {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "UnsupportedOperationException",
					"Key rotation is supported only for symmetric ENCRYPT_DECRYPT keys.", readOnly, eventID, verified)
				return
			}
			if errors.Is(err, store.ErrInvalidKeyState) {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "KMSInvalidStateException",
					"Key is not in a valid state for rotation.", readOnly, eventID, verified)
				return
			}
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSGetKeyRotationStatus, "GetKeyRotationStatus":
		enabled, rotErr := s.store.GetKeyRotationEnabled(keyID)
		if rotErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.GetKeyRotationStatusJSON(enabled)
	case catalog.ActionKMSGetKeyPolicy, "GetKeyPolicy":
		policy, getErr := s.store.GetKeyPolicy(keyID)
		if getErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.GetKeyPolicyJSON(policy)
	case catalog.ActionKMSPutKeyPolicy, "PutKeyPolicy":
		policy, _ := params["Policy"].(string)
		if err := authz.ValidateResourcePolicyDocument(policy); err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "MalformedPolicyDocumentException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		if err := s.store.PutKeyPolicy(keyID, policy); err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Unable to put key policy.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSEncrypt, "Encrypt":
		if !store.KeyUsableForCrypto(key.KeyState) {
			s.writeKMSCryptoStateError(w, r, body, requestID, key.KeyState, readOnly, eventID, verified)
			return
		}
		if rotErr := s.store.MaybeAutoRotate(keyID, time.Time{}); rotErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to auto-rotate key material.", readOnly, eventID, verified)
			return
		}
		plain, decErr := kmssvc.DecodeBinaryField(params["Plaintext"])
		if decErr != nil || len(plain) == 0 {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Plaintext is required.", readOnly, eventID, verified)
			return
		}
		cmk, unsealErr := s.store.UnsealKeyMaterial(keyID)
		if unsealErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to load key material.", readOnly, eventID, verified)
			return
		}
		ct, encErr := kmssvc.EncryptUnderCMK(cmk, keyID, plain, encCtx)
		if encErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to encrypt.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EncryptJSON(key.ARN, ct)
	case catalog.ActionKMSDecrypt, "Decrypt":
		blob, decErr := kmssvc.DecodeBinaryField(params["CiphertextBlob"])
		if decErr != nil || len(blob) == 0 {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"CiphertextBlob is required.", readOnly, eventID, verified)
			return
		}
		if !store.KeyUsableForCrypto(key.KeyState) {
			s.writeKMSCryptoStateError(w, r, body, requestID, key.KeyState, readOnly, eventID, verified)
			return
		}
		plain, openErr := s.store.DecryptBlobWithKeyContext(keyID, blob, encCtx)
		if openErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "InvalidCiphertextException",
				"Unable to decrypt ciphertext.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.DecryptJSON(key.ARN, plain)
	case catalog.ActionKMSGenerateDataKey, "GenerateDataKey",
		catalog.ActionKMSGenerateDataKeyWithoutPlaintext, "GenerateDataKeyWithoutPlaintext":
		if !store.KeyUsableForCrypto(key.KeyState) {
			s.writeKMSCryptoStateError(w, r, body, requestID, key.KeyState, readOnly, eventID, verified)
			return
		}
		if rotErr := s.store.MaybeAutoRotate(keyID, time.Time{}); rotErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to auto-rotate key material.", readOnly, eventID, verified)
			return
		}
		dek := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, dek); err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to generate data key.", readOnly, eventID, verified)
			return
		}
		cmk, unsealErr := s.store.UnsealKeyMaterial(keyID)
		if unsealErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to load key material.", readOnly, eventID, verified)
			return
		}
		ct, encErr := kmssvc.EncryptUnderCMK(cmk, keyID, dek, encCtx)
		if encErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to encrypt data key.", readOnly, eventID, verified)
			return
		}
		includePlain := action == catalog.ActionKMSGenerateDataKey || action == "GenerateDataKey"
		payload, err = kmssvc.GenerateDataKeyJSON(key.ARN, dek, ct, includePlain)
	case catalog.ActionKMSCreateGrant, "CreateGrant":
		if !store.KeyUsableForCrypto(key.KeyState) {
			s.writeKMSCryptoStateError(w, r, body, requestID, key.KeyState, readOnly, eventID, verified)
			return
		}
		grantee, _ := params["GranteePrincipal"].(string)
		retiring, _ := params["RetiringPrincipal"].(string)
		name, _ := params["Name"].(string)
		ops := stringSliceParam(params["Operations"])
		g, createErr := s.store.CreateGrant(accountID, keyID, grantee, retiring, ops, name)
		if createErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Unable to create grant.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.CreateGrantJSON(g)
	case catalog.ActionKMSListGrants, "ListGrants":
		grants, listErr := s.store.ListGrants(keyID)
		if listErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list grants.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.ListGrantsJSON(grants)
	case catalog.ActionKMSRetireGrant, "RetireGrant":
		grantID, _ := params["GrantId"].(string)
		if err := s.store.RetireGrant(grantID, verified.Principal.ARN()); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
					"Grant not found.", readOnly, eventID, verified)
				return
			}
			s.writeKMSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"Unable to retire grant.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSRevokeGrant, "RevokeGrant":
		grantID, _ := params["GrantId"].(string)
		if err := s.store.RevokeGrant(grantID); err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Grant not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSCreateAlias, "CreateAlias":
		if aliasParam == "" {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"AliasName is required.", readOnly, eventID, verified)
			return
		}
		if err := s.store.CreateAlias(accountID, aliasParam, keyID); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") ||
				strings.Contains(strings.ToLower(err.Error()), "constraint") {
				s.writeKMSError(w, r, body, requestID, http.StatusConflict, "AlreadyExistsException",
					"Alias already exists.", readOnly, eventID, verified)
				return
			}
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Unable to create alias.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSListAliases, "ListAliases":
		aliases, listErr := s.store.ListAliases(accountID)
		if listErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list aliases.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.ListAliasesJSON(aliases, accountID)
	case catalog.ActionKMSDeleteAlias, "DeleteAlias":
		if aliasParam == "" {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"AliasName is required.", readOnly, eventID, verified)
			return
		}
		if err := s.store.DeleteAlias(accountID, aliasParam); err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Alias not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSUpdateAlias, "UpdateAlias":
		if aliasParam == "" {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"AliasName is required.", readOnly, eventID, verified)
			return
		}
		targetParam, _ := params["TargetKeyId"].(string)
		if targetParam == "" {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"TargetKeyId is required.", readOnly, eventID, verified)
			return
		}
		target, resolveErr := s.store.ResolveKeyID(accountID, targetParam)
		if resolveErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Target key not found.", readOnly, eventID, verified)
			return
		}
		if err := s.store.UpdateAlias(accountID, aliasParam, target); err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Alias not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSListResourceTags, "ListResourceTags":
		tags, listErr := s.store.ListResourceTags(accountID, key.ARN)
		if listErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list tags.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.ListResourceTagsJSON(tags)
	case catalog.ActionKMSTagResource, "TagResource":
		tags := parseKMSTags(params["Tags"])
		if len(tags) == 0 {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Tags is required.", readOnly, eventID, verified)
			return
		}
		if _, tagErr := s.store.TagResources(accountID, []string{key.ARN}, tags); tagErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to tag key.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSUntagResource, "UntagResource":
		keys := parseKMSTagKeys(params["TagKeys"])
		if len(keys) == 0 {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"TagKeys is required.", readOnly, eventID, verified)
			return
		}
		if _, untagErr := s.store.UntagResources(accountID, []string{key.ARN}, keys); untagErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to untag key.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	default:
		s.writeKMSError(w, r, body, requestID, http.StatusNotImplemented, "NotImplemented",
			"This KMS action is not implemented.", readOnly, eventID, verified)
		return
	}

	if err != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeJSONOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "kms.amazonaws.com", eventNameForRequest(r), readOnly)
}

func (s *Server) handleKMSReEncrypt(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	params map[string]any,
	verified *authn.Verified,
	readOnly bool,
) {
	accountID := verified.AccountID

	destParam, _ := params["DestinationKeyId"].(string)
	if strings.TrimSpace(destParam) == "" {
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"DestinationKeyId is required.", readOnly, eventID, verified)
		return
	}

	blob, decErr := kmssvc.DecodeBinaryField(params["CiphertextBlob"])
	if decErr != nil || len(blob) == 0 {
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"CiphertextBlob is required.", readOnly, eventID, verified)
		return
	}

	sourceParam, _ := params["SourceKeyId"].(string)
	var (
		sourceKeyID string
		err         error
	)
	if strings.TrimSpace(sourceParam) != "" {
		sourceKeyID, err = s.store.ResolveKeyID(accountID, sourceParam)
		if err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Source key not found.", readOnly, eventID, verified)
			return
		}
		if embedded, kidErr := kmssvc.KeyIDFromCiphertext(blob); kidErr == nil && embedded != "" {
			embeddedID, resolveErr := s.store.ResolveKeyID(accountID, embedded)
			if resolveErr == nil && embeddedID != sourceKeyID {
				s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "IncorrectKeyException",
					"The key ID in CiphertextBlob does not match SourceKeyId.", readOnly, eventID, verified)
				return
			}
		}
	} else {
		embedded, kidErr := kmssvc.KeyIDFromCiphertext(blob)
		if kidErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"SourceKeyId is required.", readOnly, eventID, verified)
			return
		}
		sourceKeyID, err = s.store.ResolveKeyID(accountID, embedded)
		if err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
				"Source key not found.", readOnly, eventID, verified)
			return
		}
	}

	destKeyID, err := s.store.ResolveKeyID(accountID, destParam)
	if err != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Destination key not found.", readOnly, eventID, verified)
		return
	}

	sourceKey, err := s.store.GetKey(sourceKeyID)
	if err != nil || sourceKey.AccountID != accountID {
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Source key not found.", readOnly, eventID, verified)
		return
	}
	destKey, err := s.store.GetKey(destKeyID)
	if err != nil || destKey.AccountID != accountID {
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Destination key not found.", readOnly, eventID, verified)
		return
	}

	srcEncCtx, srcCtxErr := kmssvc.ParseEncryptionContext(params["SourceEncryptionContext"])
	if srcCtxErr != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			srcCtxErr.Error(), readOnly, eventID, verified)
		return
	}
	if srcEncCtx == nil {
		// AWS accepts EncryptionContext as an alias for source context.
		srcEncCtx, srcCtxErr = kmssvc.ParseEncryptionContext(params["EncryptionContext"])
		if srcCtxErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				srcCtxErr.Error(), readOnly, eventID, verified)
			return
		}
	}
	destEncCtx, destCtxErr := kmssvc.ParseEncryptionContext(params["DestinationEncryptionContext"])
	if destCtxErr != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			destCtxErr.Error(), readOnly, eventID, verified)
		return
	}
	if destEncCtx == nil {
		destEncCtx = srcEncCtx
	}

	grantFrom := false
	if ok, gerr := s.store.FindMatchingGrant(sourceKeyID, verified.Principal.ARN(), catalog.ActionKMSReEncryptFrom); gerr == nil {
		grantFrom = ok
	}
	grantTo := false
	if ok, gerr := s.store.FindMatchingGrant(destKeyID, verified.Principal.ARN(), catalog.ActionKMSReEncryptTo); gerr == nil {
		grantTo = ok
	}

	if !s.authorizeDataplaneKMS(verified, catalog.ActionKMSReEncryptFrom, sourceKey.ARN, sourceKey.KeyPolicy, grantFrom, srcEncCtx) {
		s.writeKMSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform "+catalog.ActionKMSReEncryptFrom+".", readOnly, eventID, verified)
		return
	}
	if !s.authorizeDataplaneKMS(verified, catalog.ActionKMSReEncryptTo, destKey.ARN, destKey.KeyPolicy, grantTo, destEncCtx) {
		s.writeKMSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform "+catalog.ActionKMSReEncryptTo+".", readOnly, eventID, verified)
		return
	}

	if !store.KeyUsableForCrypto(sourceKey.KeyState) {
		s.writeKMSCryptoStateError(w, r, body, requestID, sourceKey.KeyState, readOnly, eventID, verified)
		return
	}
	if !store.KeyUsableForCrypto(destKey.KeyState) {
		s.writeKMSCryptoStateError(w, r, body, requestID, destKey.KeyState, readOnly, eventID, verified)
		return
	}

	plain, openErr := s.store.DecryptBlobWithKeyContext(sourceKeyID, blob, srcEncCtx)
	if openErr != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "InvalidCiphertextException",
			"Unable to decrypt ciphertext.", readOnly, eventID, verified)
		return
	}

	if rotErr := s.store.MaybeAutoRotate(destKeyID, time.Time{}); rotErr != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to auto-rotate key material.", readOnly, eventID, verified)
		return
	}
	destCMK, unsealErr := s.store.UnsealKeyMaterial(destKeyID)
	if unsealErr != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load destination key material.", readOnly, eventID, verified)
		return
	}
	ct, encErr := kmssvc.EncryptUnderCMK(destCMK, destKeyID, plain, destEncCtx)
	if encErr != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to encrypt.", readOnly, eventID, verified)
		return
	}

	payload, err := kmssvc.ReEncryptJSON(sourceKey.ARN, destKey.ARN, ct)
	if err != nil {
		s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeJSONOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "kms.amazonaws.com", "ReEncrypt", readOnly)
}

func jsonBodyMap(body []byte) map[string]any {
	out := map[string]any{}
	if len(body) == 0 {
		return out
	}
	_ = json.Unmarshal(body, &out)
	return out
}

func stringSliceParam(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	default:
		return nil
	}
}

// parseStringMapTags parses SQS/Lambda Tags maps ({"k":"v"}).
func parseStringMapTags(v any) map[string]string {
	return stringMapParam(v)
}

// parseKeyValueTags parses Events/SSM/Secrets Tags arrays ([{Key,Value},...]).
func parseKeyValueTags(v any) map[string]string {
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		k, _ := m["Key"].(string)
		val, _ := m["Value"].(string)
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resourceTagsToMap(tags []store.ResourceTag) map[string]string {
	out := make(map[string]string, len(tags))
	for _, t := range tags {
		out[t.Key] = t.Value
	}
	return out
}

func intFromJSONNumber(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	default:
		return 0
	}
}

func (s *Server) writeKMSCryptoStateError(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID string,
	keyState string,
	readOnly bool,
	eventID string,
	verified *authn.Verified,
) {
	switch keyState {
	case store.KeyStatePendingDeletion:
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "KMSInvalidStateException",
			"Key is pending deletion.", readOnly, eventID, verified)
	default:
		s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "DisabledException",
			"Key is disabled.", readOnly, eventID, verified)
	}
}

func (s *Server) writeKMSError(
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
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
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

func parseKMSTags(v any) map[string]string {
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		k, _ := m["TagKey"].(string)
		val, _ := m["TagValue"].(string)
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseKMSTagKeys(v any) []string {
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		switch t := item.(type) {
		case string:
			if k := strings.TrimSpace(t); k != "" {
				out = append(out, k)
			}
		}
	}
	return out
}
