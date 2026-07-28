package store

import (
	"database/sql"
	"fmt"
	"time"
)

type accessKeyCacheEntry struct {
	ak AccessKey
}

// AccessKeyUnsealCount returns how many LookupAccessKeyRecord misses performed Unseal.
func (s *Store) AccessKeyUnsealCount() uint64 {
	return s.accessKeyUnsealN.Load()
}

// invalidateAccessKeyCache drops a cached access-key row. Call only after a
// successful Delete/Update of that accessKeyID.
func (s *Store) invalidateAccessKeyCache(accessKeyID string) {
	s.accessKeyCacheMu.Lock()
	defer s.accessKeyCacheMu.Unlock()
	if s.accessKeyCache != nil {
		delete(s.accessKeyCache, accessKeyID)
	}
	s.accessKeyCacheGen++
}

// LookupAccessKeyRecord returns the full access key record including session fields.
// On a cache hit the row is copied without Unseal; on miss Unseal runs once and the
// row is inserted (unless an invalidate raced).
func (s *Store) LookupAccessKeyRecord(accessKeyID string) (AccessKey, error) {
	s.accessKeyCacheMu.Lock()
	if e, ok := s.accessKeyCache[accessKeyID]; ok {
		ak := e.ak
		s.accessKeyCacheMu.Unlock()
		return ak, nil
	}
	gen := s.accessKeyCacheGen
	s.accessKeyCacheMu.Unlock()

	ak, err := s.loadAccessKeyRecordUncached(accessKeyID)
	if err != nil {
		return AccessKey{}, err
	}
	s.accessKeyUnsealN.Add(1)

	s.accessKeyCacheMu.Lock()
	defer s.accessKeyCacheMu.Unlock()
	if s.accessKeyCacheGen == gen {
		if s.accessKeyCache == nil {
			s.accessKeyCache = make(map[string]accessKeyCacheEntry)
		}
		s.accessKeyCache[accessKeyID] = accessKeyCacheEntry{ak: ak}
	}
	return ak, nil
}

func (s *Store) loadAccessKeyRecordUncached(accessKeyID string) (AccessKey, error) {
	var (
		accountID              string
		ciphertext             []byte
		rootFlag               int
		sessionTokenCiphertext []byte
		roleARN                sql.NullString
		sessionName            sql.NullString
		expiresAt              sql.NullString
		userName               sql.NullString
		status                 sql.NullString
		sessionPolicy          sql.NullString
		federatedUser          sql.NullString
		mfaAuthenticated       int
		mfaAuthenticatedAt     sql.NullString
	)
	err := s.db.QueryRow(
		`SELECT account_id, secret_ciphertext, is_root,
		        session_token_ciphertext, role_arn, session_name, expires_at, user_name, status,
		        session_policy, federated_user, mfa_authenticated, mfa_authenticated_at
		 FROM access_keys WHERE access_key_id = ?`,
		accessKeyID,
	).Scan(&accountID, &ciphertext, &rootFlag, &sessionTokenCiphertext, &roleARN, &sessionName, &expiresAt, &userName, &status, &sessionPolicy, &federatedUser, &mfaAuthenticated, &mfaAuthenticatedAt)
	if err != nil {
		return AccessKey{}, err
	}
	plaintext, err := Unseal(s.master, ciphertext)
	if err != nil {
		return AccessKey{}, err
	}
	ak := AccessKey{
		AccessKeyID:      accessKeyID,
		AccountID:        accountID,
		Secret:           string(plaintext),
		IsRoot:           rootFlag == 1,
		Status:           AccessKeyStatusActive,
		MFAAuthenticated: mfaAuthenticated == 1,
	}
	if userName.Valid {
		ak.UserName = userName.String
	}
	if status.Valid && status.String != "" {
		ak.Status = status.String
	}
	if sessionPolicy.Valid {
		ak.SessionPolicy = sessionPolicy.String
	}
	if federatedUser.Valid {
		ak.FederatedUser = federatedUser.String
	}
	if len(sessionTokenCiphertext) > 0 {
		tokenPlain, err := Unseal(s.master, sessionTokenCiphertext)
		if err != nil {
			return AccessKey{}, fmt.Errorf("unseal session token: %w", err)
		}
		ak.SessionToken = string(tokenPlain)
	}
	if roleARN.Valid {
		ak.RoleARN = roleARN.String
	}
	if sessionName.Valid {
		ak.SessionName = sessionName.String
	}
	if expiresAt.Valid && expiresAt.String != "" {
		t, err := time.Parse(time.RFC3339, expiresAt.String)
		if err != nil {
			return AccessKey{}, fmt.Errorf("parse expires_at: %w", err)
		}
		ak.ExpiresAt = t
	}
	if mfaAuthenticatedAt.Valid && mfaAuthenticatedAt.String != "" {
		t, err := time.Parse(time.RFC3339, mfaAuthenticatedAt.String)
		if err != nil {
			return AccessKey{}, fmt.Errorf("parse mfa_authenticated_at: %w", err)
		}
		ak.MFAAuthenticatedAt = t
	}
	return ak, nil
}
