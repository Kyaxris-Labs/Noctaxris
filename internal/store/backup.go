package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrBackupNotFound   = errors.New("ResourceNotFoundException")
	ErrBackupExists     = errors.New("AlreadyExistsException")
	ErrBackupBadRequest = errors.New("InvalidParameterValueException")
	ErrBackupConflict   = errors.New("InvalidRequestException")
)

const DefaultBackupRegion = "us-east-1"

const backupSchema = `
CREATE TABLE IF NOT EXISTS backup_vaults (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  encryption_key_arn TEXT NOT NULL DEFAULT '',
  number_of_recovery_points INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, name)
);
CREATE TABLE IF NOT EXISTS backup_plans (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  plan_id TEXT NOT NULL,
  plan_arn TEXT NOT NULL,
  plan_name TEXT NOT NULL,
  version_id TEXT NOT NULL,
  rules_json TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, plan_id)
);
CREATE TABLE IF NOT EXISTS backup_jobs (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  job_id TEXT NOT NULL,
  vault_name TEXT NOT NULL,
  resource_arn TEXT NOT NULL,
  iam_role_arn TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL,
  recovery_point_arn TEXT NOT NULL DEFAULT '',
  percent_done TEXT NOT NULL DEFAULT '100.0',
  created_at INTEGER NOT NULL,
  completion_date INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, region, job_id)
);
CREATE TABLE IF NOT EXISTS backup_recovery_points (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  vault_name TEXT NOT NULL,
  recovery_point_arn TEXT NOT NULL,
  resource_arn TEXT NOT NULL,
  resource_type TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'COMPLETED',
  creation_date INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, recovery_point_arn)
);
CREATE TABLE IF NOT EXISTS backup_selections (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  plan_id TEXT NOT NULL,
  selection_id TEXT NOT NULL,
  selection_name TEXT NOT NULL,
  iam_role_arn TEXT NOT NULL DEFAULT '',
  resources_json TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, selection_id)
);
CREATE INDEX IF NOT EXISTS idx_backup_selections_plan
  ON backup_selections (account_id, region, plan_id);
`

// BackupVault is a vault row.
type BackupVault struct {
	BackupVaultName        string
	BackupVaultARN         string
	EncryptionKeyARN       string
	NumberOfRecoveryPoints int
	CreatedAt              int64
	Region                 string
}

// BackupPlan is a plan row.
type BackupPlan struct {
	BackupPlanID   string
	BackupPlanARN  string
	BackupPlanName string
	VersionID      string
	RulesJSON      string
	CreatedAt      int64
	Region         string
}

// BackupJob is a job row (completes immediately with recovery-point metadata).
type BackupJob struct {
	BackupJobID      string
	BackupVaultName  string
	ResourceARN      string
	IamRoleARN       string
	State            string
	RecoveryPointARN string
	PercentDone      string
	CreatedAt        int64
	CompletionDate   int64
	Region           string
}

// BackupRecoveryPoint is recovery-point metadata only (no snapshot engine).
type BackupRecoveryPoint struct {
	RecoveryPointARN string
	BackupVaultName  string
	ResourceARN      string
	ResourceType     string
	Status           string
	CreationDate     int64
	Region           string
}

// BackupSelection assigns resources to a backup plan (stored metadata only).
type BackupSelection struct {
	BackupPlanID  string
	SelectionID   string
	SelectionName string
	IamRoleARN    string
	ResourcesJSON string
	CreatedAt     int64
	Region        string
}

// BackupJobListFilter narrows ListBackupJobs results.
type BackupJobListFilter struct {
	BackupVaultName string
	ResourceARN     string
	ResourceType    string
	State           string
}

// EnsureBackupSchema creates Backup tables if missing.
func EnsureBackupSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure backup schema: db is nil")
	}
	if _, err := db.Exec(backupSchema); err != nil {
		return fmt.Errorf("ensure backup schema: %w", err)
	}
	return nil
}

// EnsureBackupSchema ensures Backup tables on an open store.
func (s *Store) EnsureBackupSchema() error {
	return EnsureBackupSchema(s.db)
}

func BackupVaultARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultBackupRegion
	}
	return fmt.Sprintf("arn:aws:backup:%s:%s:backup-vault:%s", region, accountID, name)
}

func BackupPlanARN(region, accountID, planID string) string {
	if region == "" {
		region = DefaultBackupRegion
	}
	return fmt.Sprintf("arn:aws:backup:%s:%s:backup-plan:%s", region, accountID, planID)
}

