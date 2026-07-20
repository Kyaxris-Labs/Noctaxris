package pipes

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreatePipeJSON builds a CreatePipe success body.
func CreatePipeJSON(p store.Pipe) ([]byte, error) {
	return json.Marshal(map[string]any{"Arn": p.ARN})
}

// DescribePipeJSON builds a DescribePipe success body.
func DescribePipeJSON(p store.Pipe) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Name":         p.Name,
		"Arn":          p.ARN,
		"Description":  p.Description,
		"DesiredState": p.DesiredState,
		"CurrentState": p.CurrentState,
		"Source":       p.SourceARN,
		"Target":       p.TargetARN,
		"RoleArn":      p.RoleARN,
		"CreationTime": float64(p.CreatedAt) / 1000.0,
	})
}

// ListPipesJSON builds a ListPipes success body.
func ListPipesJSON(pipes []store.Pipe) ([]byte, error) {
	summaries := make([]map[string]any, 0, len(pipes))
	for _, p := range pipes {
		summaries = append(summaries, map[string]any{
			"Name":         p.Name,
			"Arn":          p.ARN,
			"DesiredState": p.DesiredState,
			"CurrentState": p.CurrentState,
			"Source":       p.SourceARN,
			"Target":       p.TargetARN,
			"CreationTime": float64(p.CreatedAt) / 1000.0,
		})
	}
	return json.Marshal(map[string]any{"Pipes": summaries})
}
