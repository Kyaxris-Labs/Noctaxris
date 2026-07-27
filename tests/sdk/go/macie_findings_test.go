package sdk_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestMacieEnableInjectListGetFindings(t *testing.T) {
	requireReady(t)
	if os.Getenv("NOCTAXRIS_MACIE_INJECT") != "1" {
		t.Skip("set NOCTAXRIS_MACIE_INJECT=1 on the API process for lab InjectFindings")
	}
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-macie")
	key := "pii.csv"
	body := []byte("ssn 987-65-4321\n")

	enableStatus, enableBody, _ := signedJSONTarget(t, "macie2", "Macie2.EnableMacie", map[string]any{})
	if enableStatus != 200 {
		t.Fatalf("EnableMacie status=%d body=%s", enableStatus, enableBody)
	}

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})
	if _, err := s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body),
	}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	injStatus, injBody, injParsed := signedJSONTarget(t, "macie2", "NoctaxrisMacie.InjectFindings", map[string]any{
		"S3Objects": []map[string]any{
			{"Bucket": bucket, "Key": key},
		},
	})
	if injStatus != 200 {
		t.Fatalf("InjectFindings status=%d body=%s", injStatus, injBody)
	}
	rawIDs, _ := injParsed["findingIds"].([]any)
	if len(rawIDs) < 1 {
		t.Fatalf("InjectFindings findingIds empty: %s", injBody)
	}

	listStatus, listBody, listParsed := signedJSONTarget(t, "macie2", "Macie2.ListFindings", map[string]any{})
	if listStatus != 200 {
		t.Fatalf("ListFindings status=%d body=%s", listStatus, listBody)
	}
	listedIDs, _ := listParsed["findingIds"].([]any)
	if !jsonAnyContains(listedIDs, rawIDs[0]) {
		t.Fatalf("ListFindings missing injected id %v: %s", rawIDs[0], listBody)
	}

	getStatus, getBody, getParsed := signedJSONTarget(t, "macie2", "Macie2.GetFindings", map[string]any{
		"findingIds": rawIDs,
	})
	if getStatus != 200 {
		t.Fatalf("GetFindings status=%d body=%s", getStatus, getBody)
	}
	if !strings.Contains(string(getBody), "SensitiveData") {
		t.Fatalf("GetFindings missing SensitiveData: %s", getBody)
	}
	findings, _ := getParsed["findings"].([]any)
	if len(findings) < 1 {
		t.Fatalf("GetFindings findings empty: %s", getBody)
	}
}

func jsonAnyContains(haystack []any, needle any) bool {
	want := stringifyAny(needle)
	for _, item := range haystack {
		if stringifyAny(item) == want {
			return true
		}
	}
	return false
}

func stringifyAny(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}
