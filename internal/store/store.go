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
  federated_user TEXT,
  mfa_authenticated INTEGER NOT NULL DEFAULT 0,
  mfa_authenticated_at TEXT
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
CREATE TABLE IF NOT EXISTS kms_keys (
  key_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  arn TEXT NOT NULL,
  key_state TEXT NOT NULL,
  key_usage TEXT NOT NULL DEFAULT 'ENCRYPT_DECRYPT',
  sealed_material BLOB NOT NULL,
  key_policy TEXT NOT NULL,
  creation_date TEXT NOT NULL,
  deletion_date TEXT NOT NULL DEFAULT '',
  key_rotation_enabled INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS kms_aliases (
  alias_name TEXT NOT NULL,
  account_id TEXT NOT NULL,
  target_key_id TEXT NOT NULL,
  PRIMARY KEY (account_id, alias_name)
);
CREATE TABLE IF NOT EXISTS kms_grants (
  grant_id TEXT PRIMARY KEY,
  key_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  grantee_principal TEXT NOT NULL,
  retiring_principal TEXT,
  operations TEXT NOT NULL,
  name TEXT
);
CREATE TABLE IF NOT EXISTS s3_buckets (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  creation_date TEXT NOT NULL,
  bucket_policy TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS s3_objects (
  account_id TEXT NOT NULL,
  bucket TEXT NOT NULL,
  key TEXT NOT NULL,
  etag TEXT NOT NULL,
  size INTEGER NOT NULL,
  content_type TEXT,
  sse_algorithm TEXT,
  kms_key_id TEXT,
  sealed_dek BLOB,
  storage_path TEXT NOT NULL,
  last_modified TEXT NOT NULL,
  PRIMARY KEY (account_id, bucket, key)
);
CREATE TABLE IF NOT EXISTS s3_multipart_uploads (
  upload_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  bucket TEXT NOT NULL,
  key TEXT NOT NULL,
  content_type TEXT NOT NULL,
  sse_algorithm TEXT,
  kms_key_id TEXT,
  sealed_dek BLOB,
  initiated TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS s3_multipart_parts (
  upload_id TEXT NOT NULL,
  part_number INTEGER NOT NULL,
  etag TEXT NOT NULL,
  size INTEGER NOT NULL,
  storage_path TEXT NOT NULL,
  PRIMARY KEY (upload_id, part_number),
  FOREIGN KEY (upload_id) REFERENCES s3_multipart_uploads(upload_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_s3_multipart_uploads_bucket ON s3_multipart_uploads(account_id, bucket);
CREATE TABLE IF NOT EXISTS dynamodb_tables (
  account_id TEXT NOT NULL,
  table_name TEXT NOT NULL,
  table_arn TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  hash_key_name TEXT NOT NULL,
  hash_key_type TEXT NOT NULL,
  range_key_name TEXT NOT NULL DEFAULT '',
  range_key_type TEXT NOT NULL DEFAULT '',
  resource_policy TEXT NOT NULL DEFAULT '',
  sse_type TEXT NOT NULL DEFAULT 'AWS_OWNED',
  kms_key_id TEXT NOT NULL DEFAULT '',
  creation_date TEXT NOT NULL,
  gsi_name TEXT NOT NULL DEFAULT '',
  gsi_hash_key_name TEXT NOT NULL DEFAULT '',
  gsi_hash_key_type TEXT NOT NULL DEFAULT '',
  gsi_range_key_name TEXT NOT NULL DEFAULT '',
  gsi_range_key_type TEXT NOT NULL DEFAULT '',
  ttl_attribute_name TEXT NOT NULL DEFAULT '',
  ttl_enabled INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, table_name)
);
CREATE TABLE IF NOT EXISTS dynamodb_items (
  account_id TEXT NOT NULL,
  table_name TEXT NOT NULL,
  item_pk TEXT NOT NULL,
  item_sk TEXT NOT NULL DEFAULT '',
  gsi_pk TEXT NOT NULL DEFAULT '',
  gsi_sk TEXT NOT NULL DEFAULT '',
  item_json BLOB NOT NULL,
  sealed INTEGER NOT NULL DEFAULT 0,
  sealed_dek BLOB,
  PRIMARY KEY (account_id, table_name, item_pk, item_sk)
);
CREATE TABLE IF NOT EXISTS sqs_queues (
  account_id TEXT NOT NULL,
  queue_name TEXT NOT NULL,
  queue_url TEXT NOT NULL,
  queue_arn TEXT NOT NULL,
  attributes_json TEXT NOT NULL DEFAULT '{}',
  creation_date TEXT NOT NULL,
  PRIMARY KEY (account_id, queue_name)
);
CREATE TABLE IF NOT EXISTS sqs_messages (
  message_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  queue_name TEXT NOT NULL,
  body BLOB NOT NULL,
  sealed INTEGER NOT NULL DEFAULT 0,
  sealed_dek BLOB,
  receipt_handle TEXT UNIQUE,
  visible_after TEXT NOT NULL DEFAULT '',
  receive_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  attributes_json TEXT NOT NULL DEFAULT '{}',
  message_group_id TEXT NOT NULL DEFAULT '',
  message_deduplication_id TEXT NOT NULL DEFAULT '',
  sequence_number INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS lambda_functions (
  account_id TEXT NOT NULL,
  function_name TEXT NOT NULL,
  function_arn TEXT NOT NULL,
  role_arn TEXT NOT NULL,
  runtime TEXT NOT NULL,
  handler TEXT NOT NULL,
  timeout INTEGER NOT NULL,
  memory INTEGER NOT NULL,
  env_json TEXT NOT NULL DEFAULT '{}',
  code_sha256 TEXT NOT NULL,
  code_path TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'Active',
  description TEXT NOT NULL DEFAULT '',
  last_modified TEXT NOT NULL,
  PRIMARY KEY (account_id, function_name)
);
CREATE TABLE IF NOT EXISTS iam_groups (
  account_id TEXT NOT NULL,
  group_name TEXT NOT NULL,
  group_id TEXT NOT NULL,
  arn TEXT NOT NULL,
  PRIMARY KEY (account_id, group_name)
);
CREATE TABLE IF NOT EXISTS iam_group_memberships (
  account_id TEXT NOT NULL,
  group_name TEXT NOT NULL,
  user_name TEXT NOT NULL,
  PRIMARY KEY (account_id, group_name, user_name)
);
CREATE TABLE IF NOT EXISTS iam_permissions_boundaries (
  account_id TEXT NOT NULL,
  principal_type TEXT NOT NULL,
  principal_name TEXT NOT NULL,
  policy_arn TEXT NOT NULL,
  PRIMARY KEY (account_id, principal_type, principal_name)
);
CREATE TABLE IF NOT EXISTS iam_instance_profiles (
  account_id TEXT NOT NULL,
  profile_name TEXT NOT NULL,
  profile_arn TEXT NOT NULL,
  PRIMARY KEY (account_id, profile_name)
);
CREATE TABLE IF NOT EXISTS iam_instance_profile_roles (
  account_id TEXT NOT NULL,
  profile_name TEXT NOT NULL,
  role_name TEXT NOT NULL,
  PRIMARY KEY (account_id, profile_name)
);
CREATE TABLE IF NOT EXISTS iam_mfa_devices (
  account_id TEXT NOT NULL,
  serial TEXT NOT NULL,
  user_name TEXT NOT NULL DEFAULT '',
  seed_ciphertext BLOB NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, serial)
);
CREATE TABLE IF NOT EXISTS org_ous (
  id TEXT PRIMARY KEY,
  parent_id TEXT NOT NULL,
  name TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS org_policies (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  name TEXT NOT NULL,
  document TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS org_policy_attachments (
  policy_id TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  PRIMARY KEY (policy_id, target_type, target_id)
);
CREATE TABLE IF NOT EXISTS org_enabled_policy_types (
  root_id TEXT NOT NULL,
  policy_type TEXT NOT NULL,
  PRIMARY KEY (root_id, policy_type)
);
`

// Access key status values stored in access_keys.status.
const (
	AccessKeyStatusActive   = "Active"
	AccessKeyStatusInactive = "Inactive"
)

// AccessKey is a long-lived or temporary credential record.
type AccessKey struct {
	AccessKeyID         string
	AccountID           string
	Secret              string
	IsRoot              bool
	UserName            string // set for IAM user keys; empty for root/temp
	Status              string // Active or Inactive; empty treated as Active for legacy rows
	SessionToken        string // plaintext; empty if long-lived
	RoleARN             string
	SessionName         string
	FederatedUser       string
	SessionPolicy       string
	ExpiresAt           time.Time // zero if none
	MFAAuthenticated    bool
	MFAAuthenticatedAt  time.Time // zero if MFA not presented
}

type Store struct {
	db       *sql.DB
	master   MasterKey
	dataRoot string
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
	s := &Store{db: db, master: master, dataRoot: dataRoot}
	if err := s.migrateSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// DataRoot returns the store data directory (object bytes live under s3/).
func (s *Store) DataRoot() string {
	return s.dataRoot
}

// SealWithMaster seals plaintext under the store master key.
func (s *Store) SealWithMaster(plaintext []byte) ([]byte, error) {
	return Seal(s.master, plaintext)
}

// UnsealWithMaster opens a blob produced by SealWithMaster.
func (s *Store) UnsealWithMaster(ciphertext []byte) ([]byte, error) {
	return Unseal(s.master, ciphertext)
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
		`ALTER TABLE access_keys ADD COLUMN mfa_authenticated INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE access_keys ADD COLUMN mfa_authenticated_at TEXT`,
		`ALTER TABLE kms_keys ADD COLUMN deletion_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE kms_keys ADD COLUMN key_rotation_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_hash_key_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_hash_key_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_range_key_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_range_key_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN ttl_attribute_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN ttl_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE dynamodb_items ADD COLUMN gsi_pk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_items ADD COLUMN gsi_sk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sqs_messages ADD COLUMN message_group_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sqs_messages ADD COLUMN message_deduplication_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sqs_messages ADD COLUMN sequence_number INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE s3_buckets ADD COLUMN default_encryption_algorithm TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_buckets ADD COLUMN default_encryption_kms_key_id TEXT NOT NULL DEFAULT ''`,
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

// LookupAccessKey is a thin wrapper for long-lived credential callers.
func (s *Store) LookupAccessKey(accessKeyID string) (accountID, secret string, isRoot bool, err error) {
	ak, err := s.LookupAccessKeyRecord(accessKeyID)
	if err != nil {
		return "", "", false, err
	}
	return ak.AccountID, ak.Secret, ak.IsRoot, nil
}
