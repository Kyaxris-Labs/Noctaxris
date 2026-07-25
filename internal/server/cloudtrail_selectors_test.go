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

func mustCloudTrailTarget(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "cloudtrail", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCloudTrailPutGetEventSelectors(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	const bucket = "ct-selectors-bucket"
	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatal(err)
	}

	create := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.CreateTrail", map[string]any{
		"Name":         "selector-trail",
		"S3BucketName": bucket,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTrail status=%d body=%q", create.Code, create.Body.String())
	}

	put := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.PutEventSelectors", map[string]any{
		"TrailName": "selector-trail",
		"EventSelectors": []map[string]any{
			{
				"ReadWriteType":           "All",
				"IncludeManagementEvents": true,
				"DataResources": []map[string]any{
					{"Type": "AWS::S3::Object", "Values": []string{"arn:aws:s3:::"}},
				},
			},
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutEventSelectors status=%d body=%q", put.Code, put.Body.String())
	}

	get := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.GetEventSelectors", map[string]any{
		"TrailName": "selector-trail",
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetEventSelectors status=%d body=%q", get.Code, get.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	selectors, _ := out["EventSelectors"].([]any)
	if len(selectors) != 1 {
		t.Fatalf("EventSelectors=%v", out["EventSelectors"])
	}
	first, _ := selectors[0].(map[string]any)
	resources, _ := first["DataResources"].([]any)
	if len(resources) == 0 {
		t.Fatalf("expected S3 DataResources in %+v", first)
	}
}

func TestCloudTrailValidateLogsAPI(t *testing.T) {
	srv, st, auditDir := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	const bucket = "ct-validate-bucket"
	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatal(err)
	}
	line := `{"eventTime":"2026-07-20T15:00:00Z","eventSource":"sts.amazonaws.com","eventName":"GetCallerIdentity","eventID":"ev-val","userIdentity":{"userName":"root"}}`
	if err := os.WriteFile(filepath.Join(auditDir, "events.jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustCloudTrailTarget(t, handler, "CloudTrail_20131101.CreateTrail", map[string]any{
		"Name": "val-trail", "S3BucketName": bucket,
	}, now)
	start := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.StartLogging", map[string]any{
		"Name": "val-trail",
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartLogging status=%d body=%q", start.Code, start.Body.String())
	}

	listed, err := st.ListObjectsV2(testAccountID, bucket, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var logKey string
	for _, o := range listed.Contents {
		if strings.Contains(o.Key, "/CloudTrail/") && !strings.Contains(o.Key, "CloudTrail-Digest") {
			logKey = o.Key
			break
		}
	}
	if logKey == "" {
		t.Fatalf("missing log key in %v", listed.Contents)
	}

	val := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.ValidateLogs", map[string]any{
		"S3BucketName": bucket,
		"S3ObjectKey":  logKey,
	}, now)
	if val.Code != http.StatusOK {
		t.Fatalf("ValidateLogs status=%d body=%q", val.Code, val.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(val.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if valid, _ := out["Valid"].(bool); !valid {
		t.Fatalf("Valid=%v body=%s", out["Valid"], val.Body.String())
	}
}
