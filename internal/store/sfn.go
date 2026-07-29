package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/google/uuid"
)

var (
	ErrSFNStateMachineExists   = errors.New("StateMachineAlreadyExists")
	ErrSFNStateMachineNotFound = errors.New("StateMachineDoesNotExist")
	ErrSFNExecutionNotFound    = errors.New("ExecutionDoesNotExist")
	ErrSFNInvalidDefinition    = errors.New("InvalidDefinition")
	ErrSFNRoleArnRequired      = errors.New("ValidationException: roleArn is required when definition contains Task")
)

const (
	DefaultSFNRegion = "us-east-1"
)

// SFNTaskInvoker invokes a Task resource (Lambda ARN) with JSON input and returns JSON output.
type SFNTaskInvoker func(resourceARN, inputJSON string) (outputJSON string, err error)

const sfnSchema = `
CREATE TABLE IF NOT EXISTS sfn_state_machines (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  state_machine_arn TEXT NOT NULL,
  definition TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS sfn_executions (
  account_id TEXT NOT NULL,
  execution_arn TEXT NOT NULL,
  state_machine_arn TEXT NOT NULL,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  input TEXT NOT NULL,
  output TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  cause TEXT NOT NULL DEFAULT '',
  start_date INTEGER NOT NULL,
  stop_date INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (execution_arn)
);
CREATE INDEX IF NOT EXISTS idx_sfn_exec_sm ON sfn_executions(account_id, state_machine_arn);
CREATE TABLE IF NOT EXISTS sfn_history (
  execution_arn TEXT NOT NULL,
  id INTEGER NOT NULL,
  type TEXT NOT NULL,
  timestamp INTEGER NOT NULL,
  details TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (execution_arn, id)
);
`

// SFNStateMachine is a state machine row.
type SFNStateMachine struct {
	Name            string
	StateMachineARN string
	Definition      string
	RoleARN         string
	CreationDate    int64
}

// SFNExecution is an execution row.
type SFNExecution struct {
	Name             string
	ExecutionARN     string
	StateMachineARN  string
	Status           string
	Input            string
	Output           string
	Error            string
	Cause            string
	StartDate        int64
	StopDate         int64
}

// SFNHistoryEvent is one GetExecutionHistory event.
type SFNHistoryEvent struct {
	ID        int64
	Type      string
	Timestamp int64
	Details   string // JSON blob of type-specific details
}

// EnsureSFNSchema creates Step Functions tables if missing.
func EnsureSFNSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure sfn schema: db is nil")
	}
	if _, err := db.Exec(sfnSchema); err != nil {
		return fmt.Errorf("ensure sfn schema: %w", err)
	}
	return nil
}

// EnsureSFNSchema ensures SFN tables on an open store.
func (s *Store) EnsureSFNSchema() error {
	return EnsureSFNSchema(s.db)
}

// SFNStateMachineARN builds arn:aws:states:REGION:ACCOUNT:stateMachine:NAME
func SFNStateMachineARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultSFNRegion
	}
	return fmt.Sprintf("arn:aws:states:%s:%s:stateMachine:%s", region, accountID, name)
}

// SFNExecutionARN builds arn:aws:states:REGION:ACCOUNT:execution:SM_NAME:EXEC_NAME
func SFNExecutionARN(region, accountID, smName, execName string) string {
	if region == "" {
		region = DefaultSFNRegion
	}
	return fmt.Sprintf("arn:aws:states:%s:%s:execution:%s:%s", region, accountID, smName, execName)
}

// CreateSFNStateMachine creates a state machine after validating ASL subset.
func (s *Store) CreateSFNStateMachine(accountID, region, name, definition, roleARN string) (SFNStateMachine, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return SFNStateMachine{}, fmt.Errorf("create state machine: name is required")
	}
	if err := validateSFNDefinition(definition); err != nil {
		return SFNStateMachine{}, err
	}
	roleARN = strings.TrimSpace(roleARN)
	if sfnDefinitionHasTask(definition) && roleARN == "" {
		return SFNStateMachine{}, ErrSFNRoleArnRequired
	}
	if _, err := s.getSFNStateMachineByName(accountID, name); err == nil {
		return SFNStateMachine{}, ErrSFNStateMachineExists
	} else if !errors.Is(err, ErrSFNStateMachineNotFound) {
		return SFNStateMachine{}, err
	}
	arn := SFNStateMachineARN(region, accountID, name)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO sfn_state_machines (account_id, name, state_machine_arn, definition, role_arn, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, name, arn, definition, roleARN, now,
	)
	if err != nil {
		return SFNStateMachine{}, fmt.Errorf("create state machine: %w", err)
	}
	return SFNStateMachine{Name: name, StateMachineARN: arn, Definition: definition, RoleARN: roleARN, CreationDate: now}, nil
}

