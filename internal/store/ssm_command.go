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

const (
	SSMDocumentRunShellScript = "AWS-RunShellScript"

	SSMCommandStatusPending    = "Pending"
	SSMCommandStatusInProgress = "InProgress"
	SSMCommandStatusSuccess    = "Success"
	SSMCommandStatusFailed     = "Failed"
	SSMCommandStatusTimedOut   = "TimedOut"

	SSMDefaultTimeoutSeconds = 3600
	SSMMinTimeoutSeconds     = 30

	SSMMaxStdoutChars = 24000
	SSMMaxStderrChars = 8000
)

var (
	ErrSSMInvocationNotFound = errors.New("InvocationDoesNotExist")
	ErrSSMCommandNotFound    = errors.New("InvalidCommandId")
)

const ssmCommandSchema = `
CREATE TABLE IF NOT EXISTS ssm_commands (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  command_id TEXT NOT NULL,
  document_name TEXT NOT NULL,
  document_version TEXT NOT NULL DEFAULT '$DEFAULT',
  comment TEXT NOT NULL DEFAULT '',
  parameters_json TEXT NOT NULL DEFAULT '{}',
  instance_ids_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL,
  status_details TEXT NOT NULL,
  timeout_seconds INTEGER NOT NULL,
  target_count INTEGER NOT NULL DEFAULT 0,
  completed_count INTEGER NOT NULL DEFAULT 0,
  error_count INTEGER NOT NULL DEFAULT 0,
  requested_at TEXT NOT NULL,
  expires_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, region, command_id)
);

CREATE TABLE IF NOT EXISTS ssm_command_invocations (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  command_id TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  document_name TEXT NOT NULL,
  document_version TEXT NOT NULL DEFAULT '$DEFAULT',
  comment TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  status_details TEXT NOT NULL,
  stdout TEXT NOT NULL DEFAULT '',
  stderr TEXT NOT NULL DEFAULT '',
  response_code INTEGER NOT NULL DEFAULT -1,
  requested_at TEXT NOT NULL,
  execution_start TEXT NOT NULL DEFAULT '',
  execution_end TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, region, command_id, instance_id)
);
`

// SSMCommand is a persisted SendCommand row.
type SSMCommand struct {
	CommandID       string
	DocumentName    string
	DocumentVersion string
	Comment         string
	Parameters      map[string][]string
	InstanceIDs     []string
	Status          string
	StatusDetails   string
	TimeoutSeconds  int
	TargetCount     int
	CompletedCount  int
	ErrorCount      int
	RequestedAt     string
	ExpiresAt       string
	Region          string
}

// SSMCommandInvocation is a per-instance invocation row.
type SSMCommandInvocation struct {
	CommandID       string
	InstanceID      string
	DocumentName    string
	DocumentVersion string
	Comment         string
	Status          string
	StatusDetails   string
	Stdout          string
	Stderr          string
	ResponseCode    int
	RequestedAt     string
	ExecutionStart  string
	ExecutionEnd    string
	Region          string
}

// EnsureSSMCommandSchema creates Run Command tables if missing.
func EnsureSSMCommandSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure ssm command schema: db is nil")
	}
	if _, err := db.Exec(ssmCommandSchema); err != nil {
		return fmt.Errorf("ensure ssm command schema: %w", err)
	}
	return nil
}

// TruncateSSMOutput caps stdout/stderr to AWS-shaped limits.
func TruncateSSMOutput(s string, max int) string {
	if max < 1 || len(s) <= max {
		return s
	}
	return s[:max]
}

// NewSSMCommandID returns a UUID command id.
func NewSSMCommandID() string {
	return uuid.NewString()
}

