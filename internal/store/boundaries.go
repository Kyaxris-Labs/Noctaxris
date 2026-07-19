package store

import (
	"database/sql"
	"errors"
	"fmt"
)

const (
	boundaryPrincipalUser = "user"
	boundaryPrincipalRole = "role"
)

// PutUserPermissionsBoundary sets a managed policy ARN as the user's permissions boundary.
func (s *Store) PutUserPermissionsBoundary(accountID, userName, policyARN string) error {
	if _, err := s.GetUser(accountID, userName); err != nil {
		return fmt.Errorf("put user permissions boundary: %w", err)
	}
	return s.putPermissionsBoundary(accountID, boundaryPrincipalUser, userName, policyARN)
}

// GetUserPermissionsBoundary returns the managed policy ARN for the user's boundary.
func (s *Store) GetUserPermissionsBoundary(accountID, userName string) (string, error) {
	return s.getPermissionsBoundaryARN(accountID, boundaryPrincipalUser, userName)
}

// DeleteUserPermissionsBoundary removes the user's permissions boundary.
func (s *Store) DeleteUserPermissionsBoundary(accountID, userName string) error {
	return s.deletePermissionsBoundary(accountID, boundaryPrincipalUser, userName)
}

// PutRolePermissionsBoundary sets a managed policy ARN as the role's permissions boundary.
func (s *Store) PutRolePermissionsBoundary(accountID, roleName, policyARN string) error {
	if _, _, err := s.GetRole(accountID, roleName); err != nil {
		return fmt.Errorf("put role permissions boundary: %w", err)
	}
	return s.putPermissionsBoundary(accountID, boundaryPrincipalRole, roleName, policyARN)
}

// GetRolePermissionsBoundary returns the managed policy ARN for the role's boundary.
func (s *Store) GetRolePermissionsBoundary(accountID, roleName string) (string, error) {
	return s.getPermissionsBoundaryARN(accountID, boundaryPrincipalRole, roleName)
}

// DeleteRolePermissionsBoundary removes the role's permissions boundary.
func (s *Store) DeleteRolePermissionsBoundary(accountID, roleName string) error {
	return s.deletePermissionsBoundary(accountID, boundaryPrincipalRole, roleName)
}

// PermissionsBoundaryDoc returns the boundary policy document for principal kind
// ("user" or "role") and name. ok is false when no boundary is set.
func (s *Store) PermissionsBoundaryDoc(accountID, kind, name string) (doc string, ok bool, err error) {
	switch kind {
	case boundaryPrincipalUser, boundaryPrincipalRole:
	default:
		return "", false, fmt.Errorf("permissions boundary doc: unknown kind %q", kind)
	}
	arn, err := s.getPermissionsBoundaryARN(accountID, kind, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	doc, err = s.policyDocumentByARN(arn)
	if err != nil {
		return "", false, err
	}
	return doc, true, nil
}

func (s *Store) putPermissionsBoundary(accountID, principalType, principalName, policyARN string) error {
	if policyARN == "" {
		return fmt.Errorf("put permissions boundary: policy_arn required")
	}
	if _, err := s.policyDocumentByARN(policyARN); err != nil {
		return fmt.Errorf("put permissions boundary: %w", err)
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_permissions_boundaries (account_id, principal_type, principal_name, policy_arn)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(account_id, principal_type, principal_name) DO UPDATE SET policy_arn = excluded.policy_arn`,
		accountID, principalType, principalName, policyARN,
	)
	if err != nil {
		return fmt.Errorf("put permissions boundary %s/%s/%s: %w", accountID, principalType, principalName, err)
	}
	return nil
}

func (s *Store) getPermissionsBoundaryARN(accountID, principalType, principalName string) (string, error) {
	var arn string
	err := s.db.QueryRow(
		`SELECT policy_arn FROM iam_permissions_boundaries
		 WHERE account_id = ? AND principal_type = ? AND principal_name = ?`,
		accountID, principalType, principalName,
	).Scan(&arn)
	if err != nil {
		return "", err
	}
	return arn, nil
}

func (s *Store) deletePermissionsBoundary(accountID, principalType, principalName string) error {
	res, err := s.db.Exec(
		`DELETE FROM iam_permissions_boundaries
		 WHERE account_id = ? AND principal_type = ? AND principal_name = ?`,
		accountID, principalType, principalName,
	)
	if err != nil {
		return fmt.Errorf("delete permissions boundary %s/%s/%s: %w", accountID, principalType, principalName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete permissions boundary %s/%s/%s: %w", accountID, principalType, principalName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) policyDocumentByARN(policyARN string) (string, error) {
	var doc string
	err := s.db.QueryRow(`SELECT document FROM managed_policies WHERE policy_arn = ?`, policyARN).Scan(&doc)
	if err == nil {
		return doc, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("lookup managed policy %s: %w", policyARN, err)
	}
	err = s.db.QueryRow(`SELECT document FROM policies WHERE policy_id = ? OR arn = ?`, policyARN, policyARN).Scan(&doc)
	if err != nil {
		return "", fmt.Errorf("lookup policy document %s: %w", policyARN, err)
	}
	return doc, nil
}