func BackupRecoveryPointARN(region, accountID, vault, id string) string {
	if region == "" {
		region = DefaultBackupRegion
	}
	return fmt.Sprintf("arn:aws:backup:%s:%s:recovery-point:%s", region, accountID, id)
}

func backupResourceType(resourceARN string) string {
	switch {
	case strings.Contains(resourceARN, ":s3:"):
		return "S3"
	case strings.Contains(resourceARN, ":dynamodb:"):
		return "DynamoDB"
	default:
		return "Other"
	}
}

// CreateBackupVault creates a vault.
func (s *Store) CreateBackupVault(accountID, region, name, encryptionKeyARN string) (BackupVault, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return BackupVault{}, fmt.Errorf("%w: BackupVaultName required", ErrBackupBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	arn := BackupVaultARN(region, accountID, name)
	_, err := s.db.Exec(
		`INSERT INTO backup_vaults (account_id, region, name, arn, encryption_key_arn, number_of_recovery_points, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?)`,
		accountID, region, name, arn, encryptionKeyARN, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return BackupVault{}, ErrBackupExists
		}
		return BackupVault{}, fmt.Errorf("create backup vault: %w", err)
	}
	return BackupVault{BackupVaultName: name, BackupVaultARN: arn, EncryptionKeyARN: encryptionKeyARN, CreatedAt: now, Region: region}, nil
}

