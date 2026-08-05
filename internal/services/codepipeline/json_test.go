package codepipeline_test

import (
	"encoding/json"
	"testing"

	codepipelinesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/codepipeline"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCodePipelineJSON(t *testing.T) {
	p := store.CodePipelinePipeline{
		Name: "lab", Definition: `{"pipeline":{"name":"lab","stages":[]}}`,
	}
	raw, err := codepipelinesvc.CreatePipelineJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	raw, _ = codepipelinesvc.GetPipelineJSON(p)
	p.Definition = `{not json`
	raw, _ = codepipelinesvc.CreatePipelineJSON(p)

	ex := store.CodePipelineExecution{
		ExecutionID: "e1", PipelineName: "lab", Status: "InProgress", StartTime: 1_700_000_000_000,
		StageStates: `[{"stageName":"Source","status":"Succeeded","actionStates":[{"actionName":"Checkout","status":"Succeeded","token":"tok","projectName":"p1","buildId":"b1"},{"actionName":"bad"}]}]`,
	}
	raw, _ = codepipelinesvc.StartPipelineExecutionJSON(ex)
	_ = json.Unmarshal(raw, &out)

	raw, _ = codepipelinesvc.GetPipelineStateJSON("lab", ex)
	_ = json.Unmarshal(raw, &out)
	if out["pipelineName"] != "lab" {
		t.Fatalf("state=%v", out)
	}
	ex.ExecutionID = ""
	raw, _ = codepipelinesvc.GetPipelineStateJSON("lab", ex)
	ex.StageStates = `{invalid`
	raw, _ = codepipelinesvc.GetPipelineStateJSON("lab", ex)

	if _, err := codepipelinesvc.DeletePipelineJSON(); err != nil {
		t.Fatal(err)
	}
	raw, _ = codepipelinesvc.PutApprovalResultJSON(1_700_000_000_000)
	_ = json.Unmarshal(raw, &out)

	raw, _ = codepipelinesvc.GetPipelineExecutionJSON(ex)
	raw, _ = codepipelinesvc.ListPipelineExecutionsJSON([]store.CodePipelineExecution{ex})
	_ = json.Unmarshal(raw, &out)
}
