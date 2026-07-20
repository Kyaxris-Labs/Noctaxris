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
	ErrWAFNotFound      = errors.New("WAFNonexistentItemException")
	ErrWAFAlreadyExists = errors.New("WAFDuplicateItemException")
	ErrWAFBadRequest    = errors.New("WAFInvalidParameterException")
)

const DefaultWAFRegion = "us-east-1"

const wafSchema = `
CREATE TABLE IF NOT EXISTS wafv2_web_acls (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  id TEXT NOT NULL,
  scope TEXT NOT NULL,
  arn TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  default_action TEXT NOT NULL,
  rules_json TEXT NOT NULL DEFAULT '[]',
  lock_token TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name, scope)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_waf_id ON wafv2_web_acls(account_id, id);
CREATE TABLE IF NOT EXISTS wafv2_associations (
  account_id TEXT NOT NULL,
  web_acl_arn TEXT NOT NULL,
  resource_arn TEXT NOT NULL,
  PRIMARY KEY (account_id, resource_arn)
);
CREATE TABLE IF NOT EXISTS wafv2_rule_groups (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  id TEXT NOT NULL,
  scope TEXT NOT NULL,
  arn TEXT NOT NULL,
  capacity INTEGER NOT NULL DEFAULT 1,
  rules_json TEXT NOT NULL DEFAULT '[]',
  lock_token TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name, scope)
);
`

// WAFWebACL is a WAFv2 Web ACL row.
type WAFWebACL struct {
	Name          string
	ID            string
	Scope         string
	ARN           string
	Description   string
	DefaultAction string // Allow or Block
	RulesJSON     string
	LockToken     string
	CreatedAt     int64
}

// WAFRuleGroup is a lite rule group shape.
type WAFRuleGroup struct {
	Name      string
	ID        string
	Scope     string
	ARN       string
	Capacity  int
	RulesJSON string
	LockToken string
	CreatedAt int64
}

// WAFRule is a simple allow/block rule with a label match string.
type WAFRule struct {
	Name     string `json:"Name"`
	Priority int    `json:"Priority"`
	Action   string `json:"Action"` // Allow or Block
	Label    string `json:"Label"`  // request label to match
}

// EnsureWAFSchema creates WAFv2 tables if missing.
func EnsureWAFSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure waf schema: db is nil")
	}
	if _, err := db.Exec(wafSchema); err != nil {
		return fmt.Errorf("ensure waf schema: %w", err)
	}
	return nil
}

// EnsureWAFSchema ensures WAFv2 tables on an open store.
func (s *Store) EnsureWAFSchema() error {
	return EnsureWAFSchema(s.db)
}

// WAFWebACLARN builds arn:aws:wafv2:REGION:ACCOUNT:SCOPE/webacl/NAME/ID
func WAFWebACLARN(region, accountID, scope, name, id string) string {
	if region == "" {
		region = DefaultWAFRegion
	}
	scope = strings.ToLower(scope)
	return fmt.Sprintf("arn:aws:wafv2:%s:%s:%s/webacl/%s/%s", region, accountID, scope, name, id)
}

// CreateWAFWebACL creates a Web ACL.
func (s *Store) CreateWAFWebACL(accountID, region, name, scope, description, defaultAction string, rules []WAFRule) (WAFWebACL, error) {
	name = strings.TrimSpace(name)
	scope = strings.ToUpper(strings.TrimSpace(scope))
	if name == "" || (scope != "REGIONAL" && scope != "CLOUDFRONT") {
		return WAFWebACL{}, fmt.Errorf("%w: Name and Scope (REGIONAL|CLOUDFRONT) required", ErrWAFBadRequest)
	}
	if defaultAction == "" {
		defaultAction = "Allow"
	}
	if !strings.EqualFold(defaultAction, "Allow") && !strings.EqualFold(defaultAction, "Block") {
		return WAFWebACL{}, fmt.Errorf("%w: DefaultAction must be Allow or Block", ErrWAFBadRequest)
	}
	if rules == nil {
		rules = []WAFRule{}
	}
	rulesJSON, _ := json.Marshal(rules)
	id := uuid.NewString()
	lock := uuid.NewString()
	arn := WAFWebACLARN(region, accountID, scope, name, id)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO wafv2_web_acls (account_id, name, id, scope, arn, description, default_action, rules_json, lock_token, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, id, scope, arn, description, defaultAction, string(rulesJSON), lock, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return WAFWebACL{}, ErrWAFAlreadyExists
		}
		return WAFWebACL{}, fmt.Errorf("create web acl: %w", err)
	}
	return WAFWebACL{
		Name: name, ID: id, Scope: scope, ARN: arn, Description: description,
		DefaultAction: defaultAction, RulesJSON: string(rulesJSON), LockToken: lock, CreatedAt: now,
	}, nil
}

