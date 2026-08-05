package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3BucketCorsRoundTrip(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "cors-bucket"); err != nil {
		t.Fatal(err)
	}
	maxAge := 3000
	cfg := store.S3CORSConfiguration{Rules: []store.S3CORSRule{{
		AllowedOrigins: []string{"http://www.example.com"},
		AllowedMethods: []string{"GET", "PUT"},
		AllowedHeaders: []string{"*"},
		ExposeHeaders:  []string{"x-amz-server-side-encryption"},
		MaxAgeSeconds:  &maxAge,
	}}}
	if err := st.PutBucketCors(account, "cors-bucket", cfg); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBucketCors(account, "cors-bucket")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 1 || got.Rules[0].AllowedOrigins[0] != "http://www.example.com" {
		t.Fatalf("got=%+v", got)
	}
	if got.Rules[0].AllowedMethods[0] != "GET" || got.Rules[0].MaxAgeSeconds == nil || *got.Rules[0].MaxAgeSeconds != 3000 {
		t.Fatalf("rule=%+v", got.Rules[0])
	}
	if err := st.DeleteBucketCors(account, "cors-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBucketCors(account, "cors-bucket"); !errors.Is(err, store.ErrNoSuchCORSConfiguration) {
		t.Fatalf("want NoSuchCORSConfiguration, got %v", err)
	}
}

func TestS3BucketCorsRejectsEmpty(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "cors-empty"); err != nil {
		t.Fatal(err)
	}
	err := st.PutBucketCors(account, "cors-empty", store.S3CORSConfiguration{})
	if !errors.Is(err, store.ErrInvalidCORSConfiguration) {
		t.Fatalf("want InvalidRequest, got %v", err)
	}
}

func TestS3LifecycleConfigurationRoundTrip(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "lc-bucket"); err != nil {
		t.Fatal(err)
	}
	days := 30
	cfg := store.S3LifecycleConfiguration{Rules: []store.S3LifecycleRule{{
		ID:         "expire-logs",
		Status:     "Enabled",
		Filter:     &store.S3LifecycleFilter{Prefix: "logs/"},
		Expiration: &store.S3LifecycleExpiration{Days: &days},
	}}}
	if err := st.PutLifecycleConfiguration(account, "lc-bucket", cfg); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetLifecycleConfiguration(account, "lc-bucket")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 1 || got.Rules[0].ID != "expire-logs" || got.Rules[0].Filter.Prefix != "logs/" {
		t.Fatalf("got=%+v", got)
	}
	if got.Rules[0].Expiration == nil || got.Rules[0].Expiration.Days == nil || *got.Rules[0].Expiration.Days != 30 {
		t.Fatalf("expiration=%+v", got.Rules[0].Expiration)
	}
	if err := st.DeleteLifecycleConfiguration(account, "lc-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetLifecycleConfiguration(account, "lc-bucket"); !errors.Is(err, store.ErrNoSuchLifecycleConfiguration) {
		t.Fatalf("want NoSuchLifecycleConfiguration, got %v", err)
	}
}

func TestS3LifecycleRejectsNoAction(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "lc-bad"); err != nil {
		t.Fatal(err)
	}
	err := st.PutLifecycleConfiguration(account, "lc-bad", store.S3LifecycleConfiguration{
		Rules: []store.S3LifecycleRule{{ID: "noop", Status: "Enabled"}},
	})
	if !errors.Is(err, store.ErrInvalidLifecycleConfiguration) {
		t.Fatalf("want InvalidRequest, got %v", err)
	}
}

func TestRunS3SelectCSVLimit(t *testing.T) {
	data := []byte("a,b\n1,2\n3,4\n5,6\n")
	result, err := store.RunS3Select(data, store.S3SelectRequest{
		Expression:     "SELECT * FROM s3object LIMIT 2",
		ExpressionType: "SQL",
		Input: store.S3SelectInput{
			Format:         "CSV",
			FileHeaderInfo: "USE",
		},
		Output: store.S3SelectOutput{Format: "JSON"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("records=%d want 2 (%v)", len(result.Records), result.Records)
	}
	var row map[string]string
	if err := json.Unmarshal(result.Records[0], &row); err != nil {
		t.Fatal(err)
	}
	if row["a"] != "1" || row["b"] != "2" {
		t.Fatalf("row=%v", row)
	}
}

func TestRunS3SelectJSONLines(t *testing.T) {
	data := []byte("{\"n\":1}\n{\"n\":2}\n")
	result, err := store.RunS3Select(data, store.S3SelectRequest{
		Expression:     "select * from S3Object",
		ExpressionType: "SQL",
		Input:          store.S3SelectInput{Format: "JSON", JSONType: "LINES"},
		Output:         store.S3SelectOutput{Format: "JSON"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("records=%d", len(result.Records))
	}
}

func TestRunS3SelectUnsupportedSQLFailClosed(t *testing.T) {
	_, err := store.RunS3Select([]byte("a,b\n1,2\n"), store.S3SelectRequest{
		Expression:     "SELECT a FROM s3object WHERE a > 1",
		ExpressionType: "SQL",
		Input:          store.S3SelectInput{Format: "CSV"},
		Output:         store.S3SelectOutput{Format: "JSON"},
	})
	if !errors.Is(err, store.ErrSelectUnsupportedSQL) {
		t.Fatalf("want UnsupportedSelectSQL, got %v", err)
	}
}

func TestRunS3SelectObjectTooLarge(t *testing.T) {
	data := []byte(strings.Repeat("x", store.MaxSelectObjectBytes+1))
	_, err := store.RunS3Select(data, store.S3SelectRequest{
		Expression: "SELECT * FROM s3object",
		Input:      store.S3SelectInput{Format: "CSV"},
		Output:     store.S3SelectOutput{Format: "CSV"},
	})
	if !errors.Is(err, store.ErrSelectObjectTooLarge) {
		t.Fatalf("want SelectObjectTooLarge, got %v", err)
	}
}
