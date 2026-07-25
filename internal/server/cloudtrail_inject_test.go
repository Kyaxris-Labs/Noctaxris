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

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustCloudTrailInject(t *testing.T, handler http.Handler, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "NoctaxrisCloudTrail.InjectEvents")
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "cloudtrail", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCloudTrailInjectDisabledRejects(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustCloudTrailInject(t, handler, map[string]any{
		"Event": map[string]any{
			"eventName":       "ConsoleLogin",
			"eventSource":     "signin.amazonaws.com",
			"eventTime":       "2026-07-20T15:00:00Z",
			"sourceIPAddress": "198.51.100.10",
			"userIdentity":    map[string]any{"userName": "alice", "type": "IAMUser"},
		},
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q want 403", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("body=%q want AccessDenied", rec.Body.String())
	}
}

func TestCloudTrailInjectWritesAndLookupFilters(t *testing.T) {
	srv, _, auditDir := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.CloudTrailInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustCloudTrailInject(t, handler, map[string]any{
		"Events": []map[string]any{
			{
				"eventName":       "ConsoleLogin",
				"eventSource":     "signin.amazonaws.com",
				"eventTime":       "2026-07-20T15:00:00Z",
				"sourceIPAddress": "198.51.100.10",
				"userIdentity":    map[string]any{"userName": "alice", "type": "IAMUser"},
				"eventID":         "inj-alice-login",
				"readOnly":        false,
			},
			{
				"eventName":       "DeleteBucket",
				"eventSource":     "s3.amazonaws.com",
				"eventTime":       "2026-07-20T16:00:00Z",
				"sourceIPAddress": "203.0.113.5",
				"userIdentity":    map[string]any{"userName": "bob", "type": "IAMUser"},
				"eventID":         "inj-bob-delete",
			},
		},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("inject status=%d body=%q", rec.Code, rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "inj-alice-login") || !strings.Contains(string(data), "198.51.100.10") {
		t.Fatalf("events.jsonl missing injected line: %s", data)
	}

	lookup := func(key, value string) *httptest.ResponseRecorder {
		return mustCloudTrailJSON(t, handler, map[string]any{
			"LookupAttributes": []map[string]string{
				{"AttributeKey": key, "AttributeValue": value},
			},
			"MaxResults": 10,
		}, now)
	}

	byName := lookup("EventName", "ConsoleLogin")
	if byName.Code != http.StatusOK || !strings.Contains(byName.Body.String(), "inj-alice-login") {
		t.Fatalf("EventName filter: status=%d body=%q", byName.Code, byName.Body.String())
	}
	byUser := lookup("Username", "bob")
	if byUser.Code != http.StatusOK || !strings.Contains(byUser.Body.String(), "inj-bob-delete") {
		t.Fatalf("Username filter: status=%d body=%q", byUser.Code, byUser.Body.String())
	}
	byIP := lookup("SourceIPAddress", "198.51.100.10")
	if byIP.Code != http.StatusOK || !strings.Contains(byIP.Body.String(), "inj-alice-login") {
		t.Fatalf("SourceIPAddress filter: status=%d body=%q", byIP.Code, byIP.Body.String())
	}

	start := time.Date(2026, 7, 20, 15, 30, 0, 0, time.UTC)
	byTime := mustCloudTrailJSON(t, handler, map[string]any{
		"StartTime":  start.Format(time.RFC3339),
		"MaxResults": 10,
	}, now)
	if byTime.Code != http.StatusOK {
		t.Fatalf("time filter status=%d body=%q", byTime.Code, byTime.Body.String())
	}
	if !strings.Contains(byTime.Body.String(), "inj-bob-delete") {
		t.Fatalf("time filter missing bob: %s", byTime.Body.String())
	}
	if strings.Contains(byTime.Body.String(), "inj-alice-login") {
		t.Fatalf("time filter should exclude alice: %s", byTime.Body.String())
	}
}

func TestCloudTrailInjectTrailDeliveryIncludesInjected(t *testing.T) {
	srv, st, auditDir := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.CloudTrailInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	dataRoot := filepath.Dir(auditDir)

	const bucket = "ct-inject-delivery"
	const trailName = "lab-inject-trail"
	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCloudTrailTrail(testAccountID, store.CloudTrailTrail{
		Name:         trailName,
		S3BucketName: bucket,
		HomeRegion:   store.DefaultCloudTrailRegion,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.StartCloudTrailLogging(testAccountID, trailName, dataRoot); err != nil {
		t.Fatal(err)
	}

	rec := mustCloudTrailInject(t, handler, map[string]any{
		"Event": map[string]any{
			"eventName":       "AssumeRole",
			"eventSource":     "sts.amazonaws.com",
			"eventTime":       "2026-07-20T17:00:00Z",
			"sourceIPAddress": "192.0.2.9",
			"userIdentity":    map[string]any{"userName": "forensic", "type": "IAMUser"},
			"eventID":         "inj-forensic-1",
		},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("inject status=%d body=%q", rec.Code, rec.Body.String())
	}

	listed, err := st.ListObjectsV2(testAccountID, bucket, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Contents) < 2 {
		t.Fatalf("objects=%d want >=2 (snapshot + continuous) keys=%v", len(listed.Contents), listed.Contents)
	}
	found := false
	for _, obj := range listed.Contents {
		_, body, err := st.GetObject(testAccountID, bucket, obj.Key)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "inj-forensic-1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("injected event not found in trail S3 delivery objects")
	}
}

func TestCloudTrailInjectCategoryAndSessionContext(t *testing.T) {
	srv, _, auditDir := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.CloudTrailInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustCloudTrailInject(t, handler, map[string]any{
		"Event": map[string]any{
			"eventName":       "GetObject",
			"eventSource":     "s3.amazonaws.com",
			"eventTime":       "2026-07-20T18:00:00Z",
			"eventCategory":   "Data",
			"managementEvent": false,
			"userIdentity": map[string]any{
				"type":     "AssumedRole",
				"userName": "lab",
			},
			"sessionContext": map[string]any{
				"sessionIssuer": map[string]any{
					"type": "Role",
					"arn":  "arn:aws:iam::000000000001:role/Lab",
				},
			},
			"eventID": "inj-data-event",
		},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("inject status=%d body=%q", rec.Code, rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var line map[string]any
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(l, "inj-data-event") {
			if err := json.Unmarshal([]byte(l), &line); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if line == nil {
		t.Fatal("missing injected line")
	}
	if cat, _ := line["eventCategory"].(string); cat != "Data" {
		t.Fatalf("eventCategory=%v want Data", line["eventCategory"])
	}
	if mgmt, ok := line["managementEvent"].(bool); !ok || mgmt {
		t.Fatalf("managementEvent=%v want false", line["managementEvent"])
	}
	ui, _ := line["userIdentity"].(map[string]any)
	if ui == nil {
		t.Fatal("missing userIdentity")
	}
	sc, _ := ui["sessionContext"].(map[string]any)
	if sc == nil {
		t.Fatalf("userIdentity missing sessionContext: %v", ui)
	}
}

func TestCloudTrailInjectRedactsSecretFields(t *testing.T) {
	srv, _, auditDir := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.CloudTrailInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	const secretVal = "super-secret-value-do-not-persist"
	const passwordVal = "lab-password-should-not-land"
	rec := mustCloudTrailInject(t, handler, map[string]any{
		"Event": map[string]any{
			"eventName":   "GetSecretValue",
			"eventSource": "secretsmanager.amazonaws.com",
			"eventTime":   "2026-07-20T19:00:00Z",
			"eventID":     "inj-redact-secret",
			"requestParameters": map[string]any{
				"secretId":     "app/db",
				"SecretString": secretVal,
				"nested": map[string]any{
					"Password": passwordVal,
					"userName": "alice",
				},
			},
			"responseElements": map[string]any{
				"SecretAccessKey": "AKIAEXAMPLESECRET",
				"accessKeyId":     "AKIAEXAMPLE",
			},
		},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("inject status=%d body=%q", rec.Code, rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "inj-redact-secret") {
		t.Fatalf("missing injected line: %s", text)
	}
	if strings.Contains(text, secretVal) || strings.Contains(text, passwordVal) || strings.Contains(text, "AKIAEXAMPLESECRET") {
		t.Fatalf("events.jsonl leaked secret material: %s", text)
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] markers in %s", text)
	}
	if !strings.Contains(text, "app/db") || !strings.Contains(text, "alice") || !strings.Contains(text, "AKIAEXAMPLE") {
		t.Fatalf("expected non-secret fields retained: %s", text)
	}
}
