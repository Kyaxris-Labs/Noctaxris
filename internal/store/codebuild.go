package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	// DefaultCodeBuildRegion is the lab region embedded in CodeBuild ARNs.
	DefaultCodeBuildRegion = "us-east-1"

	// CodeBuild statuses (subset).
	CodeBuildStatusInProgress = "IN_PROGRESS"
	CodeBuildStatusSucceeded  = "SUCCEEDED"
	CodeBuildStatusFailed     = "FAILED"
)

var (
	ErrCodeBuildProjectExists   = errors.New("ResourceAlreadyExistsException")
	ErrCodeBuildProjectNotFound = errors.New("ResourceNotFoundException")
	ErrCodeBuildBuildNotFound   = errors.New("ResourceNotFoundException")
	ErrCodeBuildInvalidInput    = errors.New("InvalidInputException")
)

// CodeBuildProject is a CreateProject row.
type CodeBuildProject struct {
	Name        string
	ARN         string
	Description string
	ServiceRole string
	SourceType  string
	SourceLoc   string
	Buildspec   string
	Image       string
	Artifacts   string // JSON
	CreatedAt   string
}

// CodeBuildBuild is a StartBuild row.
type CodeBuildBuild struct {
	ID           string
	ARN          string
	ProjectName  string
	ProjectARN   string
	BuildStatus  string
	SourceType   string
	SourceLoc    string
	Buildspec    string
	Image        string
	ContainerID  string
	StartTime    string
	EndTime      string
	PhasesJSON   string
}

// CreateCodeBuildProjectInput holds CreateProject fields.
type CreateCodeBuildProjectInput struct {
	Name        string
	Description string
	ServiceRole string
	SourceType  string
	SourceLoc   string
	Buildspec   string
	Image       string
	Artifacts   map[string]any
}

const codebuildSchema = `
CREATE TABLE IF NOT EXISTS codebuild_projects (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  project_arn TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  service_role TEXT NOT NULL,
  source_type TEXT NOT NULL,
  source_location TEXT NOT NULL DEFAULT '',
  buildspec TEXT NOT NULL DEFAULT '',
  image TEXT NOT NULL,
  artifacts_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS codebuild_builds (
  account_id TEXT NOT NULL,
  build_id TEXT NOT NULL,
  build_arn TEXT NOT NULL,
  project_name TEXT NOT NULL,
  project_arn TEXT NOT NULL,
  build_status TEXT NOT NULL,
  source_type TEXT NOT NULL,
  source_location TEXT NOT NULL DEFAULT '',
  buildspec TEXT NOT NULL DEFAULT '',
  image TEXT NOT NULL,
  container_id TEXT NOT NULL DEFAULT '',
  start_time TEXT NOT NULL,
  end_time TEXT NOT NULL DEFAULT '',
  phases_json TEXT NOT NULL DEFAULT '[]',
  PRIMARY KEY (account_id, build_id)
);
CREATE INDEX IF NOT EXISTS idx_codebuild_builds_project ON codebuild_builds(account_id, project_name);
`

// EnsureCodeBuildSchema creates CodeBuild tables if missing.
func EnsureCodeBuildSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure codebuild schema: db is nil")
	}
	if _, err := db.Exec(codebuildSchema); err != nil {
		return fmt.Errorf("ensure codebuild schema: %w", err)
	}
	return nil
}

// EnsureCodeBuildSchema ensures CodeBuild tables on an open store.
func (s *Store) EnsureCodeBuildSchema() error {
	return EnsureCodeBuildSchema(s.db)
}

// CodeBuildProjectARN builds arn:aws:codebuild:REGION:ACCOUNT:project/NAME.
func CodeBuildProjectARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultCodeBuildRegion
	}
	return fmt.Sprintf("arn:aws:codebuild:%s:%s:project/%s", region, accountID, name)
}

// CodeBuildBuildARN builds arn:aws:codebuild:REGION:ACCOUNT:build/PROJECT:ID.
func CodeBuildBuildARN(region, accountID, projectName, buildID string) string {
	if region == "" {
		region = DefaultCodeBuildRegion
	}
	return fmt.Sprintf("arn:aws:codebuild:%s:%s:build/%s:%s", region, accountID, projectName, buildID)
}

