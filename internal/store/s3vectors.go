package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrS3VectorsNotFound   = errors.New("NotFoundException")
	ErrS3VectorsBadRequest = errors.New("ValidationException")
	ErrS3VectorsExists     = errors.New("ConflictException")
)

const DefaultS3VectorsRegion = "us-east-1"

const s3vectorsSchema = `
CREATE TABLE IF NOT EXISTS s3vectors_buckets (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS s3vectors_indexes (
  account_id TEXT NOT NULL,
  bucket_name TEXT NOT NULL,
  index_name TEXT NOT NULL,
  arn TEXT NOT NULL,
  dimension INTEGER NOT NULL,
  distance_metric TEXT NOT NULL,
  data_type TEXT NOT NULL DEFAULT 'float32',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, bucket_name, index_name)
);
CREATE TABLE IF NOT EXISTS s3vectors_vectors (
  account_id TEXT NOT NULL,
  bucket_name TEXT NOT NULL,
  index_name TEXT NOT NULL,
  vector_key TEXT NOT NULL,
  data_json TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY (account_id, bucket_name, index_name, vector_key)
);
`

// S3VectorBucket is a vector bucket row.
type S3VectorBucket struct {
	Name      string
	ARN       string
	CreatedAt int64
}

// S3VectorIndex is a vector index row.
type S3VectorIndex struct {
	BucketName     string
	IndexName      string
	ARN            string
	Dimension      int
	DistanceMetric string
	DataType       string
	CreatedAt      int64
}

// S3Vector is a stored vector.
type S3Vector struct {
	Key          string
	Data         []float64
	MetadataJSON string
}

// EnsureS3VectorsSchema creates S3 Vectors tables if missing.
func EnsureS3VectorsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure s3vectors schema: db is nil")
	}
	if _, err := db.Exec(s3vectorsSchema); err != nil {
		return fmt.Errorf("ensure s3vectors schema: %w", err)
	}
	return nil
}

// EnsureS3VectorsSchema ensures S3 Vectors tables on an open store.
func (s *Store) EnsureS3VectorsSchema() error {
	return EnsureS3VectorsSchema(s.db)
}

func s3VectorBucketARN(accountID, name string) string {
	return fmt.Sprintf("arn:aws:s3vectors:%s:%s:bucket/%s", DefaultS3VectorsRegion, accountID, name)
}

func s3VectorIndexARN(accountID, bucket, index string) string {
	return fmt.Sprintf("arn:aws:s3vectors:%s:%s:bucket/%s/index/%s", DefaultS3VectorsRegion, accountID, bucket, index)
}

// CreateS3VectorBucket creates a vector bucket.
func (s *Store) CreateS3VectorBucket(accountID, name string) (S3VectorBucket, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return S3VectorBucket{}, fmt.Errorf("%w: vectorBucketName required", ErrS3VectorsBadRequest)
	}
	arn := s3VectorBucketARN(accountID, name)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO s3vectors_buckets (account_id, name, arn, created_at) VALUES (?, ?, ?, ?)`,
		accountID, name, arn, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return S3VectorBucket{}, ErrS3VectorsExists
		}
		return S3VectorBucket{}, fmt.Errorf("create vector bucket: %w", err)
	}
	return S3VectorBucket{Name: name, ARN: arn, CreatedAt: now}, nil
}

// GetS3VectorBucket returns a vector bucket.
func (s *Store) GetS3VectorBucket(accountID, name string) (S3VectorBucket, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	var b S3VectorBucket
	err := s.db.QueryRow(
		`SELECT name, arn, created_at FROM s3vectors_buckets WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&b.Name, &b.ARN, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return S3VectorBucket{}, ErrS3VectorsNotFound
	}
	if err != nil {
		return S3VectorBucket{}, fmt.Errorf("get vector bucket: %w", err)
	}
	return b, nil
}

