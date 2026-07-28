package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
  mfa_authenticated_at TEXT,
  last_used_at TEXT,
  last_used_service TEXT,
  last_used_region TEXT
);
CREATE TABLE IF NOT EXISTS iam_credential_reports (
  account_id TEXT PRIMARY KEY,
  state TEXT NOT NULL,
  generated_at TEXT NOT NULL,
  content_csv BLOB NOT NULL
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
  max_session_duration INTEGER NOT NULL DEFAULT 3600,
  PRIMARY KEY (account_id, role_name)
);
CREATE TABLE IF NOT EXISTS users (
  account_id TEXT NOT NULL,
  user_name TEXT NOT NULL,
  user_id TEXT NOT NULL,
  arn TEXT NOT NULL,
  create_date TEXT,
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
  client_id TEXT NOT NULL,
  client_ids TEXT NOT NULL DEFAULT '',
  thumbprints TEXT NOT NULL DEFAULT ''
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
CREATE UNIQUE INDEX IF NOT EXISTS idx_s3_buckets_name ON s3_buckets(name);
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
  sse_kms_context TEXT NOT NULL DEFAULT '',
  canned_acl TEXT NOT NULL DEFAULT 'private',
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
  sse_kms_context TEXT NOT NULL DEFAULT '',
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
	AccessKeyID        string
	AccountID          string
	Secret             string
	IsRoot             bool
	UserName           string // set for IAM user keys; empty for root/temp
	Status             string // Active or Inactive; empty treated as Active for legacy rows
	SessionToken       string // plaintext; empty if long-lived
	RoleARN            string
	SessionName        string
	FederatedUser      string
	SessionPolicy      string
	ExpiresAt          time.Time // zero if none
	MFAAuthenticated   bool
	MFAAuthenticatedAt time.Time // zero if MFA not presented
}

type Store struct {
	db       *sql.DB
	master   MasterKey
	dataRoot string

	asyncEnqueueMu sync.Mutex
	onAsyncEnqueue func(job LambdaAsyncInvocation)

	cognitoTriggerMu      sync.Mutex
	cognitoTriggerInvoker CognitoTriggerInvoker

	sfnTaskMu      sync.Mutex
	sfnTaskInvoker SFNTaskInvoker

	curDuckMu     sync.Mutex
	curDuckRunner CURDuckRunner

	// sqsMu serializes receive/send so claim+CAS and FIFO sequence stay atomic
	// under concurrent goroutines without global BEGIN IMMEDIATE (nested writers).
	sqsMu sync.Mutex

	// s3ObjectMu + s3ObjectLocks serialize PutObject file+SQL per account/bucket/key.
	s3ObjectMu    sync.Mutex
	s3ObjectLocks map[string]*s3ObjectLock

	// wsConnOnce + wsConn hold in-memory WebSocket API lab connections (PostToConnection).
	wsConnOnce sync.Once
	wsConn     *sync.Map

	// accessKeyCache holds unsealed AccessKey rows after LookupAccessKeyRecord.
	// Always on; invalidate only after successful Delete/Update of that key.
	accessKeyCacheMu  sync.Mutex
	accessKeyCache    map[string]accessKeyCacheEntry
	accessKeyCacheGen uint64
	accessKeyUnsealN  atomic.Uint64
}

type s3ObjectLock struct {
	mu   sync.Mutex
	refs int
}

// emptyStateDB caches one fully-bootstrapped state.db per process. Unit tests open a
// fresh data root per case; copying the template avoids re-running ~70 Ensure*Schema
// DDL passes (dominant cost under -race CI).
var (
	emptyStateOnce  sync.Once
	emptyStateBytes []byte
	emptyStateErr   error
)

// Open opens or creates the SQLite store under dataRoot.
func Open(dataRoot string, master MasterKey) (*Store, error) {
	return openStore(dataRoot, master, true)
}

