package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustLogsJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Logs_20140328."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "logs", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestLogsRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createGroup := mustLogsJSON(t, handler, "CreateLogGroup", map[string]any{
		"logGroupName": "/lab/test",
	}, now)
	if createGroup.Code != http.StatusOK {
		t.Fatalf("CreateLogGroup status=%d body=%q", createGroup.Code, createGroup.Body.String())
	}

	createStream := mustLogsJSON(t, handler, "CreateLogStream", map[string]any{
		"logGroupName":  "/lab/test",
		"logStreamName": "s1",
	}, now)
	if createStream.Code != http.StatusOK {
		t.Fatalf("CreateLogStream status=%d body=%q", createStream.Code, createStream.Body.String())
	}

	put := mustLogsJSON(t, handler, "PutLogEvents", map[string]any{
		"logGroupName":  "/lab/test",
		"logStreamName": "s1",
		"logEvents": []map[string]any{
			{"timestamp": float64(now.UnixMilli()), "message": "line-one"},
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutLogEvents status=%d body=%q", put.Code, put.Body.String())
	}
	var putOut map[string]any
	if err := json.Unmarshal(put.Body.Bytes(), &putOut); err != nil {
		t.Fatal(err)
	}
	token, _ := putOut["nextSequenceToken"].(string)
	if token == "" {
		t.Fatalf("missing nextSequenceToken in %q", put.Body.String())
	}

	get := mustLogsJSON(t, handler, "GetLogEvents", map[string]any{
		"logGroupName":  "/lab/test",
		"logStreamName": "s1",
		"startFromHead": true,
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetLogEvents status=%d body=%q", get.Code, get.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	events, _ := getOut["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("events=%v", getOut["events"])
	}

	desc := mustLogsJSON(t, handler, "DescribeLogGroups", map[string]any{
		"logGroupNamePrefix": "/lab",
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeLogGroups status=%d body=%q", desc.Code, desc.Body.String())
	}
}
