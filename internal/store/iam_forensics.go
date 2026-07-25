package store

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrCredentialReportNotPresent is returned when GetCredentialReport is called before GenerateCredentialReport.
var ErrCredentialReportNotPresent = errors.New("credential report not present")

// AccessKeyLastUsed is IAM GetAccessKeyLastUsed-shaped metadata.
type AccessKeyLastUsed struct {
	UserName     string
	LastUsedDate time.Time
	ServiceName  string
	Region       string
	HasLastUsed  bool
}

// RecordAccessKeyLastUsed updates last-used metadata for a long-lived access key (AKIA*).
func (s *Store) RecordAccessKeyLastUsed(accessKeyID, serviceName, region string, at time.Time) error {
	if !strings.HasPrefix(accessKeyID, "AKIA") {
		return nil
	}
	serviceName = strings.TrimSpace(serviceName)
	region = strings.TrimSpace(region)
	at = at.UTC()
	_, err := s.db.Exec(
		`UPDATE access_keys SET last_used_at = ?, last_used_service = ?, last_used_region = ?
		 WHERE access_key_id = ?`,
		at.Format(time.RFC3339), serviceName, region, accessKeyID,
	)
	if err != nil {
		return fmt.Errorf("record access key last used: %w", err)
	}
	return nil
}

// GetAccessKeyLastUsed returns last-used metadata when the key belongs to accountID.
func (s *Store) GetAccessKeyLastUsed(accountID, accessKeyID string) (AccessKeyLastUsed, error) {
	var (
		userName    sql.NullString
		lastAt      sql.NullString
		serviceName sql.NullString
		region      sql.NullString
	)
	err := s.db.QueryRow(
		`SELECT user_name, last_used_at, last_used_service, last_used_region
		 FROM access_keys WHERE access_key_id = ? AND account_id = ?`,
		accessKeyID, accountID,
	).Scan(&userName, &lastAt, &serviceName, &region)
	if err != nil {
		return AccessKeyLastUsed{}, err
	}
	out := AccessKeyLastUsed{}
	if userName.Valid {
		out.UserName = userName.String
	}
	if lastAt.Valid && strings.TrimSpace(lastAt.String) != "" {
		t, parseErr := time.Parse(time.RFC3339, lastAt.String)
		if parseErr != nil {
			return AccessKeyLastUsed{}, fmt.Errorf("parse last_used_at: %w", parseErr)
		}
		out.LastUsedDate = t.UTC()
		out.HasLastUsed = true
	}
	if serviceName.Valid {
		out.ServiceName = serviceName.String
	}
	if region.Valid {
		out.Region = region.String
	}
	return out, nil
}

