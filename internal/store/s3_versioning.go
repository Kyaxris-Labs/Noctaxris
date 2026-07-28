package store

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
  sse_kms_context TEXT NOT NULL DEFAULT '',
  canned_acl TEXT NOT NULL DEFAULT 'private',
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
	if err := execMigrateStmt(db, `ALTER TABLE s3_object_versions ADD COLUMN is_delete_marker INTEGER NOT NULL DEFAULT 0`, nil); err != nil {
		return fmt.Errorf("ensure s3 versioning schema: migrate is_delete_marker: %w", err)
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
	VersionID      string
	IsLatest       bool
	IsDeleteMarker bool
}

// DeleteObjectVersionResult is returned from version-aware DeleteObject.
type DeleteObjectVersionResult struct {
	VersionID    string
	DeleteMarker bool
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
	if err := s.applyObjectLockMeta(accountID, bucket, &meta); err != nil {
		return ObjectMeta{}, "", err
	}

	unlock := s.lockS3Object(accountID, bucket, key)
	held := true
	defer func() {
		if held {
			unlock()
		}
	}()

	versionID := "null"
	if status == VersioningEnabled {
		versionID = uuid.NewString()
	}

	rel, abs, err := s3ObjectAbsPath(s.dataRoot, accountID, bucket, key+"."+versionID)
	if err != nil {
		return ObjectMeta{}, "", err
	}
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
	acl := normalizeCannedACL(meta.CannedACL)
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
		 (account_id, bucket, key, version_id, is_latest, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek, sse_kms_context, canned_acl, storage_path, last_modified,
		  object_lock_mode, object_lock_retain_until)
		 VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, bucket, key, version_id) DO UPDATE SET
		   is_latest = 1,
		   etag = excluded.etag,
		   size = excluded.size,
		   content_type = excluded.content_type,
		   sse_algorithm = excluded.sse_algorithm,
		   kms_key_id = excluded.kms_key_id,
		   sealed_dek = excluded.sealed_dek,
		   sse_kms_context = excluded.sse_kms_context,
		   canned_acl = excluded.canned_acl,
		   storage_path = excluded.storage_path,
		   last_modified = excluded.last_modified,
		   object_lock_mode = excluded.object_lock_mode,
		   object_lock_retain_until = excluded.object_lock_retain_until`,
		accountID, bucket, key, versionID, etag, size, ct, meta.SSEAlgorithm, meta.KMSKeyID, meta.SealedDEK, meta.SSEKMSContextJSON, acl, rel, modified,
		meta.ObjectLockMode, meta.ObjectLockRetainUntil,
	); err != nil {
		_ = os.Remove(tmp)
		return ObjectMeta{}, "", fmt.Errorf("put object version insert: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO s3_objects
		 (account_id, bucket, key, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek, sse_kms_context, canned_acl, storage_path, last_modified,
		  object_lock_mode, object_lock_retain_until)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, bucket, key) DO UPDATE SET
		   etag = excluded.etag,
		   size = excluded.size,
		   content_type = excluded.content_type,
		   sse_algorithm = excluded.sse_algorithm,
		   kms_key_id = excluded.kms_key_id,
		   sealed_dek = excluded.sealed_dek,
		   sse_kms_context = excluded.sse_kms_context,
		   canned_acl = excluded.canned_acl,
		   storage_path = excluded.storage_path,
		   last_modified = excluded.last_modified,
		   object_lock_mode = excluded.object_lock_mode,
		   object_lock_retain_until = excluded.object_lock_retain_until`,
		accountID, bucket, key, etag, size, ct, meta.SSEAlgorithm, meta.KMSKeyID, meta.SealedDEK, meta.SSEKMSContextJSON, acl, rel, modified,
		meta.ObjectLockMode, meta.ObjectLockRetainUntil,
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
	out := ObjectMeta{
		AccountID: accountID, Bucket: bucket, Key: key, ETag: etag, Size: size,
		ContentType: ct, SSEAlgorithm: meta.SSEAlgorithm, KMSKeyID: meta.KMSKeyID,
		SealedDEK: meta.SealedDEK, SSEKMSContextJSON: meta.SSEKMSContextJSON,
		CannedACL: acl, StoragePath: rel, LastModified: modified,
	}
	eventName := strings.TrimSpace(meta.NotificationEventName)
	if eventName == "" {
		eventName = "ObjectCreated:Put"
	}
	emitVersionID := versionID
	if emitVersionID == "null" {
		emitVersionID = ""
	}
	held = false
	unlock()
	s.emitS3EventNotifications(accountID, DefaultEventsRegion, bucket, key, eventName, emitVersionID, size, etag)
	return out, versionID, nil
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
	status, err := s.GetBucketVersioning(accountID, bucket)
	if err != nil {
		return ObjectMeta{}, "", nil, err
	}
	if versionID == "" && status == "" {
		meta, data, err := s.GetObject(accountID, bucket, key)
		if err != nil {
			return ObjectMeta{}, "", nil, err
		}
		return meta, "", data, nil
	}

	var (
		vid           string
		isDelete      int
		meta          ObjectMeta
		sealedDEK     []byte
		sseCtx, acl   string
		storagePath   string
	)
	if versionID == "" {
		err = s.db.QueryRow(
			`SELECT version_id, is_delete_marker, account_id, bucket, key, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek,
			        COALESCE(sse_kms_context, ''), COALESCE(canned_acl, 'private'), storage_path, last_modified
			 FROM s3_object_versions
			 WHERE account_id = ? AND bucket = ? AND key = ? AND is_latest = 1`,
			accountID, bucket, key,
		).Scan(&vid, &isDelete, &meta.AccountID, &meta.Bucket, &meta.Key, &meta.ETag, &meta.Size, &meta.ContentType,
			&meta.SSEAlgorithm, &meta.KMSKeyID, &sealedDEK, &sseCtx, &acl, &storagePath, &meta.LastModified)
	} else {
		err = s.db.QueryRow(
			`SELECT version_id, is_delete_marker, account_id, bucket, key, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek,
			        COALESCE(sse_kms_context, ''), COALESCE(canned_acl, 'private'), storage_path, last_modified
			 FROM s3_object_versions WHERE account_id = ? AND bucket = ? AND key = ? AND version_id = ?`,
			accountID, bucket, key, versionID,
		).Scan(&vid, &isDelete, &meta.AccountID, &meta.Bucket, &meta.Key, &meta.ETag, &meta.Size, &meta.ContentType,
			&meta.SSEAlgorithm, &meta.KMSKeyID, &sealedDEK, &sseCtx, &acl, &storagePath, &meta.LastModified)
	}
	if err == sql.ErrNoRows {
		return ObjectMeta{}, "", nil, ErrNoSuchKey
	}
	if err != nil {
		return ObjectMeta{}, "", nil, fmt.Errorf("get object version: %w", err)
	}
	if isDelete == 1 {
		return ObjectMeta{}, "", nil, ErrNoSuchKey
	}
	meta.SealedDEK = sealedDEK
	meta.SSEKMSContextJSON = sseCtx
	meta.CannedACL = normalizeCannedACL(acl)
	meta.StoragePath = storagePath
	data, err := os.ReadFile(filepath.Join(s.dataRoot, meta.StoragePath))
	if err != nil {
		return ObjectMeta{}, "", nil, fmt.Errorf("get object version read: %w", err)
	}
	return meta, vid, data, nil
}

