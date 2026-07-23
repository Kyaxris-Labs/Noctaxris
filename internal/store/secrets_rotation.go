package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultSecretRecoveryDays = 30
	minSecretRecoveryDays     = 7
	maxSecretRecoveryDays     = 30

	// SecretRotationStepFinishSecret is the lab single-step rotation event Step.
	// Full AWS createSecret/setSecret/testSecret/finishSecret staging is deferred.
	SecretRotationStepFinishSecret = "finishSecret"
)

// ErrSecretScheduledDeletion is returned when a secret is pending recovery-window deletion.
var ErrSecretScheduledDeletion = fmt.Errorf("InvalidRequestException: secret is scheduled for deletion")

// ErrSecretRotationLambdaInvalid is returned when RotationLambdaARN does not resolve.
var ErrSecretRotationLambdaInvalid = fmt.Errorf("InvalidRequestException: RotationLambdaARN is invalid or the function was not found")

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
	return EnsureSecretsRotationSchema(db)
}

func (s *Store) EnsureSecretsRecoverySchema() error {
	return EnsureSecretsRecoverySchema(s.db)
}

// EnsureSecretsRotationSchema adds optional Lambda rotator columns.
func EnsureSecretsRotationSchema(db *sql.DB) error {
	alters := []string{
		`ALTER TABLE secretsmanager_secrets ADD COLUMN rotation_lambda_arn TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secretsmanager_secrets ADD COLUMN rotation_role_arn TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range alters {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("ensure secrets rotation schema: %w", err)
		}
	}
	return nil
}

func (s *Store) EnsureSecretsRotationSchema() error {
	return EnsureSecretsRotationSchema(s.db)
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

// SetSecretRotationConfig persists optional Lambda rotator settings for a secret.
// Empty lambdaARN clears the rotator (lab random RotateSecret path).
func (s *Store) SetSecretRotationConfig(accountID, nameOrARN, lambdaARN, roleARN string) error {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	if _, err := s.getSecretRow(accountID, name); err != nil {
		return err
	}
	lambdaARN = strings.TrimSpace(lambdaARN)
	roleARN = strings.TrimSpace(roleARN)
	_, err = s.db.Exec(
		`UPDATE secretsmanager_secrets
		 SET rotation_lambda_arn = ?, rotation_role_arn = ?
		 WHERE account_id = ? AND name = ?`,
		lambdaARN, roleARN, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("set secret rotation config: %w", err)
	}
	return nil
}

// SecretRotationEventJSON builds the lab single-step rotator payload.
func SecretRotationEventJSON(secretID, clientRequestToken string) (string, error) {
	token := strings.TrimSpace(clientRequestToken)
	if token == "" {
		token = uuid.NewString()
	}
	raw, err := json.Marshal(map[string]string{
		"SecretId":           strings.TrimSpace(secretID),
		"ClientRequestToken": token,
		"Step":               SecretRotationStepFinishSecret,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// ResolveSecretRotationLambda returns the rotator function account/name/qualifier and
// the role ARN that PassRole must cover (configured rotation role, else function role).
func (s *Store) ResolveSecretRotationLambda(accountID, nameOrARN string) (fnAccount, functionName, qualifier, passRoleARN string, err error) {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return "", "", "", "", err
	}
	row, err := s.getSecretRow(accountID, name)
	if err != nil {
		return "", "", "", "", err
	}
	lambdaARN := strings.TrimSpace(row.RotationLambdaARN)
	if lambdaARN == "" {
		return "", "", "", "", nil
	}
	fnAccount = resourceOwnerAccountFromARN(lambdaARN)
	if fnAccount == "" {
		fnAccount = accountID
	}
	baseName, qualifier := ParseFunctionQualifier(lambdaARN)
	if baseName == "" {
		return "", "", "", "", ErrSecretRotationLambdaInvalid
	}
	fn, err := s.GetFunction(fnAccount, baseName)
	if err != nil {
		return "", "", "", "", ErrSecretRotationLambdaInvalid
	}
	passRoleARN = strings.TrimSpace(row.RotationRoleARN)
	if passRoleARN == "" {
		passRoleARN = strings.TrimSpace(fn.RoleARN)
	}
	return fnAccount, baseName, qualifier, passRoleARN, nil
}

// EnqueueSecretRotationInvoke records a quiet async Invoke for the configured rotator.
// Callers must ProcessAsyncInvocation and only then call FinishSecretRotation on success.
func (s *Store) EnqueueSecretRotationInvoke(accountID, nameOrARN, clientRequestToken string) (LambdaAsyncInvocation, error) {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return LambdaAsyncInvocation{}, err
	}
	row, err := s.getSecretRow(accountID, name)
	if err != nil {
		return LambdaAsyncInvocation{}, err
	}
	if strings.TrimSpace(row.DeletionDate) != "" {
		return LambdaAsyncInvocation{}, ErrSecretScheduledDeletion
	}
	fnAccount, functionName, qualifier, _, err := s.ResolveSecretRotationLambda(accountID, name)
	if err != nil {
		return LambdaAsyncInvocation{}, err
	}
	if functionName == "" {
		return LambdaAsyncInvocation{}, ErrSecretRotationLambdaInvalid
	}
	eventJSON, err := SecretRotationEventJSON(row.ARN, clientRequestToken)
	if err != nil {
		return LambdaAsyncInvocation{}, err
	}
	return s.EnqueueAsyncInvokeQuiet(fnAccount, functionName, qualifier, eventJSON)
}

// FinishSecretRotation applies lab finishSecret: random secret string replacement.
// Only call after a successful rotator Invoke.
func (s *Store) FinishSecretRotation(accountID, nameOrARN string) (Secret, error) {
	return s.RotateSecret(accountID, nameOrARN)
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