// GenerateCredentialReport builds and stores a lab IAM credential report CSV (state COMPLETE).
func (s *Store) GenerateCredentialReport(accountID string, at time.Time) error {
	at = at.UTC()
	csvBytes, err := s.buildCredentialReportCSV(accountID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO iam_credential_reports (account_id, state, generated_at, content_csv)
		 VALUES (?, 'COMPLETE', ?, ?)
		 ON CONFLICT(account_id) DO UPDATE SET
		   state = excluded.state,
		   generated_at = excluded.generated_at,
		   content_csv = excluded.content_csv`,
		accountID, at.Format(time.RFC3339), csvBytes,
	)
	if err != nil {
		return fmt.Errorf("store credential report: %w", err)
	}
	return nil
}

// GetCredentialReport returns the stored credential report for an account.
func (s *Store) GetCredentialReport(accountID string) (content []byte, generated time.Time, state string, err error) {
	var (
		st        string
		gen       string
		csvBlob   []byte
	)
	rowErr := s.db.QueryRow(
		`SELECT state, generated_at, content_csv FROM iam_credential_reports WHERE account_id = ?`,
		accountID,
	).Scan(&st, &gen, &csvBlob)
	if rowErr != nil {
		if errors.Is(rowErr, sql.ErrNoRows) {
			return nil, time.Time{}, "", ErrCredentialReportNotPresent
		}
		return nil, time.Time{}, "", rowErr
	}
	if strings.TrimSpace(st) == "" {
		st = "COMPLETE"
	}
	generated, err = time.Parse(time.RFC3339, gen)
	if err != nil {
		return nil, time.Time{}, "", fmt.Errorf("parse generated_at: %w", err)
	}
	return csvBlob, generated.UTC(), st, nil
}

func (s *Store) buildCredentialReportCSV(accountID string) ([]byte, error) {
	header := []string{
		"user", "arn", "user_creation_time", "password_enabled", "password_last_used",
		"password_last_changed", "password_next_rotation", "mfa_active",
		"access_key_1_active", "access_key_1_last_rotated", "access_key_1_last_used_date",
		"access_key_1_last_used_region", "access_key_1_last_used_service",
		"access_key_2_active", "access_key_2_last_rotated", "access_key_2_last_used_date",
		"access_key_2_last_used_region", "access_key_2_last_used_service",
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		return nil, err
	}

	users, err := s.ListUsers(accountID)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		mfa, err := s.userHasActiveMFA(accountID, u.UserName)
		if err != nil {
			return nil, err
		}
		keys, err := s.listAccessKeysForCredentialReport(accountID, u.UserName)
		if err != nil {
			return nil, err
		}
		row := credentialReportUserRow(u, mfa, keys)
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Store) userHasActiveMFA(accountID, userName string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM iam_mfa_devices WHERE account_id = ? AND user_name = ? AND enabled = 1`,
		accountID, userName,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

type credentialReportKey struct {
	AccessKeyID string
	Status      string
	LastUsed    AccessKeyLastUsed
}

func (s *Store) listAccessKeysForCredentialReport(accountID, userName string) ([]credentialReportKey, error) {
	rows, err := s.db.Query(
		`SELECT access_key_id, COALESCE(NULLIF(status, ''), 'Active'),
		        COALESCE(last_used_at, ''), COALESCE(last_used_service, ''), COALESCE(last_used_region, '')
		 FROM access_keys
		 WHERE account_id = ? AND user_name = ? AND COALESCE(role_arn, '') = ''
		 ORDER BY access_key_id`,
		accountID, userName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []credentialReportKey
	for rows.Next() {
		var (
			k         credentialReportKey
			lastAt    string
			service   string
			region    string
		)
		if err := rows.Scan(&k.AccessKeyID, &k.Status, &lastAt, &service, &region); err != nil {
			return nil, err
		}
		if strings.TrimSpace(lastAt) != "" {
			t, parseErr := time.Parse(time.RFC3339, lastAt)
			if parseErr != nil {
				return nil, fmt.Errorf("parse last_used_at: %w", parseErr)
			}
			k.LastUsed.LastUsedDate = t.UTC()
			k.LastUsed.HasLastUsed = true
		}
		k.LastUsed.ServiceName = service
		k.LastUsed.Region = region
		out = append(out, k)
	}
	return out, rows.Err()
}

func credentialReportUserRow(u User, mfa bool, keys []credentialReportKey) []string {
	mfaVal := "false"
	if mfa {
		mfaVal = "true"
	}
	row := []string{
		u.UserName,
		u.ARN,
		u.CreateDate,
		"false",
		"N/A",
		"N/A",
		"not_supported",
		mfaVal,
	}
	for i := 0; i < 2; i++ {
		if i < len(keys) {
			active := "false"
			if keys[i].Status == AccessKeyStatusActive {
				active = "true"
			}
			lastDate := "N/A"
			lastRegion := "N/A"
			lastService := "N/A"
			if keys[i].LastUsed.HasLastUsed {
				lastDate = keys[i].LastUsed.LastUsedDate.Format(time.RFC3339)
				if keys[i].LastUsed.Region != "" {
					lastRegion = keys[i].LastUsed.Region
				}
				if keys[i].LastUsed.ServiceName != "" {
					lastService = keys[i].LastUsed.ServiceName
				}
			}
			row = append(row, active, "N/A", lastDate, lastRegion, lastService)
		} else {
			row = append(row, "false", "N/A", "N/A", "N/A", "N/A")
		}
	}
	return row
}