// DeleteSFNStateMachine deletes a state machine by ARN or name.
func (s *Store) DeleteSFNStateMachine(accountID, stateMachineARN string) error {
	sm, err := s.GetSFNStateMachine(accountID, stateMachineARN)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM sfn_state_machines WHERE account_id = ? AND name = ?`, accountID, sm.Name)
	if err != nil {
		return fmt.Errorf("delete state machine: %w", err)
	}
	return nil
}

// GetSFNStateMachine loads by ARN or bare name.
func (s *Store) GetSFNStateMachine(accountID, arnOrName string) (SFNStateMachine, error) {
	arnOrName = strings.TrimSpace(arnOrName)
	if strings.Contains(arnOrName, ":stateMachine:") {
		var sm SFNStateMachine
		err := s.db.QueryRow(
			`SELECT name, state_machine_arn, definition, role_arn, created_at FROM sfn_state_machines
			 WHERE account_id = ? AND state_machine_arn = ?`,
			accountID, arnOrName,
		).Scan(&sm.Name, &sm.StateMachineARN, &sm.Definition, &sm.RoleARN, &sm.CreationDate)
		if errors.Is(err, sql.ErrNoRows) {
			return SFNStateMachine{}, ErrSFNStateMachineNotFound
		}
		if err != nil {
			return SFNStateMachine{}, fmt.Errorf("get state machine: %w", err)
		}
		return sm, nil
	}
	return s.getSFNStateMachineByName(accountID, arnOrName)
}

// ListSFNStateMachines lists state machines for an account.
func (s *Store) ListSFNStateMachines(accountID string) ([]SFNStateMachine, error) {
	rows, err := s.db.Query(
		`SELECT name, state_machine_arn, definition, role_arn, created_at FROM sfn_state_machines
		 WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list state machines: %w", err)
	}
	defer rows.Close()
	var out []SFNStateMachine
	for rows.Next() {
		var sm SFNStateMachine
		if err := rows.Scan(&sm.Name, &sm.StateMachineARN, &sm.Definition, &sm.RoleARN, &sm.CreationDate); err != nil {
			return nil, fmt.Errorf("list state machines: scan: %w", err)
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// SetSFNTaskInvoker registers an optional Lambda Task invoker used when EventBridge
// (or other store paths) start executions without a request-scoped callback.
func (s *Store) SetSFNTaskInvoker(fn SFNTaskInvoker) {
	s.sfnTaskMu.Lock()
	defer s.sfnTaskMu.Unlock()
	s.sfnTaskInvoker = fn
}

func (s *Store) getSFNTaskInvoker() SFNTaskInvoker {
	s.sfnTaskMu.Lock()
	defer s.sfnTaskMu.Unlock()
	return s.sfnTaskInvoker
}

// StartSFNExecution runs the ASL subset. Completes sync unless a Task pauses on waitForTaskToken
// (status stays RUNNING until SendTaskSuccess / SendTaskFailure).
func (s *Store) StartSFNExecution(accountID, region, stateMachineARN, name, input string, invoke SFNTaskInvoker) (SFNExecution, error) {
	sm, err := s.GetSFNStateMachine(accountID, stateMachineARN)
	if err != nil {
		return SFNExecution{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = uuid.NewString()
	}
	if strings.TrimSpace(input) == "" {
		input = "{}"
	}
	if !json.Valid([]byte(input)) {
		return SFNExecution{}, fmt.Errorf("start execution: input must be JSON")
	}
	execARN := SFNExecutionARN(region, accountID, sm.Name, name)
	start := time.Now().UTC().UnixMilli()
	exec := SFNExecution{
		Name: name, ExecutionARN: execARN, StateMachineARN: sm.StateMachineARN,
		Status: "RUNNING", Input: input, StartDate: start,
	}
	_, err = s.db.Exec(
		`INSERT INTO sfn_executions (account_id, execution_arn, state_machine_arn, name, status, input, start_date)
		 VALUES (?, ?, ?, ?, 'RUNNING', ?, ?)`,
		accountID, execARN, sm.StateMachineARN, name, input, start,
	)
	if err != nil {
		return SFNExecution{}, fmt.Errorf("start execution: insert: %w", err)
	}
	_ = s.appendSFNHistory(execARN, "ExecutionStarted", map[string]any{"input": input, "roleArn": sm.RoleARN})

	wrapped := s.wrapSFNTaskInvoker(accountID, sm.RoleARN, sm.StateMachineARN, invoke)
	output, status, errCode, cause, hist, wait := runSFNDefinition(sm.Definition, input, wrapped)
	for _, h := range hist {
		_ = s.appendSFNHistory(execARN, h.Type, h.DetailsMap)
	}
	if wait != nil {
		if err := s.saveSFNTaskWait(accountID, execARN, sm.Definition, sm.StateMachineARN, sm.RoleARN, wait); err != nil {
			return SFNExecution{}, err
		}
		exec.Status = "RUNNING"
		return exec, nil
	}
	stop := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`UPDATE sfn_executions SET status = ?, output = ?, error = ?, cause = ?, stop_date = ? WHERE execution_arn = ?`,
		status, output, errCode, cause, stop, execARN,
	)
	if err != nil {
		return SFNExecution{}, fmt.Errorf("start execution: finalize: %w", err)
	}
	exec.Status = status
	exec.Output = output
	exec.Error = errCode
	exec.Cause = cause
	exec.StopDate = stop
	return exec, nil
}

// DescribeSFNExecution loads an execution.
func (s *Store) DescribeSFNExecution(executionARN string) (SFNExecution, error) {
	var e SFNExecution
	err := s.db.QueryRow(
		`SELECT name, execution_arn, state_machine_arn, status, input, output, error, cause, start_date, stop_date
		 FROM sfn_executions WHERE execution_arn = ?`,
		executionARN,
	).Scan(&e.Name, &e.ExecutionARN, &e.StateMachineARN, &e.Status, &e.Input, &e.Output, &e.Error, &e.Cause, &e.StartDate, &e.StopDate)
	if errors.Is(err, sql.ErrNoRows) {
		return SFNExecution{}, ErrSFNExecutionNotFound
	}
	if err != nil {
		return SFNExecution{}, fmt.Errorf("describe execution: %w", err)
	}
	return e, nil
}

// GetSFNExecutionHistory returns history events in order.
func (s *Store) GetSFNExecutionHistory(executionARN string) ([]SFNHistoryEvent, error) {
	if _, err := s.DescribeSFNExecution(executionARN); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, type, timestamp, details FROM sfn_history WHERE execution_arn = ? ORDER BY id`,
		executionARN,
	)
	if err != nil {
		return nil, fmt.Errorf("get execution history: %w", err)
	}
	defer rows.Close()
	var out []SFNHistoryEvent
	for rows.Next() {
		var ev SFNHistoryEvent
		if err := rows.Scan(&ev.ID, &ev.Type, &ev.Timestamp, &ev.Details); err != nil {
			return nil, fmt.Errorf("get execution history: scan: %w", err)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Store) getSFNStateMachineByName(accountID, name string) (SFNStateMachine, error) {
	var sm SFNStateMachine
	err := s.db.QueryRow(
		`SELECT name, state_machine_arn, definition, role_arn, created_at FROM sfn_state_machines
		 WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&sm.Name, &sm.StateMachineARN, &sm.Definition, &sm.RoleARN, &sm.CreationDate)
	if errors.Is(err, sql.ErrNoRows) {
		return SFNStateMachine{}, ErrSFNStateMachineNotFound
	}
	if err != nil {
		return SFNStateMachine{}, fmt.Errorf("get state machine by name: %w", err)
	}
	return sm, nil
}

func (s *Store) appendSFNHistory(executionARN, typ string, details any) error {
	var max sql.NullInt64
	_ = s.db.QueryRow(`SELECT MAX(id) FROM sfn_history WHERE execution_arn = ?`, executionARN).Scan(&max)
	next := int64(1)
	if max.Valid {
		next = max.Int64 + 1
	}
	raw, _ := json.Marshal(details)
	_, err := s.db.Exec(
		`INSERT INTO sfn_history (execution_arn, id, type, timestamp, details) VALUES (?, ?, ?, ?, ?)`,
		executionARN, next, typ, time.Now().UTC().UnixMilli(), string(raw),
	)
	return err
}

type sfnHist struct {
	Type       string
	DetailsMap map[string]any
}

type sfnDef struct {
	Comment string                    `json:"Comment"`
	StartAt string                    `json:"StartAt"`
	States  map[string]json.RawMessage `json:"States"`
}

type sfnState struct {
	Type        string            `json:"Type"`
	Next        string            `json:"Next"`
	End         bool              `json:"End"`
	Resource    string            `json:"Resource"`
	Parameters  json.RawMessage   `json:"Parameters"`
	Result      json.RawMessage   `json:"Result"`
	ResultPath  string            `json:"ResultPath"`
	OutputPath  string            `json:"OutputPath"`
	InputPath   string            `json:"InputPath"`
	Error       string            `json:"Error"`
	Cause       string            `json:"Cause"`
	Choices     []sfnChoice       `json:"Choices"`
	Default     string            `json:"Default"`
	Seconds     *int              `json:"Seconds"`
	SecondsPath string            `json:"SecondsPath"`
	Branches    []json.RawMessage `json:"Branches"`
	ItemsPath   string            `json:"ItemsPath"`
	Iterator    json.RawMessage   `json:"Iterator"`
}

type sfnChoice struct {
	Variable            string   `json:"Variable"`
	StringEquals        *string  `json:"StringEquals"`
	StringGreaterThan   *string  `json:"StringGreaterThan"`
	StringLessThan      *string  `json:"StringLessThan"`
	NumericEquals       *float64 `json:"NumericEquals"`
	NumericGreaterThan  *float64 `json:"NumericGreaterThan"`
	NumericLessThan     *float64 `json:"NumericLessThan"`
	BooleanEquals       *bool    `json:"BooleanEquals"`
	IsPresent           *bool    `json:"IsPresent"`
	Next                string   `json:"Next"`
}

type sfnBranch struct {
	StartAt string                     `json:"StartAt"`
	States  map[string]json.RawMessage `json:"States"`
}

func validateSFNDefinition(definition string) error {
	var def sfnDef
	if err := json.Unmarshal([]byte(definition), &def); err != nil {
		return ErrSFNInvalidDefinition
	}
	if def.StartAt == "" || len(def.States) == 0 {
		return ErrSFNInvalidDefinition
	}
	if _, ok := def.States[def.StartAt]; !ok {
		return ErrSFNInvalidDefinition
	}
	return validateSFNStates(def.States)
}

func validateSFNStates(states map[string]json.RawMessage) error {
	for name, raw := range states {
		var st sfnState
		if err := json.Unmarshal(raw, &st); err != nil {
			return ErrSFNInvalidDefinition
		}
		switch st.Type {
		case "Pass", "Succeed", "Fail", "Task":
			if st.Type == "Task" && strings.TrimSpace(st.Resource) == "" {
				return fmt.Errorf("%w: Task %s missing Resource", ErrSFNInvalidDefinition, name)
			}
		case "Choice":
			if len(st.Choices) == 0 {
				return fmt.Errorf("%w: Choice %s missing Choices", ErrSFNInvalidDefinition, name)
			}
			for i, rawChoice := range mustChoiceRaws(raw) {
				if err := validateSFNChoiceRule(name, i, rawChoice); err != nil {
					return err
				}
			}
		case "Wait":
			if st.Seconds == nil && strings.TrimSpace(st.SecondsPath) == "" {
				return fmt.Errorf("%w: Wait %s requires Seconds or SecondsPath", ErrSFNInvalidDefinition, name)
			}
			if st.SecondsPath != "" {
				if _, ok := sfnTopLevelField(st.SecondsPath); !ok {
					return fmt.Errorf("%w: Wait %s SecondsPath must be $.field", ErrSFNInvalidDefinition, name)
				}
			}
		case "Parallel":
			if len(st.Branches) == 0 {
				return fmt.Errorf("%w: Parallel %s missing Branches", ErrSFNInvalidDefinition, name)
			}
			for bi, brRaw := range st.Branches {
				var br sfnBranch
				if err := json.Unmarshal(brRaw, &br); err != nil {
					return fmt.Errorf("%w: Parallel %s branch %d invalid", ErrSFNInvalidDefinition, name, bi)
				}
				if br.StartAt == "" || len(br.States) == 0 {
					return fmt.Errorf("%w: Parallel %s branch %d missing StartAt/States", ErrSFNInvalidDefinition, name, bi)
				}
				if _, ok := br.States[br.StartAt]; !ok {
					return fmt.Errorf("%w: Parallel %s branch %d StartAt unknown", ErrSFNInvalidDefinition, name, bi)
				}
				if err := validateSFNStates(br.States); err != nil {
					return err
				}
			}
		case "Map":
			if len(st.Iterator) == 0 {
				return fmt.Errorf("%w: Map %s missing Iterator", ErrSFNInvalidDefinition, name)
			}
			var it sfnBranch
			if err := json.Unmarshal(st.Iterator, &it); err != nil {
				return fmt.Errorf("%w: Map %s Iterator invalid", ErrSFNInvalidDefinition, name)
			}
			if it.StartAt == "" || len(it.States) == 0 {
				return fmt.Errorf("%w: Map %s Iterator missing StartAt/States", ErrSFNInvalidDefinition, name)
			}
			if _, ok := it.States[it.StartAt]; !ok {
				return fmt.Errorf("%w: Map %s Iterator StartAt unknown", ErrSFNInvalidDefinition, name)
			}
			if strings.TrimSpace(st.ItemsPath) != "" {
				if _, ok := sfnTopLevelField(st.ItemsPath); !ok {
					return fmt.Errorf("%w: Map %s ItemsPath must be $.field", ErrSFNInvalidDefinition, name)
				}
			}
			if err := validateSFNStates(it.States); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: unsupported state type %q in %s", ErrSFNInvalidDefinition, st.Type, name)
		}
	}
	return nil
}

func mustChoiceRaws(stateRaw json.RawMessage) []json.RawMessage {
	var wrapper struct {
		Choices []json.RawMessage `json:"Choices"`
	}
	_ = json.Unmarshal(stateRaw, &wrapper)
	return wrapper.Choices
}

func validateSFNChoiceRule(stateName string, index int, raw json.RawMessage) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return fmt.Errorf("%w: Choice %s rule %d invalid", ErrSFNInvalidDefinition, stateName, index)
	}
	allowed := map[string]bool{
		"Variable": true, "Next": true,
		"StringEquals": true, "StringGreaterThan": true, "StringLessThan": true,
		"NumericEquals": true, "NumericGreaterThan": true, "NumericLessThan": true,
		"BooleanEquals": true, "IsPresent": true,
	}
	var opCount int
	for k := range keys {
		if !allowed[k] {
			return fmt.Errorf("%w: Choice %s rule %d unknown operator %q", ErrSFNInvalidDefinition, stateName, index, k)
		}
		switch k {
		case "StringEquals", "StringGreaterThan", "StringLessThan",
			"NumericEquals", "NumericGreaterThan", "NumericLessThan",
			"BooleanEquals", "IsPresent":
			opCount++
		}
	}
	var c sfnChoice
	if err := json.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("%w: Choice %s rule %d invalid", ErrSFNInvalidDefinition, stateName, index)
	}
	if c.Next == "" {
		return fmt.Errorf("%w: Choice %s rule %d missing Next", ErrSFNInvalidDefinition, stateName, index)
	}
	if _, ok := sfnTopLevelField(c.Variable); !ok {
		return fmt.Errorf("%w: Choice %s rule %d Variable must be $.field", ErrSFNInvalidDefinition, stateName, index)
	}
	if opCount != 1 {
		return fmt.Errorf("%w: Choice %s rule %d requires exactly one comparison operator", ErrSFNInvalidDefinition, stateName, index)
	}
	return nil
}

