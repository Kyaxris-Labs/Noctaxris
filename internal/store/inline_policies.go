package store

import (
	"database/sql"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

// InlinePolicy is an inline policy on a principal ARN.
type InlinePolicy struct {
	PrincipalARN string
	PolicyName   string
	Document     string
}

// PutInlinePolicy stores or replaces an inline policy on principalARN.
func (s *Store) PutInlinePolicy(principalARN, name, document string) error {
	if principalARN == "" {
		return fmt.Errorf("put inline policy: principal_arn required")
	}
	if err := validate.IAMName(name); err != nil {
		return fmt.Errorf("put inline policy: %w", err)
	}
	if err := validate.PolicyDocument(document); err != nil {
		return fmt.Errorf("put inline policy: %w", err)
	}
	_, err := s.db.Exec(
		`INSERT INTO inline_policies (principal_arn, policy_name, document) VALUES (?, ?, ?)
		 ON CONFLICT(principal_arn, policy_name) DO UPDATE SET document = excluded.document`,
		principalARN, name, document,
	)
	if err != nil {
		return fmt.Errorf("put inline policy %s/%s: %w", principalARN, name, err)
	}
	return nil
}

// GetInlinePolicy returns an inline policy by principal ARN and name.
func (s *Store) GetInlinePolicy(principalARN, name string) (InlinePolicy, error) {
	var p InlinePolicy
	err := s.db.QueryRow(
		`SELECT principal_arn, policy_name, document FROM inline_policies
		 WHERE principal_arn = ? AND policy_name = ?`,
		principalARN, name,
	).Scan(&p.PrincipalARN, &p.PolicyName, &p.Document)
	if err != nil {
		return InlinePolicy{}, err
	}
	return p, nil
}

// DeleteInlinePolicy removes an inline policy.
func (s *Store) DeleteInlinePolicy(principalARN, name string) error {
	res, err := s.db.Exec(
		`DELETE FROM inline_policies WHERE principal_arn = ? AND policy_name = ?`,
		principalARN, name,
	)
	if err != nil {
		return fmt.Errorf("delete inline policy %s/%s: %w", principalARN, name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete inline policy %s/%s: %w", principalARN, name, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListInlinePolicies returns inline policies for principalARN.
func (s *Store) ListInlinePolicies(principalARN string) ([]InlinePolicy, error) {
	rows, err := s.db.Query(
		`SELECT principal_arn, policy_name, document FROM inline_policies
		 WHERE principal_arn = ? ORDER BY policy_name`,
		principalARN,
	)
	if err != nil {
		return nil, fmt.Errorf("list inline policies %s: %w", principalARN, err)
	}
	defer rows.Close()

	var out []InlinePolicy
	for rows.Next() {
		var p InlinePolicy
		if err := rows.Scan(&p.PrincipalARN, &p.PolicyName, &p.Document); err != nil {
			return nil, fmt.Errorf("list inline policies %s: %w", principalARN, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list inline policies %s: %w", principalARN, err)
	}
	if out == nil {
		out = []InlinePolicy{}
	}
	return out, nil
}
