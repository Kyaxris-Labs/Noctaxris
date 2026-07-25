package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrMacieNotFound   = errors.New("ResourceNotFoundException")
	ErrMacieBadRequest = errors.New("ValidationException")
	ErrMacieConflict   = errors.New("ConflictException")
)

const DefaultMacieRegion = "us-east-1"

const macieSchema = `
CREATE TABLE IF NOT EXISTS macie_sessions (
  account_id TEXT NOT NULL PRIMARY KEY,
  status TEXT NOT NULL DEFAULT 'ENABLED',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS macie_jobs (
  account_id TEXT NOT NULL,
  job_id TEXT NOT NULL,
  job_arn TEXT NOT NULL,
  name TEXT NOT NULL,
  job_type TEXT NOT NULL DEFAULT 'ONE_TIME',
  job_status TEXT NOT NULL DEFAULT 'COMPLETE',
  job_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, job_id)
);
CREATE TABLE IF NOT EXISTS macie_findings (
  account_id TEXT NOT NULL,
  finding_id TEXT NOT NULL,
  finding_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, finding_id)
);
CREATE INDEX IF NOT EXISTS idx_macie_findings_updated ON macie_findings(account_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_macie_jobs_created ON macie_jobs(account_id, created_at);
`

// MacieSession is a lab Macie session row.
type MacieSession struct {
	AccountID string
	Status    string
	CreatedAt int64
	UpdatedAt int64
}

// MacieJob is a lab classification job (sync COMPLETE; no ML engine).
type MacieJob struct {
	JobID     string
	JobARN    string
	Name      string
	JobType   string
	JobStatus string
	CreatedAt int64
	UpdatedAt int64
	Raw       map[string]any
}

// MacieFinding is a Macie2 Finding lite.
// Field names follow https://docs.aws.amazon.com/macie/latest/APIReference/findings.html
type MacieFinding struct {
	AccountID             string         `json:"accountId"`
	Archived              bool           `json:"archived,omitempty"`
	Category              string         `json:"category"`
	ClassificationDetails map[string]any `json:"classificationDetails,omitempty"`
	Count                 int64          `json:"count,omitempty"`
	CreatedAt             string         `json:"createdAt"`
	Description           string         `json:"description,omitempty"`
	Id                    string         `json:"id"`
	Partition             string         `json:"partition,omitempty"`
	Region                string         `json:"region"`
	ResourcesAffected     map[string]any `json:"resourcesAffected,omitempty"`
	Sample                bool           `json:"sample,omitempty"`
	SchemaVersion         string         `json:"schemaVersion,omitempty"`
	Severity              map[string]any `json:"severity,omitempty"`
	Title                 string         `json:"title,omitempty"`
	Type                  string         `json:"type"`
	UpdatedAt             string         `json:"updatedAt"`
}

// MacieS3ObjectRef names an existing lab object for canned sensitive-data inject.
type MacieS3ObjectRef struct {
	Bucket string
	Key    string
}

type macieCannedMatch struct {
	FindingType string
	Category    string
	Detection   string
	Title       string
	Description string
	Severity    map[string]any
	Pattern     *regexp.Regexp
}

