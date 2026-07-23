package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// DefaultSecretsRegion is the lab region embedded in Secrets Manager ARNs.
	DefaultSecretsRegion = "us-east-1"

	// AliasAWSSecretsManager is the lab convenience alias for Secrets Manager defaults.
	AliasAWSSecretsManager = "alias/aws/secretsmanager"
)

var (
	ErrSecretAlreadyExists = errors.New("ResourceExistsException")
	ErrSecretNotFound      = errors.New("ResourceNotFoundException")
)

// Secret is a Secrets Manager secret metadata row. Value fields are populated
// on create/get/put responses and omitted on describe/list.
type Secret struct {
	Name               string
	ARN                string
	SecretString       string
	SecretBinary       []byte
	Version            int
	VersionID          string
	VersionStages      []string
	VersionIdsToStages map[string][]string
	KmsKeyID           string
	Description        string
	ResourcePolicy     string
	CreatedDate        string
	LastChangedDate    string
	DeletedDate        string
	DeletionDate       string
	RotationLambdaARN  string
	RotationRoleARN    string
}

const secretsSchema = `
CREATE TABLE IF NOT EXISTS secretsmanager_secrets (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  arn_suffix TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  secret_string_plain TEXT NOT NULL DEFAULT '',
  secret_string_sealed BLOB,
  string_sealed INTEGER NOT NULL DEFAULT 0,
  secret_binary_plain BLOB,
  secret_binary_sealed BLOB,
  binary_sealed INTEGER NOT NULL DEFAULT 0,
  kms_key_id TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL DEFAULT 1,
  resource_policy TEXT NOT NULL DEFAULT '',
  created_date TEXT NOT NULL,
  last_changed_date TEXT NOT NULL,
  PRIMARY KEY (account_id, name)
);
`

// EnsureSecretsSchema creates the Secrets Manager table if missing.
func EnsureSecretsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure secrets schema: db is nil")
	}
	if _, err := db.Exec(secretsSchema); err != nil {
		return fmt.Errorf("ensure secrets schema: %w", err)
	}
	return EnsureSecretsRecoverySchema(db)
}

// EnsureSecretsSchema ensures Secrets Manager tables on an open store (tests and Open wiring).
func (s *Store) EnsureSecretsSchema() error {
	return EnsureSecretsSchema(s.db)
}

// SecretARN builds arn:aws:secretsmanager:REGION:ACCOUNT:secret:NAME-SUFFIX.
// Lab uses a 6-character lowercase hex suffix (AWS-like random suffix).
func SecretARN(region, accountID, name, suffix string) string {
	if region == "" {
		region = DefaultSecretsRegion
	}
	return fmt.Sprintf("arn:aws:secretsmanager:%s:%s:secret:%s-%s", region, accountID, name, suffix)
}

func newSecretARNSuffix() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToLower(hex.EncodeToString(b[:])), nil
}

// NewSecretARNSuffix returns a lab six-character hex ARN suffix (AWS-shaped).
func NewSecretARNSuffix() (string, error) {
	return newSecretARNSuffix()
}

// SecretsEncryptionContext is the AWS Secrets Manager KMS EncryptionContext map
// (SecretARN + SecretVersionId) used for seal/unseal and EvaluateKMS conditions.
func SecretsEncryptionContext(secretARN, versionID string) map[string]string {
	return map[string]string{
		"SecretARN":       secretARN,
		"SecretVersionId": versionID,
	}
}

func normalizeSecretLookup(nameOrARN string) string {
	return strings.TrimSpace(nameOrARN)
}

func secretNameFromARN(arn string) (string, error) {
	arn = strings.TrimSpace(arn)
	const marker = ":secret:"
	idx := strings.LastIndex(arn, marker)
	if idx < 0 {
		return "", fmt.Errorf("invalid secret arn %q", arn)
	}
	nameWithSuffix := arn[idx+len(marker):]
	if nameWithSuffix == "" {
		return "", fmt.Errorf("invalid secret arn %q", arn)
	}
	// Name is everything before the last hyphen plus 7-char suffix (hyphen + 6 hex).
	if i := strings.LastIndex(nameWithSuffix, "-"); i > 0 && len(nameWithSuffix)-i-1 == 6 {
		return nameWithSuffix[:i], nil
	}
	return nameWithSuffix, nil
}

