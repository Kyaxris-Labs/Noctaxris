package store

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidBucketName   = errors.New("InvalidBucketName")
	ErrInvalidObjectKey    = errors.New("InvalidObjectKey")
	ErrBucketAlreadyExists = errors.New("BucketAlreadyExists")
	ErrNoSuchBucket        = errors.New("NoSuchBucket")
	ErrBucketNotEmpty      = errors.New("BucketNotEmpty")
	ErrNoSuchKey           = errors.New("NoSuchKey")
	ErrNoSuchBucketPolicy  = errors.New("NoSuchBucketPolicy")
)

var bucketNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// Bucket is an S3 bucket metadata row.
type Bucket struct {
	AccountID    string
	Name         string
	CreationDate string
	BucketPolicy string
}

// ObjectMeta is S3 object metadata (bytes live on disk at StoragePath).
type ObjectMeta struct {
	AccountID    string
	Bucket       string
	Key          string
	ETag         string
	Size         int64
	ContentType  string
	SSEAlgorithm string
	KMSKeyID     string
	SealedDEK    []byte
	StoragePath  string
	LastModified string
}

// ListObjectsResult is a minimal ListObjectsV2 response payload.
type ListObjectsResult struct {
	Contents       []ObjectMeta
	CommonPrefixes []string
	IsTruncated    bool
	KeyCount       int
}

// ValidateBucketName checks DNS-style lab bucket naming (3–63 chars, lowercase).
func ValidateBucketName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return ErrInvalidBucketName
	}
	if name != strings.ToLower(name) {
		return ErrInvalidBucketName
	}
	if strings.Contains(name, "..") || strings.Contains(name, ".-") || strings.Contains(name, "-.") {
		return ErrInvalidBucketName
	}
	if !bucketNamePattern.MatchString(name) {
		return ErrInvalidBucketName
	}
	return nil
}

// SanitizeObjectKey validates and returns a cleaned object key.
// Rejects empty keys, null bytes, and "." / ".." path segments.
func SanitizeObjectKey(key string) (string, error) {
	if key == "" || !utf8.ValidString(key) {
		return "", ErrInvalidObjectKey
	}
	if strings.ContainsRune(key, 0) {
		return "", ErrInvalidObjectKey
	}
	if len(key) > 1024 {
		return "", ErrInvalidObjectKey
	}
	// Normalize leading slashes away for storage; S3 keys may start with /.
	cleaned := strings.TrimPrefix(key, "/")
	if cleaned == "" {
		return "", ErrInvalidObjectKey
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == "." || seg == ".." {
			return "", ErrInvalidObjectKey
		}
	}
	return cleaned, nil
}

