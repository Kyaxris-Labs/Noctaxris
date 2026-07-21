package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

const maxManagedPolicyVersions = 5

var (
	// ErrPolicyVersionLimit is returned when a managed policy already has five versions.
	ErrPolicyVersionLimit = errors.New("LimitExceeded: a managed policy can have up to five versions")
	// ErrDeleteDefaultPolicyVersion is returned when deleting the default version.
	ErrDeleteDefaultPolicyVersion = errors.New("DeleteConflict: cannot delete the default version of a policy")
	// ErrNoSuchPolicyVersion is returned when a version id is missing.
	ErrNoSuchPolicyVersion = errors.New("NoSuchEntity: policy version not found")
)

// ManagedPolicyVersion is one version of a customer-managed IAM policy.
type ManagedPolicyVersion struct {
	PolicyARN        string
	VersionID        string
	Document         string
	IsDefaultVersion bool
	CreateDate       string
}

const managedPolicyVersionsSchema = `
CREATE TABLE IF NOT EXISTS managed_policy_versions (
  policy_arn TEXT NOT NULL,
  version_id TEXT NOT NULL,
  document TEXT NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  create_date TEXT NOT NULL,
  PRIMARY KEY (policy_arn, version_id)
);
`

// EnsureManagedPolicyVersionsSchema creates the versions table and backfills v1 rows
// for policies created before versioning existed.
func EnsureManagedPolicyVersionsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure managed policy versions schema: db is nil")
	}
	if _, err := db.Exec(managedPolicyVersionsSchema); err != nil {
		return fmt.Errorf("ensure managed policy versions schema: %w", err)
	}
	_, err := db.Exec(`
		INSERT OR IGNORE INTO managed_policy_versions
		 (policy_arn, version_id, document, is_default, create_date)
		SELECT policy_arn, default_version_id, document, 1, ?
		FROM managed_policies`, nowRFC3339())
	if err != nil {
		return fmt.Errorf("ensure managed policy versions schema: backfill: %w", err)
	}
	return nil
}

// CreateManagedPolicyVersion adds a new version (vN). Optional setAsDefault updates
// the operative document used by Evaluate (managed_policies + policies sync).
func (s *Store) CreateManagedPolicyVersion(policyARN, document string, setAsDefault bool) (ManagedPolicyVersion, error) {
	if err := validate.PolicyDocument(document); err != nil {
		return ManagedPolicyVersion{}, fmt.Errorf("create policy version: %w", err)
	}
	p, err := s.GetManagedPolicy(policyARN)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedPolicyVersion{}, sql.ErrNoRows
		}
		return ManagedPolicyVersion{}, fmt.Errorf("create policy version: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return ManagedPolicyVersion{}, err
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM managed_policy_versions WHERE policy_arn = ?`,
		policyARN,
	).Scan(&count); err != nil {
		return ManagedPolicyVersion{}, fmt.Errorf("create policy version: count: %w", err)
	}
	if count >= maxManagedPolicyVersions {
		return ManagedPolicyVersion{}, ErrPolicyVersionLimit
	}

	nextID, err := nextPolicyVersionID(tx, policyARN)
	if err != nil {
		return ManagedPolicyVersion{}, err
	}
	created := nowRFC3339()
	isDefault := 0
	if setAsDefault {
		isDefault = 1
		if _, err := tx.Exec(
			`UPDATE managed_policy_versions SET is_default = 0 WHERE policy_arn = ?`,
			policyARN,
		); err != nil {
			return ManagedPolicyVersion{}, fmt.Errorf("create policy version: clear default: %w", err)
		}
		if err := syncManagedPolicyDefault(tx, p, nextID, document); err != nil {
			return ManagedPolicyVersion{}, err
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO managed_policy_versions
		 (policy_arn, version_id, document, is_default, create_date)
		 VALUES (?, ?, ?, ?, ?)`,
		policyARN, nextID, document, isDefault, created,
	); err != nil {
		return ManagedPolicyVersion{}, fmt.Errorf("create policy version: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ManagedPolicyVersion{}, err
	}
	return ManagedPolicyVersion{
		PolicyARN:        policyARN,
		VersionID:        nextID,
		Document:         document,
		IsDefaultVersion: setAsDefault,
		CreateDate:       created,
	}, nil
}

