package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// SecretVersionStageCurrent is the live secret version label.
	SecretVersionStageCurrent = "AWSCURRENT"
	// SecretVersionStagePending is the in-rotation version label.
	SecretVersionStagePending = "AWSPENDING"
	// SecretVersionStagePrevious is the prior AWSCURRENT after a successful rotate/put.
	SecretVersionStagePrevious = "AWSPREVIOUS"
)

// ErrSecretRotationInProgress is returned when AWSPENDING is on a non-current version.
var ErrSecretRotationInProgress = fmt.Errorf("InvalidRequestException: A previous rotation is still in progress")

// SecretVersion is one staged version of a secret.
type SecretVersion struct {
	VersionID    string
	Version      int
	Stages       []string
	SecretString string
	SecretBinary []byte
	KmsKeyID     string
	CreatedDate  string
}

const secretsVersionsSchema = `
CREATE TABLE IF NOT EXISTS secretsmanager_secret_versions (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  version_id TEXT NOT NULL,
  version_num INTEGER NOT NULL DEFAULT 0,
  secret_string_plain TEXT NOT NULL DEFAULT '',
  secret_string_sealed BLOB,
  string_sealed INTEGER NOT NULL DEFAULT 0,
  secret_binary_plain BLOB,
  secret_binary_sealed BLOB,
  binary_sealed INTEGER NOT NULL DEFAULT 0,
  kms_key_id TEXT NOT NULL DEFAULT '',
  stages_json TEXT NOT NULL DEFAULT '[]',
  created_date TEXT NOT NULL,
  PRIMARY KEY (account_id, name, version_id)
);
`

// EnsureSecretsVersionsSchema creates the versions table and backfills AWSCURRENT rows.
func EnsureSecretsVersionsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure secrets versions schema: db is nil")
	}
	if _, err := db.Exec(secretsVersionsSchema); err != nil {
		return fmt.Errorf("ensure secrets versions schema: %w", err)
	}
	return backfillSecretVersions(db)
}

func (s *Store) EnsureSecretsVersionsSchema() error {
	return EnsureSecretsVersionsSchema(s.db)
}

