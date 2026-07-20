package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustCloudTrailJSON(t *testing.T, handler http.Handler, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "CloudTrail_20131101.LookupEvents")
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "cloudtrail", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCloudTrailLookupEvents(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	line := `{"eventVersion":"1.08","eventTime":"2026-07-20T15:00:00Z","eventSource":"sts.amazonaws.com","eventName":"GetCallerIdentity","eventID":"ev-lookup-1","eventType":"AwsApiCall","recipientAccountId":"000000000001","readOnly":true,"userIdentity":{"type":"IAMUser","userName":"root","arn":"arn:aws:iam::000000000001:root","accessKeyId":"` + testAccessKey + `"}}`
	if err := os.WriteFile(filepath.Join(auditDir, "events.jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := mustCloudTrailJSON(t, handler, map[string]any{
		"LookupAttributes": []map[string]string{
			{"AttributeKey": "EventName", "AttributeValue": "GetCallerIdentity"},
		},
		"MaxResults": 10,
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	events, _ := out["Events"].([]any)
	if len(events) < 1 {
		t.Fatalf("Events=%v body=%q", out["Events"], rec.Body.String())
	}
	first, _ := events[0].(map[string]any)
	if name, _ := first["EventName"].(string); name != "GetCallerIdentity" {
		t.Fatalf("EventName=%v", first["EventName"])
	}
	cte, _ := first["CloudTrailEvent"].(string)
	if !strings.Contains(cte, "ev-lookup-1") {
		t.Fatalf("CloudTrailEvent=%q", cte)
	}
}
