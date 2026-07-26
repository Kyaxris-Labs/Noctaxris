package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	cbsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/codebuild"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// LabCodeBuildWebhookSecretHeader is the optional shared-secret header for lab webhooks.
const LabCodeBuildWebhookSecretHeader = "X-Noctaxris-Webhook-Secret"

func isLabCodeBuildWebhookPath(path string) bool {
	p := store.LabCodeBuildWebhookPathPrefix
	return path == p || strings.HasPrefix(path, p+"/")
}

// parseLabCodeBuildWebhookPath extracts account and project from
// /_noctaxris/codebuild/webhook/{account}/{project}.
func parseLabCodeBuildWebhookPath(path string) (accountID, projectName string, ok bool) {
	prefix := store.LabCodeBuildWebhookPathPrefix + "/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	accountID = strings.TrimSpace(parts[0])
	projectName = strings.TrimSpace(parts[1])
	if accountID == "" || projectName == "" {
		return "", "", false
	}
	return accountID, projectName, true
}

type labCodeBuildWebhookBody struct {
	Event        string   `json:"event"`
	HeadRef      string   `json:"headRef"`
	HeadRefAlt   string   `json:"head_ref"`
	FilePaths    []string `json:"filePaths"`
	FilePathsAlt []string `json:"file_paths"`
}

func (b labCodeBuildWebhookBody) toEvent() store.CodeBuildWebhookEvent {
	head := strings.TrimSpace(b.HeadRef)
	if head == "" {
		head = strings.TrimSpace(b.HeadRefAlt)
	}
	paths := b.FilePaths
	if len(paths) == 0 {
		paths = b.FilePathsAlt
	}
	return store.CodeBuildWebhookEvent{
		Event:     strings.TrimSpace(b.Event),
		HeadRef:   head,
		FilePaths: paths,
	}
}

func (s *Server) handleLabCodeBuildWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	accountID, projectName, ok := parseLabCodeBuildWebhookPath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	wh, err := s.store.GetCodeBuildWebhook(accountID, projectName)
	if errors.Is(err, store.ErrCodeBuildWebhookNotFound) {
		http.Error(w, "webhook not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if secret := strings.TrimSpace(wh.Secret); secret != "" {
		got := strings.TrimSpace(r.Header.Get(LabCodeBuildWebhookSecretHeader))
		if got == "" || got != secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "unable to read body", http.StatusBadRequest)
		return
	}
	var payload labCodeBuildWebhookBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
	}
	ev := payload.toEvent()
	if ev.Event == "" {
		http.Error(w, "event is required", http.StatusBadRequest)
		return
	}

	matched, err := store.MatchCodeBuildWebhookFilters(wh.FilterGroupsJSON, ev)
	if err != nil {
		http.Error(w, "invalid filterGroups", http.StatusInternalServerError)
		return
	}
	if !matched {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"triggered":false}`))
		return
	}

	// Match StartBuild: without DockerHost return 503 and do not create a build row.
	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"triggered":true,"error":"compute unavailable"}`))
		return
	}

	region := store.DefaultCodeBuildRegion
	b, err := s.store.StartCodeBuildBuild(accountID, region, store.StartCodeBuildBuildOpts{
		ProjectName: projectName,
	})
	if errors.Is(err, store.ErrCodeBuildProjectNotFound) {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "unable to start build", http.StatusInternalServerError)
		return
	}

	if err := s.startCodeBuildContainer(r.Context(), accountID, b); err != nil {
		_ = s.store.SetCodeBuildBuildRuntime(accountID, b.ID, "", store.CodeBuildStatusFailed, time.Now().UTC().Format(time.RFC3339))
		if strings.Contains(err.Error(), "compute unavailable") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"triggered":true,"error":"compute unavailable"}`))
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	builds, err := s.store.BatchGetCodeBuildBuilds(accountID, []string{b.ID})
	if err != nil || len(builds) == 0 {
		http.Error(w, "unable to load build", http.StatusInternalServerError)
		return
	}
	startPayload, err := cbsvc.StartBuildJSON(builds[0])
	if err != nil {
		http.Error(w, "unable to build response", http.StatusInternalServerError)
		return
	}
	var startObj map[string]any
	if err := json.Unmarshal(startPayload, &startObj); err != nil {
		http.Error(w, "unable to build response", http.StatusInternalServerError)
		return
	}
	out := map[string]any{
		"triggered": true,
		"build":     startObj["build"],
	}
	raw, err := json.Marshal(out)
	if err != nil {
		http.Error(w, "unable to build response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}
