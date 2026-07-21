package store

import (
	"database/sql"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

// ManagedPolicy is a customer-managed IAM policy.
type ManagedPolicy struct {
	PolicyARN        string
	AccountID        string
	PolicyName       string
	PolicyID         string
	DefaultVersionID string
	Document         string
}

// CreateManagedPolicy creates a customer-managed policy and syncs it into policies
// with policy_id = arn so ListAttachedPolicyDocuments keeps working.
func (s *Store) CreateManagedPolicy(accountID, name, document string) (arn string, err error) {
	if err := validate.AccountID(accountID); err != nil {
		return "", fmt.Errorf("create managed policy: %w", err)
	}
	if err := validate.IAMName(name); err != nil {
		return "", fmt.Errorf("create managed policy: %w", err)
	}
	if err := validate.PolicyDocument(document); err != nil {
		return "", fmt.Errorf("create managed policy: %w", err)
	}
	policyID, err := newIAMResourceID("ANPA")
	if err != nil {
		return "", err
	}
	arn = PolicyARN(accountID, "/", name)
	const versionID = "v1"

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	created := nowRFC3339()
	if _, err := tx.Exec(
		`INSERT INTO managed_policies
		 (policy_arn, account_id, policy_name, policy_id, default_version_id, document)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		arn, accountID, name, policyID, versionID, document,
	); err != nil {
		return "", fmt.Errorf("create managed policy %s: %w", name, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO managed_policy_versions
		 (policy_arn, version_id, document, is_default, create_date)
		 VALUES (?, ?, ?, 1, ?)`,
		arn, versionID, document, created,
	); err != nil {
		return "", fmt.Errorf("create managed policy %s version: %w", name, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO policies (policy_id, document, policy_name, account_id, arn, path, default_version_id)
		 VALUES (?, ?, ?, ?, ?, '/', ?)
		 ON CONFLICT(policy_id) DO UPDATE SET
		   document = excluded.document,
		   policy_name = excluded.policy_name,
		   account_id = excluded.account_id,
		   arn = excluded.arn,
		   default_version_id = excluded.default_version_id`,
		arn, document, name, accountID, arn, versionID,
	); err != nil {
		return "", fmt.Errorf("sync managed policy %s into policies: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return arn, nil
}

// GetManagedPolicy returns a customer-managed policy by ARN.
func (s *Store) GetManagedPolicy(policyARN string) (ManagedPolicy, error) {
	var p ManagedPolicy
	err := s.db.QueryRow(
		`SELECT policy_arn, account_id, policy_name, policy_id, default_version_id, document
		 FROM managed_policies WHERE policy_arn = ?`,
		policyARN,
	).Scan(&p.PolicyARN, &p.AccountID, &p.PolicyName, &p.PolicyID, &p.DefaultVersionID, &p.Document)
	if err != nil {
		return ManagedPolicy{}, err
	}
	return p, nil
}

// ListManagedPolicies returns customer-managed policies for accountID.
func (s *Store) ListManagedPolicies(accountID string) ([]ManagedPolicy, error) {
	rows, err := s.db.Query(
		`SELECT policy_arn, account_id, policy_name, policy_id, default_version_id, document
		 FROM managed_policies WHERE account_id = ? ORDER BY policy_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list managed policies %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []ManagedPolicy
	for rows.Next() {
		var p ManagedPolicy
		if err := rows.Scan(&p.PolicyARN, &p.AccountID, &p.PolicyName, &p.PolicyID, &p.DefaultVersionID, &p.Document); err != nil {
			return nil, fmt.Errorf("list managed policies %s: %w", accountID, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list managed policies %s: %w", accountID, err)
	}
	if out == nil {
		out = []ManagedPolicy{}
	}
	return out, nil
}

// DeleteManagedPolicy removes a managed policy and its policies-table sync row.
// Attachments must be detached first.
func (s *Store) DeleteManagedPolicy(policyARN string) error {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM policy_attachments WHERE policy_id = ?`,
		policyARN,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("delete managed policy %s: %w", policyARN, err)
	}
	if n > 0 {
		return fmt.Errorf("delete managed policy %s: %d attachment(s) remain", policyARN, n)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`DELETE FROM managed_policies WHERE policy_arn = ?`, policyARN)
	if err != nil {
		return fmt.Errorf("delete managed policy %s: %w", policyARN, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete managed policy %s: %w", policyARN, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`DELETE FROM managed_policy_versions WHERE policy_arn = ?`, policyARN); err != nil {
		return fmt.Errorf("delete managed policy versions %s: %w", policyARN, err)
	}
	if _, err := tx.Exec(`DELETE FROM policies WHERE policy_id = ?`, policyARN); err != nil {
		return fmt.Errorf("delete synced policy %s: %w", policyARN, err)
	}
	return tx.Commit()
}

// AttachUserPolicy attaches a managed policy ARN to an IAM user.
func (s *Store) AttachUserPolicy(accountID, userName, policyARN string) error {
	u, err := s.GetUser(accountID, userName)
	if err != nil {
		return fmt.Errorf("attach user policy: %w", err)
	}
	return s.AttachPolicy(u.ARN, policyARN)
}

// DetachUserPolicy detaches a managed policy ARN from an IAM user.
func (s *Store) DetachUserPolicy(accountID, userName, policyARN string) error {
	u, err := s.GetUser(accountID, userName)
	if err != nil {
		return fmt.Errorf("detach user policy: %w", err)
	}
	return s.detachPolicy(u.ARN, policyARN)
}

// AttachRolePolicy attaches a managed policy ARN to an IAM role.
func (s *Store) AttachRolePolicy(accountID, roleName, policyARN string) error {
	roleARN, _, err := s.GetRole(accountID, roleName)
	if err != nil {
		return fmt.Errorf("attach role policy: %w", err)
	}
	return s.AttachPolicy(roleARN, policyARN)
}

// DetachRolePolicy detaches a managed policy ARN from an IAM role.
func (s *Store) DetachRolePolicy(accountID, roleName, policyARN string) error {
	roleARN, _, err := s.GetRole(accountID, roleName)
	if err != nil {
		return fmt.Errorf("detach role policy: %w", err)
	}
	return s.detachPolicy(roleARN, policyARN)
}

func (s *Store) detachPolicy(principalARN, policyID string) error {
	res, err := s.db.Exec(
		`DELETE FROM policy_attachments WHERE principal_arn = ? AND policy_id = ?`,
		principalARN, policyID,
	)
	if err != nil {
		return fmt.Errorf("detach policy %s from %s: %w", policyID, principalARN, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("detach policy %s from %s: %w", policyID, principalARN, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
