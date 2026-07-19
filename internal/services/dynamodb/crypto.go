package dynamodb

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	kmssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kms"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// EncryptedItem is the ciphertext form stored for SSE-KMS tables.
type EncryptedItem struct {
	Ciphertext []byte
	SealedDEK  []byte
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

// SealItemJSON encrypts AttributeValue JSON for a KMS SSE table.
// cmk is 32-byte unsealed CMK material; keyID is embedded in the wrapped DEK.
func SealItemJSON(cmk []byte, keyID string, plainJSON []byte) (EncryptedItem, error) {
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return EncryptedItem{}, err
	}
	sealedDEK, err := kmssvc.EncryptUnderCMK(cmk, keyID, dek)
	if err != nil {
		return EncryptedItem{}, err
	}
	ct, err := EncryptAES256GCM(dek, plainJSON)
	if err != nil {
		return EncryptedItem{}, err
	}
	return EncryptedItem{Ciphertext: ct, SealedDEK: sealedDEK}, nil
}

// OpenItemJSON decrypts a sealed DynamoDB item using CMK material.
func OpenItemJSON(cmk []byte, sealed EncryptedItem) ([]byte, error) {
	dek, err := kmssvc.DecryptUnderCMK(cmk, sealed.SealedDEK)
	if err != nil {
		return nil, err
	}
	return DecryptAES256GCM(dek, sealed.Ciphertext)
}

// StoragePayload prepares item bytes for PutItemBytes based on table SSE settings.
// For AWS_OWNED, plainJSON is stored unsealed. For KMS, plainJSON is sealed under a DEK.
func StoragePayload(table store.DynamoTable, plainJSON, cmk []byte, keyID string) (data []byte, sealed bool, sealedDEK []byte, err error) {
	if table.SSEType != store.SSETypeKMS {
		return plainJSON, false, nil, nil
	}
	if len(cmk) == 0 || keyID == "" {
		return nil, false, nil, fmt.Errorf("kms material required for SSE-KMS table")
	}
	enc, err := SealItemJSON(cmk, keyID, plainJSON)
	if err != nil {
		return nil, false, nil, err
	}
	return enc.Ciphertext, true, enc.SealedDEK, nil
}

// LoadItemJSON returns plaintext AttributeValue JSON from a stored item.
func LoadItemJSON(table store.DynamoTable, item store.DynamoStoredItem, cmk []byte) ([]byte, error) {
	if !item.Sealed {
		return item.ItemJSON, nil
	}
	if table.SSEType != store.SSETypeKMS {
		return nil, fmt.Errorf("sealed item on non-KMS table")
	}
	return OpenItemJSON(cmk, EncryptedItem{
		Ciphertext: item.ItemJSON,
		SealedDEK:  item.SealedDEK,
	})
}
