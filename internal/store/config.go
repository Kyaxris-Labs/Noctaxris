package store

import (
	"database/sql"
	"encoding/json"
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
CREATE TABLE IF NOT EXISTS config_resource_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  capture_time_ms INTEGER NOT NULL,
  configuration_item_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_config_resource_history_lookup
  ON config_resource_history (account_id, resource_type, resource_id, capture_time_ms);
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
	// Honest lab signal: recorder started (not a full AWS Config history stream claim).
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

// ConfigSnapshotLite is a minimal JSON snapshot written on recorder start (not full AWS Config schema).
type ConfigSnapshotLite struct {
	AccountID                 string   `json:"accountId"`
	ConfigurationRecorderName string   `json:"configurationRecorderName"`
	SnapshotTimeMillis        int64    `json:"snapshotTimeMillis"`
	S3BucketNames             []string `json:"s3BucketNames"`
	SQSQueueNames             []string `json:"sqsQueueNames"`
}

func normalizeConfigS3KeyPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimPrefix(prefix, "/")
	if prefix == "" {
		return ""
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

// ConfigHistoryObjectKey returns the lab-shaped S3 key for a configuration snapshot object.
func ConfigHistoryObjectKey(accountID, keyPrefix, recorderName string, snapshotTimeMillis int64) string {
	prefix := normalizeConfigS3KeyPrefix(keyPrefix)
	safeRecorder := strings.ReplaceAll(strings.TrimSpace(recorderName), "/", "-")
	return fmt.Sprintf("%sAWSLogs/%s/Config/noctaxris-config-snapshot-%s-%d.json",
		prefix, accountID, safeRecorder, snapshotTimeMillis)
}

func (s *Store) buildConfigSnapshotLite(accountID, recorderName string, capturedAt int64) (ConfigSnapshotLite, error) {
	buckets, err := s.ListBuckets(accountID)
	if err != nil {
		return ConfigSnapshotLite{}, fmt.Errorf("build config snapshot: list buckets: %w", err)
	}
	queues, err := s.ListQueues(accountID, "")
	if err != nil {
		return ConfigSnapshotLite{}, fmt.Errorf("build config snapshot: list queues: %w", err)
	}
	snap := ConfigSnapshotLite{
		AccountID:                 accountID,
		ConfigurationRecorderName: recorderName,
		SnapshotTimeMillis:        capturedAt,
		S3BucketNames:             make([]string, 0, len(buckets)),
		SQSQueueNames:             make([]string, 0, len(queues)),
	}
	for _, b := range buckets {
		snap.S3BucketNames = append(snap.S3BucketNames, b.Name)
	}
	for _, q := range queues {
		snap.SQSQueueNames = append(snap.SQSQueueNames, q.QueueName)
	}
	return snap, nil
}

func (s *Store) putConfigHistorySnapshots(accountID, recorderName string, channels []ConfigDeliveryChannel, capturedAt int64) error {
	snap, err := s.buildConfigSnapshotLite(accountID, recorderName, capturedAt)
	if err != nil {
		return err
	}
	body, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("put config history: marshal snapshot: %w", err)
	}
	for _, ch := range channels {
		key := ConfigHistoryObjectKey(accountID, ch.S3KeyPrefix, recorderName, capturedAt)
		if _, err := s.PutObject(accountID, ch.S3BucketName, key, PutObjectMeta{
			Data:        body,
			PlainSize:   int64(len(body)),
			ContentType: "application/json",
		}); err != nil {
			return fmt.Errorf("put config history: %w", err)
		}
	}
	return nil
}

// StartConfigRecorder writes a configuration snapshot to each delivery channel S3 bucket, then sets recording=1.
func (s *Store) StartConfigRecorder(accountID, name string) error {
	name = strings.TrimSpace(name)
	if _, err := s.GetConfigRecorder(accountID, name); err != nil {
		return err
	}
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
	capturedAt := time.Now().UTC().UnixMilli()
	if err := s.putConfigHistorySnapshots(accountID, name, channels, capturedAt); err != nil {
		return fmt.Errorf("start configuration recorder: %w", err)
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

const (
	configConfigurationItemVersion = "1.3"
	ConfigItemStatusOK              = "OK"
	ConfigItemStatusResourceDeleted = "ResourceDeleted"
)

// ConfigConfigurationItem is a lab-shaped AWS Config configuration item.
type ConfigConfigurationItem struct {
	ConfigurationItemVersion     string `json:"configurationItemVersion"`
	ConfigurationItemCaptureTime string `json:"configurationItemCaptureTime"`
	ConfigurationItemStatus      string `json:"configurationItemStatus"`
	ResourceType                 string `json:"resourceType"`
	ResourceID                   string `json:"resourceId"`
	ResourceName                 string `json:"resourceName"`
	AWSAccountID                 string `json:"awsAccountId"`
	Configuration                string `json:"configuration,omitempty"`
}

// AccountConfigRecording is true when any configuration recorder for the account has recording enabled.
func (s *Store) AccountConfigRecording(accountID string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM config_recorders WHERE account_id = ? AND recording = 1`,
		accountID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("account config recording: %w", err)
	}
	return n > 0, nil
}

// AppendConfigHistoryItem stores a configuration item when a recorder is actively recording.
// No-op (nil) when recording is off.
func (s *Store) AppendConfigHistoryItem(accountID string, item ConfigConfigurationItem) error {
	recording, err := s.AccountConfigRecording(accountID)
	if err != nil {
		return err
	}
	if !recording {
		return nil
	}
	if strings.TrimSpace(item.ResourceType) == "" || strings.TrimSpace(item.ResourceID) == "" {
		return fmt.Errorf("%w: resourceType and resourceId required", ErrConfigBadRequest)
	}
	if item.ConfigurationItemVersion == "" {
		item.ConfigurationItemVersion = configConfigurationItemVersion
	}
	if item.ConfigurationItemCaptureTime == "" {
		item.ConfigurationItemCaptureTime = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if item.AWSAccountID == "" {
		item.AWSAccountID = accountID
	}
	if item.ResourceName == "" {
		item.ResourceName = item.ResourceID
	}
	body, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("append config history: marshal: %w", err)
	}
	captureMs := time.Now().UTC().UnixMilli()
	if t, err := time.Parse(time.RFC3339Nano, item.ConfigurationItemCaptureTime); err == nil {
		captureMs = t.UnixMilli()
	} else if t, err := time.Parse(time.RFC3339, item.ConfigurationItemCaptureTime); err == nil {
		captureMs = t.UnixMilli()
	}
	_, err = s.db.Exec(
		`INSERT INTO config_resource_history (account_id, resource_type, resource_id, capture_time_ms, configuration_item_json)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, item.ResourceType, item.ResourceID, captureMs, string(body),
	)
	if err != nil {
		return fmt.Errorf("append config history: %w", err)
	}
	return nil
}

// AppendS3BucketConfigHistory records bucket create (deleted=false) or delete (deleted=true) when recording.
func (s *Store) AppendS3BucketConfigHistory(accountID, bucketName string, deleted bool) error {
	bucketName = strings.TrimSpace(bucketName)
	if bucketName == "" {
		return nil
	}
	status := ConfigItemStatusOK
	cfg := map[string]any{"name": bucketName}
	if deleted {
		status = ConfigItemStatusResourceDeleted
	} else if b, err := s.GetBucket(accountID, bucketName); err == nil {
		cfg["creationDate"] = b.CreationDate
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("append s3 bucket config history: %w", err)
	}
	return s.AppendConfigHistoryItem(accountID, ConfigConfigurationItem{
		ConfigurationItemStatus: status,
		ResourceType:            "AWS::S3::Bucket",
		ResourceID:              bucketName,
		ResourceName:            bucketName,
		Configuration:           string(cfgJSON),
	})
}

// GetResourceConfigHistory returns configuration items in chronological order (oldest first).
func (s *Store) GetResourceConfigHistory(accountID, resourceType, resourceID string, limit int) ([]ConfigConfigurationItem, error) {
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if resourceType == "" || resourceID == "" {
		return nil, fmt.Errorf("%w: resourceType and resourceId required", ErrConfigBadRequest)
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT configuration_item_json FROM config_resource_history
		 WHERE account_id = ? AND resource_type = ? AND resource_id = ?
		 ORDER BY capture_time_ms ASC, id ASC
		 LIMIT ?`,
		accountID, resourceType, resourceID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get resource config history: %w", err)
	}
	defer rows.Close()
	var out []ConfigConfigurationItem
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("get resource config history scan: %w", err)
		}
		var item ConfigConfigurationItem
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, fmt.Errorf("get resource config history unmarshal: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
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