// EnsureSecretsManagerAlias seeds alias/aws/secretsmanager for the account, creating a lab CMK when missing.
// Idempotent: returns the target key id when the alias already exists.
func (s *Store) EnsureSecretsManagerAlias(accountID string) (string, error) {
	if strings.TrimSpace(accountID) == "" {
		return "", fmt.Errorf("ensure secretsmanager alias: account id required")
	}

	keyID, err := s.lookupAliasTarget(accountID, AliasAWSSecretsManager)
	if err == nil {
		return keyID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("ensure secretsmanager alias: lookup: %w", err)
	}

	k, err := s.CreateKey(accountID, fmt.Sprintf("arn:aws:iam::%s:root", accountID), "")
	if err != nil {
		return "", fmt.Errorf("ensure secretsmanager alias: create key: %w", err)
	}
	if err := s.CreateAlias(accountID, AliasAWSSecretsManager, k.KeyID); err != nil {
		return "", fmt.Errorf("ensure secretsmanager alias: create alias: %w", err)
	}
	return k.KeyID, nil
}

// ResolveSecretsManagerKeyID resolves KmsKeyId or defaults to alias/aws/secretsmanager.
func (s *Store) ResolveSecretsManagerKeyID(accountID, keyIDOrAlias string) (string, error) {
	id := strings.TrimSpace(keyIDOrAlias)
	if id == "" || id == AliasAWSSecretsManager {
		return s.EnsureSecretsManagerAlias(accountID)
	}
	if strings.HasPrefix(id, "alias/aws/secretsmanager") {
		if _, err := s.EnsureSecretsManagerAlias(accountID); err != nil {
			return "", err
		}
	}
	return s.ResolveKeyID(accountID, id)
}

func (s *Store) resolveSecretsKeyID(accountID, keyIDOrAlias string) (string, error) {
	return s.ResolveSecretsManagerKeyID(accountID, keyIDOrAlias)
}

type secretRow struct {
	Name               string
	ARN                string
	ARNSuffix          string
	Description        string
	SecretStringPlain  string
	SecretStringSealed []byte
	StringSealed       bool
	SecretBinaryPlain  []byte
	SecretBinarySealed []byte
	BinarySealed       bool
	KMSKeyID           string
	Version            int
	ResourcePolicy     string
	CreatedDate        string
	LastChangedDate    string
	DeletedDate        string
	DeletionDate       string
	RotationLambdaARN  string
	RotationRoleARN    string
}

func (s *Store) resolveSecretName(accountID, nameOrARN string) (string, error) {
	lookup := normalizeSecretLookup(nameOrARN)
	if lookup == "" {
		return "", fmt.Errorf("secret name is required")
	}
	if strings.HasPrefix(lookup, "arn:aws:secretsmanager:") {
		return secretNameFromARN(lookup)
	}
	row, err := s.getSecretRow(accountID, lookup)
	if err != nil {
		return "", err
	}
	return row.Name, nil
}

