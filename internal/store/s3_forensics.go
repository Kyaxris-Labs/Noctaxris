package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrObjectLockRetention = errors.New("AccessDenied")
	ErrObjectLockRequired  = errors.New("InvalidRequest")
)

// S3AccessLoggingConfig is the server access log destination for a bucket.
type S3AccessLoggingConfig struct {
	Enabled      bool
	TargetBucket string
	TargetPrefix string
}

// CreateBucketOptions carries optional CreateBucket flags.
type CreateBucketOptions struct {
	ObjectLockEnabled bool
}

// DeleteObjectOptions carries optional DeleteObject flags.
type DeleteObjectOptions struct {
	BypassGovernanceRetention bool
}

// EnsureS3ForensicsSchema adds object lock and access logging columns.
func EnsureS3ForensicsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure s3 forensics schema: db is nil")
	}
	stmts := []string{
		`ALTER TABLE s3_buckets ADD COLUMN object_lock_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE s3_buckets ADD COLUMN access_logging_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_objects ADD COLUMN object_lock_mode TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_objects ADD COLUMN object_lock_retain_until TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_object_versions ADD COLUMN object_lock_mode TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_object_versions ADD COLUMN object_lock_retain_until TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range stmts {
		if err := execMigrateStmt(db, stmt, nil); err != nil {
			return fmt.Errorf("ensure s3 forensics schema: %w", err)
		}
	}
	return nil
}

// CreateBucketWithOptions inserts a bucket with optional Object Lock.
func (s *Store) CreateBucketWithOptions(accountID, name string, opts CreateBucketOptions) (Bucket, error) {
	b, err := s.CreateBucket(accountID, name)
	if err != nil {
		return Bucket{}, err
	}
	if !opts.ObjectLockEnabled {
		return b, nil
	}
	_, err = s.db.Exec(
		`UPDATE s3_buckets SET object_lock_enabled = 1 WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return Bucket{}, fmt.Errorf("create bucket object lock: %w", err)
	}
	return b, nil
}

// BucketObjectLockEnabled reports whether Object Lock was enabled at bucket creation.
func (s *Store) BucketObjectLockEnabled(accountID, bucket string) (bool, error) {
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return false, err
	}
	var enabled int
	err := s.db.QueryRow(
		`SELECT COALESCE(object_lock_enabled, 0) FROM s3_buckets WHERE account_id = ? AND name = ?`,
		accountID, bucket,
	).Scan(&enabled)
	if err != nil {
		return false, fmt.Errorf("bucket object lock: %w", err)
	}
	return enabled != 0, nil
}

func (s *Store) validateObjectLockPut(accountID, bucket, mode, retainUntil string) error {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	retainUntil = strings.TrimSpace(retainUntil)
	if mode == "" && retainUntil == "" {
		return nil
	}
	if mode == "" || retainUntil == "" {
		return fmt.Errorf("%w: object lock mode and retain-until-date are required together", ErrObjectLockRequired)
	}
	if mode != "GOVERNANCE" && mode != "COMPLIANCE" {
		return fmt.Errorf("%w: unsupported object lock mode %q", ErrObjectLockRequired, mode)
	}
	if _, err := time.Parse(time.RFC3339, retainUntil); err != nil {
		return fmt.Errorf("%w: retain-until-date must be RFC3339", ErrObjectLockRequired)
	}
	enabled, err := s.BucketObjectLockEnabled(accountID, bucket)
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf("%w: bucket does not have Object Lock enabled", ErrObjectLockRequired)
	}
	return nil
}

func (s *Store) applyObjectLockMeta(accountID, bucket string, meta *PutObjectMeta) error {
	if meta == nil {
		return nil
	}
	mode := strings.ToUpper(strings.TrimSpace(meta.ObjectLockMode))
	retain := strings.TrimSpace(meta.ObjectLockRetainUntil)
	if err := s.validateObjectLockPut(accountID, bucket, mode, retain); err != nil {
		return err
	}
	meta.ObjectLockMode = mode
	meta.ObjectLockRetainUntil = retain
	return nil
}

func (s *Store) objectLockBlocksDelete(accountID, bucket, key string, bypassGovernance bool) error {
	meta, err := s.HeadObject(accountID, bucket, key)
	if errors.Is(err, ErrNoSuchKey) {
		// Delete markers hide the current object; version-id deletes must still proceed.
		return nil
	}
	if err != nil {
		return err
	}
	mode := strings.ToUpper(strings.TrimSpace(meta.ObjectLockMode))
	retain := strings.TrimSpace(meta.ObjectLockRetainUntil)
	if mode == "" || retain == "" {
		return nil
	}
	until, err := time.Parse(time.RFC3339, retain)
	if err != nil {
		return nil
	}
	if !time.Now().UTC().Before(until) {
		return nil
	}
	if mode == "GOVERNANCE" && bypassGovernance {
		return nil
	}
	return ErrObjectLockRetention
}

