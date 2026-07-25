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
	ErrSecurityHubBadRequest = errors.New("InvalidInputException")
	ErrSecurityHubNotFound   = errors.New("ResourceNotFoundException")
)

const DefaultSecurityHubRegion = "us-east-1"

const securityHubSchema = `
CREATE TABLE IF NOT EXISTS securityhub_findings (
  account_id TEXT NOT NULL,
  finding_id TEXT NOT NULL,
  product_arn TEXT NOT NULL DEFAULT '',
  generator_id TEXT NOT NULL DEFAULT '',
  severity_label TEXT NOT NULL DEFAULT '',
  resource_type TEXT NOT NULL DEFAULT '',
  finding_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, finding_id)
);
CREATE INDEX IF NOT EXISTS idx_sh_findings_updated ON securityhub_findings(account_id, updated_at);
`

// SecurityHubFinding is an ASFF-lite finding.
// Required fields follow BatchImportFindings / AwsSecurityFinding docs.
type SecurityHubFinding struct {
	SchemaVersion string           `json:"SchemaVersion"`
	Id            string           `json:"Id"`
	ProductArn    string           `json:"ProductArn"`
	GeneratorId   string           `json:"GeneratorId"`
	AwsAccountId  string           `json:"AwsAccountId"`
	Types         []string         `json:"Types"`
	CreatedAt     string           `json:"CreatedAt"`
	UpdatedAt     string           `json:"UpdatedAt"`
	Severity      map[string]any   `json:"Severity"`
	Title         string           `json:"Title"`
	Description   string           `json:"Description"`
	Resources     []map[string]any `json:"Resources"`
}

// EnsureSecurityHubSchema creates Security Hub tables if missing.
func EnsureSecurityHubSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure securityhub schema: db is nil")
	}
	if _, err := db.Exec(securityHubSchema); err != nil {
		return fmt.Errorf("ensure securityhub schema: %w", err)
	}
	return nil
}

// EnsureSecurityHubSchema ensures Security Hub tables on an open store.
func (s *Store) EnsureSecurityHubSchema() error {
	return EnsureSecurityHubSchema(s.db)
}

// BatchImportSecurityHubFindings upserts findings (cap 100). Returns successful IDs.
func (s *Store) BatchImportSecurityHubFindings(accountID, region string, findings []SecurityHubFinding) (succeeded []string, failed []map[string]any, err error) {
	if err := s.EnsureSecurityHubSchema(); err != nil {
		return nil, nil, err
	}
	if len(findings) == 0 {
		return nil, nil, fmt.Errorf("%w: Findings required", ErrSecurityHubBadRequest)
	}
	if len(findings) > 100 {
		return nil, nil, fmt.Errorf("%w: Findings cap is 100", ErrSecurityHubBadRequest)
	}
	if region == "" {
		region = DefaultSecurityHubRegion
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	nowMS := now.UnixMilli()
	for i := range findings {
		f := &findings[i]
		if f.Id == "" {
			f.Id = uuid.NewString()
		}
		if f.SchemaVersion == "" {
			f.SchemaVersion = "2018-10-08"
		}
		if f.AwsAccountId == "" {
			f.AwsAccountId = accountID
		}
		if f.ProductArn == "" {
			f.ProductArn = fmt.Sprintf("arn:aws:securityhub:%s:%s:product/%s/default", region, accountID, accountID)
		}
		if f.GeneratorId == "" {
			failed = append(failed, map[string]any{
				"Id":      f.Id,
				"ErrorCode": "InvalidInput",
				"ErrorMessage": "GeneratorId required",
			})
			continue
		}
		if len(f.Types) == 0 {
			failed = append(failed, map[string]any{
				"Id":           f.Id,
				"ErrorCode":    "InvalidInput",
				"ErrorMessage": "Types required",
			})
			continue
		}
		if f.Title == "" || f.Description == "" {
			failed = append(failed, map[string]any{
				"Id":           f.Id,
				"ErrorCode":    "InvalidInput",
				"ErrorMessage": "Title and Description required",
			})
			continue
		}
		if f.CreatedAt == "" {
			f.CreatedAt = nowStr
		}
		if f.UpdatedAt == "" {
			f.UpdatedAt = nowStr
		}
		if f.Severity == nil {
			f.Severity = map[string]any{"Label": "MEDIUM", "Original": "5"}
		}
		if len(f.Resources) == 0 {
			f.Resources = []map[string]any{{
				"Type":      "Other",
				"Id":        "arn:aws:noctaxris:lab",
				"Partition": "aws",
				"Region":    region,
			}}
		}
		sevLabel, _ := f.Severity["Label"].(string)
		resType := ""
		if len(f.Resources) > 0 {
			resType, _ = f.Resources[0]["Type"].(string)
		}
		raw, mErr := json.Marshal(f)
		if mErr != nil {
			failed = append(failed, map[string]any{
				"Id":           f.Id,
				"ErrorCode":    "InvalidInput",
				"ErrorMessage": "unable to marshal finding",
			})
			continue
		}
		_, e := s.db.Exec(
			`INSERT INTO securityhub_findings (
			   account_id, finding_id, product_arn, generator_id, severity_label, resource_type, finding_json, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, finding_id) DO UPDATE SET
			   product_arn = excluded.product_arn,
			   generator_id = excluded.generator_id,
			   severity_label = excluded.severity_label,
			   resource_type = excluded.resource_type,
			   finding_json = excluded.finding_json,
			   updated_at = excluded.updated_at`,
			accountID, f.Id, f.ProductArn, f.GeneratorId, sevLabel, resType, string(raw), nowMS, nowMS,
		)
		if e != nil {
			failed = append(failed, map[string]any{
				"Id":           f.Id,
				"ErrorCode":    "InternalError",
				"ErrorMessage": e.Error(),
			})
			continue
		}
		succeeded = append(succeeded, f.Id)
	}
	return succeeded, failed, nil
}

// SecurityHubFindingsFilter is a lite GetFindings filter.
type SecurityHubFindingsFilter struct {
	ProductArn     string
	GeneratorId    string
	SeverityLabel  string
	ResourceType   string
	MaxResults     int
}

// GetSecurityHubFindings returns findings newest-first with optional filters.
func (s *Store) GetSecurityHubFindings(accountID string, filter SecurityHubFindingsFilter) ([]SecurityHubFinding, error) {
	if err := s.EnsureSecurityHubSchema(); err != nil {
		return nil, err
	}
	max := filter.MaxResults
	if max <= 0 || max > 100 {
		max = 100
	}
	q := `SELECT finding_json FROM securityhub_findings WHERE account_id = ?`
	args := []any{accountID}
	if v := strings.TrimSpace(filter.ProductArn); v != "" {
		q += ` AND product_arn = ?`
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.GeneratorId); v != "" {
		q += ` AND generator_id = ?`
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.SeverityLabel); v != "" {
		q += ` AND severity_label = ?`
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.ResourceType); v != "" {
		q += ` AND resource_type = ?`
		args = append(args, v)
	}
	q += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, max)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get securityhub findings: %w", err)
	}
	defer rows.Close()
	var out []SecurityHubFinding
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("get securityhub findings scan: %w", err)
		}
		var f SecurityHubFinding
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			return nil, fmt.Errorf("unmarshal securityhub finding: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
