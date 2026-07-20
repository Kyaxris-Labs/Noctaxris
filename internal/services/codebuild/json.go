package codebuild

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateProjectJSON builds a CreateProject response.
func CreateProjectJSON(p store.CodeBuildProject) ([]byte, error) {
	return json.Marshal(map[string]any{
		"project": projectMap(p),
	})
}

// StartBuildJSON builds a StartBuild response.
func StartBuildJSON(b store.CodeBuildBuild) ([]byte, error) {
	return json.Marshal(map[string]any{
		"build": buildMap(b),
	})
}

// BatchGetBuildsJSON builds a BatchGetBuilds response.
func BatchGetBuildsJSON(builds []store.CodeBuildBuild) ([]byte, error) {
	items := make([]map[string]any, 0, len(builds))
	for _, b := range builds {
		items = append(items, buildMap(b))
	}
	return json.Marshal(map[string]any{
		"builds":       items,
		"buildsNotFound": []string{},
	})
}

// ListBuildsJSON builds a ListBuilds response.
func ListBuildsJSON(ids []string) ([]byte, error) {
	if ids == nil {
		ids = []string{}
	}
	return json.Marshal(map[string]any{"ids": ids})
}

func projectMap(p store.CodeBuildProject) map[string]any {
	var artifacts map[string]any
	_ = json.Unmarshal([]byte(p.Artifacts), &artifacts)
	if artifacts == nil {
		artifacts = map[string]any{"type": "NO_ARTIFACTS"}
	}
	return map[string]any{
		"name":        p.Name,
		"arn":         p.ARN,
		"description": p.Description,
		"serviceRole": p.ServiceRole,
		"source": map[string]any{
			"type":      p.SourceType,
			"location":  p.SourceLoc,
			"buildspec": p.Buildspec,
		},
		"environment": map[string]any{
			"type":        "LINUX_CONTAINER",
			"image":       p.Image,
			"computeType": "BUILD_GENERAL1_SMALL",
		},
		"artifacts":  artifacts,
		"created":    p.CreatedAt,
	}
}

func buildMap(b store.CodeBuildBuild) map[string]any {
	m := map[string]any{
		"id":          b.ID,
		"arn":         b.ARN,
		"projectName": b.ProjectName,
		"buildStatus": b.BuildStatus,
		"startTime":   b.StartTime,
		"source": map[string]any{
			"type":      b.SourceType,
			"location":  b.SourceLoc,
			"buildspec": b.Buildspec,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": b.Image,
		},
	}
	if b.EndTime != "" {
		m["endTime"] = b.EndTime
	}
	return m
}
