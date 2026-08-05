package store

import (
	"path/filepath"
	"testing"
	"time"
)

// Internal helpers that exported store_test cannot reach.
func TestSNSCoverageWave2InternalHelpers(t *testing.T) {
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	client := snsHTTPClient()
	if client == nil || client.Timeout != 3*time.Second || client.Transport == nil {
		t.Fatalf("snsHTTPClient=%+v", client)
	}
	if err := client.CheckRedirect(nil, nil); err == nil {
		t.Fatal("CheckRedirect must deny redirects")
	}

	account := "000000000001"
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "wave2-resolve-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	name, err := st.resolveSQSQueueName(account, q.QueueARN)
	if err != nil || name != q.QueueName {
		t.Fatalf("resolve arn name=%q err=%v", name, err)
	}
	name2, err := st.resolveSQSQueueName(account, q.QueueName)
	if err != nil || name2 != q.QueueName {
		t.Fatalf("resolve bare name=%q err=%v", name2, err)
	}
	if _, err := st.resolveSQSQueueName(account, "missing-queue"); err == nil {
		t.Fatal("missing queue must fail")
	}
	parsed, err := topicNameFromARN(q.QueueARN)
	if err != nil {
		t.Fatalf("queue ARN last-segment parse: %v", err)
	}
	if parsed != q.QueueName {
		t.Fatalf("topicNameFromARN on queue ARN got %q want %q", parsed, q.QueueName)
	}
	if _, err := topicNameFromARN("no-colon"); err == nil {
		t.Fatal("ARN without colon must fail")
	}
	if _, err := topicNameFromARN("endswith:"); err == nil {
		t.Fatal("trailing colon must fail")
	}
}
