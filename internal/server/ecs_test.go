package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustECSJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonECS."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "ecs", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

const ecsTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

const ecsTrustBad = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}`

func registerECSTaskDefinition(
	t *testing.T,
	handler http.Handler,
	family, taskRoleARN, executionRoleARN string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	return mustECSJSON(t, handler, "RegisterTaskDefinition", map[string]any{
		"family":      family,
		"taskRoleArn": taskRoleARN,
		"executionRoleArn": executionRoleARN,
		"containerDefinitions": []map[string]any{{
			"name":      "app",
			"image":     "alpine:3.20",
			"essential": true,
		}},
	}, now)
}

func TestECSRegisterTaskDefinitionPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ecs-task-bad", ecsTrustBad, now)
	mustCreateIAMRole(t, handler, "ecs-exec-bad", ecsTrustBad, now)
	taskRoleARN := "arn:aws:iam::" + testAccountID + ":role/ecs-task-bad"
	execRoleARN := "arn:aws:iam::" + testAccountID + ":role/ecs-exec-bad"

	rec := registerECSTaskDefinition(t, handler, "denied-task", taskRoleARN, execRoleARN, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("RegisterTaskDefinition status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("expected AccessDeniedException in %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not authorized to pass role to ECS") {
		t.Fatalf("expected pass role denial in %q", rec.Body.String())
	}
}

func TestECSRunTaskPassRoleDenyWithOverrides(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ecs-task-ok", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-exec-ok", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-override-bad", ecsTrustBad, now)
	taskRoleARN := "arn:aws:iam::" + testAccountID + ":role/ecs-task-ok"
	execRoleARN := "arn:aws:iam::" + testAccountID + ":role/ecs-exec-ok"
	badOverrideRole := "arn:aws:iam::" + testAccountID + ":role/ecs-override-bad"

	regRec := registerECSTaskDefinition(t, handler, "override-deny", taskRoleARN, execRoleARN, now)
	if regRec.Code != http.StatusOK {
		t.Fatalf("RegisterTaskDefinition status=%d body=%q", regRec.Code, regRec.Body.String())
	}

	runRec := mustECSJSON(t, handler, "RunTask", map[string]any{
		"cluster":        "default",
		"taskDefinition": "override-deny",
		"overrides": map[string]any{
			"taskRoleArn": badOverrideRole,
		},
	}, now)
	if runRec.Code != http.StatusForbidden {
		t.Fatalf("RunTask status=%d want 403 body=%q", runRec.Code, runRec.Body.String())
	}
	if !strings.Contains(runRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("expected AccessDeniedException in %q", runRec.Body.String())
	}
	if !strings.Contains(runRec.Body.String(), "not authorized to pass role to ECS") {
		t.Fatalf("expected pass role denial in %q", runRec.Body.String())
	}
}

func TestECSRunTaskWithoutDockerHost(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ecs-task", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-exec", ecsTrustOK, now)
	taskRoleARN := "arn:aws:iam::" + testAccountID + ":role/ecs-task"
	execRoleARN := "arn:aws:iam::" + testAccountID + ":role/ecs-exec"

	regRec := registerECSTaskDefinition(t, handler, "run-me", taskRoleARN, execRoleARN, now)
	if regRec.Code != http.StatusOK {
		t.Fatalf("RegisterTaskDefinition status=%d body=%q", regRec.Code, regRec.Body.String())
	}

	runRec := mustECSJSON(t, handler, "RunTask", map[string]any{
		"cluster":        "default",
		"taskDefinition": "run-me",
	}, now)
	if runRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("RunTask status=%d want 503 body=%q", runRec.Code, runRec.Body.String())
	}
	if !strings.Contains(runRec.Body.String(), "compute unavailable") {
		t.Fatalf("expected compute unavailable in %q", runRec.Body.String())
	}
}

func TestECSRegisterTaskDefinitionAndDescribe(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ecs-task-ok", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-exec-ok", ecsTrustOK, now)
	taskRoleARN := "arn:aws:iam::" + testAccountID + ":role/ecs-task-ok"
	execRoleARN := "arn:aws:iam::" + testAccountID + ":role/ecs-exec-ok"

	regRec := registerECSTaskDefinition(t, handler, "lab-task", taskRoleARN, execRoleARN, now)
	if regRec.Code != http.StatusOK {
		t.Fatalf("RegisterTaskDefinition status=%d body=%q", regRec.Code, regRec.Body.String())
	}

	descRec := mustECSJSON(t, handler, "DescribeTaskDefinition", map[string]any{
		"taskDefinition": "lab-task",
	}, now)
	if descRec.Code != http.StatusOK {
		t.Fatalf("DescribeTaskDefinition status=%d body=%q", descRec.Code, descRec.Body.String())
	}

	listRec := mustECSJSON(t, handler, "ListClusters", map[string]any{}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListClusters status=%d body=%q", listRec.Code, listRec.Body.String())
	}
}

func TestECSTaskStoreLifecycle(t *testing.T) {
	st := openECSStoreFromTest(t)
	account := testAccountID
	containers := []map[string]any{
		{"name": "app", "image": "alpine:3.20"},
	}

	td, err := st.RegisterTaskDefinition(account, testRegion, store.RegisterTaskDefinitionInput{
		Family:           "handler-lifecycle",
		ContainerDefs:    containers,
		TaskRoleARN:      "arn:aws:iam::" + account + ":role/ecsTaskRole",
		ExecutionRoleARN: "arn:aws:iam::" + account + ":role/ecsExecutionRole",
	})
	if err != nil {
		t.Fatal(err)
	}

	task, err := st.RunTask(account, testRegion, store.RunTaskInput{
		Cluster:        "default",
		TaskDefinition: td.ARN,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.LastStatus != store.ECSTaskStatusRunning {
		t.Fatalf("status=%q want RUNNING", task.LastStatus)
	}

	if err := st.SetTaskRuntimeID(account, task.TaskARN, "docker-cid-123"); err != nil {
		t.Fatal(err)
	}
	runtimeID, err := st.TaskRuntimeID(account, task.TaskARN)
	if err != nil || runtimeID != "docker-cid-123" {
		t.Fatalf("runtime id=%q err=%v", runtimeID, err)
	}

	stopped, err := st.StopTask(account, testRegion, store.StopTaskInput{
		Cluster: "default",
		Task:    task.TaskARN,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.LastStatus != store.ECSTaskStatusStopped {
		t.Fatalf("stop status=%q want STOPPED", stopped.LastStatus)
	}
}

func openECSStoreFromTest(t *testing.T) *store.Store {
	t.Helper()
	_, st, _ := newTestServerStore(t)
	return st
}
