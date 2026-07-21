package store

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const (
	VersioningEnabled   = "Enabled"
	VersioningSuspended = "Suspended"
)

// EnsureS3VersioningSchema creates the object versions table.
func EnsureS3VersioningSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure s3 versioning schema: db is nil")
	}
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS s3_object_versions (
  account_id TEXT NOT NULL,
  bucket TEXT NOT NULL,
  key TEXT NOT NULL,
  version_id TEXT NOT NULL,
  is_latest INTEGER NOT NULL DEFAULT 0,
  etag TEXT NOT NULL,
  size INTEGER NOT NULL,
  content_type TEXT,
  sse_algorithm TEXT,
  kms_key_id TEXT,
  sealed_dek BLOB,
  storage_path TEXT NOT NULL,
  last_modified TEXT NOT NULL,
  PRIMARY KEY (account_id, bucket, key, version_id)
);
CREATE INDEX IF NOT EXISTS idx_s3_object_versions_latest
  ON s3_object_versions(account_id, bucket, key, is_latest);
`)
	if err != nil {
		return fmt.Errorf("ensure s3 versioning schema: %w", err)
	}
	return nil
}

// EnsureS3VersioningSchema ensures versioning tables on an open store.
func (s *Store) EnsureS3VersioningSchema() error {
	return EnsureS3VersioningSchema(s.db)
}

// SetBucketVersioning sets Enabled or Suspended. Empty clears never-enabled state only when already empty.
func (s *Store) SetBucketVersioning(accountID, bucket, status string) error {
	switch status {
	case VersioningEnabled, VersioningSuspended:
	default:
		return fmt.Errorf("set bucket versioning: invalid status %q", status)
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`UPDATE s3_buckets SET versioning_status = ? WHERE account_id = ? AND name = ?`,
		status, accountID, bucket,
	)
	if err != nil {
		return fmt.Errorf("set bucket versioning: %w", err)
	}
	return nil
}

// GetBucketVersioning returns versioning status (empty when never enabled).
func (s *Store) GetBucketVersioning(accountID, bucket string) (string, error) {
	var status string
	err := s.db.QueryRow(
		`SELECT COALESCE(versioning_status, '') FROM s3_buckets WHERE account_id = ? AND name = ?`,
		accountID, bucket,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return "", ErrNoSuchBucket
	}
	if err != nil {
		return "", fmt.Errorf("get bucket versioning: %w", err)
	}
	return status, nil
}

// ObjectVersion is a versioned object row.
type ObjectVersion struct {
	ObjectMeta
	VersionID string
	IsLatest  bool
}

// PutObjectVersioned writes a new version when versioning is Enabled or Suspended.
// When versioning was never enabled, falls through to PutObject.
func (s *Store) PutObjectVersioned(accountID, bucket, key string, meta PutObjectMeta) (ObjectMeta, string, error) {
	status, err := s.GetBucketVersioning(accountID, bucket)
	if err != nil {
		return ObjectMeta{}, "", err
	}
	if status == "" {
		out, err := s.PutObject(accountID, bucket, key, meta)
		return out, "", err
	}

	key, err = SanitizeObjectKey(key)
	if err != nil {
		return ObjectMeta{}, "", err
	}

	unlock := s.lockS3Object(accountID, bucket, key)
	defer unlock()

	versionID := "null"
	if status == VersioningEnabled {
		versionID = uuid.NewString()
	}

	rel := filepath.Join("s3", accountID, bucket, filepath.FromSlash(key)+"."+versionID)
	abs := filepath.Join(s.dataRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return ObjectMeta{}, "", fmt.Errorf("put object version mkdir: %w", err)
	}
	tmp := abs + ".tmp-" + uuid.NewString()
	if err := os.WriteFile(tmp, meta.Data, 0o600); err != nil {
		return ObjectMeta{}, "", fmt.Errorf("put object version write: %w", err)
	}

	etag := meta.ETag
	if etag == "" {
		sum := md5SumHex(meta.Data)
		etag = sum
	}
	size := meta.PlainSize
	if size == 0 && len(meta.Data) > 0 && meta.SSEAlgorithm == "" {
		size = int64(len(meta.Data))
	}
	ct := meta.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	modified := nowRFC3339()

	tx, err := s.db.Begin()
	if err != nil {
		_ = os.Remove(tmp)
		return ObjectMeta{}, "", err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE s3_object_versions SET is_latest = 0 WHERE account_id = ? AND bucket = ? AND key = ?`,
		accountID, bucket, key,
	); err != nil {
		_ = os.Remove(tmp)
		return ObjectMeta{}, "", fmt.Errorf("put object version clear latest: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO s3_object_versions
		 (account_id, bucket, key, version_id, is_latest, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek, storage_path, last_modified)
		 VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, bucket, key, version_id) DO UPDATE SET
		   is_latest = 1,
		   etag = excluded.etag,
		   size = excluded.size,
		   content_type = excluded.content_type,
		   sse_algorithm = excluded.sse_algorithm,
		   kms_key_id = excluded.kms_key_id,
		   sealed_dek = excluded.sealed_dek,
		   storage_path = excluded.storage_path,
		   last_modified = excluded.last_modified`,
		accountID, bucket, key, versionID, etag, size, ct, meta.SSEAlgorithm, meta.KMSKeyID, meta.SealedDEK, rel, modified,
	); err != nil {
		_ = os.Remove(tmp)
		return ObjectMeta{}, "", fmt.Errorf("put object version insert: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO s3_objects
		 (account_id, bucket, key, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek, storage_path, last_modified)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, bucket, key) DO UPDATE SET
		   etag = excluded.etag,
		   size = excluded.size,
		   content_type = excluded.content_type,
		   sse_algorithm = excluded.sse_algorithm,
		   kms_key_id = excluded.kms_key_id,
		   sealed_dek = excluded.sealed_dek,
		   storage_path = excluded.storage_path,
		   last_modified = excluded.last_modified`,
		accountID, bucket, key, etag, size, ct, meta.SSEAlgorithm, meta.KMSKeyID, meta.SealedDEK, rel, modified,
	); err != nil {
		_ = os.Remove(tmp)
		return ObjectMeta{}, "", fmt.Errorf("put object version current: %w", err)
	}
	if err := tx.Commit(); err != nil {
		_ = os.Remove(tmp)
		return ObjectMeta{}, "", err
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		return ObjectMeta{}, "", fmt.Errorf("put object version rename: %w", err)
	}
	return ObjectMeta{
		AccountID: accountID, Bucket: bucket, Key: key, ETag: etag, Size: size,
		ContentType: ct, SSEAlgorithm: meta.SSEAlgorithm, KMSKeyID: meta.KMSKeyID,
		SealedDEK: meta.SealedDEK, StoragePath: rel, LastModified: modified,
	}, versionID, nil
}