func openStore(dataRoot string, master MasterKey, useEmptyTemplate bool) (*Store, error) {
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataRoot, "state.db")
	fresh := false
	if _, err := os.Stat(dbPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		fresh = true
		if useEmptyTemplate {
			if err := materializeEmptyStateDB(dbPath); err != nil {
				return nil, err
			}
		}
	}
	// Apply busy_timeout on every pooled connection (DSN pragma, not a one-shot Exec).
	// Do not set global _txlock=immediate: nested writers (e.g. CFN → CreateBucket)
	// would busy-wait against an open outer transaction.
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Single API process only against one data root (multi-instance unsupported).
	fromTemplate := fresh && useEmptyTemplate
	if !fromTemplate {
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			return nil, err
		}
	}
	s := &Store{db: db, master: master, dataRoot: dataRoot, s3ObjectLocks: map[string]*s3ObjectLock{}}
	if err := s.migrateSchema(); err != nil {
		db.Close()
		return nil, err
	}
	// Fresh template copies already include service DDL. Existing data roots still need
	// bootstrap so newly added Ensure* tables appear on upgrade.
	if !fromTemplate {
		if err := bootstrapServiceSchemas(db); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := s.ensureSchemaVersion(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func materializeEmptyStateDB(dst string) error {
	emptyStateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "noctaxris-empty-state-*")
		if err != nil {
			emptyStateErr = fmt.Errorf("empty state template: mkdir: %w", err)
			return
		}
		defer os.RemoveAll(dir)
		st, err := openStore(dir, MasterKey{}, false)
		if err != nil {
			emptyStateErr = fmt.Errorf("empty state template: bootstrap: %w", err)
			return
		}
		if err := st.Close(); err != nil {
			emptyStateErr = fmt.Errorf("empty state template: close: %w", err)
			return
		}
		raw, err := os.ReadFile(filepath.Join(dir, "state.db"))
		if err != nil {
			emptyStateErr = fmt.Errorf("empty state template: read: %w", err)
			return
		}
		emptyStateBytes = raw
	})
	if emptyStateErr != nil {
		return emptyStateErr
	}
	if len(emptyStateBytes) == 0 {
		return fmt.Errorf("empty state template: empty bytes")
	}
	if err := os.WriteFile(dst, emptyStateBytes, 0o600); err != nil {
		return fmt.Errorf("empty state template: write %s: %w", dst, err)
	}
	return nil
}

func bootstrapServiceSchemas(db *sql.DB) error {
	ensurers := []struct {
		name string
		fn   func(*sql.DB) error
	}{
		{"managed policy versions", EnsureManagedPolicyVersionsSchema},
		{"lambda version", EnsureLambdaVersionSchema},
		{"lambda layer", EnsureLambdaLayerSchema},
		{"lambda async", EnsureLambdaAsyncSchema},
		{"lambda image", EnsureLambdaImageSchema},
		{"lambda policy", EnsureLambdaPolicySchema},
		{"lambda esm", EnsureLambdaESMSchema},
		{"lambda function url", EnsureLambdaFunctionURLSchema},
		{"ssm", EnsureSSMSchema},
		{"secrets", EnsureSecretsSchema},
		{"sns", EnsureSNSSchema},
		{"events", EnsureEventsSchema},
		{"ecr", EnsureECRSchema},
		{"ecs", EnsureECSSchema},
		{"ecs service", EnsureECSServiceSchema},
		{"org account placement", EnsureOrgAccountPlacementSchema},
		{"logs", EnsureLogsSchema},
		{"logs subscription", EnsureLogsSubscriptionSchema},
		{"logs metric filter", EnsureLogsMetricFilterSchema},
		{"logs resource policy", EnsureLogsResourcePolicySchema},
		{"tagging", EnsureTaggingSchema},
		{"kinesis", EnsureKinesisSchema},
		{"kinesis resource policy", EnsureKinesisResourcePolicySchema},
		{"ses", EnsureSESSchema},
		{"appconfig", EnsureAppConfigSchema},
		{"sfn", EnsureSFNSchema},
		{"sfn resource policy", EnsureSFNResourcePolicySchema},
		{"kms key material", EnsureKMSKeyMaterialSchema},
		{"codebuild", EnsureCodeBuildSchema},
		{"batch", EnsureBatchSchema},
		{"cfn", EnsureCFNSchema},
		{"codepipeline", EnsureCodePipelineSchema},
		{"firehose", EnsureFirehoseSchema},
		{"glue", EnsureGlueSchema},
		{"waf", EnsureWAFSchema},
		{"config", EnsureConfigSchema},
		{"s3 versioning", EnsureS3VersioningSchema},
		{"s3 notifications", EnsureS3NotificationsSchema},
		{"s3 forensics", EnsureS3ForensicsSchema},
		{"dynamodb streams", EnsureDynamoDBStreamsSchema},
		{"dynamodb transact", EnsureDynamoDBTransactSchema},
		{"scheduler", EnsureSchedulerSchema},
		{"pipes", EnsurePipesSchema},
		{"mq", EnsureMQSchema},
		{"elasticache", EnsureElastiCacheSchema},
		{"memorydb", EnsureMemoryDBSchema},
		{"docdb", EnsureDocDBSchema},
		{"neptune", EnsureNeptuneSchema},
		{"msk", EnsureMSKSchema},
		{"eks", EnsureEKSSchema},
		{"transfer", EnsureTransferSchema},
		{"acm", EnsureACMSchema},
		{"guardduty", EnsureGuardDutySchema},
		{"detective", EnsureDetectiveSchema},
		{"macie", EnsureMacieSchema},
		{"securityhub", EnsureSecurityHubSchema},
		{"route53", EnsureRoute53Schema},
		{"service discovery", EnsureServiceDiscoverySchema},
		{"appsync", EnsureAppSyncSchema},
		{"apigatewayv2", EnsureAPIGatewayV2Schema},
		{"apigateway rest", EnsureAPIGatewayRESTSchema},
		{"cognito", EnsureCognitoSchema},
		{"cloudcontrol", EnsureCloudControlSchema},
		{"bcm export", EnsureBCMExportSchema},
		{"budgets", EnsureBudgetsSchema},
		{"cur", EnsureCURSchema},
		{"iot", EnsureIoTSchema},
		{"cloudwatch", EnsureCloudWatchSchema},
		{"lightsail", EnsureLightsailSchema},
		{"ec2", EnsureEC2Schema},
		{"autoscaling", EnsureASGSchema},
		{"beanstalk", EnsureBeanstalkSchema},
		{"backup", EnsureBackupSchema},
		{"codedeploy", EnsureCodeDeploySchema},
		{"cloudfront", EnsureCloudFrontSchema},
		{"elbv2", EnsureELBv2Schema},
		{"s3 vectors", EnsureS3VectorsSchema},
		{"bedrock", EnsureBedrockSchema},
		{"textract", EnsureTextractSchema},
		{"transcribe", EnsureTranscribeSchema},
		{"emr", EnsureEMRSchema},
		{"athena", EnsureAthenaSchema},
		{"opensearch", EnsureOpenSearchSchema},
		{"rds", EnsureRDSSchema},
		{"rds data", EnsureRDSDataSchema},
	}
	for _, e := range ensurers {
		if err := e.fn(db); err != nil {
			return fmt.Errorf("open store: ensure %s schema: %w", e.name, err)
		}
	}
	return nil
}

