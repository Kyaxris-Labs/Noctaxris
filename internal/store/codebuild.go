package store

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
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
	CodeBuildStatusStopped    = "STOPPED"
)

var (
	ErrCodeBuildProjectExists   = errors.New("ResourceAlreadyExistsException")
	ErrCodeBuildProjectNotFound = errors.New("ResourceNotFoundException")
	ErrCodeBuildBuildNotFound   = errors.New("ResourceNotFoundException")
	ErrCodeBuildInvalidInput    = errors.New("InvalidInputException")
)

// CodeBuildEnvVar is a project or build environment variable.
type CodeBuildEnvVar struct {
	Name  string
	Value string
}

// CodeBuildProject is a CreateProject row.
type CodeBuildProject struct {
	Name                 string
	ARN                  string
	Description          string
	ServiceRole          string
	SourceType           string
	SourceLoc            string
	Buildspec            string
	Image                string
	Artifacts            string // JSON
	EnvVarsJSON          string // JSON array of {Name,Value}
	OverrideAllowed      bool   // when false, StartBuild rejects buildspec/env overrides
	VpcConfigJSON        string // config-only stub JSON object
	CacheJSON            string // config-only stub JSON object
	SecondarySourcesJSON string // config-only stub JSON array
	FleetJSON            string // config-only stub JSON object (fleets)
	ReportArnsJSON       string // config-only stub JSON array of report group ARNs
	CreatedAt            string
}

// CodeBuildBuild is a StartBuild row.
type CodeBuildBuild struct {
	ID               string
	ARN              string
	ProjectName      string
	ProjectARN       string
	BuildStatus      string
	SourceType       string
	SourceLoc        string
	Buildspec        string
	Image            string
	ContainerID      string
	StartTime        string
	EndTime          string
	PhasesJSON       string
	LogsText         string
	LogGroup         string
	LogStream        string
	EnvVarsJSON      string // override snapshot JSON
	BatchID          string
	ArtifactLocation string
}

// CodeBuildBatch is a StartBuildBatch parent row.
type CodeBuildBatch struct {
	ID               string
	ARN              string
	ProjectName      string
	ProjectARN       string
	BuildBatchStatus string
	StartTime        string
	EndTime          string
	ChildBuildIDs    []string
	ServiceRole      string
	SourceType       string
	SourceLoc        string
	Buildspec        string
	Image            string
}

// CodeBuildBatchChild describes one matrix/list child build.
type CodeBuildBatchChild struct {
	Identifier string
	EnvVars    []CodeBuildEnvVar
}

// CreateCodeBuildProjectInput holds CreateProject fields.
type CreateCodeBuildProjectInput struct {
	Name             string
	Description      string
	ServiceRole      string
	SourceType       string
	SourceLoc        string
	Buildspec        string
	Image            string
	Artifacts        map[string]any
	EnvVars          []CodeBuildEnvVar
	OverrideAllowed  *bool // nil defaults to true (overrides allowed)
	VpcConfig        map[string]any
	Cache            map[string]any
	SecondarySources []any
	Fleet            map[string]any
	ReportGroupArns  []string
}

// UpdateCodeBuildProjectInput holds UpdateProject fields.
type UpdateCodeBuildProjectInput struct {
	Name             string
	Description      string
	ServiceRole      string
	SourceType       string
	SourceLoc        string
	Buildspec        string
	Image            string
	Artifacts        map[string]any
	EnvVars          []CodeBuildEnvVar
	OverrideAllowed  *bool // nil keeps existing value
	VpcConfig        map[string]any
	Cache            map[string]any
	SecondarySources []any
	Fleet            map[string]any
	ReportGroupArns  []string
}

// StartCodeBuildBuildOpts holds StartBuild options including env overrides.
type StartCodeBuildBuildOpts struct {
	ProjectName       string
	BuildspecOverride string
	EnvOverride       []CodeBuildEnvVar
	BatchID           string // set for batch children
	SkipOverrideLock  bool   // true after StartBuildBatch validates request overrides
}