func (s *Store) getSecretRow(accountID, name string) (secretRow, error) {
	name = normalizeSecretLookup(name)
	var (
		row          secretRow
		stringSeal   int
		binarySeal   int
		stringSealed []byte
		binaryPlain  []byte
		binarySealed []byte
	)
	err := s.db.QueryRow(
		`SELECT name, arn, arn_suffix, description, secret_string_plain, secret_string_sealed, string_sealed,
		        secret_binary_plain, secret_binary_sealed, binary_sealed, kms_key_id, version, resource_policy,
		        created_date, last_changed_date,
		        COALESCE(deleted_date, ''), COALESCE(deletion_date, ''),
		        COALESCE(rotation_lambda_arn, ''), COALESCE(rotation_role_arn, '')
		 FROM secretsmanager_secrets WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(
		&row.Name, &row.ARN, &row.ARNSuffix, &row.Description, &row.SecretStringPlain, &stringSealed,
		&stringSeal, &binaryPlain, &binarySealed, &binarySeal, &row.KMSKeyID, &row.Version, &row.ResourcePolicy,
		&row.CreatedDate, &row.LastChangedDate, &row.DeletedDate, &row.DeletionDate,
		&row.RotationLambdaARN, &row.RotationRoleARN,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return secretRow{}, ErrSecretNotFound
		}
		return secretRow{}, fmt.Errorf("get secret row %s: %w", name, err)
	}
	row.StringSealed = stringSeal == 1
	row.BinarySealed = binarySeal == 1
	row.SecretStringSealed = stringSealed
	row.SecretBinaryPlain = binaryPlain
	row.SecretBinarySealed = binarySealed
	return row, nil
}

func (s *Store) sealSecretValue(accountID, keyID string, plaintext []byte, encCtx map[string]string) ([]byte, string, error) {
	resolvedKeyID, err := s.resolveSecretsKeyID(accountID, keyID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve key: %w", err)
	}
	k, err := s.GetKey(resolvedKeyID)
	if err != nil {
		return nil, "", fmt.Errorf("key: %w", err)
	}
	if !KeyUsableForCrypto(k.KeyState) {
		return nil, "", fmt.Errorf("%w", ErrInvalidKeyState)
	}
	cmk, err := s.UnsealKeyMaterial(resolvedKeyID)
	if err != nil {
		return nil, "", fmt.Errorf("unseal key: %w", err)
	}
	sealed, err := EncryptUnderCMK(cmk, resolvedKeyID, plaintext, encCtx)
	if err != nil {
		return nil, "", fmt.Errorf("encrypt: %w", err)
	}
	return sealed, resolvedKeyID, nil
}

func (s *Store) storeSecretValues(
	accountID, keyID, secretString string, secretBinary []byte, encCtx map[string]string,
) (stringPlain string, stringSealed []byte, stringSealFlag int, binaryPlain, binarySealed []byte, binarySealFlag int, kmsKeyID string, err error) {
	hasString := secretString != ""
	hasBinary := len(secretBinary) > 0
	if !hasString && !hasBinary {
		// Metadata-only secret (no current version payload), matching AWS CreateSecret.
		return "", nil, 0, nil, nil, 0, "", nil
	}

	resolvedKeyID := ""
	if hasString {
		sealed, kid, sealErr := s.sealSecretValue(accountID, keyID, []byte(secretString), encCtx)
		if sealErr != nil {
			return "", nil, 0, nil, nil, 0, "", sealErr
		}
		stringSealed = sealed
		stringSealFlag = 1
		resolvedKeyID = kid
	}
	if hasBinary {
		sealed, kid, sealErr := s.sealSecretValue(accountID, keyID, secretBinary, encCtx)
		if sealErr != nil {
			return "", nil, 0, nil, nil, 0, "", sealErr
		}
		binarySealed = sealed
		binarySealFlag = 1
		if resolvedKeyID == "" {
			resolvedKeyID = kid
		}
	}
	return "", stringSealed, stringSealFlag, nil, binarySealed, binarySealFlag, resolvedKeyID, nil
}

func (s *Store) secretFromRow(row secretRow, withValues bool) (Secret, error) {
	out := Secret{
		Name:              row.Name,
		ARN:               row.ARN,
		Version:           row.Version,
		VersionID:         fmt.Sprintf("%d", row.Version),
		KmsKeyID:          row.KMSKeyID,
		Description:       row.Description,
		ResourcePolicy:    row.ResourcePolicy,
		CreatedDate:       row.CreatedDate,
		LastChangedDate:   row.LastChangedDate,
		DeletedDate:       row.DeletedDate,
		DeletionDate:      row.DeletionDate,
		RotationLambdaARN: row.RotationLambdaARN,
		RotationRoleARN:   row.RotationRoleARN,
	}
	if !withValues {
		return out, nil
	}

	encCtx := SecretsEncryptionContext(row.ARN, fmt.Sprintf("%d", row.Version))
	if row.StringSealed {
		if row.KMSKeyID == "" {
			return Secret{}, fmt.Errorf("secret %s: sealed string without kms key id", row.Name)
		}
		k, err := s.GetKey(row.KMSKeyID)
		if err != nil {
			return Secret{}, fmt.Errorf("secret %s: key: %w", row.Name, err)
		}
		if !KeyUsableForCrypto(k.KeyState) {
			return Secret{}, fmt.Errorf("secret %s: %w", row.Name, ErrInvalidKeyState)
		}
		plain, err := s.DecryptBlobWithKeyContext(row.KMSKeyID, row.SecretStringSealed, encCtx)
		if err != nil {
			return Secret{}, fmt.Errorf("secret %s: decrypt string: %w", row.Name, err)
		}
		out.SecretString = string(plain)
	} else {
		out.SecretString = row.SecretStringPlain
	}

	if row.BinarySealed {
		if row.KMSKeyID == "" {
			return Secret{}, fmt.Errorf("secret %s: sealed binary without kms key id", row.Name)
		}
		k, err := s.GetKey(row.KMSKeyID)
		if err != nil {
			return Secret{}, fmt.Errorf("secret %s: key: %w", row.Name, err)
		}
		if !KeyUsableForCrypto(k.KeyState) {
			return Secret{}, fmt.Errorf("secret %s: %w", row.Name, ErrInvalidKeyState)
		}
		plain, err := s.DecryptBlobWithKeyContext(row.KMSKeyID, row.SecretBinarySealed, encCtx)
		if err != nil {
			return Secret{}, fmt.Errorf("secret %s: decrypt binary: %w", row.Name, err)
		}
		out.SecretBinary = plain
	} else if len(row.SecretBinaryPlain) > 0 {
		out.SecretBinary = append([]byte(nil), row.SecretBinaryPlain...)
	}

	return out, nil
}

// CreateSecret stores a new secret encrypted under KMS (default alias/aws/secretsmanager).
// arnSuffix may be empty to generate one; pass a precomputed suffix when the caller
// already authorized KMS with SecretsEncryptionContext for that ARN.
func (s *Store) CreateSecret(
	accountID, region, name, secretString string, secretBinary []byte, keyID, description, arnSuffix string,
) (Secret, error) {
	name = normalizeSecretLookup(name)
	if name == "" {
		return Secret{}, fmt.Errorf("create secret: name is required")
	}
	if region == "" {
		region = DefaultSecretsRegion
	}

	if _, err := s.getSecretRow(accountID, name); err == nil {
		return Secret{}, ErrSecretAlreadyExists
	} else if !errors.Is(err, ErrSecretNotFound) {
		return Secret{}, err
	}

	suffix := strings.TrimSpace(arnSuffix)
	if suffix == "" {
		var err error
		suffix, err = newSecretARNSuffix()
		if err != nil {
			return Secret{}, fmt.Errorf("create secret: suffix: %w", err)
		}
	}
	arn := SecretARN(region, accountID, name, suffix)
	now := nowRFC3339()
	encCtx := SecretsEncryptionContext(arn, "1")

	stringPlain, stringSealed, stringSealFlag, binaryPlain, binarySealed, binarySealFlag, kmsKeyID, err := s.storeSecretValues(
		accountID, keyID, secretString, secretBinary, encCtx,
	)
	if err != nil {
		return Secret{}, fmt.Errorf("create secret: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO secretsmanager_secrets
		 (account_id, name, arn, arn_suffix, description, secret_string_plain, secret_string_sealed, string_sealed,
		  secret_binary_plain, secret_binary_sealed, binary_sealed, kms_key_id, version, created_date, last_changed_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		accountID, name, arn, suffix, description, stringPlain, stringSealed, stringSealFlag,
		binaryPlain, binarySealed, binarySealFlag, kmsKeyID, now, now,
	)
	if err != nil {
		return Secret{}, fmt.Errorf("create secret %s: %w", name, err)
	}

	row := secretRow{
		Name:               name,
		ARN:                arn,
		ARNSuffix:          suffix,
		Description:        description,
		KMSKeyID:           kmsKeyID,
		Version:            1,
		CreatedDate:        now,
		LastChangedDate:    now,
		StringSealed:       stringSealFlag == 1,
		BinarySealed:       binarySealFlag == 1,
		SecretStringSealed: stringSealed,
		SecretBinarySealed: binarySealed,
	}
	if err := s.insertSecretVersionRow(accountID, name, "1", 1, row, []string{SecretVersionStageCurrent}, now); err != nil {
		return Secret{}, err
	}
	sec, err := s.secretFromRow(row, true)
	if err != nil {
		return Secret{}, err
	}
	sec.VersionStages = []string{SecretVersionStageCurrent}
	sec.VersionIdsToStages = map[string][]string{"1": {SecretVersionStageCurrent}}
	return sec, nil
}

// GetSecretValue returns the decrypted AWSCURRENT secret value for a name or ARN.
func (s *Store) GetSecretValue(accountID, nameOrARN string) (Secret, error) {
	return s.GetSecretValueByStage(accountID, nameOrARN, SecretVersionStageCurrent, "")
}

// PutSecretValue replaces the secret value and bumps the version.
func (s *Store) PutSecretValue(
	accountID, nameOrARN, secretString string, secretBinary []byte,
) (Secret, error) {
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

	version := row.Version + 1
	encCtx := SecretsEncryptionContext(row.ARN, fmt.Sprintf("%d", version))
	stringPlain, stringSealed, stringSealFlag, binaryPlain, binarySealed, binarySealFlag, kmsKeyID, err := s.storeSecretValues(
		accountID, row.KMSKeyID, secretString, secretBinary, encCtx,
	)
	if err != nil {
		return Secret{}, fmt.Errorf("put secret value: %w", err)
	}

	modified := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE secretsmanager_secrets
		 SET secret_string_plain = ?, secret_string_sealed = ?, string_sealed = ?,
		     secret_binary_plain = ?, secret_binary_sealed = ?, binary_sealed = ?,
		     kms_key_id = ?, version = ?, last_changed_date = ?
		 WHERE account_id = ? AND name = ?`,
		stringPlain, stringSealed, stringSealFlag, binaryPlain, binarySealed, binarySealFlag,
		kmsKeyID, version, modified, accountID, name,
	)
	if err != nil {
		return Secret{}, fmt.Errorf("put secret value %s: %w", name, err)
	}

	row.Version = version
	row.LastChangedDate = modified
	row.KMSKeyID = kmsKeyID
	row.StringSealed = stringSealFlag == 1
	row.BinarySealed = binarySealFlag == 1
	row.SecretStringSealed = stringSealed
	row.SecretBinarySealed = binarySealed
	row.SecretStringPlain = stringPlain
	row.SecretBinaryPlain = binaryPlain
	versionID := fmt.Sprintf("%d", version)
	if err := s.recordPutSecretVersion(accountID, name, row, versionID, version, []string{SecretVersionStageCurrent}); err != nil {
		return Secret{}, err
	}
	sec, err := s.secretFromRow(row, true)
	if err != nil {
		return Secret{}, err
	}
	stagesMap, err := s.VersionIdsToStages(accountID, name)
	if err != nil {
		return Secret{}, err
	}
	sec.VersionID = versionID
	sec.VersionStages = []string{SecretVersionStageCurrent}
	sec.VersionIdsToStages = stagesMap
	return sec, nil
}

