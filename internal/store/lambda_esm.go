package store

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	// LambdaESMLabMaxBatchSize is the lab BatchSize ceiling (below AWS 10_000).
	LambdaESMLabMaxBatchSize = 10

	// LambdaESMDefaultBatchSize matches the AWS SQS ESM default.
	LambdaESMDefaultBatchSize = 10

	lambdaESMStateEnabled  = "Enabled"
	lambdaESMStateDisabled = "Disabled"
)

var (
	// ErrNoSuchEventSourceMapping is returned when a mapping UUID is unknown.
	ErrNoSuchEventSourceMapping = errors.New("ResourceNotFoundException")
	// ErrInvalidEventSourceARN is returned for non-SQS or malformed ARNs.
	ErrInvalidEventSourceARN = errors.New("InvalidParameterValueException: EventSourceArn must be an SQS queue ARN")
	// ErrInvalidESMBatchSize is returned when BatchSize is out of lab range.
	ErrInvalidESMBatchSize = errors.New("InvalidParameterValueException: BatchSize out of range")
)

// LambdaEventSourceMapping is a Lambda SQS event source mapping row.
type LambdaEventSourceMapping struct {
	UUID           string
	AccountID      string
	FunctionName   string
	FunctionARN    string
	EventSourceARN string
	BatchSize      int
	Enabled        bool
	State          string
	LastModified   string
	CreatedAt      string
}

// CreateEventSourceMappingInput holds CreateEventSourceMapping fields.
type CreateEventSourceMappingInput struct {
	AccountID      string
	FunctionName   string
	EventSourceARN string
	BatchSize      int
	Enabled        *bool
}

// UpdateEventSourceMappingInput holds UpdateEventSourceMapping fields.
type UpdateEventSourceMappingInput struct {
	AccountID string
	UUID      string
	BatchSize *int
	Enabled   *bool
	Function  string // optional FunctionName update
}

const lambdaESMSchema = `
CREATE TABLE IF NOT EXISTS lambda_event_source_mappings (
  uuid TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  function_name TEXT NOT NULL,
  function_arn TEXT NOT NULL,
  event_source_arn TEXT NOT NULL,
  batch_size INTEGER NOT NULL DEFAULT 10,
  enabled INTEGER NOT NULL DEFAULT 1,
  state TEXT NOT NULL DEFAULT 'Enabled',
  last_modified TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_lambda_esm_account ON lambda_event_source_mappings(account_id);
CREATE INDEX IF NOT EXISTS idx_lambda_esm_enabled ON lambda_event_source_mappings(enabled);
`

// EnsureLambdaESMSchema creates the SQS event source mapping table.
func EnsureLambdaESMSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure lambda esm schema: db is nil")
	}
	if _, err := db.Exec(lambdaESMSchema); err != nil {
		return fmt.Errorf("ensure lambda esm schema: %w", err)
	}
	return nil
}

// EnsureLambdaESMSchema ensures ESM tables on an open store.
func (s *Store) EnsureLambdaESMSchema() error {
	return EnsureLambdaESMSchema(s.db)
}

func normalizeESMBatchSize(batchSize int) (int, error) {
	if batchSize == 0 {
		return LambdaESMDefaultBatchSize, nil
	}
	if batchSize < 1 || batchSize > LambdaESMLabMaxBatchSize {
		return 0, ErrInvalidESMBatchSize
	}
	return batchSize, nil
}

func validateSQSEventSourceARN(arn string) (accountID, queueName string, err error) {
	arn = strings.TrimSpace(arn)
	accountID, err = queueAccountFromARN(arn)
	if err != nil {
		return "", "", ErrInvalidEventSourceARN
	}
	queueName, err = queueNameFromARN(arn)
	if err != nil {
		return "", "", ErrInvalidEventSourceARN
	}
	return accountID, queueName, nil
}

