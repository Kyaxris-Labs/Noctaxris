package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/server"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCodeBuildCreateWebhookRequiresSecretAndOmitsFromJSON(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-wh-role", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-wh-role"
	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "wh-secret-proj",
		"serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo ok"]}}}`},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20", "computeType": "BUILD_GENERAL1_SMALL"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}

	missing := mustCodeBuildJSON(t, handler, "CreateWebhook", map[string]any{
		"projectName": "wh-secret-proj",
	}, now)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "secret is required") {
		t.Fatalf("missing secret status=%d body=%q", missing.Code, missing.Body.String())
	}

	created := mustCodeBuildJSON(t, handler, "CreateWebhook", map[string]any{
		"projectName": "wh-secret-proj",
		"secret":      "caller-known-secret",
		"filterGroups": []any{
			[]any{map[string]any{"type": "EVENT", "pattern": "PUSH"}},
		},
	}, now)
	if created.Code != http.StatusOK {
		t.Fatalf("CreateWebhook status=%d body=%q", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "caller-known-secret") || strings.Contains(created.Body.String(), `"secret"`) {
		t.Fatalf("CreateWebhook must not echo secret body=%q", created.Body.String())
	}

	listed := mustCodeBuildJSON(t, handler, "ListWebhooks", map[string]any{}, now)
	if listed.Code != http.StatusOK {
		t.Fatalf("ListWebhooks status=%d body=%q", listed.Code, listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), "caller-known-secret") || strings.Contains(listed.Body.String(), `"secret"`) {
		t.Fatalf("ListWebhooks must not echo secret body=%q", listed.Body.String())
	}

	got, err := st.GetCodeBuildWebhook(testAccountID, "wh-secret-proj")
	if err != nil || got.Secret != "caller-known-secret" {
		t.Fatalf("stored secret=%q err=%v", got.Secret, err)
	}
}

func TestCodeBuildWebhookReceiveRejectsMissingSecret(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-wh-recv", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-wh-recv"
	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "wh-recv-proj",
		"serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo ok"]}}}`},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20", "computeType": "BUILD_GENERAL1_SMALL"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}
	if _, err := st.UpsertCodeBuildWebhook(testAccountID, store.CodeBuildWebhook{
		ProjectName: "wh-recv-proj",
		Secret:      "lab-secret",
	}); err != nil {
		t.Fatal(err)
	}

	path := store.LabCodeBuildWebhookPathPrefix + "/" + testAccountID + "/wh-recv-proj"
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"event":"PUSH"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing header status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"event":"PUSH"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(server.LabCodeBuildWebhookSecretHeader, "wrong")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"event":"PUSH","headRef":"refs/heads/main"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(server.LabCodeBuildWebhookSecretHeader, "lab-secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("matched without DockerHost status=%d want 503 body=%q", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["triggered"] != true {
		t.Fatalf("response=%v", out)
	}
	_ = now
}
