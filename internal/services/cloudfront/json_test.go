package cloudfront_test

import (
	"encoding/json"
	"testing"

	cfsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudfront"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGetDistributionConfigJSONIncludesETag(t *testing.T) {
	d := store.CloudFrontDistribution{
		ID: "EABC", ARN: "arn:aws:cloudfront::1:distribution/EABC",
		CallerReference: "ref", Comment: "c", Enabled: true,
		OriginsJSON:   `[{"Id":"o1","DomainName":"b","OriginType":"s3"}]`,
		BehaviorsJSON: `[{"PathPattern":"*","TargetOriginId":"o1"}]`,
		Status:        store.CloudFrontStatusDeployed, ETag: "etag-1",
		DomainName: "deabc.cloudfront.noctaxris.local",
	}
	raw, err := cfsvc.GetDistributionConfigJSON(d)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["ETag"] != "etag-1" {
		t.Fatalf("ETag=%v", out["ETag"])
	}
	cfg, _ := out["DistributionConfig"].(map[string]any)
	if cfg == nil || cfg["Enabled"] != true {
		t.Fatalf("DistributionConfig=%v", out["DistributionConfig"])
	}
}

func TestInvalidationJSONCompleted(t *testing.T) {
	inv := store.CloudFrontInvalidation{
		ID: "IABC", DistributionID: "EABC", CallerReference: "ref",
		PathsJSON: `["/a","/b/*"]`, Status: store.CloudFrontInvalidationStatusCompleted,
		CreatedAt: 1_700_000_000_000,
	}
	raw, err := cfsvc.CreateInvalidationJSON(inv)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	body, _ := out["Invalidation"].(map[string]any)
	if body["Status"] != "Completed" || body["Id"] != "IABC" {
		t.Fatalf("Invalidation=%v", body)
	}
}
