package store

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/chacha20poly1305"
)

const masterKeySize = 32

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
