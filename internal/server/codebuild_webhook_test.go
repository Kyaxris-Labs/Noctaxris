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

func TestMatchCodeBuildWebhookFilters(t *testing.T) {
	t.Parallel()

	pushMainSrc := store.CodeBuildWebhookEvent{
		Event:     "PUSH",
		HeadRef:   "refs/heads/main",
		FilePaths: []string{"src/app.go", "README.md"},
	}
	prFeature := store.CodeBuildWebhookEvent{
		Event:     "PULL_REQUEST_CREATED",
		HeadRef:   "refs/heads/feature/x",
		FilePaths: []string{"docs/note.md"},
	}

	cases := []struct {
		name    string
		filters string
		ev      store.CodeBuildWebhookEvent
		want    bool
	}{
		{
			name:    "empty groups match all",
			filters: `[]`,
			ev:      pushMainSrc,
			want:    true,
		},
		{
			name: "event match",
			filters: `[
			  [{"type":"EVENT","pattern":"PUSH"}]
			]`,
			ev:   pushMainSrc,
			want: true,
		},
		{
			name: "event mismatch",
			filters: `[
			  [{"type":"EVENT","pattern":"PUSH"}]
			]`,
			ev:   prFeature,
			want: false,
		},
		{
			name: "head_ref match",
			filters: `[
			  [
			    {"type":"EVENT","pattern":"PUSH"},
			    {"type":"HEAD_REF","pattern":"^refs/heads/main$"}
			  ]
			]`,
			ev:   pushMainSrc,
			want: true,
		},
		{
			name: "head_ref mismatch",
			filters: `[
			  [
			    {"type":"EVENT","pattern":"PUSH"},
			    {"type":"HEAD_REF","pattern":"^refs/heads/main$"}
			  ]
			]`,
			ev: store.CodeBuildWebhookEvent{
				Event:   "PUSH",
				HeadRef: "refs/heads/dev",
			},
			want: false,
		},
		{
			name: "file_path match",
			filters: `[
			  [
			    {"type":"EVENT","pattern":"PUSH"},
			    {"type":"FILE_PATH","pattern":"^src/"}
			  ]
			]`,
			ev:   pushMainSrc,
			want: true,
		},
		{
			name: "file_path mismatch",
			filters: `[
			  [
			    {"type":"EVENT","pattern":"PUSH"},
			    {"type":"FILE_PATH","pattern":"^src/"}
			  ]
			]`,
			ev: store.CodeBuildWebhookEvent{
				Event:     "PUSH",
				HeadRef:   "refs/heads/main",
				FilePaths: []string{"docs/only.md"},
			},
			want: false,
		},
		{
			name: "or across groups",
			filters: `[
			  [{"type":"EVENT","pattern":"PUSH"},{"type":"HEAD_REF","pattern":"^refs/heads/main$"}],
			  [{"type":"EVENT","pattern":"PULL_REQUEST_CREATED"}]
			]`,
			ev:   prFeature,
			want: true,
		},
		{
			name: "excludeMatchedPattern inverts file_path",
			filters: `[
			  [
			    {"type":"EVENT","pattern":"PUSH"},
			    {"type":"FILE_PATH","pattern":"^docs/","excludeMatchedPattern":true}
			  ]
			]`,
			ev:   pushMainSrc,
			want: true,
		},
		{
			name: "excludeMatchedPattern rejects docs-only push",
			filters: `[
			  [
			    {"type":"EVENT","pattern":"PUSH"},
			    {"type":"FILE_PATH","pattern":"^docs/","excludeMatchedPattern":true}
			  ]
			]`,
			ev: store.CodeBuildWebhookEvent{
				Event:     "PUSH",
				HeadRef:   "refs/heads/main",
				FilePaths: []string{"docs/only.md"},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := store.MatchCodeBuildWebhookFilters(tc.filters, tc.ev)
			if err != nil {
				t.Fatalf("MatchCodeBuildWebhookFilters: %v", err)
			}
			if got != tc.want {
				t.Fatalf("matched=%v want %v", got, tc.want)
			}
		})
	}
}

func TestLabCodeBuildWebhookHTTPWithoutDocker(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-wh", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-wh"

	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "wh-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo hi"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}

	filters := `[
	  [
	    {"type":"EVENT","pattern":"PUSH"},
	    {"type":"HEAD_REF","pattern":"^refs/heads/main$"},
	    {"type":"FILE_PATH","pattern":"^src/"}
	  ]
	]`
	if _, err := st.UpsertCodeBuildWebhook(testAccountID, store.CodeBuildWebhook{
		ProjectName:      "wh-proj",
		FilterGroupsJSON: filters,
		Secret:           "lab-secret",
	}); err != nil {
		t.Fatal(err)
	}

	path := store.LabCodeBuildWebhookPathPrefix + "/" + testAccountID + "/wh-proj"

	// Missing secret -> 401
	{
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"event":"PUSH","head_ref":"refs/heads/main","file_paths":["src/a.go"]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("missing secret status=%d body=%q", rec.Code, rec.Body.String())
		}
	}

	// Filter mismatch -> 200 triggered:false (no StartBuild)
	{
		body := `{"event":"PUSH","headRef":"refs/heads/dev","filePaths":["src/a.go"]}`
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(server.LabCodeBuildWebhookSecretHeader, "lab-secret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("mismatch status=%d body=%q", rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out["triggered"] != false {
			t.Fatalf("mismatch response=%v", out)
		}
	}

	// Filter match without DockerHost -> 503 (same as StartBuild), no build row
	{
		body := `{"event":"PUSH","head_ref":"refs/heads/main","file_paths":["src/a.go"]}`
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(server.LabCodeBuildWebhookSecretHeader, "lab-secret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("match status=%d want 503 body=%q", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "compute unavailable") {
			t.Fatalf("body=%q", rec.Body.String())
		}
		ids, err := st.ListCodeBuildBuilds(testAccountID, "wh-proj")
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != 0 {
			t.Fatalf("expected no builds without DockerHost, got %v", ids)
		}
	}

	// Unknown webhook path -> 404
	{
		req := httptest.NewRequest(http.MethodPost, store.LabCodeBuildWebhookPathPrefix+"/"+testAccountID+"/missing", strings.NewReader(`{"event":"PUSH"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("missing webhook status=%d body=%q", rec.Code, rec.Body.String())
		}
	}
}
