package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openAppConfigStore(t *testing.T) *store.Store {
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

func TestAppConfigHostedRoundTrip(t *testing.T) {
	st := openAppConfigStore(t)
	account := "000000000001"

	app, err := st.CreateAppConfigApplication(account, "lab-app", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := st.CreateAppConfigEnvironment(account, app.ID, "dev", "")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := st.CreateAppConfigProfile(account, app.ID, "flags", "")
	if err != nil {
		t.Fatal(err)
	}
	ver, err := st.CreateAppConfigHostedVersion(account, app.ID, profile.ID, "application/json", []byte(`{"on":true}`))
	if err != nil || ver.VersionNumber != 1 {
		t.Fatalf("ver=%v err=%v", ver, err)
	}
	got, err := st.GetAppConfigConfiguration(account, app.ID, env.ID, profile.ID)
	if err != nil || string(got.Content) != `{"on":true}` {
		t.Fatalf("got=%v err=%v", got, err)
	}
	token, err := st.StartAppConfigSession(account, app.ID, env.ID, profile.ID)
	if err != nil || token == "" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	content, ct, next, label, err := st.GetLatestAppConfigConfiguration(token)
	if err != nil || string(content) != `{"on":true}` || ct == "" || next == "" || label != "1" {
		t.Fatalf("content=%q ct=%q next=%q label=%q err=%v", content, ct, next, label, err)
	}
}
