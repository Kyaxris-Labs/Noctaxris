package codebuild

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateProjectJSON builds a CreateProject response.
func CreateProjectJSON(p store.CodeBuildProject) ([]byte, error) {
	return json.Marshal(map[string]any{
		"project": projectMap(p),
	})
}

// UpdateProjectJSON builds an UpdateProject response.
func UpdateProjectJSON(p store.CodeBuildProject) ([]byte, error) {
	return json.Marshal(map[string]any{
		"project": projectMap(p),
	})
}

// DeleteProjectJSON builds an empty DeleteProject response.
func DeleteProjectJSON() ([]byte, error) {
	return []byte("{}"), nil
}

// ListProjectsJSON builds a ListProjects response (project names).
func ListProjectsJSON(projects []store.CodeBuildProject) ([]byte, error) {
	names := make([]string, 0, len(projects))
	for _, p := range projects {
		names = append(names, p.Name)
	}
	return json.Marshal(map[string]any{"projects": names})
}

// BatchGetProjectsJSON builds a BatchGetProjects response.
func BatchGetProjectsJSON(projects []store.CodeBuildProject, notFound []string) ([]byte, error) {
	items := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		items = append(items, projectMap(p))
	}
	if notFound == nil {
		notFound = []string{}
	}
	return json.Marshal(map[string]any{
		"projects":         items,
		"projectsNotFound": notFound,
	})
}

// StartBuildJSON builds a StartBuild response.
func StartBuildJSON(b store.CodeBuildBuild) ([]byte, error) {
	return json.Marshal(map[string]any{
		"build": buildMap(b),
	})
}

// StartBuildBatchJSON builds a StartBuildBatch response.
func StartBuildBatchJSON(batch store.CodeBuildBatch, builds []store.CodeBuildBuild) ([]byte, error) {
	return json.Marshal(map[string]any{
		"buildBatch": buildBatchMap(batch, builds),
	})
}

// StopBuildJSON builds a StopBuild response.
func StopBuildJSON(b store.CodeBuildBuild) ([]byte, error) {
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
		"builds":         items,
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
	m := map[string]any{
		"name":        p.Name,
		"arn":         p.ARN,
		"description": p.Description,
		"serviceRole": p.ServiceRole,
		"source": map[string]any{
			"type":          p.SourceType,
			"location":      p.SourceLoc,
			"buildspec":     p.Buildspec,
			"allowOverride": p.OverrideAllowed,
		},
		"environment": map[string]any{
			"type":                 "LINUX_CONTAINER",
			"image":                p.Image,
			"computeType":          "BUILD_GENERAL1_SMALL",
			"environmentVariables": envVarsAPIList(p.EnvVarsJSON),
		},
		"artifacts": artifacts,
		"created":   p.CreatedAt,
	}
	if v := decodeOptionalJSON(p.VpcConfigJSON); v != nil {
		m["vpcConfig"] = v
	}
	if v := decodeOptionalJSON(p.CacheJSON); v != nil {
		m["cache"] = v
	}
	if v := decodeOptionalJSON(p.SecondarySourcesJSON); v != nil {
		m["secondarySources"] = v
	}
	if v := decodeOptionalJSON(p.FleetJSON); v != nil {
		m["fleet"] = v
	}
	if v := decodeOptionalJSON(p.ReportArnsJSON); v != nil {
		m["reportGroupArns"] = v
	}
	return m
}

func decodeOptionalJSON(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return v
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
	if b.BatchID != "" {
		arn := strings.Replace(b.ARN, ":build/", ":build-batch/", 1)
		if strings.HasSuffix(arn, ":"+b.ID) {
			arn = strings.TrimSuffix(arn, ":"+b.ID) + ":" + b.BatchID
		}
		m["buildBatchArn"] = arn
	}
	if b.ArtifactLocation != "" {
		m["artifacts"] = map[string]any{
			"location": b.ArtifactLocation,
		}
	}
	if b.LogGroup != "" || b.LogStream != "" {
		logs := map[string]any{}
		if b.LogGroup != "" {
			logs["groupName"] = b.LogGroup
		}
		if b.LogStream != "" {
			logs["streamName"] = b.LogStream
		}
		if b.LogGroup != "" && b.LogStream != "" {
			logs["deepLink"] = codebuildLogsDeepLink(b.LogGroup, b.LogStream)
		}
		cw := map[string]any{"status": "ENABLED"}
		if b.LogGroup != "" {
			cw["groupName"] = b.LogGroup
		}
		if b.LogStream != "" {
			cw["streamName"] = b.LogStream
		}
		logs["cloudWatchLogs"] = cw
		m["logs"] = logs
	}
	return m
}

func buildBatchMap(batch store.CodeBuildBatch, builds []store.CodeBuildBuild) map[string]any {
	ids := batch.ChildBuildIDs
	if ids == nil {
		ids = make([]string, 0, len(builds))
		for _, b := range builds {
			ids = append(ids, b.ID)
		}
	}
	m := map[string]any{
		"id":               batch.ID,
		"arn":              batch.ARN,
		"projectName":      batch.ProjectName,
		"buildBatchStatus": batch.BuildBatchStatus,
		"startTime":        batch.StartTime,
		"complete":         batch.BuildBatchStatus != store.CodeBuildStatusInProgress,
		"serviceRole":      batch.ServiceRole,
		"source": map[string]any{
			"type":      batch.SourceType,
			"location":  batch.SourceLoc,
			"buildspec": batch.Buildspec,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": batch.Image,
		},
		"buildGroups": []map[string]any{
			{
				"identifier":    "BUILD",
				"dependsOn":     []string{},
				"ignoreFailure": false,
				"currentBuildSummary": map[string]any{
					"buildStatus": batch.BuildBatchStatus,
					"builds":      ids,
				},
			},
		},
	}
	if batch.EndTime != "" {
		m["endTime"] = batch.EndTime
	}
	return m
}

func envVarsAPIList(rawJSON string) []map[string]string {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" || rawJSON == "null" {
		return []map[string]string{}
	}
	var stored []store.CodeBuildEnvVar
	if err := json.Unmarshal([]byte(rawJSON), &stored); err != nil {
		return []map[string]string{}
	}
	out := make([]map[string]string, 0, len(stored))
	for _, v := range stored {
		if strings.TrimSpace(v.Name) == "" {
			continue
		}
		out = append(out, map[string]string{
			"name":  v.Name,
			"value": v.Value,
		})
	}
	return out
}

func codebuildLogsDeepLink(group, stream string) string {
	// Lab-shaped console deep link (not a live AWS console URL).
	return fmt.Sprintf(
		"https://console.aws.amazon.com/cloudwatch/home#logsV2:log-groups/log-group/%s/log-events/%s",
		url.PathEscape(group),
		url.PathEscape(stream),
	)
}
