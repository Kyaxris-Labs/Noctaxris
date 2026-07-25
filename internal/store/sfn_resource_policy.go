package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
)

// EnsureSFNResourcePolicySchema adds resource_policy on sfn_state_machines.
// Lab extension: AWS Step Functions has no resource-based policies; this column
// enables EventBridge RoleArn-less StartExecution when the policy Allows
// events.amazonaws.com (fail-closed when empty).
func EnsureSFNResourcePolicySchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure sfn resource policy schema: db is nil")
	}
	if err := execMigrateStmt(db, `ALTER TABLE sfn_state_machines ADD COLUMN resource_policy TEXT NOT NULL DEFAULT ''`, nil); err != nil {
		return fmt.Errorf("ensure sfn resource policy schema: %w", err)
	}
	return nil
}

// EnsureSFNResourcePolicySchema ensures the state machine resource_policy column.
func (s *Store) EnsureSFNResourcePolicySchema() error {
	return EnsureSFNResourcePolicySchema(s.db)
}

// PutSFNResourcePolicy replaces the state machine resource policy document.
func (s *Store) PutSFNResourcePolicy(accountID, stateMachineARNOrName, policy string) error {
	sm, err := s.GetSFNStateMachine(accountID, stateMachineARNOrName)
	if err != nil {
		return err
	}
	policy = strings.TrimSpace(policy)
	if policy == "" {
		return fmt.Errorf("put sfn resource policy: Policy is required")
	}
	if err := authz.ValidateResourcePolicyDocument(policy); err != nil {
		return fmt.Errorf("put sfn resource policy: %w", err)
	}
	res, err := s.db.Exec(
		`UPDATE sfn_state_machines SET resource_policy = ? WHERE account_id = ? AND name = ?`,
		policy, accountID, sm.Name,
	)
	if err != nil {
		return fmt.Errorf("put sfn resource policy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("put sfn resource policy: %w", err)
	}
	if n == 0 {
		return ErrSFNStateMachineNotFound
	}
	return nil
}

// GetSFNResourcePolicy returns the state machine policy or ErrNoSuchResourcePolicy when empty.
func (s *Store) GetSFNResourcePolicy(accountID, stateMachineARNOrName string) (string, error) {
	sm, err := s.GetSFNStateMachine(accountID, stateMachineARNOrName)
	if err != nil {
		return "", err
	}
	var policy string
	err = s.db.QueryRow(
		`SELECT COALESCE(resource_policy, '') FROM sfn_state_machines WHERE account_id = ? AND name = ?`,
		accountID, sm.Name,
	).Scan(&policy)
	if err != nil {
		return "", fmt.Errorf("get sfn resource policy: %w", err)
	}
	if strings.TrimSpace(policy) == "" {
		return "", ErrNoSuchResourcePolicy
	}
	return policy, nil
}

// DeleteSFNResourcePolicy clears the state machine resource policy.
func (s *Store) DeleteSFNResourcePolicy(accountID, stateMachineARNOrName string) error {
	sm, err := s.GetSFNStateMachine(accountID, stateMachineARNOrName)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE sfn_state_machines SET resource_policy = '' WHERE account_id = ? AND name = ?`,
		accountID, sm.Name,
	)
	if err != nil {
		return fmt.Errorf("delete sfn resource policy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete sfn resource policy: %w", err)
	}
	if n == 0 {
		return ErrSFNStateMachineNotFound
	}
	return nil
}
