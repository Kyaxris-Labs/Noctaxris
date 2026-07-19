package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// MintTempOpts configures temporary credential minting.
type MintTempOpts struct {
	AccountID          string
	RoleARN            string
	SessionName        string
	UserName           string
	FederatedUser      string
	SessionPolicy      string
	Secret             string
	SessionToken       string
	Expires            time.Time
	IsRoot             bool
	MFAAuthenticated   bool
	MFAAuthenticatedAt time.Time
}

// MintTempCredentials stores temporary credentials (ASIA… key id) with sealed
// secret and session token.
func (s *Store) MintTempCredentials(accountID, roleARN, sessionName, secret, sessionToken string, expires time.Time) (accessKeyID string, err error) {
	return s.MintTempCredentialsOpts(MintTempOpts{
		AccountID:    accountID,
		RoleARN:      roleARN,
		SessionName:  sessionName,
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      expires,
	})
}

// MintTempCredentialsOpts stores temporary credentials with optional user,
// federated-user, session policy, and root session fields.
func (s *Store) MintTempCredentialsOpts(opts MintTempOpts) (accessKeyID string, err error) {
	accessKeyID, err = newTempAccessKeyID()
	if err != nil {
		return "", err
	}
	sealedSecret, err := Seal(s.master, []byte(opts.Secret))
	if err != nil {
		return "", fmt.Errorf("seal secret: %w", err)
	}
	sealedToken, err := Seal(s.master, []byte(opts.SessionToken))
	if err != nil {
		return "", fmt.Errorf("seal session token: %w", err)
	}
	expiresAt := opts.Expires.UTC().Format(time.RFC3339)
	isRoot := 0
	if opts.IsRoot {
		isRoot = 1
	}
	mfaAuth := 0
	var mfaAt any
	if opts.MFAAuthenticated {
		mfaAuth = 1
		at := opts.MFAAuthenticatedAt
		if at.IsZero() {
			at = opts.Expires // fallback should not happen; callers set now
		}
		mfaAt = at.UTC().Format(time.RFC3339)
	}
	_, err = s.db.Exec(
		`INSERT INTO access_keys
		 (access_key_id, account_id, secret_ciphertext, is_root,
		  session_token_ciphertext, role_arn, session_name, expires_at,
		  user_name, session_policy, federated_user, status,
		  mfa_authenticated, mfa_authenticated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accessKeyID, opts.AccountID, sealedSecret, isRoot,
		sealedToken, nullIfEmpty(opts.RoleARN), nullIfEmpty(opts.SessionName), expiresAt,
		nullIfEmpty(opts.UserName), nullIfEmpty(opts.SessionPolicy), nullIfEmpty(opts.FederatedUser),
		AccessKeyStatusActive, mfaAuth, mfaAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert temp credentials: %w", err)
	}
	return accessKeyID, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func newTempAccessKeyID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// ASIA + 16 hex chars ≈ AWS temporary key id shape (20 chars total).
	return "ASIA" + hex.EncodeToString(b[:]), nil
}
