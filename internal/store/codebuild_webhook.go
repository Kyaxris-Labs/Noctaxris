package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// LabCodeBuildWebhookPathPrefix is the unauthenticated lab webhook HTTP prefix.
// Full path: /_noctaxris/codebuild/webhook/{account}/{project}
const LabCodeBuildWebhookPathPrefix = "/_noctaxris/codebuild/webhook"

var (
	ErrCodeBuildWebhookNotFound = errors.New("ResourceNotFoundException")
)

// CodeBuildWebhookFilter is one filter inside a filter group (lab subset).
type CodeBuildWebhookFilter struct {
	Type                  string `json:"type"`
	Pattern               string `json:"pattern"`
	ExcludeMatchedPattern bool   `json:"excludeMatchedPattern,omitempty"`
}

// CodeBuildWebhookEvent is a lab push/PR simulation payload used for filter matching.
type CodeBuildWebhookEvent struct {
	Event     string   // PUSH, PULL_REQUEST_CREATED, ...
	HeadRef   string   // e.g. refs/heads/main
	FilePaths []string // changed paths for FILE_PATH filters
}

// CodeBuildWebhook is a lab webhook row for a project.
type CodeBuildWebhook struct {
	AccountID        string
	ProjectName      string
	PayloadJSON      string // optional opaque metadata JSON
	FilterGroupsJSON string // JSON [][]CodeBuildWebhookFilter
	Secret           string // optional shared secret; empty disables check
}

const codebuildWebhookSchema = `
CREATE TABLE IF NOT EXISTS codebuild_webhooks (
  account_id TEXT NOT NULL,
  project_name TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  filter_groups_json TEXT NOT NULL DEFAULT '[]',
  secret TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, project_name)
);
`

// EnsureCodeBuildWebhookSchema creates the codebuild_webhooks table if missing.
func EnsureCodeBuildWebhookSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure codebuild webhook schema: db is nil")
	}
	if _, err := db.Exec(codebuildWebhookSchema); err != nil {
		return fmt.Errorf("ensure codebuild webhook schema: %w", err)
	}
	return nil
}

func (s *Store) ensureCodeBuildWebhookSchema() error {
	return EnsureCodeBuildWebhookSchema(s.db)
}

// UpsertCodeBuildWebhook inserts or replaces a lab webhook for a project.
func (s *Store) UpsertCodeBuildWebhook(accountID string, wh CodeBuildWebhook) (CodeBuildWebhook, error) {
	if err := s.ensureCodeBuildWebhookSchema(); err != nil {
		return CodeBuildWebhook{}, err
	}
	project := strings.TrimSpace(wh.ProjectName)
	if strings.TrimSpace(accountID) == "" || project == "" {
		return CodeBuildWebhook{}, fmt.Errorf("%w: account_id and project_name are required", ErrCodeBuildInvalidInput)
	}
	if _, err := s.GetCodeBuildProject(accountID, project); err != nil {
		return CodeBuildWebhook{}, err
	}
	payload := strings.TrimSpace(wh.PayloadJSON)
	if payload == "" {
		payload = "{}"
	}
	filters := strings.TrimSpace(wh.FilterGroupsJSON)
	if filters == "" {
		filters = "[]"
	}
	if err := validateCodeBuildFilterGroupsJSON(filters); err != nil {
		return CodeBuildWebhook{}, err
	}
	_, err := s.db.Exec(
		`INSERT INTO codebuild_webhooks (account_id, project_name, payload_json, filter_groups_json, secret)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, project_name) DO UPDATE SET
		   payload_json = excluded.payload_json,
		   filter_groups_json = excluded.filter_groups_json,
		   secret = excluded.secret`,
		accountID, project, payload, filters, wh.Secret,
	)
	if err != nil {
		return CodeBuildWebhook{}, fmt.Errorf("upsert codebuild webhook: %w", err)
	}
	return CodeBuildWebhook{
		AccountID:        accountID,
		ProjectName:      project,
		PayloadJSON:      payload,
		FilterGroupsJSON: filters,
		Secret:           wh.Secret,
	}, nil
}

// GetCodeBuildWebhook returns the lab webhook for a project.
func (s *Store) GetCodeBuildWebhook(accountID, projectName string) (CodeBuildWebhook, error) {
	if err := s.ensureCodeBuildWebhookSchema(); err != nil {
		return CodeBuildWebhook{}, err
	}
	projectName = strings.TrimSpace(projectName)
	var wh CodeBuildWebhook
	err := s.db.QueryRow(
		`SELECT account_id, project_name, payload_json, filter_groups_json, secret
		 FROM codebuild_webhooks WHERE account_id = ? AND project_name = ?`,
		accountID, projectName,
	).Scan(&wh.AccountID, &wh.ProjectName, &wh.PayloadJSON, &wh.FilterGroupsJSON, &wh.Secret)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeBuildWebhook{}, ErrCodeBuildWebhookNotFound
	}
	if err != nil {
		return CodeBuildWebhook{}, fmt.Errorf("get codebuild webhook: %w", err)
	}
	return wh, nil
}