// ListObjectVersions returns versions for a bucket, optionally filtered by prefix.
func (s *Store) ListObjectVersions(accountID, bucket, prefix string) ([]ObjectVersion, error) {
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return nil, err
	}
	query := `SELECT account_id, bucket, key, version_id, is_latest, is_delete_marker, etag, size, content_type,
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
			isDelete  int
			sealedDEK []byte
		)
		if err := rows.Scan(
			&v.AccountID, &v.Bucket, &v.Key, &v.VersionID, &isLatest, &isDelete, &v.ETag, &v.Size, &v.ContentType,
			&v.SSEAlgorithm, &v.KMSKeyID, &sealedDEK, &v.StoragePath, &v.LastModified,
		); err != nil {
			return nil, fmt.Errorf("list object versions: %w", err)
		}
		v.SealedDEK = sealedDEK
		v.IsLatest = isLatest == 1
		v.IsDeleteMarker = isDelete == 1
		out = append(out, v)
	}
	if out == nil {
		out = []ObjectVersion{}
	}
	return out, rows.Err()
}

// DeleteObjectVersioned removes an object or creates a delete marker on versioned buckets.
func (s *Store) DeleteObjectVersioned(accountID, bucket, key, versionID string) (DeleteObjectVersionResult, error) {
	return s.DeleteObjectVersionedWithOptions(accountID, bucket, key, versionID, DeleteObjectOptions{})
}

// DeleteObjectVersionedWithOptions is DeleteObjectVersioned with Object Lock bypass options.
func (s *Store) DeleteObjectVersionedWithOptions(accountID, bucket, key, versionID string, opts DeleteObjectOptions) (DeleteObjectVersionResult, error) {
	status, err := s.GetBucketVersioning(accountID, bucket)
	if err != nil {
		return DeleteObjectVersionResult{}, err
	}
	if status == "" {
		if err := s.DeleteObjectWithOptions(accountID, bucket, key, opts); err != nil {
			if errors.Is(err, ErrNoSuchKey) || errors.Is(err, ErrInvalidObjectKey) {
				return DeleteObjectVersionResult{}, nil
			}
			return DeleteObjectVersionResult{}, err
		}
		return DeleteObjectVersionResult{}, nil
	}

	key, err = SanitizeObjectKey(key)
	if err != nil {
		return DeleteObjectVersionResult{}, err
	}

	if err := s.objectLockBlocksDelete(accountID, bucket, key, opts.BypassGovernanceRetention); err != nil {
		return DeleteObjectVersionResult{}, err
	}

	unlock := s.lockS3Object(accountID, bucket, key)
	held := true
	defer func() {
		if held {
			unlock()
		}
	}()

	if versionID != "" {
		if err := s.deleteObjectVersionByID(accountID, bucket, key, versionID); err != nil {
			return DeleteObjectVersionResult{}, err
		}
		held = false
		unlock()
		return DeleteObjectVersionResult{}, nil
	}
	result, err := s.createDeleteMarker(accountID, bucket, key, status)
	if err != nil {
		return DeleteObjectVersionResult{}, err
	}
	held = false
	unlock()
	s.emitS3EventNotifications(accountID, DefaultEventsRegion, bucket, key, "ObjectRemoved:DeleteMarkerCreated", result.VersionID, 0, "")
	return result, nil
}

func (s *Store) deleteObjectVersionByID(accountID, bucket, key, versionID string) error {
	var (
		isLatest      int
		isDelete      int
		storagePath   string
		wasLatest     bool
	)
	err := s.db.QueryRow(
		`SELECT is_latest, is_delete_marker, storage_path FROM s3_object_versions
		 WHERE account_id = ? AND bucket = ? AND key = ? AND version_id = ?`,
		accountID, bucket, key, versionID,
	).Scan(&isLatest, &isDelete, &storagePath)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete object version lookup: %w", err)
	}
	wasLatest = isLatest == 1

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM s3_object_versions WHERE account_id = ? AND bucket = ? AND key = ? AND version_id = ?`,
		accountID, bucket, key, versionID,
	); err != nil {
		return fmt.Errorf("delete object version row: %w", err)
	}

	if wasLatest {
		if err := s.promoteNextObjectVersionTx(tx, accountID, bucket, key); err != nil {
			return err
		}
		if err := s.syncS3ObjectFromLatestVersionTx(tx, accountID, bucket, key); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	if isDelete == 0 && storagePath != "" {
		_ = os.Remove(filepath.Join(s.dataRoot, storagePath))
	}
	return nil
}

