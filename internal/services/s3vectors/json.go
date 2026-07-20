package s3vectors

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateVectorBucketJSON builds CreateVectorBucket response.
func CreateVectorBucketJSON(b store.S3VectorBucket) ([]byte, error) {
	return json.Marshal(map[string]any{
		"vectorBucket": map[string]any{
			"vectorBucketName": b.Name,
			"vectorBucketArn":  b.ARN,
		},
	})
}

// ListVectorBucketsJSON builds ListVectorBuckets response.
func ListVectorBucketsJSON(buckets []store.S3VectorBucket) ([]byte, error) {
	items := make([]map[string]any, 0, len(buckets))
	for _, b := range buckets {
		items = append(items, map[string]any{
			"vectorBucketName": b.Name,
			"vectorBucketArn":  b.ARN,
		})
	}
	return json.Marshal(map[string]any{"vectorBuckets": items})
}

// CreateIndexJSON builds CreateIndex response.
func CreateIndexJSON(idx store.S3VectorIndex) ([]byte, error) {
	return json.Marshal(map[string]any{
		"index": map[string]any{
			"vectorBucketName": idx.BucketName,
			"indexName":        idx.IndexName,
			"indexArn":         idx.ARN,
			"dimension":        idx.Dimension,
			"distanceMetric":   idx.DistanceMetric,
			"dataType":         idx.DataType,
		},
	})
}

// ListIndexesJSON builds ListIndexes response.
func ListIndexesJSON(indexes []store.S3VectorIndex) ([]byte, error) {
	items := make([]map[string]any, 0, len(indexes))
	for _, idx := range indexes {
		items = append(items, map[string]any{
			"vectorBucketName": idx.BucketName,
			"indexName":        idx.IndexName,
			"indexArn":         idx.ARN,
			"dimension":        idx.Dimension,
			"distanceMetric":   idx.DistanceMetric,
		})
	}
	return json.Marshal(map[string]any{"indexes": items})
}

// PutVectorsJSON is an empty OK body.
func PutVectorsJSON() ([]byte, error) { return []byte(`{}`), nil }

// QueryVectorsJSON builds QueryVectors response.
func QueryVectorsJSON(matches []store.S3VectorQueryMatch) ([]byte, error) {
	items := make([]map[string]any, 0, len(matches))
	for _, m := range matches {
		items = append(items, map[string]any{
			"key":      m.Key,
			"distance": m.Distance,
			"data":     map[string]any{"float32": m.Data},
		})
	}
	return json.Marshal(map[string]any{"vectors": items})
}

// DeleteOKJSON is an empty OK body.
func DeleteOKJSON() ([]byte, error) { return []byte(`{}`), nil }
