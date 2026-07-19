package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

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
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
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
				s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
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
				s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
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
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
	}

	if needsKey && keyID != "" {
		key, err = s.store.GetKey(keyID)
		if err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		if key.AccountID != accountID {
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		keyPolicy = key.KeyPolicy
		resource = key.ARN
	}

	identityDocs := s.identityDocs(verified.Principal)
	grantSatisfied := false
	if keyID != "" {
		ok, gerr := s.store.FindMatchingGrant(keyID, verified.Principal.ARN(), action)
		if gerr == nil {
			grantSatisfied = ok
		}
	}

	decision := authz.EvaluateKMS(authz.KMSRequest{
		Caller: authz.RequestContext{
			Principal: verified.Principal,
			Action:    normalizeAction(action),
			Resource:  resource,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount": verified.AccountID,
				"aws:RequestedRegion":  verified.Region,
			},
		},
		IdentityDocs:   identityDocs,
		KeyPolicyDoc:   keyPolicy,
		GrantSatisfied: grantSatisfied,
	})
	if decision != authz.Allow {
		s.writeKMSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform "+normalizeAction(action)+".", readOnly, eventID, verified)
		return
	}

	// Session policy intersection when present.
	if sessionDocs := s.sessionPolicyDocs(verified.AccessKeyID); len(sessionDocs) > 0 {
		if authz.EvaluateWithSession(authz.RequestContext{
			Principal: verified.Principal,
			Action:    normalizeAction(action),
			Resource:  resource,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount": verified.AccountID,
				"aws:RequestedRegion":  verified.Region,
			},
		}, identityDocs, sessionDocs) != authz.Allow {
			s.writeKMSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform "+normalizeAction(action)+".", readOnly, eventID, verified)
			return
		}
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
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSDisableKey, "DisableKey":
		if err := s.store.SetKeyState(keyID, store.KeyStateDisabled); err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSGetKeyPolicy, "GetKeyPolicy":
		policy, getErr := s.store.GetKeyPolicy(keyID)
		if getErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
				"Key not found.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.GetKeyPolicyJSON(policy)
	case catalog.ActionKMSPutKeyPolicy, "PutKeyPolicy":
		policy, _ := params["Policy"].(string)
		if err := s.store.PutKeyPolicy(keyID, policy); err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Unable to put key policy.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.EmptyOKJSON()
	case catalog.ActionKMSEncrypt, "Encrypt":
		if key.KeyState != store.KeyStateEnabled {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "DisabledException",
				"Key is disabled.", readOnly, eventID, verified)
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
		ct, encErr := kmssvc.EncryptUnderCMK(cmk, keyID, plain)
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
		if key.KeyState != store.KeyStateEnabled {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "DisabledException",
				"Key is disabled.", readOnly, eventID, verified)
			return
		}
		cmk, unsealErr := s.store.UnsealKeyMaterial(keyID)
		if unsealErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to load key material.", readOnly, eventID, verified)
			return
		}
		plain, openErr := kmssvc.DecryptUnderCMK(cmk, blob)
		if openErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "InvalidCiphertextException",
				"Unable to decrypt ciphertext.", readOnly, eventID, verified)
			return
		}
		payload, err = kmssvc.DecryptJSON(key.ARN, plain)
	case catalog.ActionKMSGenerateDataKey, "GenerateDataKey",
		catalog.ActionKMSGenerateDataKeyWithoutPlaintext, "GenerateDataKeyWithoutPlaintext":
		if key.KeyState != store.KeyStateEnabled {
			s.writeKMSError(w, r, body, requestID, http.StatusBadRequest, "DisabledException",
				"Key is disabled.", readOnly, eventID, verified)
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
		ct, encErr := kmssvc.EncryptUnderCMK(cmk, keyID, dek)
		if encErr != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to encrypt data key.", readOnly, eventID, verified)
			return
		}
		includePlain := action == catalog.ActionKMSGenerateDataKey || action == "GenerateDataKey"
		payload, err = kmssvc.GenerateDataKeyJSON(key.ARN, dek, ct, includePlain)
	case catalog.ActionKMSCreateGrant, "CreateGrant":
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
				s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
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
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
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
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
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
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
				"Target key not found.", readOnly, eventID, verified)
			return
		}
		if err := s.store.UpdateAlias(accountID, aliasParam, target); err != nil {
			s.writeKMSError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
				"Alias not found.", readOnly, eventID, verified)
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
