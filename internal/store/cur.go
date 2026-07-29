package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCURNotFound      = errors.New("ReportNotFoundException")
	ErrCURBadRequest    = errors.New("ValidationException")
	ErrCURDuplicate     = errors.New("DuplicateReportNameException")
	ErrCURLimitReached  = errors.New("ReportLimitReachedException")
)

const maxCURReportsPerAccount = 5

var curReportNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

const curSchema = `
CREATE TABLE IF NOT EXISTS cur_report_definitions (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  report_name TEXT NOT NULL,
  time_unit TEXT NOT NULL,
  format TEXT NOT NULL,
  compression TEXT NOT NULL,
  s3_bucket TEXT NOT NULL,
  s3_prefix TEXT NOT NULL DEFAULT '',
  s3_region TEXT NOT NULL,
  report_versioning TEXT NOT NULL DEFAULT '',
  additional_schema_json TEXT NOT NULL DEFAULT '[]',
  additional_artifacts_json TEXT NOT NULL DEFAULT '[]',
  refresh_closed_reports INTEGER NOT NULL DEFAULT 0,
  report_status TEXT NOT NULL DEFAULT 'PENDING',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, report_name)
);
`

// CURReportDefinition is a lab Cost and Usage Report definition.
type CURReportDefinition struct {
	AccountID                string
	Region                   string
	ReportName               string
	TimeUnit                 string
	Format                   string
	Compression              string
	S3Bucket                 string
	S3Prefix                 string
	S3Region                 string
	ReportVersioning         string
	AdditionalSchemaElements []string
	AdditionalArtifacts      []string
	RefreshClosedReports     bool
	ReportStatus             string
	CreatedAt                int64
	UpdatedAt                int64
}

// EnsureCURSchema creates CUR tables if missing.
func EnsureCURSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cur schema: db is nil")
	}
	if _, err := db.Exec(curSchema); err != nil {
		return fmt.Errorf("ensure cur schema: %w", err)
	}
	return nil
}

// EnsureCURSchema ensures CUR tables on an open store.
func (s *Store) EnsureCURSchema() error {
	return EnsureCURSchema(s.db)
}

// PutCURReportDefinition creates a report definition. Optionally emits a tiny S3 artifact.
func (s *Store) PutCURReportDefinition(accountID, region string, def CURReportDefinition) (CURReportDefinition, error) {
	if err := s.EnsureCURSchema(); err != nil {
		return CURReportDefinition{}, err
	}
	if region == "" {
		region = "us-east-1"
	}
	def.AccountID = accountID
	def.Region = region
	if err := validateCURReportDefinition(&def); err != nil {
		return CURReportDefinition{}, err
	}
	n, err := s.countCURReports(accountID)
	if err != nil {
		return CURReportDefinition{}, err
	}
	if n >= maxCURReportsPerAccount {
		return CURReportDefinition{}, fmt.Errorf("%w: max %d reports per account", ErrCURLimitReached, maxCURReportsPerAccount)
	}
	now := time.Now().UTC().UnixMilli()
	def.CreatedAt = now
	def.UpdatedAt = now
	def.ReportStatus = "PENDING"
	schemaJSON, _ := json.Marshal(def.AdditionalSchemaElements)
	artJSON, _ := json.Marshal(def.AdditionalArtifacts)
	refresh := 0
	if def.RefreshClosedReports {
		refresh = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO cur_report_definitions (
		  account_id, region, report_name, time_unit, format, compression, s3_bucket, s3_prefix, s3_region,
		  report_versioning, additional_schema_json, additional_artifacts_json, refresh_closed_reports,
		  report_status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, def.ReportName, def.TimeUnit, def.Format, def.Compression, def.S3Bucket, def.S3Prefix, def.S3Region,
		def.ReportVersioning, string(schemaJSON), string(artJSON), refresh, def.ReportStatus, now, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return CURReportDefinition{}, ErrCURDuplicate
		}
		return CURReportDefinition{}, fmt.Errorf("put cur report: %w", err)
	}
	s.emitCURArtifactBestEffort(&def)
	return def, nil
}