// DeleteCodeBuildWebhook removes a lab webhook.
func (s *Store) DeleteCodeBuildWebhook(accountID, projectName string) error {
	if err := s.ensureCodeBuildWebhookSchema(); err != nil {
		return err
	}
	projectName = strings.TrimSpace(projectName)
	res, err := s.db.Exec(
		`DELETE FROM codebuild_webhooks WHERE account_id = ? AND project_name = ?`,
		accountID, projectName,
	)
	if err != nil {
		return fmt.Errorf("delete codebuild webhook: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete codebuild webhook rows: %w", err)
	}
	if n == 0 {
		return ErrCodeBuildWebhookNotFound
	}
	return nil
}

// ListCodeBuildWebhooks returns webhooks for an account, name-ordered.
func (s *Store) ListCodeBuildWebhooks(accountID string) ([]CodeBuildWebhook, error) {
	if err := s.ensureCodeBuildWebhookSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT account_id, project_name, payload_json, filter_groups_json, secret
		 FROM codebuild_webhooks WHERE account_id = ? ORDER BY project_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list codebuild webhooks: %w", err)
	}
	defer rows.Close()
	var out []CodeBuildWebhook
	for rows.Next() {
		var wh CodeBuildWebhook
		if err := rows.Scan(&wh.AccountID, &wh.ProjectName, &wh.PayloadJSON, &wh.FilterGroupsJSON, &wh.Secret); err != nil {
			return nil, fmt.Errorf("list codebuild webhooks scan: %w", err)
		}
		out = append(out, wh)
	}
	return out, rows.Err()
}

func validateCodeBuildFilterGroupsJSON(raw string) error {
	var groups [][]CodeBuildWebhookFilter
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return fmt.Errorf("%w: filterGroups must be JSON array of filter groups: %v", ErrCodeBuildInvalidInput, err)
	}
	for gi, group := range groups {
		for fi, f := range group {
			typ := strings.ToUpper(strings.TrimSpace(f.Type))
			switch typ {
			case "EVENT", "HEAD_REF", "FILE_PATH":
			default:
				return fmt.Errorf("%w: filterGroups[%d][%d].type must be EVENT, HEAD_REF, or FILE_PATH", ErrCodeBuildInvalidInput, gi, fi)
			}
			if strings.TrimSpace(f.Pattern) == "" {
				return fmt.Errorf("%w: filterGroups[%d][%d].pattern is required", ErrCodeBuildInvalidInput, gi, fi)
			}
			if typ == "HEAD_REF" || typ == "FILE_PATH" {
				if _, err := regexp.Compile(f.Pattern); err != nil {
					return fmt.Errorf("%w: filterGroups[%d][%d].pattern invalid regex: %v", ErrCodeBuildInvalidInput, gi, fi, err)
				}
			}
		}
	}
	return nil
}

// MatchCodeBuildWebhookFilters evaluates lab filterGroups (OR across groups, AND within).
// Empty or missing groups match all events. Supported filter types: EVENT, HEAD_REF, FILE_PATH.
func MatchCodeBuildWebhookFilters(filterGroupsJSON string, ev CodeBuildWebhookEvent) (bool, error) {
	raw := strings.TrimSpace(filterGroupsJSON)
	if raw == "" || raw == "null" {
		return true, nil
	}
	var groups [][]CodeBuildWebhookFilter
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return false, fmt.Errorf("parse filterGroups: %w", err)
	}
	if len(groups) == 0 {
		return true, nil
	}
	for _, group := range groups {
		ok, err := matchCodeBuildWebhookFilterGroup(group, ev)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func matchCodeBuildWebhookFilterGroup(group []CodeBuildWebhookFilter, ev CodeBuildWebhookEvent) (bool, error) {
	if len(group) == 0 {
		return true, nil
	}
	for _, f := range group {
		matched, err := matchCodeBuildWebhookFilter(f, ev)
		if err != nil {
			return false, err
		}
		if f.ExcludeMatchedPattern {
			matched = !matched
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func matchCodeBuildWebhookFilter(f CodeBuildWebhookFilter, ev CodeBuildWebhookEvent) (bool, error) {
	typ := strings.ToUpper(strings.TrimSpace(f.Type))
	pattern := strings.TrimSpace(f.Pattern)
	switch typ {
	case "EVENT":
		want := strings.ToUpper(strings.TrimSpace(ev.Event))
		for _, part := range strings.Split(pattern, ",") {
			if strings.ToUpper(strings.TrimSpace(part)) == want {
				return true, nil
			}
		}
		return false, nil
	case "HEAD_REF":
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("HEAD_REF pattern: %w", err)
		}
		return re.MatchString(ev.HeadRef), nil
	case "FILE_PATH":
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("FILE_PATH pattern: %w", err)
		}
		for _, p := range ev.FilePaths {
			if re.MatchString(p) {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, fmt.Errorf("unsupported filter type %q", f.Type)
	}
}
