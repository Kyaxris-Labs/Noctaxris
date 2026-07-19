package kms

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	ciphertextVersionV1 byte = 1
	ciphertextVersionV2 byte = 2
)

// EncryptUnderCMK seals plaintext with AES-256-GCM under cmk material.
// Output format v2: version(1)=2 || keyID_len(1) || keyID || nonce || ciphertext+tag.
func EncryptUnderCMK(cmk []byte, keyID string, plaintext []byte) ([]byte, error) {
	if len(cmk) != 32 {
		return nil, fmt.Errorf("cmk must be 32 bytes")
	}
	if keyID == "" || len(keyID) > 255 {
		return nil, fmt.Errorf("key id required and max 255 bytes")
	}
	block, err := aes.NewCipher(cmk)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	kid := []byte(keyID)
	out := make([]byte, 2+len(kid)+len(nonce)+len(sealed))
	out[0] = ciphertextVersionV2
	out[1] = byte(len(kid))
	copy(out[2:], kid)
	off := 2 + len(kid)
	copy(out[off:], nonce)
	copy(out[off+len(nonce):], sealed)
	return out, nil
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
func DecryptUnderCMK(cmk, blob []byte) ([]byte, error) {
	if len(cmk) != 32 {
		return nil, fmt.Errorf("cmk must be 32 bytes")
	}
	if len(blob) < 1 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	block, err := aes.NewCipher(cmk)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	var rest []byte
	switch blob[0] {
	case ciphertextVersionV1:
		rest = blob[1:]
	case ciphertextVersionV2:
		if len(blob) < 2 {
			return nil, fmt.Errorf("ciphertext too short")
		}
		n := int(blob[1])
		if len(blob) < 2+n {
			return nil, fmt.Errorf("ciphertext too short")
		}
		rest = blob[2+n:]
	default:
		return nil, fmt.Errorf("unsupported ciphertext version %d", blob[0])
	}
	if len(rest) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := rest[:nonceSize]
	sealed := rest[nonceSize:]
	return gcm.Open(nil, nonce, sealed, nil)
}

func b64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func keyMetadata(k store.Key) map[string]any {
	return map[string]any{
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

func GetKeyPolicyJSON(policy string) ([]byte, error) {
	return json.Marshal(map[string]string{"Policy": policy})
}

func EncryptJSON(keyID string, ciphertext []byte) ([]byte, error) {
	return json.Marshal(map[string]any{
		"CiphertextBlob": b64(ciphertext),
		"KeyId":          keyID,
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