// ListS3VectorBuckets lists vector buckets.
func (s *Store) ListS3VectorBuckets(accountID string) ([]S3VectorBucket, error) {
	rows, err := s.db.Query(
		`SELECT name, arn, created_at FROM s3vectors_buckets WHERE account_id = ? ORDER BY name`, accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list vector buckets: %w", err)
	}
	defer rows.Close()
	var out []S3VectorBucket
	for rows.Next() {
		var b S3VectorBucket
		if err := rows.Scan(&b.Name, &b.ARN, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("list vector buckets scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteS3VectorBucket deletes a vector bucket (indexes and vectors cascade lite).
func (s *Store) DeleteS3VectorBucket(accountID, name string) error {
	name = strings.TrimSpace(strings.ToLower(name))
	res, err := s.db.Exec(`DELETE FROM s3vectors_buckets WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete vector bucket: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrS3VectorsNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM s3vectors_indexes WHERE account_id = ? AND bucket_name = ?`, accountID, name)
	_, _ = s.db.Exec(`DELETE FROM s3vectors_vectors WHERE account_id = ? AND bucket_name = ?`, accountID, name)
	return nil
}

// CreateS3VectorIndex creates an index in a vector bucket.
func (s *Store) CreateS3VectorIndex(accountID, bucketName, indexName, distanceMetric string, dimension int) (S3VectorIndex, error) {
	bucketName = strings.TrimSpace(strings.ToLower(bucketName))
	indexName = strings.TrimSpace(indexName)
	distanceMetric = strings.ToLower(strings.TrimSpace(distanceMetric))
	if bucketName == "" || indexName == "" {
		return S3VectorIndex{}, fmt.Errorf("%w: vectorBucketName and indexName required", ErrS3VectorsBadRequest)
	}
	if dimension <= 0 || dimension > 4096 {
		return S3VectorIndex{}, fmt.Errorf("%w: dimension must be 1..4096", ErrS3VectorsBadRequest)
	}
	switch distanceMetric {
	case "cosine", "euclidean":
	default:
		return S3VectorIndex{}, fmt.Errorf("%w: distanceMetric must be cosine or euclidean", ErrS3VectorsBadRequest)
	}
	if _, err := s.GetS3VectorBucket(accountID, bucketName); err != nil {
		return S3VectorIndex{}, err
	}
	arn := s3VectorIndexARN(accountID, bucketName, indexName)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO s3vectors_indexes
		 (account_id, bucket_name, index_name, arn, dimension, distance_metric, data_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, bucketName, indexName, arn, dimension, distanceMetric, "float32", now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return S3VectorIndex{}, ErrS3VectorsExists
		}
		return S3VectorIndex{}, fmt.Errorf("create index: %w", err)
	}
	return S3VectorIndex{
		BucketName: bucketName, IndexName: indexName, ARN: arn,
		Dimension: dimension, DistanceMetric: distanceMetric, DataType: "float32", CreatedAt: now,
	}, nil
}

// GetS3VectorIndex returns an index.
func (s *Store) GetS3VectorIndex(accountID, bucketName, indexName string) (S3VectorIndex, error) {
	bucketName = strings.TrimSpace(strings.ToLower(bucketName))
	indexName = strings.TrimSpace(indexName)
	var idx S3VectorIndex
	err := s.db.QueryRow(
		`SELECT bucket_name, index_name, arn, dimension, distance_metric, data_type, created_at
		 FROM s3vectors_indexes WHERE account_id = ? AND bucket_name = ? AND index_name = ?`,
		accountID, bucketName, indexName,
	).Scan(&idx.BucketName, &idx.IndexName, &idx.ARN, &idx.Dimension, &idx.DistanceMetric, &idx.DataType, &idx.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return S3VectorIndex{}, ErrS3VectorsNotFound
	}
	if err != nil {
		return S3VectorIndex{}, fmt.Errorf("get index: %w", err)
	}
	return idx, nil
}

// ListS3VectorIndexes lists indexes in a bucket.
func (s *Store) ListS3VectorIndexes(accountID, bucketName string) ([]S3VectorIndex, error) {
	bucketName = strings.TrimSpace(strings.ToLower(bucketName))
	rows, err := s.db.Query(
		`SELECT bucket_name, index_name, arn, dimension, distance_metric, data_type, created_at
		 FROM s3vectors_indexes WHERE account_id = ? AND bucket_name = ? ORDER BY index_name`,
		accountID, bucketName,
	)
	if err != nil {
		return nil, fmt.Errorf("list indexes: %w", err)
	}
	defer rows.Close()
	var out []S3VectorIndex
	for rows.Next() {
		var idx S3VectorIndex
		if err := rows.Scan(&idx.BucketName, &idx.IndexName, &idx.ARN, &idx.Dimension, &idx.DistanceMetric, &idx.DataType, &idx.CreatedAt); err != nil {
			return nil, fmt.Errorf("list indexes scan: %w", err)
		}
		out = append(out, idx)
	}
	return out, rows.Err()
}

// DeleteS3VectorIndex deletes an index and its vectors.
func (s *Store) DeleteS3VectorIndex(accountID, bucketName, indexName string) error {
	bucketName = strings.TrimSpace(strings.ToLower(bucketName))
	indexName = strings.TrimSpace(indexName)
	res, err := s.db.Exec(
		`DELETE FROM s3vectors_indexes WHERE account_id = ? AND bucket_name = ? AND index_name = ?`,
		accountID, bucketName, indexName,
	)
	if err != nil {
		return fmt.Errorf("delete index: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrS3VectorsNotFound
	}
	_, _ = s.db.Exec(
		`DELETE FROM s3vectors_vectors WHERE account_id = ? AND bucket_name = ? AND index_name = ?`,
		accountID, bucketName, indexName,
	)
	return nil
}

// PutS3Vectors upserts vectors into an index.
func (s *Store) PutS3Vectors(accountID, bucketName, indexName string, vectors []S3Vector) error {
	idx, err := s.GetS3VectorIndex(accountID, bucketName, indexName)
	if err != nil {
		return err
	}
	if len(vectors) == 0 {
		return fmt.Errorf("%w: vectors required", ErrS3VectorsBadRequest)
	}
	for _, v := range vectors {
		key := strings.TrimSpace(v.Key)
		if key == "" {
			return fmt.Errorf("%w: vector key required", ErrS3VectorsBadRequest)
		}
		if len(v.Data) != idx.Dimension {
			return fmt.Errorf("%w: vector %q dimension %d does not match index %d", ErrS3VectorsBadRequest, key, len(v.Data), idx.Dimension)
		}
		dataJSON, err := json.Marshal(v.Data)
		if err != nil {
			return fmt.Errorf("marshal vector: %w", err)
		}
		meta := v.MetadataJSON
		if meta == "" {
			meta = "{}"
		}
		_, err = s.db.Exec(
			`INSERT INTO s3vectors_vectors (account_id, bucket_name, index_name, vector_key, data_json, metadata_json)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, bucket_name, index_name, vector_key)
			 DO UPDATE SET data_json = excluded.data_json, metadata_json = excluded.metadata_json`,
			accountID, idx.BucketName, idx.IndexName, key, string(dataJSON), meta,
		)
		if err != nil {
			return fmt.Errorf("put vectors: %w", err)
		}
	}
	return nil
}

// S3VectorQueryMatch is a QueryVectors hit.
type S3VectorQueryMatch struct {
	Key      string
	Distance float64
	Data     []float64
}

// QueryS3Vectors runs an in-process cosine or euclidean distance stub.
func (s *Store) QueryS3Vectors(accountID, bucketName, indexName string, query []float64, topK int) ([]S3VectorQueryMatch, error) {
	idx, err := s.GetS3VectorIndex(accountID, bucketName, indexName)
	if err != nil {
		return nil, err
	}
	if len(query) != idx.Dimension {
		return nil, fmt.Errorf("%w: query vector dimension %d does not match index %d", ErrS3VectorsBadRequest, len(query), idx.Dimension)
	}
	if topK <= 0 {
		topK = 10
	}
	rows, err := s.db.Query(
		`SELECT vector_key, data_json FROM s3vectors_vectors
		 WHERE account_id = ? AND bucket_name = ? AND index_name = ?`,
		accountID, idx.BucketName, idx.IndexName,
	)
	if err != nil {
		return nil, fmt.Errorf("query vectors: %w", err)
	}
	defer rows.Close()
	var matches []S3VectorQueryMatch
	for rows.Next() {
		var key, dataJSON string
		if err := rows.Scan(&key, &dataJSON); err != nil {
			return nil, fmt.Errorf("query vectors scan: %w", err)
		}
		var data []float64
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			continue
		}
		dist := vectorDistance(idx.DistanceMetric, query, data)
		matches = append(matches, S3VectorQueryMatch{Key: key, Distance: dist, Data: data})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// partial selection sort for topK (small lab indexes)
	for i := 0; i < len(matches) && i < topK; i++ {
		best := i
		for j := i + 1; j < len(matches); j++ {
			if matches[j].Distance < matches[best].Distance {
				best = j
			}
		}
		matches[i], matches[best] = matches[best], matches[i]
	}
	if len(matches) > topK {
		matches = matches[:topK]
	}
	return matches, nil
}

func vectorDistance(metric string, a, b []float64) float64 {
	switch metric {
	case "euclidean":
		var sum float64
		for i := range a {
			d := a[i] - b[i]
			sum += d * d
		}
		return math.Sqrt(sum)
	default: // cosine distance = 1 - cosine similarity
		var dot, na, nb float64
		for i := range a {
			dot += a[i] * b[i]
			na += a[i] * a[i]
			nb += b[i] * b[i]
		}
		if na == 0 || nb == 0 {
			return 1
		}
		sim := dot / (math.Sqrt(na) * math.Sqrt(nb))
		return 1 - sim
	}
}

// NewS3VectorKey generates a lab key when callers omit one (tests).
func NewS3VectorKey() string {
	return uuid.NewString()
}
