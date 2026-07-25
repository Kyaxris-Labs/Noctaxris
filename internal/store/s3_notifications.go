package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
)

const servicePrincipalS3 = "s3.amazonaws.com"

// ErrInvalidS3NotificationConfiguration is returned for malformed or unauthorized
// PutBucketNotificationConfiguration inputs (AWS InvalidArgument shape).
var ErrInvalidS3NotificationConfiguration = errors.New("InvalidArgument")

// S3NotificationConfig is the lab bucket notification configuration.
// Empty config (default) means notifications are off; object mutations emit only
// when a non-empty config is stored and destination authz still Allows.
type S3NotificationConfig struct {
	LambdaConfigs      []S3LambdaFunctionConfig
	QueueConfigs       []S3QueueConfig
	TopicConfigs       []S3TopicConfig
	EventBridgeEnabled bool
}

// S3LambdaFunctionConfig routes matching S3 events to a Lambda function ARN.
type S3LambdaFunctionConfig struct {
	ID           string
	Events       []string
	FilterPrefix string
	FilterSuffix string
	FunctionARN  string
}

// S3QueueConfig routes matching S3 events to an SQS queue ARN.
type S3QueueConfig struct {
	ID           string
	Events       []string
	FilterPrefix string
	FilterSuffix string
	QueueARN     string
}

// S3TopicConfig routes matching S3 events to an SNS topic ARN.
// Delivery uses in-process Publish; HTTP subscribers stay on the SNS allowlist/catcher.
type S3TopicConfig struct {
	ID           string
	Events       []string
	FilterPrefix string
	FilterSuffix string
	TopicARN     string
}

type s3NotificationPersisted struct {
	LambdaConfigs      []S3LambdaFunctionConfig `json:"lambdaConfigs,omitempty"`
	QueueConfigs       []S3QueueConfig          `json:"queueConfigs,omitempty"`
	TopicConfigs       []S3TopicConfig          `json:"topicConfigs,omitempty"`
	EventBridgeEnabled bool                     `json:"eventBridgeEnabled,omitempty"`
}

var allowedS3NotificationEvents = map[string]struct{}{
	"s3:ObjectCreated:*":                       {},
	"s3:ObjectCreated:Put":                     {},
	"s3:ObjectCreated:CompleteMultipartUpload": {},
	"s3:ObjectRemoved:*":                       {},
	"s3:ObjectRemoved:Delete":                  {},
}

// EnsureS3NotificationsSchema adds notification_json on s3_buckets.
func EnsureS3NotificationsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure s3 notifications schema: db is nil")
	}
	if err := execMigrateStmt(db, `ALTER TABLE s3_buckets ADD COLUMN notification_json TEXT NOT NULL DEFAULT ''`, nil); err != nil {
		return fmt.Errorf("ensure s3 notifications schema: %w", err)
	}
	return nil
}

// EnsureS3NotificationsSchema ensures the notification column on an open store.
func (s *Store) EnsureS3NotificationsSchema() error {
	return EnsureS3NotificationsSchema(s.db)
}

// PutBucketNotificationConfiguration replaces the bucket notification configuration.
// Empty cfg clears notifications (AWS disable shape). Destination existence and
// resource-policy Allow for s3.amazonaws.com + SourceArn are required for
// Lambda/SQS/SNS targets; EventBridgeConfiguration needs no destination check.
func (s *Store) PutBucketNotificationConfiguration(accountID, bucket string, cfg S3NotificationConfig) error {
	if err := s.EnsureS3NotificationsSchema(); err != nil {
		return err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return err
	}
	normalized, err := s.validateS3NotificationConfig(accountID, bucket, cfg)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(s3NotificationPersisted{
		LambdaConfigs:      normalized.LambdaConfigs,
		QueueConfigs:       normalized.QueueConfigs,
		TopicConfigs:       normalized.TopicConfigs,
		EventBridgeEnabled: normalized.EventBridgeEnabled,
	})
	if err != nil {
		return fmt.Errorf("put bucket notification: marshal: %w", err)
	}
	stored := string(raw)
	if isEmptyS3NotificationConfig(normalized) {
		stored = ""
	}
	_, err = s.db.Exec(
		`UPDATE s3_buckets SET notification_json = ? WHERE account_id = ? AND name = ?`,
		stored, accountID, bucket,
	)
	if err != nil {
		return fmt.Errorf("put bucket notification: %w", err)
	}
	return nil
}