func md5SumHex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

// GetObjectVersion returns a specific version, or the latest when versionID is empty.
func (s *Store) GetObjectVersion(accountID, bucket, key, versionID string) (ObjectMeta, string, []byte, error) {
	key, err := SanitizeObjectKey(key)
	if err != nil {
		return ObjectMeta{}, "", nil, err
	}
	if versionID == "" {
		meta, data, err := s.GetObject(accountID, bucket, key)
		if err != nil {
			return ObjectMeta{}, "", nil, err
		}
		status, _ := s.GetBucketVersioning(accountID, bucket)
		vid := ""
		if status != "" {
			_ = s.db.QueryRow(
				`SELECT version_id FROM s3_object_versions
				 WHERE account_id = ? AND bucket = ? AND key = ? AND is_latest = 1`,
				accountID, bucket, key,
			).Scan(&vid)
		}
		return meta, vid, data, nil
	}

	var meta ObjectMeta
	var sealedDEK []byte
	err = s.db.QueryRow(
		`SELECT account_id, bucket, key, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek, storage_path, last_modified
		 FROM s3_object_versions WHERE account_id = ? AND bucket = ? AND key = ? AND version_id = ?`,
		accountID, bucket, key, versionID,
	).Scan(&meta.AccountID, &meta.Bucket, &meta.Key, &meta.ETag, &meta.Size, &meta.ContentType,
		&meta.SSEAlgorithm, &meta.KMSKeyID, &sealedDEK, &meta.StoragePath, &meta.LastModified)
	if err == sql.ErrNoRows {
		return ObjectMeta{}, "", nil, ErrNoSuchKey
	}
	if err != nil {
		return ObjectMeta{}, "", nil, fmt.Errorf("get object version: %w", err)
	}
	meta.SealedDEK = sealedDEK
	data, err := os.ReadFile(filepath.Join(s.dataRoot, meta.StoragePath))
	if err != nil {
		return ObjectMeta{}, "", nil, fmt.Errorf("get object version read: %w", err)
	}
	return meta, versionID, data, nil
}

// ListObjectVersions returns versions for a bucket, optionally filtered by prefix.
func (s *Store) ListObjectVersions(accountID, bucket, prefix string) ([]ObjectVersion, error) {
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return nil, err
	}
	query := `SELECT account_id, bucket, key, version_id, is_latest, etag, size, content_type,
	                 sse_algorithm, kms_key_id, sealed_dek, storage_path, last_modified
	          FROM s3_object_versions WHERE account_id = ? AND bucket = ?`
	args := []any{accountID, bucket}
	if prefix != "" {
		query += ` AND key LIKE ?`
		args = append(args, prefix+"%")
	}
	query += ` ORDER BY key, last_modified DESC, version_id`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list object versions: %w", err)
	}
	defer rows.Close()
	var out []ObjectVersion
	for rows.Next() {
		var (
			v         ObjectVersion
			isLatest  int
			sealedDEK []byte
		)
		if err := rows.Scan(
			&v.AccountID, &v.Bucket, &v.Key, &v.VersionID, &isLatest, &v.ETag, &v.Size, &v.ContentType,
			&v.SSEAlgorithm, &v.KMSKeyID, &sealedDEK, &v.StoragePath, &v.LastModified,
		); err != nil {
			return nil, fmt.Errorf("list object versions: %w", err)
		}
		v.SealedDEK = sealedDEK
		v.IsLatest = isLatest == 1
		out = append(out, v)
	}
	if out == nil {
		out = []ObjectVersion{}
	}
	return out, rows.Err()
}
