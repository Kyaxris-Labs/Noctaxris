package store

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/google/uuid"
)

const (
	// LambdaESMLabMaxBatchSize is the lab BatchSize ceiling (below AWS 10_000).
	LambdaESMLabMaxBatchSize = 10

	// LambdaESMDefaultBatchSize matches the AWS SQS ESM default.
	LambdaESMDefaultBatchSize = 10

	lambdaESMStateEnabled  = "Enabled"
	lambdaESMStateDisabled = "Disabled"

	// LambdaESMReportBatchItemFailures is the FunctionResponseTypes value for partial batch failure.
	LambdaESMReportBatchItemFailures = "ReportBatchItemFailures"
)

// ErrESMBatchItemFailures is returned when ReportBatchItemFailures response is malformed
// or contains unknown itemIdentifiers (AWS fail-closed for the batch).
var ErrESMBatchItemFailures = errors.New("InvalidFunctionResponse: batchItemFailures")

var (
	// ErrNoSuchEventSourceMapping is returned when a mapping UUID is unknown.
	ErrNoSuchEventSourceMapping = errors.New("ResourceNotFoundException")
	// ErrInvalidEventSourceARN is returned for unsupported or malformed ARNs.
	ErrInvalidEventSourceARN = errors.New("InvalidParameterValueException: EventSourceArn must be an SQS queue ARN or DynamoDB stream ARN")
	// ErrInvalidESMBatchSize is returned when BatchSize is out of lab range.
	ErrInvalidESMBatchSize = errors.New("InvalidParameterValueException: BatchSize out of range")
	// ErrESMSourceAuthz is returned when the function role (or source resource policy) cannot access the event source.
	ErrESMSourceAuthz = errors.New("InvalidParameterValueException: function role cannot access event source (role session or source policy Allow required)")
)

const (
	actionSQSReceiveMessage = "sqs:ReceiveMessage"
	actionSQSDeleteMessage  = "sqs:DeleteMessage"
	actionDynamoGetRecords  = "dynamodb:GetRecords"
)

// LambdaEventSourceMapping is a Lambda SQS or DynamoDB Streams event source mapping row.
type LambdaEventSourceMapping struct {
	UUID                     string
	AccountID                string
	FunctionName             string
	FunctionARN              string
	Qualifier                 string
	EventSourceARN           string
	BatchSize                int
	Enabled                  bool
	State                    string
	SourceCursor             string
	FilterCriteriaJSON       string
	FunctionResponseTypesJSON string
	LastModified             string
	CreatedAt                string
}

// CreateEventSourceMappingInput holds CreateEventSourceMapping fields.
type CreateEventSourceMappingInput struct {
	AccountID                 string
	FunctionName              string
	EventSourceARN            string
	BatchSize                 int
	Enabled                   *bool
	FilterCriteriaJSON        string
	FunctionResponseTypesJSON string
}

// UpdateEventSourceMappingInput holds UpdateEventSourceMapping fields.
type UpdateEventSourceMappingInput struct {
	AccountID                 string
	UUID                      string
	BatchSize                 *int
	Enabled                   *bool
	Function                  string // optional FunctionName update
	FilterCriteriaJSON        *string
	FunctionResponseTypesJSON *string
}

// ESMInvokeFunc is the sync invoke callback for ESM poll. responseJSON is the function payload.
// qualifier is the Create-time version/alias (or $LATEST).
type ESMInvokeFunc func(accountID, functionName, qualifier, eventJSON string) (responseJSON string, err error)

