package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	defaultSecretRecoveryDays = 30
	minSecretRecoveryDays     = 7
	maxSecretRecoveryDays     = 30
)

// ErrSecretScheduledDeletion is returned when a secret is pending recovery-window deletion.
var ErrSecretScheduledDeletion = fmt.Errorf("InvalidRequestException: secret is scheduled for deletion")

// EnsureSecretsRecoverySchema adds recovery-window columns.
func EnsureSecretsRecoverySchema(db *sql.DB) error {
	alters := []string{
		`ALTER TABLE secretsmanager_secrets ADD COLUMN deleted_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secretsmanager_secrets ADD COLUMN deletion_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secretsmanager_secrets ADD COLUMN recovery_window_in_days INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range alters {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("ensure secrets recovery schema: %w", err)
		}
	}
	return nil
}

func (s *Store) EnsureSecretsRecoverySchema() error {
	return EnsureSecretsRecoverySchema(s.db)
}

// DeleteSecretWithRecovery schedules deletion with a recovery window, or force-deletes immediately.
func (s *Store) DeleteSecretWithRecovery(accountID, nameOrARN string, recoveryWindowDays int, force bool) (Secret, time.Time, error) {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return Secret{}, time.Time{}, err
	}
	row, err := s.getSecretRow(accountID, name)
	if err != nil {
		return Secret{}, time.Time{}, err
	}
	sec, err := s.secretFromRow(row, false)
	if err != nil {
		return Secret{}, time.Time{}, err
	}
	now := time.Now().UTC()
	if force {
		if err := s.DeleteSecret(accountID, name); err != nil {
			return Secret{}, time.Time{}, err
		}
		return sec, now, nil
	}
	days := recoveryWindowDays
	if days <= 0 {
		days = defaultSecretRecoveryDays
	}
	if days < minSecretRecoveryDays {
		days = minSecretRecoveryDays
	}
	if days > maxSecretRecoveryDays {
		days = maxSecretRecoveryDays
	}
	deletion := now.Add(time.Duration(days) * 24 * time.Hour)
	_, err = s.db.Exec(
		`UPDATE secretsmanager_secrets
		 SET deleted_date = ?, deletion_date = ?, recovery_window_in_days = ?
		 WHERE account_id = ? AND name = ?`,
		now.Format(time.RFC3339), deletion.Format(time.RFC3339), days, accountID, name,
	)
	if err != nil {
		return Secret{}, time.Time{}, fmt.Errorf("schedule secret deletion: %w", err)
	}
	sec.DeletedDate = now.Format(time.RFC3339)
	sec.DeletionDate = deletion.Format(time.RFC3339)
	return sec, deletion, nil
}

// RestoreSecret clears a scheduled deletion.
func (s *Store) RestoreSecret(accountID, nameOrARN string) error {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE secretsmanager_secrets SET deleted_date = '', deletion_date = '', recovery_window_in_days = 0
		 WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("restore secret: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSecretNotFound
	}
	return nil
}

// SweepExpiredSecrets hard-deletes secrets past deletion_date.
func (s *Store) SweepExpiredSecrets(now time.Time) (int, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res, err := s.db.Exec(
		`DELETE FROM secretsmanager_secrets
		 WHERE deletion_date != '' AND deletion_date <= ?`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("sweep secrets: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// RotateSecret lab-rotates the secret string to a new random value (no Lambda).
func (s *Store) RotateSecret(accountID, nameOrARN string) (Secret, error) {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return Secret{}, err
	}
	row, err := s.getSecretRow(accountID, name)
	if err != nil {
		return Secret{}, err
	}
	if strings.TrimSpace(row.DeletionDate) != "" {
		return Secret{}, ErrSecretScheduledDeletion
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return Secret{}, fmt.Errorf("rotate secret: %w", err)
	}
	newVal := hex.EncodeToString(b[:])
	return s.PutSecretValue(accountID, name, newVal, nil)
}
