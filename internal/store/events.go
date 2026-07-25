package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/google/uuid"
)

const (
	actionSQSSendMessage       = "sqs:SendMessage"
	actionLambdaInvokeFunction = "lambda:InvokeFunction"
	actionSNSPublish           = "sns:Publish"
	actionLogsPutLogEvents     = "logs:PutLogEvents"
	actionKinesisPutRecord     = "kinesis:PutRecord"
	actionSFNStartExecution    = "states:StartExecution"
	actionEventsPutEvents      = "events:PutEvents"
)

// LabEventBridgeLogStream is the auto-created stream name for CloudWatch Logs targets.
const LabEventBridgeLogStream = "eventbridge"

const (
	// DefaultEventsRegion is the lab region embedded in EventBridge ARNs.
	DefaultEventsRegion = "us-east-1"
	// DefaultEventBusName is the per-account default event bus.
	DefaultEventBusName = "default"
	// RuleStateEnabled is an enabled rule state.
	RuleStateEnabled = "ENABLED"
	// RuleStateDisabled is a disabled rule state.
	RuleStateDisabled = "DISABLED"
)

var (
	ErrEventBusAlreadyExists       = errors.New("ResourceAlreadyExistsException")
	ErrNoSuchEventBus              = errors.New("ResourceNotFoundException")
	ErrCannotDeleteDefaultEventBus = errors.New("InvalidOperationException")
	ErrNoSuchEventRule             = errors.New("ResourceNotFoundException")
	ErrNoSuchEventTarget           = errors.New("ResourceNotFoundException")
	ErrNoSuchEventEntry            = errors.New("EventNotFound")
	// ErrEventTargetRoleArnRequired is retained for API stability. PutTargets no longer
	// requires RoleArn for Logs/Kinesis/SFN; delivery uses RoleArn or resource policy.
	ErrEventTargetRoleArnRequired = errors.New("ValidationException: RoleArn is required for this target type")
)

// EventBus is an EventBridge bus metadata row.
type EventBus struct {
	AccountID    string
	Name         string
	ARN          string
	Policy       string
	CreationDate string
}

// EventRule is an EventBridge rule metadata row.
type EventRule struct {
	AccountID   string
	BusName     string
	Name        string
	ARN         string
	Pattern     string
	State       string
	Description string
}

// EventTarget is a rule target row.
type EventTarget struct {
	AccountID              string
	BusName                string
	RuleName               string
	ID                     string
	ARN                    string
	RoleARN                string
	Input                  string
	InputPath              string
	InputTransformerJSON   string
}

// EventTargetInput is the PutTargets input shape.
type EventTargetInput struct {
	ID               string
	ARN              string
	RoleARN          string
	Input            string
	InputPath        string
	InputTransformer *EventBridgeInputTransformer
}

// PutEventsEntry is a single PutEvents entry.
type PutEventsEntry struct {
	Source       string
	DetailType   string
	Detail       string
	EventBusName string
	Time         string
}

// PutEventsResultEntry is the per-entry PutEvents outcome.
type PutEventsResultEntry struct {
	EventID      string
	ErrorCode    string
	ErrorMessage string
}

// PutEventsResult is the PutEvents response shape.
type PutEventsResult struct {
	FailedEntryCount int
	Entries          []PutEventsResultEntry
}

// EventRuleMatch records a rule/target match for a PutEvents entry.
type EventRuleMatch struct {
	EntryID   string
	RuleARN   string
	TargetARN string
	TargetID  string
}

const eventsSchema = `
CREATE TABLE IF NOT EXISTS event_buses (
  account_id TEXT NOT NULL,
  bus_name TEXT NOT NULL,
  bus_arn TEXT NOT NULL,
  policy_json TEXT NOT NULL DEFAULT '',
  creation_date TEXT NOT NULL,
  PRIMARY KEY (account_id, bus_name)
);
CREATE TABLE IF NOT EXISTS event_rules (
  account_id TEXT NOT NULL,
  bus_name TEXT NOT NULL,
  rule_name TEXT NOT NULL,
  rule_arn TEXT NOT NULL,
  pattern_json TEXT NOT NULL DEFAULT '{}',
  state TEXT NOT NULL DEFAULT 'ENABLED',
  description TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, bus_name, rule_name)
);
CREATE TABLE IF NOT EXISTS event_targets (
  account_id TEXT NOT NULL,
  bus_name TEXT NOT NULL,
  rule_name TEXT NOT NULL,
  target_id TEXT NOT NULL,
  target_arn TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  input_json TEXT NOT NULL DEFAULT '',
  input_path TEXT NOT NULL DEFAULT '',
  input_transformer_json TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, bus_name, rule_name, target_id)
);
CREATE TABLE IF NOT EXISTS event_entries (
  entry_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  bus_name TEXT NOT NULL,
  source TEXT NOT NULL,
  detail_type TEXT NOT NULL,
  detail_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS event_rule_matches (
  match_id TEXT PRIMARY KEY,
  entry_id TEXT NOT NULL,
  rule_arn TEXT NOT NULL,
  target_arn TEXT NOT NULL,
  target_id TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_event_rules_bus ON event_rules(account_id, bus_name);
CREATE INDEX IF NOT EXISTS idx_event_targets_rule ON event_targets(account_id, bus_name, rule_name);
CREATE INDEX IF NOT EXISTS idx_event_rule_matches_entry ON event_rule_matches(entry_id);
`

