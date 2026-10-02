package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestIoTRoleAliasDescribeListDeleteAndJobDescribe(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "iot-ra-ops", iotCredentialsTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/iot-ra-ops"

	create := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias":                 "ops-alias",
		"roleArn":                   roleARN,
		"credentialDurationSeconds": 1800,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateRoleAlias %d %s", create.Code, create.Body.String())
	}

	desc := mustJSONTarget(t, handler, "AWSIotService.DescribeRoleAlias", "iot", map[string]any{
		"roleAlias": "ops-alias",
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), `"ops-alias"`) {
		t.Fatalf("DescribeRoleAlias %d %s", desc.Code, desc.Body.String())
	}
	descMiss := mustJSONTarget(t, handler, "AWSIotService.DescribeRoleAlias", "iot", map[string]any{
		"roleAlias": "missing-alias",
	}, now)
	if descMiss.Code == http.StatusOK {
		t.Fatalf("DescribeRoleAlias missing should fail: %s", descMiss.Body.String())
	}

	list := mustJSONTarget(t, handler, "AWSIotService.ListRoleAliases", "iot", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "ops-alias") {
		t.Fatalf("ListRoleAliases %d %s", list.Code, list.Body.String())
	}

	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "ops-job-thing",
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateThing %d %s", rec.Code, rec.Body.String())
	}
	createJob := mustJSONTarget(t, handler, "AWSIotService.CreateJob", "iot", map[string]any{
		"jobId":    "ops-job",
		"targets":  []string{"ops-job-thing"},
		"document": map[string]any{"op": "ping"},
	}, now)
	if createJob.Code != http.StatusOK {
		t.Fatalf("CreateJob %d %s", createJob.Code, createJob.Body.String())
	}
	dupJob := mustJSONTarget(t, handler, "AWSIotService.CreateJob", "iot", map[string]any{
		"jobId":   "ops-job",
		"targets": []string{"ops-job-thing"},
	}, now)
	if dupJob.Code == http.StatusOK {
		t.Fatalf("duplicate CreateJob should fail: %s", dupJob.Body.String())
	}
	badJob := mustJSONTarget(t, handler, "AWSIotService.CreateJob", "iot", map[string]any{
		"jobId":   "",
		"targets": []string{"ops-job-thing"},
	}, now)
	if badJob.Code == http.StatusOK {
		t.Fatalf("empty jobId should fail: %s", badJob.Body.String())
	}
	missingThingJob := mustJSONTarget(t, handler, "AWSIotService.CreateJob", "iot", map[string]any{
		"jobId":   "ops-job-2",
		"targets": []string{"no-thing"},
	}, now)
	if missingThingJob.Code == http.StatusOK {
		t.Fatalf("missing thing CreateJob should fail: %s", missingThingJob.Body.String())
	}

	jobDesc := mustJSONTarget(t, handler, "AWSIotService.DescribeJob", "iot", map[string]any{
		"jobId": "ops-job",
	}, now)
	if jobDesc.Code != http.StatusOK || !strings.Contains(jobDesc.Body.String(), `"ops-job"`) {
		t.Fatalf("DescribeJob %d %s", jobDesc.Code, jobDesc.Body.String())
	}
	jobMiss := mustJSONTarget(t, handler, "AWSIotService.DescribeJob", "iot", map[string]any{
		"jobId": "missing-job",
	}, now)
	if jobMiss.Code == http.StatusOK {
		t.Fatalf("DescribeJob missing should fail: %s", jobMiss.Body.String())
	}

	named := mustJSONTarget(t, handler, "AWSIotDataService.ListNamedShadowsForThing", "iot-data", map[string]any{
		"thingName": "ops-job-thing",
	}, now)
	if named.Code != http.StatusOK {
		namedREST := mustIoTREST(t, handler, http.MethodGet,
			"http://127.0.0.1:4566/api/things/shadow/ListNamedShadowsForThing/ops-job-thing",
			"iotdevicegateway", nil, now)
		if namedREST.Code != http.StatusOK {
			t.Fatalf("ListNamedShadows JSON=%d %s REST=%d %s", named.Code, named.Body.String(), namedREST.Code, namedREST.Body.String())
		}
	}

	pendingJSON := mustJSONTarget(t, handler, "IotJobsDataPlane.GetPendingJobExecutions", "iot-jobs-data", map[string]any{
		"thingName": "ops-job-thing",
	}, now)
	if pendingJSON.Code != http.StatusOK || !strings.Contains(pendingJSON.Body.String(), "ops-job") {
		t.Fatalf("GetPendingJobExecutions %d %s", pendingJSON.Code, pendingJSON.Body.String())
	}

	descExec := mustJSONTarget(t, handler, "IotJobsDataPlane.DescribeJobExecution", "iot-jobs-data", map[string]any{
		"thingName": "ops-job-thing",
		"jobId":     "ops-job",
	}, now)
	if descExec.Code != http.StatusOK || !strings.Contains(descExec.Body.String(), `"QUEUED"`) {
		t.Fatalf("DescribeJobExecution %d %s", descExec.Code, descExec.Body.String())
	}
	descExecMiss := mustJSONTarget(t, handler, "IotJobsDataPlane.DescribeJobExecution", "iot-jobs-data", map[string]any{
		"thingName": "ops-job-thing",
		"jobId":     "missing-exec",
	}, now)
	if descExecMiss.Code == http.StatusOK {
		t.Fatalf("DescribeJobExecution missing should fail: %s", descExecMiss.Body.String())
	}

	startJSON := mustJSONTarget(t, handler, "IotJobsDataPlane.StartNextPendingJobExecution", "iot-jobs-data", map[string]any{
		"thingName":     "ops-job-thing",
		"statusDetails": map[string]string{"n": "1"},
	}, now)
	if startJSON.Code != http.StatusOK || !strings.Contains(startJSON.Body.String(), `"IN_PROGRESS"`) {
		t.Fatalf("StartNextPendingJobExecution %d %s", startJSON.Code, startJSON.Body.String())
	}
	emptyStart := mustJSONTarget(t, handler, "IotJobsDataPlane.StartNextPendingJobExecution", "iot-jobs-data", map[string]any{
		"thingName": "no-pending-thing",
	}, now)
	if emptyStart.Code != http.StatusOK {
		t.Fatalf("empty StartNext should return OK empty body, got %d %s", emptyStart.Code, emptyStart.Body.String())
	}

	del := mustJSONTarget(t, handler, "AWSIotService.DeleteRoleAlias", "iot", map[string]any{
		"roleAlias": "ops-alias",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteRoleAlias %d %s", del.Code, del.Body.String())
	}
	delMiss := mustJSONTarget(t, handler, "AWSIotService.DeleteRoleAlias", "iot", map[string]any{
		"roleAlias": "ops-alias",
	}, now)
	if delMiss.Code == http.StatusOK {
		t.Fatalf("DeleteRoleAlias twice should fail: %s", delMiss.Body.String())
	}
}