func (s *Store) createDeleteMarker(accountID, bucket, key, versioningStatus string) (DeleteObjectVersionResult, error) {
	markerID := uuid.NewString()
	if versioningStatus == VersioningSuspended {
		markerID = "null"
	}
	modified := nowRFC3339()

	tx, err := s.db.Begin()
	if err != nil {
		return DeleteObjectVersionResult{}, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE s3_object_versions SET is_latest = 0 WHERE account_id = ? AND bucket = ? AND key = ?`,
		accountID, bucket, key,
	); err != nil {
		return DeleteObjectVersionResult{}, fmt.Errorf("delete marker clear latest: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO s3_object_versions
		 (account_id, bucket, key, version_id, is_latest, is_delete_marker, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek, sse_kms_context, canned_acl, storage_path, last_modified)
		 VALUES (?, ?, ?, ?, 1, 1, '', 0, '', '', '', NULL, '', 'private', '', ?)
		 ON CONFLICT(account_id, bucket, key, version_id) DO UPDATE SET
		   is_latest = 1,
		   is_delete_marker = 1,
		   etag = '',
		   size = 0,
		   content_type = '',
		   storage_path = '',
		   last_modified = excluded.last_modified`,
		accountID, bucket, key, markerID, modified,
	); err != nil {
		return DeleteObjectVersionResult{}, fmt.Errorf("delete marker insert: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM s3_objects WHERE account_id = ? AND bucket = ? AND key = ?`,
		accountID, bucket, key,
	); err != nil {
		return DeleteObjectVersionResult{}, fmt.Errorf("delete marker hide current: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return DeleteObjectVersionResult{}, err
	}
	return DeleteObjectVersionResult{VersionID: markerID, DeleteMarker: true}, nil
}

func (s *Store) promoteNextObjectVersionTx(tx *sql.Tx, accountID, bucket, key string) error {
	var nextID string
	err := tx.QueryRow(
		`SELECT version_id FROM s3_object_versions
		 WHERE account_id = ? AND bucket = ? AND key = ?
		 ORDER BY last_modified DESC, version_id DESC LIMIT 1`,
		accountID, bucket, key,
	).Scan(&nextID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("promote next object version: %w", err)
	}
	_, err = tx.Exec(
		`UPDATE s3_object_versions SET is_latest = 0 WHERE account_id = ? AND bucket = ? AND key = ?`,
		accountID, bucket, key,
	)
	if err != nil {
		return fmt.Errorf("promote next object version clear: %w", err)
	}
	_, err = tx.Exec(
		`UPDATE s3_object_versions SET is_latest = 1 WHERE account_id = ? AND bucket = ? AND key = ? AND version_id = ?`,
		accountID, bucket, key, nextID,
	)
	if err != nil {
		return fmt.Errorf("promote next object version set: %w", err)
	}
	return nil
}

func (s *Store) syncS3ObjectFromLatestVersionTx(tx *sql.Tx, accountID, bucket, key string) error {
	var (
		isDelete    int
		etag        string
		size        int64
		ct, sse, kms string
		sealedDEK   []byte
		sseCtx, acl string
		storagePath string
		modified    string
	)
	err := tx.QueryRow(
		`SELECT is_delete_marker, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek,
		        COALESCE(sse_kms_context, ''), COALESCE(canned_acl, 'private'), storage_path, last_modified
		 FROM s3_object_versions
		 WHERE account_id = ? AND bucket = ? AND key = ? AND is_latest = 1`,
		accountID, bucket, key,
	).Scan(&isDelete, &etag, &size, &ct, &sse, &kms, &sealedDEK, &sseCtx, &acl, &storagePath, &modified)
	if err == sql.ErrNoRows {
		_, err = tx.Exec(`DELETE FROM s3_objects WHERE account_id = ? AND bucket = ? AND key = ?`, accountID, bucket, key)
		return err
	}
	if err != nil {
		return fmt.Errorf("sync s3 object from version: %w", err)
	}
	if isDelete == 1 {
		_, err = tx.Exec(`DELETE FROM s3_objects WHERE account_id = ? AND bucket = ? AND key = ?`, accountID, bucket, key)
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO s3_objects
		 (account_id, bucket, key, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek, sse_kms_context, canned_acl, storage_path, last_modified)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, bucket, key) DO UPDATE SET
		   etag = excluded.etag,
		   size = excluded.size,
		   content_type = excluded.content_type,
		   sse_algorithm = excluded.sse_algorithm,
		   kms_key_id = excluded.kms_key_id,
		   sealed_dek = excluded.sealed_dek,
		   sse_kms_context = excluded.sse_kms_context,
		   canned_acl = excluded.canned_acl,
		   storage_path = excluded.storage_path,
		   last_modified = excluded.last_modified`,
		accountID, bucket, key, etag, size, ct, sse, kms, sealedDEK, sseCtx, normalizeCannedACL(acl), storagePath, modified,
	)
	return err
}