func backfillSecretVersions(db *sql.DB) error {
	rows, err := db.Query(
		`SELECT account_id, name, version,
		        secret_string_plain, secret_string_sealed, string_sealed,
		        secret_binary_plain, secret_binary_sealed, binary_sealed,
		        kms_key_id, created_date
		 FROM secretsmanager_secrets s
		 WHERE NOT EXISTS (
		   SELECT 1 FROM secretsmanager_secret_versions v
		   WHERE v.account_id = s.account_id AND v.name = s.name
		 )`,
	)
	if err != nil {
		return fmt.Errorf("backfill secret versions: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			accountID, name, created  string
			version                   int
			stringPlain               string
			stringSealed              []byte
			stringSeal                int
			binaryPlain, binarySealed []byte
			binarySeal                int
			kmsKeyID                  string
		)
		if err := rows.Scan(
			&accountID, &name, &version, &stringPlain, &stringSealed, &stringSeal,
			&binaryPlain, &binarySealed, &binarySeal, &kmsKeyID, &created,
		); err != nil {
			return fmt.Errorf("backfill secret versions: scan: %w", err)
		}
		if version <= 0 {
			version = 1
		}
		versionID := fmt.Sprintf("%d", version)
		stages, err := json.Marshal([]string{SecretVersionStageCurrent})
		if err != nil {
			return err
		}
		_, err = db.Exec(
			`INSERT INTO secretsmanager_secret_versions
			 (account_id, name, version_id, version_num, secret_string_plain, secret_string_sealed, string_sealed,
			  secret_binary_plain, secret_binary_sealed, binary_sealed, kms_key_id, stages_json, created_date)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			accountID, name, versionID, version, stringPlain, stringSealed, stringSeal,
			binaryPlain, binarySealed, binarySeal, kmsKeyID, string(stages), created,
		)
		if err != nil {
			return fmt.Errorf("backfill secret versions: insert %s/%s: %w", accountID, name, err)
		}
	}
	return rows.Err()
}

func encodeSecretStages(stages []string) (string, error) {
	if stages == nil {
		stages = []string{}
	}
	raw, err := json.Marshal(stages)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeSecretStages(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}
	var stages []string
	if err := json.Unmarshal([]byte(raw), &stages); err != nil {
		return nil, fmt.Errorf("decode secret stages: %w", err)
	}
	if stages == nil {
		stages = []string{}
	}
	return stages, nil
}

func stagesContain(stages []string, stage string) bool {
	for _, s := range stages {
		if s == stage {
			return true
		}
	}
	return false
}

func removeStage(stages []string, stage string) []string {
	out := make([]string, 0, len(stages))
	for _, s := range stages {
		if s != stage {
			out = append(out, s)
		}
	}
	return out
}

func addStage(stages []string, stage string) []string {
	if stagesContain(stages, stage) {
		return stages
	}
	return append(append([]string{}, stages...), stage)
}

type secretVersionRow struct {
	VersionID          string
	VersionNum         int
	SecretStringPlain  string
	SecretStringSealed []byte
	StringSealed       bool
	SecretBinaryPlain  []byte
	SecretBinarySealed []byte
	BinarySealed       bool
	KMSKeyID           string
	Stages             []string
	CreatedDate        string
}

func (s *Store) listSecretVersionRows(accountID, name string) ([]secretVersionRow, error) {
	rows, err := s.db.Query(
		`SELECT version_id, version_num, secret_string_plain, secret_string_sealed, string_sealed,
		        secret_binary_plain, secret_binary_sealed, binary_sealed, kms_key_id, stages_json, created_date
		 FROM secretsmanager_secret_versions
		 WHERE account_id = ? AND name = ?
		 ORDER BY version_num ASC, version_id ASC`,
		accountID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("list secret versions: %w", err)
	}
	defer rows.Close()

	var out []secretVersionRow
	for rows.Next() {
		var (
			row          secretVersionRow
			stringSeal   int
			binarySeal   int
			stringSealed []byte
			binaryPlain  []byte
			binarySealed []byte
			stagesJSON   string
		)
		if err := rows.Scan(
			&row.VersionID, &row.VersionNum, &row.SecretStringPlain, &stringSealed, &stringSeal,
			&binaryPlain, &binarySealed, &binarySeal, &row.KMSKeyID, &stagesJSON, &row.CreatedDate,
		); err != nil {
			return nil, fmt.Errorf("list secret versions: scan: %w", err)
		}
		stages, err := decodeSecretStages(stagesJSON)
		if err != nil {
			return nil, err
		}
		row.StringSealed = stringSeal == 1
		row.BinarySealed = binarySeal == 1
		row.SecretStringSealed = stringSealed
		row.SecretBinaryPlain = binaryPlain
		row.SecretBinarySealed = binarySealed
		row.Stages = stages
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) getSecretVersionRow(accountID, name, versionID string) (secretVersionRow, error) {
	var (
		row          secretVersionRow
		stringSeal   int
		binarySeal   int
		stringSealed []byte
		binaryPlain  []byte
		binarySealed []byte
		stagesJSON   string
	)
	err := s.db.QueryRow(
		`SELECT version_id, version_num, secret_string_plain, secret_string_sealed, string_sealed,
		        secret_binary_plain, secret_binary_sealed, binary_sealed, kms_key_id, stages_json, created_date
		 FROM secretsmanager_secret_versions
		 WHERE account_id = ? AND name = ? AND version_id = ?`,
		accountID, name, versionID,
	).Scan(
		&row.VersionID, &row.VersionNum, &row.SecretStringPlain, &stringSealed, &stringSeal,
		&binaryPlain, &binarySealed, &binarySeal, &row.KMSKeyID, &stagesJSON, &row.CreatedDate,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return secretVersionRow{}, ErrSecretNotFound
		}
		return secretVersionRow{}, fmt.Errorf("get secret version: %w", err)
	}
	stages, err := decodeSecretStages(stagesJSON)
	if err != nil {
		return secretVersionRow{}, err
	}
	row.StringSealed = stringSeal == 1
	row.BinarySealed = binarySeal == 1
	row.SecretStringSealed = stringSealed
	row.SecretBinaryPlain = binaryPlain
	row.SecretBinarySealed = binarySealed
	row.Stages = stages
	return row, nil
}

func (s *Store) findSecretVersionByStage(accountID, name, stage string) (secretVersionRow, error) {
	rows, err := s.listSecretVersionRows(accountID, name)
	if err != nil {
		return secretVersionRow{}, err
	}
	for _, row := range rows {
		if stagesContain(row.Stages, stage) {
			return row, nil
		}
	}
	return secretVersionRow{}, ErrSecretNotFound
}

func (s *Store) setSecretVersionStages(accountID, name, versionID string, stages []string) error {
	raw, err := encodeSecretStages(stages)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE secretsmanager_secret_versions SET stages_json = ?
		 WHERE account_id = ? AND name = ? AND version_id = ?`,
		raw, accountID, name, versionID,
	)
	if err != nil {
		return fmt.Errorf("set secret version stages: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSecretNotFound
	}
	return nil
}

func (s *Store) deleteSecretVersions(accountID, name string) error {
	_, err := s.db.Exec(
		`DELETE FROM secretsmanager_secret_versions WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete secret versions: %w", err)
	}
	return nil
}

func (s *Store) secretFromVersionRow(parent secretRow, vrow secretVersionRow, withValues bool) (Secret, error) {
	out := Secret{
		Name:              parent.Name,
		ARN:               parent.ARN,
		Version:           vrow.VersionNum,
		VersionID:         vrow.VersionID,
		KmsKeyID:          vrow.KMSKeyID,
		Description:       parent.Description,
		ResourcePolicy:    parent.ResourcePolicy,
		CreatedDate:       parent.CreatedDate,
		LastChangedDate:   parent.LastChangedDate,
		DeletedDate:       parent.DeletedDate,
		DeletionDate:      parent.DeletionDate,
		RotationLambdaARN: parent.RotationLambdaARN,
		RotationRoleARN:   parent.RotationRoleARN,
		VersionStages:     append([]string{}, vrow.Stages...),
	}
	if !withValues {
		return out, nil
	}
	encCtx := SecretsEncryptionContext(parent.ARN, vrow.VersionID)
	if vrow.StringSealed {
		if vrow.KMSKeyID == "" {
			return Secret{}, fmt.Errorf("secret %s: sealed string without kms key id", parent.Name)
		}
		k, err := s.GetKey(vrow.KMSKeyID)
		if err != nil {
			return Secret{}, fmt.Errorf("secret %s: key: %w", parent.Name, err)
		}
		if !KeyUsableForCrypto(k.KeyState) {
			return Secret{}, fmt.Errorf("secret %s: %w", parent.Name, ErrInvalidKeyState)
		}
		plain, err := s.DecryptBlobWithKeyContext(vrow.KMSKeyID, vrow.SecretStringSealed, encCtx)
		if err != nil {
			return Secret{}, fmt.Errorf("secret %s: decrypt string: %w", parent.Name, err)
		}
		out.SecretString = string(plain)
	} else {
		out.SecretString = vrow.SecretStringPlain
	}
	if vrow.BinarySealed {
		if vrow.KMSKeyID == "" {
			return Secret{}, fmt.Errorf("secret %s: sealed binary without kms key id", parent.Name)
		}
		k, err := s.GetKey(vrow.KMSKeyID)
		if err != nil {
			return Secret{}, fmt.Errorf("secret %s: key: %w", parent.Name, err)
		}
		if !KeyUsableForCrypto(k.KeyState) {
			return Secret{}, fmt.Errorf("secret %s: %w", parent.Name, ErrInvalidKeyState)
		}
		plain, err := s.DecryptBlobWithKeyContext(vrow.KMSKeyID, vrow.SecretBinarySealed, encCtx)
		if err != nil {
			return Secret{}, fmt.Errorf("secret %s: decrypt binary: %w", parent.Name, err)
		}
		out.SecretBinary = plain
	} else if len(vrow.SecretBinaryPlain) > 0 {
		out.SecretBinary = append([]byte(nil), vrow.SecretBinaryPlain...)
	}
	return out, nil
}

// VersionIdsToStages returns the DescribeSecret VersionIdsToStages map.
func (s *Store) VersionIdsToStages(accountID, nameOrARN string) (map[string][]string, error) {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSecretVersionsBackfilled(accountID, name); err != nil {
		return nil, err
	}
	rows, err := s.listSecretVersionRows(accountID, name)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(rows))
	for _, row := range rows {
		if len(row.Stages) == 0 {
			continue
		}
		out[row.VersionID] = append([]string{}, row.Stages...)
	}
	return out, nil
}

func (s *Store) ensureSecretVersionsBackfilled(accountID, name string) error {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM secretsmanager_secret_versions WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("count secret versions: %w", err)
	}
	if n > 0 {
		return nil
	}
	row, err := s.getSecretRow(accountID, name)
	if err != nil {
		return err
	}
	version := row.Version
	if version <= 0 {
		version = 1
	}
	return s.insertSecretVersionRow(accountID, name, fmt.Sprintf("%d", version), version, row, []string{SecretVersionStageCurrent}, row.CreatedDate)
}

func (s *Store) insertSecretVersionRow(
	accountID, name, versionID string, versionNum int, row secretRow, stages []string, created string,
) error {
	if strings.TrimSpace(created) == "" {
		created = nowRFC3339()
	}
	stagesJSON, err := encodeSecretStages(stages)
	if err != nil {
		return err
	}
	stringSeal := 0
	if row.StringSealed {
		stringSeal = 1
	}
	binarySeal := 0
	if row.BinarySealed {
		binarySeal = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO secretsmanager_secret_versions
		 (account_id, name, version_id, version_num, secret_string_plain, secret_string_sealed, string_sealed,
		  secret_binary_plain, secret_binary_sealed, binary_sealed, kms_key_id, stages_json, created_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, versionID, versionNum, row.SecretStringPlain, row.SecretStringSealed, stringSeal,
		row.SecretBinaryPlain, row.SecretBinarySealed, binarySeal, row.KMSKeyID, stagesJSON, created,
	)
	if err != nil {
		return fmt.Errorf("insert secret version: %w", err)
	}
	return nil
}

// HasPendingRotationOrphan reports AWSPENDING on a version that is not also AWSCURRENT.
func (s *Store) HasPendingRotationOrphan(accountID, nameOrARN string) (bool, error) {
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return false, err
	}
	if err := s.ensureSecretVersionsBackfilled(accountID, name); err != nil {
		return false, err
	}
	rows, err := s.listSecretVersionRows(accountID, name)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if stagesContain(row.Stages, SecretVersionStagePending) && !stagesContain(row.Stages, SecretVersionStageCurrent) {
			return true, nil
		}
	}
	return false, nil
}

// GetSecretValueByStage returns the secret value for VersionStage (default AWSCURRENT).
func (s *Store) GetSecretValueByStage(accountID, nameOrARN, versionStage, versionID string) (Secret, error) {
	_, _ = s.SweepExpiredSecrets(time.Time{})
	name, err := s.resolveSecretName(accountID, nameOrARN)
	if err != nil {
		return Secret{}, err
	}
	parent, err := s.getSecretRow(accountID, name)
	if err != nil {
		return Secret{}, err
	}
	if strings.TrimSpace(parent.DeletionDate) != "" {
		return Secret{}, ErrSecretScheduledDeletion
	}
	if err := s.ensureSecretVersionsBackfilled(accountID, name); err != nil {
		return Secret{}, err
	}

	var vrow secretVersionRow
	switch {
	case strings.TrimSpace(versionID) != "":
		vrow, err = s.getSecretVersionRow(accountID, name, strings.TrimSpace(versionID))
	default:
		stage := strings.TrimSpace(versionStage)
		if stage == "" {
			stage = SecretVersionStageCurrent
		}
		vrow, err = s.findSecretVersionByStage(accountID, name, stage)
	}
	if err != nil {
		return Secret{}, err
	}
	sec, err := s.secretFromVersionRow(parent, vrow, true)
	if err != nil {
		return Secret{}, err
	}
	stagesMap, err := s.VersionIdsToStages(accountID, name)
	if err != nil {
		return Secret{}, err
	}
	sec.VersionIdsToStages = stagesMap
	return sec, nil
}

// UpdateSecretVersionStage moves or removes a staging label (AWS-shaped).
// Moving AWSCURRENT also clears AWSPENDING on the destination version and labels the
// previous current version AWSPREVIOUS.
func (s *Store) UpdateSecretVersionStage(accountID, secretID, moveToVersionID, removeFromVersionID, stage string) error {
	name, err := s.resolveSecretName(accountID, secretID)
	if err != nil {
		return err
	}
	parent, err := s.getSecretRow(accountID, name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(parent.DeletionDate) != "" {
		return ErrSecretScheduledDeletion
	}
	if err := s.ensureSecretVersionsBackfilled(accountID, name); err != nil {
		return err
	}
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return fmt.Errorf("ValidationException: VersionStage is required")
	}
	moveTo := strings.TrimSpace(moveToVersionID)
	removeFrom := strings.TrimSpace(removeFromVersionID)
	if moveTo == "" && removeFrom == "" {
		return fmt.Errorf("ValidationException: MoveToVersionId or RemoveFromVersionId is required")
	}

	if removeFrom != "" {
		row, err := s.getSecretVersionRow(accountID, name, removeFrom)
		if err != nil {
			return err
		}
		if err := s.setSecretVersionStages(accountID, name, removeFrom, removeStage(row.Stages, stage)); err != nil {
			return err
		}
	}
	if moveTo != "" {
		row, err := s.getSecretVersionRow(accountID, name, moveTo)
		if err != nil {
			return err
		}
		stages := addStage(row.Stages, stage)
		if stage == SecretVersionStageCurrent {
			stages = removeStage(stages, SecretVersionStagePending)
		}
		if err := s.setSecretVersionStages(accountID, name, moveTo, stages); err != nil {
			return err
		}
	}

	if stage == SecretVersionStageCurrent && moveTo != "" {
		if err := s.labelPreviousCurrent(accountID, name, moveTo, removeFrom); err != nil {
			return err
		}
		if err := s.syncParentFromVersion(accountID, name, moveTo); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) labelPreviousCurrent(accountID, name, newCurrentID, removeFromID string) error {
	rows, err := s.listSecretVersionRows(accountID, name)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.VersionID == newCurrentID {
			continue
		}
		if !stagesContain(row.Stages, SecretVersionStageCurrent) && row.VersionID != removeFromID {
			continue
		}
		stages := removeStage(row.Stages, SecretVersionStageCurrent)
		if row.VersionID == removeFromID || stagesContain(row.Stages, SecretVersionStageCurrent) {
			stages = addStage(removeStage(stages, SecretVersionStagePending), SecretVersionStagePrevious)
			// Only one AWSPREVIOUS: strip from others below.
		}
		if err := s.setSecretVersionStages(accountID, name, row.VersionID, stages); err != nil {
			return err
		}
	}
	// Ensure a single AWSPREVIOUS (the removeFrom / former current).
	if removeFromID != "" {
		rows, err = s.listSecretVersionRows(accountID, name)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.VersionID == removeFromID || row.VersionID == newCurrentID {
				continue
			}
			if stagesContain(row.Stages, SecretVersionStagePrevious) {
				if err := s.setSecretVersionStages(accountID, name, row.VersionID, removeStage(row.Stages, SecretVersionStagePrevious)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Store) syncParentFromVersion(accountID, name, versionID string) error {
	vrow, err := s.getSecretVersionRow(accountID, name, versionID)
	if err != nil {
		return err
	}
	stringSeal := 0
	if vrow.StringSealed {
		stringSeal = 1
	}
	binarySeal := 0
	if vrow.BinarySealed {
		binarySeal = 1
	}
	modified := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE secretsmanager_secrets
		 SET secret_string_plain = ?, secret_string_sealed = ?, string_sealed = ?,
		     secret_binary_plain = ?, secret_binary_sealed = ?, binary_sealed = ?,
		     kms_key_id = ?, version = ?, last_changed_date = ?
		 WHERE account_id = ? AND name = ?`,
		vrow.SecretStringPlain, vrow.SecretStringSealed, stringSeal,
		vrow.SecretBinaryPlain, vrow.SecretBinarySealed, binarySeal,
		vrow.KMSKeyID, vrow.VersionNum, modified, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("sync parent from version: %w", err)
	}
	return nil
}

// createAWSPENDINGVersion creates a new version labeled AWSPENDING only.
func (s *Store) createAWSPENDINGVersion(accountID, name, versionID, secretString string, secretBinary []byte) (secretVersionRow, error) {
	parent, err := s.getSecretRow(accountID, name)
	if err != nil {
		return secretVersionRow{}, err
	}
	if _, err := s.getSecretVersionRow(accountID, name, versionID); err == nil {
		return secretVersionRow{}, fmt.Errorf("ResourceExistsException: version %s already exists", versionID)
	} else if !errors.Is(err, ErrSecretNotFound) {
		return secretVersionRow{}, err
	}
	versionNum := parent.Version + 1
	encCtx := SecretsEncryptionContext(parent.ARN, versionID)
	stringPlain, stringSealed, stringSealFlag, binaryPlain, binarySealed, binarySealFlag, kmsKeyID, err := s.storeSecretValues(
		accountID, parent.KMSKeyID, secretString, secretBinary, encCtx,
	)
	if err != nil {
		return secretVersionRow{}, fmt.Errorf("create AWSPENDING version: %w", err)
	}
	row := secretRow{
		SecretStringPlain:  stringPlain,
		SecretStringSealed: stringSealed,
		StringSealed:       stringSealFlag == 1,
		SecretBinaryPlain:  binaryPlain,
		SecretBinarySealed: binarySealed,
		BinarySealed:       binarySealFlag == 1,
		KMSKeyID:           kmsKeyID,
	}
	created := nowRFC3339()
	if err := s.insertSecretVersionRow(accountID, name, versionID, versionNum, row, []string{SecretVersionStagePending}, created); err != nil {
		return secretVersionRow{}, err
	}
	// Bump parent version counter without changing current value.
	_, err = s.db.Exec(
		`UPDATE secretsmanager_secrets SET version = ? WHERE account_id = ? AND name = ? AND version < ?`,
		versionNum, accountID, name, versionNum,
	)
	if err != nil {
		return secretVersionRow{}, fmt.Errorf("bump secret version counter: %w", err)
	}
	return s.getSecretVersionRow(accountID, name, versionID)
}

// promoteSecretVersionCurrent moves AWSCURRENT onto versionID (finishSecret).
func (s *Store) promoteSecretVersionCurrent(accountID, name, versionID string) error {
	current, err := s.findSecretVersionByStage(accountID, name, SecretVersionStageCurrent)
	removeFrom := ""
	if err == nil {
		removeFrom = current.VersionID
	} else if !errors.Is(err, ErrSecretNotFound) {
		return err
	}
	return s.UpdateSecretVersionStage(accountID, name, versionID, removeFrom, SecretVersionStageCurrent)
}

// recordPutSecretVersion records a new version after PutSecretValue / random rotate.
// Default stages promote AWSCURRENT (former current becomes AWSPREVIOUS).
func (s *Store) recordPutSecretVersion(accountID, name string, parent secretRow, versionID string, versionNum int, stages []string) error {
	if err := s.ensureSecretVersionsBackfilled(accountID, name); err != nil {
		return err
	}
	if len(stages) == 0 {
		stages = []string{SecretVersionStageCurrent}
	}
	if stagesContain(stages, SecretVersionStageCurrent) {
		rows, err := s.listSecretVersionRows(accountID, name)
		if err != nil {
			return err
		}
		var formerCurrentID string
		for _, row := range rows {
			if stagesContain(row.Stages, SecretVersionStageCurrent) {
				formerCurrentID = row.VersionID
				break
			}
		}
		for _, row := range rows {
			next := removeStage(row.Stages, SecretVersionStageCurrent)
			next = removeStage(next, SecretVersionStagePrevious)
			if row.VersionID == formerCurrentID {
				next = addStage(next, SecretVersionStagePrevious)
			}
			if err := s.setSecretVersionStages(accountID, name, row.VersionID, next); err != nil {
				return err
			}
		}
	}
	if _, err := s.getSecretVersionRow(accountID, name, versionID); err == nil {
		return s.setSecretVersionStages(accountID, name, versionID, stages)
	} else if !errors.Is(err, ErrSecretNotFound) {
		return err
	}
	return s.insertSecretVersionRow(accountID, name, versionID, versionNum, parent, stages, nowRFC3339())
}