// EnsureEventsSchema creates EventBridge tables if missing.
func EnsureEventsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure events schema: db is nil")
	}
	if _, err := db.Exec(eventsSchema); err != nil {
		return fmt.Errorf("ensure events schema: %w", err)
	}
	if err := execMigrateStmt(db, `ALTER TABLE event_targets ADD COLUMN input_transformer_json TEXT NOT NULL DEFAULT ''`, nil); err != nil {
		return fmt.Errorf("ensure events schema: input_transformer_json: %w", err)
	}
	return nil
}

// EnsureEventsSchema ensures EventBridge tables on an open store.
func (s *Store) EnsureEventsSchema() error {
	return EnsureEventsSchema(s.db)
}

// EventBusARN builds arn:aws:events:REGION:ACCOUNT:event-bus/NAME.
func EventBusARN(region, accountID, busName string) string {
	if region == "" {
		region = DefaultEventsRegion
	}
	return fmt.Sprintf("arn:aws:events:%s:%s:event-bus/%s", region, accountID, busName)
}

// EventRuleARN builds arn:aws:events:REGION:ACCOUNT:rule[/bus]/NAME.
func EventRuleARN(region, accountID, busName, ruleName string) string {
	if region == "" {
		region = DefaultEventsRegion
	}
	busName = strings.TrimSpace(busName)
	if busName == "" || busName == DefaultEventBusName {
		return fmt.Sprintf("arn:aws:events:%s:%s:rule/%s", region, accountID, ruleName)
	}
	return fmt.Sprintf("arn:aws:events:%s:%s:rule/%s/%s", region, accountID, busName, ruleName)
}

func normalizeEventBusName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultEventBusName
	}
	return name
}

func (s *Store) ensureDefaultEventBus(accountID, region string) error {
	if region == "" {
		region = DefaultEventsRegion
	}
	_, err := s.GetEventBus(accountID, DefaultEventBusName)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNoSuchEventBus) {
		return err
	}
	created := nowRFC3339()
	arn := EventBusARN(region, accountID, DefaultEventBusName)
	_, err = s.db.Exec(
		`INSERT INTO event_buses (account_id, bus_name, bus_arn, policy_json, creation_date)
		 VALUES (?, ?, ?, '', ?)`,
		accountID, DefaultEventBusName, arn, created,
	)
	if err != nil {
		return fmt.Errorf("ensure default event bus: %w", err)
	}
	return nil
}

func scanEventBus(row *sql.Row) (EventBus, error) {
	var (
		bus    EventBus
		policy string
	)
	err := row.Scan(&bus.AccountID, &bus.Name, &bus.ARN, &policy, &bus.CreationDate)
	if errors.Is(err, sql.ErrNoRows) {
		return EventBus{}, ErrNoSuchEventBus
	}
	if err != nil {
		return EventBus{}, fmt.Errorf("scan event bus: %w", err)
	}
	bus.Policy = policy
	return bus, nil
}

// GetEventBus returns a bus by name or ErrNoSuchEventBus.
func (s *Store) GetEventBus(accountID, busName string) (EventBus, error) {
	busName = normalizeEventBusName(busName)
	row := s.db.QueryRow(
		`SELECT account_id, bus_name, bus_arn, policy_json, creation_date
		 FROM event_buses WHERE account_id = ? AND bus_name = ?`,
		accountID, busName,
	)
	return scanEventBus(row)
}

