package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// LambdaAsyncMaxRetries is the lab retry count after the first invoke attempt.
const LambdaAsyncMaxRetries = 2

const (
	lambdaAsyncStatusPending   = "pending"
	lambdaAsyncStatusSucceeded = "succeeded"
	lambdaAsyncStatusFailed    = "failed"
)

// LambdaAsyncInvocation is a queued async Invoke job.
type LambdaAsyncInvocation struct {
	InvocationID string
	AccountID    string
	FunctionName string
	Qualifier    string
	EventJSON    string
	Attempts     int
	Status       string
	LastError    string
	CreatedAt    string
}

// EnsureLambdaAsyncSchema adds DLQ columns and the async invocation queue table.
func EnsureLambdaAsyncSchema(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE lambda_functions ADD COLUMN dead_letter_target_arn TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lambda_functions ADD COLUMN destination_on_failure_arn TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lambda_versions ADD COLUMN dead_letter_target_arn TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lambda_versions ADD COLUMN destination_on_failure_arn TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS lambda_async_invocations (
		  invocation_id TEXT PRIMARY KEY,
		  account_id TEXT NOT NULL,
		  function_name TEXT NOT NULL,
		  qualifier TEXT NOT NULL DEFAULT '$LATEST',
		  event_json TEXT NOT NULL,
		  attempts INTEGER NOT NULL DEFAULT 0,
		  status TEXT NOT NULL DEFAULT 'pending',
		  last_error TEXT NOT NULL DEFAULT '',
		  created_at TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				continue
			}
			return fmt.Errorf("ensure lambda async schema: %w", err)
		}
	}
	return nil
}

