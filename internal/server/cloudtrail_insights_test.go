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
)

func mustCloudTrailInsightsInject(t *testing.T, handler http.Handler, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "NoctaxrisCloudTrail.InjectInsightsEvents")
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "cloudtrail", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCloudTrailInjectInsightsAndLookupCategory(t *testing.T) {
	srv, _, auditDir := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.CloudTrailInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	// Seed a normal management event via InjectEvents.
	mgmt := mustCloudTrailInject(t, handler, map[string]any{
		"Event": map[string]any{
			"eventName":   "GetCallerIdentity",
			"eventSource": "sts.amazonaws.com",
			"eventTime":   "2026-07-20T12:00:00Z",
			"eventID":     "mgmt-1",
		},
	}, now)
	if mgmt.Code != http.StatusOK {
		t.Fatalf("mgmt inject status=%d body=%q", mgmt.Code, mgmt.Body.String())
	}

	rec := mustCloudTrailInsightsInject(t, handler, map[string]any{
		"Event": map[string]any{
			"eventTime": "2026-07-20T12:05:00Z",
			"eventID":   "insight-1",
			"insightDetails": map[string]any{
				"state":                "Start",
				"eventSource":          "sts.amazonaws.com",
				"eventName":            "AssumeRole",
				"insightType":          "ApiCallRateInsight",
				"sourceEventCategory":  "Management",
				"insightContext": map[string]any{
					"statistics": map[string]any{
						"baseline":        map[string]any{"average": 0.1},
						"insight":         map[string]any{"average": 12.0},
						"insightDuration": 5,
						"baselineDuration": 1000,
					},
				},
			},
		},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("insight inject status=%d body=%q", rec.Code, rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"eventCategory":"Insight"`) {
		t.Fatalf("missing insight line in %s", data)
	}

	lookupDefault := mustJSONTarget(t, handler, "CloudTrail_20131101.LookupEvents", "cloudtrail", map[string]any{
		"MaxResults": 50,
	}, now)
	if lookupDefault.Code != http.StatusOK {
		t.Fatalf("lookup default status=%d body=%q", lookupDefault.Code, lookupDefault.Body.String())
	}
	if strings.Contains(lookupDefault.Body.String(), "insight-1") {
		t.Fatalf("default LookupEvents should exclude insights: %s", lookupDefault.Body.String())
	}
	if !strings.Contains(lookupDefault.Body.String(), "mgmt-1") {
		t.Fatalf("default LookupEvents missing mgmt: %s", lookupDefault.Body.String())
	}

	lookupInsight := mustJSONTarget(t, handler, "CloudTrail_20131101.LookupEvents", "cloudtrail", map[string]any{
		"EventCategory": "insight",
		"MaxResults":    50,
	}, now)
	if lookupInsight.Code != http.StatusOK {
		t.Fatalf("lookup insight status=%d body=%q", lookupInsight.Code, lookupInsight.Body.String())
	}
	if !strings.Contains(lookupInsight.Body.String(), "insight-1") {
		t.Fatalf("insight LookupEvents missing insight: %s", lookupInsight.Body.String())
	}
	if strings.Contains(lookupInsight.Body.String(), "mgmt-1") {
		t.Fatalf("insight LookupEvents should exclude management: %s", lookupInsight.Body.String())
	}
}

func TestCloudTrailInjectInsightsDisabledRejects(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	rec := mustCloudTrailInsightsInject(t, handler, map[string]any{
		"Event": map[string]any{
			"insightDetails": map[string]any{
				"eventName":   "AssumeRole",
				"eventSource": "sts.amazonaws.com",
			},
		},
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q want 403", rec.Code, rec.Body.String())
	}
}