// GetBucketNotificationConfiguration returns the stored config, or empty when unset.
func (s *Store) GetBucketNotificationConfiguration(accountID, bucket string) (S3NotificationConfig, error) {
	if err := s.EnsureS3NotificationsSchema(); err != nil {
		return S3NotificationConfig{}, err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return S3NotificationConfig{}, err
	}
	var raw string
	err := s.db.QueryRow(
		`SELECT COALESCE(notification_json, '') FROM s3_buckets WHERE account_id = ? AND name = ?`,
		accountID, bucket,
	).Scan(&raw)
	if err != nil {
		return S3NotificationConfig{}, fmt.Errorf("get bucket notification: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return S3NotificationConfig{}, nil
	}
	var persisted s3NotificationPersisted
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
		return S3NotificationConfig{}, fmt.Errorf("get bucket notification: %w", err)
	}
	return S3NotificationConfig{
		LambdaConfigs:      persisted.LambdaConfigs,
		QueueConfigs:       persisted.QueueConfigs,
		TopicConfigs:       persisted.TopicConfigs,
		EventBridgeEnabled: persisted.EventBridgeEnabled,
	}, nil
}

func isEmptyS3NotificationConfig(cfg S3NotificationConfig) bool {
	return len(cfg.LambdaConfigs) == 0 &&
		len(cfg.QueueConfigs) == 0 &&
		len(cfg.TopicConfigs) == 0 &&
		!cfg.EventBridgeEnabled
}

func (s *Store) validateS3NotificationConfig(accountID, bucket string, cfg S3NotificationConfig) (S3NotificationConfig, error) {
	out := S3NotificationConfig{EventBridgeEnabled: cfg.EventBridgeEnabled}
	bucketARN := BucketARN(bucket)

	for i, lc := range cfg.LambdaConfigs {
		norm, err := normalizeS3LambdaConfig(lc, i)
		if err != nil {
			return S3NotificationConfig{}, err
		}
		if err := s.validateS3NotificationLambdaDestination(accountID, bucketARN, norm.FunctionARN); err != nil {
			return S3NotificationConfig{}, err
		}
		out.LambdaConfigs = append(out.LambdaConfigs, norm)
	}
	for i, qc := range cfg.QueueConfigs {
		norm, err := normalizeS3QueueConfig(qc, i)
		if err != nil {
			return S3NotificationConfig{}, err
		}
		if err := s.validateS3NotificationQueueDestination(accountID, bucketARN, norm.QueueARN); err != nil {
			return S3NotificationConfig{}, err
		}
		out.QueueConfigs = append(out.QueueConfigs, norm)
	}
	for i, tc := range cfg.TopicConfigs {
		norm, err := normalizeS3TopicConfig(tc, i)
		if err != nil {
			return S3NotificationConfig{}, err
		}
		if err := s.validateS3NotificationTopicDestination(accountID, bucketARN, norm.TopicARN); err != nil {
			return S3NotificationConfig{}, err
		}
		out.TopicConfigs = append(out.TopicConfigs, norm)
	}
	return out, nil
}

func normalizeS3LambdaConfig(cfg S3LambdaFunctionConfig, idx int) (S3LambdaFunctionConfig, error) {
	arn := strings.TrimSpace(cfg.FunctionARN)
	if err := validateS3NotificationLambdaARN(arn); err != nil {
		return S3LambdaFunctionConfig{}, err
	}
	events, err := validateS3NotificationEvents(cfg.Events)
	if err != nil {
		return S3LambdaFunctionConfig{}, err
	}
	id := strings.TrimSpace(cfg.ID)
	if id == "" {
		id = fmt.Sprintf("lambda-%d", idx+1)
	}
	return S3LambdaFunctionConfig{
		ID:           id,
		Events:       events,
		FilterPrefix: cfg.FilterPrefix,
		FilterSuffix: cfg.FilterSuffix,
		FunctionARN:  arn,
	}, nil
}