// PutSecretValueWithStages stores a new version with explicit VersionStages and optional VersionId
// (ClientRequestToken). Used by rotation createSecret and UpdateSecretVersionStage choreography.
func (s *Store) PutSecretValueWithStages(
	accountID, nameOrARN, secretString string, secretBinary []byte, versionID string, stages []string,
) (Secret, error) {
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
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		versionID = fmt.Sprintf("%d", row.Version+1)
	}
	if len(stages) == 1 && stages[0] == SecretVersionStagePending {
		vrow, err := s.createAWSPENDINGVersion(accountID, name, versionID, secretString, secretBinary)
		if err != nil {
			return Secret{}, err
		}
		return s.secretFromVersionRow(row, vrow, true)
	}
	if len(stages) == 0 || (len(stages) == 1 && stages[0] == SecretVersionStageCurrent) {
		return s.PutSecretValue(accountID, name, secretString, secretBinary)
	}
	// Generic staged put: insert version without promoting unless AWSCURRENT is present.
	versionNum := row.Version + 1
	encCtx := SecretsEncryptionContext(row.ARN, versionID)
	stringPlain, stringSealed, stringSealFlag, binaryPlain, binarySealed, binarySealFlag, kmsKeyID, err := s.storeSecretValues(
		accountID, row.KMSKeyID, secretString, secretBinary, encCtx,
	)
	if err != nil {
		return Secret{}, fmt.Errorf("put secret value with stages: %w", err)
	}
	vparent := secretRow{
		SecretStringPlain:  stringPlain,
		SecretStringSealed: stringSealed,
		StringSealed:       stringSealFlag == 1,
		SecretBinaryPlain:  binaryPlain,
		SecretBinarySealed: binarySealed,
		BinarySealed:       binarySealFlag == 1,
		KMSKeyID:           kmsKeyID,
	}
	if err := s.recordPutSecretVersion(accountID, name, vparent, versionID, versionNum, stages); err != nil {
		return Secret{}, err
	}
	if stagesContain(stages, SecretVersionStageCurrent) {
		_, err = s.db.Exec(
			`UPDATE secretsmanager_secrets
			 SET secret_string_plain = ?, secret_string_sealed = ?, string_sealed = ?,
			     secret_binary_plain = ?, secret_binary_sealed = ?, binary_sealed = ?,
			     kms_key_id = ?, version = ?, last_changed_date = ?
			 WHERE account_id = ? AND name = ?`,
			stringPlain, stringSealed, stringSealFlag, binaryPlain, binarySealed, binarySealFlag,
			kmsKeyID, versionNum, nowRFC3339(), accountID, name,
		)
		if err != nil {
			return Secret{}, fmt.Errorf("put secret value with stages: parent: %w", err)
		}
	} else {
		_, err = s.db.Exec(
			`UPDATE secretsmanager_secrets SET version = ? WHERE account_id = ? AND name = ? AND version < ?`,
			versionNum, accountID, name, versionNum,
		)
		if err != nil {
			return Secret{}, fmt.Errorf("put secret value with stages: bump: %w", err)
		}
	}
	return s.GetSecretValueByStage(accountID, name, "", versionID)
}