// CreateBucket inserts a new empty bucket.
func (s *Store) CreateBucket(accountID, name string) (Bucket, error) {
	if err := ValidateBucketName(name); err != nil {
		return Bucket{}, err
	}
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO s3_buckets (account_id, name, creation_date, bucket_policy) VALUES (?, ?, ?, '')`,
		accountID, name, created,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Bucket{}, ErrBucketAlreadyExists
		}
		return Bucket{}, fmt.Errorf("create bucket: %w", err)
	}
	dir := filepath.Join(s.dataRoot, "s3", accountID, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Bucket{}, fmt.Errorf("create bucket dir: %w", err)
	}
	return Bucket{AccountID: accountID, Name: name, CreationDate: created}, nil
}

// DeleteBucket removes an empty bucket.
func (s *Store) DeleteBucket(accountID, name string) error {
	if err := ValidateBucketName(name); err != nil {
		return err
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM s3_objects WHERE account_id = ? AND bucket = ?`,
		accountID, name,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("delete bucket: count objects: %w", err)
	}
	if n > 0 {
		return ErrBucketNotEmpty
	}
	res, err := s.db.Exec(
		`DELETE FROM s3_buckets WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchBucket
	}
	_ = os.RemoveAll(filepath.Join(s.dataRoot, "s3", accountID, name))
	return nil
}

// ListBuckets returns all buckets for an account.
func (s *Store) ListBuckets(accountID string) ([]Bucket, error) {
	rows, err := s.db.Query(
		`SELECT account_id, name, creation_date, bucket_policy FROM s3_buckets
		 WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	defer rows.Close()
	var out []Bucket
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.AccountID, &b.Name, &b.CreationDate, &b.BucketPolicy); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBucket returns bucket metadata or ErrNoSuchBucket.
func (s *Store) GetBucket(accountID, name string) (Bucket, error) {
	var b Bucket
	err := s.db.QueryRow(
		`SELECT account_id, name, creation_date, bucket_policy FROM s3_buckets
		 WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&b.AccountID, &b.Name, &b.CreationDate, &b.BucketPolicy)
	if errors.Is(err, sql.ErrNoRows) {
		return Bucket{}, ErrNoSuchBucket
	}
	if err != nil {
		return Bucket{}, fmt.Errorf("get bucket: %w", err)
	}
	return b, nil
}

// HeadBucket reports whether the bucket exists.
func (s *Store) HeadBucket(accountID, name string) error {
	_, err := s.GetBucket(accountID, name)
	return err
}

// PutBucketPolicy replaces the bucket policy document.
func (s *Store) PutBucketPolicy(accountID, name, policy string) error {
	res, err := s.db.Exec(
		`UPDATE s3_buckets SET bucket_policy = ? WHERE account_id = ? AND name = ?`,
		policy, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("put bucket policy: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchBucket
	}
	return nil
}

// GetBucketPolicy returns the stored policy or ErrNoSuchBucketPolicy when empty.
func (s *Store) GetBucketPolicy(accountID, name string) (string, error) {
	b, err := s.GetBucket(accountID, name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(b.BucketPolicy) == "" {
		return "", ErrNoSuchBucketPolicy
	}
	return b.BucketPolicy, nil
}

// DeleteBucketPolicy clears the bucket policy.
func (s *Store) DeleteBucketPolicy(accountID, name string) error {
	res, err := s.db.Exec(
		`UPDATE s3_buckets SET bucket_policy = '' WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete bucket policy: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchBucket
	}
	return nil
}

// PutObjectMeta describes bytes to persist for PutObject.
type PutObjectMeta struct {
	ContentType  string
	SSEAlgorithm string
	KMSKeyID     string
	SealedDEK    []byte
	// Data is the on-disk payload (ciphertext when SSE applies, else plaintext).
	Data []byte
	// PlainSize is the logical object size reported to clients.
	PlainSize int64
	// ETag overrides MD5 of Data when set (typically MD5 of plaintext).
	ETag string
}

// PutObject writes object bytes under dataRoot and upserts metadata.
func (s *Store) PutObject(accountID, bucket, key string, meta PutObjectMeta) (ObjectMeta, error) {
	if err := ValidateBucketName(bucket); err != nil {
		return ObjectMeta{}, err
	}
	key, err := SanitizeObjectKey(key)
	if err != nil {
		return ObjectMeta{}, err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return ObjectMeta{}, err
	}

	rel := filepath.Join("s3", accountID, bucket, filepath.FromSlash(key))
	abs := filepath.Join(s.dataRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return ObjectMeta{}, fmt.Errorf("put object mkdir: %w", err)
	}
	if err := os.WriteFile(abs, meta.Data, 0o600); err != nil {
		return ObjectMeta{}, fmt.Errorf("put object write: %w", err)
	}

	etag := meta.ETag
	if etag == "" {
		sum := md5.Sum(meta.Data)
		etag = hex.EncodeToString(sum[:])
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

	_, err = s.db.Exec(
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
	)
	if err != nil {
		return ObjectMeta{}, fmt.Errorf("put object meta: %w", err)
	}
	return ObjectMeta{
		AccountID:    accountID,
		Bucket:       bucket,
		Key:          key,
		ETag:         etag,
		Size:         size,
		ContentType:  ct,
		SSEAlgorithm: meta.SSEAlgorithm,
		KMSKeyID:     meta.KMSKeyID,
		SealedDEK:    meta.SealedDEK,
		StoragePath:  rel,
		LastModified: modified,
	}, nil
}

// GetObject returns metadata and on-disk bytes (ciphertext when SSE applies).
func (s *Store) GetObject(accountID, bucket, key string) (ObjectMeta, []byte, error) {
	meta, err := s.HeadObject(accountID, bucket, key)
	if err != nil {
		return ObjectMeta{}, nil, err
	}
	data, err := os.ReadFile(filepath.Join(s.dataRoot, meta.StoragePath))
	if err != nil {
		return ObjectMeta{}, nil, fmt.Errorf("get object read: %w", err)
	}
	return meta, data, nil
}

// HeadObject returns object metadata without reading bytes.
func (s *Store) HeadObject(accountID, bucket, key string) (ObjectMeta, error) {
	key, err := SanitizeObjectKey(key)
	if err != nil {
		return ObjectMeta{}, err
	}
	var m ObjectMeta
	var ct, sse, kmsID sql.NullString
	var sealed []byte
	err = s.db.QueryRow(
		`SELECT account_id, bucket, key, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek, storage_path, last_modified
		 FROM s3_objects WHERE account_id = ? AND bucket = ? AND key = ?`,
		accountID, bucket, key,
	).Scan(&m.AccountID, &m.Bucket, &m.Key, &m.ETag, &m.Size, &ct, &sse, &kmsID, &sealed, &m.StoragePath, &m.LastModified)
	if errors.Is(err, sql.ErrNoRows) {
		return ObjectMeta{}, ErrNoSuchKey
	}
	if err != nil {
		return ObjectMeta{}, fmt.Errorf("head object: %w", err)
	}
	if ct.Valid {
		m.ContentType = ct.String
	}
	if sse.Valid {
		m.SSEAlgorithm = sse.String
	}
	if kmsID.Valid {
		m.KMSKeyID = kmsID.String
	}
	m.SealedDEK = sealed
	return m, nil
}

// DeleteObject removes object metadata and file bytes.
func (s *Store) DeleteObject(accountID, bucket, key string) error {
	key, err := SanitizeObjectKey(key)
	if err != nil {
		return err
	}
	meta, err := s.HeadObject(accountID, bucket, key)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`DELETE FROM s3_objects WHERE account_id = ? AND bucket = ? AND key = ?`,
		accountID, bucket, key,
	)
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	_ = os.Remove(filepath.Join(s.dataRoot, meta.StoragePath))
	return nil
}

