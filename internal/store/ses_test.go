package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openSESStore(t *testing.T) *store.Store {
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

func TestSESCatcherRoundTrip(t *testing.T) {
	st := openSESStore(t)
	account := "000000000001"

	if err := st.VerifySESEmailIdentity(account, "sender@example.com"); err != nil {
		t.Fatal(err)
	}
	_, err := st.SendSESEmail(account, "unverified@example.com", []string{"to@example.com"}, "hi", "body", "")
	if err != store.ErrSESIdentityNotFound {
		t.Fatalf("want MessageRejected got %v", err)
	}
	msgID, err := st.SendSESEmail(account, "sender@example.com", []string{"to@example.com"}, "hi", "body", "")
	if err != nil || msgID == "" {
		t.Fatalf("msgID=%q err=%v", msgID, err)
	}
	msgs, err := st.ListSESMessages(account)
	if err != nil || len(msgs) != 1 || msgs[0].Subject != "hi" {
		t.Fatalf("msgs=%v err=%v", msgs, err)
	}
	stats, err := st.GetSESSendStatistics(account)
	if err != nil || stats.DeliveryAttempts != 1 {
		t.Fatalf("stats=%v err=%v", stats, err)
	}
	ids, err := st.ListSESIdentities(account, "")
	if err != nil || len(ids) != 1 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}