// DataRoot returns the store data directory (object bytes live under s3/).
func (s *Store) DataRoot() string {
	return s.dataRoot
}

// Ping verifies the SQLite connection is usable (readiness).
func (s *Store) Ping() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is closed")
	}
	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("sqlite ping: %w", err)
	}
	return nil
}

const schemaVersionCurrent = 1

func (s *Store) ensureSchemaVersion() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS schema_version (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  version INTEGER NOT NULL
);`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("ensure schema_version: %w", err)
	}
	var ver int
	err := s.db.QueryRow(`SELECT version FROM schema_version WHERE id = 1`).Scan(&ver)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.Exec(`INSERT INTO schema_version (id, version) VALUES (1, ?)`, schemaVersionCurrent)
		if err != nil {
			return fmt.Errorf("insert schema_version: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read schema_version: %w", err)
	}
	if ver < schemaVersionCurrent {
		_, err = s.db.Exec(`UPDATE schema_version SET version = ? WHERE id = 1`, schemaVersionCurrent)
		if err != nil {
			return fmt.Errorf("update schema_version: %w", err)
		}
	}
	return nil
}

// SchemaVersion returns the stored schema version row.
func (s *Store) SchemaVersion() (int, error) {
	var ver int
	err := s.db.QueryRow(`SELECT version FROM schema_version WHERE id = 1`).Scan(&ver)
	if err != nil {
		return 0, fmt.Errorf("schema version: %w", err)
	}
	return ver, nil
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
		`ALTER TABLE roles ADD COLUMN max_session_duration INTEGER NOT NULL DEFAULT 3600`,
		`ALTER TABLE oidc_providers ADD COLUMN client_ids TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE oidc_providers ADD COLUMN thumbprints TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN create_date TEXT`,
		`ALTER TABLE access_keys ADD COLUMN mfa_authenticated INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE access_keys ADD COLUMN mfa_authenticated_at TEXT`,
		`ALTER TABLE access_keys ADD COLUMN last_used_at TEXT`,
		`ALTER TABLE access_keys ADD COLUMN last_used_service TEXT`,
		`ALTER TABLE access_keys ADD COLUMN last_used_region TEXT`,
		`CREATE TABLE IF NOT EXISTS iam_credential_reports (
  account_id TEXT PRIMARY KEY,
  state TEXT NOT NULL,
  generated_at TEXT NOT NULL,
  content_csv BLOB NOT NULL
)`,
		`ALTER TABLE kms_keys ADD COLUMN deletion_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE kms_keys ADD COLUMN key_rotation_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE kms_keys ADD COLUMN last_rotation_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE kms_keys ADD COLUMN rotation_period_days INTEGER NOT NULL DEFAULT 365`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_hash_key_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_hash_key_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_range_key_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi_range_key_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi2_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi2_hash_key_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi2_hash_key_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi2_range_key_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN gsi2_range_key_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN ttl_attribute_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN ttl_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE dynamodb_items ADD COLUMN gsi_pk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_items ADD COLUMN gsi_sk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_items ADD COLUMN gsi2_pk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_items ADD COLUMN gsi2_sk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN lsi_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN lsi_range_key_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN lsi_range_key_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN lsi2_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN lsi2_range_key_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_tables ADD COLUMN lsi2_range_key_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_items ADD COLUMN lsi_sk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dynamodb_items ADD COLUMN lsi2_sk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_buckets ADD COLUMN versioning_status TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sqs_messages ADD COLUMN message_group_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sqs_messages ADD COLUMN message_deduplication_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sqs_messages ADD COLUMN sequence_number INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE s3_buckets ADD COLUMN default_encryption_algorithm TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_buckets ADD COLUMN default_encryption_kms_key_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_buckets ADD COLUMN notification_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_objects ADD COLUMN sse_kms_context TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_multipart_uploads ADD COLUMN sse_kms_context TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_object_versions ADD COLUMN sse_kms_context TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_objects ADD COLUMN canned_acl TEXT NOT NULL DEFAULT 'private'`,
		`ALTER TABLE s3_object_versions ADD COLUMN canned_acl TEXT NOT NULL DEFAULT 'private'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_s3_buckets_name ON s3_buckets(name)`,
	}
	cache := map[string]map[string]bool{}
	for _, stmt := range alters {
		if err := execMigrateStmt(s.db, stmt, cache); err != nil {
			return fmt.Errorf("migrate schema: %w", err)
		}
	}
	return nil
}

