package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCodePipelineExists   = errors.New("PipelineNameInUseException")
	ErrCodePipelineNotFound = errors.New("PipelineNotFoundException")
	ErrCodePipelineBadReq   = errors.New("InvalidStructureException")
)

const DefaultCodePipelineRegion = "us-east-1"

const codepipelineSchema = `
CREATE TABLE IF NOT EXISTS codepipeline_pipelines (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  pipeline_arn TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  definition TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS codepipeline_executions (
  account_id TEXT NOT NULL,
  pipeline_name TEXT NOT NULL,
  execution_id TEXT NOT NULL,
  status TEXT NOT NULL,
  start_time INTEGER NOT NULL,
  stage_states TEXT NOT NULL,
  PRIMARY KEY (execution_id)
);
CREATE INDEX IF NOT EXISTS idx_cp_exec_pipe ON codepipeline_executions(account_id, pipeline_name);
`

// CodePipelinePipeline is a pipeline row.
type CodePipelinePipeline struct {
	Name        string
	PipelineARN string
	RoleARN     string
	Definition  string // JSON pipeline declaration
	CreatedAt   int64
	UpdatedAt   int64
}

// CodePipelineExecution is one StartPipelineExecution result.
type CodePipelineExecution struct {
	PipelineName string
	ExecutionID  string
	Status       string
	StartTime    int64
	StageStates  string // JSON
}

// CodePipelineDeclaration is the CreatePipeline pipeline object (lab subset).
type CodePipelineDeclaration struct {
	Name    string                     `json:"name"`
	RoleARN string                     `json:"roleArn"`
	Stages  []CodePipelineStageDecl    `json:"stages"`
}

// CodePipelineStageDecl is one stage.
type CodePipelineStageDecl struct {
	Name    string                    `json:"name"`
	Actions []CodePipelineActionDecl  `json:"actions"`
}

// CodePipelineActionDecl is one action.
type CodePipelineActionDecl struct {
	Name            string            `json:"name"`
	ActionTypeID    CodePipelineActionTypeID `json:"actionTypeId"`
	Configuration   map[string]string `json:"configuration"`
	RunOrder        int               `json:"runOrder"`
}

// CodePipelineActionTypeID identifies a provider.
type CodePipelineActionTypeID struct {
	Category string `json:"category"`
	Owner    string `json:"owner"`
	Provider string `json:"provider"`
	Version  string `json:"version"`
}

// EnsureCodePipelineSchema creates CodePipeline tables if missing.
func EnsureCodePipelineSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure codepipeline schema: db is nil")
	}
	if _, err := db.Exec(codepipelineSchema); err != nil {
		return fmt.Errorf("ensure codepipeline schema: %w", err)
	}
	return nil
}

// EnsureCodePipelineSchema ensures CodePipeline tables on an open store.
func (s *Store) EnsureCodePipelineSchema() error {
	return EnsureCodePipelineSchema(s.db)
}

// CodePipelineARN builds arn:aws:codepipeline:REGION:ACCOUNT:NAME
func CodePipelineARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultCodePipelineRegion
	}
	return fmt.Sprintf("arn:aws:codepipeline:%s:%s:%s", region, accountID, name)
}

// CreateCodePipeline stores a pipeline. Requires at least one CodeBuild action.
func (s *Store) CreateCodePipeline(accountID, region string, decl CodePipelineDeclaration) (CodePipelinePipeline, error) {
	decl.Name = strings.TrimSpace(decl.Name)
	if decl.Name == "" {
		return CodePipelinePipeline{}, fmt.Errorf("%w: name is required", ErrCodePipelineBadReq)
	}
	if len(decl.Stages) == 0 {
		return CodePipelinePipeline{}, fmt.Errorf("%w: stages required", ErrCodePipelineBadReq)
	}
	hasCodeBuild := false
	for _, st := range decl.Stages {
		for _, a := range st.Actions {
			if strings.EqualFold(a.ActionTypeID.Provider, "CodeBuild") {
				hasCodeBuild = true
				if strings.TrimSpace(a.Configuration["ProjectName"]) == "" {
					return CodePipelinePipeline{}, fmt.Errorf("%w: CodeBuild ProjectName required", ErrCodePipelineBadReq)
				}
			}
		}
	}
	if !hasCodeBuild {
		return CodePipelinePipeline{}, fmt.Errorf("%w: at least one CodeBuild action required", ErrCodePipelineBadReq)
	}
	raw, err := json.Marshal(decl)
	if err != nil {
		return CodePipelinePipeline{}, fmt.Errorf("marshal pipeline: %w", err)
	}
	arn := CodePipelineARN(region, accountID, decl.Name)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO codepipeline_pipelines (account_id, name, pipeline_arn, role_arn, definition, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, decl.Name, arn, decl.RoleARN, string(raw), now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return CodePipelinePipeline{}, ErrCodePipelineExists
		}
		return CodePipelinePipeline{}, fmt.Errorf("create pipeline: %w", err)
	}
	return CodePipelinePipeline{
		Name: decl.Name, PipelineARN: arn, RoleARN: decl.RoleARN,
		Definition: string(raw), CreatedAt: now, UpdatedAt: now,
	}, nil
}