// DescribeBackupVault returns a vault.
func (s *Store) DescribeBackupVault(accountID, region, name string) (BackupVault, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	var v BackupVault
	err := s.db.QueryRow(
		`SELECT name, arn, encryption_key_arn, number_of_recovery_points, created_at, region
		 FROM backup_vaults WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, strings.TrimSpace(name),
	).Scan(&v.BackupVaultName, &v.BackupVaultARN, &v.EncryptionKeyARN, &v.NumberOfRecoveryPoints, &v.CreatedAt, &v.Region)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupVault{}, ErrBackupNotFound
	}
	if err != nil {
		return BackupVault{}, fmt.Errorf("describe backup vault: %w", err)
	}
	return v, nil
}

// ListBackupVaults lists vaults.
func (s *Store) ListBackupVaults(accountID, region string) ([]BackupVault, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	rows, err := s.db.Query(
		`SELECT name, arn, encryption_key_arn, number_of_recovery_points, created_at, region
		 FROM backup_vaults WHERE account_id = ? AND region = ? ORDER BY created_at, name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list backup vaults: %w", err)
	}
	defer rows.Close()
	var out []BackupVault
	for rows.Next() {
		var v BackupVault
		if err := rows.Scan(&v.BackupVaultName, &v.BackupVaultARN, &v.EncryptionKeyARN, &v.NumberOfRecoveryPoints, &v.CreatedAt, &v.Region); err != nil {
			return nil, fmt.Errorf("list backup vaults scan: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeleteBackupVault deletes an empty vault.
func (s *Store) DeleteBackupVault(accountID, region, name string) error {
	if region == "" {
		region = DefaultBackupRegion
	}
	v, err := s.DescribeBackupVault(accountID, region, name)
	if err != nil {
		return err
	}
	if v.NumberOfRecoveryPoints > 0 {
		return fmt.Errorf("%w: vault is not empty", ErrBackupConflict)
	}
	_, err = s.db.Exec(`DELETE FROM backup_vaults WHERE account_id = ? AND region = ? AND name = ?`, accountID, region, strings.TrimSpace(name))
	if err != nil {
		return fmt.Errorf("delete backup vault: %w", err)
	}
	return nil
}

// CreateBackupPlan creates a plan with rules JSON.
func (s *Store) CreateBackupPlan(accountID, region, planName string, rules any) (BackupPlan, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	planName = strings.TrimSpace(planName)
	if planName == "" {
		return BackupPlan{}, fmt.Errorf("%w: BackupPlanName required", ErrBackupBadRequest)
	}
	rulesJSON := "[]"
	if rules != nil {
		raw, err := json.Marshal(rules)
		if err != nil {
			return BackupPlan{}, fmt.Errorf("%w: Rules", ErrBackupBadRequest)
		}
		rulesJSON = string(raw)
	}
	now := time.Now().UTC().UnixMilli()
	planID := uuid.NewString()
	versionID := uuid.NewString()
	arn := BackupPlanARN(region, accountID, planID)
	_, err := s.db.Exec(
		`INSERT INTO backup_plans (account_id, region, plan_id, plan_arn, plan_name, version_id, rules_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, planID, arn, planName, versionID, rulesJSON, now,
	)
	if err != nil {
		return BackupPlan{}, fmt.Errorf("create backup plan: %w", err)
	}
	return BackupPlan{
		BackupPlanID: planID, BackupPlanARN: arn, BackupPlanName: planName,
		VersionID: versionID, RulesJSON: rulesJSON, CreatedAt: now, Region: region,
	}, nil
}

// GetBackupPlan returns a plan by id.
func (s *Store) GetBackupPlan(accountID, region, planID string) (BackupPlan, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	var p BackupPlan
	err := s.db.QueryRow(
		`SELECT plan_id, plan_arn, plan_name, version_id, rules_json, created_at, region
		 FROM backup_plans WHERE account_id = ? AND region = ? AND plan_id = ?`,
		accountID, region, strings.TrimSpace(planID),
	).Scan(&p.BackupPlanID, &p.BackupPlanARN, &p.BackupPlanName, &p.VersionID, &p.RulesJSON, &p.CreatedAt, &p.Region)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupPlan{}, ErrBackupNotFound
	}
	if err != nil {
		return BackupPlan{}, fmt.Errorf("get backup plan: %w", err)
	}
	return p, nil
}

// ListBackupPlans lists plans.
func (s *Store) ListBackupPlans(accountID, region string) ([]BackupPlan, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	rows, err := s.db.Query(
		`SELECT plan_id, plan_arn, plan_name, version_id, rules_json, created_at, region
		 FROM backup_plans WHERE account_id = ? AND region = ? ORDER BY created_at`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list backup plans: %w", err)
	}
	defer rows.Close()
	var out []BackupPlan
	for rows.Next() {
		var p BackupPlan
		if err := rows.Scan(&p.BackupPlanID, &p.BackupPlanARN, &p.BackupPlanName, &p.VersionID, &p.RulesJSON, &p.CreatedAt, &p.Region); err != nil {
			return nil, fmt.Errorf("list backup plans scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteBackupPlan deletes a plan and its selections.
func (s *Store) DeleteBackupPlan(accountID, region, planID string) error {
	planID = strings.TrimSpace(planID)
	if region == "" {
		region = DefaultBackupRegion
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete backup plan: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`DELETE FROM backup_selections WHERE account_id = ? AND region = ? AND plan_id = ?`,
		accountID, region, planID,
	); err != nil {
		return fmt.Errorf("delete backup plan selections: %w", err)
	}
	res, err := tx.Exec(
		`DELETE FROM backup_plans WHERE account_id = ? AND region = ? AND plan_id = ?`,
		accountID, region, planID,
	)
	if err != nil {
		return fmt.Errorf("delete backup plan: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete backup plan: %w", err)
	}
	if n == 0 {
		return ErrBackupNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete backup plan: %w", err)
	}
	return nil
}

// StartBackupJob creates a COMPLETED job and recovery-point metadata for S3/DDB ARN strings.
func (s *Store) StartBackupJob(accountID, region, vaultName, resourceARN, iamRoleARN string) (BackupJob, BackupRecoveryPoint, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	vaultName = strings.TrimSpace(vaultName)
	resourceARN = strings.TrimSpace(resourceARN)
	if vaultName == "" || resourceARN == "" {
		return BackupJob{}, BackupRecoveryPoint{}, fmt.Errorf("%w: BackupVaultName and ResourceArn required", ErrBackupBadRequest)
	}
	if _, err := s.DescribeBackupVault(accountID, region, vaultName); err != nil {
		return BackupJob{}, BackupRecoveryPoint{}, err
	}
	now := time.Now().UTC().UnixMilli()
	jobID := uuid.NewString()
	rpID := uuid.NewString()
	rpARN := BackupRecoveryPointARN(region, accountID, vaultName, rpID)
	resourceType := backupResourceType(resourceARN)
	_, err := s.db.Exec(
		`INSERT INTO backup_recovery_points
		 (account_id, region, vault_name, recovery_point_arn, resource_arn, resource_type, status, creation_date)
		 VALUES (?, ?, ?, ?, ?, ?, 'COMPLETED', ?)`,
		accountID, region, vaultName, rpARN, resourceARN, resourceType, now,
	)
	if err != nil {
		return BackupJob{}, BackupRecoveryPoint{}, fmt.Errorf("create recovery point: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE backup_vaults SET number_of_recovery_points = number_of_recovery_points + 1
		 WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, vaultName,
	)
	if err != nil {
		return BackupJob{}, BackupRecoveryPoint{}, fmt.Errorf("increment vault recovery points: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO backup_jobs
		 (account_id, region, job_id, vault_name, resource_arn, iam_role_arn, state, recovery_point_arn, percent_done, created_at, completion_date)
		 VALUES (?, ?, ?, ?, ?, ?, 'COMPLETED', ?, '100.0', ?, ?)`,
		accountID, region, jobID, vaultName, resourceARN, iamRoleARN, rpARN, now, now,
	)
	if err != nil {
		return BackupJob{}, BackupRecoveryPoint{}, fmt.Errorf("create backup job: %w", err)
	}
	job := BackupJob{
		BackupJobID: jobID, BackupVaultName: vaultName, ResourceARN: resourceARN, IamRoleARN: iamRoleARN,
		State: "COMPLETED", RecoveryPointARN: rpARN, PercentDone: "100.0", CreatedAt: now, CompletionDate: now, Region: region,
	}
	rp := BackupRecoveryPoint{
		RecoveryPointARN: rpARN, BackupVaultName: vaultName, ResourceARN: resourceARN,
		ResourceType: resourceType, Status: "COMPLETED", CreationDate: now, Region: region,
	}
	return job, rp, nil
}

// DescribeBackupJob returns a job.
func (s *Store) DescribeBackupJob(accountID, region, jobID string) (BackupJob, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	var j BackupJob
	err := s.db.QueryRow(
		`SELECT job_id, vault_name, resource_arn, iam_role_arn, state, recovery_point_arn, percent_done, created_at, completion_date, region
		 FROM backup_jobs WHERE account_id = ? AND region = ? AND job_id = ?`,
		accountID, region, strings.TrimSpace(jobID),
	).Scan(&j.BackupJobID, &j.BackupVaultName, &j.ResourceARN, &j.IamRoleARN, &j.State, &j.RecoveryPointARN,
		&j.PercentDone, &j.CreatedAt, &j.CompletionDate, &j.Region)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupJob{}, ErrBackupNotFound
	}
	if err != nil {
		return BackupJob{}, fmt.Errorf("describe backup job: %w", err)
	}
	return j, nil
}

// DescribeRecoveryPoint returns a recovery point by ARN.
func (s *Store) DescribeRecoveryPoint(accountID, region, vaultName, recoveryPointARN string) (BackupRecoveryPoint, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	var rp BackupRecoveryPoint
	err := s.db.QueryRow(
		`SELECT recovery_point_arn, vault_name, resource_arn, resource_type, status, creation_date, region
		 FROM backup_recovery_points WHERE account_id = ? AND region = ? AND vault_name = ? AND recovery_point_arn = ?`,
		accountID, region, strings.TrimSpace(vaultName), strings.TrimSpace(recoveryPointARN),
	).Scan(&rp.RecoveryPointARN, &rp.BackupVaultName, &rp.ResourceARN, &rp.ResourceType, &rp.Status, &rp.CreationDate, &rp.Region)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupRecoveryPoint{}, ErrBackupNotFound
	}
	if err != nil {
		return BackupRecoveryPoint{}, fmt.Errorf("describe recovery point: %w", err)
	}
	return rp, nil
}

// ListRecoveryPointsByBackupVault lists recovery points in a vault.
func (s *Store) ListRecoveryPointsByBackupVault(accountID, region, vaultName string) ([]BackupRecoveryPoint, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	if _, err := s.DescribeBackupVault(accountID, region, vaultName); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT recovery_point_arn, vault_name, resource_arn, resource_type, status, creation_date, region
		 FROM backup_recovery_points WHERE account_id = ? AND region = ? AND vault_name = ? ORDER BY creation_date`,
		accountID, region, strings.TrimSpace(vaultName),
	)
	if err != nil {
		return nil, fmt.Errorf("list recovery points: %w", err)
	}
	defer rows.Close()
	var out []BackupRecoveryPoint
	for rows.Next() {
		var rp BackupRecoveryPoint
		if err := rows.Scan(&rp.RecoveryPointARN, &rp.BackupVaultName, &rp.ResourceARN, &rp.ResourceType, &rp.Status, &rp.CreationDate, &rp.Region); err != nil {
			return nil, fmt.Errorf("list recovery points scan: %w", err)
		}
		out = append(out, rp)
	}
	return out, rows.Err()
}

// ListBackupJobs lists jobs with optional vault/resource/state/type filters.
func (s *Store) ListBackupJobs(accountID, region string, filter BackupJobListFilter) ([]BackupJob, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	rows, err := s.db.Query(
		`SELECT job_id, vault_name, resource_arn, iam_role_arn, state, recovery_point_arn, percent_done, created_at, completion_date, region
		 FROM backup_jobs WHERE account_id = ? AND region = ? ORDER BY created_at, job_id`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list backup jobs: %w", err)
	}
	defer rows.Close()
	vaultFilter := strings.TrimSpace(filter.BackupVaultName)
	resourceFilter := strings.TrimSpace(filter.ResourceARN)
	stateFilter := strings.TrimSpace(filter.State)
	typeFilter := strings.TrimSpace(filter.ResourceType)
	var out []BackupJob
	for rows.Next() {
		var j BackupJob
		if err := rows.Scan(&j.BackupJobID, &j.BackupVaultName, &j.ResourceARN, &j.IamRoleARN, &j.State, &j.RecoveryPointARN,
			&j.PercentDone, &j.CreatedAt, &j.CompletionDate, &j.Region); err != nil {
			return nil, fmt.Errorf("list backup jobs scan: %w", err)
		}
		if vaultFilter != "" && j.BackupVaultName != vaultFilter {
			continue
		}
		if resourceFilter != "" && j.ResourceARN != resourceFilter {
			continue
		}
		if stateFilter != "" && !strings.EqualFold(j.State, stateFilter) {
			continue
		}
		if typeFilter != "" && !strings.EqualFold(backupResourceType(j.ResourceARN), typeFilter) {
			continue
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// StopBackupJob aborts a non-terminal job. COMPLETED/ABORTED/FAILED is a no-op.
func (s *Store) StopBackupJob(accountID, region, jobID string) error {
	job, err := s.DescribeBackupJob(accountID, region, jobID)
	if err != nil {
		return err
	}
	switch strings.ToUpper(job.State) {
	case "COMPLETED", "ABORTED", "FAILED", "EXPIRED":
		return nil
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`UPDATE backup_jobs SET state = 'ABORTED', percent_done = '100.0', completion_date = ?
		 WHERE account_id = ? AND region = ? AND job_id = ?`,
		now, accountID, region, strings.TrimSpace(jobID),
	)
	if err != nil {
		return fmt.Errorf("stop backup job: %w", err)
	}
	return nil
}

// DeleteRecoveryPoint removes recovery-point metadata and decrements the vault count.
func (s *Store) DeleteRecoveryPoint(accountID, region, vaultName, recoveryPointARN string) error {
	if region == "" {
		region = DefaultBackupRegion
	}
	vaultName = strings.TrimSpace(vaultName)
	recoveryPointARN = strings.TrimSpace(recoveryPointARN)
	if vaultName == "" || recoveryPointARN == "" {
		return fmt.Errorf("%w: BackupVaultName and RecoveryPointArn required", ErrBackupBadRequest)
	}
	if _, err := s.DescribeRecoveryPoint(accountID, region, vaultName, recoveryPointARN); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete recovery point: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		`DELETE FROM backup_recovery_points WHERE account_id = ? AND region = ? AND vault_name = ? AND recovery_point_arn = ?`,
		accountID, region, vaultName, recoveryPointARN,
	)
	if err != nil {
		return fmt.Errorf("delete recovery point: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete recovery point: %w", err)
	}
	if n == 0 {
		return ErrBackupNotFound
	}
	if _, err := tx.Exec(
		`UPDATE backup_vaults SET number_of_recovery_points = CASE
			WHEN number_of_recovery_points > 0 THEN number_of_recovery_points - 1 ELSE 0 END
		 WHERE account_id = ? AND region = ? AND name = ?`,
		accountID, region, vaultName,
	); err != nil {
		return fmt.Errorf("decrement vault recovery points: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete recovery point: %w", err)
	}
	return nil
}

func backupResourcesJSON(resources []string) (string, error) {
	if resources == nil {
		resources = []string{}
	}
	raw, err := json.Marshal(resources)
	if err != nil {
		return "", fmt.Errorf("%w: Resources", ErrBackupBadRequest)
	}
	return string(raw), nil
}

// CreateBackupSelection stores a selection under a plan (IamRoleArn + Resources).
func (s *Store) CreateBackupSelection(accountID, region, planID, selectionName, iamRoleARN string, resources []string) (BackupSelection, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	planID = strings.TrimSpace(planID)
	selectionName = strings.TrimSpace(selectionName)
	iamRoleARN = strings.TrimSpace(iamRoleARN)
	if planID == "" || selectionName == "" || iamRoleARN == "" {
		return BackupSelection{}, fmt.Errorf("%w: BackupPlanId, SelectionName, and IamRoleArn required", ErrBackupBadRequest)
	}
	if _, err := s.GetBackupPlan(accountID, region, planID); err != nil {
		return BackupSelection{}, err
	}
	resourcesJSON, err := backupResourcesJSON(resources)
	if err != nil {
		return BackupSelection{}, err
	}
	now := time.Now().UTC().UnixMilli()
	selectionID := uuid.NewString()
	_, err = s.db.Exec(
		`INSERT INTO backup_selections
		 (account_id, region, plan_id, selection_id, selection_name, iam_role_arn, resources_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, planID, selectionID, selectionName, iamRoleARN, resourcesJSON, now,
	)
	if err != nil {
		return BackupSelection{}, fmt.Errorf("create backup selection: %w", err)
	}
	return BackupSelection{
		BackupPlanID: planID, SelectionID: selectionID, SelectionName: selectionName,
		IamRoleARN: iamRoleARN, ResourcesJSON: resourcesJSON, CreatedAt: now, Region: region,
	}, nil
}

// GetBackupSelection returns a selection by plan and selection id.
func (s *Store) GetBackupSelection(accountID, region, planID, selectionID string) (BackupSelection, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	var sel BackupSelection
	err := s.db.QueryRow(
		`SELECT plan_id, selection_id, selection_name, iam_role_arn, resources_json, created_at, region
		 FROM backup_selections WHERE account_id = ? AND region = ? AND plan_id = ? AND selection_id = ?`,
		accountID, region, strings.TrimSpace(planID), strings.TrimSpace(selectionID),
	).Scan(&sel.BackupPlanID, &sel.SelectionID, &sel.SelectionName, &sel.IamRoleARN, &sel.ResourcesJSON, &sel.CreatedAt, &sel.Region)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupSelection{}, ErrBackupNotFound
	}
	if err != nil {
		return BackupSelection{}, fmt.Errorf("get backup selection: %w", err)
	}
	return sel, nil
}

// ListBackupSelections lists selections for a plan.
func (s *Store) ListBackupSelections(accountID, region, planID string) ([]BackupSelection, error) {
	if region == "" {
		region = DefaultBackupRegion
	}
	planID = strings.TrimSpace(planID)
	if _, err := s.GetBackupPlan(accountID, region, planID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT plan_id, selection_id, selection_name, iam_role_arn, resources_json, created_at, region
		 FROM backup_selections WHERE account_id = ? AND region = ? AND plan_id = ? ORDER BY created_at, selection_id`,
		accountID, region, planID,
	)
	if err != nil {
		return nil, fmt.Errorf("list backup selections: %w", err)
	}
	defer rows.Close()
	var out []BackupSelection
	for rows.Next() {
		var sel BackupSelection
		if err := rows.Scan(&sel.BackupPlanID, &sel.SelectionID, &sel.SelectionName, &sel.IamRoleARN, &sel.ResourcesJSON, &sel.CreatedAt, &sel.Region); err != nil {
			return nil, fmt.Errorf("list backup selections scan: %w", err)
		}
		out = append(out, sel)
	}
	return out, rows.Err()
}

// DeleteBackupSelection deletes a selection.
func (s *Store) DeleteBackupSelection(accountID, region, planID, selectionID string) error {
	if region == "" {
		region = DefaultBackupRegion
	}
	res, err := s.db.Exec(
		`DELETE FROM backup_selections WHERE account_id = ? AND region = ? AND plan_id = ? AND selection_id = ?`,
		accountID, region, strings.TrimSpace(planID), strings.TrimSpace(selectionID),
	)
	if err != nil {
		return fmt.Errorf("delete backup selection: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete backup selection: %w", err)
	}
	if n == 0 {
		return ErrBackupNotFound
	}
	return nil
}