// CreateCodeBuildProject inserts a project.
func (s *Store) CreateCodeBuildProject(accountID, region string, in CreateCodeBuildProjectInput) (CodeBuildProject, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return CodeBuildProject{}, fmt.Errorf("%w: name is required", ErrCodeBuildInvalidInput)
	}
	if strings.TrimSpace(in.ServiceRole) == "" {
		return CodeBuildProject{}, fmt.Errorf("%w: serviceRole is required", ErrCodeBuildInvalidInput)
	}
	sourceType := strings.TrimSpace(in.SourceType)
	if sourceType == "" {
		return CodeBuildProject{}, fmt.Errorf("%w: source.type is required", ErrCodeBuildInvalidInput)
	}
	switch strings.ToUpper(sourceType) {
	case "NO_SOURCE", "S3":
		sourceType = strings.ToUpper(sourceType)
	default:
		return CodeBuildProject{}, fmt.Errorf("%w: source.type must be NO_SOURCE or S3", ErrCodeBuildInvalidInput)
	}
	image := strings.TrimSpace(in.Image)
	if image == "" {
		return CodeBuildProject{}, fmt.Errorf("%w: environment.image is required", ErrCodeBuildInvalidInput)
	}
	buildspec := in.Buildspec
	if sourceType == "NO_SOURCE" && strings.TrimSpace(buildspec) == "" {
		return CodeBuildProject{}, fmt.Errorf("%w: source.buildspec is required for NO_SOURCE", ErrCodeBuildInvalidInput)
	}
	if sourceType == "S3" && strings.TrimSpace(in.SourceLoc) == "" {
		return CodeBuildProject{}, fmt.Errorf("%w: source.location is required for S3", ErrCodeBuildInvalidInput)
	}

	artifacts := in.Artifacts
	if artifacts == nil {
		artifacts = map[string]any{"type": "NO_ARTIFACTS"}
	}
	artJSON, err := json.Marshal(artifacts)
	if err != nil {
		return CodeBuildProject{}, fmt.Errorf("marshal artifacts: %w", err)
	}

	arn := CodeBuildProjectARN(region, accountID, name)
	created := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO codebuild_projects (
			account_id, name, project_arn, description, service_role,
			source_type, source_location, buildspec, image, artifacts_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, arn, strings.TrimSpace(in.Description), strings.TrimSpace(in.ServiceRole),
		sourceType, strings.TrimSpace(in.SourceLoc), buildspec, image, string(artJSON), created,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return CodeBuildProject{}, ErrCodeBuildProjectExists
		}
		return CodeBuildProject{}, fmt.Errorf("create codebuild project: %w", err)
	}
	return CodeBuildProject{
		Name:        name,
		ARN:         arn,
		Description: strings.TrimSpace(in.Description),
		ServiceRole: strings.TrimSpace(in.ServiceRole),
		SourceType:  sourceType,
		SourceLoc:   strings.TrimSpace(in.SourceLoc),
		Buildspec:   buildspec,
		Image:       image,
		Artifacts:   string(artJSON),
		CreatedAt:   created,
	}, nil
}

