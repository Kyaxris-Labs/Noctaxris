package store

import (
	"database/sql"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

// Role is an IAM role record.
type Role struct {
	AccountID   string
	RoleName    string
	RoleARN     string
	TrustPolicy string
	RoleID      string
	CreateDate  string
}

// PutRole stores or replaces a role trust policy for accountID/roleName.
func (s *Store) PutRole(accountID, roleName, roleARN, trustPolicy string) error {
	_, err := s.db.Exec(
		`INSERT INTO roles (account_id, role_name, role_arn, trust_policy) VALUES (?, ?, ?, ?)
		 ON CONFLICT(account_id, role_name) DO UPDATE SET
		   role_arn = excluded.role_arn,
		   trust_policy = excluded.trust_policy`,
		accountID, roleName, roleARN, trustPolicy,
	)
	if err != nil {
		return fmt.Errorf("put role %s/%s: %w", accountID, roleName, err)
	}
	return nil
}

// CreateRole creates a new IAM role with a generated role ID.
func (s *Store) CreateRole(accountID, roleName, trustPolicy string) (roleARN string, err error) {
	if err := validate.AccountID(accountID); err != nil {
		return "", fmt.Errorf("create role: %w", err)
	}
	if err := validate.IAMName(roleName); err != nil {
		return "", fmt.Errorf("create role: %w", err)
	}
	if err := validate.PolicyDocument(trustPolicy); err != nil {
		return "", fmt.Errorf("create role: %w", err)
	}
	roleID, err := newIAMResourceID("AROA")
	if err != nil {
		return "", err
	}
	roleARN = RoleARN(accountID, roleName)
	createDate := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO roles (account_id, role_name, role_arn, trust_policy, role_id, path, create_date)
		 VALUES (?, ?, ?, ?, ?, '/', ?)`,
		accountID, roleName, roleARN, trustPolicy, roleID, createDate,
	)
	if err != nil {
		return "", fmt.Errorf("create role %s/%s: %w", accountID, roleName, err)
	}
	return roleARN, nil
}

// GetRole returns the role ARN and trust policy for accountID/roleName.
func (s *Store) GetRole(accountID, roleName string) (roleARN, trustPolicy string, err error) {
	r, err := s.GetRoleRecord(accountID, roleName)
	if err != nil {
		return "", "", err
	}
	return r.RoleARN, r.TrustPolicy, nil
}

// GetRoleRecord returns the full IAM role record.
func (s *Store) GetRoleRecord(accountID, roleName string) (Role, error) {
	var r Role
	err := s.db.QueryRow(
		`SELECT account_id, role_name, role_arn, trust_policy, COALESCE(role_id, ''), COALESCE(create_date, '')
		 FROM roles WHERE account_id = ? AND role_name = ?`,
		accountID, roleName,
	).Scan(&r.AccountID, &r.RoleName, &r.RoleARN, &r.TrustPolicy, &r.RoleID, &r.CreateDate)
	if err != nil {
		return Role{}, err
	}
	return r, nil
}

// ListRoles returns IAM roles in accountID ordered by role_name.
func (s *Store) ListRoles(accountID string) ([]Role, error) {
	rows, err := s.db.Query(
		`SELECT account_id, role_name, role_arn, trust_policy, COALESCE(role_id, ''), COALESCE(create_date, '')
		 FROM roles WHERE account_id = ? ORDER BY role_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list roles %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.AccountID, &r.RoleName, &r.RoleARN, &r.TrustPolicy, &r.RoleID, &r.CreateDate); err != nil {
			return nil, fmt.Errorf("list roles %s: %w", accountID, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list roles %s: %w", accountID, err)
	}
	if out == nil {
		out = []Role{}
	}
	return out, nil
}

// DeleteRole removes an IAM role.
func (s *Store) DeleteRole(accountID, roleName string) error {
	res, err := s.db.Exec(
		`DELETE FROM roles WHERE account_id = ? AND role_name = ?`,
		accountID, roleName,
	)
	if err != nil {
		return fmt.Errorf("delete role %s/%s: %w", accountID, roleName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete role %s/%s: %w", accountID, roleName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateAssumeRolePolicy replaces the trust policy for an existing role.
func (s *Store) UpdateAssumeRolePolicy(accountID, roleName, trustPolicy string) error {
	if err := validate.PolicyDocument(trustPolicy); err != nil {
		return fmt.Errorf("update assume role policy: %w", err)
	}
	res, err := s.db.Exec(
		`UPDATE roles SET trust_policy = ? WHERE account_id = ? AND role_name = ?`,
		trustPolicy, accountID, roleName,
	)
	if err != nil {
		return fmt.Errorf("update assume role policy %s/%s: %w", accountID, roleName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update assume role policy %s/%s: %w", accountID, roleName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