func sfnTopLevelField(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "$.") {
		return "", false
	}
	field := path[2:]
	if field == "" || strings.ContainsAny(field, ".[") {
		return "", false
	}
	return field, true
}

func sfnDefinitionHasTask(definition string) bool {
	var def sfnDef
	if err := json.Unmarshal([]byte(definition), &def); err != nil {
		return false
	}
	return sfnStatesHaveTask(def.States)
}

func sfnStatesHaveTask(states map[string]json.RawMessage) bool {
	for _, raw := range states {
		var st sfnState
		if err := json.Unmarshal(raw, &st); err != nil {
			continue
		}
		switch st.Type {
		case "Task":
			return true
		case "Parallel":
			for _, brRaw := range st.Branches {
				var br sfnBranch
				if err := json.Unmarshal(brRaw, &br); err != nil {
					continue
				}
				if sfnStatesHaveTask(br.States) {
					return true
				}
			}
		case "Map":
			var it sfnBranch
			if err := json.Unmarshal(st.Iterator, &it); err != nil {
				continue
			}
			if sfnStatesHaveTask(it.States) {
				return true
			}
		}
	}
	return false
}

func runSFNDefinition(definition, input string, invoke SFNTaskInvoker) (output, status, errCode, cause string, hist []sfnHist, wait *sfnTaskWait) {
	var def sfnDef
	if err := json.Unmarshal([]byte(definition), &def); err != nil {
		return "", "FAILED", "InvalidDefinition", err.Error(), nil, nil
	}
	return runSFNStates(def.States, def.StartAt, input, invoke)
}

