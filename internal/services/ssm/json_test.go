package ssm_test

import (
	"encoding/json"
	"testing"

	ssmsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ssm"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSSMJSON(t *testing.T) {
	if _, err := ssmsvc.PutParameterJSON(2); err != nil {
		t.Fatal(err)
	}
	p := store.Parameter{
		Name: "/lab/key", Type: "SecureString", Value: "secret", Version: 1,
		LastModified: "2024-01-01T00:00:00Z", KeyID: "arn:aws:kms:us-east-1:1:key/k", Selector: ":1",
		ARN: "arn:aws:ssm:us-east-1:1:parameter/lab/key",
	}
	get, err := ssmsvc.GetParameterJSON(p, true)
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(get, &getOut); err != nil {
		t.Fatal(err)
	}
	if _, err := ssmsvc.LabelParameterVersionJSON(3, nil); err != nil {
		t.Fatal(err)
	}
	hist := []store.ParameterHistory{{
		Name: p.Name, Type: p.Type, Value: p.Value, Version: 1, KeyID: p.KeyID,
		LastModified: "2024-01-02T00:00:00Z", Labels: []string{"AWSCURRENT"},
	}}
	if _, err := ssmsvc.GetParameterHistoryJSON(hist, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ssmsvc.GetParametersJSON([]store.Parameter{p}, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ssmsvc.GetParametersByPathJSON([]store.Parameter{p}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ssmsvc.DescribeParametersJSON([]store.Parameter{p}); err != nil {
		t.Fatal(err)
	}
	if _, err := ssmsvc.EmptyOKJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := ssmsvc.ListTagsForResourceJSON([]store.ResourceTag{{Key: "k", Value: "v"}}); err != nil {
		t.Fatal(err)
	}
	cmd := store.SSMCommand{
		CommandID: "cmd-1", DocumentName: "AWS-RunShellScript", DocumentVersion: "$DEFAULT",
		Comment: "lab", Parameters: map[string][]string{"commands": {"echo hi"}},
		InstanceIDs: []string{"i-1"}, Status: "Success", StatusDetails: "done",
		TimeoutSeconds: 30, TargetCount: 1, CompletedCount: 1, RequestedAt: "2024-01-01T00:00:00Z",
		ExpiresAt: "2024-01-01T01:00:00Z",
	}
	if _, err := ssmsvc.SendCommandJSON(cmd); err != nil {
		t.Fatal(err)
	}
	inv := store.SSMCommandInvocation{
		CommandID: cmd.CommandID, InstanceID: "i-1", Status: "Success", Stdout: "ok", Stderr: "",
		ExecutionStart: "2024-01-01T00:00:01Z", ExecutionEnd: "2024-01-01T00:00:02Z", ResponseCode: 0,
		RequestedAt: "2024-01-01T00:00:00Z", DocumentName: cmd.DocumentName,
	}
	if _, err := ssmsvc.GetCommandInvocationJSON(inv); err != nil {
		t.Fatal(err)
	}
	if _, err := ssmsvc.ListCommandInvocationsJSON([]store.SSMCommandInvocation{inv}); err != nil {
		t.Fatal(err)
	}
}
