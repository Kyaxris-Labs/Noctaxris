package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultSecretRecoveryDays = 30
	minSecretRecoveryDays     = 7
	maxSecretRecoveryDays     = 30

	// SecretRotationStepCreateSecret is the first Lambda rotation step.
	SecretRotationStepCreateSecret = "createSecret"
	// SecretRotationStepSetSecret is the second Lambda rotation step.
	SecretRotationStepSetSecret = "setSecret"
	// SecretRotationStepTestSecret is the third Lambda rotation step.
	SecretRotationStepTestSecret = "testSecret"
	// SecretRotationStepFinishSecret is the fourth Lambda rotation step.
	SecretRotationStepFinishSecret = "finishSecret"
)

// ErrSecretScheduledDeletion is returned when a secret is pending recovery-window deletion.
var ErrSecretScheduledDeletion = fmt.Errorf("InvalidRequestException: secret is scheduled for deletion")

// ErrSecretRotationLambdaInvalid is returned when RotationLambdaARN does not resolve.
var ErrSecretRotationLambdaInvalid = fmt.Errorf("InvalidRequestException: RotationLambdaARN is invalid or the function was not found")

// SecretRotationInvoker invokes the rotator Lambda once with the given event JSON.
// Store tests inject a fake; the server enqueues quiet async Invoke and processes it.
type SecretRotationInvoker func(eventJSON string) error

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
	if err := EnsureSecretsRotationSchema(db); err != nil {
		return err
	}
	if err := EnsureSecretsRotationScheduleSchema(db); err != nil {
		return err
	}
	return EnsureSecretsVersionsSchema(db)
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
	rows, err := s.db.Query(
		`SELECT account_id, name FROM secretsmanager_secrets
		 WHERE deletion_date != '' AND deletion_date <= ?`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("sweep secrets: query: %w", err)
	}
	defer rows.Close()
	type pair struct{ accountID, name string }
	var doomed []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.accountID, &p.name); err != nil {
			return 0, fmt.Errorf("sweep secrets: scan: %w", err)
		}
		doomed = append(doomed, p)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, p := range doomed {
		if err := s.deleteSecretVersions(p.accountID, p.name); err != nil {
			return 0, err
		}
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

// SecretRotationEventJSON builds a rotator payload for finishSecret (compat helper).
func SecretRotationEventJSON(secretID, clientRequestToken string) (string, error) {
	return SecretRotationEventJSONForStep(secretID, clientRequestToken, SecretRotationStepFinishSecret)
}

// SecretRotationEventJSONForStep builds the Lambda rotation event for one step.
func SecretRotationEventJSONForStep(secretID, clientRequestToken, step string) (string, error) {
	token := strings.TrimSpace(clientRequestToken)
	if token == "" {
		token = uuid.NewString()
	}
	step = strings.TrimSpace(step)
	if step == "" {
		step = SecretRotationStepFinishSecret
	}
	raw, err := json.Marshal(map[string]string{
		"SecretId":           strings.TrimSpace(secretID),
		"ClientRequestToken": token,
		"Step":               step,
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

// EnqueueSecretRotationInvoke records a quiet async Invoke for one rotation step.
func (s *Store) EnqueueSecretRotationInvoke(accountID, nameOrARN, clientRequestToken string) (LambdaAsyncInvocation, error) {
	return s.EnqueueSecretRotationInvokeStep(accountID, nameOrARN, clientRequestToken, SecretRotationStepFinishSecret)
}

// EnqueueSecretRotationInvokeStep records a quiet async Invoke for the configured rotator step.
func (s *Store) EnqueueSecretRotationInvokeStep(accountID, nameOrARN, clientRequestToken, step string) (LambdaAsyncInvocation, error) {
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
	eventJSON, err := SecretRotationEventJSONForStep(row.ARN, clientRequestToken, step)
	if err != nil {
		return LambdaAsyncInvocation{}, err
	}
	return s.EnqueueAsyncInvokeQuiet(fnAccount, functionName, qualifier, eventJSON)
}

// RotateSecretFourStep runs createSecret → setSecret → testSecret → finishSecret.
// createSecret creates an AWSPENDING version; finishSecret promotes it to AWSCURRENT.
// invoke is called once per step with the Lambda event JSON (inject a fake in store tests).
func (s *Store) RotateSecretFourStep(accountID, nameOrARN, clientRequestToken string, invoke SecretRotationInvoker) (Secret, error) {
	if invoke == nil {
		return Secret{}, fmt.Errorf("rotation invoker is required")
	}
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
	if err := s.ensureSecretVersionsBackfilled(accountID, name); err != nil {
		return Secret{}, err
	}
	orphan, err := s.HasPendingRotationOrphan(accountID, name)
	if err != nil {
		return Secret{}, err
	}
	if orphan {
		return Secret{}, ErrSecretRotationInProgress
	}

	token := strings.TrimSpace(clientRequestToken)
	if token == "" {
		token = uuid.NewString()
	}

	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return Secret{}, fmt.Errorf("rotate secret: %w", err)
	}
	pendingValue := hex.EncodeToString(b[:])
	if _, err := s.createAWSPENDINGVersion(accountID, name, token, pendingValue, nil); err != nil {
		return Secret{}, err
	}

	steps := []string{
		SecretRotationStepCreateSecret,
		SecretRotationStepSetSecret,
		SecretRotationStepTestSecret,
		SecretRotationStepFinishSecret,
	}
	for _, step := range steps {
		eventJSON, err := SecretRotationEventJSONForStep(row.ARN, token, step)
		if err != nil {
			return Secret{}, err
		}
		if err := invoke(eventJSON); err != nil {
			return Secret{}, fmt.Errorf("rotation step %s: %w", step, err)
		}
	}
	if err := s.promoteSecretVersionCurrent(accountID, name, token); err != nil {
		return Secret{}, err
	}
	return s.GetSecretValue(accountID, name)
}

// FinishSecretRotation promotes an existing AWSPENDING version to AWSCURRENT.
// Prefer RotateSecretFourStep for new Lambda-backed rotates.
func (s *Store) FinishSecretRotation(accountID, nameOrARN string) (Secret, error) {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return Secret{}, err
	}
	pending, err := s.findSecretVersionByStage(accountID, name, SecretVersionStagePending)
	if err != nil {
		if errors.Is(err, ErrSecretNotFound) {
			// Legacy path: no staging row — random put as before.
			return s.RotateSecret(accountID, nameOrARN)
		}
		return Secret{}, err
	}
	if err := s.promoteSecretVersionCurrent(accountID, name, pending.VersionID); err != nil {
		return Secret{}, err
	}
	return s.GetSecretValue(accountID, name)
}

// RotateSecret lab-rotates the secret string to a new random AWSCURRENT value (no Lambda).
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
