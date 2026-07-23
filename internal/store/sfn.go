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
	ErrSFNRoleArnRequired = errors.New("ValidationException: roleArn is required when definition contains Task")
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

// StartSFNExecution runs the ASL subset to completion (sync lab).
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
	output, status, errCode, cause, hist := runSFNDefinition(sm.Definition, input, wrapped)
	for _, h := range hist {
		_ = s.appendSFNHistory(execARN, h.Type, h.DetailsMap)
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
	Type       string          `json:"Type"`
	Next       string          `json:"Next"`
	End        bool            `json:"End"`
	Resource   string          `json:"Resource"`
	Result     json.RawMessage `json:"Result"`
	ResultPath string          `json:"ResultPath"`
	OutputPath string          `json:"OutputPath"`
	InputPath  string          `json:"InputPath"`
	Error      string          `json:"Error"`
	Cause      string          `json:"Cause"`
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
	for name, raw := range def.States {
		var st sfnState
		if err := json.Unmarshal(raw, &st); err != nil {
			return ErrSFNInvalidDefinition
		}
		switch st.Type {
		case "Pass", "Succeed", "Fail", "Task":
		default:
			return fmt.Errorf("%w: unsupported state type %q in %s", ErrSFNInvalidDefinition, st.Type, name)
		}
		if st.Type == "Task" && strings.TrimSpace(st.Resource) == "" {
			return fmt.Errorf("%w: Task %s missing Resource", ErrSFNInvalidDefinition, name)
		}
	}
	return nil
}

func sfnDefinitionHasTask(definition string) bool {
	var def sfnDef
	if err := json.Unmarshal([]byte(definition), &def); err != nil {
		return false
	}
	for _, raw := range def.States {
		var st sfnState
		if err := json.Unmarshal(raw, &st); err != nil {
			continue
		}
		if st.Type == "Task" {
			return true
		}
	}
	return false
}

func runSFNDefinition(definition, input string, invoke SFNTaskInvoker) (output, status, errCode, cause string, hist []sfnHist) {
	var def sfnDef
	if err := json.Unmarshal([]byte(definition), &def); err != nil {
		return "", "FAILED", "InvalidDefinition", err.Error(), nil
	}
	current := input
	stateName := def.StartAt
	for i := 0; i < 100; i++ {
		raw, ok := def.States[stateName]
		if !ok {
			return current, "FAILED", "States.Runtime", "Unknown state "+stateName, hist
		}
		var st sfnState
		_ = json.Unmarshal(raw, &st)
		hist = append(hist, sfnHist{Type: "TaskStateEntered", DetailsMap: map[string]any{"name": stateName, "input": current}})
		// Use PassStateEntered for Pass etc. Keep simple: StateEntered naming lite.
		hist[len(hist)-1].Type = st.Type + "StateEntered"

		switch st.Type {
		case "Pass":
			out := current
			if len(st.Result) > 0 {
				out = string(st.Result)
			}
			hist = append(hist, sfnHist{Type: "PassStateExited", DetailsMap: map[string]any{"name": stateName, "output": out}})
			current = out
			if st.End || st.Next == "" {
				hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": current}})
				return current, "SUCCEEDED", "", "", hist
			}
			stateName = st.Next
		case "Succeed":
			hist = append(hist, sfnHist{Type: "SucceedStateEntered", DetailsMap: map[string]any{"name": stateName}})
			hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": current}})
			return current, "SUCCEEDED", "", "", hist
		case "Fail":
			errCode = st.Error
			if errCode == "" {
				errCode = "States.FAILED"
			}
			cause = st.Cause
			hist = append(hist, sfnHist{Type: "FailStateEntered", DetailsMap: map[string]any{"name": stateName, "error": errCode, "cause": cause}})
			hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": errCode, "cause": cause}})
			return "", "FAILED", errCode, cause, hist
		case "Task":
			if invoke == nil {
				hist = append(hist, sfnHist{Type: "TaskFailed", DetailsMap: map[string]any{"error": "States.TaskFailed", "cause": "Task invoker unavailable"}})
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.TaskFailed", "cause": "Task invoker unavailable"}})
				return "", "FAILED", "States.TaskFailed", "Task invoker unavailable", hist
			}
			hist = append(hist, sfnHist{Type: "TaskScheduled", DetailsMap: map[string]any{"resource": st.Resource, "parameters": current}})
			hist = append(hist, sfnHist{Type: "TaskStarted", DetailsMap: map[string]any{}})
			out, err := invoke(st.Resource, current)
			if err != nil {
				cause = err.Error()
				hist = append(hist, sfnHist{Type: "TaskFailed", DetailsMap: map[string]any{"error": "States.TaskFailed", "cause": cause}})
				hist = append(hist, sfnHist{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.TaskFailed", "cause": cause}})
				return "", "FAILED", "States.TaskFailed", cause, hist
			}
			if out == "" {
				out = "{}"
			}
			hist = append(hist, sfnHist{Type: "TaskSucceeded", DetailsMap: map[string]any{"output": out}})
			hist = append(hist, sfnHist{Type: "TaskStateExited", DetailsMap: map[string]any{"name": stateName, "output": out}})
			current = out
			if st.End || st.Next == "" {
				hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": current}})
				return current, "SUCCEEDED", "", "", hist
			}
			stateName = st.Next
		default:
			return current, "FAILED", "States.Runtime", "unsupported type", hist
		}
	}
	return current, "FAILED", "States.Runtime", "too many transitions", hist
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
