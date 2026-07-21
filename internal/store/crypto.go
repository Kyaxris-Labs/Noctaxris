package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	masterKeySize = 32

	ciphertextVersionV1 byte = 1
	ciphertextVersionV2 byte = 2
)

type MasterKey [masterKeySize]byte

func LoadOrCreateMasterKey(path string) (MasterKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return MasterKey{}, err
		}
		var key MasterKey
		if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
			return MasterKey{}, err
		}
		if err := os.WriteFile(path, key[:], 0o600); err != nil {
			return MasterKey{}, err
		}
		return key, nil
	}
	if len(data) != masterKeySize {
		return MasterKey{}, fmt.Errorf("master key file %s: want %d bytes, got %d", path, masterKeySize, len(data))
	}
	var key MasterKey
	copy(key[:], data)
	return key, nil
}

func Seal(key MasterKey, plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := aead.Seal(nil, nonce, plaintext, nil)
	return append(nonce, sealed...), nil
}

func Unseal(key MasterKey, ciphertext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		return nil, err
	}
	nonceSize := aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := ciphertext[:nonceSize]
	sealed := ciphertext[nonceSize:]
	return aead.Open(nil, nonce, sealed, nil)
}

// EncryptionContextAAD builds deterministic GCM additional authenticated data
// from an encryption context map. Nil/empty context yields nil AAD (v2 blobs
// encrypted without context remain decryptable with a nil/empty context).
func EncryptionContextAAD(encryptionContext map[string]string) []byte {
	if len(encryptionContext) == 0 {
		return nil
	}
	keys := make([]string, 0, len(encryptionContext))
	for k := range encryptionContext {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(0)
		b.WriteString(encryptionContext[k])
		b.WriteByte(0)
	}
	return []byte(b.String())
}

// EncryptUnderCMK seals plaintext with AES-256-GCM under cmk material.
// encryptionContext is bound as GCM AAD (AWS EncryptionContext semantics).
// Output format v2: version(1)=2 || keyID_len(1) || keyID || nonce || ciphertext+tag.
func EncryptUnderCMK(cmk []byte, keyID string, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
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
	aad := EncryptionContextAAD(encryptionContext)
	sealed := gcm.Seal(nil, nonce, plaintext, aad)
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

// DecryptUnderCMK opens a blob produced by EncryptUnderCMK (v1 or v2).
// encryptionContext must match the map supplied at encrypt time (AAD).
func DecryptUnderCMK(cmk, blob []byte, encryptionContext map[string]string) ([]byte, error) {
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
	aad := EncryptionContextAAD(encryptionContext)
	return gcm.Open(nil, nonce, sealed, aad)
}
