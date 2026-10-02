package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCodePipelineStartRequiresCodeBuildStartBuild(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	proj := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name": "cp-sb-proj",
		"source": map[string]any{
			"type": "NO_SOURCE",
			"buildspec": "version: 0.2\nphases:\n  build:\n    commands:\n      - echo hi\n",
		},
		"environment": map[string]any{
			"type":                     "LINUX_CONTAINER",
			"image":                    "aws/codebuild/standard:7.0",
			"computeType":              "BUILD_GENERAL1_SMALL",
			"privilegedMode":           false,
			"imagePullCredentialsType": "CODEBUILD",
		},
		"serviceRole": "arn:aws:iam::" + testAccountID + ":role/codebuild-service",
	}, now)
	if proj.Code != http.StatusOK && proj.Code != http.StatusBadRequest {
		// Role may be missing; create role then project.
	}
	roleARN, err := st.CreateRole(testAccountID, "codebuild-service", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"codebuild.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil && !strings.Contains(err.Error(), "EntityAlreadyExists") {
		t.Fatal(err)
	}
	_ = roleARN
	proj = mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name": "cp-sb-proj",
		"source": map[string]any{
			"type": "NO_SOURCE",
			"buildspec": "version: 0.2\nphases:\n  build:\n    commands:\n      - echo hi\n",
		},
		"environment": map[string]any{
			"type":        "LINUX_CONTAINER",
			"image":       "aws/codebuild/standard:7.0",
			"computeType": "BUILD_GENERAL1_SMALL",
		},
		"serviceRole": "arn:aws:iam::" + testAccountID + ":role/codebuild-service",
	}, now)
	if proj.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", proj.Code, proj.Body.String())
	}

	pipeBody := map[string]any{
		"pipeline": map[string]any{
			"name": "cp-sb-pipe",
			"stages": []map[string]any{
				{
					"name": "Build",
					"actions": []map[string]any{
						{
							"name": "BuildAction",
							"actionTypeId": map[string]any{
								"category": "Build",
								"owner":    "AWS",
								"provider": "CodeBuild",
								"version":  "1",
							},
							"configuration": map[string]string{"ProjectName": "cp-sb-proj"},
						},
					},
				},
			},
		},
	}
	createPipe := mustJSONTarget(t, handler, "CodePipeline_20150709.CreatePipeline", "codepipeline", pipeBody, now)
	if createPipe.Code != http.StatusOK {
		t.Fatalf("CreatePipeline status=%d body=%q", createPipe.Code, createPipe.Body.String())
	}

	userName := "cp-no-startbuild"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowPipeOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["codepipeline:*"],"Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "CPOnly", allowPipeOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}

	raw, _ := json.Marshal(map[string]any{"name": "cp-sb-pipe"})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "CodePipeline_20150709.StartPipelineExecution")
	signHeader(t, req, raw, akid, secret, testRegion, "codepipeline", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "codebuild:StartBuild") {
		t.Fatalf("StartPipeline without StartBuild status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}

	allowBuild := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["codebuild:StartBuild"],"Resource":"*"}]}`
	buildARN, err := st.CreateManagedPolicy(testAccountID, "CPStartBuild", allowBuild)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, buildARN); err != nil {
		t.Fatal(err)
	}
	reqOK := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	reqOK.Header.Set("Content-Type", "application/x-amz-json-1.1")
	reqOK.Header.Set("X-Amz-Target", "CodePipeline_20150709.StartPipelineExecution")
	signHeader(t, reqOK, raw, akid, secret, testRegion, "codepipeline", now)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, reqOK)
	if okRec.Code != http.StatusOK {
		t.Fatalf("StartPipeline with StartBuild status=%d body=%q", okRec.Code, okRec.Body.String())
	}
}
