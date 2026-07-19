package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS accounts (
  account_id TEXT PRIMARY KEY
);
CREATE TABLE IF NOT EXISTS access_keys (
  access_key_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  secret_ciphertext BLOB NOT NULL,
  is_root INTEGER NOT NULL DEFAULT 0,
  session_token_ciphertext BLOB,
  role_arn TEXT,
  session_name TEXT,
  expires_at TEXT,
  user_name TEXT,
  status TEXT NOT NULL DEFAULT 'Active',
  session_policy TEXT,
  federated_user TEXT
);
CREATE TABLE IF NOT EXISTS policies (
  policy_id TEXT PRIMARY KEY,
  document TEXT NOT NULL,
  policy_name TEXT,
  account_id TEXT,
  arn TEXT,
  path TEXT,
  default_version_id TEXT
);
CREATE TABLE IF NOT EXISTS policy_attachments (
  principal_arn TEXT NOT NULL,
  policy_id TEXT NOT NULL,
  PRIMARY KEY (principal_arn, policy_id)
);
CREATE TABLE IF NOT EXISTS create_account_requests (
  request_id TEXT PRIMARY KEY,
  status TEXT NOT NULL,
  account_name TEXT,
  email TEXT,
  account_id TEXT,
  requested_by_account_id TEXT NOT NULL,
  failure_reason TEXT
);
CREATE TABLE IF NOT EXISTS roles (
  account_id TEXT NOT NULL,
  role_name TEXT NOT NULL,
  role_arn TEXT NOT NULL,
  trust_policy TEXT NOT NULL,
  role_id TEXT,
  path TEXT NOT NULL DEFAULT '/',
  create_date TEXT,
  description TEXT,
  PRIMARY KEY (account_id, role_name)
);
CREATE TABLE IF NOT EXISTS users (
  account_id TEXT NOT NULL,
  user_name TEXT NOT NULL,
  user_id TEXT NOT NULL,
  arn TEXT NOT NULL,
  PRIMARY KEY (account_id, user_name)
);
CREATE TABLE IF NOT EXISTS managed_policies (
  policy_arn TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  policy_name TEXT NOT NULL,
  policy_id TEXT NOT NULL,
  default_version_id TEXT NOT NULL,
  document TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS inline_policies (
  principal_arn TEXT NOT NULL,
  policy_name TEXT NOT NULL,
  document TEXT NOT NULL,
  PRIMARY KEY (principal_arn, policy_name)
);
CREATE TABLE IF NOT EXISTS oidc_providers (
  provider_arn TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  url TEXT NOT NULL,
  client_id TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS saml_providers (
  provider_arn TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  metadata_xml TEXT NOT NULL
);
`

// Access key status values stored in access_keys.status.
const (
	AccessKeyStatusActive   = "Active"
	AccessKeyStatusInactive = "Inactive"
)

// AccessKey is a long-lived or temporary credential record.
type AccessKey struct {
	AccessKeyID   string
	AccountID     string
	Secret        string
	IsRoot        bool
	UserName      string // set for IAM user keys; empty for root/temp
	Status        string // Active or Inactive; empty treated as Active for legacy rows
	SessionToken  string // plaintext; empty if long-lived
	RoleARN       string
	SessionName   string
	FederatedUser string
	SessionPolicy string
	ExpiresAt     time.Time // zero if none
}

type Store struct {
	db     *sql.DB
	master MasterKey
}

func Open(dataRoot string, master MasterKey) (*Store, error) {
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataRoot, "state.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, master: master}
	if err := s.migrateSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrateSchema() error {
	alters := []string{
		`ALTER TABLE access_keys ADD COLUMN session_token_ciphertext BLOB`,
		`ALTER TABLE access_keys ADD COLUMN role_arn TEXT`,
		`ALTER TABLE access_keys ADD COLUMN session_name TEXT`,
		`ALTER TABLE access_keys ADD COLUMN expires_at TEXT`,
		`ALTER TABLE access_keys ADD COLUMN user_name TEXT`,
		`ALTER TABLE access_keys ADD COLUMN status TEXT NOT NULL DEFAULT 'Active'`,
		`ALTER TABLE access_keys ADD COLUMN session_policy TEXT`,
		`ALTER TABLE access_keys ADD COLUMN federated_user TEXT`,
		`ALTER TABLE policies ADD COLUMN policy_name TEXT`,
		`ALTER TABLE policies ADD COLUMN account_id TEXT`,
		`ALTER TABLE policies ADD COLUMN arn TEXT`,
		`ALTER TABLE policies ADD COLUMN path TEXT`,
		`ALTER TABLE policies ADD COLUMN default_version_id TEXT`,
		`ALTER TABLE roles ADD COLUMN role_id TEXT`,
		`ALTER TABLE roles ADD COLUMN path TEXT NOT NULL DEFAULT '/'`,
		`ALTER TABLE roles ADD COLUMN create_date TEXT`,
		`ALTER TABLE roles ADD COLUMN description TEXT`,
	}
	for _, stmt := range alters {
		if _, err := s.db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("migrate schema: %w", err)
		}
	}
	return nil
}

func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column")
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) EnsureRoot(accountID, accessKeyID, secret string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT OR IGNORE INTO accounts (account_id) VALUES (?)`, accountID); err != nil {
		return err
	}

	var existingAccountID string
	var ciphertext []byte
	var isRoot int
	rowErr := tx.QueryRow(
		`SELECT account_id, secret_ciphertext, is_root FROM access_keys WHERE access_key_id = ?`,
		accessKeyID,
	).Scan(&existingAccountID, &ciphertext, &isRoot)

	if rowErr == nil {
		if existingAccountID != accountID || isRoot != 1 {
			return fmt.Errorf("access key %s already exists", accessKeyID)
		}
		plaintext, err := Unseal(s.master, ciphertext)
		if err != nil {
			return err
		}
		if string(plaintext) != secret {
			return fmt.Errorf("access key %s already exists with different secret", accessKeyID)
		}
		return tx.Commit()
	}
	if !errors.Is(rowErr, sql.ErrNoRows) {
		return rowErr
	}

	sealed, err := Seal(s.master, []byte(secret))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO access_keys (access_key_id, account_id, secret_ciphertext, is_root) VALUES (?, ?, ?, 1)`,
		accessKeyID, accountID, sealed,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// LookupAccessKeyRecord returns the full access key record including session fields.
func (s *Store) LookupAccessKeyRecord(accessKeyID string) (AccessKey, error) {
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
	)
	err := s.db.QueryRow(
		`SELECT account_id, secret_ciphertext, is_root,
		        session_token_ciphertext, role_arn, session_name, expires_at, user_name, status,
		        session_policy, federated_user
		 FROM access_keys WHERE access_key_id = ?`,
		accessKeyID,
	).Scan(&accountID, &ciphertext, &rootFlag, &sessionTokenCiphertext, &roleARN, &sessionName, &expiresAt, &userName, &status, &sessionPolicy, &federatedUser)
	if err != nil {
		return AccessKey{}, err
	}
	plaintext, err := Unseal(s.master, ciphertext)
	if err != nil {
		return AccessKey{}, err
	}
	ak := AccessKey{
		AccessKeyID: accessKeyID,
		AccountID:   accountID,
		Secret:      string(plaintext),
		IsRoot:      rootFlag == 1,
		Status:      AccessKeyStatusActive,
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
	return ak, nil
}

// LookupAccessKey is a thin wrapper for long-lived credential callers.
func (s *Store) LookupAccessKey(accessKeyID string) (accountID, secret string, isRoot bool, err error) {
	ak, err := s.LookupAccessKeyRecord(accessKeyID)
	if err != nil {
		return "", "", false, err
	}
	return ak.AccountID, ak.Secret, ak.IsRoot, nil
}