// StartCodeBuildBuildBatchOpts holds StartBuildBatch options.
type StartCodeBuildBuildBatchOpts struct {
	ProjectName       string
	BuildspecOverride string
	EnvOverride       []CodeBuildEnvVar // request-level; subject to override-lock
	Children          []CodeBuildBatchChild
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
  env_vars_json TEXT NOT NULL DEFAULT '[]',
  override_allowed INTEGER NOT NULL DEFAULT 1,
  vpc_config_json TEXT NOT NULL DEFAULT '',
  cache_json TEXT NOT NULL DEFAULT '',
  secondary_sources_json TEXT NOT NULL DEFAULT '',
  fleet_json TEXT NOT NULL DEFAULT '',
  report_arns_json TEXT NOT NULL DEFAULT '',
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
  logs_text TEXT NOT NULL DEFAULT '',
  log_group TEXT NOT NULL DEFAULT '',
  log_stream TEXT NOT NULL DEFAULT '',
  env_vars_json TEXT NOT NULL DEFAULT '[]',
  batch_id TEXT NOT NULL DEFAULT '',
  artifact_location TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, build_id)
);
CREATE TABLE IF NOT EXISTS codebuild_batches (
  account_id TEXT NOT NULL,
  batch_id TEXT NOT NULL,
  batch_arn TEXT NOT NULL,
  project_name TEXT NOT NULL,
  project_arn TEXT NOT NULL,
  build_batch_status TEXT NOT NULL,
  start_time TEXT NOT NULL,
  end_time TEXT NOT NULL DEFAULT '',
  child_build_ids_json TEXT NOT NULL DEFAULT '[]',
  PRIMARY KEY (account_id, batch_id)
);
CREATE INDEX IF NOT EXISTS idx_codebuild_builds_project ON codebuild_builds(account_id, project_name);
CREATE INDEX IF NOT EXISTS idx_codebuild_builds_batch ON codebuild_builds(account_id, batch_id);
`

// EnsureCodeBuildSchema creates CodeBuild tables if missing and applies additive migrations.
func EnsureCodeBuildSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure codebuild schema: db is nil")
	}
	if _, err := db.Exec(codebuildSchema); err != nil {
		return fmt.Errorf("ensure codebuild schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE codebuild_projects ADD COLUMN env_vars_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE codebuild_projects ADD COLUMN override_allowed INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE codebuild_projects ADD COLUMN vpc_config_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE codebuild_projects ADD COLUMN cache_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE codebuild_projects ADD COLUMN secondary_sources_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE codebuild_projects ADD COLUMN fleet_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE codebuild_projects ADD COLUMN report_arns_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE codebuild_builds ADD COLUMN logs_text TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE codebuild_builds ADD COLUMN log_group TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE codebuild_builds ADD COLUMN log_stream TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE codebuild_builds ADD COLUMN env_vars_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE codebuild_builds ADD COLUMN batch_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE codebuild_builds ADD COLUMN artifact_location TEXT NOT NULL DEFAULT ''`,
	}); err != nil {
		return fmt.Errorf("ensure codebuild schema alter: %w", err)
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

// CodeBuildBuildBatchARN builds arn:aws:codebuild:REGION:ACCOUNT:build-batch/PROJECT:ID.
func CodeBuildBuildBatchARN(region, accountID, projectName, batchID string) string {
	if region == "" {
		region = DefaultCodeBuildRegion
	}
	return fmt.Sprintf("arn:aws:codebuild:%s:%s:build-batch/%s:%s", region, accountID, projectName, batchID)
}

func codebuildOverrideAllowedInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func codebuildRejectOverridesIfLocked(p CodeBuildProject, buildspecOverride string, envOverride []CodeBuildEnvVar) error {
	if p.OverrideAllowed {
		return nil
	}
	if strings.TrimSpace(buildspecOverride) != "" || len(envOverride) > 0 {
		return fmt.Errorf("%w: project does not allow buildspecOverride or environmentVariablesOverride", ErrCodeBuildInvalidInput)
	}
	return nil
}

func marshalCodeBuildEnvVars(vars []CodeBuildEnvVar) (string, error) {
	if vars == nil {
		vars = []CodeBuildEnvVar{}
	}
	b, err := json.Marshal(vars)
	if err != nil {
		return "", fmt.Errorf("marshal env vars: %w", err)
	}
	return string(b), nil
}

func normalizeCodeBuildSourceType(sourceType string) (string, error) {
	sourceType = strings.TrimSpace(sourceType)
	if sourceType == "" {
		return "", fmt.Errorf("%w: source.type is required", ErrCodeBuildInvalidInput)
	}
	switch strings.ToUpper(sourceType) {
	case "NO_SOURCE", "S3", "CODECOMMIT":
		return strings.ToUpper(sourceType), nil
	default:
		return "", fmt.Errorf("%w: source.type must be NO_SOURCE, S3, or CODECOMMIT", ErrCodeBuildInvalidInput)
	}
}

func validateCodeBuildProjectFields(sourceType, sourceLoc, buildspec, image, serviceRole string) error {
	if strings.TrimSpace(serviceRole) == "" {
		return fmt.Errorf("%w: serviceRole is required", ErrCodeBuildInvalidInput)
	}
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("%w: environment.image is required", ErrCodeBuildInvalidInput)
	}
	if sourceType == "NO_SOURCE" && strings.TrimSpace(buildspec) == "" {
		return fmt.Errorf("%w: source.buildspec is required for NO_SOURCE", ErrCodeBuildInvalidInput)
	}
	if sourceType == "S3" && strings.TrimSpace(sourceLoc) == "" {
		return fmt.Errorf("%w: source.location is required for S3", ErrCodeBuildInvalidInput)
	}
	if sourceType == "CODECOMMIT" && strings.TrimSpace(sourceLoc) == "" {
		return fmt.Errorf("%w: source.location is required for CODECOMMIT", ErrCodeBuildInvalidInput)
	}
	return nil
}

func marshalCodeBuildOptionalJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 {
			return "", nil
		}
	case []any:
		if len(t) == 0 {
			return "", nil
		}
	case []string:
		if len(t) == 0 {
			return "", nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal codebuild config: %w", err)
	}
	return string(b), nil
}

