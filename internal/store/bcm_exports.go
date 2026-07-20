package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrBCMExportNotFound   = errors.New("ResourceNotFoundException")
	ErrBCMExportBadRequest = errors.New("ValidationException")
)

const bcmExportSchema = `
CREATE TABLE IF NOT EXISTS bcm_exports (
  account_id TEXT NOT NULL,
  export_arn TEXT NOT NULL,
  export_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  format TEXT NOT NULL,
  file_path TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, export_arn)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bcm_export_name ON bcm_exports(account_id, export_name);
`

// BCMExport is a lab BCM Data Exports definition.
type BCMExport struct {
	ExportARN   string
	ExportName  string
	Description string
	Format      string
	FilePath    string
	CreatedAt   int64
}

// EnsureBCMExportSchema creates BCM export tables if missing.
func EnsureBCMExportSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure bcm export schema: db is nil")
	}
	if _, err := db.Exec(bcmExportSchema); err != nil {
		return fmt.Errorf("ensure bcm export schema: %w", err)
	}
	return nil
}

// EnsureBCMExportSchema ensures BCM export tables on an open store.
func (s *Store) EnsureBCMExportSchema() error {
	return EnsureBCMExportSchema(s.db)
}

// BCMExportARN builds arn:aws:bcm-data-exports:REGION:ACCOUNT:export/NAME/ID
func BCMExportARN(region, accountID, name, id string) string {
	if region == "" {
		region = "us-east-1"
	}
	return fmt.Sprintf("arn:aws:bcm-data-exports:%s:%s:export/%s/%s", region, accountID, name, id)
}

// CreateBCMExport creates an export definition and writes a sample file under data root.
func (s *Store) CreateBCMExport(accountID, region, name, description, format string) (BCMExport, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return BCMExport{}, fmt.Errorf("%w: Name required", ErrBCMExportBadRequest)
	}
	format = strings.ToUpper(strings.TrimSpace(format))
	if format == "" {
		format = "CSV"
	}
	if format != "CSV" && format != "JSON" {
		return BCMExport{}, fmt.Errorf("%w: Format must be CSV or JSON", ErrBCMExportBadRequest)
	}
	id := uuid.NewString()
	arn := BCMExportARN(region, accountID, name, id)
	dir := filepath.Join(s.dataRoot, "bcm-exports", accountID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return BCMExport{}, fmt.Errorf("create bcm export dir: %w", err)
	}
	ext := ".csv"
	if format == "JSON" {
		ext = ".json"
	}
	filePath := filepath.Join(dir, name+"-"+id[:8]+ext)
	sample, err := bcmSampleContent(format, accountID)
	if err != nil {
		return BCMExport{}, err
	}
	if err := os.WriteFile(filePath, sample, 0o600); err != nil {
		return BCMExport{}, fmt.Errorf("write bcm export sample: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO bcm_exports (account_id, export_arn, export_name, description, format, file_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, arn, name, strings.TrimSpace(description), format, filePath, now,
	)
	if err != nil {
		_ = os.Remove(filePath)
		return BCMExport{}, fmt.Errorf("insert bcm export: %w", err)
	}
	return BCMExport{
		ExportARN: arn, ExportName: name, Description: description,
		Format: format, FilePath: filePath, CreatedAt: now,
	}, nil
}

func bcmSampleContent(format, accountID string) ([]byte, error) {
	if format == "JSON" {
		return json.Marshal([]map[string]any{{
			"identity/LineItemId":      "lab-line-1",
			"bill/BillingPeriodStart":  "2026-07-01",
			"lineItem/UsageAccountId":  accountID,
			"lineItem/ProductCode":     "AmazonS3",
			"lineItem/UnblendedCost":   "1.23",
			"lineItem/CurrencyCode":    "USD",
		}})
	}
	csv := "identity/LineItemId,bill/BillingPeriodStart,lineItem/UsageAccountId,lineItem/ProductCode,lineItem/UnblendedCost,lineItem/CurrencyCode\n" +
		"lab-line-1,2026-07-01," + accountID + ",AmazonS3,1.23,USD\n"
	return []byte(csv), nil
}

// GetBCMExport returns an export by ARN.
func (s *Store) GetBCMExport(accountID, exportARN string) (BCMExport, error) {
	exportARN = strings.TrimSpace(exportARN)
	var e BCMExport
	err := s.db.QueryRow(
		`SELECT export_arn, export_name, description, format, file_path, created_at
		 FROM bcm_exports WHERE account_id = ? AND export_arn = ?`,
		accountID, exportARN,
	).Scan(&e.ExportARN, &e.ExportName, &e.Description, &e.Format, &e.FilePath, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BCMExport{}, ErrBCMExportNotFound
	}
	if err != nil {
		return BCMExport{}, fmt.Errorf("get bcm export: %w", err)
	}
	return e, nil
}

// ListBCMExports lists exports for an account.
func (s *Store) ListBCMExports(accountID string) ([]BCMExport, error) {
	rows, err := s.db.Query(
		`SELECT export_arn, export_name, description, format, file_path, created_at
		 FROM bcm_exports WHERE account_id = ? ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list bcm exports: %w", err)
	}
	defer rows.Close()
	var out []BCMExport
	for rows.Next() {
		var e BCMExport
		if err := rows.Scan(&e.ExportARN, &e.ExportName, &e.Description, &e.Format, &e.FilePath, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("list bcm exports scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteBCMExport deletes an export definition and sample file.
func (s *Store) DeleteBCMExport(accountID, exportARN string) error {
	e, err := s.GetBCMExport(accountID, exportARN)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM bcm_exports WHERE account_id = ? AND export_arn = ?`, accountID, exportARN)
	if err != nil {
		return fmt.Errorf("delete bcm export: %w", err)
	}
	_ = os.Remove(e.FilePath)
	return nil
}