// CreateSSMCommand inserts a command and one invocation per instance id.
func (s *Store) CreateSSMCommand(
	accountID, region string,
	documentName, documentVersion, comment string,
	parameters map[string][]string,
	instanceIDs []string,
	timeoutSeconds int,
) (SSMCommand, []SSMCommandInvocation, error) {
	if strings.TrimSpace(accountID) == "" {
		return SSMCommand{}, nil, fmt.Errorf("create ssm command: account id required")
	}
	if region == "" {
		region = DefaultSSMRegion
	}
	documentName = strings.TrimSpace(documentName)
	if documentName == "" {
		return SSMCommand{}, nil, fmt.Errorf("ValidationException: DocumentName is required")
	}
	if len(instanceIDs) == 0 {
		return SSMCommand{}, nil, fmt.Errorf("InvalidInstanceId: At least one InstanceId is required")
	}
	if timeoutSeconds < SSMMinTimeoutSeconds {
		return SSMCommand{}, nil, fmt.Errorf(
			"ValidationException: 1 validation error detected: Value '%d' at 'timeoutSeconds' failed to satisfy constraint: Member must have value greater than or equal to %d",
			timeoutSeconds, SSMMinTimeoutSeconds,
		)
	}
	if documentVersion == "" {
		documentVersion = "$DEFAULT"
	}
	if parameters == nil {
		parameters = map[string][]string{}
	}

	commandID := NewSSMCommandID()
	now := nowRFC3339()
	expires := ""
	if t, err := time.Parse(time.RFC3339, now); err == nil {
		expires = t.Add(time.Duration(timeoutSeconds) * time.Second).UTC().Format(time.RFC3339)
	}

	paramsJSON, err := json.Marshal(parameters)
	if err != nil {
		return SSMCommand{}, nil, fmt.Errorf("create ssm command: parameters: %w", err)
	}
	idsJSON, err := json.Marshal(instanceIDs)
	if err != nil {
		return SSMCommand{}, nil, fmt.Errorf("create ssm command: instance ids: %w", err)
	}

	cmd := SSMCommand{
		CommandID:       commandID,
		DocumentName:    documentName,
		DocumentVersion: documentVersion,
		Comment:         comment,
		Parameters:      parameters,
		InstanceIDs:     append([]string(nil), instanceIDs...),
		Status:          SSMCommandStatusInProgress,
		StatusDetails:   SSMCommandStatusInProgress,
		TimeoutSeconds:  timeoutSeconds,
		TargetCount:     len(instanceIDs),
		RequestedAt:     now,
		ExpiresAt:       expires,
		Region:          region,
	}

	tx, err := s.db.Begin()
	if err != nil {
		return SSMCommand{}, nil, fmt.Errorf("create ssm command: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(
		`INSERT INTO ssm_commands (
			account_id, region, command_id, document_name, document_version, comment,
			parameters_json, instance_ids_json, status, status_details, timeout_seconds,
			target_count, completed_count, error_count, requested_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?)`,
		accountID, region, commandID, documentName, documentVersion, comment,
		string(paramsJSON), string(idsJSON), cmd.Status, cmd.StatusDetails, timeoutSeconds,
		cmd.TargetCount, now, expires,
	)
	if err != nil {
		return SSMCommand{}, nil, fmt.Errorf("create ssm command: insert: %w", err)
	}

	invocations := make([]SSMCommandInvocation, 0, len(instanceIDs))
	for _, instanceID := range instanceIDs {
		inv := SSMCommandInvocation{
			CommandID:       commandID,
			InstanceID:      instanceID,
			DocumentName:    documentName,
			DocumentVersion: documentVersion,
			Comment:         comment,
			Status:          SSMCommandStatusPending,
			StatusDetails:   SSMCommandStatusPending,
			ResponseCode:    -1,
			RequestedAt:     now,
			Region:          region,
		}
		_, err = tx.Exec(
			`INSERT INTO ssm_command_invocations (
				account_id, region, command_id, instance_id, document_name, document_version,
				comment, status, status_details, stdout, stderr, response_code,
				requested_at, execution_start, execution_end
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', -1, ?, '', '')`,
			accountID, region, commandID, instanceID, documentName, documentVersion,
			comment, inv.Status, inv.StatusDetails, now,
		)
		if err != nil {
			return SSMCommand{}, nil, fmt.Errorf("create ssm command: invocation %s: %w", instanceID, err)
		}
		invocations = append(invocations, inv)
	}

	if err := tx.Commit(); err != nil {
		return SSMCommand{}, nil, fmt.Errorf("create ssm command: commit: %w", err)
	}
	return cmd, invocations, nil
}

// GetSSMCommand returns a command by id.
func (s *Store) GetSSMCommand(accountID, region, commandID string) (SSMCommand, error) {
	if region == "" {
		region = DefaultSSMRegion
	}
	var (
		paramsJSON, idsJSON string
		cmd                 SSMCommand
	)
	err := s.db.QueryRow(
		`SELECT command_id, document_name, document_version, comment, parameters_json, instance_ids_json,
			status, status_details, timeout_seconds, target_count, completed_count, error_count,
			requested_at, expires_at
		 FROM ssm_commands WHERE account_id = ? AND region = ? AND command_id = ?`,
		accountID, region, commandID,
	).Scan(
		&cmd.CommandID, &cmd.DocumentName, &cmd.DocumentVersion, &cmd.Comment, &paramsJSON, &idsJSON,
		&cmd.Status, &cmd.StatusDetails, &cmd.TimeoutSeconds, &cmd.TargetCount, &cmd.CompletedCount, &cmd.ErrorCount,
		&cmd.RequestedAt, &cmd.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SSMCommand{}, ErrSSMCommandNotFound
		}
		return SSMCommand{}, fmt.Errorf("get ssm command: %w", err)
	}
	cmd.Region = region
	if err := json.Unmarshal([]byte(paramsJSON), &cmd.Parameters); err != nil {
		return SSMCommand{}, fmt.Errorf("get ssm command: parameters: %w", err)
	}
	if cmd.Parameters == nil {
		cmd.Parameters = map[string][]string{}
	}
	if err := json.Unmarshal([]byte(idsJSON), &cmd.InstanceIDs); err != nil {
		return SSMCommand{}, fmt.Errorf("get ssm command: instance ids: %w", err)
	}
	if cmd.InstanceIDs == nil {
		cmd.InstanceIDs = []string{}
	}
	return cmd, nil
}

