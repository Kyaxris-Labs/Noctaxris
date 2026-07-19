package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// MintTempCredentials stores temporary credentials (ASIA… key id) with sealed
// secret and session token.
func (s *Store) MintTempCredentials(accountID, roleARN, sessionName, secret, sessionToken string, expires time.Time) (accessKeyID string, err error) {
	accessKeyID, err = newTempAccessKeyID()
	if err != nil {
		return "", err
	}
	sealedSecret, err := Seal(s.master, []byte(secret))
	if err != nil {
		return "", fmt.Errorf("seal secret: %w", err)
	}
	sealedToken, err := Seal(s.master, []byte(sessionToken))
	if err != nil {
		return "", fmt.Errorf("seal session token: %w", err)
	}
	expiresAt := expires.UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		`INSERT INTO access_keys
		 (access_key_id, account_id, secret_ciphertext, is_root,
		  session_token_ciphertext, role_arn, session_name, expires_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?, ?)`,
		accessKeyID, accountID, sealedSecret, sealedToken, roleARN, sessionName, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert temp credentials: %w", err)
	}
	return accessKeyID, nil
}

func newTempAccessKeyID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// ASIA + 16 hex chars ≈ AWS temporary key id shape (20 chars total).
	return "ASIA" + hex.EncodeToString(b[:]), nil
}
