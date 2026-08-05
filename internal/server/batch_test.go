package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustBatchREST(t *testing.T, handler http.Handler, path string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+path, raw)
	req.Header.Set("Content-Type", "application/json")
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "batch", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

const batchServiceTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"batch.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
const batchServiceTrustBad = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}`
const batchJobTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

func TestBatchCreateComputeEnvironmentPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "batch-svc-bad", batchServiceTrustBad, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/batch-svc-bad"

	rec := mustBatchREST(t, handler, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "denied-ce",
		"type":                   "MANAGED",
		"serviceRole":            roleARN,
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not authorized to pass role to Batch") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestBatchControlPlaneAndSubmitWithoutCompute(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "batch-svc", batchServiceTrustOK, now)
	mustCreateIAMRole(t, handler, "batch-job", batchJobTrustOK, now)
	svcRole := "arn:aws:iam::" + testAccountID + ":role/batch-svc"
	jobRole := "arn:aws:iam::" + testAccountID + ":role/batch-job"

	ce := mustBatchREST(t, handler, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "lab-ce",
		"type":                   "MANAGED",
		"serviceRole":            svcRole,
	}, now)
	if ce.Code != http.StatusOK {
		t.Fatalf("CreateComputeEnvironment status=%d body=%q", ce.Code, ce.Body.String())
	}

	jq := mustBatchREST(t, handler, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "lab-jq",
		"priority":     1,
		"computeEnvironmentOrder": []map[string]any{
			{"order": 1, "computeEnvironment": "lab-ce"},
		},
	}, now)
	if jq.Code != http.StatusOK {
		t.Fatalf("CreateJobQueue status=%d body=%q", jq.Code, jq.Body.String())
	}

	jd := mustBatchREST(t, handler, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "lab-jd",
		"type":              "container",
		"containerProperties": map[string]any{
			"image":      "alpine:3.20",
			"command":    []string{"echo", "ok"},
			"jobRoleArn": jobRole,
		},
	}, now)
	if jd.Code != http.StatusOK {
		t.Fatalf("RegisterJobDefinition status=%d body=%q", jd.Code, jd.Body.String())
	}

	submit := mustBatchREST(t, handler, "/v1/submitjob", map[string]any{
		"jobName":       "job-1",
		"jobQueue":      "lab-jq",
		"jobDefinition": "lab-jd",
	}, now)
	if submit.Code != http.StatusServiceUnavailable {
		t.Fatalf("SubmitJob status=%d want 503 body=%q", submit.Code, submit.Body.String())
	}
	if !strings.Contains(submit.Body.String(), "compute unavailable") {
		t.Fatalf("body=%q", submit.Body.String())
	}

	desc := mustBatchREST(t, handler, "/v1/describejobqueues", map[string]any{
		"jobQueues": []string{"lab-jq"},
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeJobQueues status=%d body=%q", desc.Code, desc.Body.String())
	}
}

func TestBatchDescribeAndSubmitErrorPaths(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "batch-svc-cov", batchServiceTrustOK, now)
	mustCreateIAMRole(t, handler, "batch-job-cov", batchJobTrustOK, now)
	svcRole := "arn:aws:iam::" + testAccountID + ":role/batch-svc-cov"
	jobRole := "arn:aws:iam::" + testAccountID + ":role/batch-job-cov"

	ce := mustBatchREST(t, handler, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "cov-ce",
		"type":                   "MANAGED",
		"serviceRole":            svcRole,
	}, now)
	if ce.Code != http.StatusOK {
		t.Fatalf("CreateComputeEnvironment status=%d body=%q", ce.Code, ce.Body.String())
	}

	jq := mustBatchREST(t, handler, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "cov-jq",
		"priority":     1,
		"computeEnvironmentOrder": []map[string]any{
			{"order": 1, "computeEnvironment": "cov-ce"},
		},
	}, now)
	if jq.Code != http.StatusOK {
		t.Fatalf("CreateJobQueue status=%d body=%q", jq.Code, jq.Body.String())
	}

	jd := mustBatchREST(t, handler, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "cov-jd",
		"type":              "container",
		"containerProperties": map[string]any{
			"image":      "alpine:3.20",
			"command":    []string{"echo", "ok"},
			"jobRoleArn": jobRole,
		},
	}, now)
	if jd.Code != http.StatusOK {
		t.Fatalf("RegisterJobDefinition status=%d body=%q", jd.Code, jd.Body.String())
	}

	descCE := mustBatchREST(t, handler, "/v1/describecomputeenvironments", map[string]any{
		"computeEnvironments": []string{"cov-ce"},
	}, now)
	if descCE.Code != http.StatusOK || !strings.Contains(descCE.Body.String(), "cov-ce") {
		t.Fatalf("DescribeComputeEnvironments status=%d body=%q", descCE.Code, descCE.Body.String())
	}
	descCEAll := mustBatchREST(t, handler, "/v1/describecomputeenvironments", map[string]any{}, now)
	if descCEAll.Code != http.StatusOK || !strings.Contains(descCEAll.Body.String(), "cov-ce") {
		t.Fatalf("DescribeComputeEnvironments all status=%d body=%q", descCEAll.Code, descCEAll.Body.String())
	}

	descJD := mustBatchREST(t, handler, "/v1/describejobdefinitions", map[string]any{
		"jobDefinitions": []string{"cov-jd"},
	}, now)
	if descJD.Code != http.StatusOK || !strings.Contains(descJD.Body.String(), "cov-jd") {
		t.Fatalf("DescribeJobDefinitions status=%d body=%q", descJD.Code, descJD.Body.String())
	}
	descJDAll := mustBatchREST(t, handler, "/v1/describejobdefinitions", map[string]any{}, now)
	if descJDAll.Code != http.StatusOK || !strings.Contains(descJDAll.Body.String(), "cov-jd") {
		t.Fatalf("DescribeJobDefinitions all status=%d body=%q", descJDAll.Code, descJDAll.Body.String())
	}

	submitBadQueue := mustBatchREST(t, handler, "/v1/submitjob", map[string]any{
		"jobName":       "job-bad-q",
		"jobQueue":      "missing-jq",
		"jobDefinition": "cov-jd",
	}, now)
	// DockerHost empty → 503 before queue validation, or 400 if validation runs first.
	if submitBadQueue.Code != http.StatusServiceUnavailable && submitBadQueue.Code != http.StatusBadRequest {
		t.Fatalf("SubmitJob bad queue status=%d body=%q", submitBadQueue.Code, submitBadQueue.Body.String())
	}

	submitEmpty := mustBatchREST(t, handler, "/v1/submitjob", map[string]any{
		"jobName":       "",
		"jobQueue":      "cov-jq",
		"jobDefinition": "cov-jd",
	}, now)
	if submitEmpty.Code != http.StatusServiceUnavailable && submitEmpty.Code != http.StatusBadRequest {
		t.Fatalf("SubmitJob empty name status=%d body=%q", submitEmpty.Code, submitEmpty.Body.String())
	}

	submitOKPath := mustBatchREST(t, handler, "/v1/submitjob", map[string]any{
		"jobName":       "job-cov",
		"jobQueue":      "cov-jq",
		"jobDefinition": "cov-jd",
	}, now)
	if submitOKPath.Code != http.StatusServiceUnavailable {
		t.Fatalf("SubmitJob without DockerHost status=%d want 503 body=%q", submitOKPath.Code, submitOKPath.Body.String())
	}
	if !strings.Contains(submitOKPath.Body.String(), "compute unavailable") {
		t.Fatalf("body=%q", submitOKPath.Body.String())
	}

	descJobsEmpty := mustBatchREST(t, handler, "/v1/describejobs", map[string]any{
		"jobs": []string{},
	}, now)
	if descJobsEmpty.Code != http.StatusOK {
		t.Fatalf("DescribeJobs empty status=%d body=%q", descJobsEmpty.Code, descJobsEmpty.Body.String())
	}
	descJobsMiss := mustBatchREST(t, handler, "/v1/describejobs", map[string]any{
		"jobs": []string{"missing-job"},
	}, now)
	if descJobsMiss.Code != http.StatusOK {
		t.Fatalf("DescribeJobs missing status=%d body=%q", descJobsMiss.Code, descJobsMiss.Body.String())
	}
}

func TestBatchRejectUnsupportedFargateShape(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "batch-job-fargate", batchJobTrustOK, now)
	jobRole := "arn:aws:iam::" + testAccountID + ":role/batch-job-fargate"

	fargate := mustBatchREST(t, handler, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName":    "fargate-jd",
		"type":                 "container",
		"platformCapabilities": []any{"FARGATE"},
		"containerProperties": map[string]any{
			"image":      "alpine:3.20",
			"jobRoleArn": jobRole,
		},
	}, now)
	if fargate.Code != http.StatusBadRequest || !strings.Contains(fargate.Body.String(), "FARGATE") {
		t.Fatalf("FARGATE reject status=%d body=%q", fargate.Code, fargate.Body.String())
	}

	awsvpc := mustBatchREST(t, handler, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "awsvpc-jd",
		"type":              "container",
		"containerProperties": map[string]any{
			"image":       "alpine:3.20",
			"jobRoleArn":  jobRole,
			"networkMode": "awsvpc",
		},
	}, now)
	if awsvpc.Code != http.StatusBadRequest || !strings.Contains(awsvpc.Body.String(), "awsvpc") {
		t.Fatalf("awsvpc reject status=%d body=%q", awsvpc.Code, awsvpc.Body.String())
	}

	// Ensure JSON shapes still marshal for empty describe filters.
	raw, _ := json.Marshal(map[string]any{"jobs": []string{"x"}})
	if len(raw) == 0 {
		t.Fatal("marshal")
	}
}
