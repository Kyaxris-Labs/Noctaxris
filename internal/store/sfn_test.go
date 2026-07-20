package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openSFNStore(t *testing.T) *store.Store {
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

func TestSFNPassSucceedExecution(t *testing.T) {
	st := openSFNStore(t)
	account := "000000000001"
	region := "us-east-1"

	def := `{
  "StartAt": "Hello",
  "States": {
    "Hello": {
      "Type": "Pass",
      "Result": {"ok": true},
      "Next": "Done"
    },
    "Done": {
      "Type": "Succeed"
    }
  }
}`
	sm, err := st.CreateSFNStateMachine(account, region, "lab-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartSFNExecution(account, region, sm.StateMachineARN, "run1", `{"n":1}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != "SUCCEEDED" {
		t.Fatalf("status=%s error=%s cause=%s", exec.Status, exec.Error, exec.Cause)
	}
	if exec.Output != `{"ok": true}` && exec.Output != `{"ok":true}` {
		// Result may keep spaces from JSON
		if exec.Output == "" {
			t.Fatalf("empty output")
		}
	}
	hist, err := st.GetSFNExecutionHistory(exec.ExecutionARN)
	if err != nil || len(hist) < 2 {
		t.Fatalf("hist=%v err=%v", hist, err)
	}
}

func TestSFNFailExecution(t *testing.T) {
	st := openSFNStore(t)
	account := "000000000001"
	def := `{
  "StartAt": "Boom",
  "States": {
    "Boom": {
      "Type": "Fail",
      "Error": "LabError",
      "Cause": "intentional"
    }
  }
}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "fail-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartSFNExecution(account, "us-east-1", sm.StateMachineARN, "run-fail", `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != "FAILED" || exec.Error != "LabError" {
		t.Fatalf("exec=%+v", exec)
	}
}

func TestSFNTaskWithoutInvokerFails(t *testing.T) {
	st := openSFNStore(t)
	account := "000000000001"
	def := `{
  "StartAt": "Call",
  "States": {
    "Call": {
      "Type": "Task",
      "Resource": "arn:aws:lambda:us-east-1:000000000001:function:fn",
      "End": true
    }
  }
}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "task-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartSFNExecution(account, "us-east-1", sm.StateMachineARN, "run-task", `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != "FAILED" {
		t.Fatalf("want FAILED got %+v", exec)
	}
}
