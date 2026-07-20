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
)

var (
	ErrTranscribeBadRequest = errors.New("BadRequestException")
	ErrTranscribeConflict   = errors.New("ConflictException")
	ErrTranscribeNotFound   = errors.New("NotFoundException")
)

const DefaultTranscribeRegion = "us-east-1"

const transcribeSchema = `
CREATE TABLE IF NOT EXISTS transcribe_jobs (
  account_id TEXT NOT NULL,
  job_name TEXT NOT NULL,
  job_status TEXT NOT NULL,
  language_code TEXT NOT NULL DEFAULT 'en-US',
  media_uri TEXT NOT NULL DEFAULT '',
  transcript_uri TEXT NOT NULL DEFAULT '',
  failure_reason TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  completed_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, job_name)
);
`

// TranscribeJob is a StartTranscriptionJob stub row.
type TranscribeJob struct {
	JobName       string
	JobStatus     string
	LanguageCode  string
	MediaURI      string
	TranscriptURI string
	FailureReason string
	CreatedAt     int64
	CompletedAt   int64
}

// EnsureTranscribeSchema creates Transcribe tables if missing.
func EnsureTranscribeSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure transcribe schema: db is nil")
	}
	if _, err := db.Exec(transcribeSchema); err != nil {
		return fmt.Errorf("ensure transcribe schema: %w", err)
	}
	return nil
}

// EnsureTranscribeSchema ensures Transcribe tables on an open store.
func (s *Store) EnsureTranscribeSchema() error {
	return EnsureTranscribeSchema(s.db)
}