// ModifyCURReportDefinition replaces mutable fields on an existing report.
func (s *Store) ModifyCURReportDefinition(accountID, region, reportName string, incoming CURReportDefinition) (CURReportDefinition, error) {
	if err := s.EnsureCURSchema(); err != nil {
		return CURReportDefinition{}, err
	}
	if region == "" {
		region = "us-east-1"
	}
	existing, err := s.GetCURReportDefinition(accountID, region, reportName)
	if err != nil {
		return CURReportDefinition{}, err
	}
	incoming.AccountID = accountID
	incoming.Region = region
	if strings.TrimSpace(incoming.ReportName) == "" {
		incoming.ReportName = reportName
	}
	if incoming.ReportName != reportName {
		return CURReportDefinition{}, fmt.Errorf("%w: ReportName in the body must match the ReportName parameter", ErrCURBadRequest)
	}
	if err := validateCURReportDefinition(&incoming); err != nil {
		return CURReportDefinition{}, err
	}
	now := time.Now().UTC().UnixMilli()
	incoming.CreatedAt = existing.CreatedAt
	incoming.UpdatedAt = now
	incoming.ReportStatus = existing.ReportStatus
	if incoming.ReportStatus == "" {
		incoming.ReportStatus = "PENDING"
	}
	schemaJSON, _ := json.Marshal(incoming.AdditionalSchemaElements)
	artJSON, _ := json.Marshal(incoming.AdditionalArtifacts)
	refresh := 0
	if incoming.RefreshClosedReports {
		refresh = 1
	}
	_, err = s.db.Exec(
		`UPDATE cur_report_definitions SET
		  time_unit = ?, format = ?, compression = ?, s3_bucket = ?, s3_prefix = ?, s3_region = ?,
		  report_versioning = ?, additional_schema_json = ?, additional_artifacts_json = ?,
		  refresh_closed_reports = ?, report_status = ?, updated_at = ?
		 WHERE account_id = ? AND region = ? AND report_name = ?`,
		incoming.TimeUnit, incoming.Format, incoming.Compression, incoming.S3Bucket, incoming.S3Prefix, incoming.S3Region,
		incoming.ReportVersioning, string(schemaJSON), string(artJSON), refresh, incoming.ReportStatus, now,
		accountID, region, reportName,
	)
	if err != nil {
		return CURReportDefinition{}, fmt.Errorf("modify cur report: %w", err)
	}
	s.emitCURArtifactBestEffort(&incoming)
	return incoming, nil
}

// GetCURReportDefinition returns one report by name.
func (s *Store) GetCURReportDefinition(accountID, region, reportName string) (CURReportDefinition, error) {
	if err := s.EnsureCURSchema(); err != nil {
		return CURReportDefinition{}, err
	}
	if region == "" {
		region = "us-east-1"
	}
	reportName = strings.TrimSpace(reportName)
	var d CURReportDefinition
	var schemaJSON, artJSON string
	var refresh int
	err := s.db.QueryRow(
		`SELECT account_id, region, report_name, time_unit, format, compression, s3_bucket, s3_prefix, s3_region,
		        report_versioning, additional_schema_json, additional_artifacts_json, refresh_closed_reports,
		        report_status, created_at, updated_at
		 FROM cur_report_definitions WHERE account_id = ? AND region = ? AND report_name = ?`,
		accountID, region, reportName,
	).Scan(
		&d.AccountID, &d.Region, &d.ReportName, &d.TimeUnit, &d.Format, &d.Compression, &d.S3Bucket, &d.S3Prefix, &d.S3Region,
		&d.ReportVersioning, &schemaJSON, &artJSON, &refresh, &d.ReportStatus, &d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CURReportDefinition{}, ErrCURNotFound
	}
	if err != nil {
		return CURReportDefinition{}, fmt.Errorf("get cur report: %w", err)
	}
	d.RefreshClosedReports = refresh != 0
	_ = json.Unmarshal([]byte(schemaJSON), &d.AdditionalSchemaElements)
	_ = json.Unmarshal([]byte(artJSON), &d.AdditionalArtifacts)
	if d.AdditionalSchemaElements == nil {
		d.AdditionalSchemaElements = []string{}
	}
	if d.AdditionalArtifacts == nil {
		d.AdditionalArtifacts = []string{}
	}
	return d, nil
}