// EnqueueAsyncInvoke stores a pending async invocation job.
func (s *Store) EnqueueAsyncInvoke(accountID, functionName, qualifier, eventJSON string) (LambdaAsyncInvocation, error) {
	if err := ValidateFunctionName(functionName); err != nil {
		return LambdaAsyncInvocation{}, err
	}
	qualifier = strings.TrimSpace(qualifier)
	if qualifier == "" {
		qualifier = "$LATEST"
	}
	if strings.TrimSpace(eventJSON) == "" {
		eventJSON = "{}"
	}
	invocationID := uuid.NewString()
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO lambda_async_invocations
		 (invocation_id, account_id, function_name, qualifier, event_json, attempts, status, last_error, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, '', ?)`,
		invocationID, accountID, functionName, qualifier, eventJSON, lambdaAsyncStatusPending, created,
	)
	if err != nil {
		return LambdaAsyncInvocation{}, fmt.Errorf("enqueue async invoke: %w", err)
	}
	return LambdaAsyncInvocation{
		InvocationID: invocationID,
		AccountID:      accountID,
		FunctionName:   functionName,
		Qualifier:      qualifier,
		EventJSON:      eventJSON,
		Attempts:       0,
		Status:         lambdaAsyncStatusPending,
		CreatedAt:      created,
	}, nil
}

// GetAsyncInvocation returns a queued async invoke job for tests and diagnostics.
func (s *Store) GetAsyncInvocation(invocationID string) (LambdaAsyncInvocation, error) {
	return s.getAsyncInvocation(invocationID)
}

// LatestAsyncInvocation returns the most recently enqueued async job for a function.
func (s *Store) LatestAsyncInvocation(accountID, functionName string) (LambdaAsyncInvocation, error) {
	row := s.db.QueryRow(
		`SELECT invocation_id, account_id, function_name, qualifier, event_json, attempts, status, last_error, created_at
		 FROM lambda_async_invocations
		 WHERE account_id = ? AND function_name = ?
		 ORDER BY created_at DESC, invocation_id DESC
		 LIMIT 1`,
		accountID, functionName,
	)
	var job LambdaAsyncInvocation
	err := row.Scan(
		&job.InvocationID, &job.AccountID, &job.FunctionName, &job.Qualifier, &job.EventJSON,
		&job.Attempts, &job.Status, &job.LastError, &job.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LambdaAsyncInvocation{}, fmt.Errorf("async invocation not found")
	}
	if err != nil {
		return LambdaAsyncInvocation{}, fmt.Errorf("latest async invocation: %w", err)
	}
	return job, nil
}

func (s *Store) getAsyncInvocation(invocationID string) (LambdaAsyncInvocation, error) {
	row := s.db.QueryRow(
		`SELECT invocation_id, account_id, function_name, qualifier, event_json, attempts, status, last_error, created_at
		 FROM lambda_async_invocations WHERE invocation_id = ?`,
		invocationID,
	)
	var job LambdaAsyncInvocation
	err := row.Scan(
		&job.InvocationID, &job.AccountID, &job.FunctionName, &job.Qualifier, &job.EventJSON,
		&job.Attempts, &job.Status, &job.LastError, &job.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LambdaAsyncInvocation{}, fmt.Errorf("async invocation not found")
	}
	if err != nil {
		return LambdaAsyncInvocation{}, fmt.Errorf("get async invocation: %w", err)
	}
	return job, nil
}

func (s *Store) recordAsyncAttempt(invocationID string, attempt int, invokeErr error) error {
	lastErr := ""
	if invokeErr != nil {
		lastErr = invokeErr.Error()
	}
	_, err := s.db.Exec(
		`UPDATE lambda_async_invocations SET attempts = ?, last_error = ? WHERE invocation_id = ?`,
		attempt, lastErr, invocationID,
	)
	if err != nil {
		return fmt.Errorf("record async attempt: %w", err)
	}
	return nil
}

func (s *Store) markAsyncInvocationStatus(invocationID, status string) error {
	_, err := s.db.Exec(
		`UPDATE lambda_async_invocations SET status = ? WHERE invocation_id = ?`,
		status, invocationID,
	)
	if err != nil {
		return fmt.Errorf("mark async invocation %s: %w", status, err)
	}
	return nil
}

// ProcessAsyncInvocation runs the invoke attempt function with lab retries, then
// delivers to configured SQS DLQ targets on final failure.
func (s *Store) ProcessAsyncInvocation(invocationID string, maxRetries int, attempt func() error) error {
	if maxRetries < 0 {
		maxRetries = 0
	}
	job, err := s.getAsyncInvocation(invocationID)
	if err != nil {
		return err
	}
	fn, _, err := s.ResolveFunction(job.AccountID, job.FunctionName, job.Qualifier)
	if err != nil {
		return err
	}

	var lastErr error
	totalAttempts := maxRetries + 1
	for i := 1; i <= totalAttempts; i++ {
		lastErr = attempt()
		if recordErr := s.recordAsyncAttempt(invocationID, i, lastErr); recordErr != nil {
			return recordErr
		}
		if lastErr == nil {
			return s.markAsyncInvocationStatus(invocationID, lambdaAsyncStatusSucceeded)
		}
	}
	if err := s.DeliverLambdaFailure(fn, job.EventJSON, lastErr); err != nil {
		return err
	}
	return s.markAsyncInvocationStatus(invocationID, lambdaAsyncStatusFailed)
}

// DeliverLambdaFailure sends the failed event to DeadLetterConfig.TargetArn and/or
// DestinationConfig.OnFailure when they reference SQS queue ARNs or SNS topic ARNs.
func (s *Store) DeliverLambdaFailure(fn LambdaFunction, eventJSON string, invokeErr error) error {
	targets := lambdaFailureTargets(fn)
	if len(targets) == 0 {
		return nil
	}
	body, err := lambdaFailureMessageBody(fn, eventJSON, invokeErr)
	if err != nil {
		return err
	}
	for _, arn := range targets {
		if err := s.sendLambdaFailureDestination(fn.AccountID, arn, body); err != nil {
			return err
		}
	}
	return nil
}

func lambdaFailureTargets(fn LambdaFunction) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, arn := range []string{fn.DeadLetterTargetArn, fn.DestinationOnFailureArn} {
		arn = strings.TrimSpace(arn)
		if arn == "" {
			continue
		}
		if _, ok := seen[arn]; ok {
			continue
		}
		seen[arn] = struct{}{}
		out = append(out, arn)
	}
	return out
}

func lambdaFailureMessageBody(fn LambdaFunction, eventJSON string, invokeErr error) ([]byte, error) {
	var requestPayload any
	if err := json.Unmarshal([]byte(eventJSON), &requestPayload); err != nil {
		requestPayload = eventJSON
	}
	errMsg := ""
	if invokeErr != nil {
		errMsg = invokeErr.Error()
	}
	return json.Marshal(map[string]any{
		"requestPayload": requestPayload,
		"responseContext": map[string]any{
			"statusCode":    200,
			"functionError": "Unhandled",
		},
		"version":      "1.0",
		"timestamp":    nowRFC3339(),
		"requestContext": map[string]any{
			"functionArn": fn.FunctionARN,
		},
		"errorMessage": errMsg,
	})
}

func (s *Store) sendLambdaFailureDestination(accountID, destARN string, body []byte) error {
	destARN = strings.TrimSpace(destARN)
	switch {
	case strings.HasPrefix(destARN, "arn:aws:sqs:"):
		queueName, err := queueNameFromARN(destARN)
		if err != nil {
			return err
		}
		if _, err := s.SendMessage(accountID, queueName, body, false, nil, "", nil); err != nil {
			return fmt.Errorf("send lambda failure to SQS: %w", err)
		}
		return nil
	case strings.HasPrefix(destARN, "arn:aws:sns:"):
		topic, err := s.GetTopicByARN(destARN)
		if err != nil {
			return fmt.Errorf("send lambda failure to SNS: %w", err)
		}
		if topic.AccountID != accountID {
			return fmt.Errorf("send lambda failure to SNS: topic account mismatch")
		}
		if _, err := s.Publish(accountID, topic.TopicName, string(body), "", nil); err != nil {
			return fmt.Errorf("send lambda failure to SNS: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("failure destination must be an SQS queue ARN or SNS topic ARN")
	}
}
