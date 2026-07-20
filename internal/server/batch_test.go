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