// DescribeCURReportDefinitions lists reports for an account (all regions).
func (s *Store) DescribeCURReportDefinitions(accountID string) ([]CURReportDefinition, error) {
	if err := s.EnsureCURSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT account_id, region, report_name, time_unit, format, compression, s3_bucket, s3_prefix, s3_region,
		        report_versioning, additional_schema_json, additional_artifacts_json, refresh_closed_reports,
		        report_status, created_at, updated_at
		 FROM cur_report_definitions WHERE account_id = ? ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("describe cur reports: %w", err)
	}
	defer rows.Close()
	var out []CURReportDefinition
	for rows.Next() {
		var d CURReportDefinition
		var schemaJSON, artJSON string
		var refresh int
		if err := rows.Scan(
			&d.AccountID, &d.Region, &d.ReportName, &d.TimeUnit, &d.Format, &d.Compression, &d.S3Bucket, &d.S3Prefix, &d.S3Region,
			&d.ReportVersioning, &schemaJSON, &artJSON, &refresh, &d.ReportStatus, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("describe cur reports scan: %w", err)
		}
		d.RefreshClosedReports = refresh != 0
		_ = json.Unmarshal([]byte(schemaJSON), &d.AdditionalSchemaElements)
		_ = json.Unmarshal([]byte(artJSON), &d.AdditionalArtifacts)
		if d.AdditionalSchemaElements == nil {
			d.AdditionalSchemaElements = []string{}
		}
		if d.AdditionalArtifacts == nil {
			d.AdditionalArtifacts = []string{}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteCURReportDefinition removes a report. Idempotent (nil when absent).
func (s *Store) DeleteCURReportDefinition(accountID, region, reportName string) error {
	if err := s.EnsureCURSchema(); err != nil {
		return err
	}
	if region == "" {
		region = "us-east-1"
	}
	_, err := s.db.Exec(
		`DELETE FROM cur_report_definitions WHERE account_id = ? AND region = ? AND report_name = ?`,
		accountID, region, strings.TrimSpace(reportName),
	)
	if err != nil {
		return fmt.Errorf("delete cur report: %w", err)
	}
	return nil
}

func (s *Store) countCURReports(accountID string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM cur_report_definitions WHERE account_id = ?`, accountID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count cur reports: %w", err)
	}
	return n, nil
}

func validateCURReportDefinition(d *CURReportDefinition) error {
	if d == nil {
		return fmt.Errorf("%w: ReportDefinition is required", ErrCURBadRequest)
	}
	d.ReportName = strings.TrimSpace(d.ReportName)
	if d.ReportName == "" {
		return fmt.Errorf("%w: ReportName required", ErrCURBadRequest)
	}
	if len(d.ReportName) > 256 {
		return fmt.Errorf("%w: ReportName must be at most 256 characters", ErrCURBadRequest)
	}
	if !curReportNamePattern.MatchString(d.ReportName) {
		return fmt.Errorf("%w: ReportName may only contain alphanumeric characters, hyphens, and underscores", ErrCURBadRequest)
	}
	d.TimeUnit = strings.TrimSpace(d.TimeUnit)
	switch d.TimeUnit {
	case "HOURLY", "DAILY", "MONTHLY":
	default:
		return fmt.Errorf("%w: TimeUnit must be HOURLY, DAILY, or MONTHLY", ErrCURBadRequest)
	}
	d.Format = strings.TrimSpace(d.Format)
	switch d.Format {
	case "textORcsv", "Parquet", "FOCUS":
	default:
		return fmt.Errorf("%w: Format must be textORcsv, Parquet, or FOCUS", ErrCURBadRequest)
	}
	d.Compression = strings.TrimSpace(d.Compression)
	switch d.Format {
	case "Parquet":
		if d.Compression != "Parquet" {
			return fmt.Errorf("%w: Compression must be Parquet when Format is Parquet", ErrCURBadRequest)
		}
	case "FOCUS":
		switch d.Compression {
		case "ZIP", "GZIP", "Parquet":
		default:
			return fmt.Errorf("%w: Compression must be ZIP, GZIP, or Parquet when Format is FOCUS", ErrCURBadRequest)
		}
	default:
		switch d.Compression {
		case "ZIP", "GZIP":
		default:
			return fmt.Errorf("%w: Compression must be ZIP or GZIP when Format is textORcsv", ErrCURBadRequest)
		}
	}
	d.S3Bucket = strings.TrimSpace(d.S3Bucket)
	if d.S3Bucket == "" {
		return fmt.Errorf("%w: S3Bucket required", ErrCURBadRequest)
	}
	if err := ValidateBucketName(d.S3Bucket); err != nil {
		return fmt.Errorf("%w: S3Bucket invalid", ErrCURBadRequest)
	}
	if d.S3Prefix == "" {
		d.S3Prefix = ""
	} else if err := requireSafeCURKeySegment(d.S3Prefix); err != nil {
		return err
	}
	d.S3Region = strings.TrimSpace(d.S3Region)
	if d.S3Region == "" {
		return fmt.Errorf("%w: S3Region required", ErrCURBadRequest)
	}
	if d.ReportVersioning != "" {
		switch d.ReportVersioning {
		case "CREATE_NEW_REPORT", "OVERWRITE_REPORT":
		default:
			return fmt.Errorf("%w: ReportVersioning must be CREATE_NEW_REPORT or OVERWRITE_REPORT", ErrCURBadRequest)
		}
	}
	allowedArtifacts := map[string]struct{}{"REDSHIFT": {}, "QUICKSIGHT": {}, "ATHENA": {}}
	for _, a := range d.AdditionalArtifacts {
		if _, ok := allowedArtifacts[a]; !ok {
			return fmt.Errorf("%w: AdditionalArtifacts must be REDSHIFT, QUICKSIGHT, or ATHENA", ErrCURBadRequest)
		}
	}
	allowedSchema := map[string]struct{}{
		"RESOURCES": {}, "SPLIT_COST_ALLOCATION_DATA": {}, "MANUAL_DISCOUNT_COMPATIBILITY": {},
		"FOCUS": {},
	}
	for _, e := range d.AdditionalSchemaElements {
		if _, ok := allowedSchema[e]; !ok {
			return fmt.Errorf("%w: AdditionalSchemaElements must be RESOURCES, SPLIT_COST_ALLOCATION_DATA, MANUAL_DISCOUNT_COMPATIBILITY, or FOCUS", ErrCURBadRequest)
		}
	}
	if d.AdditionalSchemaElements == nil {
		d.AdditionalSchemaElements = []string{}
	}
	if d.AdditionalArtifacts == nil {
		d.AdditionalArtifacts = []string{}
	}
	return nil
}

func requireSafeCURKeySegment(value string) error {
	for _, c := range value {
		ok := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '/'
		if !ok {
			return fmt.Errorf("%w: S3Prefix contains characters not permitted in an S3 key segment", ErrCURBadRequest)
		}
	}
	return nil
}

// curEmitEnabled returns whether Put/Modify should write a lab artifact to S3.
// Default on; set NOCTAXRIS_CUR_EMIT=0 or off to disable.
func curEmitEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("NOCTAXRIS_CUR_EMIT")))
	return v != "0" && v != "off" && v != "false"
}

func (s *Store) emitCURArtifactBestEffort(d *CURReportDefinition) {
	if d == nil || !curEmitEnabled() {
		return
	}
	if _, err := s.GetBucket(d.AccountID, d.S3Bucket); err != nil {
		_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "ERROR")
		return
	}
	if curRequestsFOCUS(d) {
		s.emitCURFOCUSBestEffort(d)
		return
	}
	runID := uuid.NewString()
	body := []byte(
		"identity/LineItemId,bill/BillingPeriodStart,lineItem/UsageAccountId,lineItem/ProductCode,lineItem/UnblendedCost,lineItem/CurrencyCode\n" +
			"lab-line-1,2026-07-01," + d.AccountID + ",AmazonS3,1.23,USD\n",
	)
	key := path.Join(strings.Trim(d.S3Prefix, "/"), d.ReportName, runID+".csv")
	key = strings.TrimPrefix(key, "/")
	if _, err := s.PutObject(d.AccountID, d.S3Bucket, key, PutObjectMeta{
		Data:        body,
		PlainSize:   int64(len(body)),
		ContentType: "text/csv",
	}); err != nil {
		_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "ERROR")
		return
	}
	_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "SUCCESS")
	d.ReportStatus = "SUCCESS"
}

// emitCURFOCUSBestEffort enumerates lab usage, projects FOCUS columns, then writes CSV or Parquet.
func (s *Store) emitCURFOCUSBestEffort(d *CURReportDefinition) {
	lines := s.CollectCURUsageLines(d.AccountID, d.Region)
	rows := ProjectFOCUSRows(lines)
	if curFOCUSUsesParquet(d) {
		s.emitCURFOCUSParquetBestEffort(d, rows)
		return
	}
	body, err := encodeFOCUSCSV(rows)
	if err != nil {
		_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "ERROR")
		d.ReportStatus = "ERROR"
		return
	}
	runID := uuid.NewString()
	key := path.Join(strings.Trim(d.S3Prefix, "/"), d.ReportName, runID+".csv")
	key = strings.TrimPrefix(key, "/")
	if _, err := s.PutObject(d.AccountID, d.S3Bucket, key, PutObjectMeta{
		Data:        body,
		PlainSize:   int64(len(body)),
		ContentType: "text/csv",
	}); err != nil {
		_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "ERROR")
		d.ReportStatus = "ERROR"
		return
	}
	_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "SUCCESS")
	d.ReportStatus = "SUCCESS"
}

func curFOCUSUsesParquet(d *CURReportDefinition) bool {
	if d == nil {
		return false
	}
	if d.Format == "Parquet" {
		return true
	}
	return d.Format == "FOCUS" && d.Compression == "Parquet"
}

// emitCURFOCUSParquetBestEffort stages FOCUS NDJSON then asks DuckDB for COPY FORMAT PARQUET.
// Fail-closed: missing runner or Duck/S3 errors set ReportStatus=ERROR (never SUCCESS with JSON stand-in).
func (s *Store) emitCURFOCUSParquetBestEffort(d *CURReportDefinition, rows []FocusRow) {
	run := s.getCURDuckRunner()
	if run == nil {
		_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "ERROR")
		d.ReportStatus = "ERROR"
		return
	}
	runID := uuid.NewString()
	stagingKey := curParquetStagingKey(d.ReportName, runID)
	destKey := curParquetDestKey(d.S3Prefix, d.ReportName, runID)
	ndjson, err := encodeFOCUSNDJSON(rows)
	if err != nil {
		_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "ERROR")
		d.ReportStatus = "ERROR"
		return
	}
	if _, err := s.PutObject(d.AccountID, d.S3Bucket, stagingKey, PutObjectMeta{
		Data:        ndjson,
		PlainSize:   int64(len(ndjson)),
		ContentType: "application/x-ndjson",
	}); err != nil {
		_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "ERROR")
		d.ReportStatus = "ERROR"
		return
	}
	querySQL, setupSQL := BuildCURParquetDuckSQL(
		curS3URI(d.S3Bucket, stagingKey),
		curS3URI(d.S3Bucket, destKey),
	)
	duckErr := run(d.AccountID, querySQL, setupSQL)
	_ = s.DeleteObject(d.AccountID, d.S3Bucket, stagingKey)
	if duckErr != nil {
		_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "ERROR")
		d.ReportStatus = "ERROR"
		return
	}
	_ = s.markCURReportStatus(d.AccountID, d.Region, d.ReportName, "SUCCESS")
	d.ReportStatus = "SUCCESS"
}

func (s *Store) markCURReportStatus(accountID, region, reportName, status string) error {
	_, err := s.db.Exec(
		`UPDATE cur_report_definitions SET report_status = ?, updated_at = ? WHERE account_id = ? AND region = ? AND report_name = ?`,
		status, time.Now().UTC().UnixMilli(), accountID, region, reportName,
	)
	return err
}