// CreateCodeBuildProject inserts a project.
func (s *Store) CreateCodeBuildProject(accountID, region string, in CreateCodeBuildProjectInput) (CodeBuildProject, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return CodeBuildProject{}, fmt.Errorf("%w: name is required", ErrCodeBuildInvalidInput)
	}
	sourceType, err := normalizeCodeBuildSourceType(in.SourceType)
	if err != nil {
		return CodeBuildProject{}, err
	}
	if err := validateCodeBuildProjectFields(sourceType, in.SourceLoc, in.Buildspec, in.Image, in.ServiceRole); err != nil {
		return CodeBuildProject{}, err
	}

	artifacts := in.Artifacts
	if artifacts == nil {
		artifacts = map[string]any{"type": "NO_ARTIFACTS"}
	}
	artJSON, err := json.Marshal(artifacts)
	if err != nil {
		return CodeBuildProject{}, fmt.Errorf("marshal artifacts: %w", err)
	}
	envJSON, err := marshalCodeBuildEnvVars(in.EnvVars)
	if err != nil {
		return CodeBuildProject{}, err
	}
	vpcJSON, err := marshalCodeBuildOptionalJSON(in.VpcConfig)
	if err != nil {
		return CodeBuildProject{}, err
	}
	cacheJSON, err := marshalCodeBuildOptionalJSON(in.Cache)
	if err != nil {
		return CodeBuildProject{}, err
	}
	secJSON, err := marshalCodeBuildOptionalJSON(in.SecondarySources)
	if err != nil {
		return CodeBuildProject{}, err
	}
	fleetJSON, err := marshalCodeBuildOptionalJSON(in.Fleet)
	if err != nil {
		return CodeBuildProject{}, err
	}
	reportJSON, err := marshalCodeBuildOptionalJSON(in.ReportGroupArns)
	if err != nil {
		return CodeBuildProject{}, err
	}

	arn := CodeBuildProjectARN(region, accountID, name)
	created := nowRFC3339()
	image := strings.TrimSpace(in.Image)
	overrideAllowed := true
	if in.OverrideAllowed != nil {
		overrideAllowed = *in.OverrideAllowed
	}
	_, err = s.db.Exec(
		`INSERT INTO codebuild_projects (
			account_id, name, project_arn, description, service_role,
			source_type, source_location, buildspec, image, artifacts_json, env_vars_json, override_allowed,
			vpc_config_json, cache_json, secondary_sources_json, fleet_json, report_arns_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, arn, strings.TrimSpace(in.Description), strings.TrimSpace(in.ServiceRole),
		sourceType, strings.TrimSpace(in.SourceLoc), in.Buildspec, image, string(artJSON), envJSON,
		codebuildOverrideAllowedInt(overrideAllowed),
		vpcJSON, cacheJSON, secJSON, fleetJSON, reportJSON, created,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return CodeBuildProject{}, ErrCodeBuildProjectExists
		}
		return CodeBuildProject{}, fmt.Errorf("create codebuild project: %w", err)
	}
	return CodeBuildProject{
		Name:                 name,
		ARN:                  arn,
		Description:          strings.TrimSpace(in.Description),
		ServiceRole:          strings.TrimSpace(in.ServiceRole),
		SourceType:           sourceType,
		SourceLoc:            strings.TrimSpace(in.SourceLoc),
		Buildspec:            in.Buildspec,
		Image:                image,
		Artifacts:            string(artJSON),
		EnvVarsJSON:          envJSON,
		OverrideAllowed:      overrideAllowed,
		VpcConfigJSON:        vpcJSON,
		CacheJSON:            cacheJSON,
		SecondarySourcesJSON: secJSON,
		FleetJSON:            fleetJSON,
		ReportArnsJSON:       reportJSON,
		CreatedAt:            created,
	}, nil
}

// GetCodeBuildProject returns a project by name.
func (s *Store) GetCodeBuildProject(accountID, name string) (CodeBuildProject, error) {
	name = strings.TrimSpace(name)
	var p CodeBuildProject
	var overrideInt int
	err := s.db.QueryRow(
		`SELECT name, project_arn, description, service_role, source_type, source_location,
			buildspec, image, artifacts_json, env_vars_json, override_allowed,
			vpc_config_json, cache_json, secondary_sources_json, fleet_json, report_arns_json, created_at
		 FROM codebuild_projects WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(
		&p.Name, &p.ARN, &p.Description, &p.ServiceRole, &p.SourceType, &p.SourceLoc,
		&p.Buildspec, &p.Image, &p.Artifacts, &p.EnvVarsJSON, &overrideInt,
		&p.VpcConfigJSON, &p.CacheJSON, &p.SecondarySourcesJSON, &p.FleetJSON, &p.ReportArnsJSON, &p.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeBuildProject{}, ErrCodeBuildProjectNotFound
	}
	if err != nil {
		return CodeBuildProject{}, fmt.Errorf("get codebuild project: %w", err)
	}
	p.OverrideAllowed = overrideInt != 0
	return p, nil
}

