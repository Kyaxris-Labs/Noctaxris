package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

var (
	ErrPipeExists   = errors.New("ConflictException")
	ErrPipeNotFound = errors.New("NotFoundException")
	ErrPipeBadReq   = errors.New("ValidationException")
)

const DefaultPipesRegion = "us-east-1"

const pipesSchema = `
CREATE TABLE IF NOT EXISTS pipes (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  desired_state TEXT NOT NULL,
  current_state TEXT NOT NULL,
  source_arn TEXT NOT NULL,
  target_arn TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  enrichment_arn TEXT NOT NULL DEFAULT '',
  source_cursor TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
`

// Pipe is an EventBridge Pipe row.
type Pipe struct {
	Name           string
	ARN            string
	Description    string
	DesiredState   string // RUNNING or STOPPED
	CurrentState   string
	SourceARN      string
	TargetARN      string
	RoleARN        string
	EnrichmentARN  string
	SourceCursor   string
	CreatedAt      int64
}

// EnsurePipesSchema creates pipes tables if missing.
func EnsurePipesSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure pipes schema: db is nil")
	}
	if _, err := db.Exec(pipesSchema); err != nil {
		return fmt.Errorf("ensure pipes schema: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE pipes ADD COLUMN enrichment_arn TEXT NOT NULL DEFAULT ''`); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
			return fmt.Errorf("ensure pipes schema: enrichment_arn: %w", err)
		}
	}
	return nil
}

// EnsurePipesSchema ensures pipes tables on an open store.
func (s *Store) EnsurePipesSchema() error {
	return EnsurePipesSchema(s.db)
}

// PipeARN builds arn:aws:pipes:REGION:ACCOUNT:pipe/NAME
func PipeARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultPipesRegion
	}
	return fmt.Sprintf("arn:aws:pipes:%s:%s:pipe/%s", region, accountID, name)
}

// CreatePipe inserts a pipe.
func (s *Store) CreatePipe(accountID, region, name, description, sourceARN, targetARN, roleARN, desiredState string) (Pipe, error) {
	return s.CreatePipeWithEnrichment(accountID, region, name, description, sourceARN, targetARN, roleARN, "", desiredState)
}

// CreatePipeWithEnrichment inserts a pipe with optional Lambda enrichment ARN.
func (s *Store) CreatePipeWithEnrichment(accountID, region, name, description, sourceARN, targetARN, roleARN, enrichmentARN, desiredState string) (Pipe, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Pipe{}, fmt.Errorf("%w: Name is required", ErrPipeBadReq)
	}
	if strings.TrimSpace(sourceARN) == "" || strings.TrimSpace(targetARN) == "" {
		return Pipe{}, fmt.Errorf("%w: Source and Target are required", ErrPipeBadReq)
	}
	enrichmentARN = strings.TrimSpace(enrichmentARN)
	if enrichmentARN != "" && !strings.Contains(enrichmentARN, ":lambda:") {
		return Pipe{}, fmt.Errorf("%w: Enrichment must be a Lambda ARN", ErrPipeBadReq)
	}
	desiredState = strings.ToUpper(strings.TrimSpace(desiredState))
	if desiredState == "" {
		desiredState = "RUNNING"
	}
	if desiredState != "RUNNING" && desiredState != "STOPPED" {
		return Pipe{}, fmt.Errorf("%w: DesiredState must be RUNNING or STOPPED", ErrPipeBadReq)
	}
	if _, err := s.GetPipe(accountID, name); err == nil {
		return Pipe{}, ErrPipeExists
	} else if !errors.Is(err, ErrPipeNotFound) {
		return Pipe{}, err
	}
	now := time.Now().UTC().UnixMilli()
	arn := PipeARN(region, accountID, name)
	current := "RUNNING"
	if desiredState == "STOPPED" {
		current = "STOPPED"
	}
	_, err := s.db.Exec(
		`INSERT INTO pipes (account_id, name, arn, description, desired_state, current_state, source_arn, target_arn, role_arn, enrichment_arn, source_cursor, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		accountID, name, arn, description, desiredState, current, sourceARN, targetARN, roleARN, enrichmentARN, now,
	)
	if err != nil {
		return Pipe{}, fmt.Errorf("create pipe: %w", err)
	}
	return Pipe{
		Name: name, ARN: arn, Description: description,
		DesiredState: desiredState, CurrentState: current,
		SourceARN: sourceARN, TargetARN: targetARN, RoleARN: roleARN,
		EnrichmentARN: enrichmentARN, CreatedAt: now,
	}, nil
}

// GetPipe returns a pipe by name.
func (s *Store) GetPipe(accountID, name string) (Pipe, error) {
	var p Pipe
	err := s.db.QueryRow(
		`SELECT name, arn, description, desired_state, current_state, source_arn, target_arn, role_arn,
		        COALESCE(enrichment_arn, ''), source_cursor, created_at
		 FROM pipes WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&p.Name, &p.ARN, &p.Description, &p.DesiredState, &p.CurrentState, &p.SourceARN, &p.TargetARN, &p.RoleARN, &p.EnrichmentARN, &p.SourceCursor, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Pipe{}, ErrPipeNotFound
	}
	if err != nil {
		return Pipe{}, fmt.Errorf("get pipe: %w", err)
	}
	return p, nil
}