// CreateEventBus inserts a custom event bus.
func (s *Store) CreateEventBus(accountID, region, busName string) (EventBus, error) {
	busName = strings.TrimSpace(busName)
	if busName == "" {
		return EventBus{}, fmt.Errorf("create event bus: name is required")
	}
	if busName == DefaultEventBusName {
		if err := s.ensureDefaultEventBus(accountID, region); err != nil {
			return EventBus{}, err
		}
		return s.GetEventBus(accountID, DefaultEventBusName)
	}
	arn := EventBusARN(region, accountID, busName)
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO event_buses (account_id, bus_name, bus_arn, policy_json, creation_date)
		 VALUES (?, ?, ?, '', ?)`,
		accountID, busName, arn, created,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return EventBus{}, ErrEventBusAlreadyExists
		}
		return EventBus{}, fmt.Errorf("create event bus: %w", err)
	}
	return EventBus{
		AccountID:    accountID,
		Name:         busName,
		ARN:          arn,
		CreationDate: created,
	}, nil
}

// DescribeEventBus returns bus metadata, seeding the default bus when needed.
func (s *Store) DescribeEventBus(accountID, busName string) (EventBus, error) {
	busName = normalizeEventBusName(busName)
	if busName == DefaultEventBusName {
		if err := s.ensureDefaultEventBus(accountID, DefaultEventsRegion); err != nil {
			return EventBus{}, err
		}
	}
	return s.GetEventBus(accountID, busName)
}

// ListEventBuses returns all buses for an account, ensuring default exists.
func (s *Store) ListEventBuses(accountID string) ([]EventBus, error) {
	if err := s.ensureDefaultEventBus(accountID, DefaultEventsRegion); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT account_id, bus_name, bus_arn, policy_json, creation_date
		 FROM event_buses WHERE account_id = ? ORDER BY bus_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list event buses: %w", err)
	}
	defer rows.Close()
	out := []EventBus{}
	for rows.Next() {
		var (
			bus    EventBus
			policy string
		)
		if err := rows.Scan(&bus.AccountID, &bus.Name, &bus.ARN, &policy, &bus.CreationDate); err != nil {
			return nil, fmt.Errorf("list event buses: %w", err)
		}
		bus.Policy = policy
		out = append(out, bus)
	}
	return out, rows.Err()
}

// DeleteEventBus removes a custom bus and its rules/targets. Default bus cannot be deleted.
func (s *Store) DeleteEventBus(accountID, busName string) error {
	busName = normalizeEventBusName(busName)
	if busName == DefaultEventBusName {
		return ErrCannotDeleteDefaultEventBus
	}
	if _, err := s.GetEventBus(accountID, busName); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete event bus: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM event_targets WHERE account_id = ? AND bus_name = ?`,
		accountID, busName,
	); err != nil {
		return fmt.Errorf("delete event bus targets: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM event_rules WHERE account_id = ? AND bus_name = ?`,
		accountID, busName,
	); err != nil {
		return fmt.Errorf("delete event bus rules: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM event_buses WHERE account_id = ? AND bus_name = ?`,
		accountID, busName,
	); err != nil {
		return fmt.Errorf("delete event bus: %w", err)
	}
	return tx.Commit()
}

func scanEventRule(row *sql.Row) (EventRule, error) {
	var rule EventRule
	err := row.Scan(
		&rule.AccountID, &rule.BusName, &rule.Name, &rule.ARN,
		&rule.Pattern, &rule.State, &rule.Description,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EventRule{}, ErrNoSuchEventRule
	}
	if err != nil {
		return EventRule{}, fmt.Errorf("scan event rule: %w", err)
	}
	return rule, nil
}

// PutRule creates or updates a rule on a bus.
func (s *Store) PutRule(accountID, region, busName, ruleName, pattern, description, state string) (EventRule, error) {
	busName = normalizeEventBusName(busName)
	ruleName = strings.TrimSpace(ruleName)
	if ruleName == "" {
		return EventRule{}, fmt.Errorf("put rule: name is required")
	}
	if strings.TrimSpace(pattern) == "" {
		pattern = "{}"
	}
	if err := validateEventPattern(pattern); err != nil {
		return EventRule{}, err
	}
	state = normalizeRuleState(state)
	if busName == DefaultEventBusName {
		if err := s.ensureDefaultEventBus(accountID, region); err != nil {
			return EventRule{}, err
		}
	} else if _, err := s.GetEventBus(accountID, busName); err != nil {
		return EventRule{}, err
	}
	arn := EventRuleARN(region, accountID, busName, ruleName)
	_, err := s.db.Exec(
		`INSERT INTO event_rules
		 (account_id, bus_name, rule_name, rule_arn, pattern_json, state, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, bus_name, rule_name) DO UPDATE SET
		   rule_arn = excluded.rule_arn,
		   pattern_json = excluded.pattern_json,
		   state = excluded.state,
		   description = excluded.description`,
		accountID, busName, ruleName, arn, pattern, state, strings.TrimSpace(description),
	)
	if err != nil {
		return EventRule{}, fmt.Errorf("put rule: %w", err)
	}
	return EventRule{
		AccountID:   accountID,
		BusName:     busName,
		Name:        ruleName,
		ARN:         arn,
		Pattern:     pattern,
		State:       state,
		Description: strings.TrimSpace(description),
	}, nil
}

func normalizeRuleState(state string) string {
	state = strings.TrimSpace(strings.ToUpper(state))
	switch state {
	case RuleStateDisabled:
		return RuleStateDisabled
	default:
		return RuleStateEnabled
	}
}

func validateEventPattern(pattern string) error {
	return validateEventPatternDeep(pattern)
}

// DescribeRule returns a rule by bus and name.
func (s *Store) DescribeRule(accountID, busName, ruleName string) (EventRule, error) {
	busName = normalizeEventBusName(busName)
	row := s.db.QueryRow(
		`SELECT account_id, bus_name, rule_name, rule_arn, pattern_json, state, description
		 FROM event_rules WHERE account_id = ? AND bus_name = ? AND rule_name = ?`,
		accountID, busName, ruleName,
	)
	return scanEventRule(row)
}

// ListRules returns rules on a bus.
func (s *Store) ListRules(accountID, busName string) ([]EventRule, error) {
	busName = normalizeEventBusName(busName)
	if busName == DefaultEventBusName {
		if err := s.ensureDefaultEventBus(accountID, DefaultEventsRegion); err != nil {
			return nil, err
		}
	}
	rows, err := s.db.Query(
		`SELECT account_id, bus_name, rule_name, rule_arn, pattern_json, state, description
		 FROM event_rules WHERE account_id = ? AND bus_name = ? ORDER BY rule_name`,
		accountID, busName,
	)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	out := []EventRule{}
	for rows.Next() {
		var rule EventRule
		if err := rows.Scan(
			&rule.AccountID, &rule.BusName, &rule.Name, &rule.ARN,
			&rule.Pattern, &rule.State, &rule.Description,
		); err != nil {
			return nil, fmt.Errorf("list rules: %w", err)
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

// DeleteRule removes a rule and its targets.
func (s *Store) DeleteRule(accountID, busName, ruleName string) error {
	if _, err := s.DescribeRule(accountID, busName, ruleName); err != nil {
		return err
	}
	busName = normalizeEventBusName(busName)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM event_targets WHERE account_id = ? AND bus_name = ? AND rule_name = ?`,
		accountID, busName, ruleName,
	); err != nil {
		return fmt.Errorf("delete rule targets: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM event_rules WHERE account_id = ? AND bus_name = ? AND rule_name = ?`,
		accountID, busName, ruleName,
	); err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	return tx.Commit()
}

// EnableRule sets rule state to ENABLED.
func (s *Store) EnableRule(accountID, busName, ruleName string) error {
	return s.setRuleState(accountID, busName, ruleName, RuleStateEnabled)
}

// DisableRule sets rule state to DISABLED.
func (s *Store) DisableRule(accountID, busName, ruleName string) error {
	return s.setRuleState(accountID, busName, ruleName, RuleStateDisabled)
}

func (s *Store) setRuleState(accountID, busName, ruleName, state string) error {
	if _, err := s.DescribeRule(accountID, busName, ruleName); err != nil {
		return err
	}
	busName = normalizeEventBusName(busName)
	_, err := s.db.Exec(
		`UPDATE event_rules SET state = ? WHERE account_id = ? AND bus_name = ? AND rule_name = ?`,
		state, accountID, busName, ruleName,
	)
	if err != nil {
		return fmt.Errorf("set rule state: %w", err)
	}
	return nil
}

// PutTargets upserts targets on a rule.
func (s *Store) PutTargets(accountID, busName, ruleName string, targets []EventTargetInput) error {
	if _, err := s.DescribeRule(accountID, busName, ruleName); err != nil {
		return err
	}
	busName = normalizeEventBusName(busName)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("put targets: %w", err)
	}
	defer tx.Rollback()
	for _, tgt := range targets {
		id := strings.TrimSpace(tgt.ID)
		arn := strings.TrimSpace(tgt.ARN)
		if id == "" || arn == "" {
			return fmt.Errorf("put targets: target id and arn are required")
		}
		roleARN := strings.TrimSpace(tgt.RoleARN)
		if roleARN == "" && eventTargetRequiresRoleARN(arn) {
			return ErrEventTargetRoleArnRequired
		}
		transformerJSON, err := marshalEventBridgeInputTransformer(tgt.InputTransformer)
		if err != nil {
			return fmt.Errorf("put targets: InputTransformer: %w", err)
		}
		if transformerJSON != "" && (strings.TrimSpace(tgt.Input) != "" || strings.TrimSpace(tgt.InputPath) != "") {
			return fmt.Errorf("put targets: InputTransformer cannot be combined with Input or InputPath")
		}
		_, err = tx.Exec(
			`INSERT INTO event_targets
			 (account_id, bus_name, rule_name, target_id, target_arn, role_arn, input_json, input_path, input_transformer_json)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, bus_name, rule_name, target_id) DO UPDATE SET
			   target_arn = excluded.target_arn,
			   role_arn = excluded.role_arn,
			   input_json = excluded.input_json,
			   input_path = excluded.input_path,
			   input_transformer_json = excluded.input_transformer_json`,
			accountID, busName, ruleName, id, arn,
			roleARN,
			strings.TrimSpace(tgt.Input),
			strings.TrimSpace(tgt.InputPath),
			transformerJSON,
		)
		if err != nil {
			return fmt.Errorf("put targets: %w", err)
		}
	}
	return tx.Commit()
}

// RemoveTargets deletes targets by id from a rule.
func (s *Store) RemoveTargets(accountID, busName, ruleName string, targetIDs []string) error {
	if _, err := s.DescribeRule(accountID, busName, ruleName); err != nil {
		return err
	}
	busName = normalizeEventBusName(busName)
	for _, id := range targetIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		res, err := s.db.Exec(
			`DELETE FROM event_targets
			 WHERE account_id = ? AND bus_name = ? AND rule_name = ? AND target_id = ?`,
			accountID, busName, ruleName, id,
		)
		if err != nil {
			return fmt.Errorf("remove targets: %w", err)
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			return ErrNoSuchEventTarget
		}
	}
	return nil
}

// ListTargetsByRule returns targets attached to a rule.
func (s *Store) ListTargetsByRule(accountID, busName, ruleName string) ([]EventTarget, error) {
	if _, err := s.DescribeRule(accountID, busName, ruleName); err != nil {
		return nil, err
	}
	busName = normalizeEventBusName(busName)
	rows, err := s.db.Query(
		`SELECT account_id, bus_name, rule_name, target_id, target_arn, role_arn, input_json, input_path,
		        COALESCE(input_transformer_json, '')
		 FROM event_targets WHERE account_id = ? AND bus_name = ? AND rule_name = ?
		 ORDER BY target_id`,
		accountID, busName, ruleName,
	)
	if err != nil {
		return nil, fmt.Errorf("list targets by rule: %w", err)
	}
	defer rows.Close()
	out := []EventTarget{}
	for rows.Next() {
		var tgt EventTarget
		if err := rows.Scan(
			&tgt.AccountID, &tgt.BusName, &tgt.RuleName, &tgt.ID, &tgt.ARN,
			&tgt.RoleARN, &tgt.Input, &tgt.InputPath, &tgt.InputTransformerJSON,
		); err != nil {
			return nil, fmt.Errorf("list targets by rule: %w", err)
		}
		out = append(out, tgt)
	}
	return out, rows.Err()
}

// PutEvents ingests events, matches enabled rules, records target matches, and stubs delivery.
func (s *Store) PutEvents(accountID string, entries []PutEventsEntry) (PutEventsResult, error) {
	result := PutEventsResult{
		Entries: make([]PutEventsResultEntry, 0, len(entries)),
	}
	for _, entry := range entries {
		out, err := s.putEventsEntry(accountID, entry)
		if err != nil {
			result.FailedEntryCount++
			result.Entries = append(result.Entries, PutEventsResultEntry{
				ErrorCode:    "InternalFailure",
				ErrorMessage: err.Error(),
			})
			continue
		}
		result.Entries = append(result.Entries, out)
	}
	return result, nil
}

func (s *Store) putEventsEntry(accountID string, entry PutEventsEntry) (PutEventsResultEntry, error) {
	busName := normalizeEventBusName(entry.EventBusName)
	if busName == DefaultEventBusName {
		if err := s.ensureDefaultEventBus(accountID, DefaultEventsRegion); err != nil {
			return PutEventsResultEntry{}, err
		}
	} else if _, err := s.GetEventBus(accountID, busName); err != nil {
		return PutEventsResultEntry{}, err
	}

	detail := strings.TrimSpace(entry.Detail)
	if detail == "" {
		detail = "{}"
	}
	if !json.Valid([]byte(detail)) {
		return PutEventsResultEntry{}, fmt.Errorf("put events: detail must be valid json")
	}

	source := strings.TrimSpace(entry.Source)
	detailType := strings.TrimSpace(entry.DetailType)
	entryID := uuid.NewString()
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO event_entries
		 (entry_id, account_id, bus_name, source, detail_type, detail_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entryID, accountID, busName, source, detailType, detail, created,
	)
	if err != nil {
		return PutEventsResultEntry{}, fmt.Errorf("put events: %w", err)
	}

	if err := s.matchAndRecordEventTargets(accountID, busName, entryID, source, detailType, detail, created); err != nil {
		return PutEventsResultEntry{}, err
	}
	return PutEventsResultEntry{EventID: entryID}, nil
}

func (s *Store) matchAndRecordEventTargets(accountID, busName, entryID, source, detailType, detailJSON, created string) error {
	rules, err := s.ListRules(accountID, busName)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.State != RuleStateEnabled {
			continue
		}
		ok, err := matchEventPattern(rule.Pattern, source, detailType, detailJSON)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		targets, err := s.ListTargetsByRule(accountID, busName, rule.Name)
		if err != nil {
			return err
		}
		for _, tgt := range targets {
			if !s.eventTargetDeliveryAuthorized(accountID, rule.ARN, tgt) {
				log.Printf("events delivery skipped entry=%s rule=%s target=%s reason=unauthorized",
					entryID, rule.ARN, tgt.ARN)
				continue
			}
			if err := s.recordEventRuleMatch(entryID, rule.ARN, tgt.ARN, tgt.ID, created); err != nil {
				return err
			}
			s.deliverEventTarget(accountID, entryID, rule, tgt, source, detailType, detailJSON, created)
		}
	}
	return nil
}

