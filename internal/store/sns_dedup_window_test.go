package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFindSNSDedupMessageHonorsFiveMinuteWindow(t *testing.T) {
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
	if err := st.EnsureRoot("000000000001", "AKIAROOT1", "secret-root"); err != nil {
		t.Fatal(err)
	}

	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "dedup-window.fifo", map[string]string{
		"FifoTopic":                 "true",
		"ContentBasedDeduplication": "true",
	}); err != nil {
		t.Fatal(err)
	}
	first, err := st.PublishWithOpts(account, "dedup-window.fifo", "payload", "", nil, &PublishOpts{MessageGroupID: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	dup, err := st.PublishWithOpts(account, "dedup-window.fifo", "payload", "", nil, &PublishOpts{MessageGroupID: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.MessageID != dup.MessageID {
		t.Fatalf("within window want same id first=%s dup=%s", first.MessageID, dup.MessageID)
	}
	_, err = st.db.Exec(
		`UPDATE sns_published_messages SET created_at = ? WHERE message_id = ?`,
		time.Now().UTC().Add(-6*time.Minute).Format(time.RFC3339), first.MessageID,
	)
	if err != nil {
		t.Fatal(err)
	}
	again, err := st.PublishWithOpts(account, "dedup-window.fifo", "payload", "", nil, &PublishOpts{MessageGroupID: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	if again.MessageID == first.MessageID {
		t.Fatal("outside window should mint a new message id")
	}
}