// ListCodeBuildProjects returns all projects for an account, name-ordered.
func (s *Store) ListCodeBuildProjects(accountID string) ([]CodeBuildProject, error) {
	rows, err := s.db.Query(
		`SELECT name, project_arn, description, service_role, source_type, source_location,
			buildspec, image, artifacts_json, env_vars_json, override_allowed,
			vpc_config_json, cache_json, secondary_sources_json, fleet_json, report_arns_json, created_at
		 FROM codebuild_projects WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list codebuild projects: %w", err)
	}
	defer rows.Close()
	var out []CodeBuildProject
	for rows.Next() {
		var p CodeBuildProject
		var overrideInt int
		if err := rows.Scan(
			&p.Name, &p.ARN, &p.Description, &p.ServiceRole, &p.SourceType, &p.SourceLoc,
			&p.Buildspec, &p.Image, &p.Artifacts, &p.EnvVarsJSON, &overrideInt,
			&p.VpcConfigJSON, &p.CacheJSON, &p.SecondarySourcesJSON, &p.FleetJSON, &p.ReportArnsJSON, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("list codebuild projects scan: %w", err)
		}
		p.OverrideAllowed = overrideInt != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// BatchGetCodeBuildProjects returns projects by name (missing names omitted).
func (s *Store) BatchGetCodeBuildProjects(accountID string, names []string) ([]CodeBuildProject, error) {
	out := make([]CodeBuildProject, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		p, err := s.GetCodeBuildProject(accountID, name)
		if errors.Is(err, ErrCodeBuildProjectNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// UpdateCodeBuildProject updates an existing project.
func (s *Store) UpdateCodeBuildProject(accountID, region string, in UpdateCodeBuildProjectInput) (CodeBuildProject, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return CodeBuildProject{}, fmt.Errorf("%w: name is required", ErrCodeBuildInvalidInput)
	}
	existing, err := s.GetCodeBuildProject(accountID, name)
	if err != nil {
		return CodeBuildProject{}, err
	}
	_ = region // ARN is stable; region kept for API symmetry with Create.

	sourceType, err := normalizeCodeBuildSourceType(in.SourceType)
	if err != nil {
		return CodeBuildProject{}, err
	}
	if err := validateCodeBuildProjectFields(sourceType, in.SourceLoc, in.Buildspec, in.Image, in.ServiceRole); err != nil {
		return CodeBuildProject{}, err
	}

	artifacts := in.Artifacts
	if artifacts == nil {
		artifacts = map[string]any{"type": "NO_ARTIFACTS"}
	}
	artJSON, err := json.Marshal(artifacts)
	if err != nil {
		return CodeBuildProject{}, fmt.Errorf("marshal artifacts: %w", err)
	}
	envJSON, err := marshalCodeBuildEnvVars(in.EnvVars)
	if err != nil {
		return CodeBuildProject{}, err
	}
	vpcJSON, err := marshalCodeBuildOptionalJSON(in.VpcConfig)
	if err != nil {
		return CodeBuildProject{}, err
	}
	cacheJSON, err := marshalCodeBuildOptionalJSON(in.Cache)
	if err != nil {
		return CodeBuildProject{}, err
	}
	secJSON, err := marshalCodeBuildOptionalJSON(in.SecondarySources)
	if err != nil {
		return CodeBuildProject{}, err
	}
	fleetJSON, err := marshalCodeBuildOptionalJSON(in.Fleet)
	if err != nil {
		return CodeBuildProject{}, err
	}
	reportJSON, err := marshalCodeBuildOptionalJSON(in.ReportGroupArns)
	if err != nil {
		return CodeBuildProject{}, err
	}

	image := strings.TrimSpace(in.Image)
	desc := strings.TrimSpace(in.Description)
	role := strings.TrimSpace(in.ServiceRole)
	loc := strings.TrimSpace(in.SourceLoc)
	overrideAllowed := existing.OverrideAllowed
	if in.OverrideAllowed != nil {
		overrideAllowed = *in.OverrideAllowed
	}
	_, err = s.db.Exec(
		`UPDATE codebuild_projects SET
			description = ?, service_role = ?, source_type = ?, source_location = ?,
			buildspec = ?, image = ?, artifacts_json = ?, env_vars_json = ?, override_allowed = ?,
			vpc_config_json = ?, cache_json = ?, secondary_sources_json = ?, fleet_json = ?, report_arns_json = ?
		 WHERE account_id = ? AND name = ?`,
		desc, role, sourceType, loc, in.Buildspec, image, string(artJSON), envJSON,
		codebuildOverrideAllowedInt(overrideAllowed),
		vpcJSON, cacheJSON, secJSON, fleetJSON, reportJSON,
		accountID, name,
	)
	if err != nil {
		return CodeBuildProject{}, fmt.Errorf("update codebuild project: %w", err)
	}
	return CodeBuildProject{
		Name:                 name,
		ARN:                  existing.ARN,
		Description:          desc,
		ServiceRole:          role,
		SourceType:           sourceType,
		SourceLoc:            loc,
		Buildspec:            in.Buildspec,
		Image:                image,
		Artifacts:            string(artJSON),
		EnvVarsJSON:          envJSON,
		OverrideAllowed:      overrideAllowed,
		VpcConfigJSON:        vpcJSON,
		CacheJSON:            cacheJSON,
		SecondarySourcesJSON: secJSON,
		FleetJSON:            fleetJSON,
		ReportArnsJSON:       reportJSON,
		CreatedAt:            existing.CreatedAt,
	}, nil
}

// DeleteCodeBuildProject removes a project by name.
func (s *Store) DeleteCodeBuildProject(accountID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrCodeBuildInvalidInput)
	}
	res, err := s.db.Exec(
		`DELETE FROM codebuild_projects WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete codebuild project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete codebuild project rows: %w", err)
	}
	if n == 0 {
		return ErrCodeBuildProjectNotFound
	}
	return nil
}

// StartCodeBuildBuild creates an IN_PROGRESS build from a project.
func (s *Store) StartCodeBuildBuild(accountID, region string, opts StartCodeBuildBuildOpts) (CodeBuildBuild, error) {
	p, err := s.GetCodeBuildProject(accountID, opts.ProjectName)
	if err != nil {
		return CodeBuildBuild{}, err
	}
	if !opts.SkipOverrideLock {
		if err := codebuildRejectOverridesIfLocked(p, opts.BuildspecOverride, opts.EnvOverride); err != nil {
			return CodeBuildBuild{}, err
		}
	}
	if strings.EqualFold(p.SourceType, "CODECOMMIT") {
		if _, err := s.EnsureCodeBuildCodeCommitSource(accountID, p.SourceLoc); err != nil {
			return CodeBuildBuild{}, err
		}
	}
	buildspec := p.Buildspec
	if strings.TrimSpace(opts.BuildspecOverride) != "" {
		buildspec = opts.BuildspecOverride
	}
	envJSON, err := marshalCodeBuildEnvVars(opts.EnvOverride)
	if err != nil {
		return CodeBuildBuild{}, err
	}
	id := uuid.NewString()
	arn := CodeBuildBuildARN(region, accountID, p.Name, id)
	start := nowRFC3339()
	batchID := strings.TrimSpace(opts.BatchID)
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
		EnvVarsJSON: envJSON,
		BatchID:     batchID,
	}
	_, err = s.db.Exec(
		`INSERT INTO codebuild_builds (
			account_id, build_id, build_arn, project_name, project_arn, build_status,
			source_type, source_location, buildspec, image, container_id, start_time, end_time, phases_json,
			logs_text, log_group, log_stream, env_vars_json, batch_id, artifact_location
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, '', '[]', '', '', '', ?, ?, '')`,
		accountID, b.ID, b.ARN, b.ProjectName, b.ProjectARN, b.BuildStatus,
		b.SourceType, b.SourceLoc, b.Buildspec, b.Image, b.StartTime, b.EnvVarsJSON, batchID,
	)
	if err != nil {
		return CodeBuildBuild{}, fmt.Errorf("start codebuild build: %w", err)
	}
	return b, nil
}