func runSFNStates(states map[string]json.RawMessage, startAt, input string, invoke SFNTaskInvoker) (output, status, errCode, cause string, hist []sfnHist, wait *sfnTaskWait) {
	current := input
	stateName := startAt
	for i := 0; i < 100; i++ {
		raw, ok := states[stateName]
		if !ok {
			return current, "FAILED", "States.Runtime", "Unknown state "+stateName, hist, nil
		}
		var st sfnState
		_ = json.Unmarshal(raw, &st)
		hist = append(hist, sfnHist{Type: st.Type + "StateEntered", DetailsMap: map[string]any{"name": stateName, "input": current}})

		switch st.Type {
		case "Pass":
			effective, err := sfnApplyInputPath(current, st.InputPath)
			if err != nil {
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": err.Error()}})
				return "", "FAILED", "States.Runtime", err.Error(), hist, nil
			}
			out := effective
			if len(st.Result) > 0 {
				out = string(st.Result)
			}
			out, err = sfnApplyResultPath(current, out, st.ResultPath)
			if err != nil {
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": err.Error()}})
				return "", "FAILED", "States.Runtime", err.Error(), hist, nil
			}
			hist = append(hist, sfnHist{Type: "PassStateExited", DetailsMap: map[string]any{"name": stateName, "output": out}})
			current = out
			if st.End || st.Next == "" {
				hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": current}})
				return current, "SUCCEEDED", "", "", hist, nil
			}
			stateName = st.Next
		case "Succeed":
			hist = append(hist, sfnHist{Type: "SucceedStateEntered", DetailsMap: map[string]any{"name": stateName}})
			hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": current}})
			return current, "SUCCEEDED", "", "", hist, nil
		case "Fail":
			errCode = st.Error
			if errCode == "" {
				errCode = "States.FAILED"
			}
			cause = st.Cause
			hist = append(hist, sfnHist{Type: "FailStateEntered", DetailsMap: map[string]any{"name": stateName, "error": errCode, "cause": cause}})
			hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": errCode, "cause": cause}})
			return "", "FAILED", errCode, cause, hist, nil
		case "Task":
			effective, err := sfnApplyInputPath(current, st.InputPath)
			if err != nil {
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": err.Error()}})
				return "", "FAILED", "States.Runtime", err.Error(), hist, nil
			}
			if sfnIsWaitForTaskToken(st) {
				token := sfnNewTaskToken()
				paramsJSON := sfnResolveParameters(st.Parameters, effective, token)
				hist = append(hist, sfnHist{Type: "TaskScheduled", DetailsMap: map[string]any{
					"resource": st.Resource, "parameters": paramsJSON, "taskToken": token,
				}})
				hist = append(hist, sfnHist{Type: "TaskStarted", DetailsMap: map[string]any{"taskToken": token}})
				return current, "RUNNING", "", "", hist, &sfnTaskWait{
					TaskToken:  token,
					StateName:  stateName,
					Next:       st.Next,
					End:        st.End || st.Next == "",
					StateInput: current,
					ResultPath: st.ResultPath,
					Resource:   st.Resource,
					Parameters: paramsJSON,
				}
			}
			if invoke == nil {
				hist = append(hist, sfnHist{Type: "TaskFailed", DetailsMap: map[string]any{"error": "States.TaskFailed", "cause": "Task invoker unavailable"}})
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.TaskFailed", "cause": "Task invoker unavailable"}})
				return "", "FAILED", "States.TaskFailed", "Task invoker unavailable", hist, nil
			}
			hist = append(hist, sfnHist{Type: "TaskScheduled", DetailsMap: map[string]any{"resource": st.Resource, "parameters": effective}})
			hist = append(hist, sfnHist{Type: "TaskStarted", DetailsMap: map[string]any{}})
			out, err := invoke(st.Resource, effective)
			if err != nil {
				cause = err.Error()
				hist = append(hist, sfnHist{Type: "TaskFailed", DetailsMap: map[string]any{"error": "States.TaskFailed", "cause": cause}})
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.TaskFailed", "cause": cause}})
				return "", "FAILED", "States.TaskFailed", cause, hist, nil
			}
			if out == "" {
				out = "{}"
			}
			out, err = sfnApplyResultPath(current, out, st.ResultPath)
			if err != nil {
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": err.Error()}})
				return "", "FAILED", "States.Runtime", err.Error(), hist, nil
			}
			hist = append(hist, sfnHist{Type: "TaskSucceeded", DetailsMap: map[string]any{"output": out}})
			hist = append(hist, sfnHist{Type: "TaskStateExited", DetailsMap: map[string]any{"name": stateName, "output": out}})
			current = out
			if st.End || st.Next == "" {
				hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": current}})
				return current, "SUCCEEDED", "", "", hist, nil
			}
			stateName = st.Next
		case "Choice":
			next, ok := sfnEvalChoice(st, current)
			if !ok {
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.NoChoiceMatched", "cause": "no Choice matched and no Default"}})
				return "", "FAILED", "States.NoChoiceMatched", "no Choice matched and no Default", hist, nil
			}
			hist = append(hist, sfnHist{Type: "ChoiceStateExited", DetailsMap: map[string]any{"name": stateName, "output": current, "next": next}})
			stateName = next
		case "Wait":
			secs, err := sfnWaitSeconds(st, current)
			if err != nil {
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": err.Error()}})
				return "", "FAILED", "States.Runtime", err.Error(), hist, nil
			}
			if secs > 0 {
				time.Sleep(time.Duration(secs) * time.Second)
			}
			hist = append(hist, sfnHist{Type: "WaitStateExited", DetailsMap: map[string]any{"name": stateName, "output": current}})
			if st.End || st.Next == "" {
				hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": current}})
				return current, "SUCCEEDED", "", "", hist, nil
			}
			stateName = st.Next
		case "Parallel":
			branchOutputs := make([]json.RawMessage, 0, len(st.Branches))
			for bi, brRaw := range st.Branches {
				var br sfnBranch
				if err := json.Unmarshal(brRaw, &br); err != nil {
					cause = fmt.Sprintf("Parallel branch %d invalid: %v", bi, err)
					hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": cause}})
					return "", "FAILED", "States.Runtime", cause, hist, nil
				}
				brDef, _ := json.Marshal(sfnDef{StartAt: br.StartAt, States: br.States})
				bout, bstatus, berr, bcause, bhist, bwait := runSFNDefinition(string(brDef), current, invoke)
				for _, h := range bhist {
					if h.Type == "ExecutionSucceeded" || h.Type == "ExecutionFailed" {
						continue
					}
					hist = append(hist, h)
				}
				if bwait != nil {
					cause = fmt.Sprintf("Parallel branch %d: waitForTaskToken is not supported inside Parallel", bi)
					hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": cause}})
					return "", "FAILED", "States.Runtime", cause, hist, nil
				}
				if bstatus != "SUCCEEDED" {
					if berr == "" {
						berr = "States.BranchFailed"
					}
					if bcause == "" {
						bcause = fmt.Sprintf("Parallel branch %d failed", bi)
					}
					hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": berr, "cause": bcause}})
					return "", "FAILED", berr, bcause, hist, nil
				}
				if bout == "" {
					bout = "{}"
				}
				branchOutputs = append(branchOutputs, json.RawMessage(bout))
			}
			merged, _ := json.Marshal(branchOutputs)
			out := string(merged)
			var err error
			out, err = sfnApplyResultPath(current, out, st.ResultPath)
			if err != nil {
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": err.Error()}})
				return "", "FAILED", "States.Runtime", err.Error(), hist, nil
			}
			hist = append(hist, sfnHist{Type: "ParallelStateExited", DetailsMap: map[string]any{"name": stateName, "output": out}})
			current = out
			if st.End || st.Next == "" {
				hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": current}})
				return current, "SUCCEEDED", "", "", hist, nil
			}
			stateName = st.Next
		case "Map":
			out, mapErr := sfnRunMap(st, current, invoke, &hist)
			if mapErr != nil {
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": mapErr.Error()}})
				return "", "FAILED", "States.Runtime", mapErr.Error(), hist, nil
			}
			hist = append(hist, sfnHist{Type: "MapStateExited", DetailsMap: map[string]any{"name": stateName, "output": out}})
			current = out
			if st.End || st.Next == "" {
				hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": current}})
				return current, "SUCCEEDED", "", "", hist, nil
			}
			stateName = st.Next
		default:
			return current, "FAILED", "States.Runtime", "unsupported type", hist, nil
		}
	}
	return current, "FAILED", "States.Runtime", "too many transitions", hist, nil
}

