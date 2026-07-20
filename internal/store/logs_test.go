package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openLogsStore(t *testing.T) *store.Store {
	t.Helper()
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
	return st
}

func TestLogsCreatePutGetRoundTrip(t *testing.T) {
	st := openLogsStore(t)
	account := "000000000001"

	if _, err := st.CreateLogGroup(account, "us-east-1", "/lab/app"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(account, "us-east-1", "/lab/app", "instance-1"); err != nil {
		t.Fatal(err)
	}

	next, _, err := st.PutLogEvents(account, "/lab/app", "instance-1", "", []store.LogEvent{
		{Timestamp: 1_000, Message: "hello"},
		{Timestamp: 2_000, Message: "world"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next == "" {
		t.Fatal("expected next sequence token")
	}

	_, _, err = st.PutLogEvents(account, "/lab/app", "instance-1", "bad-token", []store.LogEvent{
		{Timestamp: 3_000, Message: "nope"},
	})
	if !errors.Is(err, store.ErrInvalidSequenceToken) {
		t.Fatalf("err=%v want InvalidSequenceToken", err)
	}

	_, _, err = st.PutLogEvents(account, "/lab/app", "instance-1", next, []store.LogEvent{
		{Timestamp: 3_000, Message: "again"},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, err := st.GetLogEvents(account, "/lab/app", "instance-1", 0, 0, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("len=%d want 3", len(events))
	}
	if events[0].Message != "hello" || events[2].Message != "again" {
		t.Fatalf("events=%+v", events)
	}

	groups, err := st.DescribeLogGroups(account, "/lab")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].LogGroupName != "/lab/app" {
		t.Fatalf("groups=%+v", groups)
	}

	streams, err := st.DescribeLogStreams(account, "/lab/app", "inst")
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 1 || streams[0].LogStreamName != "instance-1" {
		t.Fatalf("streams=%+v", streams)
	}

	if err := st.DeleteLogStream(account, "/lab/app", "instance-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetLogEvents(account, "/lab/app", "instance-1", 0, 0, true, 10); !errors.Is(err, store.ErrLogStreamNotFound) {
		t.Fatalf("err=%v want stream not found", err)
	}
	if err := st.DeleteLogGroup(account, "/lab/app"); err != nil {
		t.Fatal(err)
	}
	groups, err = st.DescribeLogGroups(account, "/lab")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups after delete=%+v", groups)
	}
}

func TestLogsDuplicateGroup(t *testing.T) {
	st := openLogsStore(t)
	account := "000000000001"
	if _, err := st.CreateLogGroup(account, "us-east-1", "g"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogGroup(account, "us-east-1", "g"); !errors.Is(err, store.ErrLogGroupAlreadyExists) {
		t.Fatalf("err=%v", err)
	}
}