func (s *Store) recordEventRuleMatch(entryID, ruleARN, targetARN, targetID, created string) error {
	_, err := s.db.Exec(
		`INSERT INTO event_rule_matches (match_id, entry_id, rule_arn, target_arn, target_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), entryID, ruleARN, targetARN, targetID, created,
	)
	if err != nil {
		return fmt.Errorf("record event rule match: %w", err)
	}
	return nil
}

// deliverEventTarget fans out a matched event to supported targets.
// Callers must authorize delivery first via eventTargetDeliveryAuthorized.
// Without RoleArn, delivery requires the target resource policy to Allow
// events.amazonaws.com or the account root for the target action (best-effort skip).
// With RoleArn, PassRole is enforced at PutTargets and delivery mints a role session
// then requires the role identity policies to Allow the target action.
// Constant Input overrides the envelope. Otherwise InputTransformer, then InputPath.
func (s *Store) deliverEventTarget(accountID, entryID string, rule EventRule, tgt EventTarget, source, detailType, detailJSON, created string) {
	body, err := eventBridgeDeliveryBody(accountID, entryID, DefaultEventsRegion, source, detailType, detailJSON, created)
	if err != nil {
		log.Printf("events delivery marshal failed entry=%s target=%s err=%v", entryID, tgt.ARN, err)
		return
	}
	if input := strings.TrimSpace(tgt.Input); input != "" {
		body = input
	} else if tr, ok, trErr := parseEventBridgeInputTransformerJSON(tgt.InputTransformerJSON); trErr != nil {
		log.Printf("events InputTransformer parse failed entry=%s target=%s err=%v", entryID, tgt.ARN, trErr)
		return
	} else if ok {
		transformed, trErr := applyEventBridgeInputTransformer(body, tr)
		if trErr != nil {
			log.Printf("events InputTransformer failed entry=%s target=%s err=%v", entryID, tgt.ARN, trErr)
			return
		}
		body = transformed
	} else if path := strings.TrimSpace(tgt.InputPath); path != "" {
		extracted, pathErr := applyEventBridgeInputPath(body, path)
		if pathErr != nil {
			log.Printf("events InputPath failed entry=%s target=%s path=%s err=%v", entryID, tgt.ARN, path, pathErr)
			return
		}
		body = extracted
	}
	arn := strings.TrimSpace(tgt.ARN)
	var deliverErr error
	switch {
	case strings.HasPrefix(arn, "arn:aws:sqs:"):
		queueAccount, queueName, err := s.resolveSQSEndpoint(accountID, arn)
		if err != nil {
			deliverErr = err
			break
		}
		_, deliverErr = s.SendMessage(queueAccount, queueName, []byte(body), false, nil, "", nil)
	case strings.HasPrefix(arn, "arn:aws:lambda:"):
		fnAccount, functionName, ok := ParseLambdaARNFromSFNResource(arn)
		if !ok || functionName == "" {
			deliverErr = fmt.Errorf("empty function name")
			break
		}
		if fnAccount == "" {
			fnAccount = accountID
		}
		_, qualifier := ParseFunctionQualifier(arn)
		_, deliverErr = s.EnqueueAsyncInvoke(fnAccount, functionName, qualifier, body)
	case strings.HasPrefix(arn, "arn:aws:sns:"):
		topic, err := s.GetTopicByARN(arn)
		if err != nil {
			deliverErr = err
			break
		}
		_, deliverErr = s.Publish(topic.AccountID, topic.TopicName, body, "", nil)
	case strings.HasPrefix(arn, "arn:aws:logs:"):
		deliverErr = s.deliverEventTargetToLogs(accountID, arn, body, source, created)
	case strings.HasPrefix(arn, "arn:aws:kinesis:"):
		deliverErr = s.deliverEventTargetToKinesis(accountID, arn, body, entryID)
	case strings.Contains(arn, ":stateMachine:"):
		deliverErr = s.deliverEventTargetToSFN(accountID, arn, body)
	default:
		_ = rule
		return
	}
	if deliverErr != nil {
		log.Printf("events delivery failed entry=%s rule=%s target=%s err=%v",
			entryID, rule.ARN, arn, deliverErr)
	}
}

func (s *Store) deliverEventTargetToLogs(accountID, logGroupARN, body, source, created string) error {
	group, err := parseLogGroupNameFromARN(logGroupARN)
	if err != nil {
		return err
	}
	logsAccount := accountID
	if owner := resourceOwnerAccountFromARN(logGroupARN); owner != "" {
		logsAccount = owner
	}
	if _, err := s.getLogGroup(logsAccount, group); err != nil {
		return err
	}
	if _, err := s.getLogStream(logsAccount, group, LabEventBridgeLogStream); errors.Is(err, ErrLogStreamNotFound) {
		if _, err := s.CreateLogStream(logsAccount, DefaultEventsRegion, group, LabEventBridgeLogStream); err != nil && !errors.Is(err, ErrLogStreamAlreadyExists) {
			return err
		}
	} else if err != nil {
		return err
	}
	st, err := s.getLogStream(logsAccount, group, LabEventBridgeLogStream)
	if err != nil {
		return err
	}
	ts := time.Now().UTC().UnixMilli()
	if t, perr := time.Parse(time.RFC3339, created); perr == nil {
		ts = t.UnixMilli()
	}
	msg := body
	if strings.TrimSpace(source) != "" {
		// AWS without InputTransformer uses event payload as message.
		_ = source
	}
	_, _, err = s.PutLogEvents(logsAccount, group, LabEventBridgeLogStream, st.UploadSequenceToken, []LogEvent{{
		Timestamp: ts,
		Message:   msg,
	}})
	return err
}

func (s *Store) deliverEventTargetToKinesis(accountID, streamARN, body, entryID string) error {
	name, err := parseKinesisStreamNameFromARN(streamARN)
	if err != nil {
		return err
	}
	streamAccount := accountID
	if owner := resourceOwnerAccountFromARN(streamARN); owner != "" {
		streamAccount = owner
	}
	partitionKey := entryID
	if partitionKey == "" {
		partitionKey = uuid.NewString()
	}
	_, _, err = s.PutKinesisRecord(streamAccount, name, partitionKey, []byte(body))
	return err
}

func (s *Store) deliverEventTargetToSFN(accountID, stateMachineARN, body string) error {
	smAccount := accountID
	if owner := resourceOwnerAccountFromARN(stateMachineARN); owner != "" {
		smAccount = owner
	}
	_, err := s.StartSFNExecution(smAccount, DefaultSFNRegion, stateMachineARN, "", body, s.getSFNTaskInvoker())
	return err
}

func parseLogGroupNameFromARN(arn string) (string, error) {
	// arn:aws:logs:region:account:log-group:NAME[:*]
	const marker = ":log-group:"
	i := strings.Index(arn, marker)
	if i < 0 {
		return "", fmt.Errorf("invalid logs ARN")
	}
	rest := arn[i+len(marker):]
	rest = strings.TrimSuffix(rest, ":*")
	if j := strings.Index(rest, ":log-stream:"); j >= 0 {
		rest = rest[:j]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", fmt.Errorf("invalid logs ARN")
	}
	return rest, nil
}

func parseKinesisStreamNameFromARN(arn string) (string, error) {
	// arn:aws:kinesis:region:account:stream/NAME
	const marker = ":stream/"
	i := strings.Index(arn, marker)
	if i < 0 {
		return "", fmt.Errorf("invalid kinesis ARN")
	}
	name := strings.TrimSpace(arn[i+len(marker):])
	if name == "" {
		return "", fmt.Errorf("invalid kinesis ARN")
	}
	return name, nil
}

func (s *Store) eventTargetDeliveryAuthorized(accountID, ruleARN string, tgt EventTarget) bool {
	arn := strings.TrimSpace(tgt.ARN)
	action, ok := eventTargetDeliveryAction(arn)
	if !ok {
		return false
	}
	roleARN := strings.TrimSpace(tgt.RoleARN)
	// Without RoleArn: SQS/Lambda/SNS/Logs/Kinesis/SFN deliver only when the destination
	// resource policy Allows events.amazonaws.com (or account root) with SourceArn keys.
	// Empty/missing policy skips delivery (fail-closed; not an open proxy).
	if roleARN == "" {
		if eventTargetRequiresRoleARN(arn) {
			return false
		}
		return s.deliveryTargetResourcePolicyAllows(accountID, arn, action, authz.ServicePrincipalEvents, ruleARN)
	}
	return s.deliveryAuthorizedRoleAndResource(
		accountID, roleARN, action, arn, authz.ServicePrincipalEvents, ruleARN, "events-delivery", DefaultEventsRegion,
	)
}

// eventTargetRequiresRoleARN reports target types that still require RoleArn at PutTargets.
// Logs/Kinesis/SFN authorize at delivery via RoleArn session or resource policy, like SQS.
func eventTargetRequiresRoleARN(targetARN string) bool {
	_ = targetARN
	return false
}

func (s *Store) identityPolicyDocsForRoleARN(roleARN string) ([]string, error) {
	docs, err := s.ListAttachedPolicyDocuments(roleARN)
	if err != nil {
		return nil, err
	}
	inline, err := s.ListInlinePolicies(roleARN)
	if err != nil {
		return nil, err
	}
	for _, p := range inline {
		docs = append(docs, p.Document)
	}
	if docs == nil {
		docs = []string{}
	}
	return docs, nil
}

func randomHexSecret(nbytes int) (string, error) {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func eventTargetDeliveryAction(targetARN string) (string, bool) {
	switch {
	case strings.HasPrefix(targetARN, "arn:aws:sqs:"):
		return actionSQSSendMessage, true
	case strings.HasPrefix(targetARN, "arn:aws:lambda:"):
		return actionLambdaInvokeFunction, true
	case strings.HasPrefix(targetARN, "arn:aws:sns:"):
		return actionSNSPublish, true
	case strings.HasPrefix(targetARN, "arn:aws:logs:"):
		return actionLogsPutLogEvents, true
	case strings.HasPrefix(targetARN, "arn:aws:kinesis:"):
		return actionKinesisPutRecord, true
	case strings.Contains(targetARN, ":stateMachine:"):
		return actionSFNStartExecution, true
	default:
		return "", false
	}
}

func eventBridgeDeliveryBody(accountID, entryID, region, source, detailType, detailJSON, created string) (string, error) {
	var detail any
	if err := json.Unmarshal([]byte(detailJSON), &detail); err != nil {
		detail = map[string]any{}
	}
	payload := map[string]any{
		"version":     "0",
		"id":          entryID,
		"detail-type": detailType,
		"source":      source,
		"account":     accountID,
		"time":        created,
		"region":      region,
		"resources":   []string{},
		"detail":      detail,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal eventbridge delivery body: %w", err)
	}
	return string(raw), nil
}

// GetPutEventsMatches returns recorded rule/target matches for a PutEvents entry.
func (s *Store) GetPutEventsMatches(entryID string) ([]EventRuleMatch, error) {
	rows, err := s.db.Query(
		`SELECT entry_id, rule_arn, target_arn, target_id
		 FROM event_rule_matches WHERE entry_id = ? ORDER BY target_id`,
		entryID,
	)
	if err != nil {
		return nil, fmt.Errorf("get put events matches: %w", err)
	}
	defer rows.Close()
	out := []EventRuleMatch{}
	for rows.Next() {
		var m EventRuleMatch
		if err := rows.Scan(&m.EntryID, &m.RuleARN, &m.TargetARN, &m.TargetID); err != nil {
			return nil, fmt.Errorf("get put events matches: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM event_entries WHERE entry_id = ?`, entryID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoSuchEventEntry
		}
		return nil, fmt.Errorf("get put events matches: %w", err)
	}
	return out, nil
}

func matchEventPattern(patternJSON, source, detailType, detailJSON string) (bool, error) {
	return matchEventPatternJSON(patternJSON, source, detailType, detailJSON)
}