func sfnEvalChoice(st sfnState, inputJSON string) (next string, ok bool) {
	var obj map[string]any
	_ = json.Unmarshal([]byte(inputJSON), &obj)
	for _, c := range st.Choices {
		field, pathOK := sfnTopLevelField(c.Variable)
		if !pathOK {
			continue
		}
		val, present := obj[field]
		matched := false
		switch {
		case c.IsPresent != nil:
			matched = present == *c.IsPresent
		case !present:
			continue
		case c.StringEquals != nil:
			s, isStr := val.(string)
			matched = isStr && s == *c.StringEquals
		case c.StringGreaterThan != nil:
			s, isStr := val.(string)
			matched = isStr && s > *c.StringGreaterThan
		case c.StringLessThan != nil:
			s, isStr := val.(string)
			matched = isStr && s < *c.StringLessThan
		case c.NumericEquals != nil:
			n, isNum := sfnAsFloat(val)
			matched = isNum && n == *c.NumericEquals
		case c.NumericGreaterThan != nil:
			n, isNum := sfnAsFloat(val)
			matched = isNum && n > *c.NumericGreaterThan
		case c.NumericLessThan != nil:
			n, isNum := sfnAsFloat(val)
			matched = isNum && n < *c.NumericLessThan
		case c.BooleanEquals != nil:
			b, isBool := val.(bool)
			matched = isBool && b == *c.BooleanEquals
		}
		if matched {
			return c.Next, true
		}
	}
	if st.Default != "" {
		return st.Default, true
	}
	return "", false
}

