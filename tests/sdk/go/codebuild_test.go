package sdk_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

const codebuildTrust = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"codebuild.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

func TestCodeBuildControlPlaneAndOptionalStartBuild(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	iamc := newIAM(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	roleName := prefix + "-cb-role"
	if len(roleName) > 64 {
		roleName = roleName[:64]
	}
	roleOut, err := iamc.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(codebuildTrust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if roleOut.Role == nil || roleOut.Role.Arn == nil {
		t.Fatal("CreateRole missing Arn")
	}
	roleARN := *roleOut.Role.Arn
	t.Cleanup(func() {
		_, _ = iamc.DeleteRole(context.Background(), &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})

	repoName := "cb-src-" + strings.ReplaceAll(prefix, "_", "-")
	if len(repoName) > 100 {
		repoName = repoName[:100]
	}
	repoStatus, repoBody, _ := signedJSONTarget(t, "codecommit", "CodeCommit_20150413.CreateRepository", map[string]any{
		"repositoryName": repoName,
	})
	if repoStatus != 200 {
		t.Fatalf("CreateRepository status=%d body=%s", repoStatus, repoBody)
	}
	t.Cleanup(func() {
		_, _, _ = signedJSONTarget(t, "codecommit", "CodeCommit_20150413.DeleteRepository", map[string]any{
			"repositoryName": repoName,
		})
	})

	putStatus, putBody, putParsed := signedJSONTarget(t, "codecommit", "CodeCommit_20150413.PutFile", map[string]any{
		"repositoryName": repoName,
		"branchName":     "main",
		"filePath":       "README.md",
		"fileContent":    "hello-codecommit",
	})
	if putStatus != 200 {
		t.Fatalf("PutFile status=%d body=%s", putStatus, putBody)
	}
	if putParsed["commitId"] == nil || putParsed["commitId"] == "" {
		t.Fatalf("PutFile missing commitId: %s", putBody)
	}

	projectName := "cb-proj-" + strings.ReplaceAll(prefix, "_", "-")
	if len(projectName) > 100 {
		projectName = projectName[:100]
	}
	createStatus, createBody, createParsed := signedJSONTarget(t, "codebuild", "CodeBuild_20161006.CreateProject", map[string]any{
		"name":        projectName,
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "CODECOMMIT",
			"location":  repoName,
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["cat README.md"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	})
	if createStatus != 200 {
		t.Fatalf("CreateProject status=%d body=%s", createStatus, createBody)
	}
	proj, _ := createParsed["project"].(map[string]any)
	if proj == nil || proj["name"] != projectName {
		t.Fatalf("CreateProject missing project: %s", createBody)
	}
	t.Cleanup(func() {
		_, _, _ = signedJSONTarget(t, "codebuild", "CodeBuild_20161006.DeleteProject", map[string]any{
			"name": projectName,
		})
	})

	listStatus, listBody, listParsed := signedJSONTarget(t, "codebuild", "CodeBuild_20161006.ListProjects", map[string]any{})
	if listStatus != 200 {
		t.Fatalf("ListProjects status=%d body=%s", listStatus, listBody)
	}
	names, _ := listParsed["projects"].([]any)
	found := false
	for _, n := range names {
		if n == projectName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListProjects missing %s body=%s", projectName, listBody)
	}

	batchStatus, batchBody, batchParsed := signedJSONTarget(t, "codebuild", "CodeBuild_20161006.BatchGetProjects", map[string]any{
		"names": []any{projectName},
	})
	if batchStatus != 200 {
		t.Fatalf("BatchGetProjects status=%d body=%s", batchStatus, batchBody)
	}
	projects, _ := batchParsed["projects"].([]any)
	if len(projects) != 1 {
		t.Fatalf("BatchGetProjects len=%d body=%s", len(projects), batchBody)
	}
	got, _ := projects[0].(map[string]any)
	src, _ := got["source"].(map[string]any)
	if src["type"] != "CODECOMMIT" || src["location"] != repoName {
		t.Fatalf("BatchGetProjects source=%v body=%s", src, batchBody)
	}

	whStatus, whBody, whParsed := signedJSONTarget(t, "codebuild", "CodeBuild_20161006.CreateWebhook", map[string]any{
		"projectName": projectName,
		"filterGroups": []any{
			[]any{map[string]any{"type": "EVENT", "pattern": "PUSH"}},
		},
	})
	if whStatus != 200 {
		t.Fatalf("CreateWebhook status=%d body=%s", whStatus, whBody)
	}
	webhook, _ := whParsed["webhook"].(map[string]any)
	if webhook == nil || webhook["payloadUrl"] == nil || webhook["secret"] == nil {
		t.Fatalf("CreateWebhook missing payloadUrl/secret: %s", whBody)
	}
	listWhStatus, listWhBody, listWhParsed := signedJSONTarget(t, "codebuild", "CodeBuild_20161006.ListWebhooks", map[string]any{
		"projectName": projectName,
	})
	if listWhStatus != 200 {
		t.Fatalf("ListWebhooks status=%d body=%s", listWhStatus, listWhBody)
	}
	if webs, _ := listWhParsed["webhooks"].([]any); len(webs) != 1 {
		t.Fatalf("ListWebhooks len=%d body=%s", len(webs), listWhBody)
	}
	delWhStatus, delWhBody, _ := signedJSONTarget(t, "codebuild", "CodeBuild_20161006.DeleteWebhook", map[string]any{
		"projectName": projectName,
	})
	if delWhStatus != 200 {
		t.Fatalf("DeleteWebhook status=%d body=%s", delWhStatus, delWhBody)
	}

	if strings.TrimSpace(os.Getenv("NOCTAXRIS_NESTED")) != "1" {
		return
	}

	startStatus, startBody, startParsed := signedJSONTarget(t, "codebuild", "CodeBuild_20161006.StartBuild", map[string]any{
		"projectName": projectName,
	})
	if startStatus == 503 && strings.Contains(string(startBody), "compute unavailable") {
		t.Skipf("CodeBuild StartBuild skipped: compute unavailable (nested engine not healthy)")
	}
	if startStatus != 200 {
		t.Fatalf("StartBuild status=%d body=%s", startStatus, startBody)
	}
	build, _ := startParsed["build"].(map[string]any)
	if build == nil || build["id"] == nil || build["id"] == "" {
		t.Fatalf("StartBuild missing build.id: %s", startBody)
	}
}
