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
	id, err := st.CreateSESIdentityV2(account, "v2@example.com")
	if err != nil || !id.Verified {
		t.Fatalf("create v2 email id=%+v err=%v", id, err)
	}
	if _, err := st.CreateSESIdentityV2(account, "v2@example.com"); err != store.ErrSESIdentityAlreadyExists {
		t.Fatalf("want AlreadyExists got %v", err)
	}
	domain, err := st.CreateSESIdentityV2(account, "example.com")
	if err != nil || domain.Verified {
		t.Fatalf("domain=%+v err=%v", domain, err)
	}
	got, err := st.GetSESIdentity(account, "v2@example.com")
	if err != nil || got.Identity != "v2@example.com" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	if err := st.DeleteSESIdentity(account, "v2@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSESIdentity(account, "v2@example.com"); err != store.ErrSESIdentityNotFoundV2 {
		t.Fatalf("want not found got %v", err)
	}
	_, err = st.SendSESEmail(account, "unverified@example.com", []string{"to@example.com"}, "hi", "body", "")
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
	if err != nil || len(ids) < 2 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}