// DescribePipe is an alias for GetPipe.
func (s *Store) DescribePipe(accountID, name string) (Pipe, error) {
	return s.GetPipe(accountID, name)
}

// DeletePipe removes a pipe.
func (s *Store) DeletePipe(accountID, name string) error {
	res, err := s.db.Exec(`DELETE FROM pipes WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete pipe: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPipeNotFound
	}
	return nil
}

// ListPipes lists pipes for an account.
func (s *Store) ListPipes(accountID string) ([]Pipe, error) {
	rows, err := s.db.Query(
		`SELECT name, arn, description, desired_state, current_state, source_arn, target_arn, role_arn,
		        COALESCE(enrichment_arn, ''), source_cursor, created_at
		 FROM pipes WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list pipes: %w", err)
	}
	defer rows.Close()
	out := []Pipe{}
	for rows.Next() {
		var p Pipe
		if err := rows.Scan(&p.Name, &p.ARN, &p.Description, &p.DesiredState, &p.CurrentState, &p.SourceARN, &p.TargetARN, &p.RoleARN, &p.EnrichmentARN, &p.SourceCursor, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list pipes: scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListRunningPipes returns pipes with CurrentState RUNNING.
func (s *Store) ListRunningPipes() ([]struct {
	AccountID string
	Pipe      Pipe
}, error) {
	rows, err := s.db.Query(
		`SELECT account_id, name, arn, description, desired_state, current_state, source_arn, target_arn, role_arn,
		        COALESCE(enrichment_arn, ''), source_cursor, created_at
		 FROM pipes WHERE current_state = 'RUNNING' ORDER BY account_id, name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list running pipes: %w", err)
	}
	defer rows.Close()
	var out []struct {
		AccountID string
		Pipe      Pipe
	}
	for rows.Next() {
		var accountID string
		var p Pipe
		if err := rows.Scan(&accountID, &p.Name, &p.ARN, &p.Description, &p.DesiredState, &p.CurrentState, &p.SourceARN, &p.TargetARN, &p.RoleARN, &p.EnrichmentARN, &p.SourceCursor, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list running pipes: scan: %w", err)
		}
		out = append(out, struct {
			AccountID string
			Pipe      Pipe
		}{AccountID: accountID, Pipe: p})
	}
	return out, rows.Err()
}

// SetPipeSourceCursor stores the last consumed cursor for a pipe.
func (s *Store) SetPipeSourceCursor(accountID, name, cursor string) error {
	_, err := s.db.Exec(
		`UPDATE pipes SET source_cursor = ? WHERE account_id = ? AND name = ?`,
		cursor, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("set pipe cursor: %w", err)
	}
	return nil
}

// PipeInvokeFunc invokes a Lambda for pipe enrichment or target delivery.
// Enrichment uses the returned resultJSON as the payload forwarded to the target.
// Target Lambda delivery ignores resultJSON.
type PipeInvokeFunc func(accountID, functionName, payloadJSON string) (resultJSON string, err error)

// PollPipeOnce drains one batch from the pipe source toward the target.
// invoke is called for Lambda enrichment and Lambda targets.
// SQS targets use SendMessage directly. Returns nil when there is nothing to drain.
func (s *Store) PollPipeOnce(accountID, pipeName string, invoke PipeInvokeFunc) error {
	p, err := s.GetPipe(accountID, pipeName)
	if err != nil {
		return err
	}
	if p.CurrentState != "RUNNING" {
		return nil
	}
	source := strings.TrimSpace(p.SourceARN)
	target := strings.TrimSpace(p.TargetARN)

	switch {
	case strings.Contains(source, ":sqs:"):
		return s.pollPipeFromSQS(accountID, p, source, target, invoke)
	case strings.Contains(source, ":dynamodb:") && strings.Contains(source, "/stream/"):
		return s.pollPipeFromDynamoStream(accountID, p, source, target, invoke)
	case strings.Contains(source, ":events:") && strings.Contains(source, ":event-bus/"):
		return s.pollPipeFromEventBus(accountID, p, source, target, invoke)
	default:
		return fmt.Errorf("%w: unsupported Source ARN", ErrPipeBadReq)
	}
}

func (s *Store) pollPipeFromSQS(accountID string, p Pipe, sourceARN, targetARN string, invoke PipeInvokeFunc) error {
	if !s.pipeSourceAuthorized(accountID, p, sourceARN) {
		return fmt.Errorf("%w: source not authorized for pipe", ErrPipeBadReq)
	}
	queueAccount, queueName, err := s.resolveSQSEndpoint(accountID, sourceARN)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPipeBadReq, err)
	}
	msgs, err := s.ReceiveMessages(queueAccount, queueName, 10)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}
	for _, msg := range msgs {
		if !s.pipeDeliveryAuthorized(accountID, p, targetARN) {
			log.Printf("pipes delivery denied pipe=%s target=%s", p.ARN, targetARN)
			return fmt.Errorf("%w: delivery not authorized for target", ErrPipeBadReq)
		}
		body, err := s.pipeEnrichPayload(accountID, p, string(msg.Body), invoke)
		if err != nil {
			return err
		}
		if err := s.deliverPipePayload(accountID, targetARN, body, invoke); err != nil {
			return err
		}
		if err := s.DeleteMessage(queueAccount, queueName, msg.ReceiptHandle); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pollPipeFromDynamoStream(accountID string, p Pipe, sourceARN, targetARN string, invoke PipeInvokeFunc) error {
	if !s.pipeSourceAuthorized(accountID, p, sourceARN) {
		return fmt.Errorf("%w: source not authorized for pipe", ErrPipeBadReq)
	}
	acct, tableName, label, ok := ParseDynamoStreamARN(sourceARN)
	if !ok || acct != accountID {
		return fmt.Errorf("%w: invalid DynamoDB stream ARN", ErrPipeBadReq)
	}
	if _, err := s.DescribeDynamoStream(acct, tableName, label); err != nil {
		return err
	}
	iterator := p.SourceCursor
	if iterator == "" {
		it, err := s.GetDynamoStreamShardIterator(acct, tableName, LabDynamoStreamShardID, "TRIM_HORIZON", "")
		if err != nil {
			return err
		}
		iterator = it
	}
	records, next, err := s.GetDynamoStreamRecords(iterator, 10)
	if err != nil {
		return err
	}
	_ = s.SetPipeSourceCursor(accountID, p.Name, next)
	for _, rec := range records {
		payloadMap := map[string]any{
			"eventName":      rec.EventName,
			"sequenceNumber": rec.SequenceNumber,
			"keys":           json.RawMessage(rec.KeysJSON),
		}
		if rec.NewImageJSON != "" {
			payloadMap["newImage"] = json.RawMessage(rec.NewImageJSON)
		}
		payload, _ := json.Marshal(payloadMap)
		if !s.pipeDeliveryAuthorized(accountID, p, targetARN) {
			log.Printf("pipes delivery denied pipe=%s target=%s", p.ARN, targetARN)
			return fmt.Errorf("%w: delivery not authorized for target", ErrPipeBadReq)
		}
		body, err := s.pipeEnrichPayload(accountID, p, string(payload), invoke)
		if err != nil {
			return err
		}
		if err := s.deliverPipePayload(accountID, targetARN, body, invoke); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pollPipeFromEventBus(accountID string, p Pipe, sourceARN, targetARN string, invoke PipeInvokeFunc) error {
	if !s.pipeSourceAuthorized(accountID, p, sourceARN) {
		return fmt.Errorf("%w: source not authorized for pipe", ErrPipeBadReq)
	}
	_, busAccount, busName, ok := ParseEventBusARN(sourceARN)
	if !ok || busAccount != accountID {
		return fmt.Errorf("%w: invalid EventBridge bus source ARN", ErrPipeBadReq)
	}
	if _, err := s.GetEventBus(accountID, busName); err != nil {
		return err
	}
	cursor := strings.TrimSpace(p.SourceCursor)
	rows, err := s.db.Query(
		`SELECT entry_id, source, detail_type, detail_json, created_at FROM event_entries
		 WHERE account_id = ? AND bus_name = ? AND entry_id > ?
		 ORDER BY entry_id LIMIT 10`,
		accountID, busName, cursor,
	)
	if err != nil {
		return fmt.Errorf("pipe event bus source: %w", err)
	}
	type busEntry struct {
		entryID, source, detailType, detailJSON, created string
	}
	var batch []busEntry
	for rows.Next() {
		var e busEntry
		if err := rows.Scan(&e.entryID, &e.source, &e.detailType, &e.detailJSON, &e.created); err != nil {
			_ = rows.Close()
			return err
		}
		batch = append(batch, e)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	var lastID string
	for _, e := range batch {
		body, err := eventBridgeDeliveryBody(accountID, e.entryID, DefaultEventsRegion, e.source, e.detailType, e.detailJSON, e.created)
		if err != nil {
			return err
		}
		if !s.pipeDeliveryAuthorized(accountID, p, targetARN) {
			return fmt.Errorf("%w: delivery not authorized for target", ErrPipeBadReq)
		}
		enriched, err := s.pipeEnrichPayload(accountID, p, body, invoke)
		if err != nil {
			return err
		}
		if err := s.deliverPipePayload(accountID, targetARN, enriched, invoke); err != nil {
			return err
		}
		lastID = e.entryID
	}
	if lastID != "" {
		_ = s.SetPipeSourceCursor(accountID, p.Name, lastID)
	}
	return nil
}

// pipeEnrichPayload optionally invokes Enrichment Lambda and returns its payload (or original body).
// Lab enrichment is sync via the same invoke callback; when EnrichmentARN is set and invoke is nil, fails.
func (s *Store) pipeEnrichPayload(accountID string, p Pipe, body string, invoke PipeInvokeFunc) (string, error) {
	enrichARN := strings.TrimSpace(p.EnrichmentARN)
	if enrichARN == "" {
		return body, nil
	}
	if !s.pipeEnrichmentAuthorized(accountID, p, enrichARN) {
		return "", fmt.Errorf("%w: enrichment not authorized for pipe", ErrPipeBadReq)
	}
	fnAccount, fnName, ok := ParseLambdaARNFromSFNResource(enrichARN)
	if !ok || fnName == "" {
		return "", fmt.Errorf("%w: invalid Enrichment ARN", ErrPipeBadReq)
	}
	if fnAccount == "" {
		fnAccount = accountID
	}
	if invoke == nil {
		return "", fmt.Errorf("pipes: enrichment invoke callback is required")
	}
	resultJSON, err := invoke(fnAccount, fnName, body)
	if err != nil {
		return "", fmt.Errorf("pipes enrichment: %w", err)
	}
	if strings.TrimSpace(resultJSON) != "" {
		return resultJSON, nil
	}
	return body, nil
}

func (s *Store) deliverPipePayload(accountID, targetARN, body string, invoke PipeInvokeFunc) error {
	switch {
	case strings.Contains(targetARN, ":sqs:"):
		queueAccount, qName, err := s.resolveSQSEndpoint(accountID, targetARN)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPipeBadReq, err)
		}
		_, err = s.SendMessage(queueAccount, qName, []byte(body), false, nil, "", nil)
		return err
	case strings.Contains(targetARN, ":lambda:"):
		fnAccount, fnName, ok := ParseLambdaARNFromSFNResource(targetARN)
		if !ok || fnName == "" {
			return fmt.Errorf("%w: invalid Lambda target ARN", ErrPipeBadReq)
		}
		if fnAccount == "" {
			fnAccount = accountID
		}
		if invoke == nil {
			return fmt.Errorf("pipes: lambda invoke callback is required")
		}
		_, err := invoke(fnAccount, fnName, body)
		return err
	default:
		return fmt.Errorf("%w: unsupported Target ARN", ErrPipeBadReq)
	}
}

// pipeSourceAuthorized requires a RoleArn session Allow on source poll actions,
// or (SQS) a source queue policy Allow for pipes.amazonaws.com with SourceArn=pipe.
func (s *Store) pipeSourceAuthorized(accountID string, p Pipe, sourceARN string) bool {
	sourceARN = strings.TrimSpace(sourceARN)
	roleARN := strings.TrimSpace(p.RoleARN)
	switch {
	case strings.Contains(sourceARN, ":sqs:"):
		if roleARN != "" {
			if s.deliveryRoleSessionAllows(accountID, roleARN, actionSQSReceiveMessage, sourceARN, "pipes-source", DefaultPipesRegion) &&
				s.deliveryRoleSessionAllows(accountID, roleARN, actionSQSDeleteMessage, sourceARN, "pipes-source", DefaultPipesRegion) {
				return true
			}
		}
		return s.deliveryTargetResourcePolicyAllows(accountID, sourceARN, actionSQSReceiveMessage, authz.ServicePrincipalPipes, p.ARN) &&
			s.deliveryTargetResourcePolicyAllows(accountID, sourceARN, actionSQSDeleteMessage, authz.ServicePrincipalPipes, p.ARN)
	case strings.Contains(sourceARN, ":dynamodb:") && strings.Contains(sourceARN, "/stream/"):
		if roleARN == "" {
			return false
		}
		return s.deliveryRoleSessionAllows(accountID, roleARN, actionDynamoGetRecords, sourceARN, "pipes-source", DefaultPipesRegion)
	case strings.Contains(sourceARN, ":events:") && strings.Contains(sourceARN, ":event-bus/"):
		// Lab bus sources require a RoleArn that exists in-account (full events:Retrieve*
		// surface is not modeled). Resource-policy-only bus drain is rejected.
		if roleARN == "" {
			return false
		}
		roleAccountID, roleName, ok := sts.ParseRoleARN(roleARN)
		if !ok || roleAccountID != accountID {
			return false
		}
		_, _, err := s.GetRole(accountID, roleName)
		return err == nil
	default:
		return false
	}
}

// pipeEnrichmentAuthorized requires RoleArn session Allow on lambda:InvokeFunction
// for the enrichment ARN (fail closed when Enrichment is set without RoleArn).
func (s *Store) pipeEnrichmentAuthorized(accountID string, p Pipe, enrichARN string) bool {
	roleARN := strings.TrimSpace(p.RoleARN)
	if roleARN == "" {
		return false
	}
	return s.deliveryRoleSessionAllows(accountID, roleARN, actionLambdaInvokeFunction, enrichARN, "pipes-enrichment", DefaultPipesRegion)
}

// pipeDeliveryAuthorized mirrors Scheduler: RoleArn session EvaluateFull, or
// target resource policy Allow for pipes.amazonaws.com / account root.
func (s *Store) pipeDeliveryAuthorized(accountID string, p Pipe, targetARN string) bool {
	arn := strings.TrimSpace(targetARN)
	action, ok := deliveryActionForARN(arn)
	if !ok {
		return false
	}
	roleARN := strings.TrimSpace(p.RoleARN)
	if roleARN == "" {
		return s.deliveryTargetResourcePolicyAllows(accountID, arn, action, authz.ServicePrincipalPipes, p.ARN)
	}
	return s.deliveryRoleSessionAllows(accountID, roleARN, action, arn, "pipes-delivery", DefaultPipesRegion)
}
