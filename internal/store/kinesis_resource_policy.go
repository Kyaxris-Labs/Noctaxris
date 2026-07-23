package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
)

// EnsureKinesisResourcePolicySchema adds resource_policy on kinesis_streams.
func EnsureKinesisResourcePolicySchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure kinesis resource policy schema: db is nil")
	}
	_, err := db.Exec(`ALTER TABLE kinesis_streams ADD COLUMN resource_policy TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return nil
		}
		return fmt.Errorf("ensure kinesis resource policy schema: %w", err)
	}
	return nil
}

// EnsureKinesisResourcePolicySchema ensures the stream resource_policy column.
func (s *Store) EnsureKinesisResourcePolicySchema() error {
	return EnsureKinesisResourcePolicySchema(s.db)
}

func (s *Store) resolveKinesisStreamName(accountID, nameOrARN string) (string, error) {
	nameOrARN = strings.TrimSpace(nameOrARN)
	if nameOrARN == "" {
		return "", fmt.Errorf("kinesis stream: name or ARN is required")
	}
	if strings.Contains(nameOrARN, ":stream/") {
		name, err := parseKinesisStreamNameFromARN(nameOrARN)
		if err != nil {
			return "", err
		}
		if _, err := s.getKinesisStream(accountID, name); err != nil {
			return "", err
		}
		return name, nil
	}
	if _, err := s.getKinesisStream(accountID, nameOrARN); err != nil {
		return "", err
	}
	return nameOrARN, nil
}

// PutKinesisResourcePolicy replaces the stream resource policy document.
func (s *Store) PutKinesisResourcePolicy(accountID, nameOrARN, policy string) error {
	name, err := s.resolveKinesisStreamName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	policy = strings.TrimSpace(policy)
	if policy == "" {
		return fmt.Errorf("put kinesis resource policy: Policy is required")
	}
	if err := authz.ValidateResourcePolicyDocument(policy); err != nil {
		return fmt.Errorf("put kinesis resource policy: %w", err)
	}
	res, err := s.db.Exec(
		`UPDATE kinesis_streams SET resource_policy = ? WHERE account_id = ? AND stream_name = ?`,
		policy, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("put kinesis resource policy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("put kinesis resource policy: %w", err)
	}
	if n == 0 {
		return ErrKinesisStreamNotFound
	}
	return nil
}

// GetKinesisResourcePolicy returns the stream policy or ErrNoSuchResourcePolicy when empty.
func (s *Store) GetKinesisResourcePolicy(accountID, nameOrARN string) (string, error) {
	name, err := s.resolveKinesisStreamName(accountID, nameOrARN)
	if err != nil {
		return "", err
	}
	var policy string
	err = s.db.QueryRow(
		`SELECT COALESCE(resource_policy, '') FROM kinesis_streams WHERE account_id = ? AND stream_name = ?`,
		accountID, name,
	).Scan(&policy)
	if err != nil {
		return "", fmt.Errorf("get kinesis resource policy: %w", err)
	}
	if strings.TrimSpace(policy) == "" {
		return "", ErrNoSuchResourcePolicy
	}
	return policy, nil
}

// DeleteKinesisResourcePolicy clears the stream resource policy.
func (s *Store) DeleteKinesisResourcePolicy(accountID, nameOrARN string) error {
	name, err := s.resolveKinesisStreamName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE kinesis_streams SET resource_policy = '' WHERE account_id = ? AND stream_name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete kinesis resource policy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete kinesis resource policy: %w", err)
	}
	if n == 0 {
		return ErrKinesisStreamNotFound
	}
	return nil
}