// UpdateWAFWebACL updates rules and default action.
func (s *Store) UpdateWAFWebACL(accountID, name, scope, lockToken, defaultAction string, rules []WAFRule) (WAFWebACL, error) {
	acl, err := s.GetWAFWebACL(accountID, name, scope, "")
	if err != nil {
		return WAFWebACL{}, err
	}
	if lockToken != "" && lockToken != acl.LockToken {
		return WAFWebACL{}, fmt.Errorf("%w: lock token mismatch", ErrWAFBadRequest)
	}
	if defaultAction == "" {
		defaultAction = acl.DefaultAction
	}
	if rules == nil {
		rules = []WAFRule{}
		_ = json.Unmarshal([]byte(acl.RulesJSON), &rules)
	}
	rulesJSON, _ := json.Marshal(rules)
	newLock := uuid.NewString()
	_, err = s.db.Exec(
		`UPDATE wafv2_web_acls SET default_action = ?, rules_json = ?, lock_token = ? WHERE account_id = ? AND name = ? AND scope = ?`,
		defaultAction, string(rulesJSON), newLock, accountID, name, strings.ToUpper(scope),
	)
	if err != nil {
		return WAFWebACL{}, fmt.Errorf("update web acl: %w", err)
	}
	acl.DefaultAction = defaultAction
	acl.RulesJSON = string(rulesJSON)
	acl.LockToken = newLock
	return acl, nil
}

// GetWAFWebACL returns a Web ACL by name+scope or id.
func (s *Store) GetWAFWebACL(accountID, name, scope, id string) (WAFWebACL, error) {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	var a WAFWebACL
	var err error
	if id != "" {
		err = s.db.QueryRow(
			`SELECT name, id, scope, arn, description, default_action, rules_json, lock_token, created_at
			 FROM wafv2_web_acls WHERE account_id = ? AND id = ?`,
			accountID, id,
		).Scan(&a.Name, &a.ID, &a.Scope, &a.ARN, &a.Description, &a.DefaultAction, &a.RulesJSON, &a.LockToken, &a.CreatedAt)
	} else {
		err = s.db.QueryRow(
			`SELECT name, id, scope, arn, description, default_action, rules_json, lock_token, created_at
			 FROM wafv2_web_acls WHERE account_id = ? AND name = ? AND scope = ?`,
			accountID, name, scope,
		).Scan(&a.Name, &a.ID, &a.Scope, &a.ARN, &a.Description, &a.DefaultAction, &a.RulesJSON, &a.LockToken, &a.CreatedAt)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return WAFWebACL{}, ErrWAFNotFound
	}
	if err != nil {
		return WAFWebACL{}, fmt.Errorf("get web acl: %w", err)
	}
	return a, nil
}

