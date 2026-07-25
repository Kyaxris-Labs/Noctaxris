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
	ErrGuardDutyNotFound   = errors.New("BadRequestException")
	ErrGuardDutyBadRequest = errors.New("BadRequestException")
)

const DefaultGuardDutyRegion = "us-east-1"

const guarddutySchema = `
CREATE TABLE IF NOT EXISTS guardduty_detectors (
  account_id TEXT NOT NULL,
  detector_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ENABLED',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, detector_id)
);
CREATE TABLE IF NOT EXISTS guardduty_findings (
  account_id TEXT NOT NULL,
  detector_id TEXT NOT NULL,
  finding_id TEXT NOT NULL,
  finding_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, detector_id, finding_id)
);
CREATE INDEX IF NOT EXISTS idx_gd_findings_detector ON guardduty_findings(account_id, detector_id, updated_at);
`

// GuardDutyDetector is a lab detector row.
type GuardDutyDetector struct {
	DetectorID string
	Status     string
	CreatedAt  int64
}

// GuardDutyFinding is a lab finding (AWS Finding lite).
// Field names follow https://docs.aws.amazon.com/guardduty/latest/APIReference/API_Finding.html
type GuardDutyFinding struct {
	AccountID     string         `json:"accountId"`
	Arn           string         `json:"arn"`
	Id            string         `json:"id"`
	Type          string         `json:"type"`
	Severity      float64        `json:"severity"`
	Title         string         `json:"title,omitempty"`
	Description   string         `json:"description,omitempty"`
	Region        string         `json:"region"`
	SchemaVersion string         `json:"schemaVersion"`
	CreatedAt     string         `json:"createdAt"`
	UpdatedAt     string         `json:"updatedAt"`
	Partition     string         `json:"partition,omitempty"`
	Resource      map[string]any `json:"resource"`
	Service       map[string]any `json:"service,omitempty"`
}

// EnsureGuardDutySchema creates GuardDuty tables if missing.
func EnsureGuardDutySchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure guardduty schema: db is nil")
	}
	if _, err := db.Exec(guarddutySchema); err != nil {
		return fmt.Errorf("ensure guardduty schema: %w", err)
	}
	return nil
}

// EnsureGuardDutySchema ensures GuardDuty tables on an open store.
func (s *Store) EnsureGuardDutySchema() error {
	return EnsureGuardDutySchema(s.db)
}

