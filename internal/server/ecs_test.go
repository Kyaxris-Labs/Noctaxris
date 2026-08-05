package server_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
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
		"family":           family,
		"taskRoleArn":      taskRoleARN,
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

func writeECSTestTLSCerts(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "noctaxris-ecs-cov"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	for _, name := range []string{"ca.pem", "cert.pem"} {
		if err := os.WriteFile(filepath.Join(dir, name), certPEM, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestECSListDescribeStopTasksAndComputeError(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ecs-cov-task", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-cov-exec", ecsTrustOK, now)
	taskRole := "arn:aws:iam::" + testAccountID + ":role/ecs-cov-task"
	execRole := "arn:aws:iam::" + testAccountID + ":role/ecs-cov-exec"

	reg := mustECSJSON(t, handler, "RegisterTaskDefinition", map[string]any{
		"family": "cov-list-stop",
		"containerDefinitions": []map[string]any{{
			"name":      "app",
			"image":     "alpine:3.20",
			"essential": true,
			"command":   []any{"sh", "-c", "echo hi"},
			"environment": []any{
				map[string]any{"name": "FOO", "value": "bar"},
			},
		}},
		"taskRoleArn":      taskRole,
		"executionRoleArn": execRole,
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTaskDefinition status=%d body=%q", reg.Code, reg.Body.String())
	}

	td, err := st.RegisterTaskDefinition(testAccountID, testRegion, store.RegisterTaskDefinitionInput{
		Family: "cov-store-task",
		ContainerDefs: []map[string]any{{
			"name":    "app",
			"image":   "alpine:3.20",
			"command": []any{"true"},
			"environment": []any{
				map[string]any{"name": "A", "value": "1"},
			},
		}},
		TaskRoleARN:      taskRole,
		ExecutionRoleARN: execRole,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.RunTask(testAccountID, testRegion, store.RunTaskInput{
		Cluster:        "default",
		TaskDefinition: td.ARN,
	})
	if err != nil {
		t.Fatal(err)
	}

	list := mustECSJSON(t, handler, "ListTasks", map[string]any{"cluster": "default"}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), task.TaskARN) {
		t.Fatalf("ListTasks status=%d body=%q", list.Code, list.Body.String())
	}
	desc := mustECSJSON(t, handler, "DescribeTasks", map[string]any{
		"cluster": "default",
		"tasks":   []any{task.TaskARN},
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), task.TaskARN) {
		t.Fatalf("DescribeTasks status=%d body=%q", desc.Code, desc.Body.String())
	}

	stopMissing := mustECSJSON(t, handler, "StopTask", map[string]any{
		"cluster": "default",
	}, now)
	if stopMissing.Code != http.StatusBadRequest {
		t.Fatalf("StopTask missing task want 400 status=%d body=%q", stopMissing.Code, stopMissing.Body.String())
	}
	stopGone := mustECSJSON(t, handler, "StopTask", map[string]any{
		"cluster": "default",
		"task":    "arn:aws:ecs:" + testRegion + ":" + testAccountID + ":task/default/missing",
	}, now)
	if stopGone.Code != http.StatusBadRequest || !strings.Contains(stopGone.Body.String(), "Task not found") {
		t.Fatalf("StopTask missing want ClientException status=%d body=%q", stopGone.Code, stopGone.Body.String())
	}

	stop := mustECSJSON(t, handler, "StopTask", map[string]any{
		"cluster": "default",
		"task":    task.TaskARN,
	}, now)
	if stop.Code != http.StatusOK || !strings.Contains(stop.Body.String(), "STOPPED") {
		t.Fatalf("StopTask status=%d body=%q", stop.Code, stop.Body.String())
	}

	runNoDocker := mustECSJSON(t, handler, "RunTask", map[string]any{
		"cluster":        "default",
		"taskDefinition": "cov-list-stop",
	}, now)
	if runNoDocker.Code != http.StatusServiceUnavailable || !strings.Contains(runNoDocker.Body.String(), "compute unavailable") {
		t.Fatalf("RunTask no docker want 503 status=%d body=%q", runNoDocker.Code, runNoDocker.Body.String())
	}
}

func TestECSRunTaskHitsExecuteErrorBranches(t *testing.T) {
	// Allowlisted host + TLS PEMs so computeClient constructs; RunECSTask then fails closed without DinD.
	certDir := writeECSTestTLSCerts(t)
	t.Setenv(compute.EnvDockerHostAllowlist, "tcp://127.0.0.1:1")
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.DockerHost = "tcp://127.0.0.1:1"
		cfg.DockerTLSCertPath = certDir
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ecs-exec-task", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-exec-exec", ecsTrustOK, now)
	taskRole := "arn:aws:iam::" + testAccountID + ":role/ecs-exec-task"
	execRole := "arn:aws:iam::" + testAccountID + ":role/ecs-exec-exec"

	reg := mustECSJSON(t, handler, "RegisterTaskDefinition", map[string]any{
		"family": "cov-execute",
		"containerDefinitions": []map[string]any{{
			"name":      "app",
			"image":     "alpine:3.20",
			"essential": true,
			"command":   []any{"sh", "-c", "true"},
			"environment": []any{
				map[string]any{"name": "ENV", "value": "1"},
				map[string]any{"name": "", "value": "skip"},
				"bad",
			},
		}},
		"taskRoleArn":      taskRole,
		"executionRoleArn": execRole,
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTaskDefinition status=%d body=%q", reg.Code, reg.Body.String())
	}

	run := mustECSJSON(t, handler, "RunTask", map[string]any{
		"cluster":        "default",
		"taskDefinition": "cov-execute",
	}, now)
	if run.Code == http.StatusOK {
		t.Skip("DinD available; executeECSTask succeeded unexpectedly in this environment")
	}
	if run.Code != http.StatusServiceUnavailable && run.Code != http.StatusInternalServerError {
		t.Fatalf("RunTask with fake DockerHost want compute error status=%d body=%q", run.Code, run.Body.String())
	}
	if !strings.Contains(run.Body.String(), "compute unavailable") && !strings.Contains(run.Body.String(), "ServiceException") {
		t.Fatalf("expected compute fail-closed body=%q", run.Body.String())
	}

	regEmptyImg := mustECSJSON(t, handler, "RegisterTaskDefinition", map[string]any{
		"family": "cov-empty-img",
		"containerDefinitions": []map[string]any{{
			"name":      "app",
			"image":     "",
			"essential": true,
		}},
		"taskRoleArn":      taskRole,
		"executionRoleArn": execRole,
	}, now)
	if regEmptyImg.Code == http.StatusOK {
		runEmpty := mustECSJSON(t, handler, "RunTask", map[string]any{
			"cluster":        "default",
			"taskDefinition": "cov-empty-img",
		}, now)
		if runEmpty.Code == http.StatusOK {
			t.Fatal("RunTask empty image should fail closed")
		}
	}

	awsvpc := mustECSJSON(t, handler, "RunTask", map[string]any{
		"cluster":        "default",
		"taskDefinition": "cov-execute",
		"networkConfiguration": map[string]any{
			"awsvpcConfiguration": map[string]any{
				"subnets": []any{"subnet-1"},
			},
		},
	}, now)
	if awsvpc.Code != http.StatusBadRequest || !strings.Contains(awsvpc.Body.String(), "awsvpc") {
		t.Fatalf("RunTask awsvpc want InvalidParameter status=%d body=%q", awsvpc.Code, awsvpc.Body.String())
	}
}

func TestECSServiceHandlersCoverage(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	t.Cleanup(func() { srv.StopECSServiceReconciler() })

	mustCreateIAMRole(t, handler, "ecs-svc-cov-task", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-svc-cov-exec", ecsTrustOK, now)
	taskRole := "arn:aws:iam::" + testAccountID + ":role/ecs-svc-cov-task"
	execRole := "arn:aws:iam::" + testAccountID + ":role/ecs-svc-cov-exec"

	reg := mustECSJSON(t, handler, "RegisterTaskDefinition", map[string]any{
		"family": "svc-cov-td",
		"containerDefinitions": []map[string]any{{
			"name": "app", "image": "alpine:3.20", "essential": true,
		}},
		"taskRoleArn": taskRole, "executionRoleArn": execRole,
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTaskDefinition %d %s", reg.Code, reg.Body.String())
	}

	badTD := mustECSJSON(t, handler, "CreateService", map[string]any{
		"cluster": "default", "serviceName": "bad-td", "taskDefinition": "missing-family", "desiredCount": 0,
	}, now)
	if badTD.Code != http.StatusBadRequest || !strings.Contains(badTD.Body.String(), "task definition") {
		t.Fatalf("CreateService bad td want 400 got %d %s", badTD.Code, badTD.Body.String())
	}

	create := mustECSJSON(t, handler, "CreateService", map[string]any{
		"cluster": "default", "serviceName": "svc-cov", "taskDefinition": "svc-cov-td", "desiredCount": 0,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateService %d %s", create.Code, create.Body.String())
	}

	dup := mustECSJSON(t, handler, "CreateService", map[string]any{
		"cluster": "default", "serviceName": "svc-cov", "taskDefinition": "svc-cov-td", "desiredCount": 0,
	}, now)
	if dup.Code != http.StatusBadRequest {
		t.Fatalf("CreateService dup want 400 got %d %s", dup.Code, dup.Body.String())
	}

	missingSvc := mustECSJSON(t, handler, "DescribeServices", map[string]any{
		"cluster": "default", "services": []any{"no-such-svc"},
	}, now)
	if missingSvc.Code != http.StatusOK {
		t.Fatalf("DescribeServices missing %d", missingSvc.Code)
	}

	list := mustECSJSON(t, handler, "ListServices", map[string]any{"cluster": "default"}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "svc-cov") {
		t.Fatalf("ListServices %d %s", list.Code, list.Body.String())
	}

	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	svcMap, _ := created["service"].(map[string]any)
	svcARN, _ := svcMap["serviceArn"].(string)
	if svcARN == "" {
		t.Fatalf("missing serviceArn: %v", created)
	}

	tagMissing := mustECSJSON(t, handler, "TagResource", map[string]any{"tags": []any{}}, now)
	if tagMissing.Code != http.StatusBadRequest {
		t.Fatalf("TagResource missing arn want 400 got %d", tagMissing.Code)
	}
	tag := mustECSJSON(t, handler, "TagResource", map[string]any{
		"resourceArn": svcARN,
		"tags":        []map[string]any{{"key": "env", "value": "cov"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource %d %s", tag.Code, tag.Body.String())
	}
	listTags := mustECSJSON(t, handler, "ListTagsForResource", map[string]any{"resourceArn": svcARN}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource %d %s", listTags.Code, listTags.Body.String())
	}
	untag := mustECSJSON(t, handler, "UntagResource", map[string]any{
		"resourceArn": svcARN, "tagKeys": []any{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource %d %s", untag.Code, untag.Body.String())
	}

	updBadTD := mustECSJSON(t, handler, "UpdateService", map[string]any{
		"cluster": "default", "service": "svc-cov", "taskDefinition": "missing-td", "desiredCount": 0,
	}, now)
	if updBadTD.Code != http.StatusBadRequest {
		t.Fatalf("UpdateService bad td want 400 got %d %s", updBadTD.Code, updBadTD.Body.String())
	}

	upd := mustECSJSON(t, handler, "UpdateService", map[string]any{
		"cluster": "default", "serviceName": "svc-cov", "desiredCount": 0,
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateService %d %s", upd.Code, upd.Body.String())
	}

	// Scale-down reconcile: service task marked running, desired 0 triggers StopTask path.
	td, err := st.DescribeTaskDefinition(testAccountID, "svc-cov-td", 1)
	if err != nil {
		t.Fatal(err)
	}
	svcTask, err := st.RunServiceTask(testAccountID, testRegion, "default", "svc-cov", td.ARN)
	if err != nil {
		t.Fatal(err)
	}
	_ = svcTask.TaskARN
	scaleDown := mustECSJSON(t, handler, "UpdateService", map[string]any{
		"cluster": "default", "service": "svc-cov", "desiredCount": 0,
	}, now)
	if scaleDown.Code != http.StatusOK {
		t.Fatalf("UpdateService scale down %d %s", scaleDown.Code, scaleDown.Body.String())
	}

	updMissing := mustECSJSON(t, handler, "UpdateService", map[string]any{
		"cluster": "default", "service": "gone-svc", "desiredCount": 0,
	}, now)
	if updMissing.Code != http.StatusBadRequest || !strings.Contains(updMissing.Body.String(), "ServiceNotFound") {
		t.Fatalf("UpdateService missing want ServiceNotFound got %d %s", updMissing.Code, updMissing.Body.String())
	}

	delMissing := mustECSJSON(t, handler, "DeleteService", map[string]any{
		"cluster": "default", "service": "not-there",
	}, now)
	if delMissing.Code != http.StatusBadRequest {
		t.Fatalf("DeleteService missing want 400 got %d", delMissing.Code)
	}

	del := mustECSJSON(t, handler, "DeleteService", map[string]any{
		"cluster": "default", "service": "svc-cov",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteService %d %s", del.Code, del.Body.String())
	}
}

func TestECSServiceNegativesAndDupCoverage(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ecs-neg-task", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-neg-exec", ecsTrustOK, now)
	taskRole := "arn:aws:iam::" + testAccountID + ":role/ecs-neg-task"
	execRole := "arn:aws:iam::" + testAccountID + ":role/ecs-neg-exec"

	reg := mustECSJSON(t, handler, "RegisterTaskDefinition", map[string]any{
		"family": "neg-svc-td",
		"containerDefinitions": []map[string]any{
			{"name": "app", "image": "alpine:3.20", "essential": true},
		},
		"taskRoleArn":      taskRole,
		"executionRoleArn": execRole,
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTaskDefinition %d %s", reg.Code, reg.Body.String())
	}

	// Missing task definition
	missingTD := mustECSJSON(t, handler, "CreateService", map[string]any{
		"cluster": "default", "serviceName": "gone", "taskDefinition": "no-such-td", "desiredCount": 0,
	}, now)
	if missingTD.Code == http.StatusOK {
		t.Fatalf("CreateService missing td should fail: %s", missingTD.Body.String())
	}

	create := mustECSJSON(t, handler, "CreateService", map[string]any{
		"cluster": "default", "serviceName": "neg-web", "taskDefinition": "neg-svc-td", "desiredCount": 0,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateService %d %s", create.Code, create.Body.String())
	}

	// Duplicate service
	dup := mustECSJSON(t, handler, "CreateService", map[string]any{
		"cluster": "default", "serviceName": "neg-web", "taskDefinition": "neg-svc-td", "desiredCount": 0,
	}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("duplicate CreateService should fail: %s", dup.Body.String())
	}

	// Update missing service
	updMissing := mustECSJSON(t, handler, "UpdateService", map[string]any{
		"cluster": "default", "service": "no-svc", "desiredCount": 0,
	}, now)
	if updMissing.Code == http.StatusOK || !strings.Contains(updMissing.Body.String(), "ServiceNotFound") {
		t.Fatalf("UpdateService missing want ServiceNotFound got %d %s", updMissing.Code, updMissing.Body.String())
	}

	// Update via serviceName key + desiredCount scale (still 0; no DinD)
	upd := mustECSJSON(t, handler, "UpdateService", map[string]any{
		"cluster": "default", "serviceName": "neg-web", "desiredCount": 0, "taskDefinition": "neg-svc-td",
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateService %d %s", upd.Code, upd.Body.String())
	}

	list := mustECSJSON(t, handler, "ListServices", map[string]any{"cluster": "default"}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "neg-web") {
		t.Fatalf("ListServices %d %s", list.Code, list.Body.String())
	}
	desc := mustECSJSON(t, handler, "DescribeServices", map[string]any{
		"cluster": "default", "services": []any{"neg-web", "missing-svc"},
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeServices %d %s", desc.Code, desc.Body.String())
	}

	del := mustECSJSON(t, handler, "DeleteService", map[string]any{
		"cluster": "default", "service": "neg-web",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteService %d %s", del.Code, del.Body.String())
	}
	delGone := mustECSJSON(t, handler, "DeleteService", map[string]any{
		"cluster": "default", "service": "neg-web",
	}, now)
	// Lab DeleteService is idempotent for already-INACTIVE services (returns OK).
	if delGone.Code != http.StatusOK {
		t.Fatalf("DeleteService already-inactive %d %s", delGone.Code, delGone.Body.String())
	}
}
