package codepipeline

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreatePipelineJSON builds a CreatePipeline response.
func CreatePipelineJSON(p store.CodePipelinePipeline) ([]byte, error) {
	var decl any
	_ = json.Unmarshal([]byte(p.Definition), &decl)
	return json.Marshal(map[string]any{"pipeline": decl})
}

// GetPipelineJSON builds a GetPipeline response.
func GetPipelineJSON(p store.CodePipelinePipeline) ([]byte, error) {
	return CreatePipelineJSON(p)
}

// StartPipelineExecutionJSON builds a StartPipelineExecution response.
func StartPipelineExecutionJSON(e store.CodePipelineExecution) ([]byte, error) {
	return json.Marshal(map[string]any{"pipelineExecutionId": e.ExecutionID})
}

// GetPipelineStateJSON builds a GetPipelineState response.
func GetPipelineStateJSON(pipelineName string, e store.CodePipelineExecution) ([]byte, error) {
	var stages any
	_ = json.Unmarshal([]byte(e.StageStates), &stages)
	m := map[string]any{
		"pipelineName": pipelineName,
		"stageStates":  stages,
	}
	if e.ExecutionID != "" {
		m["latestExecution"] = map[string]any{
			"pipelineExecutionId": e.ExecutionID,
			"status":              e.Status,
		}
	}
	return json.Marshal(m)
}

// DeletePipelineJSON is an empty OK body.
func DeletePipelineJSON() ([]byte, error) {
	return []byte(`{}`), nil
}