// ListObjectsV2 returns objects under optional prefix with optional delimiter grouping.
func (s *Store) ListObjectsV2(accountID, bucket, prefix, delimiter string) (ListObjectsResult, error) {
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return ListObjectsResult{}, err
	}
	rows, err := s.db.Query(
		`SELECT account_id, bucket, key, etag, size, content_type, sse_algorithm, kms_key_id, sealed_dek, storage_path, last_modified
		 FROM s3_objects WHERE account_id = ? AND bucket = ? AND key LIKE ? ORDER BY key`,
		accountID, bucket, prefix+"%",
	)
	if err != nil {
		return ListObjectsResult{}, fmt.Errorf("list objects: %w", err)
	}
	defer rows.Close()

	var result ListObjectsResult
	seenPrefix := map[string]struct{}{}
	for rows.Next() {
		var m ObjectMeta
		var ct, sse, kmsID sql.NullString
		var sealed []byte
		if err := rows.Scan(&m.AccountID, &m.Bucket, &m.Key, &m.ETag, &m.Size, &ct, &sse, &kmsID, &sealed, &m.StoragePath, &m.LastModified); err != nil {
			return ListObjectsResult{}, err
		}
		if ct.Valid {
			m.ContentType = ct.String
		}
		if sse.Valid {
			m.SSEAlgorithm = sse.String
		}
		if kmsID.Valid {
			m.KMSKeyID = kmsID.String
		}
		m.SealedDEK = sealed

		rest := strings.TrimPrefix(m.Key, prefix)
		if delimiter != "" {
			if i := strings.Index(rest, delimiter); i >= 0 {
				cp := prefix + rest[:i+len(delimiter)]
				if _, ok := seenPrefix[cp]; !ok {
					seenPrefix[cp] = struct{}{}
					result.CommonPrefixes = append(result.CommonPrefixes, cp)
				}
				continue
			}
		}
		result.Contents = append(result.Contents, m)
	}
	if err := rows.Err(); err != nil {
		return ListObjectsResult{}, err
	}
	result.KeyCount = len(result.Contents) + len(result.CommonPrefixes)
	return result, nil
}

// BucketARN builds arn:aws:s3:::bucket.
func BucketARN(bucket string) string {
	return "arn:aws:s3:::" + bucket
}

// ObjectARN builds arn:aws:s3:::bucket/key.
func ObjectARN(bucket, key string) string {
	return "arn:aws:s3:::" + bucket + "/" + key
}
