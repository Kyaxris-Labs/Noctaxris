package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
)

const (
	// LabLogsResourcePolicySoftCap mirrors the AWS account/region quota (10).
	LabLogsResourcePolicySoftCap = 10
)

var (
	ErrLogsResourcePolicyNotFound = errors.New("ResourceNotFoundException")
	ErrLogsResourcePolicyLimit    = errors.New("LimitExceededException")
)

// LogsResourcePolicy is an account-scoped CloudWatch Logs resource policy row.
type LogsResourcePolicy struct {
	PolicyName     string
	PolicyDocument string
	LastUpdated    int64 // epoch millis
}

const logsResourcePolicySchema = `
CREATE TABLE IF NOT EXISTS logs_resource_policies (
  account_id TEXT NOT NULL,
  policy_name TEXT NOT NULL,
  policy_document TEXT NOT NULL,
  last_updated INTEGER NOT NULL,
  PRIMARY KEY (account_id, policy_name)
);
`

// EnsureLogsResourcePolicySchema creates the account Logs resource-policy table.
func EnsureLogsResourcePolicySchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure logs resource policy schema: db is nil")
	}
	if _, err := db.Exec(logsResourcePolicySchema); err != nil {
		return fmt.Errorf("ensure logs resource policy schema: %w", err)
	}
	return nil
}

// EnsureLogsResourcePolicySchema ensures Logs resource-policy tables on an open store.
func (s *Store) EnsureLogsResourcePolicySchema() error {
	return EnsureLogsResourcePolicySchema(s.db)
}

// PutLogsResourcePolicy upserts a named account resource policy (AWS Logs shape).
func (s *Store) PutLogsResourcePolicy(accountID, policyName, policyDocument string) (LogsResourcePolicy, error) {
	policyName = strings.TrimSpace(policyName)
	policyDocument = strings.TrimSpace(policyDocument)
	if policyName == "" {
		return LogsResourcePolicy{}, fmt.Errorf("put logs resource policy: policyName is required")
	}
	if policyDocument == "" {
		return LogsResourcePolicy{}, fmt.Errorf("put logs resource policy: policyDocument is required")
	}
	if err := authz.ValidateResourcePolicyDocument(policyDocument); err != nil {
		return LogsResourcePolicy{}, fmt.Errorf("put logs resource policy: %w", err)
	}
	existing, err := s.GetLogsResourcePolicy(accountID, policyName)
	if err != nil && !errors.Is(err, ErrLogsResourcePolicyNotFound) {
		return LogsResourcePolicy{}, err
	}
	if errors.Is(err, ErrLogsResourcePolicyNotFound) {
		n, countErr := s.countLogsResourcePolicies(accountID)
		if countErr != nil {
			return LogsResourcePolicy{}, countErr
		}
		if n >= LabLogsResourcePolicySoftCap {
			return LogsResourcePolicy{}, ErrLogsResourcePolicyLimit
		}
	} else {
		_ = existing
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO logs_resource_policies (account_id, policy_name, policy_document, last_updated)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(account_id, policy_name) DO UPDATE SET
		   policy_document = excluded.policy_document,
		   last_updated = excluded.last_updated`,
		accountID, policyName, policyDocument, now,
	)
	if err != nil {
		return LogsResourcePolicy{}, fmt.Errorf("put logs resource policy: %w", err)
	}
	return LogsResourcePolicy{PolicyName: policyName, PolicyDocument: policyDocument, LastUpdated: now}, nil
}

// GetLogsResourcePolicy returns one named account policy.
func (s *Store) GetLogsResourcePolicy(accountID, policyName string) (LogsResourcePolicy, error) {
	policyName = strings.TrimSpace(policyName)
	if policyName == "" {
		return LogsResourcePolicy{}, fmt.Errorf("get logs resource policy: policyName is required")
	}
	var p LogsResourcePolicy
	err := s.db.QueryRow(
		`SELECT policy_name, policy_document, last_updated FROM logs_resource_policies
		 WHERE account_id = ? AND policy_name = ?`,
		accountID, policyName,
	).Scan(&p.PolicyName, &p.PolicyDocument, &p.LastUpdated)
	if errors.Is(err, sql.ErrNoRows) {
		return LogsResourcePolicy{}, ErrLogsResourcePolicyNotFound
	}
	if err != nil {
		return LogsResourcePolicy{}, fmt.Errorf("get logs resource policy: %w", err)
	}
	return p, nil
}

// DeleteLogsResourcePolicy removes a named account policy.
func (s *Store) DeleteLogsResourcePolicy(accountID, policyName string) error {
	policyName = strings.TrimSpace(policyName)
	if policyName == "" {
		return fmt.Errorf("delete logs resource policy: policyName is required")
	}
	res, err := s.db.Exec(
		`DELETE FROM logs_resource_policies WHERE account_id = ? AND policy_name = ?`,
		accountID, policyName,
	)
	if err != nil {
		return fmt.Errorf("delete logs resource policy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete logs resource policy: %w", err)
	}
	if n == 0 {
		return ErrLogsResourcePolicyNotFound
	}
	return nil
}

// DescribeLogsResourcePolicies lists account resource policies (AWS DescribeResourcePolicies).
func (s *Store) DescribeLogsResourcePolicies(accountID string) ([]LogsResourcePolicy, error) {
	rows, err := s.db.Query(
		`SELECT policy_name, policy_document, last_updated FROM logs_resource_policies
		 WHERE account_id = ? ORDER BY policy_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("describe logs resource policies: %w", err)
	}
	defer rows.Close()
	var out []LogsResourcePolicy
	for rows.Next() {
		var p LogsResourcePolicy
		if err := rows.Scan(&p.PolicyName, &p.PolicyDocument, &p.LastUpdated); err != nil {
			return nil, fmt.Errorf("describe logs resource policies: scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) countLogsResourcePolicies(accountID string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM logs_resource_policies WHERE account_id = ?`,
		accountID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count logs resource policies: %w", err)
	}
	return n, nil
}

// logsResourcePolicyAllowsDelivery reports whether any account Logs resource policy
// Allows action for the service principal on the log-group ARN (bare or :* form).
func (s *Store) logsResourcePolicyAllowsDelivery(accountID, logGroupARN, action, servicePrincipal, sourceARN string) bool {
	policies, err := s.DescribeLogsResourcePolicies(accountID)
	if err != nil || len(policies) == 0 {
		return false
	}
	keys := authz.DeliverySourceConditionKeys(sourceARN, "")
	candidates := []string{logGroupARN}
	if !strings.HasSuffix(logGroupARN, ":*") {
		candidates = append(candidates, logGroupARN+":*")
	}
	for _, p := range policies {
		for _, res := range candidates {
			if authz.EventTargetResourcePolicyAllows(
				p.PolicyDocument, action, res, servicePrincipal, accountID, keys,
			) {
				return true
			}
		}
	}
	return false
}
