package store_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openCodeDeployStore(t *testing.T) *store.Store {
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
	if err := st.EnsureCodeDeploySchema(); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestCodeDeployApplicationGroupDeployment(t *testing.T) {
	st := openCodeDeployStore(t)
	account := "000000000001"

	if err := store.EnsureCodeDeploySchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}

	_, err := st.CreateCodeDeployApplication(account, "", "ECS")
	if !errors.Is(err, store.ErrCodeDeployBadRequest) {
		t.Fatalf("empty name err=%v", err)
	}
	app, err := st.CreateCodeDeployApplication(account, "LabApp", "")
	if err != nil {
		t.Fatal(err)
	}
	if app.ComputePlatform != "Server" || app.ApplicationID == "" {
		t.Fatalf("app=%+v", app)
	}
	_, err = st.CreateCodeDeployApplication(account, "LabApp", "ECS")
	if !errors.Is(err, store.ErrCodeDeployExists) {
		t.Fatalf("dup app err=%v", err)
	}
	gotApp, err := st.GetCodeDeployApplication(account, "LabApp")
	if err != nil || gotApp.ApplicationName != "LabApp" {
		t.Fatalf("get app=%+v err=%v", gotApp, err)
	}
	_, err = st.GetCodeDeployApplication(account, "missing")
	if !errors.Is(err, store.ErrCodeDeployNotFound) {
		t.Fatalf("missing app err=%v", err)
	}

	_, err = st.CreateCodeDeployDeploymentGroup(account, "", "dg", "", "", "", "")
	if !errors.Is(err, store.ErrCodeDeployBadRequest) {
		t.Fatalf("empty app/dg err=%v", err)
	}
	_, err = st.CreateCodeDeployDeploymentGroup(account, "missing", "dg", "", "", "", "")
	if !errors.Is(err, store.ErrCodeDeployNotFound) {
		t.Fatalf("dg without app err=%v", err)
	}
	dg, err := st.CreateCodeDeployDeploymentGroup(account, "LabApp", "LabDG", "arn:aws:iam::1:role/r", "web", "default", "fn")
	if err != nil {
		t.Fatal(err)
	}
	if dg.ECSServiceName != "web" || dg.LambdaFunctionName != "fn" {
		t.Fatalf("dg=%+v", dg)
	}
	_, err = st.CreateCodeDeployDeploymentGroup(account, "LabApp", "LabDG", "", "", "", "")
	if !errors.Is(err, store.ErrCodeDeployDGExists) {
		t.Fatalf("dup dg err=%v", err)
	}
	gotDG, err := st.GetCodeDeployDeploymentGroup(account, "LabApp", "LabDG")
	if err != nil || gotDG.DeploymentGroupName != "LabDG" {
		t.Fatalf("get dg=%+v err=%v", gotDG, err)
	}
	_, err = st.GetCodeDeployDeploymentGroup(account, "LabApp", "missing")
	if !errors.Is(err, store.ErrCodeDeployDGNotFound) {
		t.Fatalf("missing dg err=%v", err)
	}

	_, err = st.CreateCodeDeployDeployment(account, "LabApp", "missing", "x")
	if !errors.Is(err, store.ErrCodeDeployDGNotFound) {
		t.Fatalf("deploy without dg err=%v", err)
	}
	dep, err := st.CreateCodeDeployDeployment(account, "LabApp", "LabDG", "lab deploy")
	if err != nil {
		t.Fatal(err)
	}
	if dep.Status != "Succeeded" || !strings.HasPrefix(dep.DeploymentID, "d-") {
		t.Fatalf("dep=%+v", dep)
	}
	gotDep, err := st.GetCodeDeployDeployment(account, dep.DeploymentID)
	if err != nil || gotDep.DeploymentID != dep.DeploymentID {
		t.Fatalf("get dep=%+v err=%v", gotDep, err)
	}
	_, err = st.GetCodeDeployDeployment(account, "d-missing")
	if !errors.Is(err, store.ErrCodeDeployNotFound) {
		t.Fatalf("missing dep err=%v", err)
	}
	filtered, err := st.ListCodeDeployDeployments(account, "LabApp")
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filtered=%v err=%v", filtered, err)
	}
	all, err := st.ListCodeDeployDeployments(account, "")
	if err != nil || len(all) != 1 {
		t.Fatalf("all=%v err=%v", all, err)
	}
}