func esmMappingQualifier(m LambdaEventSourceMapping) string {
	q := strings.TrimSpace(m.Qualifier)
	if q == "" {
		return "$LATEST"
	}
	return q
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
  source_cursor TEXT NOT NULL DEFAULT '',
  last_modified TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_lambda_esm_account ON lambda_event_source_mappings(account_id);
CREATE INDEX IF NOT EXISTS idx_lambda_esm_enabled ON lambda_event_source_mappings(enabled);
`

const lambdaESMSelectCols = `uuid, account_id, function_name, function_arn, event_source_arn, batch_size, enabled, state, source_cursor, COALESCE(filter_criteria_json, ''), COALESCE(function_response_types_json, ''), last_modified, created_at, COALESCE(function_qualifier, '$LATEST')`

// EnsureLambdaESMSchema creates the SQS / DynamoDB Streams event source mapping table.
func EnsureLambdaESMSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure lambda esm schema: db is nil")
	}
	if _, err := db.Exec(lambdaESMSchema); err != nil {
		return fmt.Errorf("ensure lambda esm schema: %w", err)
	}
	for _, col := range []struct {
		stmt string
		name string
	}{
		{`ALTER TABLE lambda_event_source_mappings ADD COLUMN source_cursor TEXT NOT NULL DEFAULT ''`, "source_cursor"},
		{`ALTER TABLE lambda_event_source_mappings ADD COLUMN filter_criteria_json TEXT NOT NULL DEFAULT ''`, "filter_criteria_json"},
		{`ALTER TABLE lambda_event_source_mappings ADD COLUMN function_response_types_json TEXT NOT NULL DEFAULT ''`, "function_response_types_json"},
		{`ALTER TABLE lambda_event_source_mappings ADD COLUMN function_qualifier TEXT NOT NULL DEFAULT '$LATEST'`, "function_qualifier"},
	} {
		if _, err := db.Exec(col.stmt); err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
				return fmt.Errorf("ensure lambda esm schema: %s: %w", col.name, err)
			}
		}
	}
	return nil
}

func normalizeFunctionResponseTypesJSON(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var types []string
	if err := json.Unmarshal([]byte(raw), &types); err != nil {
		return "", fmt.Errorf("FunctionResponseTypes must be a JSON array of strings")
	}
	for _, t := range types {
		if strings.TrimSpace(t) != LambdaESMReportBatchItemFailures {
			return "", fmt.Errorf("FunctionResponseTypes: unsupported value %q", t)
		}
	}
	out, err := json.Marshal(types)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func esmReportsBatchItemFailures(typesJSON string) bool {
	typesJSON = strings.TrimSpace(typesJSON)
	if typesJSON == "" {
		return false
	}
	var types []string
	if err := json.Unmarshal([]byte(typesJSON), &types); err != nil {
		return false
	}
	for _, t := range types {
		if t == LambdaESMReportBatchItemFailures {
			return true
		}
	}
	return false
}

// parseBatchItemFailureIDs returns failed itemIdentifiers from an invoke response.
// Empty batchItemFailures (or omitted) means all succeeded. Malformed JSON / non-array /
// unknown identifiers return ErrESMBatchItemFailures (fail closed for the batch).
func parseBatchItemFailureIDs(responseJSON string, validIDs map[string]struct{}) ([]string, error) {
	responseJSON = strings.TrimSpace(responseJSON)
	if responseJSON == "" {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(responseJSON), &payload); err != nil {
		return nil, ErrESMBatchItemFailures
	}
	raw, ok := payload["batchItemFailures"]
	if !ok || raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, ErrESMBatchItemFailures
	}
	out := make([]string, 0, len(list))
	seen := map[string]struct{}{}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, ErrESMBatchItemFailures
		}
		id, _ := m["itemIdentifier"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, ErrESMBatchItemFailures
		}
		if _, known := validIDs[id]; !known {
			return nil, ErrESMBatchItemFailures
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
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

func isDynamoStreamEventSourceARN(arn string) bool {
	_, _, _, ok := ParseDynamoStreamARN(arn)
	return ok
}

func (s *Store) validateEventSourceARN(accountID, arn string) error {
	arn = strings.TrimSpace(arn)
	if isDynamoStreamEventSourceARN(arn) {
		acct, tableName, label, ok := ParseDynamoStreamARN(arn)
		if !ok || acct != accountID {
			return ErrInvalidEventSourceARN
		}
		if _, err := s.DescribeDynamoStream(acct, tableName, label); err != nil {
			return ErrInvalidEventSourceARN
		}
		return nil
	}
	queueAccount, queueName, err := validateSQSEventSourceARN(arn)
	if err != nil {
		return err
	}
	if queueAccount != accountID {
		return ErrInvalidEventSourceARN
	}
	if _, err := s.GetQueue(accountID, queueName); err != nil {
		return err
	}
	return nil
}

// CreateEventSourceMapping inserts an SQS→Lambda or DynamoDB Streams→Lambda mapping.
func (s *Store) CreateEventSourceMapping(in CreateEventSourceMappingInput) (LambdaEventSourceMapping, error) {
	fnName, qualifier := ParseFunctionQualifier(strings.TrimSpace(in.FunctionName))
	if err := ValidateFunctionName(fnName); err != nil {
		return LambdaEventSourceMapping{}, err
	}
	qualifier = strings.TrimSpace(qualifier)
	if qualifier == "" {
		qualifier = "$LATEST"
	}
	fn, _, err := s.ResolveFunction(in.AccountID, fnName, qualifier)
	if err != nil {
		return LambdaEventSourceMapping{}, err
	}
	if err := s.validateEventSourceARN(in.AccountID, in.EventSourceARN); err != nil {
		return LambdaEventSourceMapping{}, err
	}
	if !s.esmEventSourceAllows(fn, strings.TrimSpace(in.EventSourceARN)) {
		return LambdaEventSourceMapping{}, ErrESMSourceAuthz
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
	arn := strings.TrimSpace(in.EventSourceARN)
	filterJSON := strings.TrimSpace(in.FilterCriteriaJSON)
	if filterJSON != "" {
		fc, ferr := parseESMFilterCriteriaJSON(filterJSON)
		if ferr != nil {
			return LambdaEventSourceMapping{}, ferr
		}
		if len(fc.Filters) == 0 {
			filterJSON = ""
		}
	}
	respTypesJSON, err := normalizeFunctionResponseTypesJSON(in.FunctionResponseTypesJSON)
	if err != nil {
		return LambdaEventSourceMapping{}, err
	}
	functionARN := fn.FunctionARN
	if qualifier != "$LATEST" {
		if isNumericVersion(qualifier) {
			var ver int
			_, _ = fmt.Sscanf(qualifier, "%d", &ver)
			functionARN = LambdaVersionARN(in.AccountID, DefaultLambdaRegion, fnName, ver)
		} else {
			functionARN = LambdaAliasARN(in.AccountID, DefaultLambdaRegion, fnName, qualifier)
		}
	}
	_, err = s.db.Exec(
		`INSERT INTO lambda_event_source_mappings
		 (uuid, account_id, function_name, function_arn, event_source_arn, batch_size, enabled, state, source_cursor, filter_criteria_json, function_response_types_json, last_modified, created_at, function_qualifier)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?)`,
		id, in.AccountID, fn.FunctionName, functionARN, arn,
		batchSize, enabledInt, state, filterJSON, respTypesJSON, now, now, qualifier,
	)
	if err != nil {
		return LambdaEventSourceMapping{}, fmt.Errorf("create event source mapping: %w", err)
	}
	return LambdaEventSourceMapping{
		UUID:                      id,
		AccountID:                 in.AccountID,
		FunctionName:              fn.FunctionName,
		FunctionARN:               functionARN,
		Qualifier:                 qualifier,
		EventSourceARN:            arn,
		BatchSize:                 batchSize,
		Enabled:                   enabled,
		State:                     state,
		FilterCriteriaJSON:        filterJSON,
		FunctionResponseTypesJSON: respTypesJSON,
		LastModified:              now,
		CreatedAt:                 now,
	}, nil
}

// GetEventSourceMapping returns a mapping by UUID for the account.
func (s *Store) GetEventSourceMapping(accountID, mappingUUID string) (LambdaEventSourceMapping, error) {
	return s.scanEventSourceMapping(
		s.db.QueryRow(
			`SELECT `+lambdaESMSelectCols+`
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
			`SELECT `+lambdaESMSelectCols+`
			 FROM lambda_event_source_mappings WHERE account_id = ? ORDER BY created_at, uuid`,
			accountID,
		)
	} else {
		base, _ := ParseFunctionQualifier(functionName)
		rows, err = s.db.Query(
			`SELECT `+lambdaESMSelectCols+`
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
		`SELECT ` + lambdaESMSelectCols + `
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
	filterJSON := cur.FilterCriteriaJSON
	if in.FilterCriteriaJSON != nil {
		filterJSON = strings.TrimSpace(*in.FilterCriteriaJSON)
		if filterJSON != "" {
			fc, ferr := parseESMFilterCriteriaJSON(filterJSON)
			if ferr != nil {
				return LambdaEventSourceMapping{}, ferr
			}
			if len(fc.Filters) == 0 {
				filterJSON = ""
			}
		}
	}
	respTypesJSON := cur.FunctionResponseTypesJSON
	if in.FunctionResponseTypesJSON != nil {
		normalized, nerr := normalizeFunctionResponseTypesJSON(*in.FunctionResponseTypesJSON)
		if nerr != nil {
			return LambdaEventSourceMapping{}, nerr
		}
		respTypesJSON = normalized
	}
	now := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE lambda_event_source_mappings
		 SET function_name = ?, function_arn = ?, batch_size = ?, enabled = ?, state = ?, filter_criteria_json = ?, function_response_types_json = ?, last_modified = ?
		 WHERE account_id = ? AND uuid = ?`,
		fnName, fnARN, batchSize, enabledInt, state, filterJSON, respTypesJSON, now, in.AccountID, in.UUID,
	)
	if err != nil {
		return LambdaEventSourceMapping{}, fmt.Errorf("update event source mapping: %w", err)
	}
	cur.FunctionName = fnName
	cur.FunctionARN = fnARN
	cur.BatchSize = batchSize
	cur.Enabled = enabled
	cur.State = state
	cur.FilterCriteriaJSON = filterJSON
	cur.FunctionResponseTypesJSON = respTypesJSON
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

// esmEventSourceAllows reports whether the function execution role session Allows
// the required source actions, or (SQS only) the queue policy Allows lambda.amazonaws.com.
func (s *Store) esmEventSourceAllows(fn LambdaFunction, eventSourceARN string) bool {
	if strings.TrimSpace(fn.RoleARN) == "" {
		return false
	}
	session := "esm-" + fn.FunctionName
	sourceARN := fn.FunctionARN
	if isDynamoStreamEventSourceARN(eventSourceARN) {
		return s.deliveryRoleSessionAllows(fn.AccountID, fn.RoleARN, actionDynamoGetRecords, eventSourceARN, session, DefaultLambdaRegion, sourceARN)
	}
	if s.deliveryRoleSessionAllows(fn.AccountID, fn.RoleARN, actionSQSReceiveMessage, eventSourceARN, session, DefaultLambdaRegion, sourceARN) &&
		s.deliveryRoleSessionAllows(fn.AccountID, fn.RoleARN, actionSQSDeleteMessage, eventSourceARN, session, DefaultLambdaRegion, sourceARN) {
		return true
	}
	return s.deliveryTargetResourcePolicyAllows(fn.AccountID, eventSourceARN, actionSQSReceiveMessage, authz.ServicePrincipalLambda, sourceARN) &&
		s.deliveryTargetResourcePolicyAllows(fn.AccountID, eventSourceARN, actionSQSDeleteMessage, authz.ServicePrincipalLambda, sourceARN)
}

// PollEventSourceMappingOnce receives up to BatchSize records from SQS or DynamoDB Streams,
// invokes the callback synchronously with the Lambda event JSON, and advances the source on success.
// On invoke error, SQS messages remain invisible until the visibility timeout expires; DynamoDB cursor is not advanced.
// With FunctionResponseTypes ReportBatchItemFailures, only non-failed SQS messages are deleted;
// DynamoDB cursor advances only when batchItemFailures is empty.
func (s *Store) PollEventSourceMappingOnce(mappingUUID string, invoke ESMInvokeFunc) error {
	row := s.db.QueryRow(
		`SELECT `+lambdaESMSelectCols+`
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
	qualifier := strings.TrimSpace(m.Qualifier)
	if qualifier == "" {
		qualifier = "$LATEST"
	}
	fn, _, err := s.ResolveFunction(m.AccountID, m.FunctionName, qualifier)
	if err != nil {
		return err
	}
	if !s.esmEventSourceAllows(fn, m.EventSourceARN) {
		return ErrESMSourceAuthz
	}
	if isDynamoStreamEventSourceARN(m.EventSourceARN) {
		return s.pollDynamoEventSourceMappingOnce(m, invoke)
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
	fc, _ := parseESMFilterCriteriaJSON(m.FilterCriteriaJSON)
	matched := filterSQSMessages(fc, msgs)
	// Drop non-matching messages (AWS deletes filtered SQS records without invoke).
	for _, msg := range msgs {
		keep := false
		for _, m2 := range matched {
			if m2.ReceiptHandle == msg.ReceiptHandle {
				keep = true
				break
			}
		}
		if !keep {
			_ = s.DeleteMessage(m.AccountID, queueName, msg.ReceiptHandle)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	eventJSON, err := buildSQSLambdaEventJSON(m.EventSourceARN, matched)
	if err != nil {
		return err
	}
	respJSON, invErr := invoke(m.AccountID, m.FunctionName, esmMappingQualifier(m), eventJSON)
	if invErr != nil {
		return invErr
	}
	toDelete := matched
	if esmReportsBatchItemFailures(m.FunctionResponseTypesJSON) {
		valid := make(map[string]struct{}, len(matched))
		for _, msg := range matched {
			valid[msg.MessageID] = struct{}{}
		}
		failedIDs, perr := parseBatchItemFailureIDs(respJSON, valid)
		if perr != nil {
			return perr
		}
		failed := make(map[string]struct{}, len(failedIDs))
		for _, id := range failedIDs {
			failed[id] = struct{}{}
		}
		toDelete = toDelete[:0]
		for _, msg := range matched {
			if _, bad := failed[msg.MessageID]; !bad {
				toDelete = append(toDelete, msg)
			}
		}
	}
	for _, msg := range toDelete {
		if delErr := s.DeleteMessage(m.AccountID, queueName, msg.ReceiptHandle); delErr != nil {
			return delErr
		}
	}
	return nil
}

func (s *Store) pollDynamoEventSourceMappingOnce(m LambdaEventSourceMapping, invoke ESMInvokeFunc) error {
	acct, tableName, label, ok := ParseDynamoStreamARN(m.EventSourceARN)
	if !ok || acct != m.AccountID {
		return ErrInvalidEventSourceARN
	}
	table, err := s.DescribeDynamoStream(acct, tableName, label)
	if err != nil {
		return err
	}
	iterator := m.SourceCursor
	if iterator == "" {
		it, err := s.GetDynamoStreamShardIterator(acct, tableName, LabDynamoStreamShardID, "TRIM_HORIZON", "")
		if err != nil {
			return err
		}
		iterator = it
	}
	records, next, err := s.GetDynamoStreamRecords(iterator, m.BatchSize)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		_ = s.setESMSourceCursor(m.UUID, next)
		return nil
	}
	fc, _ := parseESMFilterCriteriaJSON(m.FilterCriteriaJSON)
	matched := filterDynamoStreamRecords(fc, records)
	if len(matched) == 0 {
		// Advance cursor past filtered records (lab: no invoke, no redelivery).
		return s.setESMSourceCursor(m.UUID, next)
	}
	eventJSON, err := buildDynamoLambdaEventJSON(m.EventSourceARN, table.StreamViewType, matched)
	if err != nil {
		return err
	}
	respJSON, err := invoke(m.AccountID, m.FunctionName, esmMappingQualifier(m), eventJSON)
	if err != nil {
		return err
	}
	if esmReportsBatchItemFailures(m.FunctionResponseTypesJSON) {
		valid := make(map[string]struct{}, len(matched))
		for _, rec := range matched {
			valid[rec.SequenceNumber] = struct{}{}
		}
		failedIDs, perr := parseBatchItemFailureIDs(respJSON, valid)
		if perr != nil {
			return perr
		}
		// Lab: any reported failure keeps the cursor (retry whole matched set).
		if len(failedIDs) > 0 {
			return nil
		}
	}
	return s.setESMSourceCursor(m.UUID, next)
}

func (s *Store) setESMSourceCursor(mappingUUID, cursor string) error {
	_, err := s.db.Exec(
		`UPDATE lambda_event_source_mappings SET source_cursor = ?, last_modified = ? WHERE uuid = ?`,
		cursor, nowRFC3339(), strings.TrimSpace(mappingUUID),
	)
	if err != nil {
		return fmt.Errorf("set esm source cursor: %w", err)
	}
	return nil
}

func buildDynamoLambdaEventJSON(streamARN, streamViewType string, records []DynamoStreamRecord) (string, error) {
	region := DefaultDynamoRegion
	if parts := strings.Split(streamARN, ":"); len(parts) >= 4 {
		region = parts[3]
	}
	if streamViewType == "" {
		streamViewType = StreamViewNewImage
	}
	out := make([]map[string]any, 0, len(records))
	for i, rec := range records {
		ddb := map[string]any{
			"SequenceNumber":  rec.SequenceNumber,
			"StreamViewType":  streamViewType,
			"SizeBytes":       len(rec.KeysJSON) + len(rec.NewImageJSON),
			"ApproximateCreationDateTime": float64(rec.ArrivalMS) / 1000.0,
		}
		if rec.KeysJSON != "" {
			ddb["Keys"] = json.RawMessage(rec.KeysJSON)
		}
		if rec.NewImageJSON != "" {
			ddb["NewImage"] = json.RawMessage(rec.NewImageJSON)
		}
		out = append(out, map[string]any{
			"eventID":        fmt.Sprintf("%d", i+1),
			"eventName":      rec.EventName,
			"eventVersion":   "1.1",
			"eventSource":    "aws:dynamodb",
			"awsRegion":      region,
			"eventSourceARN": streamARN,
			"dynamodb":       ddb,
		})
	}
	raw, err := json.Marshal(map[string]any{"Records": out})
	if err != nil {
		return "", fmt.Errorf("marshal dynamodb lambda event: %w", err)
	}
	return string(raw), nil
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
		&m.BatchSize, &enabledInt, &m.State, &m.SourceCursor, &m.FilterCriteriaJSON, &m.FunctionResponseTypesJSON,
		&m.LastModified, &m.CreatedAt, &m.Qualifier,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LambdaEventSourceMapping{}, ErrNoSuchEventSourceMapping
	}
	if err != nil {
		return LambdaEventSourceMapping{}, fmt.Errorf("get event source mapping: %w", err)
	}
	m.Enabled = enabledInt == 1
	if strings.TrimSpace(m.Qualifier) == "" {
		m.Qualifier = "$LATEST"
	}
	return m, nil
}

func scanEventSourceMappingRow(rows *sql.Rows) (LambdaEventSourceMapping, error) {
	var m LambdaEventSourceMapping
	var enabledInt int
	if err := rows.Scan(
		&m.UUID, &m.AccountID, &m.FunctionName, &m.FunctionARN, &m.EventSourceARN,
		&m.BatchSize, &enabledInt, &m.State, &m.SourceCursor, &m.FilterCriteriaJSON, &m.FunctionResponseTypesJSON,
		&m.LastModified, &m.CreatedAt, &m.Qualifier,
	); err != nil {
		return LambdaEventSourceMapping{}, fmt.Errorf("scan event source mapping: %w", err)
	}
	m.Enabled = enabledInt == 1
	if strings.TrimSpace(m.Qualifier) == "" {
		m.Qualifier = "$LATEST"
	}
	return m, nil
}
