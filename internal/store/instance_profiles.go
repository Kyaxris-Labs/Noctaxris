package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

// InstanceProfile is an IAM instance profile with at most one role.
type InstanceProfile struct {
	AccountID   string
	ProfileName string
	ProfileARN  string
	RoleName    string // empty when no role is associated
	RoleARN     string
	RoleID      string
}

// CreateInstanceProfile creates an instance profile with no role.
func (s *Store) CreateInstanceProfile(accountID, profileName string) (arn string, err error) {
	if err := validate.AccountID(accountID); err != nil {
		return "", fmt.Errorf("create instance profile: %w", err)
	}
	if err := validate.IAMName(profileName); err != nil {
		return "", fmt.Errorf("create instance profile: %w", err)
	}
	arn = InstanceProfileARN(accountID, "/", profileName)
	_, err = s.db.Exec(
		`INSERT INTO iam_instance_profiles (account_id, profile_name, profile_arn) VALUES (?, ?, ?)`,
		accountID, profileName, arn,
	)
	if err != nil {
		return "", fmt.Errorf("create instance profile %s/%s: %w", accountID, profileName, err)
	}
	return arn, nil
}

// AddRoleToInstanceProfile associates a role with a profile. At most one role is allowed.
func (s *Store) AddRoleToInstanceProfile(accountID, profileName, roleName string) error {
	if _, err := s.GetInstanceProfile(accountID, profileName); err != nil {
		return fmt.Errorf("add role to instance profile: %w", err)
	}
	if _, _, err := s.GetRole(accountID, roleName); err != nil {
		return fmt.Errorf("add role to instance profile: %w", err)
	}

	var existing string
	err := s.db.QueryRow(
		`SELECT role_name FROM iam_instance_profile_roles WHERE account_id = ? AND profile_name = ?`,
		accountID, profileName,
	).Scan(&existing)
	if err == nil {
		return fmt.Errorf("add role to instance profile %s/%s: already has role %q (at most one role)", accountID, profileName, existing)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("add role to instance profile %s/%s: %w", accountID, profileName, err)
	}

	_, err = s.db.Exec(
		`INSERT INTO iam_instance_profile_roles (account_id, profile_name, role_name) VALUES (?, ?, ?)`,
		accountID, profileName, roleName,
	)
	if err != nil {
		return fmt.Errorf("add role to instance profile %s/%s: %w", accountID, profileName, err)
	}
	return nil
}

// GetInstanceProfile returns an instance profile and its optional role.
func (s *Store) GetInstanceProfile(accountID, profileName string) (InstanceProfile, error) {
	var p InstanceProfile
	err := s.db.QueryRow(
		`SELECT account_id, profile_name, profile_arn FROM iam_instance_profiles
		 WHERE account_id = ? AND profile_name = ?`,
		accountID, profileName,
	).Scan(&p.AccountID, &p.ProfileName, &p.ProfileARN)
	if err != nil {
		return InstanceProfile{}, err
	}
	var roleName sql.NullString
	err = s.db.QueryRow(
		`SELECT role_name FROM iam_instance_profile_roles WHERE account_id = ? AND profile_name = ?`,
		accountID, profileName,
	).Scan(&roleName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return InstanceProfile{}, fmt.Errorf("get instance profile role %s/%s: %w", accountID, profileName, err)
	}
	if roleName.Valid {
		p.RoleName = roleName.String
		if role, roleErr := s.GetRoleRecord(accountID, p.RoleName); roleErr == nil {
			p.RoleARN = role.RoleARN
			p.RoleID = role.RoleID
		}
	}
	return p, nil
}

// DeleteInstanceProfile deletes an instance profile and its role association.
func (s *Store) DeleteInstanceProfile(accountID, profileName string) error {
	if _, err := s.GetInstanceProfile(accountID, profileName); err != nil {
		return fmt.Errorf("delete instance profile: %w", err)
	}
	if _, err := s.db.Exec(
		`DELETE FROM iam_instance_profile_roles WHERE account_id = ? AND profile_name = ?`,
		accountID, profileName,
	); err != nil {
		return fmt.Errorf("delete instance profile roles %s/%s: %w", accountID, profileName, err)
	}
	res, err := s.db.Exec(
		`DELETE FROM iam_instance_profiles WHERE account_id = ? AND profile_name = ?`,
		accountID, profileName,
	)
	if err != nil {
		return fmt.Errorf("delete instance profile %s/%s: %w", accountID, profileName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete instance profile %s/%s: %w", accountID, profileName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RemoveRoleFromInstanceProfile removes the role association from a profile.
func (s *Store) RemoveRoleFromInstanceProfile(accountID, profileName, roleName string) error {
	res, err := s.db.Exec(
		`DELETE FROM iam_instance_profile_roles
		 WHERE account_id = ? AND profile_name = ? AND role_name = ?`,
		accountID, profileName, roleName,
	)
	if err != nil {
		return fmt.Errorf("remove role from instance profile %s/%s: %w", accountID, profileName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove role from instance profile %s/%s: %w", accountID, profileName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListInstanceProfiles returns instance profiles for accountID.
func (s *Store) ListInstanceProfiles(accountID string) ([]InstanceProfile, error) {
	rows, err := s.db.Query(
		`SELECT account_id, profile_name, profile_arn FROM iam_instance_profiles
		 WHERE account_id = ? ORDER BY profile_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list instance profiles %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []InstanceProfile
	for rows.Next() {
		var p InstanceProfile
		if err := rows.Scan(&p.AccountID, &p.ProfileName, &p.ProfileARN); err != nil {
			return nil, fmt.Errorf("list instance profiles %s: %w", accountID, err)
		}
		var roleName sql.NullString
		err = s.db.QueryRow(
			`SELECT role_name FROM iam_instance_profile_roles WHERE account_id = ? AND profile_name = ?`,
			p.AccountID, p.ProfileName,
		).Scan(&roleName)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("list instance profiles %s: %w", accountID, err)
		}
		if roleName.Valid {
			p.RoleName = roleName.String
			if role, roleErr := s.GetRoleRecord(p.AccountID, p.RoleName); roleErr == nil {
				p.RoleARN = role.RoleARN
				p.RoleID = role.RoleID
			}
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list instance profiles %s: %w", accountID, err)
	}
	if out == nil {
		out = []InstanceProfile{}
	}
	return out, nil
}
