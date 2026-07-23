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
// The lab S3 bucket must exist (fail closed); Config does not create buckets.
func (s *Store) PutConfigDeliveryChannel(accountID, name, bucket, prefix, snsARN string) (ConfigDeliveryChannel, error) {
	name = strings.TrimSpace(name)
	bucket = strings.TrimSpace(bucket)
	if name == "" || bucket == "" {
		return ConfigDeliveryChannel{}, fmt.Errorf("%w: name and s3BucketName required", ErrConfigBadRequest)
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		if errors.Is(err, ErrNoSuchBucket) {
			return ConfigDeliveryChannel{}, fmt.Errorf("%w: s3BucketName %q does not exist", ErrConfigBadRequest, bucket)
		}
		return ConfigDeliveryChannel{}, fmt.Errorf("put delivery channel: get bucket: %w", err)
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

// ListConfigDeliveryChannels returns delivery channels for an account.
func (s *Store) ListConfigDeliveryChannels(accountID string) ([]ConfigDeliveryChannel, error) {
	rows, err := s.db.Query(
		`SELECT name, s3_bucket_name, s3_key_prefix, sns_topic_arn, created_at
		 FROM config_delivery_channels WHERE account_id = ? ORDER BY created_at, name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list config delivery channels: %w", err)
	}
	defer rows.Close()
	var out []ConfigDeliveryChannel
	for rows.Next() {
		var ch ConfigDeliveryChannel
		if err := rows.Scan(&ch.Name, &ch.S3BucketName, &ch.S3KeyPrefix, &ch.SNSTopicARN, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("list config delivery channels scan: %w", err)
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// NotifyConfigDeliveryChannelsSNS publishes a lab start notification to each delivery channel SNS topic (best-effort).
func (s *Store) NotifyConfigDeliveryChannelsSNS(accountID, recorderName string) {
	channels, err := s.ListConfigDeliveryChannels(accountID)
	if err != nil {
		return
	}
	// Honest lab signal: recorder started. No config history PutObject is performed.
	msg := fmt.Sprintf(
		`{"messageType":"ConfigurationRecorderStarted","configurationRecorderName":"%s"}`,
		recorderName,
	)
	for _, ch := range channels {
		arn := strings.TrimSpace(ch.SNSTopicARN)
		if arn == "" {
			continue
		}
		topic, err := s.GetTopicByARN(arn)
		if err != nil || topic.AccountID != accountID {
			continue
		}
		_, _ = s.Publish(accountID, topic.TopicName, msg, "AWS Config Notification", nil)
	}
}

// StartConfigRecorder marks a recorder as recording when a delivery channel exists.
// No configuration history objects are written to S3 (control-plane flag only).
func (s *Store) StartConfigRecorder(accountID, name string) error {
	name = strings.TrimSpace(name)
	channels, err := s.ListConfigDeliveryChannels(accountID)
	if err != nil {
		return fmt.Errorf("start configuration recorder: %w", err)
	}
	if len(channels) == 0 {
		return fmt.Errorf("%w: a delivery channel is required before starting the recorder", ErrConfigBadRequest)
	}
	for _, ch := range channels {
		if _, err := s.GetBucket(accountID, ch.S3BucketName); err != nil {
			if errors.Is(err, ErrNoSuchBucket) {
				return fmt.Errorf("%w: delivery channel bucket %q does not exist", ErrConfigBadRequest, ch.S3BucketName)
			}
			return fmt.Errorf("start configuration recorder: get bucket: %w", err)
		}
	}
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

// PutConfigRule stores a config rule name for compliance describe honesty (lab lite).
func (s *Store) PutConfigRule(accountID, ruleName, description string) error {
	ruleName = strings.TrimSpace(ruleName)
	if ruleName == "" {
		return fmt.Errorf("%w: ConfigRuleName required", ErrConfigBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO config_rules (account_id, rule_name, description, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(account_id, rule_name) DO UPDATE SET description = excluded.description`,
		accountID, ruleName, description, now,
	)
	if err != nil {
		return fmt.Errorf("put config rule: %w", err)
	}
	return nil
}

// DescribeConfigComplianceByRule returns NOT_APPLICABLE only for stored config_rules rows.
// Unknown rule names yield an empty list (no invented compliance theatre).
func (s *Store) DescribeConfigComplianceByRule(accountID, ruleName string) ([]ConfigComplianceResult, error) {
	ruleName = strings.TrimSpace(ruleName)
	if ruleName == "" {
		rows, err := s.db.Query(
			`SELECT rule_name FROM config_rules WHERE account_id = ? ORDER BY rule_name`,
			accountID,
		)
		if err != nil {
			return nil, fmt.Errorf("describe compliance: %w", err)
		}
		defer rows.Close()
		var out []ConfigComplianceResult
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			out = append(out, ConfigComplianceResult{
				ConfigRuleName: name,
				ComplianceType: "NOT_APPLICABLE",
				ResourceType:   "AWS::Config::ConfigRule",
				ResourceID:     name,
				Annotation:     "rule evaluation not implemented",
			})
		}
		return out, rows.Err()
	}
	var exists string
	err := s.db.QueryRow(
		`SELECT rule_name FROM config_rules WHERE account_id = ? AND rule_name = ?`,
		accountID, ruleName,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return []ConfigComplianceResult{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("describe compliance: %w", err)
	}
	return []ConfigComplianceResult{{
		ConfigRuleName: ruleName,
		ComplianceType: "NOT_APPLICABLE",
		ResourceType:   "AWS::Config::ConfigRule",
		ResourceID:     ruleName,
		Annotation:     "rule evaluation not implemented",
	}}, nil
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