// CreateGuardDutyDetector creates a detector (lab: one active detector per account is enough).
func (s *Store) CreateGuardDutyDetector(accountID string) (GuardDutyDetector, error) {
	if err := s.EnsureGuardDutySchema(); err != nil {
		return GuardDutyDetector{}, err
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO guardduty_detectors (account_id, detector_id, status, created_at) VALUES (?, ?, 'ENABLED', ?)`,
		accountID, id, now,
	)
	if err != nil {
		return GuardDutyDetector{}, fmt.Errorf("create guardduty detector: %w", err)
	}
	return GuardDutyDetector{DetectorID: id, Status: "ENABLED", CreatedAt: now}, nil
}

// ListGuardDutyDetectors returns detector IDs for an account.
func (s *Store) ListGuardDutyDetectors(accountID string) ([]string, error) {
	if err := s.EnsureGuardDutySchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT detector_id FROM guardduty_detectors WHERE account_id = ? ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list guardduty detectors: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list guardduty detectors scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// EnsureGuardDutyDetector returns an existing detector or creates one.
func (s *Store) EnsureGuardDutyDetector(accountID string) (string, error) {
	ids, err := s.ListGuardDutyDetectors(accountID)
	if err != nil {
		return "", err
	}
	if len(ids) > 0 {
		return ids[0], nil
	}
	d, err := s.CreateGuardDutyDetector(accountID)
	if err != nil {
		return "", err
	}
	return d.DetectorID, nil
}

// GetGuardDutyDetector returns a detector or ErrGuardDutyNotFound.
func (s *Store) GetGuardDutyDetector(accountID, detectorID string) (GuardDutyDetector, error) {
	if err := s.EnsureGuardDutySchema(); err != nil {
		return GuardDutyDetector{}, err
	}
	var d GuardDutyDetector
	err := s.db.QueryRow(
		`SELECT detector_id, status, created_at FROM guardduty_detectors WHERE account_id = ? AND detector_id = ?`,
		accountID, detectorID,
	).Scan(&d.DetectorID, &d.Status, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GuardDutyDetector{}, fmt.Errorf("%w: detector not found", ErrGuardDutyNotFound)
	}
	if err != nil {
		return GuardDutyDetector{}, fmt.Errorf("get guardduty detector: %w", err)
	}
	return d, nil
}

// InjectGuardDutyFindings upserts lab findings (cap 50). Fills required AWS Finding fields.
func (s *Store) InjectGuardDutyFindings(accountID, detectorID, region string, findings []GuardDutyFinding) ([]string, error) {
	if err := s.EnsureGuardDutySchema(); err != nil {
		return nil, err
	}
	if _, err := s.GetGuardDutyDetector(accountID, detectorID); err != nil {
		return nil, err
	}
	if len(findings) == 0 {
		return nil, fmt.Errorf("%w: Findings required", ErrGuardDutyBadRequest)
	}
	if len(findings) > 50 {
		return nil, fmt.Errorf("%w: Findings cap is 50", ErrGuardDutyBadRequest)
	}
	if region == "" {
		region = DefaultGuardDutyRegion
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	nowMS := now.UnixMilli()
	ids := make([]string, 0, len(findings))
	for i := range findings {
		f := &findings[i]
		if f.Id == "" {
			f.Id = uuid.NewString()
		}
		if f.AccountID == "" {
			f.AccountID = accountID
		}
		if f.Region == "" {
			f.Region = region
		}
		if f.SchemaVersion == "" {
			f.SchemaVersion = "2.0"
		}
		if f.Partition == "" {
			f.Partition = "aws"
		}
		if f.Type == "" {
			return nil, fmt.Errorf("%w: type required", ErrGuardDutyBadRequest)
		}
		if f.Severity == 0 {
			f.Severity = 5
		}
		if f.CreatedAt == "" {
			f.CreatedAt = nowStr
		}
		if f.UpdatedAt == "" {
			f.UpdatedAt = nowStr
		}
		if f.Arn == "" {
			f.Arn = fmt.Sprintf("arn:aws:guardduty:%s:%s:detector/%s/finding/%s",
				f.Region, f.AccountID, detectorID, f.Id)
		}
		if f.Resource == nil {
			f.Resource = map[string]any{"resourceType": "AccessKey"}
		}
		raw, err := json.Marshal(f)
		if err != nil {
			return nil, fmt.Errorf("marshal guardduty finding: %w", err)
		}
		_, err = s.db.Exec(
			`INSERT INTO guardduty_findings (account_id, detector_id, finding_id, finding_json, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, detector_id, finding_id) DO UPDATE SET
			   finding_json = excluded.finding_json,
			   updated_at = excluded.updated_at`,
			accountID, detectorID, f.Id, string(raw), nowMS, nowMS,
		)
		if err != nil {
			return nil, fmt.Errorf("inject guardduty finding: %w", err)
		}
		ids = append(ids, f.Id)
	}
	return ids, nil
}

// ListGuardDutyFindingIDs returns finding IDs newest-first (lab MaxResults).
func (s *Store) ListGuardDutyFindingIDs(accountID, detectorID string, maxResults int) ([]string, error) {
	if err := s.EnsureGuardDutySchema(); err != nil {
		return nil, err
	}
	if _, err := s.GetGuardDutyDetector(accountID, detectorID); err != nil {
		return nil, err
	}
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 50
	}
	rows, err := s.db.Query(
		`SELECT finding_id FROM guardduty_findings
		 WHERE account_id = ? AND detector_id = ?
		 ORDER BY updated_at DESC LIMIT ?`,
		accountID, detectorID, maxResults,
	)
	if err != nil {
		return nil, fmt.Errorf("list guardduty finding ids: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list guardduty finding ids scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetGuardDutyFindings returns findings by ID (skips missing).
func (s *Store) GetGuardDutyFindings(accountID, detectorID string, findingIDs []string) ([]GuardDutyFinding, error) {
	if err := s.EnsureGuardDutySchema(); err != nil {
		return nil, err
	}
	if _, err := s.GetGuardDutyDetector(accountID, detectorID); err != nil {
		return nil, err
	}
	if len(findingIDs) == 0 {
		return nil, fmt.Errorf("%w: findingIds required", ErrGuardDutyBadRequest)
	}
	if len(findingIDs) > 50 {
		return nil, fmt.Errorf("%w: findingIds cap is 50", ErrGuardDutyBadRequest)
	}
	out := make([]GuardDutyFinding, 0, len(findingIDs))
	for _, id := range findingIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		var raw string
		err := s.db.QueryRow(
			`SELECT finding_json FROM guardduty_findings
			 WHERE account_id = ? AND detector_id = ? AND finding_id = ?`,
			accountID, detectorID, id,
		).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("get guardduty finding: %w", err)
		}
		var f GuardDutyFinding
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			return nil, fmt.Errorf("unmarshal guardduty finding: %w", err)
		}
		out = append(out, f)
	}
	return out, nil
}
