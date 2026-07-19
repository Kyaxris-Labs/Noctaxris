package server_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase 0 security regressions: unauthenticated AWS-shaped calls fail closed,
// health stays open for container checks, and secrets never appear in audit.

func TestUnauthenticatedAWSPathReturns403(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Amz-Target", "AWSSecurityTokenServiceV20110615.GetCallerIdentity")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "MissingAuthenticationToken") {
		t.Fatalf("expected MissingAuthenticationToken in body %q", body)
	}
}

func TestUnauthenticatedAWSPathWritesAuditErrorCode(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Amz-Target", "AWSSecurityTokenServiceV20110615.GetCallerIdentity")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
	}

	eventsPath := filepath.Join(auditDir, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected audit event")
	}
	auditText := string(data)
	if !strings.Contains(auditText, `"errorCode":"MissingAuthenticationToken"`) {
		t.Fatalf("expected audit errorCode MissingAuthenticationToken in %q", auditText)
	}
	if strings.Contains(auditText, testSecret) {
		t.Fatal("audit must not contain secret values")
	}
}

func TestHealthWorksWithoutAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/_noctaxris/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Fatalf("body=%q want ok", body)
	}
}