// SetDefaultManagedPolicyVersion points the operative policy document at versionID.
func (s *Store) SetDefaultManagedPolicyVersion(policyARN, versionID string) error {
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		return fmt.Errorf("set default policy version: VersionId is required")
	}
	p, err := s.GetManagedPolicy(policyARN)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var document string
	err = tx.QueryRow(
		`SELECT document FROM managed_policy_versions WHERE policy_arn = ? AND version_id = ?`,
		policyARN, versionID,
	).Scan(&document)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoSuchPolicyVersion
	}
	if err != nil {
		return fmt.Errorf("set default policy version: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE managed_policy_versions SET is_default = 0 WHERE policy_arn = ?`,
		policyARN,
	); err != nil {
		return fmt.Errorf("set default policy version: clear default: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE managed_policy_versions SET is_default = 1 WHERE policy_arn = ? AND version_id = ?`,
		policyARN, versionID,
	); err != nil {
		return fmt.Errorf("set default policy version: mark default: %w", err)
	}
	if err := syncManagedPolicyDefault(tx, p, versionID, document); err != nil {
		return err
	}
	return tx.Commit()
}

// GetManagedPolicyVersion returns one version including its document.
func (s *Store) GetManagedPolicyVersion(policyARN, versionID string) (ManagedPolicyVersion, error) {
	var v ManagedPolicyVersion
	var isDefault int
	err := s.db.QueryRow(
		`SELECT policy_arn, version_id, document, is_default, create_date
		 FROM managed_policy_versions WHERE policy_arn = ? AND version_id = ?`,
		policyARN, versionID,
	).Scan(&v.PolicyARN, &v.VersionID, &v.Document, &isDefault, &v.CreateDate)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedPolicyVersion{}, ErrNoSuchPolicyVersion
		}
		return ManagedPolicyVersion{}, err
	}
	v.IsDefaultVersion = isDefault == 1
	return v, nil
}

// ListManagedPolicyVersions returns versions for a policy (newest VersionId last).
func (s *Store) ListManagedPolicyVersions(policyARN string) ([]ManagedPolicyVersion, error) {
	if _, err := s.GetManagedPolicy(policyARN); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT policy_arn, version_id, document, is_default, create_date
		 FROM managed_policy_versions WHERE policy_arn = ?
		 ORDER BY CAST(SUBSTR(version_id, 2) AS INTEGER)`,
		policyARN,
	)
	if err != nil {
		return nil, fmt.Errorf("list policy versions: %w", err)
	}
	defer rows.Close()
	var out []ManagedPolicyVersion
	for rows.Next() {
		var v ManagedPolicyVersion
		var isDefault int
		if err := rows.Scan(&v.PolicyARN, &v.VersionID, &v.Document, &isDefault, &v.CreateDate); err != nil {
			return nil, fmt.Errorf("list policy versions: %w", err)
		}
		v.IsDefaultVersion = isDefault == 1
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list policy versions: %w", err)
	}
	if out == nil {
		out = []ManagedPolicyVersion{}
	}
	return out, nil
}

// DeleteManagedPolicyVersion removes a non-default version.
func (s *Store) DeleteManagedPolicyVersion(policyARN, versionID string) error {
	p, err := s.GetManagedPolicy(policyARN)
	if err != nil {
		return err
	}
	if p.DefaultVersionID == versionID {
		return ErrDeleteDefaultPolicyVersion
	}
	res, err := s.db.Exec(
		`DELETE FROM managed_policy_versions WHERE policy_arn = ? AND version_id = ?`,
		policyARN, versionID,
	)
	if err != nil {
		return fmt.Errorf("delete policy version: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete policy version: %w", err)
	}
	if n == 0 {
		return ErrNoSuchPolicyVersion
	}
	return nil
}

func nextPolicyVersionID(tx *sql.Tx, policyARN string) (string, error) {
	rows, err := tx.Query(
		`SELECT version_id FROM managed_policy_versions WHERE policy_arn = ?`,
		policyARN,
	)
	if err != nil {
		return "", fmt.Errorf("create policy version: list ids: %w", err)
	}
	defer rows.Close()
	maxN := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", fmt.Errorf("create policy version: scan id: %w", err)
		}
		if !strings.HasPrefix(id, "v") {
			continue
		}
		n, err := strconv.Atoi(id[1:])
		if err != nil {
			continue
		}
		if n > maxN {
			maxN = n
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("create policy version: ids: %w", err)
	}
	return fmt.Sprintf("v%d", maxN+1), nil
}

func syncManagedPolicyDefault(tx *sql.Tx, p ManagedPolicy, versionID, document string) error {
	if _, err := tx.Exec(
		`UPDATE managed_policies SET default_version_id = ?, document = ? WHERE policy_arn = ?`,
		versionID, document, p.PolicyARN,
	); err != nil {
		return fmt.Errorf("sync managed policy default: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE policies SET document = ?, default_version_id = ? WHERE policy_id = ?`,
		document, versionID, p.PolicyARN,
	); err != nil {
		return fmt.Errorf("sync policies default document: %w", err)
	}
	return nil
}
