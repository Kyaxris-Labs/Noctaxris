package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

// User is an IAM user record.
type User struct {
	AccountID string
	UserName  string
	UserID    string
	ARN       string
}

// CreateUser creates an IAM user in accountID.
func (s *Store) CreateUser(accountID, userName string) (userID, arn string, err error) {
	if err := validate.AccountID(accountID); err != nil {
		return "", "", fmt.Errorf("create user: %w", err)
	}
	if err := validate.IAMName(userName); err != nil {
		return "", "", fmt.Errorf("create user: %w", err)
	}
	userID, err = newIAMResourceID("AIDA")
	if err != nil {
		return "", "", err
	}
	arn = UserARN(accountID, "/", userName)
	_, err = s.db.Exec(
		`INSERT INTO users (account_id, user_name, user_id, arn) VALUES (?, ?, ?, ?)`,
		accountID, userName, userID, arn,
	)
	if err != nil {
		return "", "", fmt.Errorf("create user %s/%s: %w", accountID, userName, err)
	}
	return userID, arn, nil
}

// GetUser returns the IAM user for accountID/userName.
func (s *Store) GetUser(accountID, userName string) (User, error) {
	var u User
	err := s.db.QueryRow(
		`SELECT account_id, user_name, user_id, arn FROM users WHERE account_id = ? AND user_name = ?`,
		accountID, userName,
	).Scan(&u.AccountID, &u.UserName, &u.UserID, &u.ARN)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// ListUsers returns IAM users in accountID ordered by user_name.
func (s *Store) ListUsers(accountID string) ([]User, error) {
	rows, err := s.db.Query(
		`SELECT account_id, user_name, user_id, arn FROM users WHERE account_id = ? ORDER BY user_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list users %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.AccountID, &u.UserName, &u.UserID, &u.ARN); err != nil {
			return nil, fmt.Errorf("list users %s: %w", accountID, err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users %s: %w", accountID, err)
	}
	if out == nil {
		out = []User{}
	}
	return out, nil
}

// DeleteUser removes an IAM user. Fails if any access keys still exist for the user.
func (s *Store) DeleteUser(accountID, userName string) error {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM access_keys WHERE account_id = ? AND user_name = ?`,
		accountID, userName,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("delete user %s/%s: %w", accountID, userName, err)
	}
	if n > 0 {
		return fmt.Errorf("delete user %s/%s: %d access key(s) remain", accountID, userName, n)
	}
	res, err := s.db.Exec(
		`DELETE FROM users WHERE account_id = ? AND user_name = ?`,
		accountID, userName,
	)
	if err != nil {
		return fmt.Errorf("delete user %s/%s: %w", accountID, userName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete user %s/%s: %w", accountID, userName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CreateUserAccessKey mints a long-lived AKIA key for an IAM user.
func (s *Store) CreateUserAccessKey(accountID, userName string) (accessKeyID, secret string, err error) {
	if _, err := s.GetUser(accountID, userName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", fmt.Errorf("create access key: user %s/%s not found", accountID, userName)
		}
		return "", "", err
	}
	accessKeyID, err = newAccessKeyID()
	if err != nil {
		return "", "", err
	}
	secret, err = newAccessKeySecret()
	if err != nil {
		return "", "", err
	}
	sealed, err := Seal(s.master, []byte(secret))
	if err != nil {
		return "", "", fmt.Errorf("seal secret: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO access_keys
		 (access_key_id, account_id, secret_ciphertext, is_root, user_name, status)
		 VALUES (?, ?, ?, 0, ?, ?)`,
		accessKeyID, accountID, sealed, userName, AccessKeyStatusActive,
	)
	if err != nil {
		return "", "", fmt.Errorf("create access key for %s/%s: %w", accountID, userName, err)
	}
	return accessKeyID, secret, nil
}

// AccessKeyMeta is a listed access key without the secret.
type AccessKeyMeta struct {
	AccessKeyID string
	UserName    string
	Status      string
}

// ListAccessKeys returns access keys for an IAM user (no secrets).
func (s *Store) ListAccessKeys(accountID, userName string) ([]AccessKeyMeta, error) {
	rows, err := s.db.Query(
		`SELECT access_key_id, user_name, COALESCE(NULLIF(status, ''), 'Active')
		 FROM access_keys
		 WHERE account_id = ? AND user_name = ?
		 ORDER BY access_key_id`,
		accountID, userName,
	)
	if err != nil {
		return nil, fmt.Errorf("list access keys %s/%s: %w", accountID, userName, err)
	}
	defer rows.Close()

	var out []AccessKeyMeta
	for rows.Next() {
		var m AccessKeyMeta
		var un sql.NullString
		if err := rows.Scan(&m.AccessKeyID, &un, &m.Status); err != nil {
			return nil, fmt.Errorf("list access keys %s/%s: %w", accountID, userName, err)
		}
		if un.Valid {
			m.UserName = un.String
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list access keys %s/%s: %w", accountID, userName, err)
	}
	if out == nil {
		out = []AccessKeyMeta{}
	}
	return out, nil
}

// DeleteAccessKey removes an access key by id.
func (s *Store) DeleteAccessKey(accessKeyID string) error {
	res, err := s.db.Exec(`DELETE FROM access_keys WHERE access_key_id = ?`, accessKeyID)
	if err != nil {
		return fmt.Errorf("delete access key %s: %w", accessKeyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete access key %s: %w", accessKeyID, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateAccessKey sets status to Active or Inactive.
func (s *Store) UpdateAccessKey(accessKeyID, status string) error {
	switch status {
	case AccessKeyStatusActive, AccessKeyStatusInactive:
	default:
		return fmt.Errorf("update access key %s: status must be %q or %q", accessKeyID, AccessKeyStatusActive, AccessKeyStatusInactive)
	}
	res, err := s.db.Exec(
		`UPDATE access_keys SET status = ? WHERE access_key_id = ?`,
		status, accessKeyID,
	)
	if err != nil {
		return fmt.Errorf("update access key %s: %w", accessKeyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update access key %s: %w", accessKeyID, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
