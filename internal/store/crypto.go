package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	masterKeySize = 32

	ciphertextVersionV1 byte = 1
	ciphertextVersionV2 byte = 2

	// EnvAllowMasterKeyInDataRoot permits a master key path under NOCTAXRIS_DATA_ROOT
	// (including the historical $DATA_ROOT/master.key default when this env is set).
	EnvAllowMasterKeyInDataRoot = "NOCTAXRIS_ALLOW_MASTER_KEY_IN_DATA_ROOT"
)

type MasterKey [masterKeySize]byte

// AllowMasterKeyInDataRoot reports whether master.key may live under DATA_ROOT.
func AllowMasterKeyInDataRoot() bool {
	v := strings.TrimSpace(os.Getenv(EnvAllowMasterKeyInDataRoot))
	return v == "1" || strings.EqualFold(v, "true")
}

// DefaultMasterKeyPath returns a path adjacent to but outside dataRoot.
// Example: dataRoot=/var/lib/noctaxris → /var/lib/noctaxris-secrets/master.key
func DefaultMasterKeyPath(dataRoot string) string {
	abs, err := filepath.Abs(dataRoot)
	if err != nil {
		abs = dataRoot
	}
	parent := filepath.Dir(abs)
	base := filepath.Base(abs)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "noctaxris"
	}
	return filepath.Join(parent, base+"-secrets", "master.key")
}

// MasterKeyPathUnderDataRoot reports whether keyPath resolves under dataRoot.
func MasterKeyPathUnderDataRoot(keyPath, dataRoot string) (bool, error) {
	absKey, err := filepath.Abs(keyPath)
	if err != nil {
		return false, fmt.Errorf("resolve master key path: %w", err)
	}
	absRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return false, fmt.Errorf("resolve data root: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absKey)
	if err != nil {
		return false, fmt.Errorf("relate master key to data root: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

// ResolveMasterKeyPath picks the master key file path.
// Empty configured path defaults outside DATA_ROOT unless
// NOCTAXRIS_ALLOW_MASTER_KEY_IN_DATA_ROOT is set (then $DATA_ROOT/master.key).
// Any path under DATA_ROOT requires that opt-in.
func ResolveMasterKeyPath(configured, dataRoot string) (string, error) {
	path := strings.TrimSpace(configured)
	allow := AllowMasterKeyInDataRoot()
	if path == "" {
		if allow {
			path = filepath.Join(dataRoot, "master.key")
		} else {
			path = DefaultMasterKeyPath(dataRoot)
		}
	}
	under, err := MasterKeyPathUnderDataRoot(path, dataRoot)
	if err != nil {
		return "", err
	}
	if under && !allow {
		return "", fmt.Errorf("master key path %q is under NOCTAXRIS_DATA_ROOT %q; set NOCTAXRIS_MASTER_KEY_FILE outside the data root, or %s=1 to allow colocation",
			path, dataRoot, EnvAllowMasterKeyInDataRoot)
	}
	return path, nil
}

func ensureMasterKeyFileMode(path string) error {
	// Windows ACL model does not expose Unix permission bits reliably.
	if runtime.GOOS == "windows" {
		_ = os.Chmod(path, 0o600)
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if mode&0o004 != 0 {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("master key file %s is world-readable and chmod 0600 failed: %w", path, err)
		}
		info, err = os.Stat(path)
		if err != nil {
			return err
		}
		mode = info.Mode().Perm()
		if mode&0o004 != 0 {
			return fmt.Errorf("master key file %s remains world-readable after chmod 0600 (mode %04o)", path, mode)
		}
		return nil
	}
	if mode != 0o600 {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("master key file %s chmod 0600: %w", path, err)
		}
	}
	return nil
}

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
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return MasterKey{}, fmt.Errorf("create master key directory: %w", err)
		}
		if err := os.WriteFile(path, key[:], 0o600); err != nil {
			return MasterKey{}, err
		}
		if err := ensureMasterKeyFileMode(path); err != nil {
			return MasterKey{}, err
		}
		return key, nil
	}
	if err := ensureMasterKeyFileMode(path); err != nil {
		return MasterKey{}, err
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
