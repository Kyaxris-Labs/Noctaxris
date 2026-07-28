package store_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openSSMCommandStore(t *testing.T) *store.Store {
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
	if err := st.EnsureSSMSchema(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSSMCommandCreateGetUpdate(t *testing.T) {
	st := openSSMCommandStore(t)
	account := "000000000001"
	region := "us-east-1"

	cmd, invs, err := st.CreateSSMCommand(
		account, region,
		store.SSMDocumentRunShellScript, "$DEFAULT", "lab",
		map[string][]string{"commands": {"echo hi"}},
		[]string{"i-aaa", "i-bbb"},
		60,
	)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.CommandID == "" || cmd.Status != store.SSMCommandStatusInProgress {
		t.Fatalf("cmd=%+v", cmd)
	}
	if len(invs) != 2 {
		t.Fatalf("invs=%d", len(invs))
	}
	if invs[0].Status != store.SSMCommandStatusPending {
		t.Fatalf("inv status=%q", invs[0].Status)
	}

	got, err := st.GetSSMCommand(account, region, cmd.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DocumentName != store.SSMDocumentRunShellScript || len(got.InstanceIDs) != 2 {
		t.Fatalf("got=%+v", got)
	}
	if len(got.Parameters["commands"]) != 1 || got.Parameters["commands"][0] != "echo hi" {
		t.Fatalf("parameters=%v", got.Parameters)
	}

	if err := st.MarkSSMCommandInvocationInProgress(account, region, cmd.CommandID, "i-aaa", "2026-07-28T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateSSMCommandInvocationStatus(
		account, region, cmd.CommandID, "i-aaa",
		store.SSMCommandStatusSuccess, store.SSMCommandStatusSuccess,
		"hi\n", "", 0, "2026-07-28T00:00:00Z", "2026-07-28T00:00:01Z",
	); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateSSMCommandInvocationStatus(
		account, region, cmd.CommandID, "i-bbb",
		store.SSMCommandStatusFailed, store.SSMCommandStatusFailed,
		"", "boom", 1, "2026-07-28T00:00:00Z", "2026-07-28T00:00:02Z",
	); err != nil {
		t.Fatal(err)
	}

	inv, err := st.GetSSMCommandInvocation(account, region, cmd.CommandID, "i-aaa")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != store.SSMCommandStatusSuccess || inv.Stdout != "hi\n" || inv.ResponseCode != 0 {
		t.Fatalf("inv=%+v", inv)
	}

	cmd2, err := st.GetSSMCommand(account, region, cmd.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if cmd2.Status != store.SSMCommandStatusFailed || cmd2.CompletedCount != 2 || cmd2.ErrorCount != 1 {
		t.Fatalf("cmd2=%+v", cmd2)
	}

	list, err := st.ListSSMCommandInvocations(account, region, cmd.CommandID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list=%d", len(list))
	}
}

func TestSSMCommandTimeoutValidation(t *testing.T) {
	st := openSSMCommandStore(t)
	_, _, err := st.CreateSSMCommand(
		"000000000001", "us-east-1",
		store.SSMDocumentRunShellScript, "", "", nil,
		[]string{"i-1"}, 10,
	)
	if err == nil || !strings.Contains(err.Error(), "ValidationException") {
		t.Fatalf("err=%v", err)
	}
}

func TestSSMCommandInvocationNotFound(t *testing.T) {
	st := openSSMCommandStore(t)
	_, err := st.GetSSMCommandInvocation("000000000001", "us-east-1", "missing", "i-1")
	if !errors.Is(err, store.ErrSSMInvocationNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestTruncateSSMOutput(t *testing.T) {
	long := strings.Repeat("a", store.SSMMaxStdoutChars+10)
	got := store.TruncateSSMOutput(long, store.SSMMaxStdoutChars)
	if len(got) != store.SSMMaxStdoutChars {
		t.Fatalf("len=%d", len(got))
	}
}
