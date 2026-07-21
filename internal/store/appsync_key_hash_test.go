package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAppSyncAPIKeyHashedAtRest(t *testing.T) {
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

	account := "000000000001"
	api, err := st.CreateAppSyncGraphqlAPI(account, "us-east-1", "hash-api", AppSyncAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateAppSyncAPIKey(account, api.APIID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.APIKey, "da2-") {
		t.Fatalf("plaintext key = %q", created.APIKey)
	}
	var stored string
	if err := st.db.QueryRow(`SELECT api_key FROM appsync_api_keys WHERE id = ?`, created.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" || stored == created.APIKey || strings.HasPrefix(stored, "da2-") {
		t.Fatalf("api_key at rest must be HMAC hash, got %q", stored)
	}
	gotAcct, gotAPI, err := st.LookupAppSyncAPIKey(created.APIKey)
	if err != nil || gotAcct != account || gotAPI != api.APIID {
		t.Fatalf("lookup: acct=%s api=%s err=%v", gotAcct, gotAPI, err)
	}
}
