package codebuild_test

import (
	"encoding/json"
	"strings"
	"testing"

	codebuildsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/codebuild"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func sampleProject() store.CodeBuildProject {
	return store.CodeBuildProject{
		Name:                 "lab-proj",
		ARN:                  "arn:aws:codebuild:us-east-1:000000000001:project/lab-proj",
		Description:          "desc",
		ServiceRole:          "arn:aws:iam::000000000001:role/CodeBuild",
		SourceType:           "CODECOMMIT",
		SourceLoc:            "repo",
		Buildspec:            "buildspec.yml",
		Image:                "aws/codebuild/standard:7.0",
		Artifacts:            `{"type":"S3","location":"s3://bucket/out"}`,
		EnvVarsJSON:          `[{"Name":"FOO","Value":"bar"},{"Name":"","Value":"skip"}]`,
		OverrideAllowed:      true,
		VpcConfigJSON:        `{"vpcId":"vpc-1"}`,
		CacheJSON:            `{"type":"LOCAL"}`,
		SecondarySourcesJSON: `[{"type":"GITHUB","location":"gh"}]`,
		FleetJSON:            `{"fleetArn":"arn:fleet"}`,
		ReportArnsJSON:       `["arn:aws:codebuild:us-east-1:000000000001:report-group/rg"]`,
		CreatedAt:            "2024-01-01T00:00:00Z",
	}
}

func sampleBuild() store.CodeBuildBuild {
	return store.CodeBuildBuild{
		ID:               "build-1",
		ARN:              "arn:aws:codebuild:us-east-1:000000000001:build/lab-proj:build-1",
		ProjectName:      "lab-proj",
		BuildStatus:      store.CodeBuildStatusSucceeded,
		SourceType:       "CODECOMMIT",
		SourceLoc:        "repo",
		Buildspec:        "buildspec.yml",
		Image:            "aws/codebuild/standard:7.0",
		StartTime:        "2024-01-01T00:00:00Z",
		EndTime:          "2024-01-01T00:05:00Z",
		LogGroup:         "/aws/codebuild/lab-proj",
		LogStream:        "stream-1",
		BatchID:          "batch-9",
		ArtifactLocation: "s3://bucket/out.zip",
	}
}

func TestCodeBuildProjectAndBuildJSON(t *testing.T) {
	p := sampleProject()
	createRaw, err := codebuildsvc.CreateProjectJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	updateRaw, err := codebuildsvc.UpdateProjectJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(createRaw) != string(updateRaw) {
		t.Fatalf("create/update mismatch")
	}
	var projOut map[string]any
	if err := json.Unmarshal(createRaw, &projOut); err != nil {
		t.Fatal(err)
	}
	proj, _ := projOut["project"].(map[string]any)
	if proj["name"] != "lab-proj" {
		t.Fatalf("project=%v", proj)
	}
	env, _ := proj["environment"].(map[string]any)
	vars, _ := env["environmentVariables"].([]any)
	if len(vars) != 1 {
		t.Fatalf("env vars=%v", vars)
	}

	listNames, err := codebuildsvc.ListProjectsJSON([]store.CodeBuildProject{p})
	if err != nil {
		t.Fatal(err)
	}
	batchGet, err := codebuildsvc.BatchGetProjectsJSON([]store.CodeBuildProject{p}, []string{"missing"})
	if err != nil {
		t.Fatal(err)
	}
	var batchOut map[string]any
	if err := json.Unmarshal(batchGet, &batchOut); err != nil {
		t.Fatal(err)
	}
	nf, _ := batchOut["projectsNotFound"].([]any)
	if len(nf) != 1 {
		t.Fatalf("notFound=%v", batchOut)
	}

	delProj, err := codebuildsvc.DeleteProjectJSON()
	if err != nil || string(delProj) != "{}" {
		t.Fatalf("delete project=%s err=%v", delProj, err)
	}

	b := sampleBuild()
	startRaw, err := codebuildsvc.StartBuildJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	stopRaw, err := codebuildsvc.StopBuildJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(startRaw) != string(stopRaw) {
		t.Fatalf("start/stop mismatch")
	}
	batch := store.CodeBuildBatch{
		ID:               "batch-9",
		ARN:              "arn:aws:codebuild:us-east-1:000000000001:build-batch/batch-9",
		ProjectName:      "lab-proj",
		BuildBatchStatus: store.CodeBuildStatusSucceeded,
		StartTime:        "2024-01-01T00:00:00Z",
		EndTime:          "2024-01-01T00:10:00Z",
		SourceType:       "CODECOMMIT",
		SourceLoc:        "repo",
		Buildspec:        "buildspec.yml",
		Image:            "aws/codebuild/standard:7.0",
		ServiceRole:      "arn:aws:iam::000000000001:role/CodeBuild",
		ChildBuildIDs:    []string{"build-1"},
	}
	batchRaw, err := codebuildsvc.StartBuildBatchJSON(batch, []store.CodeBuildBuild{b})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(batchRaw), "buildBatch") {
		t.Fatalf("batch=%s", batchRaw)
	}

	batchBuilds, err := codebuildsvc.BatchGetBuildsJSON([]store.CodeBuildBuild{b})
	if err != nil {
		t.Fatal(err)
	}
	listIDs, err := codebuildsvc.ListBuildsJSON([]string{"build-1"})
	if err != nil {
		t.Fatal(err)
	}
	nilList, err := codebuildsvc.ListBuildsJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(nilList) != string(listIDs) && len(listIDs) == 0 {
		t.Fatalf("list ids=%s", listIDs)
	}

	wh := store.CodeBuildWebhook{ProjectName: "lab-proj", Secret: "sec", FilterGroupsJSON: `[["EVENT","PUSH"]]`}
	whCreate, err := codebuildsvc.CreateWebhookJSON(wh, "https://lab/hook")
	if err != nil {
		t.Fatal(err)
	}
	whList, err := codebuildsvc.ListWebhooksJSON([]store.CodeBuildWebhook{wh}, func(name string) string {
		return "https://lab/hook/" + name
	})
	if err != nil {
		t.Fatal(err)
	}
	whNilList, err := codebuildsvc.ListWebhooksJSON(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(whCreate), "sec") || !strings.Contains(string(whList), "lab-proj") {
		t.Fatalf("webhook create=%s list=%s", whCreate, whList)
	}
	if !strings.Contains(string(whNilList), "webhooks") {
		t.Fatalf("nil list=%s", whNilList)
	}
	delWh, err := codebuildsvc.DeleteWebhookJSON()
	if err != nil || string(delWh) != "{}" {
		t.Fatalf("delete webhook err=%v", err)
	}

	// Invalid optional JSON branches (decodeOptionalJSON returns nil).
	bare := p
	bare.Artifacts = "not-json"
	bare.VpcConfigJSON = "null"
	bare.EnvVarsJSON = "{bad"
	rawBare, err := codebuildsvc.CreateProjectJSON(bare)
	if err != nil {
		t.Fatal(err)
	}
	var bareOut map[string]any
	if err := json.Unmarshal(rawBare, &bareOut); err != nil {
		t.Fatal(err)
	}
	projBare, _ := bareOut["project"].(map[string]any)
	art, _ := projBare["artifacts"].(map[string]any)
	if art["type"] != "NO_ARTIFACTS" {
		t.Fatalf("artifacts=%v", art)
	}
	_ = listNames
	_ = batchBuilds
}