// StopCodeBuildBuild marks an IN_PROGRESS build as STOPPED.
func (s *Store) StopCodeBuildBuild(accountID, buildID string) (CodeBuildBuild, error) {
	buildID = strings.TrimSpace(buildID)
	if i := strings.LastIndex(buildID, ":"); i >= 0 && strings.Contains(buildID, ":build/") {
		buildID = buildID[i+1:]
	}
	b, err := s.getCodeBuildBuild(accountID, buildID)
	if err != nil {
		return CodeBuildBuild{}, err
	}
	if b.BuildStatus != CodeBuildStatusInProgress {
		return b, nil
	}
	end := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE codebuild_builds SET build_status = ?, end_time = ?
		 WHERE account_id = ? AND build_id = ?`,
		CodeBuildStatusStopped, end, accountID, buildID,
	)
	if err != nil {
		return CodeBuildBuild{}, fmt.Errorf("stop codebuild build: %w", err)
	}
	b.BuildStatus = CodeBuildStatusStopped
	b.EndTime = end
	return b, nil
}

// SetCodeBuildBuildLogs stores captured build logs and CloudWatch lite identifiers.
func (s *Store) SetCodeBuildBuildLogs(accountID, buildID, logs, logGroup, logStream string) error {
	buildID = strings.TrimSpace(buildID)
	if i := strings.LastIndex(buildID, ":"); i >= 0 && strings.Contains(buildID, ":build/") {
		buildID = buildID[i+1:]
	}
	res, err := s.db.Exec(
		`UPDATE codebuild_builds SET logs_text = ?, log_group = ?, log_stream = ?
		 WHERE account_id = ? AND build_id = ?`,
		logs, logGroup, logStream, accountID, buildID,
	)
	if err != nil {
		return fmt.Errorf("set codebuild build logs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set codebuild build logs rows: %w", err)
	}
	if n == 0 {
		return ErrCodeBuildBuildNotFound
	}
	return nil
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
			buildspec, image, container_id, start_time, end_time, phases_json,
			logs_text, log_group, log_stream, env_vars_json, batch_id, artifact_location
		 FROM codebuild_builds WHERE account_id = ? AND build_id = ?`,
		accountID, buildID,
	).Scan(
		&b.ID, &b.ARN, &b.ProjectName, &b.ProjectARN, &b.BuildStatus, &b.SourceType, &b.SourceLoc,
		&b.Buildspec, &b.Image, &b.ContainerID, &b.StartTime, &b.EndTime, &b.PhasesJSON,
		&b.LogsText, &b.LogGroup, &b.LogStream, &b.EnvVarsJSON, &b.BatchID, &b.ArtifactLocation,
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

// ResolveCodeBuildBuildspec returns buildspec text, loading from S3 or CODECOMMIT when empty.
func (s *Store) ResolveCodeBuildBuildspec(accountID string, b CodeBuildBuild) (string, error) {
	if strings.TrimSpace(b.Buildspec) != "" {
		return b.Buildspec, nil
	}
	switch strings.ToUpper(b.SourceType) {
	case "S3":
		bucket, key, err := parseS3SourceLocation(b.SourceLoc)
		if err != nil {
			return "", err
		}
		_, data, err := s.GetObject(accountID, bucket, key)
		if err != nil {
			return "", fmt.Errorf("%w: unable to load S3 source: %v", ErrCodeBuildInvalidInput, err)
		}
		return string(data), nil
	case "CODECOMMIT":
		repoName, err := s.EnsureCodeBuildCodeCommitSource(accountID, b.SourceLoc)
		if err != nil {
			return "", err
		}
		tree, err := s.ExportCodeCommitRepoTree(accountID, repoName)
		if err != nil {
			return "", fmt.Errorf("%w: unable to export CODECOMMIT source: %v", ErrCodeBuildInvalidInput, err)
		}
		for _, name := range []string{"buildspec.yml", "buildspec.yaml", "buildspec.json"} {
			raw, readErr := os.ReadFile(filepath.Join(tree, name))
			if readErr != nil {
				continue
			}
			if strings.TrimSpace(string(raw)) == "" {
				continue
			}
			return string(raw), nil
		}
		return "", fmt.Errorf("%w: buildspec is empty and no buildspec.yml in CODECOMMIT source", ErrCodeBuildInvalidInput)
	default:
		return "", fmt.Errorf("%w: buildspec is empty", ErrCodeBuildInvalidInput)
	}
}

// ParseCodeBuildCodeCommitLocation accepts a lab repo name or CodeCommit ARN.
func ParseCodeBuildCodeCommitLocation(loc string) (string, error) {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return "", fmt.Errorf("%w: source.location is required for CODECOMMIT", ErrCodeBuildInvalidInput)
	}
	if strings.HasPrefix(strings.ToLower(loc), "arn:aws:codecommit:") {
		// arn:aws:codecommit:REGION:ACCOUNT:NAME
		parts := strings.Split(loc, ":")
		if len(parts) < 6 {
			return "", fmt.Errorf("%w: invalid CODECOMMIT source.location ARN", ErrCodeBuildInvalidInput)
		}
		name := strings.TrimSpace(parts[len(parts)-1])
		if name == "" || strings.Contains(name, "/") {
			return "", fmt.Errorf("%w: invalid CODECOMMIT source.location ARN", ErrCodeBuildInvalidInput)
		}
		return name, nil
	}
	if strings.Contains(loc, "/") || strings.Contains(loc, ":") {
		return "", fmt.Errorf("%w: CODECOMMIT source.location must be a repository name or lab ARN", ErrCodeBuildInvalidInput)
	}
	return loc, nil
}

// EnsureCodeBuildCodeCommitSource resolves location and fails closed when the repo is missing.
func (s *Store) EnsureCodeBuildCodeCommitSource(accountID, sourceLoc string) (string, error) {
	repoName, err := ParseCodeBuildCodeCommitLocation(sourceLoc)
	if err != nil {
		return "", err
	}
	if _, err := s.GetCodeCommitRepository(accountID, DefaultCodeCommitRegion, repoName); err != nil {
		if errors.Is(err, ErrCodeCommitRepoNotFound) {
			return "", fmt.Errorf("%w: CODECOMMIT repository %q not found", ErrCodeBuildInvalidInput, repoName)
		}
		return "", fmt.Errorf("%w: unable to resolve CODECOMMIT source: %v", ErrCodeBuildInvalidInput, err)
	}
	return repoName, nil
}

