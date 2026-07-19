package sqs

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"strings"
)

// SSEMode is the queue server-side encryption mode derived from attributes.
type SSEMode int

const (
	// SSENone means message bodies are stored in plaintext.
	SSENone SSEMode = iota
	// SSESQS encrypts bodies with a random DEK sealed by the store master key.
	SSESQS
	// SSEKMS encrypts bodies with a DEK sealed under a customer-managed KMS CMK.
	SSEKMS
)

const (
	attrSqsManagedSSE = "SqsManagedSseEnabled"
	attrKmsMasterKey  = "KmsMasterKeyId"
)

// ModeFromAttributes detects SSE from queue attributes.
// KmsMasterKeyId wins when both SSE options are present (AWS allows only one).
func ModeFromAttributes(attrs map[string]string) SSEMode {
	if attrs == nil {
		return SSENone
	}
	if strings.TrimSpace(attrs[attrKmsMasterKey]) != "" {
		return SSEKMS
	}
	if isTruthy(attrs[attrSqsManagedSSE]) {
		return SSESQS
	}
	return SSENone
}

// KmsMasterKeyID returns the KmsMasterKeyId attribute, if any.
func KmsMasterKeyID(attrs map[string]string) string {
	if attrs == nil {
		return ""
	}
	return strings.TrimSpace(attrs[attrKmsMasterKey])
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

// NewRandomDEK returns a 32-byte AES-256 data key.
func NewRandomDEK() ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("generate dek: %w", err)
	}
	return dek, nil
}

// EncryptAES256GCM seals plaintext under a 32-byte DEK.
// Output: nonce || ciphertext+tag.
func EncryptAES256GCM(dek, plaintext []byte) ([]byte, error) {
	if len(dek) != 32 {
		return nil, fmt.Errorf("dek must be 32 bytes")
	}
	block, err := aes.NewCipher(dek)
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
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// DecryptAES256GCM opens a blob produced by EncryptAES256GCM.
func DecryptAES256GCM(dek, blob []byte) ([]byte, error) {
	if len(dek) != 32 {
		return nil, fmt.Errorf("dek must be 32 bytes")
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(blob) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, sealed := blob[:nonceSize], blob[nonceSize:]
	return gcm.Open(nil, nonce, sealed, nil)
}