// PutBucketLogging sets server access logging on a bucket.
func (s *Store) PutBucketLogging(accountID, bucket string, cfg S3AccessLoggingConfig) error {
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return err
	}
	if cfg.Enabled {
		target := strings.TrimSpace(cfg.TargetBucket)
		if target == "" {
			return fmt.Errorf("%w: TargetBucket is required when logging is enabled", ErrInvalidBucketName)
		}
		if _, err := s.GetBucket(accountID, target); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("put bucket logging: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE s3_buckets SET access_logging_json = ? WHERE account_id = ? AND name = ?`,
		string(raw), accountID, bucket,
	)
	if err != nil {
		return fmt.Errorf("put bucket logging: %w", err)
	}
	return nil
}

// GetBucketLogging returns server access logging configuration.
func (s *Store) GetBucketLogging(accountID, bucket string) (S3AccessLoggingConfig, error) {
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return S3AccessLoggingConfig{}, err
	}
	var raw string
	err := s.db.QueryRow(
		`SELECT COALESCE(access_logging_json, '') FROM s3_buckets WHERE account_id = ? AND name = ?`,
		accountID, bucket,
	).Scan(&raw)
	if err != nil {
		return S3AccessLoggingConfig{}, fmt.Errorf("get bucket logging: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return S3AccessLoggingConfig{}, nil
	}
	var cfg S3AccessLoggingConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return S3AccessLoggingConfig{}, fmt.Errorf("get bucket logging: %w", err)
	}
	return cfg, nil
}

// S3ServerAccessLogInput captures one data-plane request for server access logging.
type S3ServerAccessLogInput struct {
	Operation   string // REST.GET.OBJECT, REST.PUT.OBJECT, REST.DELETE.OBJECT
	Key         string
	RequestID   string
	RemoteIP    string
	Requester   string
	HTTPStatus  int
	BytesSent   int64
	ObjectSize  int64
	RequestTime time.Time
}

// S3ServerAccessLogObjectKey returns a daily log object key under the configured prefix.
func S3ServerAccessLogObjectKey(sourceBucket, prefix string, ts time.Time) string {
	prefix = normalizeAccessLogS3KeyPrefix(prefix)
	ts = ts.UTC()
	day := ts.Format("2006-01-02")
	return fmt.Sprintf("%s%s_%s.log", prefix, sourceBucket, day)
}

// FormatS3ServerAccessLogLine builds a space-separated lite server access log line.
func FormatS3ServerAccessLogLine(ownerAccount, bucket string, in S3ServerAccessLogInput) string {
	if in.RequestTime.IsZero() {
		in.RequestTime = time.Now().UTC()
	}
	if in.HTTPStatus == 0 {
		in.HTTPStatus = 200
	}
	fields := []string{
		ownerAccount,
		bucket,
		in.RequestTime.UTC().Format(time.RFC3339),
		strings.TrimSpace(in.RemoteIP),
		strings.TrimSpace(in.Requester),
		strings.TrimSpace(in.RequestID),
		strings.TrimSpace(in.Operation),
		quoteALBLogField(strings.TrimSpace(in.Key)),
		strconv.Itoa(in.HTTPStatus),
		"-",
		strconv.FormatInt(in.BytesSent, 10),
		strconv.FormatInt(in.ObjectSize, 10),
	}
	return strings.Join(fields, " ")
}

// AppendS3ServerAccessLog appends a line to the configured target bucket when logging is enabled.
func (s *Store) AppendS3ServerAccessLog(accountID, bucket string, in S3ServerAccessLogInput) error {
	cfg, err := s.GetBucketLogging(accountID, bucket)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return nil
	}
	target := strings.TrimSpace(cfg.TargetBucket)
	if target == "" {
		return nil
	}
	if _, err := s.GetBucket(accountID, target); err != nil {
		return err
	}
	if in.RequestTime.IsZero() {
		in.RequestTime = time.Now().UTC()
	}
	key := S3ServerAccessLogObjectKey(bucket, cfg.TargetPrefix, in.RequestTime)
	line := FormatS3ServerAccessLogLine(accountID, bucket, in)
	return s.appendS3TextLine(accountID, target, key, []byte(line))
}