// ListWAFWebACLs lists Web ACLs for a scope.
func (s *Store) ListWAFWebACLs(accountID, scope string) ([]WAFWebACL, error) {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	rows, err := s.db.Query(
		`SELECT name, id, scope, arn, description, default_action, rules_json, lock_token, created_at
		 FROM wafv2_web_acls WHERE account_id = ? AND scope = ? ORDER BY name`,
		accountID, scope,
	)
	if err != nil {
		return nil, fmt.Errorf("list web acls: %w", err)
	}
	defer rows.Close()
	var out []WAFWebACL
	for rows.Next() {
		var a WAFWebACL
		if err := rows.Scan(&a.Name, &a.ID, &a.Scope, &a.ARN, &a.Description, &a.DefaultAction, &a.RulesJSON, &a.LockToken, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("list web acls scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateWAFRuleGroup creates a lite rule group.
func (s *Store) CreateWAFRuleGroup(accountID, region, name, scope string, capacity int, rules []WAFRule) (WAFRuleGroup, error) {
	name = strings.TrimSpace(name)
	scope = strings.ToUpper(strings.TrimSpace(scope))
	if name == "" || (scope != "REGIONAL" && scope != "CLOUDFRONT") {
		return WAFRuleGroup{}, fmt.Errorf("%w: Name and Scope required", ErrWAFBadRequest)
	}
	if capacity <= 0 {
		capacity = 1
	}
	if rules == nil {
		rules = []WAFRule{}
	}
	rulesJSON, _ := json.Marshal(rules)
	id := uuid.NewString()
	lock := uuid.NewString()
	arn := fmt.Sprintf("arn:aws:wafv2:%s:%s:%s/rulegroup/%s/%s", regionOrDefault(region), accountID, strings.ToLower(scope), name, id)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO wafv2_rule_groups (account_id, name, id, scope, arn, capacity, rules_json, lock_token, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, id, scope, arn, capacity, string(rulesJSON), lock, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return WAFRuleGroup{}, ErrWAFAlreadyExists
		}
		return WAFRuleGroup{}, fmt.Errorf("create rule group: %w", err)
	}
	return WAFRuleGroup{
		Name: name, ID: id, Scope: scope, ARN: arn, Capacity: capacity,
		RulesJSON: string(rulesJSON), LockToken: lock, CreatedAt: now,
	}, nil
}

func regionOrDefault(region string) string {
	if region == "" {
		return DefaultWAFRegion
	}
	return region
}

// AssociateWAFWebACL associates a Web ACL ARN with a lab resource ARN string.
func (s *Store) AssociateWAFWebACL(accountID, webACLARN, resourceARN string) error {
	webACLARN = strings.TrimSpace(webACLARN)
	resourceARN = strings.TrimSpace(resourceARN)
	if webACLARN == "" || resourceARN == "" {
		return fmt.Errorf("%w: WebACLArn and ResourceArn required", ErrWAFBadRequest)
	}
	_, err := s.db.Exec(
		`INSERT INTO wafv2_associations (account_id, web_acl_arn, resource_arn) VALUES (?, ?, ?)
		 ON CONFLICT(account_id, resource_arn) DO UPDATE SET web_acl_arn = excluded.web_acl_arn`,
		accountID, webACLARN, resourceARN,
	)
	if err != nil {
		return fmt.Errorf("associate web acl: %w", err)
	}
	return nil
}

// EvaluateWAFRequest applies Web ACL rules to a labeled request. Returns Allow or Block.
func (s *Store) EvaluateWAFRequest(accountID, webACLARN, requestLabel string) (string, error) {
	var rulesJSON, defaultAction string
	err := s.db.QueryRow(
		`SELECT rules_json, default_action FROM wafv2_web_acls WHERE account_id = ? AND arn = ?`,
		accountID, webACLARN,
	).Scan(&rulesJSON, &defaultAction)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrWAFNotFound
	}
	if err != nil {
		return "", fmt.Errorf("evaluate waf: %w", err)
	}
	var rules []WAFRule
	_ = json.Unmarshal([]byte(rulesJSON), &rules)
	for _, rule := range rules {
		if rule.Label != "" && rule.Label == requestLabel {
			if strings.EqualFold(rule.Action, "Block") {
				return "Block", nil
			}
			return "Allow", nil
		}
	}
	if strings.EqualFold(defaultAction, "Block") {
		return "Block", nil
	}
	return "Allow", nil
}