// GetCodePipeline returns a pipeline by name.
func (s *Store) GetCodePipeline(accountID, name string) (CodePipelinePipeline, error) {
	var p CodePipelinePipeline
	err := s.db.QueryRow(
		`SELECT name, pipeline_arn, role_arn, definition, created_at, updated_at
		 FROM codepipeline_pipelines WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&p.Name, &p.PipelineARN, &p.RoleARN, &p.Definition, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CodePipelinePipeline{}, ErrCodePipelineNotFound
	}
	if err != nil {
		return CodePipelinePipeline{}, fmt.Errorf("get pipeline: %w", err)
	}
	return p, nil
}

// DeleteCodePipeline deletes a pipeline by name.
func (s *Store) DeleteCodePipeline(accountID, name string) error {
	res, err := s.db.Exec(`DELETE FROM codepipeline_pipelines WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete pipeline: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCodePipelineNotFound
	}
	return nil
}

// CodePipelineBuildRunner starts a nested CodeBuild for a ProjectName action.
// Nil keeps the lab stub that records ProjectName without calling StartBuild.
type CodePipelineBuildRunner func(projectName string) (buildID, status string, err error)

// StartCodePipelineExecution runs stages synchronously.
// When runBuild is non-nil, CodeBuild actions call it (nested StartBuild path).
// When runBuild is nil, CodeBuild actions succeed as stubs that record ProjectName.
func (s *Store) StartCodePipelineExecution(accountID, pipelineName string, runBuild CodePipelineBuildRunner) (CodePipelineExecution, error) {
	p, err := s.GetCodePipeline(accountID, pipelineName)
	if err != nil {
		return CodePipelineExecution{}, err
	}
	var decl CodePipelineDeclaration
	if err := json.Unmarshal([]byte(p.Definition), &decl); err != nil {
		return CodePipelineExecution{}, fmt.Errorf("parse pipeline: %w", err)
	}
	type actionState struct {
		ActionName string `json:"actionName"`
		Status     string `json:"status"`
		Project    string `json:"projectName,omitempty"`
		BuildID    string `json:"buildId,omitempty"`
	}
	type stageState struct {
		StageName    string        `json:"stageName"`
		Status       string        `json:"status"`
		ActionStates []actionState `json:"actionStates"`
	}
	stages := make([]stageState, 0, len(decl.Stages))
	overall := "Succeeded"
	for _, st := range decl.Stages {
		ss := stageState{StageName: st.Name, Status: "Succeeded"}
		for _, a := range st.Actions {
			as := actionState{ActionName: a.Name, Status: "Succeeded"}
			if strings.EqualFold(a.ActionTypeID.Provider, "CodeBuild") {
				as.Project = a.Configuration["ProjectName"]
				if runBuild != nil {
					buildID, status, buildErr := runBuild(as.Project)
					as.BuildID = buildID
					if buildErr != nil || status == "" {
						status = "Failed"
					}
					as.Status = status
					if status != "Succeeded" {
						ss.Status = "Failed"
						overall = "Failed"
					}
				}
			}
			ss.ActionStates = append(ss.ActionStates, as)
		}
		stages = append(stages, ss)
	}
	stageJSON, _ := json.Marshal(stages)
	execID := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO codepipeline_executions (account_id, pipeline_name, execution_id, status, start_time, stage_states)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, pipelineName, execID, overall, now, string(stageJSON),
	)
	if err != nil {
		return CodePipelineExecution{}, fmt.Errorf("start execution: %w", err)
	}
	return CodePipelineExecution{
		PipelineName: pipelineName, ExecutionID: execID, Status: overall,
		StartTime: now, StageStates: string(stageJSON),
	}, nil
}

// GetCodePipelineState returns the latest execution stage states for a pipeline.
func (s *Store) GetCodePipelineState(accountID, pipelineName string) (CodePipelineExecution, error) {
	if _, err := s.GetCodePipeline(accountID, pipelineName); err != nil {
		return CodePipelineExecution{}, err
	}
	var e CodePipelineExecution
	err := s.db.QueryRow(
		`SELECT pipeline_name, execution_id, status, start_time, stage_states
		 FROM codepipeline_executions WHERE account_id = ? AND pipeline_name = ?
		 ORDER BY start_time DESC LIMIT 1`,
		accountID, pipelineName,
	).Scan(&e.PipelineName, &e.ExecutionID, &e.Status, &e.StartTime, &e.StageStates)
	if errors.Is(err, sql.ErrNoRows) {
		return CodePipelineExecution{PipelineName: pipelineName, Status: "Idle", StageStates: "[]"}, nil
	}
	if err != nil {
		return CodePipelineExecution{}, fmt.Errorf("get pipeline state: %w", err)
	}
	return e, nil
}