// sfnApplyInputPath returns the effective state input. Empty path keeps raw input.
// "$" selects the whole input. "$.field" selects a top-level field (JSON-encoded).
func sfnApplyInputPath(inputJSON, inputPath string) (string, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" || inputPath == "$" {
		return inputJSON, nil
	}
	field, ok := sfnTopLevelField(inputPath)
	if !ok {
		return "", fmt.Errorf("InputPath must be $.field")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &obj); err != nil {
		return "", fmt.Errorf("InputPath input is not a JSON object")
	}
	v, present := obj[field]
	if !present {
		return "null", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// sfnApplyResultPath merges result into input. Empty path replaces input with result.
// "$.field" sets that top-level field on a copy of the original input object.
func sfnApplyResultPath(inputJSON, resultJSON, resultPath string) (string, error) {
	resultPath = strings.TrimSpace(resultPath)
	if resultPath == "" || resultPath == "$" {
		return resultJSON, nil
	}
	field, ok := sfnTopLevelField(resultPath)
	if !ok {
		return "", fmt.Errorf("ResultPath must be $.field")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &obj); err != nil {
		return "", fmt.Errorf("ResultPath input is not a JSON object")
	}
	var result any
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return "", fmt.Errorf("ResultPath result is not valid JSON")
	}
	obj[field] = result
	b, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func sfnRunMap(st sfnState, inputJSON string, invoke SFNTaskInvoker, hist *[]sfnHist) (string, error) {
	var it sfnBranch
	if err := json.Unmarshal(st.Iterator, &it); err != nil {
		return "", fmt.Errorf("Map Iterator invalid: %w", err)
	}
	itemsJSON := inputJSON
	if strings.TrimSpace(st.ItemsPath) != "" {
		var err error
		itemsJSON, err = sfnApplyInputPath(inputJSON, st.ItemsPath)
		if err != nil {
			return "", err
		}
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
		return "", fmt.Errorf("Map ItemsPath must select a JSON array")
	}
	outputs := make([]json.RawMessage, 0, len(items))
	for i, item := range items {
		itemInput := string(item)
		itDef, _ := json.Marshal(sfnDef{StartAt: it.StartAt, States: it.States})
		bout, bstatus, berr, bcause, bhist, bwait := runSFNDefinition(string(itDef), itemInput, invoke)
		for _, h := range bhist {
			if h.Type == "ExecutionSucceeded" || h.Type == "ExecutionFailed" {
				continue
			}
			*hist = append(*hist, h)
		}
		if bwait != nil {
			return "", fmt.Errorf("Map item %d: waitForTaskToken is not supported inside Map", i)
		}
		if bstatus != "SUCCEEDED" {
			if bcause == "" {
				bcause = berr
			}
			return "", fmt.Errorf("Map item %d failed: %s", i, bcause)
		}
		if bout == "" {
			bout = "{}"
		}
		outputs = append(outputs, json.RawMessage(bout))
	}
	merged, err := json.Marshal(outputs)
	if err != nil {
		return "", err
	}
	return sfnApplyResultPath(inputJSON, string(merged), st.ResultPath)
}

func sfnAsFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

const sfnWaitSecondsMax = 5

func sfnWaitSeconds(st sfnState, inputJSON string) (int, error) {
	var secs int
	switch {
	case st.Seconds != nil:
		secs = *st.Seconds
	case strings.TrimSpace(st.SecondsPath) != "":
		field, ok := sfnTopLevelField(st.SecondsPath)
		if !ok {
			return 0, fmt.Errorf("SecondsPath must be $.field")
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(inputJSON), &obj); err != nil {
			return 0, fmt.Errorf("SecondsPath input is not a JSON object")
		}
		n, ok := sfnAsFloat(obj[field])
		if !ok {
			return 0, fmt.Errorf("SecondsPath field %q is not numeric", field)
		}
		secs = int(n)
	default:
		return 0, fmt.Errorf("Wait requires Seconds or SecondsPath")
	}
	if secs < 0 {
		secs = 0
	}
	if secs > sfnWaitSecondsMax {
		secs = sfnWaitSecondsMax
	}
	return secs, nil
}

// ParseLambdaARNFromSFNResource extracts function name from a Lambda ARN or bare name.
func ParseLambdaARNFromSFNResource(resource string) (accountID, functionName string, ok bool) {
	resource = strings.TrimSpace(resource)
	// arn:aws:lambda:region:account:function:name
	if strings.HasPrefix(resource, "arn:aws:lambda:") {
		parts := strings.Split(resource, ":")
		if len(parts) >= 7 && parts[5] == "function" {
			return parts[4], parts[6], true
		}
		return "", "", false
	}
	return "", resource, resource != ""
}

func (s *Store) wrapSFNTaskInvoker(accountID, roleARN, stateMachineARN string, invoke SFNTaskInvoker) SFNTaskInvoker {
	return func(resourceARN, inputJSON string) (string, error) {
		if out, handled, err := s.sfnInvokeBuiltInTask(accountID, roleARN, stateMachineARN, resourceARN, inputJSON); handled {
			return out, err
		}
		if fnARN, ok := sfnLambdaTaskARN(accountID, resourceARN); ok {
			if !s.sfnTaskDeliveryAuthorized(accountID, roleARN, stateMachineARN, actionLambdaInvokeFunction, fnARN) {
				return "", fmt.Errorf("not authorized to invoke %s via state machine role", fnARN)
			}
		}
		if invoke != nil {
			return invoke(resourceARN, inputJSON)
		}
		if fallback := s.getSFNTaskInvoker(); fallback != nil {
			return fallback(resourceARN, inputJSON)
		}
		return "", fmt.Errorf("Task invoker unavailable")
	}
}

func sfnLambdaTaskARN(accountID, resourceARN string) (string, bool) {
	acct, name, ok := ParseLambdaARNFromSFNResource(resourceARN)
	if !ok || name == "" {
		return "", false
	}
	if acct == "" {
		acct = accountID
	}
	if strings.HasPrefix(strings.TrimSpace(resourceARN), "arn:aws:lambda:") {
		return strings.TrimSpace(resourceARN), true
	}
	return fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", DefaultLambdaRegion, acct, name), true
}

// sfnTaskDeliveryAuthorized authorizes Task delivery via the state machine RoleArn
// session, or (when RoleArn empty) a target resource policy Allow for states.amazonaws.com.
// Foreign SQS/Lambda/SNS targets require RoleArn session Allow AND destination resource policy.
func (s *Store) sfnTaskDeliveryAuthorized(accountID, roleARN, stateMachineARN, action, targetARN string) bool {
	return s.deliveryAuthorizedRoleAndResource(
		accountID, roleARN, action, targetARN, authz.ServicePrincipalStates, stateMachineARN, "sfn-task", DefaultSFNRegion,
	)
}

// sfnBusPutEventsDualEval requires bus resource-policy dual-eval after RoleArn Allow.
func (s *Store) sfnBusPutEventsDualEval(accountID, roleARN, stateMachineARN, busAccount, busName string) bool {
	bus, err := s.DescribeEventBus(busAccount, busName)
	if err != nil {
		return false
	}
	roleARN = strings.TrimSpace(roleARN)
	if roleARN == "" {
		return authz.EventTargetResourcePolicyAllows(
			bus.Policy, actionEventsPutEvents, bus.ARN, authz.ServicePrincipalStates, busAccount,
			authz.DeliverySourceConditionKeys(stateMachineARN, ""),
		)
	}
	roleAccountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok || roleAccountID != accountID {
		return false
	}
	docs, err := s.identityPolicyDocsForRoleARN(roleARN)
	if err != nil {
		return false
	}
	principal := identity.RoleSessionPrincipal(accountID, roleName, "sfn-task", "ASIATEMP")
	return EvaluateEventBusPutEvents(
		principal, docs, bus, DefaultSFNRegion, authz.DeliverySourceConditionKeys(stateMachineARN, ""),
	) == authz.Allow
}

// sfnInvokeBuiltInTask handles lab Task resources that do not need nested Lambda compute:
// SQS SendMessage, SNS Publish, and EventBridge PutEvents (bus ARN or name).
func (s *Store) sfnInvokeBuiltInTask(accountID, roleARN, stateMachineARN, resourceARN, inputJSON string) (output string, handled bool, err error) {
	resourceARN = strings.TrimSpace(resourceARN)
	switch {
	case strings.HasPrefix(resourceARN, "arn:aws:sqs:"):
		if !s.sfnTaskDeliveryAuthorized(accountID, roleARN, stateMachineARN, actionSQSSendMessage, resourceARN) {
			return "", true, fmt.Errorf("not authorized to SendMessage via state machine role")
		}
		queueAccount, qName, qerr := s.resolveSQSEndpoint(accountID, resourceARN)
		if qerr != nil {
			return "", true, qerr
		}
		if _, err := s.SendMessage(queueAccount, qName, []byte(inputJSON), false, nil, "", nil); err != nil {
			return "", true, err
		}
		return `{"ok":true}`, true, nil
	case strings.HasPrefix(resourceARN, "arn:aws:sns:"):
		if !s.sfnTaskDeliveryAuthorized(accountID, roleARN, stateMachineARN, actionSNSPublish, resourceARN) {
			return "", true, fmt.Errorf("not authorized to Publish via state machine role")
		}
		topic, terr := s.GetTopicByARN(resourceARN)
		if terr != nil {
			return "", true, terr
		}
		if _, err := s.Publish(topic.AccountID, topic.TopicName, inputJSON, "", nil); err != nil {
			return "", true, err
		}
		return `{"ok":true}`, true, nil
	case strings.HasPrefix(resourceARN, "arn:aws:events:") && strings.Contains(resourceARN, ":event-bus/"):
		if !s.sfnTaskDeliveryAuthorized(accountID, roleARN, stateMachineARN, actionEventsPutEvents, resourceARN) {
			return "", true, fmt.Errorf("not authorized to PutEvents via state machine role")
		}
		busName := resourceARN[strings.LastIndex(resourceARN, "/")+1:]
		detail := inputJSON
		if strings.TrimSpace(detail) == "" {
			detail = "{}"
		}
		busAccount := accountID
		if owner := resourceOwnerAccountFromARN(resourceARN); owner != "" {
			busAccount = owner
		}
		if !s.sfnBusPutEventsDualEval(accountID, roleARN, stateMachineARN, busAccount, busName) {
			return "", true, fmt.Errorf("not authorized to PutEvents on event bus policy")
		}
		result, perr := s.PutEvents(busAccount, []PutEventsEntry{{
			Source:       "noctaxris.sfn",
			DetailType:   "StepFunctionsTask",
			Detail:       detail,
			EventBusName: busName,
		}})
		if perr != nil {
			return "", true, perr
		}
		if result.FailedEntryCount > 0 {
			return "", true, fmt.Errorf("PutEvents failed")
		}
		eventID := ""
		if len(result.Entries) > 0 {
			eventID = result.Entries[0].EventID
		}
		out, _ := json.Marshal(map[string]any{"Entries": []map[string]string{{"EventId": eventID}}})
		return string(out), true, nil
	default:
		return "", false, nil
	}
}
