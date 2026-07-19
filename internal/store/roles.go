package store

import (
	"fmt"
)

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

// GetRole returns the role ARN and trust policy for accountID/roleName.
func (s *Store) GetRole(accountID, roleName string) (roleARN, trustPolicy string, err error) {
	err = s.db.QueryRow(
		`SELECT role_arn, trust_policy FROM roles WHERE account_id = ? AND role_name = ?`,
		accountID, roleName,
	).Scan(&roleARN, &trustPolicy)
	if err != nil {
		return "", "", err
	}
	return roleARN, trustPolicy, nil
}
