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
// Approval actions expose token under actionStates[].latestExecution for PutApprovalResult.
func GetPipelineStateJSON(pipelineName string, e store.CodePipelineExecution) ([]byte, error) {
	var raw []map[string]any
	_ = json.Unmarshal([]byte(e.StageStates), &raw)
	stages := make([]map[string]any, 0, len(raw))
	for _, st := range raw {
		out := map[string]any{
			"stageName": st["stageName"],
			"status":    st["status"],
		}
		actionsIn, _ := st["actionStates"].([]any)
		actionsOut := make([]map[string]any, 0, len(actionsIn))
		for _, a := range actionsIn {
			am, _ := a.(map[string]any)
			if am == nil {
				continue
			}
			ao := map[string]any{"actionName": am["actionName"]}
			latest := map[string]any{}
			if status, ok := am["status"]; ok {
				latest["status"] = status
			}
			if token, ok := am["token"].(string); ok && token != "" {
				latest["token"] = token
			}
			if proj, ok := am["projectName"]; ok {
				ao["currentRevision"] = map[string]any{"revisionId": proj}
			}
			if buildID, ok := am["buildId"].(string); ok && buildID != "" {
				latest["externalExecutionId"] = buildID
			}
			if len(latest) > 0 {
				ao["latestExecution"] = latest
			}
			actionsOut = append(actionsOut, ao)
		}
		out["actionStates"] = actionsOut
		stages = append(stages, out)
	}
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

// PutApprovalResultJSON builds a PutApprovalResult response.
func PutApprovalResultJSON(approvedAtMillis int64) ([]byte, error) {
	sec := float64(approvedAtMillis) / 1000.0
	return json.Marshal(map[string]any{"approvedAt": sec})
}

// GetPipelineExecutionJSON builds a GetPipelineExecution response.
func GetPipelineExecutionJSON(e store.CodePipelineExecution) ([]byte, error) {
	return json.Marshal(map[string]any{
		"pipelineExecution": map[string]any{
			"pipelineExecutionId": e.ExecutionID,
			"pipelineName":        e.PipelineName,
			"status":              e.Status,
			"artifactRevisions":   []any{},
		},
	})
}

// ListPipelineExecutionsJSON builds a ListPipelineExecutions response.
func ListPipelineExecutionsJSON(execs []store.CodePipelineExecution) ([]byte, error) {
	summaries := make([]map[string]any, 0, len(execs))
	for _, e := range execs {
		summaries = append(summaries, map[string]any{
			"pipelineExecutionId": e.ExecutionID,
			"status":              e.Status,
			"startTime":           float64(e.StartTime) / 1000.0,
			"lastUpdateTime":      float64(e.StartTime) / 1000.0,
		})
	}
	return json.Marshal(map[string]any{"pipelineExecutionSummaries": summaries})
}