// MaterializeCodeBuildCodeCommitSource copies the lab CodeCommit tree into destDir.
func (s *Store) MaterializeCodeBuildCodeCommitSource(accountID, sourceLoc, destDir string) (string, error) {
	repoName, err := s.EnsureCodeBuildCodeCommitSource(accountID, sourceLoc)
	if err != nil {
		return "", err
	}
	if err := s.MaterializeCodeCommitRepo(accountID, repoName, destDir); err != nil {
		return "", fmt.Errorf("%w: materialize CODECOMMIT source: %v", ErrCodeBuildInvalidInput, err)
	}
	return repoName, nil
}

// BuildCodeBuildCodeCommitShell builds a /bin/sh -c script that unpacks a materialized
// tree into /codebuild/src then runs build commands (works with RunECSTask without binds).
func BuildCodeBuildCodeCommitShell(srcDir string, cmds []string) (string, error) {
	srcDir = filepath.Clean(strings.TrimSpace(srcDir))
	if srcDir == "" || srcDir == "." {
		return "", fmt.Errorf("%w: CODECOMMIT source dir required", ErrCodeBuildInvalidInput)
	}
	if len(cmds) == 0 {
		return "", fmt.Errorf("%w: no build commands", ErrCodeBuildInvalidInput)
	}
	payload, err := tarDirectoryBase64(srcDir)
	if err != nil {
		return "", err
	}
	build := strings.Join(cmds, " && ")
	return "mkdir -p /codebuild/src && cd /codebuild/src && echo '" + payload + "' | base64 -d | tar -xf - && " + build, nil
}

func tarDirectoryBase64(srcDir string) (string, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = rel + "/"
			return tw.WriteHeader(hdr)
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = tw.Close()
		return "", fmt.Errorf("%w: pack CODECOMMIT source: %v", ErrCodeBuildInvalidInput, err)
	}
	if err := tw.Close(); err != nil {
		return "", fmt.Errorf("%w: pack CODECOMMIT source: %v", ErrCodeBuildInvalidInput, err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
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

// ExpandCodeBuildBatchChildren resolves matrix/list children for StartBuildBatch.
// Prefer explicit children; otherwise parse buildspec batch.build-list / batch.build-matrix;
// otherwise a single default child.
func ExpandCodeBuildBatchChildren(buildspec string, children []CodeBuildBatchChild) []CodeBuildBatchChild {
	if len(children) > 0 {
		out := make([]CodeBuildBatchChild, 0, len(children))
		for i, c := range children {
			id := strings.TrimSpace(c.Identifier)
			if id == "" {
				id = fmt.Sprintf("BUILD_%d", i)
			}
			out = append(out, CodeBuildBatchChild{Identifier: id, EnvVars: c.EnvVars})
		}
		return out
	}
	if parsed := parseCodeBuildBatchFromBuildspec(buildspec); len(parsed) > 0 {
		return parsed
	}
	return []CodeBuildBatchChild{{Identifier: "BUILD"}}
}

func parseCodeBuildBatchFromBuildspec(buildspec string) []CodeBuildBatchChild {
	buildspec = strings.TrimSpace(buildspec)
	if !strings.HasPrefix(buildspec, "{") {
		return nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(buildspec), &doc); err != nil {
		return nil
	}
	batch, _ := doc["batch"].(map[string]any)
	if batch == nil {
		return nil
	}
	if list, ok := batch["build-list"].([]any); ok && len(list) > 0 {
		out := make([]CodeBuildBatchChild, 0, len(list))
		for i, item := range list {
			m, _ := item.(map[string]any)
			id := codebuildStringFromAny(m["identifier"])
			if id == "" {
				id = fmt.Sprintf("BUILD_%d", i)
			}
			out = append(out, CodeBuildBatchChild{
				Identifier: id,
				EnvVars:    envVarsFromBatchEnv(m["env"]),
			})
		}
		return out
	}
	matrix, _ := batch["build-matrix"].(map[string]any)
	if matrix == nil {
		return nil
	}
	if static, ok := matrix["static"].([]any); ok && len(static) > 0 {
		out := make([]CodeBuildBatchChild, 0, len(static))
		for i, item := range static {
			m, _ := item.(map[string]any)
			id := codebuildStringFromAny(m["identifier"])
			if id == "" {
				id = fmt.Sprintf("MATRIX_%d", i)
			}
			out = append(out, CodeBuildBatchChild{
				Identifier: id,
				EnvVars:    envVarsFromBatchEnv(m["env"]),
			})
		}
		return out
	}
	if dynamic, ok := matrix["dynamic"].(map[string]any); ok {
		envBlock, _ := dynamic["env"].(map[string]any)
		vars, _ := envBlock["variables"].(map[string]any)
		if len(vars) == 0 {
			return nil
		}
		// Cartesian product of listed values (lab-lite; cap at 32).
		keys := make([]string, 0, len(vars))
		valueLists := make([][]string, 0, len(vars))
		for k, raw := range vars {
			keys = append(keys, k)
			valueLists = append(valueLists, stringSliceFromAny(raw))
		}
		combos := cartesianStrings(valueLists)
		if len(combos) > 32 {
			combos = combos[:32]
		}
		out := make([]CodeBuildBatchChild, 0, len(combos))
		for i, combo := range combos {
			envs := make([]CodeBuildEnvVar, 0, len(keys))
			for j, k := range keys {
				envs = append(envs, CodeBuildEnvVar{Name: k, Value: combo[j]})
			}
			out = append(out, CodeBuildBatchChild{
				Identifier: fmt.Sprintf("MATRIX_%d", i),
				EnvVars:    envs,
			})
		}
		return out
	}
	return nil
}

func envVarsFromBatchEnv(v any) []CodeBuildEnvVar {
	m, _ := v.(map[string]any)
	if m == nil {
		return nil
	}
	vars, _ := m["variables"].(map[string]any)
	if vars == nil {
		return nil
	}
	out := make([]CodeBuildEnvVar, 0, len(vars))
	for k, raw := range vars {
		out = append(out, CodeBuildEnvVar{Name: k, Value: codebuildStringFromAny(raw)})
	}
	return out
}

func codebuildStringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%t", t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func stringSliceFromAny(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s := codebuildStringFromAny(item)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		return []string{t}
	default:
		s := codebuildStringFromAny(v)
		if s == "" {
			return nil
		}
		return []string{s}
	}
}

