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
  source_cursor TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
`

// Pipe is an EventBridge Pipe row.
type Pipe struct {
	Name         string
	ARN          string
	Description  string
	DesiredState string // RUNNING or STOPPED
	CurrentState string
	SourceARN    string
	TargetARN    string
	RoleARN      string
	SourceCursor string
	CreatedAt    int64
}

// EnsurePipesSchema creates pipes tables if missing.
func EnsurePipesSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure pipes schema: db is nil")
	}
	if _, err := db.Exec(pipesSchema); err != nil {
		return fmt.Errorf("ensure pipes schema: %w", err)
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
	name = strings.TrimSpace(name)
	if name == "" {
		return Pipe{}, fmt.Errorf("%w: Name is required", ErrPipeBadReq)
	}
	if strings.TrimSpace(sourceARN) == "" || strings.TrimSpace(targetARN) == "" {
		return Pipe{}, fmt.Errorf("%w: Source and Target are required", ErrPipeBadReq)
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
		`INSERT INTO pipes (account_id, name, arn, description, desired_state, current_state, source_arn, target_arn, role_arn, source_cursor, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		accountID, name, arn, description, desiredState, current, sourceARN, targetARN, roleARN, now,
	)
	if err != nil {
		return Pipe{}, fmt.Errorf("create pipe: %w", err)
	}
	return Pipe{
		Name: name, ARN: arn, Description: description,
		DesiredState: desiredState, CurrentState: current,
		SourceARN: sourceARN, TargetARN: targetARN, RoleARN: roleARN, CreatedAt: now,
	}, nil
}

// GetPipe returns a pipe by name.
func (s *Store) GetPipe(accountID, name string) (Pipe, error) {
	var p Pipe
	err := s.db.QueryRow(
		`SELECT name, arn, description, desired_state, current_state, source_arn, target_arn, role_arn, source_cursor, created_at
		 FROM pipes WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&p.Name, &p.ARN, &p.Description, &p.DesiredState, &p.CurrentState, &p.SourceARN, &p.TargetARN, &p.RoleARN, &p.SourceCursor, &p.CreatedAt)
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
		`SELECT name, arn, description, desired_state, current_state, source_arn, target_arn, role_arn, source_cursor, created_at
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
		if err := rows.Scan(&p.Name, &p.ARN, &p.Description, &p.DesiredState, &p.CurrentState, &p.SourceARN, &p.TargetARN, &p.RoleARN, &p.SourceCursor, &p.CreatedAt); err != nil {
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
		`SELECT account_id, name, arn, description, desired_state, current_state, source_arn, target_arn, role_arn, source_cursor, created_at
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
		if err := rows.Scan(&accountID, &p.Name, &p.ARN, &p.Description, &p.DesiredState, &p.CurrentState, &p.SourceARN, &p.TargetARN, &p.RoleARN, &p.SourceCursor, &p.CreatedAt); err != nil {
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

// PollPipeOnce drains one batch from the pipe source toward the target.
// invoke is called for Lambda targets with (accountID, functionName, payloadJSON).
// SQS targets use SendMessage directly. Returns nil when there is nothing to drain.
func (s *Store) PollPipeOnce(accountID, pipeName string, invoke func(accountID, functionName, payloadJSON string) error) error {
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
	default:
		return fmt.Errorf("%w: unsupported Source ARN", ErrPipeBadReq)
	}
}

func (s *Store) pollPipeFromSQS(accountID string, p Pipe, sourceARN, targetARN string, invoke func(accountID, functionName, payloadJSON string) error) error {
	queueName, err := queueNameFromARN(sourceARN)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPipeBadReq, err)
	}
	msgs, err := s.ReceiveMessages(accountID, queueName, 10)
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
		if err := s.deliverPipePayload(accountID, targetARN, string(msg.Body), invoke); err != nil {
			return err
		}
		if err := s.DeleteMessage(accountID, queueName, msg.ReceiptHandle); err != nil {
			return err
		}
	}
	_ = p
	return nil
}

func (s *Store) pollPipeFromDynamoStream(accountID string, p Pipe, sourceARN, targetARN string, invoke func(accountID, functionName, payloadJSON string) error) error {
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
		if err := s.deliverPipePayload(accountID, targetARN, string(payload), invoke); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) deliverPipePayload(accountID, targetARN, body string, invoke func(accountID, functionName, payloadJSON string) error) error {
	switch {
	case strings.Contains(targetARN, ":sqs:"):
		qName, err := queueNameFromARN(targetARN)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPipeBadReq, err)
		}
		_, err = s.SendMessage(accountID, qName, []byte(body), false, nil, "", nil)
		return err
	case strings.Contains(targetARN, ":lambda:"):
		_, fnName, ok := ParseLambdaARNFromSFNResource(targetARN)
		if !ok || fnName == "" {
			return fmt.Errorf("%w: invalid Lambda target ARN", ErrPipeBadReq)
		}
		if invoke == nil {
			return fmt.Errorf("pipes: lambda invoke callback is required")
		}
		return invoke(accountID, fnName, body)
	default:
		return fmt.Errorf("%w: unsupported Target ARN", ErrPipeBadReq)
	}
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
		return s.deliveryTargetResourcePolicyAllows(accountID, arn, action, authz.ServicePrincipalPipes)
	}
	return s.deliveryRoleSessionAllows(accountID, roleARN, action, arn, "pipes-delivery", DefaultPipesRegion)
}
