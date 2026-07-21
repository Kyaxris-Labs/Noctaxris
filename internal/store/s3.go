package store

import (
	"bytes"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

func (s *Store) lockS3Object(accountID, bucket, key string) func() {
	lockKey := accountID + "\x00" + bucket + "\x00" + key
	s.s3ObjectMu.Lock()
	if s.s3ObjectLocks == nil {
		s.s3ObjectLocks = map[string]*s3ObjectLock{}
	}
	entry, ok := s.s3ObjectLocks[lockKey]
	if !ok {
		entry = &s3ObjectLock{}
		s.s3ObjectLocks[lockKey] = entry
	}
	entry.refs++
	s.s3ObjectMu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		s.s3ObjectMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.s3ObjectLocks, lockKey)
		}
		s.s3ObjectMu.Unlock()
	}
}

const (
	sseAES256 = "AES256"
	sseAWSKMS = "aws:kms"
)

var (
	ErrInvalidBucketName   = errors.New("InvalidBucketName")
	ErrInvalidObjectKey    = errors.New("InvalidObjectKey")
	ErrBucketAlreadyExists = errors.New("BucketAlreadyExists")
	ErrNoSuchBucket        = errors.New("NoSuchBucket")
	ErrBucketNotEmpty      = errors.New("BucketNotEmpty")
	ErrNoSuchKey           = errors.New("NoSuchKey")
	ErrNoSuchBucketPolicy  = errors.New("NoSuchBucketPolicy")
	ErrNoSuchUpload        = errors.New("NoSuchUpload")
	ErrInvalidPart         = errors.New("InvalidPart")
	ErrEntityTooSmall           = errors.New("EntityTooSmall")
	ErrNoSuchBucketEncryption   = errors.New("ServerSideEncryptionConfigurationNotFoundError")
	ErrInvalidBucketEncryption  = errors.New("InvalidBucketEncryption")
)

// MinMultipartPartSize is the AWS minimum part size (5 MiB) except the final part.
const MinMultipartPartSize = 5 * 1024 * 1024

var bucketNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// Bucket is an S3 bucket metadata row.
type Bucket struct {
	AccountID    string
	Name         string
	CreationDate string
	BucketPolicy string
}

