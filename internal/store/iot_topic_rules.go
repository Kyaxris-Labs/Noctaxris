package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// IoTTopicRule is a lab IoT topic rule (Rules engine lite).
type IoTTopicRule struct {
	AccountID    string
	Region       string
	RuleName     string
	RuleARN      string
	SQL          string
	Description  string
	RuleDisabled bool
	ActionsJSON  string
	CreatedAt    int64
}

// IoTTopicRuleARN builds arn:aws:iot:REGION:ACCOUNT:rule/NAME
func IoTTopicRuleARN(region, accountID, ruleName string) string {
	return fmt.Sprintf("arn:aws:iot:%s:%s:rule/%s", iotRegion(region), accountID, ruleName)
}

// ExtractIoTTopicFilter parses a minimal SELECT ... FROM 'topic/filter' SQL string.
// Returns empty string when the FROM clause topic is missing or malformed.
func ExtractIoTTopicFilter(sqlText string) string {
	if sqlText == "" {
		return ""
	}
	upper := strings.ToUpper(sqlText)
	from := strings.Index(upper, " FROM ")
	if from < 0 {
		return ""
	}
	tail := strings.TrimSpace(sqlText[from+len(" FROM "):])
	if len(tail) < 2 {
		return ""
	}
	quote := tail[0]
	if quote != '\'' && quote != '"' {
		return ""
	}
	end := strings.IndexByte(tail[1:], quote)
	if end < 0 {
		return ""
	}
	return tail[1 : 1+end]
}

// IoTTopicMatches implements MQTT-style + / # filter matching against a topic.
func IoTTopicMatches(filter, topic string) bool {
	if filter == "" || topic == "" {
		return false
	}
	filterParts := strings.Split(filter, "/")
	topicParts := strings.Split(topic, "/")
	i := 0
	for ; i < len(filterParts); i++ {
		part := filterParts[i]
		if part == "#" {
			return i == len(filterParts)-1
		}
		if i >= len(topicParts) {
			return false
		}
		if part != "+" && part != topicParts[i] {
			return false
		}
	}
	return i == len(topicParts)
}

// CreateIoTTopicRule stores a new topic rule.
func (s *Store) CreateIoTTopicRule(accountID, region, ruleName, sqlText, description, actionsJSON string, ruleDisabled bool) (IoTTopicRule, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTTopicRule{}, err
	}
	region = iotRegion(region)
	ruleName = strings.TrimSpace(ruleName)
	sqlText = strings.TrimSpace(sqlText)
	if ruleName == "" {
		return IoTTopicRule{}, fmt.Errorf("%w: ruleName required", ErrIoTBadRequest)
	}
	if sqlText == "" {
		return IoTTopicRule{}, fmt.Errorf("%w: sql required", ErrIoTBadRequest)
	}
	if ExtractIoTTopicFilter(sqlText) == "" {
		return IoTTopicRule{}, fmt.Errorf("%w: sql must be SELECT ... FROM 'topic/filter'", ErrIoTBadRequest)
	}
	actionsJSON = normalizeActionsJSON(actionsJSON)
	if _, err := s.GetIoTTopicRule(accountID, region, ruleName); err == nil {
		return IoTTopicRule{}, ErrIoTConflict
	} else if !errors.Is(err, ErrIoTNotFound) {
		return IoTTopicRule{}, err
	}
	now := time.Now().UTC().UnixMilli()
	arn := IoTTopicRuleARN(region, accountID, ruleName)
	disabled := 0
	if ruleDisabled {
		disabled = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO iot_topic_rules
		 (account_id, region, rule_name, rule_arn, sql_text, description, rule_disabled, actions_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, ruleName, arn, sqlText, description, disabled, actionsJSON, now,
	)
	if err != nil {
		return IoTTopicRule{}, fmt.Errorf("create topic rule: %w", err)
	}
	return IoTTopicRule{
		AccountID: accountID, Region: region, RuleName: ruleName, RuleARN: arn,
		SQL: sqlText, Description: description, RuleDisabled: ruleDisabled,
		ActionsJSON: actionsJSON, CreatedAt: now,
	}, nil
}

// ReplaceIoTTopicRule replaces an existing topic rule (preserves createdAt).
func (s *Store) ReplaceIoTTopicRule(accountID, region, ruleName, sqlText, description, actionsJSON string, ruleDisabled bool) (IoTTopicRule, error) {
	existing, err := s.GetIoTTopicRule(accountID, region, ruleName)
	if err != nil {
		return IoTTopicRule{}, err
	}
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return IoTTopicRule{}, fmt.Errorf("%w: sql required", ErrIoTBadRequest)
	}
	if ExtractIoTTopicFilter(sqlText) == "" {
		return IoTTopicRule{}, fmt.Errorf("%w: sql must be SELECT ... FROM 'topic/filter'", ErrIoTBadRequest)
	}
	actionsJSON = normalizeActionsJSON(actionsJSON)
	disabled := 0
	if ruleDisabled {
		disabled = 1
	}
	_, err = s.db.Exec(
		`UPDATE iot_topic_rules SET sql_text = ?, description = ?, rule_disabled = ?, actions_json = ?
		 WHERE account_id = ? AND region = ? AND rule_name = ?`,
		sqlText, description, disabled, actionsJSON, accountID, iotRegion(region), ruleName,
	)
	if err != nil {
		return IoTTopicRule{}, fmt.Errorf("replace topic rule: %w", err)
	}
	existing.SQL = sqlText
	existing.Description = description
	existing.RuleDisabled = ruleDisabled
	existing.ActionsJSON = actionsJSON
	return existing, nil
}

