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
	// ErrSFNInvalidToken is returned when a task token is unknown or already consumed.
	ErrSFNInvalidToken = errors.New("InvalidToken")
)

const sfnTaskTokenSchema = `
CREATE TABLE IF NOT EXISTS sfn_task_tokens (
  task_token TEXT PRIMARY KEY,
  execution_arn TEXT NOT NULL,
  account_id TEXT NOT NULL,
  state_name TEXT NOT NULL,
  next_state TEXT NOT NULL DEFAULT '',
  end_state INTEGER NOT NULL DEFAULT 0,
  state_input TEXT NOT NULL,
  result_path TEXT NOT NULL DEFAULT '',
  definition TEXT NOT NULL,
  state_machine_arn TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sfn_task_tokens_exec ON sfn_task_tokens(execution_arn);
`

// sfnTaskWait holds pause state when a Task uses waitForTaskToken.
type sfnTaskWait struct {
	TaskToken  string
	StateName  string
	Next       string
	End        bool
	StateInput string
	ResultPath string
	Resource   string
	Parameters string
}

// EnsureSFNTaskTokenSchema creates the callback task-token table.
func EnsureSFNTaskTokenSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure sfn task token schema: db is nil")
	}
	if _, err := db.Exec(sfnTaskTokenSchema); err != nil {
		return fmt.Errorf("ensure sfn task token schema: %w", err)
	}
	return nil
}

// EnsureSFNTaskTokenSchema ensures the callback task-token table on an open store.
func (s *Store) EnsureSFNTaskTokenSchema() error {
	return EnsureSFNTaskTokenSchema(s.db)
}

func (s *Store) saveSFNTaskWait(accountID, executionARN, definition, stateMachineARN, roleARN string, wait *sfnTaskWait) error {
	if wait == nil || strings.TrimSpace(wait.TaskToken) == "" {
		return fmt.Errorf("save sfn task wait: task token is required")
	}
	end := 0
	if wait.End {
		end = 1
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO sfn_task_tokens (
			task_token, execution_arn, account_id, state_name, next_state, end_state,
			state_input, result_path, definition, state_machine_arn, role_arn, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wait.TaskToken, executionARN, accountID, wait.StateName, wait.Next, end,
		wait.StateInput, wait.ResultPath, definition, stateMachineARN, roleARN, now,
	)
	if err != nil {
		return fmt.Errorf("save sfn task wait: %w", err)
	}
	return nil
}

type sfnPendingTask struct {
	TaskToken       string
	ExecutionARN    string
	AccountID       string
	StateName       string
	Next            string
	End             bool
	StateInput      string
	ResultPath      string
	Definition      string
	StateMachineARN string
	RoleARN         string
}

func (s *Store) loadSFNPendingTask(taskToken string) (sfnPendingTask, error) {
	taskToken = strings.TrimSpace(taskToken)
	if taskToken == "" {
		return sfnPendingTask{}, ErrSFNInvalidToken
	}
	var p sfnPendingTask
	var end int
	err := s.db.QueryRow(
		`SELECT task_token, execution_arn, account_id, state_name, next_state, end_state,
			state_input, result_path, definition, state_machine_arn, role_arn
		 FROM sfn_task_tokens WHERE task_token = ?`,
		taskToken,
	).Scan(
		&p.TaskToken, &p.ExecutionARN, &p.AccountID, &p.StateName, &p.Next, &end,
		&p.StateInput, &p.ResultPath, &p.Definition, &p.StateMachineARN, &p.RoleARN,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return sfnPendingTask{}, ErrSFNInvalidToken
	}
	if err != nil {
		return sfnPendingTask{}, fmt.Errorf("load sfn pending task: %w", err)
	}
	p.End = end != 0
	return p, nil
}

func (s *Store) deleteSFNPendingTask(taskToken string) error {
	_, err := s.db.Exec(`DELETE FROM sfn_task_tokens WHERE task_token = ?`, taskToken)
	if err != nil {
		return fmt.Errorf("delete sfn pending task: %w", err)
	}
	return nil
}

func (s *Store) sfnTaskTokenExists(taskToken string) bool {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM sfn_task_tokens WHERE task_token = ?`, strings.TrimSpace(taskToken)).Scan(&one)
	return err == nil
}

// SFNSendTaskSuccess resumes a waitForTaskToken Task with the given output JSON.
func (s *Store) SFNSendTaskSuccess(taskToken, output string) error {
	pending, err := s.loadSFNPendingTask(taskToken)
	if err != nil {
		return err
	}
	if err := s.deleteSFNPendingTask(taskToken); err != nil {
		return err
	}
	exec, err := s.DescribeSFNExecution(pending.ExecutionARN)
	if err != nil {
		return err
	}
	if exec.Status != "RUNNING" {
		return ErrSFNInvalidToken
	}
	if strings.TrimSpace(output) == "" {
		output = "{}"
	}
	if !json.Valid([]byte(output)) {
		return fmt.Errorf("SendTaskSuccess: output must be JSON")
	}
	merged, err := sfnApplyResultPath(pending.StateInput, output, pending.ResultPath)
	if err != nil {
		return s.failSFNExecution(pending.ExecutionARN, "States.Runtime", err.Error(), []sfnHist{
			{Type: "TaskFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": err.Error()}},
			{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "States.Runtime", "cause": err.Error()}},
		})
	}
	hist := []sfnHist{
		{Type: "TaskSucceeded", DetailsMap: map[string]any{"output": merged}},
		{Type: "TaskStateExited", DetailsMap: map[string]any{"name": pending.StateName, "output": merged}},
	}
	if pending.End || strings.TrimSpace(pending.Next) == "" {
		hist = append(hist, sfnHist{Type: "ExecutionSucceeded", DetailsMap: map[string]any{"output": merged}})
		for _, h := range hist {
			_ = s.appendSFNHistory(pending.ExecutionARN, h.Type, h.DetailsMap)
		}
		return s.finalizeSFNExecution(pending.ExecutionARN, "SUCCEEDED", merged, "", "")
	}
	for _, h := range hist {
		_ = s.appendSFNHistory(pending.ExecutionARN, h.Type, h.DetailsMap)
	}
	return s.continueSFNExecution(pending, merged)
}

// SFNSendTaskFailure fails the execution waiting on the given task token.
func (s *Store) SFNSendTaskFailure(taskToken, errorCode, cause string) error {
	pending, err := s.loadSFNPendingTask(taskToken)
	if err != nil {
		return err
	}
	if err := s.deleteSFNPendingTask(taskToken); err != nil {
		return err
	}
	exec, err := s.DescribeSFNExecution(pending.ExecutionARN)
	if err != nil {
		return err
	}
	if exec.Status != "RUNNING" {
		return ErrSFNInvalidToken
	}
	if strings.TrimSpace(errorCode) == "" {
		errorCode = "States.TaskFailed"
	}
	hist := []sfnHist{
		{Type: "TaskFailed", DetailsMap: map[string]any{"error": errorCode, "cause": cause}},
		{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": errorCode, "cause": cause}},
	}
	return s.failSFNExecution(pending.ExecutionARN, errorCode, cause, hist)
}

// SFNSendTaskHeartbeat is a lab no-op success when the task token is still pending.
func (s *Store) SFNSendTaskHeartbeat(taskToken string) error {
	if !s.sfnTaskTokenExists(taskToken) {
		return ErrSFNInvalidToken
	}
	return nil
}

func (s *Store) continueSFNExecution(pending sfnPendingTask, currentInput string) error {
	var def sfnDef
	if err := json.Unmarshal([]byte(pending.Definition), &def); err != nil {
		return s.failSFNExecution(pending.ExecutionARN, "InvalidDefinition", err.Error(), []sfnHist{
			{Type: "ExecutionFailed", DetailsMap: map[string]any{"error": "InvalidDefinition", "cause": err.Error()}},
		})
	}
	wrapped := s.wrapSFNTaskInvoker(pending.AccountID, pending.RoleARN, pending.StateMachineARN, nil)
	output, status, errCode, cause, hist, wait := runSFNStates(def.States, pending.Next, currentInput, wrapped)
	for _, h := range hist {
		_ = s.appendSFNHistory(pending.ExecutionARN, h.Type, h.DetailsMap)
	}
	if wait != nil {
		if err := s.saveSFNTaskWait(pending.AccountID, pending.ExecutionARN, pending.Definition, pending.StateMachineARN, pending.RoleARN, wait); err != nil {
			return err
		}
		_, err := s.db.Exec(`UPDATE sfn_executions SET status = 'RUNNING' WHERE execution_arn = ?`, pending.ExecutionARN)
		return err
	}
	return s.finalizeSFNExecution(pending.ExecutionARN, status, output, errCode, cause)
}

func (s *Store) finalizeSFNExecution(executionARN, status, output, errCode, cause string) error {
	stop := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE sfn_executions SET status = ?, output = ?, error = ?, cause = ?, stop_date = ? WHERE execution_arn = ?`,
		status, output, errCode, cause, stop, executionARN,
	)
	if err != nil {
		return fmt.Errorf("finalize sfn execution: %w", err)
	}
	return nil
}

