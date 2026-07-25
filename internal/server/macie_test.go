package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustMacieJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "macie2", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestMacieInjectDisabledRejects(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	enable := mustMacieJSON(t, handler, "Macie2.EnableMacie", map[string]any{}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("enable status=%d body=%q", enable.Code, enable.Body.String())
	}
	rec := mustMacieJSON(t, handler, "NoctaxrisMacie.InjectFindings", map[string]any{
		"Finding": map[string]any{"type": "SensitiveData:S3Object/Personal"},
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q want 403", rec.Code, rec.Body.String())
	}
}

func TestMacieSessionJobInjectListGet(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.MacieInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	enable := mustMacieJSON(t, handler, "Macie2.EnableMacie", map[string]any{}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("enable status=%d body=%q", enable.Code, enable.Body.String())
	}
	sess := mustMacieJSON(t, handler, "Macie2.GetMacieSession", map[string]any{}, now)
	if sess.Code != http.StatusOK || !strings.Contains(sess.Body.String(), "ENABLED") {
		t.Fatalf("session status=%d body=%q", sess.Code, sess.Body.String())
	}

	createJob := mustMacieJSON(t, handler, "Macie2.CreateClassificationJob", map[string]any{
		"name":    "lab-scan",
		"jobType": "ONE_TIME",
		"s3JobDefinition": map[string]any{
			"bucketDefinitions": []map[string]any{
				{"accountId": testAccountID, "buckets": []string{"macie-src"}},
			},
		},
	}, now)
	if createJob.Code != http.StatusOK {
		t.Fatalf("create job status=%d body=%q", createJob.Code, createJob.Body.String())
	}
	var jobResp map[string]any
	if err := json.Unmarshal(createJob.Body.Bytes(), &jobResp); err != nil {
		t.Fatal(err)
	}
	jobID, _ := jobResp["jobId"].(string)
	if jobID == "" {
		t.Fatalf("jobResp=%v", jobResp)
	}

	if _, err := st.CreateBucket(testAccountID, "macie-src"); err != nil {
		t.Fatal(err)
	}
	body := []byte("ssn 987-65-4321\n")
	if _, err := st.PutObject(testAccountID, "macie-src", "pii.csv", store.PutObjectMeta{
		Data: body, ContentType: "text/csv", PlainSize: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}

	inject := mustMacieJSON(t, handler, "NoctaxrisMacie.InjectFindings", map[string]any{
		"JobId": jobID,
		"S3Objects": []map[string]any{
			{"Bucket": "macie-src", "Key": "pii.csv"},
		},
	}, now)
	if inject.Code != http.StatusOK {
		t.Fatalf("inject status=%d body=%q", inject.Code, inject.Body.String())
	}
	var inj map[string]any
	if err := json.Unmarshal(inject.Body.Bytes(), &inj); err != nil {
		t.Fatal(err)
	}
	rawIDs, _ := inj["findingIds"].([]any)
	if len(rawIDs) < 1 {
		t.Fatalf("inject=%v", inj)
	}

	list := mustMacieJSON(t, handler, "Macie2.ListFindings", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%q", list.Code, list.Body.String())
	}
	get := mustMacieJSON(t, handler, "Macie2.GetFindings", map[string]any{
		"findingIds": rawIDs,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "SensitiveData") {
		t.Fatalf("get status=%d body=%q", get.Code, get.Body.String())
	}
}