// BucketEncryption is the default SSE configuration for a bucket.
type BucketEncryption struct {
	Algorithm string
	KMSKeyID  string
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
	if _, err := s.GetBucketByName(name); err == nil {
		return Bucket{}, ErrBucketAlreadyExists
	} else if !errors.Is(err, ErrNoSuchBucket) {
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

// GetBucketByName returns bucket metadata by global lab bucket name.
// Bucket names are unique in the lab; use this to resolve the owner account.
func (s *Store) GetBucketByName(name string) (Bucket, error) {
	if err := ValidateBucketName(name); err != nil {
		return Bucket{}, err
	}
	var b Bucket
	err := s.db.QueryRow(
		`SELECT account_id, name, creation_date, bucket_policy FROM s3_buckets
		 WHERE name = ? LIMIT 1`,
		name,
	).Scan(&b.AccountID, &b.Name, &b.CreationDate, &b.BucketPolicy)
	if errors.Is(err, sql.ErrNoRows) {
		return Bucket{}, ErrNoSuchBucket
	}
	if err != nil {
		return Bucket{}, fmt.Errorf("get bucket by name: %w", err)
	}
	return b, nil
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

// PutBucketEncryption sets the bucket default SSE configuration.
func (s *Store) PutBucketEncryption(accountID, name string, enc BucketEncryption) error {
	algo := strings.TrimSpace(enc.Algorithm)
	switch strings.ToUpper(algo) {
	case "AES256":
		algo = sseAES256
	case "AWS:KMS":
		algo = sseAWSKMS
	default:
		return ErrInvalidBucketEncryption
	}
	kmsKeyID := strings.TrimSpace(enc.KMSKeyID)
	if algo == sseAWSKMS && kmsKeyID == "" {
		return ErrInvalidBucketEncryption
	}
	if algo == sseAES256 {
		kmsKeyID = ""
	}
	res, err := s.db.Exec(
		`UPDATE s3_buckets SET default_encryption_algorithm = ?, default_encryption_kms_key_id = ?
		 WHERE account_id = ? AND name = ?`,
		algo, kmsKeyID, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("put bucket encryption: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchBucket
	}
	return nil
}

// GetBucketEncryption returns the bucket default SSE configuration.
func (s *Store) GetBucketEncryption(accountID, name string) (BucketEncryption, error) {
	var algo, kmsID sql.NullString
	err := s.db.QueryRow(
		`SELECT default_encryption_algorithm, default_encryption_kms_key_id FROM s3_buckets
		 WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&algo, &kmsID)
	if errors.Is(err, sql.ErrNoRows) {
		return BucketEncryption{}, ErrNoSuchBucket
	}
	if err != nil {
		return BucketEncryption{}, fmt.Errorf("get bucket encryption: %w", err)
	}
	if !algo.Valid || strings.TrimSpace(algo.String) == "" {
		return BucketEncryption{}, ErrNoSuchBucketEncryption
	}
	out := BucketEncryption{Algorithm: algo.String}
	if kmsID.Valid {
		out.KMSKeyID = kmsID.String
	}
	return out, nil
}

// DeleteBucketEncryption removes the bucket default SSE configuration.
func (s *Store) DeleteBucketEncryption(accountID, name string) error {
	res, err := s.db.Exec(
		`UPDATE s3_buckets SET default_encryption_algorithm = '', default_encryption_kms_key_id = ''
		 WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete bucket encryption: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchBucket
	}
	return nil
}

// CopyObject stores dest from caller-supplied payload and metadata (same account).
func (s *Store) CopyObject(accountID, srcBucket, srcKey, destBucket, destKey string, dest PutObjectMeta) (ObjectMeta, error) {
	srcMeta, srcData, err := s.GetObject(accountID, srcBucket, srcKey)
	if err != nil {
		return ObjectMeta{}, err
	}
	if dest.Data == nil {
		dest.Data = srcData
	}
	if dest.ContentType == "" {
		dest.ContentType = srcMeta.ContentType
	}
	if dest.PlainSize == 0 {
		dest.PlainSize = srcMeta.Size
	}
	out, _, err := s.PutObjectVersioned(accountID, destBucket, destKey, dest)
	return out, err
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
// File write and SQL upsert are serialized per account/bucket/key (temp file + rename).
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

	unlock := s.lockS3Object(accountID, bucket, key)
	defer unlock()

	rel := filepath.Join("s3", accountID, bucket, filepath.FromSlash(key))
	abs := filepath.Join(s.dataRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return ObjectMeta{}, fmt.Errorf("put object mkdir: %w", err)
	}
	tmp := abs + ".tmp-" + uuid.NewString()
	if err := os.WriteFile(tmp, meta.Data, 0o600); err != nil {
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
		_ = os.Remove(tmp)
		return ObjectMeta{}, fmt.Errorf("put object meta: %w", err)
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		return ObjectMeta{}, fmt.Errorf("put object rename: %w", err)
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

// MultipartUpload is in-progress multipart upload metadata.
type MultipartUpload struct {
	AccountID    string
	Bucket       string
	Key          string
	UploadID     string
	ContentType  string
	SSEAlgorithm string
	KMSKeyID     string
	SealedDEK    []byte
	Initiated    string
}

// MultipartPart is a stored upload part.
type MultipartPart struct {
	PartNumber  int
	ETag        string
	Size        int64
	StoragePath string
}

// CreateMultipartUploadMeta carries optional SSE and content metadata for a new upload.
type CreateMultipartUploadMeta struct {
	ContentType  string
	SSEAlgorithm string
	KMSKeyID     string
	SealedDEK    []byte
}

// CompletedPartInput is a part reference supplied to CompleteMultipartUpload.
type CompletedPartInput struct {
	PartNumber int
	ETag       string
}

// CreateMultipartUpload starts a multipart upload and returns an upload ID.
func (s *Store) CreateMultipartUpload(accountID, bucket, key string, meta CreateMultipartUploadMeta) (MultipartUpload, error) {
	if err := ValidateBucketName(bucket); err != nil {
		return MultipartUpload{}, err
	}
	key, err := SanitizeObjectKey(key)
	if err != nil {
		return MultipartUpload{}, err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return MultipartUpload{}, err
	}
	ct := meta.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	uploadID := uuid.NewString()
	initiated := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO s3_multipart_uploads
		 (upload_id, account_id, bucket, key, content_type, sse_algorithm, kms_key_id, sealed_dek, initiated)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uploadID, accountID, bucket, key, ct, nullIfEmpty(meta.SSEAlgorithm), nullIfEmpty(meta.KMSKeyID), meta.SealedDEK, initiated,
	)
	if err != nil {
		return MultipartUpload{}, fmt.Errorf("create multipart upload: %w", err)
	}
	return MultipartUpload{
		AccountID:    accountID,
		Bucket:       bucket,
		Key:          key,
		UploadID:     uploadID,
		ContentType:  ct,
		SSEAlgorithm: meta.SSEAlgorithm,
		KMSKeyID:     meta.KMSKeyID,
		SealedDEK:    meta.SealedDEK,
		Initiated:    initiated,
	}, nil
}

// UploadPartMeta describes bytes for one multipart part.
type UploadPartMeta struct {
	Data      []byte
	ETag      string
	PlainSize int64
}

// UploadPart stores one part blob for a multipart upload.
func (s *Store) UploadPart(accountID, bucket, key, uploadID string, partNumber int, meta UploadPartMeta) (MultipartPart, error) {
	if partNumber < 1 || partNumber > 10000 {
		return MultipartPart{}, ErrInvalidPart
	}
	upload, err := s.getMultipartUpload(accountID, bucket, key, uploadID)
	if err != nil {
		return MultipartPart{}, err
	}
	data := meta.Data
	etag := normalizeETag(meta.ETag)
	if etag == "" {
		plainSum := md5.Sum(data)
		etag = hex.EncodeToString(plainSum[:])
	}
	size := meta.PlainSize
	if size == 0 {
		size = int64(len(data))
	}
	rel := filepath.Join("s3", accountID, bucket, ".multipart", uploadID, fmt.Sprintf("part-%05d", partNumber))
	abs := filepath.Join(s.dataRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return MultipartPart{}, fmt.Errorf("upload part mkdir: %w", err)
	}
	if err := os.WriteFile(abs, data, 0o600); err != nil {
		return MultipartPart{}, fmt.Errorf("upload part write: %w", err)
	}
	part := MultipartPart{
		PartNumber:  partNumber,
		ETag:        etag,
		Size:        size,
		StoragePath: rel,
	}
	_, err = s.db.Exec(
		`INSERT INTO s3_multipart_parts (upload_id, part_number, etag, size, storage_path)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(upload_id, part_number) DO UPDATE SET
		   etag = excluded.etag,
		   size = excluded.size,
		   storage_path = excluded.storage_path`,
		upload.UploadID, partNumber, etag, part.Size, rel,
	)
	if err != nil {
		_ = os.Remove(abs)
		return MultipartPart{}, fmt.Errorf("upload part meta: %w", err)
	}
	return part, nil
}

// MultipartCompleteResult is the assembled payload before final PutObject.
type MultipartCompleteResult struct {
	Upload MultipartUpload
	Data   []byte
	ETag   string
}

// CompleteMultipartUpload validates parts and returns the assembled plaintext object bytes.
func (s *Store) CompleteMultipartUpload(accountID, bucket, key, uploadID string, completed []CompletedPartInput) (MultipartCompleteResult, error) {
	upload, err := s.getMultipartUpload(accountID, bucket, key, uploadID)
	if err != nil {
		return MultipartCompleteResult{}, err
	}
	if len(completed) == 0 {
		return MultipartCompleteResult{}, ErrInvalidPart
	}
	storedParts, err := s.listAllParts(uploadID)
	if err != nil {
		return MultipartCompleteResult{}, err
	}
	partByNum := make(map[int]MultipartPart, len(storedParts))
	for _, p := range storedParts {
		partByNum[p.PartNumber] = p
	}
	sort.Slice(completed, func(i, j int) bool {
		return completed[i].PartNumber < completed[j].PartNumber
	})
	var assembled []byte
	var plainETags [][]byte
	for i, want := range completed {
		got, ok := partByNum[want.PartNumber]
		if !ok {
			return MultipartCompleteResult{}, ErrInvalidPart
		}
		if i < len(completed)-1 && got.Size < MinMultipartPartSize {
			return MultipartCompleteResult{}, ErrEntityTooSmall
		}
		if normalizeETag(want.ETag) != got.ETag {
			return MultipartCompleteResult{}, ErrInvalidPart
		}
		data, err := os.ReadFile(filepath.Join(s.dataRoot, got.StoragePath))
		if err != nil {
			return MultipartCompleteResult{}, fmt.Errorf("complete read part: %w", err)
		}
		assembled = append(assembled, data...)
		sum := md5.Sum(data)
		plainETags = append(plainETags, sum[:])
	}
	return MultipartCompleteResult{
		Upload: upload,
		Data:   assembled,
		ETag:   multipartCompositeETag(plainETags),
	}, nil
}

// CleanupMultipartUpload deletes upload metadata and part blobs after a successful finalize.
func (s *Store) CleanupMultipartUpload(uploadID string) error {
	return s.deleteMultipartUpload(uploadID)
}

// AbortMultipartUpload removes upload metadata and part blobs.
func (s *Store) AbortMultipartUpload(accountID, bucket, key, uploadID string) error {
	upload, err := s.getMultipartUpload(accountID, bucket, key, uploadID)
	if err != nil {
		return err
	}
	return s.deleteMultipartUpload(upload.UploadID)
}

// ListParts returns parts for an upload with optional pagination.
func (s *Store) ListParts(accountID, bucket, key, uploadID string, partNumberMarker, maxParts int) ([]MultipartPart, bool, error) {
	upload, err := s.getMultipartUpload(accountID, bucket, key, uploadID)
	if err != nil {
		return nil, false, err
	}
	if maxParts <= 0 {
		maxParts = 1000
	}
	query := `SELECT part_number, etag, size, storage_path FROM s3_multipart_parts
		WHERE upload_id = ? AND part_number > ? ORDER BY part_number LIMIT ?`
	rows, err := s.db.Query(query, upload.UploadID, partNumberMarker, maxParts+1)
	if err != nil {
		return nil, false, fmt.Errorf("list parts: %w", err)
	}
	defer rows.Close()
	var parts []MultipartPart
	for rows.Next() {
		var p MultipartPart
		if err := rows.Scan(&p.PartNumber, &p.ETag, &p.Size, &p.StoragePath); err != nil {
			return nil, false, err
		}
		parts = append(parts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(parts) > maxParts
	if truncated {
		parts = parts[:maxParts]
	}
	return parts, truncated, nil
}

// ListMultipartUploads returns in-progress uploads for a bucket.
func (s *Store) ListMultipartUploads(accountID, bucket, prefix string) ([]MultipartUpload, error) {
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT upload_id, account_id, bucket, key, content_type, sse_algorithm, kms_key_id, sealed_dek, initiated
		 FROM s3_multipart_uploads WHERE account_id = ? AND bucket = ? AND key LIKE ? ORDER BY key`,
		accountID, bucket, prefix+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("list multipart uploads: %w", err)
	}
	defer rows.Close()
	var out []MultipartUpload
	for rows.Next() {
		u, err := scanMultipartUpload(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetMultipartUpload returns in-progress upload metadata.
func (s *Store) GetMultipartUpload(accountID, bucket, key, uploadID string) (MultipartUpload, error) {
	return s.getMultipartUpload(accountID, bucket, key, uploadID)
}

func (s *Store) getMultipartUpload(accountID, bucket, key, uploadID string) (MultipartUpload, error) {
	key, err := SanitizeObjectKey(key)
	if err != nil {
		return MultipartUpload{}, err
	}
	row := s.db.QueryRow(
		`SELECT upload_id, account_id, bucket, key, content_type, sse_algorithm, kms_key_id, sealed_dek, initiated
		 FROM s3_multipart_uploads WHERE upload_id = ? AND account_id = ? AND bucket = ? AND key = ?`,
		uploadID, accountID, bucket, key,
	)
	u, err := scanMultipartUploadRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return MultipartUpload{}, ErrNoSuchUpload
	}
	if err != nil {
		return MultipartUpload{}, fmt.Errorf("get multipart upload: %w", err)
	}
	return u, nil
}

func (s *Store) listAllParts(uploadID string) ([]MultipartPart, error) {
	rows, err := s.db.Query(
		`SELECT part_number, etag, size, storage_path FROM s3_multipart_parts
		 WHERE upload_id = ? ORDER BY part_number`,
		uploadID,
	)
	if err != nil {
		return nil, fmt.Errorf("list all parts: %w", err)
	}
	defer rows.Close()
	var parts []MultipartPart
	for rows.Next() {
		var p MultipartPart
		if err := rows.Scan(&p.PartNumber, &p.ETag, &p.Size, &p.StoragePath); err != nil {
			return nil, err
		}
		parts = append(parts, p)
	}
	return parts, rows.Err()
}

func (s *Store) deleteMultipartUpload(uploadID string) error {
	parts, err := s.listAllParts(uploadID)
	if err != nil {
		return err
	}
	for _, p := range parts {
		_ = os.Remove(filepath.Join(s.dataRoot, p.StoragePath))
	}
	if len(parts) > 0 {
		_ = os.RemoveAll(filepath.Dir(filepath.Join(s.dataRoot, parts[0].StoragePath)))
	}
	_, err = s.db.Exec(`DELETE FROM s3_multipart_uploads WHERE upload_id = ?`, uploadID)
	if err != nil {
		return fmt.Errorf("delete multipart upload: %w", err)
	}
	return nil
}

func scanMultipartUpload(rows *sql.Rows) (MultipartUpload, error) {
	var u MultipartUpload
	var ct, sse, kmsID sql.NullString
	var sealed []byte
	if err := rows.Scan(&u.UploadID, &u.AccountID, &u.Bucket, &u.Key, &ct, &sse, &kmsID, &sealed, &u.Initiated); err != nil {
		return MultipartUpload{}, err
	}
	if ct.Valid {
		u.ContentType = ct.String
	}
	if sse.Valid {
		u.SSEAlgorithm = sse.String
	}
	if kmsID.Valid {
		u.KMSKeyID = kmsID.String
	}
	u.SealedDEK = sealed
	return u, nil
}

func scanMultipartUploadRow(row *sql.Row) (MultipartUpload, error) {
	var u MultipartUpload
	var ct, sse, kmsID sql.NullString
	var sealed []byte
	if err := row.Scan(&u.UploadID, &u.AccountID, &u.Bucket, &u.Key, &ct, &sse, &kmsID, &sealed, &u.Initiated); err != nil {
		return MultipartUpload{}, err
	}
	if ct.Valid {
		u.ContentType = ct.String
	}
	if sse.Valid {
		u.SSEAlgorithm = sse.String
	}
	if kmsID.Valid {
		u.KMSKeyID = kmsID.String
	}
	u.SealedDEK = sealed
	return u, nil
}

func multipartCompositeETag(partMD5s [][]byte) string {
	combined := bytes.Join(partMD5s, nil)
	sum := md5.Sum(combined)
	return hex.EncodeToString(sum[:]) + "-" + strconv.Itoa(len(partMD5s))
}

func normalizeETag(etag string) string {
	return strings.Trim(strings.TrimSpace(etag), `"`)
}
