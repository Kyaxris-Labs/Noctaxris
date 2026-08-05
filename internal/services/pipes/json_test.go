package pipes_test

import (
	"encoding/json"
	"testing"

	pipessvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/pipes"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestPipesJSON(t *testing.T) {
	p := store.Pipe{
		Name: "lab-pipe", ARN: "arn:aws:pipes:us-east-1:1:pipe/lab",
		Description: "d", DesiredState: "RUNNING", CurrentState: "RUNNING",
		SourceARN: "arn:aws:sqs:us-east-1:1:q", TargetARN: "arn:aws:lambda:us-east-1:1:f",
		RoleARN: "arn:aws:iam::1:role/pipe", CreatedAt: 1_700_000_000_000,
	}
	createRaw, err := pipessvc.CreatePipeJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	var createOut map[string]string
	if err := json.Unmarshal(createRaw, &createOut); err != nil {
		t.Fatal(err)
	}
	if createOut["Arn"] != p.ARN {
		t.Fatalf("create=%v", createOut)
	}

	descRaw, err := pipessvc.DescribePipeJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRaw, &descOut); err != nil {
		t.Fatal(err)
	}
	if descOut["Name"] != p.Name || descOut["RoleArn"] != p.RoleARN {
		t.Fatalf("describe=%v", descOut)
	}

	listRaw, err := pipessvc.ListPipesJSON([]store.Pipe{p})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	pipes, _ := listOut["Pipes"].([]any)
	if len(pipes) != 1 {
		t.Fatalf("list=%v", listOut)
	}
}
