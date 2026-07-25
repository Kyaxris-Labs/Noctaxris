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

func mustLabForensicsJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "noctaxris", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestLabForensicsDisabledRejects(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustLabForensicsJSON(t, handler, "NoctaxrisLab.FreezeClock", map[string]any{}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q want 403", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NOCTAXRIS_LAB_FORENSICS") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestLabForensicsSetClockAndBulkSeed(t *testing.T) {
	srv, st, auditDir := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabForensics = true
	})
	handler := srv.Handler()
	signNow := time.Now().UTC().Truncate(time.Second)
	fixed := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	rec := mustLabForensicsJSON(t, handler, "NoctaxrisLab.SetClock", map[string]any{
		"FixedTime": fixed.Format(time.RFC3339),
	}, signNow)
	if rec.Code != http.StatusOK {
		t.Fatalf("SetClock status=%d body=%q", rec.Code, rec.Body.String())
	}
	var setResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &setResp); err != nil {
		t.Fatal(err)
	}
	if setResp["ClockTime"] != fixed.Format(time.RFC3339) {
		t.Fatalf("ClockTime=%v want %s", setResp["ClockTime"], fixed.Format(time.RFC3339))
	}

	rec = mustLabForensicsJSON(t, handler, "NoctaxrisLab.BulkSeed", map[string]any{
		"ScenarioId":       "crypto-mining",
		"IncludeGuardDuty": true,
	}, signNow)
	if rec.Code != http.StatusOK {
		t.Fatalf("BulkSeed status=%d body=%q", rec.Code, rec.Body.String())
	}
	var seedResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &seedResp); err != nil {
		t.Fatal(err)
	}
	if seedResp["ScenarioId"] != "crypto-mining" {
		t.Fatalf("ScenarioId=%v", seedResp["ScenarioId"])
	}
	ctCount, _ := seedResp["CloudTrailEventCount"].(float64)
	if ctCount < 1 {
		t.Fatalf("CloudTrailEventCount=%v", seedResp["CloudTrailEventCount"])
	}
	gdCount, _ := seedResp["GuardDutyFindingCount"].(float64)
	if gdCount < 1 {
		t.Fatalf("GuardDutyFindingCount=%v", seedResp["GuardDutyFindingCount"])
	}

	eventsPath := filepath.Join(auditDir, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "RunInstances") {
		t.Fatalf("audit jsonl missing RunInstances: %s", string(data))
	}

	detectorID, err := st.EnsureGuardDutyDetector(testAccountID)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := st.ListGuardDutyFindingIDs(testAccountID, detectorID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("expected GuardDuty findings after BulkSeed")
	}
	got, err := st.GetGuardDutyFindings(testAccountID, detectorID, ids)
	if err != nil {
		t.Fatal(err)
	}
	foundCrypto := false
	for _, f := range got {
		if strings.Contains(f.Type, "CryptoCurrency") {
			foundCrypto = true
			break
		}
	}
	if !foundCrypto {
		t.Fatalf("findings=%+v want CryptoCurrency type", got)
	}
}

func TestLabForensicsFreezeClock(t *testing.T) {
	srv, _, auditDir := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabForensics = true
	})
	handler := srv.Handler()
	signNow := time.Now().UTC().Truncate(time.Second)
	fixed := time.Date(2026, 3, 1, 8, 30, 0, 0, time.UTC)

	rec := mustLabForensicsJSON(t, handler, "NoctaxrisLab.SetClock", map[string]any{
		"FixedTime": fixed.Format(time.RFC3339),
	}, signNow)
	if rec.Code != http.StatusOK {
		t.Fatalf("SetClock: %d %s", rec.Code, rec.Body.String())
	}

	rec = mustLabForensicsJSON(t, handler, "NoctaxrisLab.FreezeClock", map[string]any{}, signNow)
	if rec.Code != http.StatusOK {
		t.Fatalf("FreezeClock: %d %s", rec.Code, rec.Body.String())
	}

	rec = mustLabForensicsJSON(t, handler, "NoctaxrisLab.BulkSeed", map[string]any{
		"ScenarioId":       "suspicious-login",
		"IncludeGuardDuty": false,
	}, signNow)
	if rec.Code != http.StatusOK {
		t.Fatalf("BulkSeed: %d %s", rec.Code, rec.Body.String())
	}

	eventsPath := filepath.Join(auditDir, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ConsoleLogin") {
		t.Fatalf("missing ConsoleLogin in %s", string(data))
	}
	if !strings.Contains(string(data), fixed.Add(-2*time.Hour).Format(time.RFC3339)) {
		t.Fatalf("expected eventTime relative to frozen clock in %s", string(data))
	}

	rec = mustLabForensicsJSON(t, handler, "NoctaxrisLab.UnfreezeClock", map[string]any{}, signNow)
	if rec.Code != http.StatusOK {
		t.Fatalf("UnfreezeClock: %d %s", rec.Code, rec.Body.String())
	}
}
