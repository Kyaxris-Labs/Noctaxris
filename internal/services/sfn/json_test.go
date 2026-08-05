package sfn_test

import (
	"encoding/json"
	"testing"

	sfnsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/sfn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSFNJSON(t *testing.T) {
	sm := store.SFNStateMachine{
		StateMachineARN: "arn:aws:states:us-east-1:1:stateMachine:lab",
		Name: "lab", Definition: `{"StartAt":"A"}`, RoleARN: "arn:role", CreationDate: 1_700_000_000_000,
	}
	raw, err := sfnsvc.CreateStateMachineJSON(sm)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	raw, _ = sfnsvc.DescribeStateMachineJSON(sm)
	_ = json.Unmarshal(raw, &out)
	if out["name"] != "lab" {
		t.Fatalf("describe sm=%v", out)
	}

	raw, _ = sfnsvc.ListStateMachinesJSON([]store.SFNStateMachine{sm})
	_ = json.Unmarshal(raw, &out)

	if _, err := sfnsvc.DeleteStateMachineJSON(); err != nil {
		t.Fatal(err)
	}

	ex := store.SFNExecution{
		ExecutionARN: "arn:exec", StateMachineARN: sm.StateMachineARN, Name: "run-1",
		Status: "SUCCEEDED", StartDate: 1_700_000_000_000, StopDate: 1_700_000_001_000,
		Input: `{}`, Output: `{"ok":true}`, Error: "States.TaskFailed", Cause: "boom",
	}
	raw, _ = sfnsvc.StartExecutionJSON(ex)
	_ = json.Unmarshal(raw, &out)

	raw, _ = sfnsvc.DescribeExecutionJSON(ex)
	_ = json.Unmarshal(raw, &out)
	if out["error"] != "States.TaskFailed" {
		t.Fatalf("exec=%v", out)
	}
	ex.StopDate, ex.Output, ex.Error, ex.Cause = 0, "", "", ""
	raw, _ = sfnsvc.DescribeExecutionJSON(ex)

	events := []store.SFNHistoryEvent{
		{ID: 1, Type: "ExecutionStarted", Timestamp: 1_700_000_000_000, Details: `{"input":"{}"}`},
		{ID: 2, Type: "ExecutionSucceeded", Timestamp: 1_700_000_001_000, Details: `{"output":"{}"}`},
		{ID: 3, Type: "ExecutionFailed", Timestamp: 1_700_000_002_000, Details: `{"error":"x"}`},
		{ID: 4, Type: "TaskStateEntered", Timestamp: 1_700_000_003_000, Details: `{bad`},
	}
	raw, _ = sfnsvc.GetExecutionHistoryJSON(events)
	_ = json.Unmarshal(raw, &out)
	evs, _ := out["events"].([]any)
	if len(evs) != 4 {
		t.Fatalf("history=%v", out)
	}
}