// GetCodeBuildProject returns a project by name.
func (s *Store) GetCodeBuildProject(accountID, name string) (CodeBuildProject, error) {
	name = strings.TrimSpace(name)
	var p CodeBuildProject
	err := s.db.QueryRow(
		`SELECT name, project_arn, description, service_role, source_type, source_location,
			buildspec, image, artifacts_json, created_at
		 FROM codebuild_projects WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(
		&p.Name, &p.ARN, &p.Description, &p.ServiceRole, &p.SourceType, &p.SourceLoc,
		&p.Buildspec, &p.Image, &p.Artifacts, &p.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeBuildProject{}, ErrCodeBuildProjectNotFound
	}
	if err != nil {
		return CodeBuildProject{}, fmt.Errorf("get codebuild project: %w", err)
	}
	return p, nil
}

// StartCodeBuildBuild creates an IN_PROGRESS build from a project (optional buildspec override).
func (s *Store) StartCodeBuildBuild(accountID, region, projectName, buildspecOverride string) (CodeBuildBuild, error) {
	p, err := s.GetCodeBuildProject(accountID, projectName)
	if err != nil {
		return CodeBuildBuild{}, err
	}
	buildspec := p.Buildspec
	if strings.TrimSpace(buildspecOverride) != "" {
		buildspec = buildspecOverride
	}
	id := uuid.NewString()
	arn := CodeBuildBuildARN(region, accountID, p.Name, id)
	start := nowRFC3339()
	b := CodeBuildBuild{
		ID:          id,
		ARN:         arn,
		ProjectName: p.Name,
		ProjectARN:  p.ARN,
		BuildStatus: CodeBuildStatusInProgress,
		SourceType:  p.SourceType,
		SourceLoc:   p.SourceLoc,
		Buildspec:   buildspec,
		Image:       p.Image,
		StartTime:   start,
		PhasesJSON:  `[]`,
	}
	_, err = s.db.Exec(
		`INSERT INTO codebuild_builds (
			account_id, build_id, build_arn, project_name, project_arn, build_status,
			source_type, source_location, buildspec, image, container_id, start_time, end_time, phases_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, '', '[]')`,
		accountID, b.ID, b.ARN, b.ProjectName, b.ProjectARN, b.BuildStatus,
		b.SourceType, b.SourceLoc, b.Buildspec, b.Image, b.StartTime,
	)
	if err != nil {
		return CodeBuildBuild{}, fmt.Errorf("start codebuild build: %w", err)
	}
	return b, nil
}

// SetCodeBuildBuildRuntime updates container id and optional status.
func (s *Store) SetCodeBuildBuildRuntime(accountID, buildID, containerID, status, endTime string) error {
	_, err := s.db.Exec(
		`UPDATE codebuild_builds SET container_id = ?, build_status = ?, end_time = ?
		 WHERE account_id = ? AND build_id = ?`,
		containerID, status, endTime, accountID, buildID,
	)
	if err != nil {
		return fmt.Errorf("set codebuild build runtime: %w", err)
	}
	return nil
}

// BatchGetCodeBuildBuilds returns builds by id (missing ids omitted).
func (s *Store) BatchGetCodeBuildBuilds(accountID string, ids []string) ([]CodeBuildBuild, error) {
	out := make([]CodeBuildBuild, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		// Accept full build ARN or bare id.
		if i := strings.LastIndex(id, ":"); i >= 0 && strings.Contains(id, ":build/") {
			id = id[i+1:]
		}
		b, err := s.getCodeBuildBuild(accountID, id)
		if errors.Is(err, ErrCodeBuildBuildNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (s *Store) getCodeBuildBuild(accountID, buildID string) (CodeBuildBuild, error) {
	var b CodeBuildBuild
	err := s.db.QueryRow(
		`SELECT build_id, build_arn, project_name, project_arn, build_status, source_type, source_location,
			buildspec, image, container_id, start_time, end_time, phases_json
		 FROM codebuild_builds WHERE account_id = ? AND build_id = ?`,
		accountID, buildID,
	).Scan(
		&b.ID, &b.ARN, &b.ProjectName, &b.ProjectARN, &b.BuildStatus, &b.SourceType, &b.SourceLoc,
		&b.Buildspec, &b.Image, &b.ContainerID, &b.StartTime, &b.EndTime, &b.PhasesJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeBuildBuild{}, ErrCodeBuildBuildNotFound
	}
	if err != nil {
		return CodeBuildBuild{}, fmt.Errorf("get codebuild build: %w", err)
	}
	return b, nil
}

// ListCodeBuildBuilds returns build ids newest-first (optional project filter).
func (s *Store) ListCodeBuildBuilds(accountID, projectName string) ([]string, error) {
	projectName = strings.TrimSpace(projectName)
	var rows *sql.Rows
	var err error
	if projectName == "" {
		rows, err = s.db.Query(
			`SELECT build_id FROM codebuild_builds WHERE account_id = ? ORDER BY start_time DESC`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT build_id FROM codebuild_builds WHERE account_id = ? AND project_name = ? ORDER BY start_time DESC`,
			accountID, projectName,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list codebuild builds: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list codebuild builds scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ResolveCodeBuildBuildspec returns buildspec text, loading from S3 when source is S3 and buildspec empty.
func (s *Store) ResolveCodeBuildBuildspec(accountID string, b CodeBuildBuild) (string, error) {
	if strings.TrimSpace(b.Buildspec) != "" {
		return b.Buildspec, nil
	}
	if strings.ToUpper(b.SourceType) != "S3" {
		return "", fmt.Errorf("%w: buildspec is empty", ErrCodeBuildInvalidInput)
	}
	bucket, key, err := parseS3SourceLocation(b.SourceLoc)
	if err != nil {
		return "", err
	}
	_, data, err := s.GetObject(accountID, bucket, key)
	if err != nil {
		return "", fmt.Errorf("%w: unable to load S3 source: %v", ErrCodeBuildInvalidInput, err)
	}
	return string(data), nil
}

func parseS3SourceLocation(loc string) (bucket, key string, err error) {
	loc = strings.TrimSpace(loc)
	loc = strings.TrimPrefix(loc, "s3://")
	parts := strings.SplitN(loc, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("%w: source.location must be bucket/key", ErrCodeBuildInvalidInput)
	}
	return parts[0], parts[1], nil
}

// ExtractBuildspecCommands returns shell commands from a lab buildspec (JSON preferred, YAML-ish fallback).
func ExtractBuildspecCommands(buildspec string) []string {
	buildspec = strings.TrimSpace(buildspec)
	if buildspec == "" {
		return nil
	}
	if strings.HasPrefix(buildspec, "{") {
		var doc map[string]any
		if err := json.Unmarshal([]byte(buildspec), &doc); err == nil {
			if phases, ok := doc["phases"].(map[string]any); ok {
				for _, phaseName := range []string{"install", "pre_build", "build", "post_build"} {
					phase, _ := phases[phaseName].(map[string]any)
					cmds, _ := phase["commands"].([]any)
					out := make([]string, 0, len(cmds))
					for _, c := range cmds {
						if s, ok := c.(string); ok && strings.TrimSpace(s) != "" {
							out = append(out, s)
						}
					}
					if len(out) > 0 {
						return out
					}
				}
			}
		}
	}
	// YAML-ish: collect indented "- cmd" lines after a commands: key.
	var out []string
	inCommands := false
	for _, line := range strings.Split(buildspec, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasSuffix(trim, "commands:") || trim == "commands:" {
			inCommands = true
			continue
		}
		if inCommands {
			if strings.HasPrefix(trim, "- ") {
				out = append(out, strings.TrimSpace(strings.TrimPrefix(trim, "- ")))
				continue
			}
			if trim != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inCommands = false
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	return []string{buildspec}
}
