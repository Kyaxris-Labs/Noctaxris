package store_test

import (
	"errors"
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
	if err := st.EnsureAppConfigSchema(); err != nil {
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
	_, err = st.GetAppConfigConfiguration(account, app.ID, env.ID, profile.ID)
	if !errors.Is(err, store.ErrAppConfigNotFound) {
		t.Fatalf("expected not found before deploy, got=%v err=%v", ver, err)
	}
	dep, err := st.StartAppConfigDeployment(account, app.ID, env.ID, profile.ID, ver.VersionNumber)
	if err != nil || dep.State != "DEPLOYED" || dep.ConfigurationVersion != 1 {
		t.Fatalf("dep=%v err=%v", dep, err)
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

func TestAppConfigStartDeploymentUndeployedVsDeployed(t *testing.T) {
	st := openAppConfigStore(t)
	account := "000000000001"

	app, err := st.CreateAppConfigApplication(account, "deploy-app", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := st.CreateAppConfigEnvironment(account, app.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := st.CreateAppConfigProfile(account, app.ID, "cfg", "")
	if err != nil {
		t.Fatal(err)
	}

	v1, err := st.CreateAppConfigHostedVersion(account, app.ID, profile.ID, "application/json", []byte(`{"v":1}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.StartAppConfigDeployment(account, app.ID, env.ID, profile.ID, v1.VersionNumber)
	if err != nil {
		t.Fatal(err)
	}

	v2, err := st.CreateAppConfigHostedVersion(account, app.ID, profile.ID, "application/json", []byte(`{"v":2}`))
	if err != nil || v2.VersionNumber != 2 {
		t.Fatalf("v2=%v err=%v", v2, err)
	}

	got, err := st.GetAppConfigConfiguration(account, app.ID, env.ID, profile.ID)
	if err != nil || string(got.Content) != `{"v":1}` {
		t.Fatalf("before deploy v2: got=%q err=%v", got.Content, err)
	}

	dep, err := st.StartAppConfigDeployment(account, app.ID, env.ID, profile.ID, v2.VersionNumber)
	if err != nil || dep.State != "DEPLOYED" {
		t.Fatalf("dep=%v err=%v", dep, err)
	}

	got, err = st.GetAppConfigConfiguration(account, app.ID, env.ID, profile.ID)
	if err != nil || string(got.Content) != `{"v":2}` {
		t.Fatalf("after deploy v2: got=%q err=%v", got.Content, err)
	}

	token, err := st.StartAppConfigSession(account, app.ID, env.ID, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	content, _, _, label, err := st.GetLatestAppConfigConfiguration(token)
	if err != nil || string(content) != `{"v":2}` || label != "2" {
		t.Fatalf("latest content=%q label=%q err=%v", content, label, err)
	}

	fetched, err := st.GetAppConfigDeployment(account, dep.DeploymentID)
	if err != nil || fetched.ConfigurationVersion != 2 {
		t.Fatalf("get deployment=%v err=%v", fetched, err)
	}
	list, err := st.ListAppConfigDeployments(account, app.ID, env.ID, profile.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if list[0].ConfigurationVersion != 2 || list[1].ConfigurationVersion != 1 {
		t.Fatalf("list order/versions=%v", list)
	}
}
