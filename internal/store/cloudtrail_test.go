package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func writeCloudTrailFixture(t *testing.T, dataRoot string, lines ...string) {
	t.Helper()
	dir := filepath.Join(dataRoot, "cloudtrail")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var body string
	for _, line := range lines {
		body += line + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendCloudTrailFixture(t *testing.T, dataRoot string, lines ...string) {
	t.Helper()
	path := filepath.Join(dataRoot, "cloudtrail", "events.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLookupCloudTrailEventsFilters(t *testing.T) {
	dir := t.TempDir()
	writeCloudTrailFixture(t, dir,
		`{"eventVersion":"1.08","eventTime":"2026-07-20T10:00:00Z","eventSource":"sts.amazonaws.com","eventName":"GetCallerIdentity","eventID":"ev-1","eventType":"AwsApiCall","recipientAccountId":"000000000001","readOnly":true,"userIdentity":{"type":"IAMUser","userName":"alice","arn":"arn:aws:iam::000000000001:user/alice","accessKeyId":"AKIAALICE"}}`,
		`{"eventVersion":"1.08","eventTime":"2026-07-20T11:00:00Z","eventSource":"s3.amazonaws.com","eventName":"CreateBucket","eventID":"ev-2","eventType":"AwsApiCall","recipientAccountId":"000000000001","readOnly":false,"userIdentity":{"type":"IAMUser","userName":"bob","arn":"arn:aws:iam::000000000001:user/bob","accessKeyId":"AKIABOB"}}`,
		`{"eventVersion":"1.08","eventTime":"2026-07-20T12:00:00Z","eventSource":"sts.amazonaws.com","eventName":"GetCallerIdentity","eventID":"ev-3","eventType":"AwsApiCall","recipientAccountId":"000000000001","readOnly":true,"userIdentity":{"type":"IAMUser","userName":"alice","arn":"arn:aws:iam::000000000001:user/alice","accessKeyId":"AKIAALICE"}}`,
	)

	start := time.Date(2026, 7, 20, 10, 30, 0, 0, time.UTC)
	got, err := store.LookupCloudTrailEvents(dir, store.CloudTrailLookupFilter{
		StartTime:      &start,
		AttributeKey:   "EventName",
		AttributeValue: "GetCallerIdentity",
		MaxResults:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d want 1 got=%+v", len(got), got)
	}
	if got[0].EventId != "ev-3" || got[0].Username != "alice" {
		t.Fatalf("got=%+v", got[0])
	}

	byUser, err := store.LookupCloudTrailEvents(dir, store.CloudTrailLookupFilter{
		AttributeKey:   "Username",
		AttributeValue: "bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(byUser) != 1 || byUser[0].EventName != "CreateBucket" {
		t.Fatalf("byUser=%+v", byUser)
	}
}

func TestLookupCloudTrailEventsMissingFile(t *testing.T) {
	got, err := store.LookupCloudTrailEvents(t.TempDir(), store.CloudTrailLookupFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestLookupCloudTrailEventsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeCloudTrailFixture(t, dir,
		`{"eventTime":"2026-07-20T10:00:00Z","eventName":"A","eventID":"1","eventSource":"x","userIdentity":{"userName":"u"}}`,
		`{"eventTime":"2026-07-20T12:00:00Z","eventName":"B","eventID":"2","eventSource":"x","userIdentity":{"userName":"u"}}`,
		`{"eventTime":"2026-07-20T11:00:00Z","eventName":"C","eventID":"3","eventSource":"x","userIdentity":{"userName":"u"}}`,
	)
	got, err := store.LookupCloudTrailEvents(dir, store.CloudTrailLookupFilter{MaxResults: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].EventId != "2" || got[1].EventId != "3" {
		t.Fatalf("got=%+v", got)
	}
}

func TestCloudTrailContinuousDeliveryAfterStartLogging(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const account = "000000000001"
	const bucket = "ct-lab-bucket"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}

	writeCloudTrailFixture(t, dir,
		`{"eventTime":"2026-07-20T10:00:00Z","eventName":"GetCallerIdentity","eventID":"ev-start","eventSource":"sts.amazonaws.com","userIdentity":{"userName":"root"}}`,
	)

	trail, err := st.CreateCloudTrailTrail(account, store.CloudTrailTrail{
		Name:         "lab-trail",
		S3BucketName: bucket,
		S3KeyPrefix:  "prefix",
		HomeRegion:   store.DefaultCloudTrailRegion,
	})
	if err != nil {
		t.Fatalf("CreateCloudTrailTrail: %v", err)
	}
	if trail.IsLogging {
		t.Fatal("want IsLogging=false after create")
	}

	if err := st.StartCloudTrailLogging(account, trail.Name, dir); err != nil {
		t.Fatalf("StartCloudTrailLogging: %v", err)
	}
	got, err := st.GetCloudTrailTrail(account, trail.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsLogging {
		t.Fatal("want IsLogging=true after StartLogging")
	}

	listed, err := st.ListObjectsV2(account, bucket, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Contents) != 1 {
		t.Fatalf("after StartLogging objects=%d want 1", len(listed.Contents))
	}
	startKey := listed.Contents[0].Key

	appendCloudTrailFixture(t, dir,
		`{"eventTime":"2026-07-20T11:00:00Z","eventName":"CreateBucket","eventID":"ev-continuous","eventSource":"s3.amazonaws.com","userIdentity":{"userName":"root"}}`,
	)
	if err := st.ShipCloudTrailContinuousDeliveries(dir); err != nil {
		t.Fatalf("ShipCloudTrailContinuousDeliveries: %v", err)
	}

	listed, err = st.ListObjectsV2(account, bucket, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Contents) != 2 {
		t.Fatalf("after continuous ship objects=%d want 2 keys=%v", len(listed.Contents), objectKeys(listed))
	}

	var continuousKey string
	for _, obj := range listed.Contents {
		if obj.Key != startKey {
			continuousKey = obj.Key
			break
		}
	}
	if continuousKey == "" {
		t.Fatal("missing continuous delivery object")
	}
	_, body, err := st.GetObject(account, bucket, continuousKey)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Records []json.RawMessage `json:"Records"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Records) != 1 {
		t.Fatalf("Records=%d want 1 body=%s", len(payload.Records), body)
	}
	if !strings.Contains(string(payload.Records[0]), "ev-continuous") {
		t.Fatalf("continuous body missing ev-continuous: %s", body)
	}

	// Idempotent: second ship with no new lines leaves object count unchanged.
	if err := st.ShipCloudTrailContinuousDeliveries(dir); err != nil {
		t.Fatalf("second ship: %v", err)
	}
	listed, err = st.ListObjectsV2(account, bucket, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Contents) != 2 {
		t.Fatalf("after idle ship objects=%d want 2", len(listed.Contents))
	}
}

func TestCloudTrailContinuousDeliveryOptionalLogs(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const account = "000000000001"
	const bucket = "ct-logs-bucket"
	const group = "/noctaxris/cloudtrail"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogGroup(account, store.DefaultCloudTrailRegion, group); err != nil {
		t.Fatal(err)
	}

	writeCloudTrailFixture(t, dir,
		`{"eventTime":"2026-07-20T10:00:00Z","eventName":"A","eventID":"ev-a","eventSource":"x","userIdentity":{"userName":"u"}}`,
	)
	_, err = st.CreateCloudTrailTrail(account, store.CloudTrailTrail{
		Name:                      "logs-trail",
		S3BucketName:              bucket,
		CloudWatchLogsLogGroupArn: "arn:aws:logs:us-east-1:000000000001:log-group:" + group,
		HomeRegion:                store.DefaultCloudTrailRegion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.StartCloudTrailLogging(account, "logs-trail", dir); err != nil {
		t.Fatal(err)
	}

	appendCloudTrailFixture(t, dir,
		`{"eventTime":"2026-07-20T11:00:00Z","eventName":"B","eventID":"ev-b","eventSource":"x","userIdentity":{"userName":"u"}}`,
	)
	if err := st.ShipCloudTrailContinuousDeliveries(dir); err != nil {
		t.Fatal(err)
	}

	stream := store.CloudTrailLabLogStreamName("logs-trail")
	events, err := st.GetLogEvents(account, group, stream, 0, 0, true, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range events {
		if strings.Contains(ev.Message, "ev-b") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ev-b in log events got=%+v", events)
	}
}

func TestCloudTrailStopLoggingSkipsContinuous(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const account = "000000000001"
	const bucket = "ct-stop-bucket"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	writeCloudTrailFixture(t, dir,
		`{"eventTime":"2026-07-20T10:00:00Z","eventName":"A","eventID":"ev-a","eventSource":"x","userIdentity":{"userName":"u"}}`,
	)
	if _, err := st.CreateCloudTrailTrail(account, store.CloudTrailTrail{
		Name: "stop-trail", S3BucketName: bucket, HomeRegion: store.DefaultCloudTrailRegion,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.StartCloudTrailLogging(account, "stop-trail", dir); err != nil {
		t.Fatal(err)
	}
	if err := st.StopCloudTrailLogging(account, "stop-trail"); err != nil {
		t.Fatal(err)
	}
	appendCloudTrailFixture(t, dir,
		`{"eventTime":"2026-07-20T11:00:00Z","eventName":"B","eventID":"ev-b","eventSource":"x","userIdentity":{"userName":"u"}}`,
	)
	if err := st.ShipCloudTrailContinuousDeliveries(dir); err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListObjectsV2(account, bucket, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Contents) != 1 {
		t.Fatalf("after stop+ship objects=%d want 1 (start snapshot only)", len(listed.Contents))
	}
}

func objectKeys(listed store.ListObjectsResult) []string {
	out := make([]string, 0, len(listed.Contents))
	for _, o := range listed.Contents {
		out = append(out, o.Key)
	}
	return out
}