// CreateEventSourceMapping inserts an SQS→Lambda mapping.
func (s *Store) CreateEventSourceMapping(in CreateEventSourceMappingInput) (LambdaEventSourceMapping, error) {
	fnName, qualifier := ParseFunctionQualifier(strings.TrimSpace(in.FunctionName))
	if err := ValidateFunctionName(fnName); err != nil {
		return LambdaEventSourceMapping{}, err
	}
	fn, _, err := s.ResolveFunction(in.AccountID, fnName, qualifier)
	if err != nil {
		return LambdaEventSourceMapping{}, err
	}
	queueAccount, _, err := validateSQSEventSourceARN(in.EventSourceARN)
	if err != nil {
		return LambdaEventSourceMapping{}, err
	}
	if queueAccount != in.AccountID {
		return LambdaEventSourceMapping{}, ErrInvalidEventSourceARN
	}
	queueName, err := queueNameFromARN(in.EventSourceARN)
	if err != nil {
		return LambdaEventSourceMapping{}, ErrInvalidEventSourceARN
	}
	if _, err := s.GetQueue(in.AccountID, queueName); err != nil {
		return LambdaEventSourceMapping{}, err
	}
	batchSize, err := normalizeESMBatchSize(in.BatchSize)
	if err != nil {
		return LambdaEventSourceMapping{}, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	state := lambdaESMStateEnabled
	if !enabled {
		state = lambdaESMStateDisabled
	}
	id := uuid.NewString()
	now := nowRFC3339()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO lambda_event_source_mappings
		 (uuid, account_id, function_name, function_arn, event_source_arn, batch_size, enabled, state, last_modified, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.AccountID, fn.FunctionName, fn.FunctionARN, strings.TrimSpace(in.EventSourceARN),
		batchSize, enabledInt, state, now, now,
	)
	if err != nil {
		return LambdaEventSourceMapping{}, fmt.Errorf("create event source mapping: %w", err)
	}
	return LambdaEventSourceMapping{
		UUID:           id,
		AccountID:      in.AccountID,
		FunctionName:   fn.FunctionName,
		FunctionARN:    fn.FunctionARN,
		EventSourceARN: strings.TrimSpace(in.EventSourceARN),
		BatchSize:      batchSize,
		Enabled:        enabled,
		State:          state,
		LastModified:   now,
		CreatedAt:      now,
	}, nil
}

// GetEventSourceMapping returns a mapping by UUID for the account.
func (s *Store) GetEventSourceMapping(accountID, mappingUUID string) (LambdaEventSourceMapping, error) {
	return s.scanEventSourceMapping(
		s.db.QueryRow(
			`SELECT uuid, account_id, function_name, function_arn, event_source_arn, batch_size, enabled, state, last_modified, created_at
			 FROM lambda_event_source_mappings WHERE account_id = ? AND uuid = ?`,
			accountID, strings.TrimSpace(mappingUUID),
		),
	)
}

// ListEventSourceMappings returns mappings for an account, optionally filtered by function name.
func (s *Store) ListEventSourceMappings(accountID, functionName string) ([]LambdaEventSourceMapping, error) {
	functionName = strings.TrimSpace(functionName)
	var (
		rows *sql.Rows
		err  error
	)
	if functionName == "" {
		rows, err = s.db.Query(
			`SELECT uuid, account_id, function_name, function_arn, event_source_arn, batch_size, enabled, state, last_modified, created_at
			 FROM lambda_event_source_mappings WHERE account_id = ? ORDER BY created_at, uuid`,
			accountID,
		)
	} else {
		base, _ := ParseFunctionQualifier(functionName)
		rows, err = s.db.Query(
			`SELECT uuid, account_id, function_name, function_arn, event_source_arn, batch_size, enabled, state, last_modified, created_at
			 FROM lambda_event_source_mappings WHERE account_id = ? AND function_name = ? ORDER BY created_at, uuid`,
			accountID, base,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list event source mappings: %w", err)
	}
	defer rows.Close()
	out := []LambdaEventSourceMapping{}
	for rows.Next() {
		m, err := scanEventSourceMappingRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListEnabledEventSourceMappings returns all enabled mappings (poller).
func (s *Store) ListEnabledEventSourceMappings() ([]LambdaEventSourceMapping, error) {
	rows, err := s.db.Query(
		`SELECT uuid, account_id, function_name, function_arn, event_source_arn, batch_size, enabled, state, last_modified, created_at
		 FROM lambda_event_source_mappings WHERE enabled = 1 ORDER BY created_at, uuid`,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled event source mappings: %w", err)
	}
	defer rows.Close()
	out := []LambdaEventSourceMapping{}
	for rows.Next() {
		m, err := scanEventSourceMappingRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpdateEventSourceMapping updates Enabled and/or BatchSize.
func (s *Store) UpdateEventSourceMapping(in UpdateEventSourceMappingInput) (LambdaEventSourceMapping, error) {
	cur, err := s.GetEventSourceMapping(in.AccountID, in.UUID)
	if err != nil {
		return LambdaEventSourceMapping{}, err
	}
	batchSize := cur.BatchSize
	if in.BatchSize != nil {
		batchSize, err = normalizeESMBatchSize(*in.BatchSize)
		if err != nil {
			return LambdaEventSourceMapping{}, err
		}
	}
	enabled := cur.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	fnName := cur.FunctionName
	fnARN := cur.FunctionARN
	if strings.TrimSpace(in.Function) != "" {
		base, qualifier := ParseFunctionQualifier(strings.TrimSpace(in.Function))
		fn, _, err := s.ResolveFunction(in.AccountID, base, qualifier)
		if err != nil {
			return LambdaEventSourceMapping{}, err
		}
		fnName = fn.FunctionName
		fnARN = fn.FunctionARN
	}
	state := lambdaESMStateEnabled
	if !enabled {
		state = lambdaESMStateDisabled
	}
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	now := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE lambda_event_source_mappings
		 SET function_name = ?, function_arn = ?, batch_size = ?, enabled = ?, state = ?, last_modified = ?
		 WHERE account_id = ? AND uuid = ?`,
		fnName, fnARN, batchSize, enabledInt, state, now, in.AccountID, in.UUID,
	)
	if err != nil {
		return LambdaEventSourceMapping{}, fmt.Errorf("update event source mapping: %w", err)
	}
	cur.FunctionName = fnName
	cur.FunctionARN = fnARN
	cur.BatchSize = batchSize
	cur.Enabled = enabled
	cur.State = state
	cur.LastModified = now
	return cur, nil
}

// DeleteEventSourceMapping removes a mapping.
func (s *Store) DeleteEventSourceMapping(accountID, mappingUUID string) error {
	res, err := s.db.Exec(
		`DELETE FROM lambda_event_source_mappings WHERE account_id = ? AND uuid = ?`,
		accountID, strings.TrimSpace(mappingUUID),
	)
	if err != nil {
		return fmt.Errorf("delete event source mapping: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoSuchEventSourceMapping
	}
	return nil
}

// PollEventSourceMappingOnce receives up to BatchSize messages, invokes the callback
// synchronously with the SQS Lambda event JSON, and deletes messages on success.
// On invoke error, messages remain invisible until the visibility timeout expires.
func (s *Store) PollEventSourceMappingOnce(mappingUUID string, invoke func(accountID, functionName, eventJSON string) error) error {
	row := s.db.QueryRow(
		`SELECT uuid, account_id, function_name, function_arn, event_source_arn, batch_size, enabled, state, last_modified, created_at
		 FROM lambda_event_source_mappings WHERE uuid = ?`,
		strings.TrimSpace(mappingUUID),
	)
	m, err := s.scanEventSourceMapping(row)
	if err != nil {
		return err
	}
	if !m.Enabled {
		return nil
	}
	queueName, err := queueNameFromARN(m.EventSourceARN)
	if err != nil {
		return err
	}
	msgs, err := s.ReceiveMessages(m.AccountID, queueName, m.BatchSize)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}
	eventJSON, err := buildSQSLambdaEventJSON(m.EventSourceARN, msgs)
	if err != nil {
		return err
	}
	if err := invoke(m.AccountID, m.FunctionName, eventJSON); err != nil {
		return err
	}
	for _, msg := range msgs {
		if delErr := s.DeleteMessage(m.AccountID, queueName, msg.ReceiptHandle); delErr != nil {
			return delErr
		}
	}
	return nil
}

func buildSQSLambdaEventJSON(eventSourceARN string, msgs []Message) (string, error) {
	records := make([]map[string]any, 0, len(msgs))
	region := DefaultSQSRegion
	if parts := strings.Split(eventSourceARN, ":"); len(parts) >= 4 {
		region = parts[3]
	}
	for _, msg := range msgs {
		body := string(msg.Body)
		sum := md5.Sum(msg.Body)
		rec := map[string]any{
			"messageId":         msg.MessageID,
			"receiptHandle":     msg.ReceiptHandle,
			"body":              body,
			"attributes":        map[string]string{"ApproximateReceiveCount": fmt.Sprintf("%d", msg.ReceiveCount)},
			"messageAttributes": map[string]any{},
			"md5OfBody":         hex.EncodeToString(sum[:]),
			"eventSource":       "aws:sqs",
			"eventSourceARN":    eventSourceARN,
			"awsRegion":         region,
		}
		if msg.MessageGroupID != "" {
			attrs, _ := rec["attributes"].(map[string]string)
			attrs["MessageGroupId"] = msg.MessageGroupID
		}
		records = append(records, rec)
	}
	raw, err := json.Marshal(map[string]any{"Records": records})
	if err != nil {
		return "", fmt.Errorf("marshal sqs lambda event: %w", err)
	}
	return string(raw), nil
}

func (s *Store) scanEventSourceMapping(row *sql.Row) (LambdaEventSourceMapping, error) {
	var m LambdaEventSourceMapping
	var enabledInt int
	err := row.Scan(
		&m.UUID, &m.AccountID, &m.FunctionName, &m.FunctionARN, &m.EventSourceARN,
		&m.BatchSize, &enabledInt, &m.State, &m.LastModified, &m.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LambdaEventSourceMapping{}, ErrNoSuchEventSourceMapping
	}
	if err != nil {
		return LambdaEventSourceMapping{}, fmt.Errorf("get event source mapping: %w", err)
	}
	m.Enabled = enabledInt == 1
	return m, nil
}

func scanEventSourceMappingRow(rows *sql.Rows) (LambdaEventSourceMapping, error) {
	var m LambdaEventSourceMapping
	var enabledInt int
	if err := rows.Scan(
		&m.UUID, &m.AccountID, &m.FunctionName, &m.FunctionARN, &m.EventSourceARN,
		&m.BatchSize, &enabledInt, &m.State, &m.LastModified, &m.CreatedAt,
	); err != nil {
		return LambdaEventSourceMapping{}, fmt.Errorf("scan event source mapping: %w", err)
	}
	m.Enabled = enabledInt == 1
	return m, nil
}