// StartTranscriptionJobStub creates a completed job with a canned transcript under data root.
// MediaFileUri must be s3://bucket/key and the object must exist (HeadObject).
func (s *Store) StartTranscriptionJobStub(accountID, region, jobName, mediaURI, languageCode string) (TranscribeJob, error) {
	jobName = strings.TrimSpace(jobName)
	if jobName == "" {
		return TranscribeJob{}, fmt.Errorf("%w: TranscriptionJobName is required", ErrTranscribeBadRequest)
	}
	mediaURI = strings.TrimSpace(mediaURI)
	if mediaURI == "" {
		return TranscribeJob{}, fmt.Errorf("%w: Media.MediaFileUri is required", ErrTranscribeBadRequest)
	}
	bucket, key, err := parseTranscribeS3URI(mediaURI)
	if err != nil {
		return TranscribeJob{}, err
	}
	if _, err := s.HeadObject(accountID, bucket, key); err != nil {
		if errors.Is(err, ErrNoSuchBucket) || errors.Is(err, ErrNoSuchKey) {
			return TranscribeJob{}, fmt.Errorf("%w: Media.MediaFileUri object not found", ErrTranscribeBadRequest)
		}
		return TranscribeJob{}, fmt.Errorf("%w: %v", ErrTranscribeBadRequest, err)
	}
	if languageCode == "" {
		languageCode = "en-US"
	}
	var existing string
	err = s.db.QueryRow(
		`SELECT job_name FROM transcribe_jobs WHERE account_id = ? AND job_name = ?`,
		accountID, jobName,
	).Scan(&existing)
	if err == nil {
		return TranscribeJob{}, ErrTranscribeConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TranscribeJob{}, fmt.Errorf("start transcription job: %w", err)
	}

	if region == "" {
		region = DefaultTranscribeRegion
	}
	dir := filepath.Join(s.dataRoot, "transcribe", accountID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return TranscribeJob{}, fmt.Errorf("start transcription job: mkdir: %w", err)
	}
	transcriptPath := filepath.Join(dir, jobName+".json")
	canned := map[string]any{
		"jobName":        jobName,
		"accountId":      accountID,
		"results": map[string]any{
			"transcripts": []map[string]any{
				{"transcript": "Noctaxris stub transcript."},
			},
			"items": []map[string]any{
				{"type": "pronunciation", "alternatives": []map[string]any{{"confidence": "1.0", "content": "Noctaxris"}}, "start_time": "0.0", "end_time": "0.5"},
				{"type": "pronunciation", "alternatives": []map[string]any{{"confidence": "1.0", "content": "stub"}}, "start_time": "0.5", "end_time": "0.9"},
				{"type": "pronunciation", "alternatives": []map[string]any{{"confidence": "1.0", "content": "transcript"}}, "start_time": "0.9", "end_time": "1.4"},
			},
		},
		"status": "COMPLETED",
	}
	raw, err := json.Marshal(canned)
	if err != nil {
		return TranscribeJob{}, fmt.Errorf("start transcription job: marshal: %w", err)
	}
	if err := os.WriteFile(transcriptPath, raw, 0o640); err != nil {
		return TranscribeJob{}, fmt.Errorf("start transcription job: write transcript: %w", err)
	}
	// Lab URI points at data-root artifact (not a live S3 listener).
	transcriptURI := fmt.Sprintf("file://%s", filepath.ToSlash(transcriptPath))
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO transcribe_jobs
		 (account_id, job_name, job_status, language_code, media_uri, transcript_uri, failure_reason, created_at, completed_at)
		 VALUES (?, ?, 'COMPLETED', ?, ?, ?, '', ?, ?)`,
		accountID, jobName, languageCode, mediaURI, transcriptURI, now, now,
	)
	if err != nil {
		return TranscribeJob{}, fmt.Errorf("start transcription job: insert: %w", err)
	}
	_ = region
	return TranscribeJob{
		JobName: jobName, JobStatus: "COMPLETED", LanguageCode: languageCode,
		MediaURI: mediaURI, TranscriptURI: transcriptURI, CreatedAt: now, CompletedAt: now,
	}, nil
}

// GetTranscriptionJob returns a job by name.
func (s *Store) GetTranscriptionJob(accountID, jobName string) (TranscribeJob, error) {
	var j TranscribeJob
	err := s.db.QueryRow(
		`SELECT job_name, job_status, language_code, media_uri, transcript_uri, failure_reason, created_at, completed_at
		 FROM transcribe_jobs WHERE account_id = ? AND job_name = ?`,
		accountID, jobName,
	).Scan(&j.JobName, &j.JobStatus, &j.LanguageCode, &j.MediaURI, &j.TranscriptURI, &j.FailureReason, &j.CreatedAt, &j.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TranscribeJob{}, ErrTranscribeNotFound
	}
	if err != nil {
		return TranscribeJob{}, fmt.Errorf("get transcription job: %w", err)
	}
	return j, nil
}

// ListTranscriptionJobs lists jobs for an account.
func (s *Store) ListTranscriptionJobs(accountID string) ([]TranscribeJob, error) {
	rows, err := s.db.Query(
		`SELECT job_name, job_status, language_code, media_uri, transcript_uri, failure_reason, created_at, completed_at
		 FROM transcribe_jobs WHERE account_id = ? ORDER BY job_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list transcription jobs: %w", err)
	}
	defer rows.Close()
	out := []TranscribeJob{}
	for rows.Next() {
		var j TranscribeJob
		if err := rows.Scan(&j.JobName, &j.JobStatus, &j.LanguageCode, &j.MediaURI, &j.TranscriptURI, &j.FailureReason, &j.CreatedAt, &j.CompletedAt); err != nil {
			return nil, fmt.Errorf("list transcription jobs: scan: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func parseTranscribeS3URI(uri string) (bucket, key string, err error) {
	const prefix = "s3://"
	if !strings.HasPrefix(strings.ToLower(uri), prefix) {
		return "", "", fmt.Errorf("%w: Media.MediaFileUri must be s3://bucket/key", ErrTranscribeBadRequest)
	}
	rest := uri[len(prefix):]
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 || slash == len(rest)-1 {
		return "", "", fmt.Errorf("%w: Media.MediaFileUri must be s3://bucket/key", ErrTranscribeBadRequest)
	}
	bucket = rest[:slash]
	key = rest[slash+1:]
	if bucket == "" || key == "" {
		return "", "", fmt.Errorf("%w: Media.MediaFileUri must be s3://bucket/key", ErrTranscribeBadRequest)
	}
	return bucket, key, nil
}