var macieCannedMatchers = []macieCannedMatch{
	{
		FindingType: "SensitiveData:S3Object/Credentials",
		Category:    "CREDENTIALS",
		Detection:   "AWS_CREDENTIALS",
		Title:       "The S3 object contains credentials",
		Description: "Lab canned matcher found an AWS access key id pattern.",
		Severity:    map[string]any{"description": "High", "score": 3},
		Pattern:     regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	},
	{
		FindingType: "SensitiveData:S3Object/Financial",
		Category:    "FINANCIAL_INFORMATION",
		Detection:   "CREDIT_CARD_NUMBER",
		Title:       "The S3 object contains financial information",
		Description: "Lab canned matcher found a credit-card-shaped number pattern.",
		Severity:    map[string]any{"description": "High", "score": 3},
		Pattern:     regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`),
	},
	{
		FindingType: "SensitiveData:S3Object/Personal",
		Category:    "PERSONAL_INFORMATION",
		Detection:   "USA_SOCIAL_SECURITY_NUMBER",
		Title:       "The S3 object contains personal information",
		Description: "Lab canned matcher found a US SSN-shaped pattern.",
		Severity:    map[string]any{"description": "High", "score": 3},
		Pattern:     regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
	},
}

// EnsureMacieSchema creates Macie tables if missing.
func EnsureMacieSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure macie schema: db is nil")
	}
	if _, err := db.Exec(macieSchema); err != nil {
		return fmt.Errorf("ensure macie schema: %w", err)
	}
	return nil
}

// EnsureMacieSchema ensures Macie tables on an open store.
func (s *Store) EnsureMacieSchema() error {
	return EnsureMacieSchema(s.db)
}

// EnableMacie creates or returns the account Macie session (ENABLED).
func (s *Store) EnableMacie(accountID string) (MacieSession, error) {
	if err := s.EnsureMacieSchema(); err != nil {
		return MacieSession{}, err
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO macie_sessions (account_id, status, created_at, updated_at) VALUES (?, 'ENABLED', ?, ?)
		 ON CONFLICT(account_id) DO UPDATE SET status = 'ENABLED', updated_at = excluded.updated_at`,
		accountID, now, now,
	)
	if err != nil {
		return MacieSession{}, fmt.Errorf("enable macie: %w", err)
	}
	return s.GetMacieSession(accountID)
}

// GetMacieSession returns the session or ErrMacieNotFound.
func (s *Store) GetMacieSession(accountID string) (MacieSession, error) {
	if err := s.EnsureMacieSchema(); err != nil {
		return MacieSession{}, err
	}
	var sess MacieSession
	err := s.db.QueryRow(
		`SELECT account_id, status, created_at, updated_at FROM macie_sessions WHERE account_id = ?`,
		accountID,
	).Scan(&sess.AccountID, &sess.Status, &sess.CreatedAt, &sess.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MacieSession{}, fmt.Errorf("%w: Macie is not enabled", ErrMacieNotFound)
	}
	if err != nil {
		return MacieSession{}, fmt.Errorf("get macie session: %w", err)
	}
	return sess, nil
}

func (s *Store) requireMacieEnabled(accountID string) error {
	_, err := s.GetMacieSession(accountID)
	return err
}

// CreateMacieClassificationJob stores a ONE_TIME job that completes immediately (lab).
func (s *Store) CreateMacieClassificationJob(accountID, region, name, jobType string, def map[string]any) (MacieJob, error) {
	if err := s.EnsureMacieSchema(); err != nil {
		return MacieJob{}, err
	}
	if err := s.requireMacieEnabled(accountID); err != nil {
		return MacieJob{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return MacieJob{}, fmt.Errorf("%w: name is required", ErrMacieBadRequest)
	}
	if jobType == "" {
		jobType = "ONE_TIME"
	}
	if region == "" {
		region = DefaultMacieRegion
	}
	jobID := strings.ReplaceAll(uuid.NewString(), "-", "")
	jobARN := fmt.Sprintf("arn:aws:macie2:%s:%s:classification-job/%s", region, accountID, jobID)
	now := time.Now().UTC().UnixMilli()
	raw := map[string]any{
		"jobId":     jobID,
		"jobArn":    jobARN,
		"name":      name,
		"jobType":   jobType,
		"jobStatus": "COMPLETE",
		"createdAt": time.UnixMilli(now).UTC().Format(time.RFC3339),
	}
	if def != nil {
		raw["s3JobDefinition"] = def
	}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		return MacieJob{}, fmt.Errorf("marshal macie job: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO macie_jobs (account_id, job_id, job_arn, name, job_type, job_status, job_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'COMPLETE', ?, ?, ?)`,
		accountID, jobID, jobARN, name, jobType, string(rawJSON), now, now,
	)
	if err != nil {
		return MacieJob{}, fmt.Errorf("create macie job: %w", err)
	}
	return MacieJob{
		JobID: jobID, JobARN: jobARN, Name: name, JobType: jobType,
		JobStatus: "COMPLETE", CreatedAt: now, UpdatedAt: now, Raw: raw,
	}, nil
}

// DescribeMacieClassificationJob returns a job or ErrMacieNotFound.
func (s *Store) DescribeMacieClassificationJob(accountID, jobID string) (MacieJob, error) {
	if err := s.EnsureMacieSchema(); err != nil {
		return MacieJob{}, err
	}
	if err := s.requireMacieEnabled(accountID); err != nil {
		return MacieJob{}, err
	}
	var j MacieJob
	var raw string
	err := s.db.QueryRow(
		`SELECT job_id, job_arn, name, job_type, job_status, job_json, created_at, updated_at
		 FROM macie_jobs WHERE account_id = ? AND job_id = ?`,
		accountID, jobID,
	).Scan(&j.JobID, &j.JobARN, &j.Name, &j.JobType, &j.JobStatus, &raw, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MacieJob{}, fmt.Errorf("%w: job not found", ErrMacieNotFound)
	}
	if err != nil {
		return MacieJob{}, fmt.Errorf("describe macie job: %w", err)
	}
	_ = json.Unmarshal([]byte(raw), &j.Raw)
	return j, nil
}

// ListMacieClassificationJobs returns jobs newest-first.
func (s *Store) ListMacieClassificationJobs(accountID string, maxResults int) ([]MacieJob, error) {
	if err := s.EnsureMacieSchema(); err != nil {
		return nil, err
	}
	if err := s.requireMacieEnabled(accountID); err != nil {
		return nil, err
	}
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 50
	}
	rows, err := s.db.Query(
		`SELECT job_id, job_arn, name, job_type, job_status, job_json, created_at, updated_at
		 FROM macie_jobs WHERE account_id = ? ORDER BY created_at DESC LIMIT ?`,
		accountID, maxResults,
	)
	if err != nil {
		return nil, fmt.Errorf("list macie jobs: %w", err)
	}
	defer rows.Close()
	var out []MacieJob
	for rows.Next() {
		var j MacieJob
		var raw string
		if err := rows.Scan(&j.JobID, &j.JobARN, &j.Name, &j.JobType, &j.JobStatus, &raw, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list macie jobs scan: %w", err)
		}
		_ = json.Unmarshal([]byte(raw), &j.Raw)
		out = append(out, j)
	}
	return out, rows.Err()
}

