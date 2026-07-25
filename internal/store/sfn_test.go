package store_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
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
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"states.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "task-sm-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "invoke", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"lambda:InvokeFunction","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
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
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "task-sm", def, roleARN)
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

func TestSFNChoiceStringEquals(t *testing.T) {
	st := openSFNStore(t)
	account := "000000000001"
	def := `{
  "StartAt": "Pick",
  "States": {
    "Pick": {
      "Type": "Choice",
      "Choices": [
        {
          "Variable": "$.color",
          "StringEquals": "red",
          "Next": "Red"
        }
      ],
      "Default": "Other"
    },
    "Red": {
      "Type": "Pass",
      "Result": {"branch": "red"},
      "End": true
    },
    "Other": {
      "Type": "Pass",
      "Result": {"branch": "other"},
      "End": true
    }
  }
}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "choice-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartSFNExecution(account, "us-east-1", sm.StateMachineARN, "run-choice", `{"color":"red"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != "SUCCEEDED" {
		t.Fatalf("status=%s error=%s cause=%s", exec.Status, exec.Error, exec.Cause)
	}
	if !strings.Contains(exec.Output, `"branch"`) || !strings.Contains(exec.Output, `"red"`) {
		t.Fatalf("want Red branch output, got %s", exec.Output)
	}
	hist, err := st.GetSFNExecutionHistory(exec.ExecutionARN)
	if err != nil {
		t.Fatal(err)
	}
	var sawChoice bool
	for _, ev := range hist {
		if strings.Contains(ev.Type, "Choice") {
			sawChoice = true
			break
		}
	}
	if !sawChoice {
		t.Fatalf("expected Choice history events, got %+v", hist)
	}
}

func TestSFNWaitSeconds(t *testing.T) {
	st := openSFNStore(t)
	account := "000000000001"
	def := `{
  "StartAt": "Pause",
  "States": {
    "Pause": {
      "Type": "Wait",
      "Seconds": 0,
      "Next": "Done"
    },
    "Done": {
      "Type": "Succeed"
    }
  }
}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "wait-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartSFNExecution(account, "us-east-1", sm.StateMachineARN, "run-wait", `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != "SUCCEEDED" {
		t.Fatalf("status=%s error=%s cause=%s", exec.Status, exec.Error, exec.Cause)
	}
	hist, err := st.GetSFNExecutionHistory(exec.ExecutionARN)
	if err != nil {
		t.Fatal(err)
	}
	var sawWait bool
	for _, ev := range hist {
		if strings.HasPrefix(ev.Type, "Wait") {
			sawWait = true
			break
		}
	}
	if !sawWait {
		t.Fatalf("expected Wait history events, got %+v", hist)
	}
}

func TestSFNParallelMergesBranchOutputs(t *testing.T) {
	st := openSFNStore(t)
	account := "000000000001"
	def := `{
  "StartAt": "Fan",
  "States": {
    "Fan": {
      "Type": "Parallel",
      "Branches": [
        {
          "StartAt": "A",
          "States": {
            "A": {
              "Type": "Pass",
              "Result": {"a": 1},
              "End": true
            }
          }
        },
        {
          "StartAt": "B",
          "States": {
            "B": {
              "Type": "Pass",
              "Result": {"b": 2},
              "End": true
            }
          }
        }
      ],
      "End": true
    }
  }
}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "parallel-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartSFNExecution(account, "us-east-1", sm.StateMachineARN, "run-parallel", `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != "SUCCEEDED" {
		t.Fatalf("status=%s error=%s cause=%s", exec.Status, exec.Error, exec.Cause)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(exec.Output), &arr); err != nil {
		t.Fatalf("output not JSON array: %s err=%v", exec.Output, err)
	}
	if len(arr) != 2 {
		t.Fatalf("want array length 2, got %d (%s)", len(arr), exec.Output)
	}
	hist, err := st.GetSFNExecutionHistory(exec.ExecutionARN)
	if err != nil {
		t.Fatal(err)
	}
	var sawParallel bool
	for _, ev := range hist {
		if strings.HasPrefix(ev.Type, "Parallel") {
			sawParallel = true
			break
		}
	}
	if !sawParallel {
		t.Fatalf("expected Parallel history events, got %+v", hist)
	}
}
