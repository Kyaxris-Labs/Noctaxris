package store_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCloudTrailDigestObjectKey(t *testing.T) {
	logKey := "pfx/AWSLogs/000000000001/CloudTrail/us-east-1/2026/07/25/000000000001_CloudTrail_us-east-1_20260725T1345Z_abc.json"
	want := "pfx/AWSLogs/000000000001/CloudTrail-Digest/us-east-1/2026/07/25/000000000001_CloudTrail-Digest_us-east-1_20260725T1345Z_abc.json"
	if got := store.CloudTrailDigestObjectKey(logKey); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	gzLog := strings.TrimSuffix(logKey, ".json") + ".json.gz"
	gzWant := want
	if got := store.CloudTrailDigestObjectKey(gzLog); got != gzWant {
		t.Fatalf("gzip digest key got %q want %q", got, gzWant)
	}
}

func TestCloudTrailDeliveryWritesDigestSidecar(t *testing.T) {
	dir := t.TempDir()
	st := openStore(t, dir)
	const account = "000000000001"
	const bucket = "ct-digest-bucket"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	writeCloudTrailFixture(t, dir,
		`{"eventTime":"2026-07-20T10:00:00Z","eventName":"GetCallerIdentity","eventID":"ev-digest","eventSource":"sts.amazonaws.com","userIdentity":{"userName":"root"}}`,
	)
	if _, err := st.CreateCloudTrailTrail(account, store.CloudTrailTrail{
		Name: "digest-trail", S3BucketName: bucket, HomeRegion: store.DefaultCloudTrailRegion,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.StartCloudTrailLogging(account, "digest-trail", dir); err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListObjectsV2(account, bucket, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var logKey string
	for _, o := range listed.Contents {
		if strings.Contains(o.Key, "/CloudTrail/") && !strings.Contains(o.Key, "CloudTrail-Digest") {
			logKey = o.Key
			break
		}
	}
	if logKey == "" {
		t.Fatalf("missing log object keys=%v", objectKeys(listed))
	}
	digestKey := store.CloudTrailDigestObjectKey(logKey)
	if err := st.ValidateCloudTrailLogFile(account, bucket, logKey); err != nil {
		t.Fatalf("ValidateCloudTrailLogFile: %v", err)
	}
	_, digestBody, err := st.GetObject(account, bucket, digestKey)
	if err != nil {
		t.Fatal(err)
	}
	var doc store.CloudTrailDigestDocument
	if err := json.Unmarshal(digestBody, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.HashAlgorithm != "SHA256" || doc.LogS3Key != logKey {
		t.Fatalf("digest doc=%+v", doc)
	}
}

func TestCloudTrailEventSelectorFiltersDelivery(t *testing.T) {
	dir := t.TempDir()
	st := openStore(t, dir)
	const account = "000000000001"
	const bucket = "ct-sel-bucket"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	writeCloudTrailFixture(t, dir,
		`{"eventTime":"2026-07-20T10:00:00Z","eventName":"GetCallerIdentity","eventID":"ev-mgmt","eventSource":"sts.amazonaws.com","managementEvent":true,"userIdentity":{"userName":"root"}}`,
		`{"eventTime":"2026-07-20T10:01:00Z","eventName":"GetObject","eventID":"ev-data","eventSource":"s3.amazonaws.com","eventCategory":"Data","managementEvent":false,"userIdentity":{"userName":"root"}}`,
	)
	if _, err := st.CreateCloudTrailTrail(account, store.CloudTrailTrail{
		Name: "sel-trail", S3BucketName: bucket, HomeRegion: store.DefaultCloudTrailRegion,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutCloudTrailEventSelectors(account, "sel-trail", store.CloudTrailEventSelectors{
		IncludeManagementEvents: true,
		S3DataEventsEnabled:     false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.StartCloudTrailLogging(account, "sel-trail", dir); err != nil {
		t.Fatal(err)
	}
	_, body, err := st.GetObject(account, bucket, firstCloudTrailLogKey(t, st, account, bucket))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ev-mgmt") {
		t.Fatalf("expected management event in body %s", body)
	}
	if strings.Contains(string(body), "ev-data") {
		t.Fatalf("S3 data event should be filtered out body %s", body)
	}
}

func firstCloudTrailLogKey(t *testing.T, st *store.Store, account, bucket string) string {
	t.Helper()
	listed, err := st.ListObjectsV2(account, bucket, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range listed.Contents {
		if strings.Contains(o.Key, "/CloudTrail/") && !strings.Contains(o.Key, "CloudTrail-Digest") {
			return o.Key
		}
	}
	t.Fatalf("no log key in %v", objectKeys(listed))
	return ""
}

func openStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