// InjectMacieFindings upserts lab findings (cap 50).
func (s *Store) InjectMacieFindings(accountID, region string, findings []MacieFinding) ([]string, error) {
	if err := s.EnsureMacieSchema(); err != nil {
		return nil, err
	}
	if err := s.requireMacieEnabled(accountID); err != nil {
		return nil, err
	}
	if len(findings) == 0 {
		return nil, fmt.Errorf("%w: Findings required", ErrMacieBadRequest)
	}
	if len(findings) > 50 {
		return nil, fmt.Errorf("%w: Findings cap is 50", ErrMacieBadRequest)
	}
	if region == "" {
		region = DefaultMacieRegion
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
		if f.Partition == "" {
			f.Partition = "aws"
		}
		if f.SchemaVersion == "" {
			f.SchemaVersion = "1.0"
		}
		if f.Category == "" {
			f.Category = "CLASSIFICATION"
		}
		if f.Type == "" {
			return nil, fmt.Errorf("%w: type required", ErrMacieBadRequest)
		}
		if f.CreatedAt == "" {
			f.CreatedAt = nowStr
		}
		if f.UpdatedAt == "" {
			f.UpdatedAt = nowStr
		}
		if f.Severity == nil {
			f.Severity = map[string]any{"description": "Medium", "score": 2}
		}
		if f.Count == 0 {
			f.Count = 1
		}
		raw, err := json.Marshal(f)
		if err != nil {
			return nil, fmt.Errorf("marshal macie finding: %w", err)
		}
		_, err = s.db.Exec(
			`INSERT INTO macie_findings (account_id, finding_id, finding_json, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, finding_id) DO UPDATE SET
			   finding_json = excluded.finding_json,
			   updated_at = excluded.updated_at`,
			accountID, f.Id, string(raw), nowMS, nowMS,
		)
		if err != nil {
			return nil, fmt.Errorf("inject macie finding: %w", err)
		}
		ids = append(ids, f.Id)
	}
	return ids, nil
}