// GetSSMCommandInvocation returns one invocation.
func (s *Store) GetSSMCommandInvocation(accountID, region, commandID, instanceID string) (SSMCommandInvocation, error) {
	if region == "" {
		region = DefaultSSMRegion
	}
	var inv SSMCommandInvocation
	err := s.db.QueryRow(
		`SELECT command_id, instance_id, document_name, document_version, comment,
			status, status_details, stdout, stderr, response_code,
			requested_at, execution_start, execution_end
		 FROM ssm_command_invocations
		 WHERE account_id = ? AND region = ? AND command_id = ? AND instance_id = ?`,
		accountID, region, commandID, instanceID,
	).Scan(
		&inv.CommandID, &inv.InstanceID, &inv.DocumentName, &inv.DocumentVersion, &inv.Comment,
		&inv.Status, &inv.StatusDetails, &inv.Stdout, &inv.Stderr, &inv.ResponseCode,
		&inv.RequestedAt, &inv.ExecutionStart, &inv.ExecutionEnd,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SSMCommandInvocation{}, ErrSSMInvocationNotFound
		}
		return SSMCommandInvocation{}, fmt.Errorf("get ssm command invocation: %w", err)
	}
	inv.Region = region
	return inv, nil
}

// ListSSMCommandInvocations lists invocations filtered by optional command/instance id.
func (s *Store) ListSSMCommandInvocations(accountID, region, commandID, instanceID string) ([]SSMCommandInvocation, error) {
	if region == "" {
		region = DefaultSSMRegion
	}
	query := `SELECT command_id, instance_id, document_name, document_version, comment,
		status, status_details, stdout, stderr, response_code,
		requested_at, execution_start, execution_end
		FROM ssm_command_invocations WHERE account_id = ? AND region = ?`
	args := []any{accountID, region}
	if commandID != "" {
		query += ` AND command_id = ?`
		args = append(args, commandID)
	}
	if instanceID != "" {
		query += ` AND instance_id = ?`
		args = append(args, instanceID)
	}
	query += ` ORDER BY requested_at, instance_id`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list ssm command invocations: %w", err)
	}
	defer rows.Close()

	var out []SSMCommandInvocation
	for rows.Next() {
		var inv SSMCommandInvocation
		if err := rows.Scan(
			&inv.CommandID, &inv.InstanceID, &inv.DocumentName, &inv.DocumentVersion, &inv.Comment,
			&inv.Status, &inv.StatusDetails, &inv.Stdout, &inv.Stderr, &inv.ResponseCode,
			&inv.RequestedAt, &inv.ExecutionStart, &inv.ExecutionEnd,
		); err != nil {
			return nil, fmt.Errorf("list ssm command invocations: %w", err)
		}
		inv.Region = region
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list ssm command invocations: %w", err)
	}
	if out == nil {
		out = []SSMCommandInvocation{}
	}
	return out, nil
}

