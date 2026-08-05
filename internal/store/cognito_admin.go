package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// EnsureCognitoAdminSchema adds the per-user enabled column used by AdminDisableUser.
func EnsureCognitoAdminSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cognito admin schema: db is nil")
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE cognito_users ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`,
	}); err != nil {
		return fmt.Errorf("ensure cognito admin schema: migrate: %w", err)
	}
	return nil
}

func (s *Store) EnsureCognitoAdminSchema() error {
	return EnsureCognitoAdminSchema(s.db)
}

// AdminGetCognitoUser returns a pool user (password never returned).
func (s *Store) AdminGetCognitoUser(accountID, poolID, username string) (CognitoUser, error) {
	if err := s.EnsureCognitoAdminSchema(); err != nil {
		return CognitoUser{}, err
	}
	username = strings.TrimSpace(username)
	poolID = strings.TrimSpace(poolID)
	if username == "" || poolID == "" {
		return CognitoUser{}, fmt.Errorf("%w: Username and UserPoolId required", ErrCognitoBadRequest)
	}
	if _, err := s.DescribeCognitoUserPool(accountID, poolID); err != nil {
		return CognitoUser{}, err
	}
	var u CognitoUser
	var enabled int
	err := s.db.QueryRow(
		`SELECT username, sub, user_status, pool_id, created_at, enabled
		 FROM cognito_users WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	).Scan(&u.Username, &u.Sub, &u.UserStatus, &u.PoolID, &u.CreatedAt, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return CognitoUser{}, ErrCognitoUserNotFound
	}
	if err != nil {
		return CognitoUser{}, fmt.Errorf("admin get user: %w", err)
	}
	u.Enabled = enabled == 1
	return u, nil
}

// ListCognitoUsers lists users in a pool (lab lite; no filter/pagination).
func (s *Store) ListCognitoUsers(accountID, poolID string) ([]CognitoUser, error) {
	if err := s.EnsureCognitoAdminSchema(); err != nil {
		return nil, err
	}
	poolID = strings.TrimSpace(poolID)
	if poolID == "" {
		return nil, fmt.Errorf("%w: UserPoolId required", ErrCognitoBadRequest)
	}
	if _, err := s.DescribeCognitoUserPool(accountID, poolID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT username, sub, user_status, pool_id, created_at, enabled
		 FROM cognito_users WHERE account_id = ? AND pool_id = ?
		 ORDER BY username`,
		accountID, poolID,
	)
	if err != nil {
		return nil, fmt.Errorf("list cognito users: %w", err)
	}
	defer rows.Close()
	var out []CognitoUser
	for rows.Next() {
		var u CognitoUser
		var enabled int
		if err := rows.Scan(&u.Username, &u.Sub, &u.UserStatus, &u.PoolID, &u.CreatedAt, &enabled); err != nil {
			return nil, fmt.Errorf("list cognito users: %w", err)
		}
		u.Enabled = enabled == 1
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cognito users: %w", err)
	}
	if out == nil {
		out = []CognitoUser{}
	}
	return out, nil
}

// AdminSetCognitoUserPassword sets a user password. Permanent=true keeps/sets CONFIRMED;
// Permanent=false sets FORCE_CHANGE_PASSWORD (lab lite).
func (s *Store) AdminSetCognitoUserPassword(accountID, poolID, username, password string, permanent bool) error {
	if err := s.EnsureCognitoAdminSchema(); err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	poolID = strings.TrimSpace(poolID)
	if username == "" || poolID == "" || password == "" {
		return fmt.Errorf("%w: Username, UserPoolId, and Password required", ErrCognitoBadRequest)
	}
	if _, err := s.DescribeCognitoUserPool(accountID, poolID); err != nil {
		return err
	}
	if _, _, _, err := s.getUser(accountID, poolID, username); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	status := "FORCE_CHANGE_PASSWORD"
	if permanent {
		status = "CONFIRMED"
	}
	res, err := s.db.Exec(
		`UPDATE cognito_users SET password_hash = ?, user_status = ?
		 WHERE account_id = ? AND pool_id = ? AND username = ?`,
		hash, status, accountID, poolID, username,
	)
	if err != nil {
		return fmt.Errorf("admin set user password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCognitoUserNotFound
	}
	return s.storeUserSRPVerifier(accountID, poolID, username, password)
}

// AdminDeleteCognitoUser deletes a user and related refresh tokens.
func (s *Store) AdminDeleteCognitoUser(accountID, poolID, username string) error {
	username = strings.TrimSpace(username)
	poolID = strings.TrimSpace(poolID)
	if username == "" || poolID == "" {
		return fmt.Errorf("%w: Username and UserPoolId required", ErrCognitoBadRequest)
	}
	if _, err := s.DescribeCognitoUserPool(accountID, poolID); err != nil {
		return err
	}
	if _, _, _, err := s.getUser(accountID, poolID, username); err != nil {
		return err
	}
	_, _ = s.db.Exec(
		`DELETE FROM cognito_refresh_tokens WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	)
	res, err := s.db.Exec(
		`DELETE FROM cognito_users WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	)
	if err != nil {
		return fmt.Errorf("admin delete user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCognitoUserNotFound
	}
	return nil
}

// AdminDisableCognitoUser deactivates a user (Enabled=false).
func (s *Store) AdminDisableCognitoUser(accountID, poolID, username string) error {
	if err := s.EnsureCognitoAdminSchema(); err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	poolID = strings.TrimSpace(poolID)
	if username == "" || poolID == "" {
		return fmt.Errorf("%w: Username and UserPoolId required", ErrCognitoBadRequest)
	}
	if _, err := s.DescribeCognitoUserPool(accountID, poolID); err != nil {
		return err
	}
	if _, _, _, err := s.getUser(accountID, poolID, username); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE cognito_users SET enabled = 0
		 WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	)
	if err != nil {
		return fmt.Errorf("admin disable user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCognitoUserNotFound
	}
	return nil
}

func (s *Store) cognitoUserEnabled(accountID, poolID, username string) (bool, error) {
	if err := s.EnsureCognitoAdminSchema(); err != nil {
		return false, err
	}
	var enabled int
	err := s.db.QueryRow(
		`SELECT enabled FROM cognito_users WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrCognitoUserNotFound
	}
	if err != nil {
		return false, fmt.Errorf("cognito user enabled: %w", err)
	}
	return enabled == 1, nil
}