// InjectMacieFindingsFromS3Objects scans existing lab objects with canned patterns and stores findings.
func (s *Store) InjectMacieFindingsFromS3Objects(accountID, region, jobID string, objects []MacieS3ObjectRef) ([]string, error) {
	if err := s.requireMacieEnabled(accountID); err != nil {
		return nil, err
	}
	if len(objects) == 0 {
		return nil, fmt.Errorf("%w: S3Objects required", ErrMacieBadRequest)
	}
	if len(objects) > 50 {
		return nil, fmt.Errorf("%w: S3Objects cap is 50", ErrMacieBadRequest)
	}
	if region == "" {
		region = DefaultMacieRegion
	}
	var findings []MacieFinding
	for _, obj := range objects {
		bucket := strings.TrimSpace(obj.Bucket)
		key := strings.TrimSpace(obj.Key)
		if bucket == "" || key == "" {
			return nil, fmt.Errorf("%w: bucket and key required", ErrMacieBadRequest)
		}
		_, data, err := s.GetObject(accountID, bucket, key)
		if errors.Is(err, ErrNoSuchKey) {
			return nil, fmt.Errorf("%w: object s3://%s/%s not found", ErrMacieBadRequest, bucket, key)
		}
		if err != nil {
			return nil, fmt.Errorf("read s3 object for macie inject: %w", err)
		}
		text := string(data)
		for _, m := range macieCannedMatchers {
			matches := m.Pattern.FindAllStringIndex(text, -1)
			if len(matches) == 0 {
				continue
			}
			findings = append(findings, MacieFinding{
				Type:        m.FindingType,
				Category:    "CLASSIFICATION",
				Title:       m.Title,
				Description: m.Description,
				Severity:    m.Severity,
				Count:       int64(len(matches)),
				ResourcesAffected: map[string]any{
					"s3Bucket": map[string]any{
						"name": bucket,
						"arn":  fmt.Sprintf("arn:aws:s3:::%s", bucket),
					},
					"s3Object": map[string]any{
						"bucketArn": fmt.Sprintf("arn:aws:s3:::%s", bucket),
						"key":       key,
						"path":      bucket + "/" + key,
						"extension": strings.TrimPrefix(path.Ext(key), "."),
					},
				},
				ClassificationDetails: map[string]any{
					"jobId": jobID,
					"result": map[string]any{
						"status": map[string]any{"code": "COMPLETE"},
						"sensitiveData": []map[string]any{
							{
								"category":   m.Category,
								"totalCount": len(matches),
								"detections": []map[string]any{
									{"type": m.Detection, "count": len(matches)},
								},
							},
						},
					},
				},
			})
		}
	}
	if len(findings) == 0 {
		return nil, fmt.Errorf("%w: no canned sensitive-data matches in supplied objects", ErrMacieBadRequest)
	}
	if len(findings) > 50 {
		findings = findings[:50]
	}
	return s.InjectMacieFindings(accountID, region, findings)
}

// ListMacieFindingIDs returns finding IDs newest-first.
func (s *Store) ListMacieFindingIDs(accountID string, maxResults int) ([]string, error) {
	if err := s.EnsureMacieSchema(); err != nil {
		return nil, err
	}
	if err := s.requireMacieEnabled(accountID); err != nil {
		return nil, err
	}
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 50
	}
	rows, err := s.db.Query(
		`SELECT finding_id FROM macie_findings WHERE account_id = ? ORDER BY updated_at DESC LIMIT ?`,
		accountID, maxResults,
	)
	if err != nil {
		return nil, fmt.Errorf("list macie finding ids: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list macie finding ids scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetMacieFindings returns findings by ID (skips missing).
func (s *Store) GetMacieFindings(accountID string, findingIDs []string) ([]MacieFinding, error) {
	if err := s.EnsureMacieSchema(); err != nil {
		return nil, err
	}
	if err := s.requireMacieEnabled(accountID); err != nil {
		return nil, err
	}
	if len(findingIDs) == 0 {
		return nil, fmt.Errorf("%w: findingIds required", ErrMacieBadRequest)
	}
	if len(findingIDs) > 50 {
		return nil, fmt.Errorf("%w: findingIds cap is 50", ErrMacieBadRequest)
	}
	out := make([]MacieFinding, 0, len(findingIDs))
	for _, id := range findingIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		var raw string
		err := s.db.QueryRow(
			`SELECT finding_json FROM macie_findings WHERE account_id = ? AND finding_id = ?`,
			accountID, id,
		).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("get macie finding: %w", err)
		}
		var f MacieFinding
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			return nil, fmt.Errorf("unmarshal macie finding: %w", err)
		}
		out = append(out, f)
	}
	return out, nil
}