// DeleteSecret removes a secret immediately (force path / internal).
func (s *Store) DeleteSecret(accountID, nameOrARN string) error {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	if err := s.deleteSecretVersions(accountID, name); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM secretsmanager_secrets WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete secret %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete secret %s: %w", name, err)
	}
	if affected == 0 {
		return ErrSecretNotFound
	}
	return nil
}

// DescribeSecret returns secret metadata without values.
func (s *Store) DescribeSecret(accountID, nameOrARN string) (Secret, error) {
	_, _ = s.SweepExpiredSecrets(time.Time{})
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return Secret{}, err
	}
	row, err := s.getSecretRow(accountID, name)
	if err != nil {
		return Secret{}, err
	}
	sec, err := s.secretFromRow(row, false)
	if err != nil {
		return Secret{}, err
	}
	stagesMap, err := s.VersionIdsToStages(accountID, name)
	if err != nil {
		return Secret{}, err
	}
	sec.VersionIdsToStages = stagesMap
	if current, err := s.findSecretVersionByStage(accountID, name, SecretVersionStageCurrent); err == nil {
		sec.VersionID = current.VersionID
		sec.Version = current.VersionNum
		sec.VersionStages = append([]string{}, current.Stages...)
	}
	return sec, nil
}

// ListSecrets returns secret metadata for an account. Values are omitted.
func (s *Store) ListSecrets(accountID string) ([]Secret, error) {
	_, _ = s.SweepExpiredSecrets(time.Time{})
	rows, err := s.db.Query(
		`SELECT name, arn, arn_suffix, description, secret_string_plain, secret_string_sealed, string_sealed,
		        secret_binary_plain, secret_binary_sealed, binary_sealed, kms_key_id, version, resource_policy,
		        created_date, last_changed_date,
		        COALESCE(deleted_date, ''), COALESCE(deletion_date, '')
		 FROM secretsmanager_secrets WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list secrets %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []Secret
	for rows.Next() {
		var (
			row          secretRow
			stringSeal   int
			binarySeal   int
			stringSealed []byte
			binaryPlain  []byte
			binarySealed []byte
		)
		if err := rows.Scan(
			&row.Name, &row.ARN, &row.ARNSuffix, &row.Description, &row.SecretStringPlain, &stringSealed,
			&stringSeal, &binaryPlain, &binarySealed, &binarySeal, &row.KMSKeyID, &row.Version, &row.ResourcePolicy,
			&row.CreatedDate, &row.LastChangedDate, &row.DeletedDate, &row.DeletionDate,
		); err != nil {
			return nil, fmt.Errorf("list secrets %s: %w", accountID, err)
		}
		sec, err := s.secretFromRow(row, false)
		if err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list secrets %s: %w", accountID, err)
	}
	if out == nil {
		out = []Secret{}
	}
	return out, nil
}

// PutSecretResourcePolicy replaces the secret resource policy document.
func (s *Store) PutSecretResourcePolicy(accountID, nameOrARN, policy string) error {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE secretsmanager_secrets SET resource_policy = ? WHERE account_id = ? AND name = ?`,
		policy, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("put secret resource policy %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("put secret resource policy %s: %w", name, err)
	}
	if affected == 0 {
		return ErrSecretNotFound
	}
	return nil
}

// GetSecretResourcePolicy returns the stored policy or ErrNoSuchResourcePolicy when empty.
func (s *Store) GetSecretResourcePolicy(accountID, nameOrARN string) (string, error) {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return "", err
	}
	row, err := s.getSecretRow(accountID, name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(row.ResourcePolicy) == "" {
		return "", ErrNoSuchResourcePolicy
	}
	return row.ResourcePolicy, nil
}

// DeleteSecretResourcePolicy clears the secret resource policy.
func (s *Store) DeleteSecretResourcePolicy(accountID, nameOrARN string) error {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE secretsmanager_secrets SET resource_policy = '' WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete secret resource policy %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete secret resource policy %s: %w", name, err)
	}
	if affected == 0 {
		return ErrSecretNotFound
	}
	return nil
}
