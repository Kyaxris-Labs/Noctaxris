package store_test

import (
	"os"
	"path/filepath"
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