func normalizeS3QueueConfig(cfg S3QueueConfig, idx int) (S3QueueConfig, error) {
	arn := strings.TrimSpace(cfg.QueueARN)
	if err := validateS3NotificationQueueARN(arn); err != nil {
		return S3QueueConfig{}, err
	}
	events, err := validateS3NotificationEvents(cfg.Events)
	if err != nil {
		return S3QueueConfig{}, err
	}
	id := strings.TrimSpace(cfg.ID)
	if id == "" {
		id = fmt.Sprintf("queue-%d", idx+1)
	}
	return S3QueueConfig{
		ID:           id,
		Events:       events,
		FilterPrefix: cfg.FilterPrefix,
		FilterSuffix: cfg.FilterSuffix,
		QueueARN:     arn,
	}, nil
}

func normalizeS3TopicConfig(cfg S3TopicConfig, idx int) (S3TopicConfig, error) {
	arn := strings.TrimSpace(cfg.TopicARN)
	if err := validateS3NotificationTopicARN(arn); err != nil {
		return S3TopicConfig{}, err
	}
	events, err := validateS3NotificationEvents(cfg.Events)
	if err != nil {
		return S3TopicConfig{}, err
	}
	id := strings.TrimSpace(cfg.ID)
	if id == "" {
		id = fmt.Sprintf("topic-%d", idx+1)
	}
	return S3TopicConfig{
		ID:           id,
		Events:       events,
		FilterPrefix: cfg.FilterPrefix,
		FilterSuffix: cfg.FilterSuffix,
		TopicARN:     arn,
	}, nil
}

func validateS3NotificationEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("%w: at least one Event is required", ErrInvalidS3NotificationConfiguration)
	}
	out := make([]string, 0, len(events))
	seen := map[string]struct{}{}
	for _, ev := range events {
		ev = strings.TrimSpace(ev)
		if ev == "" {
			continue
		}
		if _, ok := allowedS3NotificationEvents[ev]; !ok {
			return nil, fmt.Errorf("%w: unsupported event %q", ErrInvalidS3NotificationConfiguration, ev)
		}
		if _, dup := seen[ev]; dup {
			continue
		}
		seen[ev] = struct{}{}
		out = append(out, ev)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one Event is required", ErrInvalidS3NotificationConfiguration)
	}
	return out, nil
}

func validateS3NotificationLambdaARN(arn string) error {
	if looksLikeOpenProxyDestination(arn) {
		return fmt.Errorf("%w: LambdaFunctionArn must be a lambda ARN", ErrInvalidS3NotificationConfiguration)
	}
	parts := strings.Split(arn, ":")
	// arn:aws:lambda:region:account:function:name
	if len(parts) < 7 || parts[0] != "arn" || parts[1] != "aws" || parts[2] != "lambda" || parts[5] != "function" {
		return fmt.Errorf("%w: LambdaFunctionArn must be a lambda ARN", ErrInvalidS3NotificationConfiguration)
	}
	if strings.TrimSpace(parts[3]) == "" || len(parts[4]) != 12 || strings.TrimSpace(parts[6]) == "" {
		return fmt.Errorf("%w: LambdaFunctionArn must be a lambda ARN", ErrInvalidS3NotificationConfiguration)
	}
	return nil
}

func validateS3NotificationQueueARN(arn string) error {
	if looksLikeOpenProxyDestination(arn) {
		return fmt.Errorf("%w: QueueArn must be an sqs ARN", ErrInvalidS3NotificationConfiguration)
	}
	parts := strings.Split(arn, ":")
	if len(parts) != 6 || parts[0] != "arn" || parts[1] != "aws" || parts[2] != "sqs" {
		return fmt.Errorf("%w: QueueArn must be an sqs ARN", ErrInvalidS3NotificationConfiguration)
	}
	if strings.TrimSpace(parts[3]) == "" || len(parts[4]) != 12 || strings.TrimSpace(parts[5]) == "" {
		return fmt.Errorf("%w: QueueArn must be an sqs ARN", ErrInvalidS3NotificationConfiguration)
	}
	return nil
}

func validateS3NotificationTopicARN(arn string) error {
	if looksLikeOpenProxyDestination(arn) {
		return fmt.Errorf("%w: TopicArn must be an sns ARN", ErrInvalidS3NotificationConfiguration)
	}
	parts := strings.Split(arn, ":")
	if len(parts) != 6 || parts[0] != "arn" || parts[1] != "aws" || parts[2] != "sns" {
		return fmt.Errorf("%w: TopicArn must be an sns ARN", ErrInvalidS3NotificationConfiguration)
	}
	if strings.TrimSpace(parts[3]) == "" || len(parts[4]) != 12 || strings.TrimSpace(parts[5]) == "" {
		return fmt.Errorf("%w: TopicArn must be an sns ARN", ErrInvalidS3NotificationConfiguration)
	}
	return nil
}