func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists")
}

func isNoSuchTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table")
}

func isSafeSQLiteIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

// parseAlterAddColumn extracts table and column from ALTER TABLE t ADD COLUMN c ...
func parseAlterAddColumn(stmt string) (table, column string, ok bool) {
	stmt = strings.TrimSpace(stmt)
	upper := strings.ToUpper(stmt)
	const prefix = "ALTER TABLE "
	if !strings.HasPrefix(upper, prefix) {
		return "", "", false
	}
	rest := strings.TrimSpace(stmt[len(prefix):])
	idx := strings.Index(strings.ToUpper(rest), " ADD COLUMN ")
	if idx < 0 {
		return "", "", false
	}
	table = strings.TrimSpace(rest[:idx])
	after := strings.TrimSpace(rest[idx+len(" ADD COLUMN "):])
	fields := strings.Fields(after)
	if len(fields) < 1 {
		return "", "", false
	}
	column = fields[0]
	if !isSafeSQLiteIdent(table) || !isSafeSQLiteIdent(column) {
		return "", "", false
	}
	return table, column, true
}

func sqliteColumnSet(db *sql.DB, table string) (map[string]bool, error) {
	if !isSafeSQLiteIdent(table) {
		return nil, fmt.Errorf("sqlite column set: unsafe table %q", table)
	}
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		if isNoSuchTableErr(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
}

func cachedColumnSet(db *sql.DB, table string, cache map[string]map[string]bool) (map[string]bool, error) {
	if cache != nil {
		if cols, ok := cache[table]; ok {
			return cols, nil
		}
	}
	cols, err := sqliteColumnSet(db, table)
	if err != nil {
		return nil, err
	}
	if cache != nil {
		cache[table] = cols
	}
	return cols, nil
}

// execMigrateStmt runs additive DDL. ALTER TABLE ... ADD COLUMN is skipped when the
// column already exists (avoids expensive parse+error on every Open under -race tests).
func execMigrateStmt(db *sql.DB, stmt string, cache map[string]map[string]bool) error {
	stmt = strings.TrimSpace(stmt)
	if table, col, ok := parseAlterAddColumn(stmt); ok {
		cols, err := cachedColumnSet(db, table, cache)
		if err != nil {
			return err
		}
		if cols[strings.ToLower(col)] {
			return nil
		}
		if _, err := db.Exec(stmt); err != nil {
			if isDuplicateColumnErr(err) || isNoSuchTableErr(err) {
				return nil
			}
			return err
		}
		if cache != nil {
			if cache[table] == nil {
				cache[table] = map[string]bool{}
			}
			cache[table][strings.ToLower(col)] = true
		}
		return nil
	}
	if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) && !isNoSuchTableErr(err) {
		return err
	}
	return nil
}

// execMigrateStmts runs stmts with a shared per-call column cache.
func execMigrateStmts(db *sql.DB, stmts []string) error {
	cache := map[string]map[string]bool{}
	for _, stmt := range stmts {
		if err := execMigrateStmt(db, stmt, cache); err != nil {
			return err
		}
	}
	return nil
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

// LookupAccessKey is a thin wrapper for long-lived credential callers.
func (s *Store) LookupAccessKey(accessKeyID string) (accountID, secret string, isRoot bool, err error) {
	ak, err := s.LookupAccessKeyRecord(accessKeyID)
	if err != nil {
		return "", "", false, err
	}
	return ak.AccountID, ak.Secret, ak.IsRoot, nil
}
