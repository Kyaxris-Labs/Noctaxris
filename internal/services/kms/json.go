package kms

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	ciphertextVersionV2 byte = 2
)

// EncryptUnderCMK seals plaintext with AES-256-GCM under cmk material.
// encryptionContext is bound as GCM AAD (AWS EncryptionContext semantics).
func EncryptUnderCMK(cmk []byte, keyID string, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
	return store.EncryptUnderCMK(cmk, keyID, plaintext, encryptionContext)
}

// KeyIDFromCiphertext extracts the embedded key id from a v2 ciphertext blob.
func KeyIDFromCiphertext(blob []byte) (string, error) {
	if len(blob) < 2 || blob[0] != ciphertextVersionV2 {
		return "", fmt.Errorf("ciphertext has no embedded key id")
	}
	n := int(blob[1])
	if n <= 0 || len(blob) < 2+n {
		return "", fmt.Errorf("ciphertext key id truncated")
	}
	return string(blob[2 : 2+n]), nil
}

// DecryptUnderCMK opens a blob produced by EncryptUnderCMK (v1 or v2).
// encryptionContext must match the map supplied at encrypt time (AAD).
func DecryptUnderCMK(cmk, blob []byte, encryptionContext map[string]string) ([]byte, error) {
	return store.DecryptUnderCMK(cmk, blob, encryptionContext)
}

// ParseEncryptionContext reads a KMS EncryptionContext map from JSON params.
// Unsupported value shapes return an error (fail closed).
func ParseEncryptionContext(v any) (map[string]string, error) {
	if v == nil {
		return nil, nil
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("EncryptionContext must be a string map")
	}
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(raw))
	for k, val := range raw {
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("EncryptionContext values must be strings")
		}
		if k == "" {
			return nil, fmt.Errorf("EncryptionContext keys must be non-empty")
		}
		out[k] = s
	}
	return out, nil
}

func b64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func keyMetadata(k store.Key) map[string]any {
	meta := map[string]any{
		"AWSAccountId": k.AccountID,
		"KeyId":        k.KeyID,
		"Arn":          k.ARN,
		"CreationDate": parseCreationFloat(k.CreationDate),
		"Enabled":      k.KeyState == store.KeyStateEnabled,
		"KeyState":     k.KeyState,
		"KeyUsage":     k.KeyUsage,
		"KeyManager":   "CUSTOMER",
		"Origin":       "AWS_KMS",
	}
	if k.DeletionDate != "" {
		meta["DeletionDate"] = parseCreationFloat(k.DeletionDate)
	}
	return meta
}

func parseCreationFloat(rfc3339 string) float64 {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return 0
	}
	return float64(t.Unix())
}

func CreateKeyJSON(k store.Key) ([]byte, error) {
	return json.Marshal(map[string]any{"KeyMetadata": keyMetadata(k)})
}

func DescribeKeyJSON(k store.Key) ([]byte, error) {
	return json.Marshal(map[string]any{"KeyMetadata": keyMetadata(k)})
}

func ListKeysJSON(keys []store.Key) ([]byte, error) {
	entries := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		entries = append(entries, map[string]string{
			"KeyId":  k.KeyID,
			"KeyArn": k.ARN,
		})
	}
	return json.Marshal(map[string]any{"Keys": entries})
}

func EmptyOKJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// ListResourceTagsJSON builds a ListResourceTags success body.
func ListResourceTagsJSON(tags []store.ResourceTag) ([]byte, error) {
	entries := make([]map[string]string, 0, len(tags))
	for _, t := range tags {
		entries = append(entries, map[string]string{
			"TagKey":   t.Key,
			"TagValue": t.Value,
		})
	}
	return json.Marshal(map[string]any{
		"Tags":      entries,
		"Truncated": false,
	})
}

func GetKeyPolicyJSON(policy string) ([]byte, error) {
	return json.Marshal(map[string]string{"Policy": policy})
}

func EncryptJSON(keyID string, ciphertext []byte) ([]byte, error) {
	return json.Marshal(map[string]any{
		"CiphertextBlob": b64(ciphertext),
		"KeyId":          keyID,
	})
}

func ReEncryptJSON(sourceKeyARN, destKeyARN string, ciphertext []byte) ([]byte, error) {
	return json.Marshal(map[string]any{
		"CiphertextBlob":                 b64(ciphertext),
		"SourceKeyId":                    sourceKeyARN,
		"KeyId":                          destKeyARN,
		"SourceEncryptionAlgorithm":      "SYMMETRIC_DEFAULT",
		"DestinationEncryptionAlgorithm": "SYMMETRIC_DEFAULT",
	})
}

func DecryptJSON(keyID string, plaintext []byte) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Plaintext": b64(plaintext),
		"KeyId":     keyID,
	})
}

func GenerateDataKeyJSON(keyID string, plaintext, ciphertext []byte, includePlaintext bool) ([]byte, error) {
	out := map[string]any{
		"CiphertextBlob": b64(ciphertext),
		"KeyId":          keyID,
	}
	if includePlaintext {
		out["Plaintext"] = b64(plaintext)
	}
	return json.Marshal(out)
}

func CreateGrantJSON(g store.Grant) ([]byte, error) {
	return json.Marshal(map[string]any{
		"GrantId":    g.GrantID,
		"GrantToken": g.GrantID,
	})
}

func ListGrantsJSON(grants []store.Grant) ([]byte, error) {
	entries := make([]map[string]any, 0, len(grants))
	for _, g := range grants {
		entry := map[string]any{
			"KeyId":            g.KeyID,
			"GrantId":          g.GrantID,
			"GranteePrincipal": g.GranteePrincipal,
			"Operations":       g.Operations,
		}
		if g.RetiringPrincipal != "" {
			entry["RetiringPrincipal"] = g.RetiringPrincipal
		}
		if g.Name != "" {
			entry["Name"] = g.Name
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]any{"Grants": entries})
}

func ListAliasesJSON(aliases []store.Alias, accountID string) ([]byte, error) {
	entries := make([]map[string]string, 0, len(aliases))
	for _, a := range aliases {
		entries = append(entries, map[string]string{
			"AliasName":   a.AliasName,
			"AliasArn":    store.AliasARN(store.DefaultKMSRegion, accountID, a.AliasName),
			"TargetKeyId": a.TargetKeyID,
		})
	}
	return json.Marshal(map[string]any{"Aliases": entries})
}

func ScheduleKeyDeletionJSON(k store.Key) ([]byte, error) {
	return json.Marshal(map[string]any{
		"KeyId":               k.ARN,
		"KeyState":            k.KeyState,
		"DeletionDate":        parseCreationFloat(k.DeletionDate),
		"PendingWindowInDays": k.PendingWindowInDays,
	})
}

func CancelKeyDeletionJSON(keyARN string) ([]byte, error) {
	return json.Marshal(map[string]string{"KeyId": keyARN})
}

func GetKeyRotationStatusJSON(enabled bool) ([]byte, error) {
	return json.Marshal(map[string]any{"KeyRotationEnabled": enabled})
}

// DecodeBinaryField decodes a base64 string from a JSON request field.
func DecodeBinaryField(v any) ([]byte, error) {
	switch t := v.(type) {
	case string:
		return base64.StdEncoding.DecodeString(t)
	case []byte:
		return t, nil
	default:
		return nil, fmt.Errorf("binary field must be base64 string")
	}
}