func looksLikeOpenProxyDestination(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "ftp://") ||
		strings.Contains(lower, "://")
}

func (s *Store) validateS3NotificationLambdaDestination(bucketAccount, bucketARN, functionARN string) error {
	owner := resourceOwnerAccountFromARN(functionARN)
	if owner == "" {
		return fmt.Errorf("%w: unable to validate lambda destination", ErrInvalidS3NotificationConfiguration)
	}
	name, _ := ParseFunctionQualifier(functionARN)
	if name == "" {
		return fmt.Errorf("%w: unable to validate lambda destination", ErrInvalidS3NotificationConfiguration)
	}
	if _, err := s.GetFunction(owner, name); err != nil {
		if errors.Is(err, ErrNoSuchFunction) {
			return fmt.Errorf("%w: lambda destination does not exist", ErrInvalidS3NotificationConfiguration)
		}
		return fmt.Errorf("%w: unable to validate lambda destination: %v", ErrInvalidS3NotificationConfiguration, err)
	}
	if !s.s3NotificationDestinationAuthorized(bucketAccount, bucketARN, functionARN, actionLambdaInvokeFunction) {
		return fmt.Errorf("%w: unable to validate the following destination configurations", ErrInvalidS3NotificationConfiguration)
	}
	return nil
}

func (s *Store) validateS3NotificationQueueDestination(bucketAccount, bucketARN, queueARN string) error {
	owner, err := queueAccountFromARN(queueARN)
	if err != nil {
		return fmt.Errorf("%w: unable to validate queue destination", ErrInvalidS3NotificationConfiguration)
	}
	name, err := queueNameFromARN(queueARN)
	if err != nil {
		return fmt.Errorf("%w: unable to validate queue destination", ErrInvalidS3NotificationConfiguration)
	}
	if _, err := s.GetQueue(owner, name); err != nil {
		return fmt.Errorf("%w: queue destination does not exist", ErrInvalidS3NotificationConfiguration)
	}
	if !s.s3NotificationDestinationAuthorized(bucketAccount, bucketARN, queueARN, actionSQSSendMessage) {
		return fmt.Errorf("%w: unable to validate the following destination configurations", ErrInvalidS3NotificationConfiguration)
	}
	return nil
}

func (s *Store) validateS3NotificationTopicDestination(bucketAccount, bucketARN, topicARN string) error {
	if _, err := s.GetTopicByARN(topicARN); err != nil {
		if errors.Is(err, ErrNoSuchTopic) {
			return fmt.Errorf("%w: topic destination does not exist", ErrInvalidS3NotificationConfiguration)
		}
		return fmt.Errorf("%w: unable to validate topic destination: %v", ErrInvalidS3NotificationConfiguration, err)
	}
	if !s.s3NotificationDestinationAuthorized(bucketAccount, bucketARN, topicARN, actionSNSPublish) {
		return fmt.Errorf("%w: unable to validate the following destination configurations", ErrInvalidS3NotificationConfiguration)
	}
	return nil
}

// s3NotificationDestinationAuthorized checks destination resource policy Allow for
// s3.amazonaws.com. Bucket ARNs have no account segment, so aws:SourceAccount is
// taken from the bucket owner (required for Lambda AddPermission SourceAccount locks).
func (s *Store) s3NotificationDestinationAuthorized(bucketAccount, bucketARN, targetARN, action string) bool {
	policyAccount := bucketAccount
	if owner := resourceOwnerAccountFromARN(targetARN); owner != "" {
		policyAccount = owner
	}
	policyDoc, err := s.deliveryTargetResourcePolicyDoc(policyAccount, targetARN)
	if err != nil {
		return false
	}
	keys := authz.DeliverySourceConditionKeys(bucketARN, bucketAccount)
	return authz.EventTargetResourcePolicyAllows(
		policyDoc,
		action,
		targetARN,
		servicePrincipalS3,
		policyAccount,
		keys,
	)
}
