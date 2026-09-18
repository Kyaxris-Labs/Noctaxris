package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestIoTJobsDataPlaneREST(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "job-thing",
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateThing %d %s", rec.Code, rec.Body.String())
	}

	create := mustJSONTarget(t, handler, "AWSIotService.CreateJob", "iot", map[string]any{
		"jobId":    "job-one",
		"targets":  []string{"job-thing"},
		"document": `{"op":"reboot"}`,
	}, now)
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), `"jobId":"job-one"`) {
		t.Fatalf("CreateJob %d %s", create.Code, create.Body.String())
	}

	pending := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/things/job-thing/jobs", "iot-jobs-data", nil, now)
	if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), `"job-one"`) {
		t.Fatalf("GetPending %d %s", pending.Code, pending.Body.String())
	}
	var pendingOut map[string]any
	if err := json.Unmarshal(pending.Body.Bytes(), &pendingOut); err != nil {
		t.Fatal(err)
	}
	queued, _ := pendingOut["queuedJobs"].([]any)
	if len(queued) != 1 {
		t.Fatalf("queuedJobs=%v", pendingOut["queuedJobs"])
	}

	desc := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/things/job-thing/jobs/job-one", "iot-jobs-data", nil, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), `"QUEUED"`) {
		t.Fatalf("DescribeJobExecution %d %s", desc.Code, desc.Body.String())
	}

	start := mustIoTREST(t, handler, http.MethodPut,
		"http://127.0.0.1:4566/things/job-thing/jobs/$next", "iot-jobs-data",
		[]byte(`{"statusDetails":{"step":"1"}}`), now)
	if start.Code != http.StatusOK || !strings.Contains(start.Body.String(), `"IN_PROGRESS"`) {
		t.Fatalf("StartNext %d %s", start.Code, start.Body.String())
	}

	again := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/things/job-thing/jobs", "iot-jobs-data", nil, now)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), `"inProgressJobs"`) {
		t.Fatalf("pending after start %d %s", again.Code, again.Body.String())
	}
}