// GetIoTTopicRule returns a topic rule.
func (s *Store) GetIoTTopicRule(accountID, region, ruleName string) (IoTTopicRule, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTTopicRule{}, err
	}
	row := s.db.QueryRow(
		`SELECT account_id, region, rule_name, rule_arn, sql_text, description, rule_disabled, actions_json, created_at
		 FROM iot_topic_rules WHERE account_id = ? AND region = ? AND rule_name = ?`,
		accountID, iotRegion(region), strings.TrimSpace(ruleName),
	)
	return scanIoTTopicRule(row)
}

// ListIoTTopicRules returns topic rules for an account/region ordered by name.
func (s *Store) ListIoTTopicRules(accountID, region string) ([]IoTTopicRule, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT account_id, region, rule_name, rule_arn, sql_text, description, rule_disabled, actions_json, created_at
		 FROM iot_topic_rules WHERE account_id = ? AND region = ? ORDER BY rule_name`,
		accountID, iotRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("list topic rules: %w", err)
	}
	defer rows.Close()
	return scanIoTTopicRules(rows)
}

// ListEnabledIoTTopicRulesByRegion returns enabled rules across accounts for MQTT dispatch.
func (s *Store) ListEnabledIoTTopicRulesByRegion(region string) ([]IoTTopicRule, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT account_id, region, rule_name, rule_arn, sql_text, description, rule_disabled, actions_json, created_at
		 FROM iot_topic_rules WHERE region = ? AND rule_disabled = 0 ORDER BY account_id, rule_name`,
		iotRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled topic rules: %w", err)
	}
	defer rows.Close()
	return scanIoTTopicRules(rows)
}

// DeleteIoTTopicRule removes a topic rule.
func (s *Store) DeleteIoTTopicRule(accountID, region, ruleName string) error {
	if _, err := s.GetIoTTopicRule(accountID, region, ruleName); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`DELETE FROM iot_topic_rules WHERE account_id = ? AND region = ? AND rule_name = ?`,
		accountID, iotRegion(region), strings.TrimSpace(ruleName),
	)
	if err != nil {
		return fmt.Errorf("delete topic rule: %w", err)
	}
	return nil
}

// SetIoTTopicRuleDisabled sets ruleDisabled (true = disabled).
func (s *Store) SetIoTTopicRuleDisabled(accountID, region, ruleName string, disabled bool) error {
	if _, err := s.GetIoTTopicRule(accountID, region, ruleName); err != nil {
		return err
	}
	flag := 0
	if disabled {
		flag = 1
	}
	_, err := s.db.Exec(
		`UPDATE iot_topic_rules SET rule_disabled = ? WHERE account_id = ? AND region = ? AND rule_name = ?`,
		flag, accountID, iotRegion(region), strings.TrimSpace(ruleName),
	)
	if err != nil {
		return fmt.Errorf("set topic rule disabled: %w", err)
	}
	return nil
}

func normalizeActionsJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "[]"
	}
	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return "[]"
	}
	out, err := json.Marshal(arr)
	if err != nil {
		return "[]"
	}
	return string(out)
}

func scanIoTTopicRule(row *sql.Row) (IoTTopicRule, error) {
	var (
		r        IoTTopicRule
		disabled int
	)
	err := row.Scan(
		&r.AccountID, &r.Region, &r.RuleName, &r.RuleARN, &r.SQL, &r.Description,
		&disabled, &r.ActionsJSON, &r.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return IoTTopicRule{}, ErrIoTNotFound
	}
	if err != nil {
		return IoTTopicRule{}, fmt.Errorf("get topic rule: %w", err)
	}
	r.RuleDisabled = disabled != 0
	return r, nil
}

func scanIoTTopicRules(rows *sql.Rows) ([]IoTTopicRule, error) {
	out := []IoTTopicRule{}
	for rows.Next() {
		var (
			r        IoTTopicRule
			disabled int
		)
		if err := rows.Scan(
			&r.AccountID, &r.Region, &r.RuleName, &r.RuleARN, &r.SQL, &r.Description,
			&disabled, &r.ActionsJSON, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan topic rule: %w", err)
		}
		r.RuleDisabled = disabled != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
