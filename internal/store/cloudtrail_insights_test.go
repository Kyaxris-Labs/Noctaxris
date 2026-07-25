package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLookupCloudTrailEventsEventCategoryInsight(t *testing.T) {
	dir := t.TempDir()
	ctDir := filepath.Join(dir, "cloudtrail")
	if err := os.MkdirAll(ctDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ctDir, "events.jsonl")
	lines := "" +
		`{"eventTime":"2026-07-20T10:00:00Z","eventName":"GetCallerIdentity","eventID":"ev-mgmt","eventSource":"sts.amazonaws.com","eventCategory":"Management","recipientAccountId":"000000000001"}` + "\n" +
		`{"eventVersion":"1.09","eventTime":"2026-07-20T10:05:00Z","eventID":"ev-insight","eventType":"AwsCloudTrailInsight","eventCategory":"Insight","eventName":"AssumeRole","eventSource":"sts.amazonaws.com","recipientAccountId":"000000000001","sharedEventID":"shared-1","insightDetails":{"state":"Start","eventSource":"sts.amazonaws.com","eventName":"AssumeRole","insightType":"ApiCallRateInsight","sourceEventCategory":"Management"}}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	mgmt, err := store.LookupCloudTrailEvents(dir, store.CloudTrailLookupFilter{
		RecipientAccountID: "000000000001",
		MaxResults:         50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mgmt) != 1 || mgmt[0].EventId != "ev-mgmt" {
		t.Fatalf("default lookup=%+v want only management", mgmt)
	}

	insight, err := store.LookupCloudTrailEvents(dir, store.CloudTrailLookupFilter{
		RecipientAccountID: "000000000001",
		EventCategory:      "insight",
		MaxResults:         50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(insight) != 1 || insight[0].EventId != "ev-insight" {
		t.Fatalf("insight lookup=%+v", insight)
	}
	if insight[0].EventName != "AssumeRole" || insight[0].EventSource != "sts.amazonaws.com" {
		t.Fatalf("insight fields=%+v", insight[0])
	}
}
