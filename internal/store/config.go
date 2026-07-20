package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrConfigBadRequest = errors.New("InvalidParameterValueException")
	ErrConfigNotFound   = errors.New("NoSuchConfigurationRecorderException")
)

const DefaultConfigRegion = "us-east-1"

const configSchema = `
CREATE TABLE IF NOT EXISTS config_recorders (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  recording_group TEXT NOT NULL DEFAULT 'ALL',
  recording INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS config_delivery_channels (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  s3_bucket_name TEXT NOT NULL,
  s3_key_prefix TEXT NOT NULL DEFAULT '',
  sns_topic_arn TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS config_rules (
  account_id TEXT NOT NULL,
  rule_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, rule_name)
);
`

// ConfigRecorder is a configuration recorder row.
type ConfigRecorder struct {
	Name           string
	RoleARN        string
	RecordingGroup string
	Recording      bool
	CreatedAt      int64
}

// ConfigDeliveryChannel is a delivery channel row.
type ConfigDeliveryChannel struct {
	Name         string
	S3BucketName string
	S3KeyPrefix  string
	SNSTopicARN  string
	CreatedAt    int64
}

// ConfigComplianceResult is a stub compliance row over tagged resources.
type ConfigComplianceResult struct {
	ConfigRuleName     string
	ComplianceType     string // COMPLIANT, NON_COMPLIANT, NOT_APPLICABLE
	ResourceType       string
	ResourceID         string
	Annotation         string
}

// EnsureConfigSchema creates AWS Config tables if missing.
func EnsureConfigSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure config schema: db is nil")
	}
	if _, err := db.Exec(configSchema); err != nil {
		return fmt.Errorf("ensure config schema: %w", err)
	}
	return nil
}

// EnsureConfigSchema ensures Config tables on an open store.
func (s *Store) EnsureConfigSchema() error {
	return EnsureConfigSchema(s.db)
}

// PutConfigRecorder upserts a configuration recorder.
func (s *Store) PutConfigRecorder(accountID, name, roleARN, recordingGroup string) (ConfigRecorder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ConfigRecorder{}, fmt.Errorf("%w: name required", ErrConfigBadRequest)
	}
	if recordingGroup == "" {
		recordingGroup = "ALL"
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO config_recorders (account_id, name, role_arn, recording_group, recording, created_at)
		 VALUES (?, ?, ?, ?, 0, ?)
		 ON CONFLICT(account_id, name) DO UPDATE SET
		   role_arn = excluded.role_arn,
		   recording_group = excluded.recording_group`,
		accountID, name, roleARN, recordingGroup, now,
	)
	if err != nil {
		return ConfigRecorder{}, fmt.Errorf("put configuration recorder: %w", err)
	}
	return ConfigRecorder{Name: name, RoleARN: roleARN, RecordingGroup: recordingGroup, CreatedAt: now}, nil
}

// PutConfigDeliveryChannel upserts a delivery channel.
func (s *Store) PutConfigDeliveryChannel(accountID, name, bucket, prefix, snsARN string) (ConfigDeliveryChannel, error) {
	name = strings.TrimSpace(name)
	bucket = strings.TrimSpace(bucket)
	if name == "" || bucket == "" {
		return ConfigDeliveryChannel{}, fmt.Errorf("%w: name and s3BucketName required", ErrConfigBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO config_delivery_channels (account_id, name, s3_bucket_name, s3_key_prefix, sns_topic_arn, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, name) DO UPDATE SET
		   s3_bucket_name = excluded.s3_bucket_name,
		   s3_key_prefix = excluded.s3_key_prefix,
		   sns_topic_arn = excluded.sns_topic_arn`,
		accountID, name, bucket, prefix, snsARN, now,
	)
	if err != nil {
		return ConfigDeliveryChannel{}, fmt.Errorf("put delivery channel: %w", err)
	}
	return ConfigDeliveryChannel{Name: name, S3BucketName: bucket, S3KeyPrefix: prefix, SNSTopicARN: snsARN, CreatedAt: now}, nil
}

// StartConfigRecorder marks a recorder as recording.
func (s *Store) StartConfigRecorder(accountID, name string) error {
	name = strings.TrimSpace(name)
	res, err := s.db.Exec(
		`UPDATE config_recorders SET recording = 1 WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("start configuration recorder: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrConfigNotFound
	}
	return nil
}

// GetConfigRecorder returns a recorder by name.
func (s *Store) GetConfigRecorder(accountID, name string) (ConfigRecorder, error) {
	var r ConfigRecorder
	var recording int
	err := s.db.QueryRow(
		`SELECT name, role_arn, recording_group, recording, created_at FROM config_recorders
		 WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&r.Name, &r.RoleARN, &r.RecordingGroup, &recording, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ConfigRecorder{}, ErrConfigNotFound
	}
	if err != nil {
		return ConfigRecorder{}, fmt.Errorf("get configuration recorder: %w", err)
	}
	r.Recording = recording == 1
	return r, nil
}

// DescribeConfigComplianceByRule stubs compliance over tagged resources.
// Resources with any tags are COMPLIANT. Untagged ARNs passed in resourceIDs are NON_COMPLIANT.
func (s *Store) DescribeConfigComplianceByRule(accountID, ruleName string) ([]ConfigComplianceResult, error) {
	if ruleName == "" {
		ruleName = "lab-tagged-resources"
	}
	tagged, err := s.GetResources(accountID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("compliance get resources: %w", err)
	}
	var out []ConfigComplianceResult
	for _, tr := range tagged {
		rtype, rid := splitResourceARN(tr.ResourceARN)
		ctype := "COMPLIANT"
		if len(tr.Tags) == 0 {
			ctype = "NON_COMPLIANT"
		}
		out = append(out, ConfigComplianceResult{
			ConfigRuleName: ruleName,
			ComplianceType: ctype,
			ResourceType:   rtype,
			ResourceID:     rid,
			Annotation:     "lab stub over Tagging API resources",
		})
	}
	if len(out) == 0 {
		out = append(out, ConfigComplianceResult{
			ConfigRuleName: ruleName,
			ComplianceType: "NOT_APPLICABLE",
			ResourceType:   "AWS::Tagging::Resource",
			ResourceID:     "none",
			Annotation:     "no tagged resources in lab account",
		})
	}
	return out, nil
}

func splitResourceARN(arn string) (resourceType, resourceID string) {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 {
		return "Unknown", arn
	}
	rest := parts[5]
	if i := strings.Index(rest, "/"); i >= 0 {
		return parts[2], rest[i+1:]
	}
	return parts[2], rest
}