// UpdateSSMCommandInvocationStatus updates invocation fields and refreshes parent command counts.
func (s *Store) UpdateSSMCommandInvocationStatus(
	accountID, region, commandID, instanceID, status, statusDetails, stdout, stderr string,
	responseCode int, executionStart, executionEnd string,
) error {
	if region == "" {
		region = DefaultSSMRegion
	}
	stdout = TruncateSSMOutput(stdout, SSMMaxStdoutChars)
	stderr = TruncateSSMOutput(stderr, SSMMaxStderrChars)
	if statusDetails == "" {
		statusDetails = status
	}

	res, err := s.db.Exec(
		`UPDATE ssm_command_invocations
		 SET status = ?, status_details = ?, stdout = ?, stderr = ?, response_code = ?,
		     execution_start = ?, execution_end = ?
		 WHERE account_id = ? AND region = ? AND command_id = ? AND instance_id = ?`,
		status, statusDetails, stdout, stderr, responseCode, executionStart, executionEnd,
		accountID, region, commandID, instanceID,
	)
	if err != nil {
		return fmt.Errorf("update ssm command invocation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update ssm command invocation: %w", err)
	}
	if n == 0 {
		return ErrSSMInvocationNotFound
	}
	return s.refreshSSMCommandStatus(accountID, region, commandID)
}

// MarkSSMCommandInvocationInProgress moves Pending → InProgress.
func (s *Store) MarkSSMCommandInvocationInProgress(accountID, region, commandID, instanceID, executionStart string) error {
	if region == "" {
		region = DefaultSSMRegion
	}
	res, err := s.db.Exec(
		`UPDATE ssm_command_invocations
		 SET status = ?, status_details = ?, execution_start = ?
		 WHERE account_id = ? AND region = ? AND command_id = ? AND instance_id = ?
		   AND status = ?`,
		SSMCommandStatusInProgress, SSMCommandStatusInProgress, executionStart,
		accountID, region, commandID, instanceID, SSMCommandStatusPending,
	)
	if err != nil {
		return fmt.Errorf("mark ssm invocation in progress: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark ssm invocation in progress: %w", err)
	}
	if n == 0 {
		// Already progressed or missing; treat missing as not found.
		_, getErr := s.GetSSMCommandInvocation(accountID, region, commandID, instanceID)
		return getErr
	}
	return nil
}

func (s *Store) refreshSSMCommandStatus(accountID, region, commandID string) error {
	rows, err := s.db.Query(
		`SELECT status FROM ssm_command_invocations
		 WHERE account_id = ? AND region = ? AND command_id = ?`,
		accountID, region, commandID,
	)
	if err != nil {
		return fmt.Errorf("refresh ssm command status: %w", err)
	}
	defer rows.Close()

	completed := 0
	errors := 0
	pending := 0
	inProgress := 0
	timedOut := 0
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			return fmt.Errorf("refresh ssm command status: %w", err)
		}
		switch st {
		case SSMCommandStatusSuccess:
			completed++
		case SSMCommandStatusFailed:
			completed++
			errors++
		case SSMCommandStatusTimedOut:
			completed++
			errors++
			timedOut++
		case SSMCommandStatusPending:
			pending++
		case SSMCommandStatusInProgress:
			inProgress++
		default:
			pending++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("refresh ssm command status: %w", err)
	}

	status := SSMCommandStatusInProgress
	switch {
	case pending+inProgress > 0:
		status = SSMCommandStatusInProgress
	case timedOut > 0 && errors == timedOut && completed == timedOut:
		status = SSMCommandStatusTimedOut
	case errors > 0:
		status = SSMCommandStatusFailed
	default:
		status = SSMCommandStatusSuccess
	}

	_, err = s.db.Exec(
		`UPDATE ssm_commands
		 SET status = ?, status_details = ?, completed_count = ?, error_count = ?
		 WHERE account_id = ? AND region = ? AND command_id = ?`,
		status, status, completed, errors, accountID, region, commandID,
	)
	if err != nil {
		return fmt.Errorf("refresh ssm command status: update: %w", err)
	}
	return nil
}