func cartesianStrings(lists [][]string) [][]string {
	if len(lists) == 0 {
		return nil
	}
	out := [][]string{{}}
	for _, list := range lists {
		if len(list) == 0 {
			list = []string{""}
		}
		next := make([][]string, 0, len(out)*len(list))
		for _, prefix := range out {
			for _, v := range list {
				row := make([]string, len(prefix)+1)
				copy(row, prefix)
				row[len(prefix)] = v
				next = append(next, row)
			}
		}
		out = next
	}
	return out
}

func mergeCodeBuildEnvVars(base, extra []CodeBuildEnvVar) []CodeBuildEnvVar {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	seen := map[string]int{}
	out := make([]CodeBuildEnvVar, 0, len(base)+len(extra))
	for _, v := range base {
		if strings.TrimSpace(v.Name) == "" {
			continue
		}
		if i, ok := seen[v.Name]; ok {
			out[i] = v
			continue
		}
		seen[v.Name] = len(out)
		out = append(out, v)
	}
	for _, v := range extra {
		if strings.TrimSpace(v.Name) == "" {
			continue
		}
		if i, ok := seen[v.Name]; ok {
			out[i] = v
			continue
		}
		seen[v.Name] = len(out)
		out = append(out, v)
	}
	return out
}

// StartCodeBuildBuildBatch creates a parent batch row and N child builds.
func (s *Store) StartCodeBuildBuildBatch(accountID, region string, opts StartCodeBuildBuildBatchOpts) (CodeBuildBatch, []CodeBuildBuild, error) {
	p, err := s.GetCodeBuildProject(accountID, opts.ProjectName)
	if err != nil {
		return CodeBuildBatch{}, nil, err
	}
	if err := codebuildRejectOverridesIfLocked(p, opts.BuildspecOverride, opts.EnvOverride); err != nil {
		return CodeBuildBatch{}, nil, err
	}
	buildspec := p.Buildspec
	if strings.TrimSpace(opts.BuildspecOverride) != "" {
		buildspec = opts.BuildspecOverride
	}
	children := ExpandCodeBuildBatchChildren(buildspec, opts.Children)
	if len(children) == 0 {
		return CodeBuildBatch{}, nil, fmt.Errorf("%w: batch requires at least one child build", ErrCodeBuildInvalidInput)
	}

	batchID := uuid.NewString()
	batchARN := CodeBuildBuildBatchARN(region, accountID, p.Name, batchID)
	start := nowRFC3339()
	builds := make([]CodeBuildBuild, 0, len(children))
	childIDs := make([]string, 0, len(children))
	for _, child := range children {
		env := mergeCodeBuildEnvVars(opts.EnvOverride, child.EnvVars)
		b, err := s.StartCodeBuildBuild(accountID, region, StartCodeBuildBuildOpts{
			ProjectName:       opts.ProjectName,
			BuildspecOverride: opts.BuildspecOverride,
			EnvOverride:       env,
			BatchID:           batchID,
			SkipOverrideLock:  true,
		})
		if err != nil {
			return CodeBuildBatch{}, nil, err
		}
		builds = append(builds, b)
		childIDs = append(childIDs, b.ID)
	}
	childJSON, err := json.Marshal(childIDs)
	if err != nil {
		return CodeBuildBatch{}, nil, fmt.Errorf("marshal batch children: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO codebuild_batches (
			account_id, batch_id, batch_arn, project_name, project_arn, build_batch_status,
			start_time, end_time, child_build_ids_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, '', ?)`,
		accountID, batchID, batchARN, p.Name, p.ARN, CodeBuildStatusInProgress, start, string(childJSON),
	)
	if err != nil {
		return CodeBuildBatch{}, nil, fmt.Errorf("start codebuild build batch: %w", err)
	}
	return CodeBuildBatch{
		ID:               batchID,
		ARN:              batchARN,
		ProjectName:      p.Name,
		ProjectARN:       p.ARN,
		BuildBatchStatus: CodeBuildStatusInProgress,
		StartTime:        start,
		ChildBuildIDs:    childIDs,
		ServiceRole:      p.ServiceRole,
		SourceType:       p.SourceType,
		SourceLoc:        p.SourceLoc,
		Buildspec:        buildspec,
		Image:            p.Image,
	}, builds, nil
}

// GetCodeBuildBuildBatch returns a batch by id.
func (s *Store) GetCodeBuildBuildBatch(accountID, batchID string) (CodeBuildBatch, error) {
	batchID = strings.TrimSpace(batchID)
	if i := strings.LastIndex(batchID, ":"); i >= 0 && strings.Contains(batchID, ":build-batch/") {
		batchID = batchID[i+1:]
	}
	var b CodeBuildBatch
	var childJSON string
	err := s.db.QueryRow(
		`SELECT batch_id, batch_arn, project_name, project_arn, build_batch_status, start_time, end_time, child_build_ids_json
		 FROM codebuild_batches WHERE account_id = ? AND batch_id = ?`,
		accountID, batchID,
	).Scan(&b.ID, &b.ARN, &b.ProjectName, &b.ProjectARN, &b.BuildBatchStatus, &b.StartTime, &b.EndTime, &childJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeBuildBatch{}, ErrCodeBuildBuildNotFound
	}
	if err != nil {
		return CodeBuildBatch{}, fmt.Errorf("get codebuild build batch: %w", err)
	}
	_ = json.Unmarshal([]byte(childJSON), &b.ChildBuildIDs)
	if b.ChildBuildIDs == nil {
		b.ChildBuildIDs = []string{}
	}
	if p, err := s.GetCodeBuildProject(accountID, b.ProjectName); err == nil {
		b.ServiceRole = p.ServiceRole
		b.SourceType = p.SourceType
		b.SourceLoc = p.SourceLoc
		b.Buildspec = p.Buildspec
		b.Image = p.Image
	}
	return b, nil
}

// SetCodeBuildBuildArtifactLocation stores the S3 artifact location on a build.
func (s *Store) SetCodeBuildBuildArtifactLocation(accountID, buildID, location string) error {
	buildID = strings.TrimSpace(buildID)
	if i := strings.LastIndex(buildID, ":"); i >= 0 && strings.Contains(buildID, ":build/") {
		buildID = buildID[i+1:]
	}
	res, err := s.db.Exec(
		`UPDATE codebuild_builds SET artifact_location = ? WHERE account_id = ? AND build_id = ?`,
		location, accountID, buildID,
	)
	if err != nil {
		return fmt.Errorf("set codebuild artifact location: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set codebuild artifact location rows: %w", err)
	}
	if n == 0 {
		return ErrCodeBuildBuildNotFound
	}
	return nil
}

// CodeBuildS3ArtifactTarget holds parsed project S3 artifact settings.
type CodeBuildS3ArtifactTarget struct {
	Bucket    string
	Path      string
	Name      string
	Packaging string // ZIP or NONE
}

// ParseCodeBuildS3Artifacts extracts S3 artifact target from project artifacts JSON.
// Returns ok=false when type is not S3 or location (bucket) is missing.
func ParseCodeBuildS3Artifacts(artifactsJSON string) (CodeBuildS3ArtifactTarget, bool) {
	var art map[string]any
	if err := json.Unmarshal([]byte(artifactsJSON), &art); err != nil || art == nil {
		return CodeBuildS3ArtifactTarget{}, false
	}
	typ := strings.ToUpper(strings.TrimSpace(codebuildStringFromAny(art["type"])))
	if typ != "S3" {
		return CodeBuildS3ArtifactTarget{}, false
	}
	bucket := strings.TrimSpace(codebuildStringFromAny(art["location"]))
	if bucket == "" {
		return CodeBuildS3ArtifactTarget{}, false
	}
	name := strings.TrimSpace(codebuildStringFromAny(art["name"]))
	if name == "" || name == "/" {
		name = "artifacts.zip"
	}
	packaging := strings.ToUpper(strings.TrimSpace(codebuildStringFromAny(art["packaging"])))
	if packaging == "" {
		packaging = "ZIP"
	}
	return CodeBuildS3ArtifactTarget{
		Bucket:    bucket,
		Path:      strings.Trim(strings.TrimSpace(codebuildStringFromAny(art["path"])), "/"),
		Name:      name,
		Packaging: packaging,
	}, true
}

// CodeBuildArtifactObjectKey builds bucket object key for a build artifact.
func CodeBuildArtifactObjectKey(target CodeBuildS3ArtifactTarget, buildID string) string {
	parts := make([]string, 0, 3)
	if target.Path != "" {
		parts = append(parts, target.Path)
	}
	parts = append(parts, buildID)
	parts = append(parts, target.Name)
	return path.Join(parts...)
}

// PublishCodeBuildBuildArtifacts uploads a lab artifact object when project artifacts.type=S3.
// When workspaceTar is non-empty, the ZIP (or body) prefers that workspace archive; otherwise
// content is logs-derived (build.log) as before.
func (s *Store) PublishCodeBuildBuildArtifacts(accountID string, b CodeBuildBuild, workspaceTar []byte) (string, error) {
	p, err := s.GetCodeBuildProject(accountID, b.ProjectName)
	if err != nil {
		return "", err
	}
	target, ok := ParseCodeBuildS3Artifacts(p.Artifacts)
	if !ok {
		return "", nil
	}
	key := CodeBuildArtifactObjectKey(target, b.ID)
	logsBody := []byte(b.LogsText)
	if len(logsBody) == 0 {
		logsBody = []byte("noctaxris-codebuild-artifact\n")
	}
	var data []byte
	contentType := "text/plain"
	useZip := target.Packaging == "ZIP" || strings.HasSuffix(strings.ToLower(target.Name), ".zip")
	if useZip {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		if len(workspaceTar) > 0 {
			w, err := zw.Create("codebuild-src.tar")
			if err != nil {
				return "", fmt.Errorf("create zip workspace entry: %w", err)
			}
			if _, err := w.Write(workspaceTar); err != nil {
				_ = zw.Close()
				return "", fmt.Errorf("write zip workspace entry: %w", err)
			}
		}
		w, err := zw.Create("build.log")
		if err != nil {
			_ = zw.Close()
			return "", fmt.Errorf("create zip entry: %w", err)
		}
		if _, err := w.Write(logsBody); err != nil {
			_ = zw.Close()
			return "", fmt.Errorf("write zip entry: %w", err)
		}
		if err := zw.Close(); err != nil {
			return "", fmt.Errorf("close zip: %w", err)
		}
		data = buf.Bytes()
		contentType = "application/zip"
	} else if len(workspaceTar) > 0 {
		data = workspaceTar
		contentType = "application/x-tar"
	} else {
		data = logsBody
	}
	if _, err := s.GetBucket(accountID, target.Bucket); err != nil {
		if _, cerr := s.CreateBucket(accountID, target.Bucket); cerr != nil {
			return "", fmt.Errorf("ensure artifact bucket: %w", cerr)
		}
	}
	if _, err := s.PutObject(accountID, target.Bucket, key, PutObjectMeta{
		Data:        data,
		PlainSize:   int64(len(data)),
		ContentType: contentType,
	}); err != nil {
		return "", fmt.Errorf("put codebuild artifact: %w", err)
	}
	location := target.Bucket + "/" + key
	if err := s.SetCodeBuildBuildArtifactLocation(accountID, b.ID, location); err != nil {
		return "", err
	}
	return location, nil
}