func (s *Store) failSFNExecution(executionARN, errCode, cause string, hist []sfnHist) error {
	for _, h := range hist {
		_ = s.appendSFNHistory(executionARN, h.Type, h.DetailsMap)
	}
	return s.finalizeSFNExecution(executionARN, "FAILED", "", errCode, cause)
}

func sfnIsWaitForTaskToken(st sfnState) bool {
	if strings.HasSuffix(strings.TrimSpace(st.Resource), ".waitForTaskToken") {
		return true
	}
	if len(st.Parameters) == 0 {
		return false
	}
	raw := string(st.Parameters)
	return strings.Contains(raw, "WaitForTaskToken") || strings.Contains(raw, "$$.Task.Token")
}

func sfnNewTaskToken() string {
	return uuid.NewString()
}

// sfnResolveParameters substitutes $$.Task.Token into Parameters (lab JSONPath subset).
func sfnResolveParameters(parameters json.RawMessage, stateInput, taskToken string) string {
	if len(parameters) == 0 {
		return stateInput
	}
	var root any
	if err := json.Unmarshal(parameters, &root); err != nil {
		return string(parameters)
	}
	resolved := sfnResolveParamValue(root, taskToken)
	b, err := json.Marshal(resolved)
	if err != nil {
		return string(parameters)
	}
	return string(b)
}

func sfnResolveParamValue(v any, taskToken string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if strings.HasSuffix(k, ".$") {
				path, ok := val.(string)
				if ok && strings.TrimSpace(path) == "$$.Task.Token" {
					out[strings.TrimSuffix(k, ".$")] = taskToken
					continue
				}
			}
			if s, ok := val.(string); ok && strings.TrimSpace(s) == "$$.Task.Token" {
				out[k] = taskToken
				continue
			}
			out[k] = sfnResolveParamValue(val, taskToken)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = sfnResolveParamValue(item, taskToken)
		}
		return out
	default:
		return v
	}
}
