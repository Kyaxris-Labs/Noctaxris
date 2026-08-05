package s3vectors_test

import (
	"encoding/json"
	"testing"

	s3vectorssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/s3vectors"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3VectorsJSON(t *testing.T) {
	b := store.S3VectorBucket{Name: "vb", ARN: "arn:aws:s3vectors:us-east-1:1:bucket/vb"}
	raw, err := s3vectorssvc.CreateVectorBucketJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	raw, _ = s3vectorssvc.ListVectorBucketsJSON([]store.S3VectorBucket{b})
	_ = json.Unmarshal(raw, &out)
	if len(out["vectorBuckets"].([]any)) != 1 {
		t.Fatalf("buckets=%v", out)
	}
	_, _ = s3vectorssvc.ListVectorBucketsJSON(nil)

	idx := store.S3VectorIndex{
		BucketName: "vb", IndexName: "idx", ARN: "arn:idx",
		Dimension: 128, DistanceMetric: "cosine", DataType: "float32",
	}
	raw, _ = s3vectorssvc.CreateIndexJSON(idx)
	_ = json.Unmarshal(raw, &out)

	raw, _ = s3vectorssvc.ListIndexesJSON([]store.S3VectorIndex{idx})
	_ = json.Unmarshal(raw, &out)

	if raw, err := s3vectorssvc.PutVectorsJSON(); err != nil || string(raw) != "{}" {
		t.Fatal(err)
	}

	match := store.S3VectorQueryMatch{Key: "k1", Distance: 0.1, Data: []float64{1, 0}}
	raw, _ = s3vectorssvc.QueryVectorsJSON([]store.S3VectorQueryMatch{match})
	_ = json.Unmarshal(raw, &out)
	if len(out["vectors"].([]any)) != 1 {
		t.Fatalf("query=%v", out)
	}

	if _, err := s3vectorssvc.DeleteOKJSON(); err != nil {
		t.Fatal(err)
	}
}
