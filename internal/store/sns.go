package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/google/uuid"
)

const (
	// DefaultSNSRegion is the lab region embedded in SNS ARNs.
	DefaultSNSRegion = "us-east-1"
)

var (
	ErrTopicAlreadyExists        = errors.New("TopicAlreadyExists")
	ErrNoSuchTopic               = errors.New("NotFound")
	ErrNoSuchSubscription        = errors.New("SubscriptionNotFound")
	ErrNoSuchSNSMessage          = errors.New("MessageNotFound")
	ErrSNSPolicyStatementExists  = errors.New("ResourceConflictException: statement id already exists")
	ErrSNSInvalidParameter       = errors.New("InvalidParameter")
	ErrSNSEndpointNotAllowed     = errors.New("InvalidParameter: HTTP endpoint not allowlisted")
	ErrSNSMissingMessageGroupID  = errors.New("InvalidParameter: MessageGroupId required for FIFO topic")
	ErrSNSMissingDeduplicationID = errors.New("InvalidParameter: MessageDeduplicationId required for FIFO topic")
)

// Topic is an SNS topic metadata row.
type Topic struct {
	AccountID    string
	TopicName    string
	TopicARN     string
	Policy       string
	Attributes   map[string]string
	CreationDate string
}

// Subscription is an SNS subscription row.
type Subscription struct {
	SubscriptionARN string
	TopicARN        string
	Protocol        string
	Endpoint        string
	Confirmed       bool
	Owner           string
	ConfirmToken    string
}

// PublishResult is the outcome of Publish (message id for fan-out and delivery).
type PublishResult struct {
	MessageID string
}

// PublishOpts carries optional FIFO publish parameters.
type PublishOpts struct {
	MessageGroupID         string
	MessageDeduplicationID string
}

// PublishedMessage is a persisted publish metadata row.
type PublishedMessage struct {
	MessageID              string
	TopicARN               string
	Body                   string
	Subject                string
	Attributes             map[string]string
	CreatedAt              string
	MessageGroupID         string
	MessageDeduplicationID string
}

// snsSchema creates tables and indexes that do not depend on migrated columns.
// FIFO columns and their indexes are applied after ALTER so old volumes upgrade cleanly.
const snsSchema = `
CREATE TABLE IF NOT EXISTS sns_topics (
  account_id TEXT NOT NULL,
  topic_name TEXT NOT NULL,
  topic_arn TEXT NOT NULL,
  policy_json TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  creation_date TEXT NOT NULL,
  PRIMARY KEY (account_id, topic_name)
);
CREATE TABLE IF NOT EXISTS sns_subscriptions (
  subscription_arn TEXT PRIMARY KEY,
  topic_arn TEXT NOT NULL,
  protocol TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  confirmed INTEGER NOT NULL DEFAULT 0,
  owner TEXT NOT NULL DEFAULT '',
  confirm_token TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS sns_published_messages (
  message_id TEXT PRIMARY KEY,
  topic_arn TEXT NOT NULL,
  body TEXT NOT NULL,
  subject TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  message_group_id TEXT NOT NULL DEFAULT '',
  message_deduplication_id TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS sns_http_catcher (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  subscription_arn TEXT NOT NULL DEFAULT '',
  topic_arn TEXT NOT NULL DEFAULT '',
  message_type TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  received_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sns_subscriptions_topic ON sns_subscriptions(topic_arn);
`

// EnsureSNSSchema creates SNS tables if missing and migrates FIFO columns before indexes.
func EnsureSNSSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure sns schema: db is nil")
	}
	if _, err := db.Exec(snsSchema); err != nil {
		return fmt.Errorf("ensure sns schema: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE sns_published_messages ADD COLUMN message_group_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sns_published_messages ADD COLUMN message_deduplication_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sns_subscriptions ADD COLUMN confirm_token TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("ensure sns schema: migrate: %w", err)
		}
	}
	if _, err := db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_sns_fifo_dedup ON sns_published_messages(topic_arn, message_deduplication_id)`,
	); err != nil {
		return fmt.Errorf("ensure sns schema: fifo index: %w", err)
	}
	return nil
}

// EnsureSNSSchema ensures SNS tables on an open store (tests and Open wiring).
func (s *Store) EnsureSNSSchema() error {
	return EnsureSNSSchema(s.db)
}

// TopicARN builds arn:aws:sns:REGION:ACCOUNT:NAME.
func TopicARN(region, accountID, topicName string) string {
	if region == "" {
		region = DefaultSNSRegion
	}
	return fmt.Sprintf("arn:aws:sns:%s:%s:%s", region, accountID, topicName)
}

// SubscriptionARN builds topic-arn:uuid for lab subscriptions.
func SubscriptionARN(topicARN string) string {
	return topicARN + ":" + uuid.NewString()
}

func topicNameFromARN(arn string) (string, error) {
	i := strings.LastIndex(arn, ":")
	if i < 0 || i == len(arn)-1 {
		return "", fmt.Errorf("invalid topic arn %q", arn)
	}
	return arn[i+1:], nil
}

func labAutoConfirmProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "sqs", "lambda":
		return true
	default:
		return false
	}
}

func topicIsFIFO(attrs map[string]string, topicName string) bool {
	if strings.HasSuffix(topicName, ".fifo") {
		return true
	}
	return attrTruthy(attrs, "FifoTopic")
}

func topicContentBasedDedup(attrs map[string]string) bool {
	return attrTruthy(attrs, "ContentBasedDeduplication")
}

func scanTopic(row *sql.Row) (Topic, error) {
	var (
		t      Topic
		attrs  string
		policy string
	)
	err := row.Scan(&t.AccountID, &t.TopicName, &t.TopicARN, &policy, &attrs, &t.CreationDate)
	if errors.Is(err, sql.ErrNoRows) {
		return Topic{}, ErrNoSuchTopic
	}
	if err != nil {
		return Topic{}, fmt.Errorf("scan topic: %w", err)
	}
	t.Policy = policy
	t.Attributes, err = unmarshalAttributes(attrs)
	if err != nil {
		return Topic{}, err
	}
	return t, nil
}

// CreateTopic inserts a new SNS topic.
func (s *Store) CreateTopic(accountID, region, topicName string, attributes map[string]string) (Topic, error) {
	if strings.TrimSpace(topicName) == "" {
		return Topic{}, fmt.Errorf("create topic: name is required")
	}
	if attributes == nil {
		attributes = map[string]string{}
	}
	fifo := topicIsFIFO(attributes, topicName)
	if fifo && !strings.HasSuffix(topicName, ".fifo") {
		return Topic{}, fmt.Errorf("%w: FIFO topic names must end with .fifo", ErrSNSInvalidParameter)
	}
	if strings.HasSuffix(topicName, ".fifo") {
		attributes["FifoTopic"] = "true"
		fifo = true
	}
	if fifo && attributes["FifoTopic"] == "" {
		attributes["FifoTopic"] = "true"
	}
	attrsJSON, err := marshalAttributes(attributes)
	if err != nil {
		return Topic{}, err
	}
	arn := TopicARN(region, accountID, topicName)
	created := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO sns_topics
		 (account_id, topic_name, topic_arn, policy_json, attributes_json, creation_date)
		 VALUES (?, ?, ?, '', ?, ?)`,
		accountID, topicName, arn, attrsJSON, created,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Topic{}, ErrTopicAlreadyExists
		}
		return Topic{}, fmt.Errorf("create topic: %w", err)
	}
	attrsCopy, err := unmarshalAttributes(attrsJSON)
	if err != nil {
		return Topic{}, err
	}
	return Topic{
		AccountID:    accountID,
		TopicName:    topicName,
		TopicARN:     arn,
		Attributes:   attrsCopy,
		CreationDate: created,
	}, nil
}

// GetTopic returns a topic by name or ErrNoSuchTopic.
func (s *Store) GetTopic(accountID, topicName string) (Topic, error) {
	row := s.db.QueryRow(
		`SELECT account_id, topic_name, topic_arn, policy_json, attributes_json, creation_date
		 FROM sns_topics WHERE account_id = ? AND topic_name = ?`,
		accountID, topicName,
	)
	return scanTopic(row)
}

// GetTopicByARN returns a topic by ARN or ErrNoSuchTopic.
func (s *Store) GetTopicByARN(topicARN string) (Topic, error) {
	row := s.db.QueryRow(
		`SELECT account_id, topic_name, topic_arn, policy_json, attributes_json, creation_date
		 FROM sns_topics WHERE topic_arn = ?`,
		topicARN,
	)
	return scanTopic(row)
}

// ListTopics returns all topics for an account.
func (s *Store) ListTopics(accountID string) ([]Topic, error) {
	rows, err := s.db.Query(
		`SELECT account_id, topic_name, topic_arn, policy_json, attributes_json, creation_date
		 FROM sns_topics WHERE account_id = ? ORDER BY topic_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()
	out := []Topic{}
	for rows.Next() {
		var (
			t      Topic
			attrs  string
			policy string
		)
		if err := rows.Scan(&t.AccountID, &t.TopicName, &t.TopicARN, &policy, &attrs, &t.CreationDate); err != nil {
			return nil, fmt.Errorf("list topics: %w", err)
		}
		t.Policy = policy
		t.Attributes, err = unmarshalAttributes(attrs)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTopic removes a topic and its subscriptions and published message metadata.
func (s *Store) DeleteTopic(accountID, topicName string) error {
	topic, err := s.GetTopic(accountID, topicName)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete topic: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM sns_subscriptions WHERE topic_arn = ?`, topic.TopicARN); err != nil {
		return fmt.Errorf("delete topic subscriptions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM sns_published_messages WHERE topic_arn = ?`, topic.TopicARN); err != nil {
		return fmt.Errorf("delete topic messages: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM sns_topics WHERE account_id = ? AND topic_name = ?`,
		accountID, topicName,
	); err != nil {
		return fmt.Errorf("delete topic: %w", err)
	}
	return tx.Commit()
}

func (s *Store) topicAttributesMap(topic Topic) map[string]string {
	attrs := map[string]string{}
	for k, v := range topic.Attributes {
		attrs[k] = v
	}
	attrs["TopicArn"] = topic.TopicARN
	if strings.TrimSpace(topic.Policy) != "" {
		attrs["Policy"] = topic.Policy
	}
	return attrs
}

// GetTopicAttributes returns the topic attribute map (including TopicArn and Policy when set).
func (s *Store) GetTopicAttributes(accountID, topicName string) (map[string]string, error) {
	topic, err := s.GetTopic(accountID, topicName)
	if err != nil {
		return nil, err
	}
	return s.topicAttributesMap(topic), nil
}

// SetTopicAttributes merges attributes into the topic. Policy is stored on the topic row.
func (s *Store) SetTopicAttributes(accountID, topicName string, attrs map[string]string) error {
	topic, err := s.GetTopic(accountID, topicName)
	if err != nil {
		return err
	}
	merged := topic.Attributes
	if merged == nil {
		merged = map[string]string{}
	}
	policy := topic.Policy
	for k, v := range attrs {
		if k == "Policy" {
			policy = v
			continue
		}
		merged[k] = v
	}
	attrsJSON, err := marshalAttributes(merged)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE sns_topics SET policy_json = ?, attributes_json = ? WHERE account_id = ? AND topic_name = ?`,
		policy, attrsJSON, accountID, topicName,
	)
	if err != nil {
		return fmt.Errorf("set topic attributes: %w", err)
	}
	return nil
}

// Publish persists message metadata and fans out to confirmed subscriptions.
func (s *Store) Publish(accountID, topicName, message, subject string, messageAttrs map[string]string) (PublishResult, error) {
	return s.PublishWithOpts(accountID, topicName, message, subject, messageAttrs, nil)
}

// PublishWithOpts is Publish with optional FIFO parameters.
func (s *Store) PublishWithOpts(accountID, topicName, message, subject string, messageAttrs map[string]string, opts *PublishOpts) (PublishResult, error) {
	topic, err := s.GetTopic(accountID, topicName)
	if err != nil {
		return PublishResult{}, err
	}
	groupID := ""
	dedupID := ""
	if topicIsFIFO(topic.Attributes, topic.TopicName) {
		if opts == nil || strings.TrimSpace(opts.MessageGroupID) == "" {
			return PublishResult{}, ErrSNSMissingMessageGroupID
		}
		groupID = strings.TrimSpace(opts.MessageGroupID)
		if topicContentBasedDedup(topic.Attributes) {
			dedupID = contentBasedDedupID([]byte(message))
		} else if opts != nil && strings.TrimSpace(opts.MessageDeduplicationID) != "" {
			dedupID = strings.TrimSpace(opts.MessageDeduplicationID)
		} else {
			return PublishResult{}, ErrSNSMissingDeduplicationID
		}
		if existing, found, findErr := s.findSNSDedupMessage(topic.TopicARN, dedupID); findErr != nil {
			return PublishResult{}, findErr
		} else if found {
			return PublishResult{MessageID: existing.MessageID}, nil
		}
	}
	messageID := uuid.NewString()
	created := nowRFC3339()
	attrsJSON, err := marshalAttributes(messageAttrs)
	if err != nil {
		return PublishResult{}, err
	}
	_, err = s.db.Exec(
		`INSERT INTO sns_published_messages
		 (message_id, topic_arn, body, subject, attributes_json, created_at, message_group_id, message_deduplication_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		messageID, topic.TopicARN, message, subject, attrsJSON, created, groupID, dedupID,
	)
	if err != nil {
		return PublishResult{}, fmt.Errorf("publish: %w", err)
	}
	msg := PublishedMessage{
		MessageID:              messageID,
		TopicARN:               topic.TopicARN,
		Body:                   message,
		Subject:                subject,
		Attributes:             messageAttrs,
		CreatedAt:              created,
		MessageGroupID:         groupID,
		MessageDeduplicationID: dedupID,
	}
	if err := s.fanOutTopicPublish(topic, msg); err != nil {
		return PublishResult{}, fmt.Errorf("publish fan-out: %w", err)
	}
	return PublishResult{MessageID: messageID}, nil
}

func (s *Store) findSNSDedupMessage(topicARN, dedupID string) (PublishedMessage, bool, error) {
	if dedupID == "" {
		return PublishedMessage{}, false, nil
	}
	var (
		m     PublishedMessage
		attrs string
	)
	cutoff := time.Now().UTC().Add(-FIFODedupWindow).Format(time.RFC3339)
	err := s.db.QueryRow(
		`SELECT message_id, topic_arn, body, subject, attributes_json, created_at,
		        COALESCE(message_group_id,''), COALESCE(message_deduplication_id,'')
		 FROM sns_published_messages
		 WHERE topic_arn = ? AND message_deduplication_id = ? AND created_at >= ?
		 ORDER BY created_at DESC LIMIT 1`,
		topicARN, dedupID, cutoff,
	).Scan(&m.MessageID, &m.TopicARN, &m.Body, &m.Subject, &attrs, &m.CreatedAt, &m.MessageGroupID, &m.MessageDeduplicationID)
	if errors.Is(err, sql.ErrNoRows) {
		return PublishedMessage{}, false, nil
	}
	if err != nil {
		return PublishedMessage{}, false, fmt.Errorf("sns dedup lookup: %w", err)
	}
	m.Attributes, err = unmarshalAttributes(attrs)
	if err != nil {
		return PublishedMessage{}, false, err
	}
	return m, true, nil
}

const snsDeliveryMaxAttempts = 2

// fanOutTopicPublish delivers to confirmed subscriptions (best-effort per target).
func (s *Store) fanOutTopicPublish(topic Topic, msg PublishedMessage) error {
	subs, err := s.listConfirmedSubscriptions(topic.TopicARN)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		s.deliverSNSSubscription(msg, sub)
	}
	return nil
}

func (s *Store) listConfirmedSubscriptions(topicARN string) ([]Subscription, error) {
	rows, err := s.db.Query(
		`SELECT subscription_arn, topic_arn, protocol, endpoint, confirmed, owner, COALESCE(confirm_token,'')
		 FROM sns_subscriptions WHERE topic_arn = ? AND confirmed = 1 ORDER BY subscription_arn`,
		topicARN,
	)
	if err != nil {
		return nil, fmt.Errorf("list confirmed subscriptions: %w", err)
	}
	defer rows.Close()
	return scanSubscriptionRows(rows)
}

func (s *Store) deliverSNSSubscription(msg PublishedMessage, sub Subscription) {
	var lastErr error
	for attempt := 0; attempt < snsDeliveryMaxAttempts; attempt++ {
		lastErr = s.deliverSNSSubscriptionOnce(msg, sub)
		if lastErr == nil {
			return
		}
	}
	log.Printf("sns delivery failed subscription=%s protocol=%s endpoint=%s err=%v",
		sub.SubscriptionARN, sub.Protocol, sub.Endpoint, lastErr)
}

func (s *Store) deliverSNSSubscriptionOnce(msg PublishedMessage, sub Subscription) error {
	switch strings.ToLower(strings.TrimSpace(sub.Protocol)) {
	case "sqs":
		return s.deliverSNSToSQS(sub, msg)
	case "lambda":
		return s.deliverSNSToLambda(sub, msg)
	case "http", "https":
		return s.deliverSNSToHTTP(sub, msg)
	default:
		return nil
	}
}

func (s *Store) deliverSNSToSQS(sub Subscription, msg PublishedMessage) error {
	queueAccount, queueName, err := s.resolveSQSEndpoint(sub.Owner, sub.Endpoint)
	if err != nil {
		return err
	}
	queueARN := sub.Endpoint
	if !strings.HasPrefix(strings.TrimSpace(queueARN), "arn:aws:sqs:") {
		q, qerr := s.GetQueue(queueAccount, queueName)
		if qerr != nil {
			return qerr
		}
		queueARN = q.QueueARN
	}
	if !s.deliveryTargetResourcePolicyAllows(queueAccount, queueARN, actionSQSSendMessage, authz.ServicePrincipalSNS) {
		return fmt.Errorf("sns sqs delivery: queue policy does not Allow sns.amazonaws.com")
	}
	body, err := json.Marshal(snsNotificationEnvelope(msg))
	if err != nil {
		return fmt.Errorf("marshal sns sqs envelope: %w", err)
	}
	var opts *SendMessageOpts
	q, qerr := s.GetQueue(queueAccount, queueName)
	if qerr == nil && queueIsFIFO(q.Attributes) {
		groupID := msg.MessageGroupID
		if groupID == "" {
			groupID = "sns-default"
		}
		dedup := msg.MessageDeduplicationID
		if dedup == "" {
			dedup = msg.MessageID
		}
		opts = &SendMessageOpts{MessageGroupID: groupID, MessageDeduplicationID: dedup}
	}
	if _, err := s.SendMessage(queueAccount, queueName, body, false, nil, "", opts); err != nil {
		return fmt.Errorf("sns sqs delivery: %w", err)
	}
	return nil
}

func (s *Store) deliverSNSToLambda(sub Subscription, msg PublishedMessage) error {
	functionName, qualifier := ParseFunctionQualifier(sub.Endpoint)
	if functionName == "" {
		return fmt.Errorf("sns lambda delivery: empty function endpoint")
	}
	fn, err := s.GetFunction(sub.Owner, functionName)
	if err != nil {
		return fmt.Errorf("sns lambda delivery: %w", err)
	}
	if !s.deliveryTargetResourcePolicyAllows(sub.Owner, fn.FunctionARN, actionLambdaInvokeFunction, authz.ServicePrincipalSNS) {
		return fmt.Errorf("sns lambda delivery: function policy does not Allow sns.amazonaws.com")
	}
	eventJSON, err := snsLambdaEventJSON(sub, msg)
	if err != nil {
		return err
	}
	if _, err := s.EnqueueAsyncInvoke(sub.Owner, functionName, qualifier, eventJSON); err != nil {
		return fmt.Errorf("sns lambda delivery: %w", err)
	}
	return nil
}

func (s *Store) resolveSQSQueueName(accountID, endpoint string) (string, error) {
	_, name, err := s.resolveSQSEndpoint(accountID, endpoint)
	return name, err
}

// resolveSQSEndpoint resolves a queue endpoint to (queueAccount, queueName).
// Same-account URL/name endpoints must match accountID. SQS ARNs may be cross-account.
func (s *Store) resolveSQSEndpoint(accountID, endpoint string) (queueAccount, queueName string, err error) {
	endpoint = strings.TrimSpace(endpoint)
	switch {
	case strings.HasPrefix(endpoint, "arn:aws:sqs:"):
		arnAccount, err := queueAccountFromARN(endpoint)
		if err != nil {
			return "", "", err
		}
		name, err := queueNameFromARN(endpoint)
		if err != nil {
			return "", "", err
		}
		if _, err := s.GetQueue(arnAccount, name); err != nil {
			return "", "", err
		}
		return arnAccount, name, nil
	case strings.Contains(endpoint, "://"):
		q, err := s.GetQueueByURL(endpoint)
		if err != nil {
			return "", "", err
		}
		if q.AccountID != accountID {
			return "", "", ErrNoSuchQueue
		}
		return q.AccountID, q.QueueName, nil
	default:
		if _, err := s.GetQueue(accountID, endpoint); err != nil {
			return "", "", err
		}
		return accountID, endpoint, nil
	}
}

func snsNotificationEnvelope(msg PublishedMessage) map[string]any {
	envelope := map[string]any{
		"Type":      "Notification",
		"MessageId": msg.MessageID,
		"TopicArn":  msg.TopicARN,
		"Message":   msg.Body,
		"Timestamp": msg.CreatedAt,
	}
	if strings.TrimSpace(msg.Subject) != "" {
		envelope["Subject"] = msg.Subject
	}
	return envelope
}

func snsLambdaEventJSON(sub Subscription, msg PublishedMessage) (string, error) {
	payload := map[string]any{
		"Records": []map[string]any{
			{
				"EventSource":          "aws:sns",
				"EventVersion":         "1.0",
				"EventSubscriptionArn": sub.SubscriptionARN,
				"Sns":                  snsNotificationEnvelope(msg),
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal sns lambda event: %w", err)
	}
	return string(raw), nil
}

// GetPublishedMessage returns persisted publish metadata by message id.
func (s *Store) GetPublishedMessage(messageID string) (PublishedMessage, error) {
	var (
		m     PublishedMessage
		attrs string
	)
	err := s.db.QueryRow(
		`SELECT message_id, topic_arn, body, subject, attributes_json, created_at,
		        COALESCE(message_group_id,''), COALESCE(message_deduplication_id,'')
		 FROM sns_published_messages WHERE message_id = ?`,
		messageID,
	).Scan(&m.MessageID, &m.TopicARN, &m.Body, &m.Subject, &attrs, &m.CreatedAt, &m.MessageGroupID, &m.MessageDeduplicationID)
	if errors.Is(err, sql.ErrNoRows) {
		return PublishedMessage{}, ErrNoSuchSNSMessage
	}
	if err != nil {
		return PublishedMessage{}, fmt.Errorf("get published message: %w", err)
	}
	m.Attributes, err = unmarshalAttributes(attrs)
	if err != nil {
		return PublishedMessage{}, err
	}
	return m, nil
}

func scanSubscription(row *sql.Row) (Subscription, error) {
	var (
		sub       Subscription
		confirmed int
	)
	err := row.Scan(&sub.SubscriptionARN, &sub.TopicARN, &sub.Protocol, &sub.Endpoint, &confirmed, &sub.Owner, &sub.ConfirmToken)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrNoSuchSubscription
	}
	if err != nil {
		return Subscription{}, fmt.Errorf("scan subscription: %w", err)
	}
	sub.Confirmed = confirmed == 1
	return sub, nil
}

// Subscribe creates a subscription on the topic. SQS and Lambda protocols are auto-confirmed in the lab.
// Cross-account Subscribe is allowed when topicNameOrARN is a foreign topic ARN; authz is enforced
// in the SNS handler via EvaluateSNS dual-eval. Delivery to foreign SQS endpoints uses the queue
// owner account for SendMessage and queue-policy checks.
func (s *Store) Subscribe(accountID, topicNameOrARN, protocol, endpoint string) (Subscription, error) {
	var topic Topic
	var err error
	if strings.HasPrefix(topicNameOrARN, "arn:aws:sns:") {
		topic, err = s.GetTopicByARN(topicNameOrARN)
	} else {
		topic, err = s.GetTopic(accountID, topicNameOrARN)
	}
	if err != nil {
		return Subscription{}, err
	}
	protocol = strings.TrimSpace(protocol)
	endpoint = strings.TrimSpace(endpoint)
	if protocol == "" || endpoint == "" {
		return Subscription{}, fmt.Errorf("subscribe: protocol and endpoint are required")
	}
	protoLower := strings.ToLower(protocol)
	if protoLower == "http" || protoLower == "https" {
		if err := validateSNSHTTPEndpoint(endpoint); err != nil {
			return Subscription{}, err
		}
	}
	if topicIsFIFO(topic.Attributes, topic.TopicName) && protoLower == "sqs" {
		qAccount, qName, qerr := s.resolveSQSEndpoint(accountID, endpoint)
		if qerr == nil {
			q, gerr := s.GetQueue(qAccount, qName)
			if gerr == nil && !queueIsFIFO(q.Attributes) {
				return Subscription{}, fmt.Errorf("%w: FIFO topic requires FIFO SQS subscription", ErrSNSInvalidParameter)
			}
		}
	}
	subARN := SubscriptionARN(topic.TopicARN)
	token := ""
	confirmed := 0
	if labAutoConfirmProtocol(protocol) {
		confirmed = 1
	} else if protoLower == "http" || protoLower == "https" {
		token = uuid.NewString()
	}
	_, err = s.db.Exec(
		`INSERT INTO sns_subscriptions
		 (subscription_arn, topic_arn, protocol, endpoint, confirmed, owner, confirm_token)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		subARN, topic.TopicARN, protocol, endpoint, confirmed, accountID, token,
	)
	if err != nil {
		return Subscription{}, fmt.Errorf("subscribe: %w", err)
	}
	sub := Subscription{
		SubscriptionARN: subARN,
		TopicARN:        topic.TopicARN,
		Protocol:        protocol,
		Endpoint:        endpoint,
		Confirmed:       confirmed == 1,
		Owner:           accountID,
		ConfirmToken:    token,
	}
	if token != "" {
		_ = s.deliverSNSHTTPConfirmation(sub)
	}
	return sub, nil
}

// ConfirmSubscription confirms a pending HTTP(S) subscription by token.
func (s *Store) ConfirmSubscription(topicARN, token string) (Subscription, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Subscription{}, fmt.Errorf("%w: Token is required", ErrSNSInvalidParameter)
	}
	row := s.db.QueryRow(
		`SELECT subscription_arn, topic_arn, protocol, endpoint, confirmed, owner, COALESCE(confirm_token,'')
		 FROM sns_subscriptions WHERE confirm_token = ?`,
		token,
	)
	sub, err := scanSubscription(row)
	if err != nil {
		return Subscription{}, err
	}
	if topicARN != "" && sub.TopicARN != topicARN {
		return Subscription{}, ErrNoSuchSubscription
	}
	_, err = s.db.Exec(
		`UPDATE sns_subscriptions SET confirmed = 1 WHERE subscription_arn = ?`,
		sub.SubscriptionARN,
	)
	if err != nil {
		return Subscription{}, fmt.Errorf("confirm subscription: %w", err)
	}
	sub.Confirmed = true
	return sub, nil
}

// Unsubscribe removes a subscription by ARN.
func (s *Store) Unsubscribe(subscriptionARN string) error {
	res, err := s.db.Exec(`DELETE FROM sns_subscriptions WHERE subscription_arn = ?`, subscriptionARN)
	if err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNoSuchSubscription
	}
	return nil
}

// GetSubscription returns a subscription by ARN.
func (s *Store) GetSubscription(subscriptionARN string) (Subscription, error) {
	row := s.db.QueryRow(
		`SELECT subscription_arn, topic_arn, protocol, endpoint, confirmed, owner, COALESCE(confirm_token,'')
		 FROM sns_subscriptions WHERE subscription_arn = ?`,
		subscriptionARN,
	)
	return scanSubscription(row)
}

func subscriptionAttributes(sub Subscription) map[string]string {
	confirmed := "false"
	if sub.Confirmed {
		confirmed = "true"
	}
	return map[string]string{
		"SubscriptionArn":              sub.SubscriptionARN,
		"TopicArn":                     sub.TopicARN,
		"Protocol":                     sub.Protocol,
		"Endpoint":                     sub.Endpoint,
		"Owner":                        sub.Owner,
		"ConfirmationWasAuthenticated": confirmed,
	}
}

// GetSubscriptionAttributes returns lab-minimum subscription attributes.
func (s *Store) GetSubscriptionAttributes(subscriptionARN string) (map[string]string, error) {
	sub, err := s.GetSubscription(subscriptionARN)
	if err != nil {
		return nil, err
	}
	return subscriptionAttributes(sub), nil
}

// ListSubscriptions returns all subscriptions owned by the account.
func (s *Store) ListSubscriptions(accountID string) ([]Subscription, error) {
	rows, err := s.db.Query(
		`SELECT subscription_arn, topic_arn, protocol, endpoint, confirmed, owner, COALESCE(confirm_token,'')
		 FROM sns_subscriptions WHERE owner = ? ORDER BY subscription_arn`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()
	return scanSubscriptionRows(rows)
}

// ListSubscriptionsByTopic returns subscriptions for a topic name in the account.
func (s *Store) ListSubscriptionsByTopic(accountID, topicName string) ([]Subscription, error) {
	topic, err := s.GetTopic(accountID, topicName)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT subscription_arn, topic_arn, protocol, endpoint, confirmed, owner, COALESCE(confirm_token,'')
		 FROM sns_subscriptions WHERE topic_arn = ? ORDER BY subscription_arn`,
		topic.TopicARN,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions by topic: %w", err)
	}
	defer rows.Close()
	return scanSubscriptionRows(rows)
}

func scanSubscriptionRows(rows *sql.Rows) ([]Subscription, error) {
	out := []Subscription{}
	for rows.Next() {
		var (
			sub       Subscription
			confirmed int
		)
		if err := rows.Scan(&sub.SubscriptionARN, &sub.TopicARN, &sub.Protocol, &sub.Endpoint, &confirmed, &sub.Owner, &sub.ConfirmToken); err != nil {
			return nil, fmt.Errorf("scan subscriptions: %w", err)
		}
		sub.Confirmed = confirmed == 1
		out = append(out, sub)
	}
	return out, rows.Err()
}

// AddTopicPermission appends an Allow statement to the topic policy (lab minimum for AddPermission).
func (s *Store) AddTopicPermission(accountID, topicName, label, principalARN, action string) error {
	topic, err := s.GetTopic(accountID, topicName)
	if err != nil {
		return err
	}
	label = strings.TrimSpace(label)
	principalARN = strings.TrimSpace(principalARN)
	action = strings.TrimSpace(action)

	doc, err := parseSNSPolicyDoc(topic.Policy)
	if err != nil {
		return err
	}
	if label != "" && snsPolicyHasSid(doc, label) {
		return ErrSNSPolicyStatementExists
	}

	stmt := map[string]any{
		"Effect":    "Allow",
		"Principal": map[string]any{"AWS": principalARN},
		"Action":    action,
		"Resource":  topic.TopicARN,
	}
	if label != "" {
		stmt["Sid"] = label
	}

	statements, err := snsPolicyStatements(doc)
	if err != nil {
		return err
	}
	statements = append(statements, stmt)
	doc["Statement"] = statements

	updated, err := marshalSNSPolicyDoc(doc)
	if err != nil {
		return err
	}
	return s.SetTopicAttributes(accountID, topicName, map[string]string{"Policy": updated})
}

func parseSNSPolicyDoc(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{
			"Version":   "2012-10-17",
			"Statement": []any{},
		}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("parse topic policy: %w", err)
	}
	if doc["Version"] == nil {
		doc["Version"] = "2012-10-17"
	}
	return doc, nil
}

func snsPolicyStatements(doc map[string]any) ([]map[string]any, error) {
	raw, ok := doc["Statement"]
	if !ok || raw == nil {
		return []map[string]any{}, nil
	}
	switch typed := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("parse topic policy: invalid statement")
			}
			out = append(out, m)
		}
		return out, nil
	case map[string]any:
		return []map[string]any{typed}, nil
	default:
		return nil, fmt.Errorf("parse topic policy: invalid statement list")
	}
}

func snsPolicyHasSid(doc map[string]any, sid string) bool {
	statements, err := snsPolicyStatements(doc)
	if err != nil {
		return false
	}
	for _, stmt := range statements {
		if s, _ := stmt["Sid"].(string); s == sid {
			return true
		}
	}
	return false
}

func marshalSNSPolicyDoc(doc map[string]any) (string, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal topic policy: %w", err)
	}
	return string(raw), nil
}

// RemoveTopicPermission is a lab stub; full policy statement removal is deferred to handlers.
func (s *Store) RemoveTopicPermission(accountID, topicName, label string) error {
	_, err := s.GetTopic(accountID, topicName)
	if err != nil {
		return err
	}
	_ = label
	return nil
}

// TopicNameFromARN parses the topic name from an SNS topic ARN.
func TopicNameFromARN(arn string) (string, error) {
	return topicNameFromARN(arn)
}

// LabSNSHTTPCatcherPath is the allowlisted in-process HTTP catcher path.
const LabSNSHTTPCatcherPath = "/_noctaxris/sns-http-catcher"

// SNSHTTPCatcherMessage is a caught HTTP delivery or confirmation.
type SNSHTTPCatcherMessage struct {
	ID              int64
	SubscriptionARN string
	TopicARN        string
	MessageType     string
	Body            string
	ReceivedAt      string
}

// EnvSNSHTTPAllowlist extends the default SNS HTTP catcher allowlist (comma-separated exact URLs).
const EnvSNSHTTPAllowlist = "NOCTAXRIS_SNS_HTTP_ALLOWLIST"

// validateSNSHTTPEndpoint allows only the lab catcher on loopback :4566
// (or exact URLs listed in NOCTAXRIS_SNS_HTTP_ALLOWLIST). Arbitrary loopback
// ports are rejected to avoid open-proxy style delivery.
func validateSNSHTTPEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%w: invalid HTTP endpoint", ErrSNSInvalidParameter)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: protocol must be http or https", ErrSNSInvalidParameter)
	}
	if allowlistedSNSHTTPEndpoint(endpoint) {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "127.0.0.1", "localhost", "::1":
	default:
		return ErrSNSEndpointNotAllowed
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if port != "4566" {
		return ErrSNSEndpointNotAllowed
	}
	path := u.Path
	if path != LabSNSHTTPCatcherPath && !strings.HasPrefix(path, LabSNSHTTPCatcherPath+"/") {
		return ErrSNSEndpointNotAllowed
	}
	return nil
}

func allowlistedSNSHTTPEndpoint(endpoint string) bool {
	raw := strings.TrimSpace(os.Getenv(EnvSNSHTTPAllowlist))
	if raw == "" {
		return false
	}
	want := strings.TrimSpace(endpoint)
	for _, entry := range strings.Split(raw, ",") {
		if strings.TrimSpace(entry) == want {
			return true
		}
	}
	return false
}

func (s *Store) deliverSNSToHTTP(sub Subscription, msg PublishedMessage) error {
	if err := validateSNSHTTPEndpoint(sub.Endpoint); err != nil {
		return err
	}
	envelope := snsNotificationEnvelope(msg)
	envelope["Type"] = "Notification"
	raw, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return s.postSNSHTTP(sub.Endpoint, sub.SubscriptionARN, sub.TopicARN, "Notification", raw)
}

func (s *Store) deliverSNSHTTPConfirmation(sub Subscription) error {
	if err := validateSNSHTTPEndpoint(sub.Endpoint); err != nil {
		return err
	}
	subscribeURL := fmt.Sprintf("http://127.0.0.1:4566%s?Action=ConfirmSubscription&Token=%s&TopicArn=%s",
		LabSNSHTTPCatcherPath, url.QueryEscape(sub.ConfirmToken), url.QueryEscape(sub.TopicARN))
	payload := map[string]any{
		"Type":            "SubscriptionConfirmation",
		"MessageId":       uuid.NewString(),
		"Token":           sub.ConfirmToken,
		"TopicArn":        sub.TopicARN,
		"Message":         "You have chosen to subscribe to the topic.",
		"SubscribeURL":    subscribeURL,
		"Timestamp":       nowRFC3339(),
		"SubscriptionArn": sub.SubscriptionARN,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.postSNSHTTP(sub.Endpoint, sub.SubscriptionARN, sub.TopicARN, "SubscriptionConfirmation", raw)
}

func (s *Store) postSNSHTTP(endpoint, subARN, topicARN, msgType string, body []byte) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	// Lab catcher path: persist without outbound HTTP (tests + same-process delivery).
	if strings.HasPrefix(u.Path, LabSNSHTTPCatcherPath) || u.Path == LabSNSHTTPCatcherPath {
		return s.RecordSNSHTTPCatcher(subARN, topicARN, msgType, string(body))
	}
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-amz-sns-message-type", msgType)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sns http delivery status %d", resp.StatusCode)
	}
	return nil
}

// RecordSNSHTTPCatcher stores a lab catcher payload.
func (s *Store) RecordSNSHTTPCatcher(subARN, topicARN, msgType, body string) error {
	_, err := s.db.Exec(
		`INSERT INTO sns_http_catcher (subscription_arn, topic_arn, message_type, body, received_at)
		 VALUES (?, ?, ?, ?, ?)`,
		subARN, topicARN, msgType, body, nowRFC3339(),
	)
	if err != nil {
		return fmt.Errorf("record sns http catcher: %w", err)
	}
	return nil
}

// ListSNSHTTPCatcher returns caught HTTP deliveries (newest last).
func (s *Store) ListSNSHTTPCatcher() ([]SNSHTTPCatcherMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, subscription_arn, topic_arn, message_type, body, received_at
		 FROM sns_http_catcher ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list sns http catcher: %w", err)
	}
	defer rows.Close()
	out := []SNSHTTPCatcherMessage{}
	for rows.Next() {
		var m SNSHTTPCatcherMessage
		if err := rows.Scan(&m.ID, &m.SubscriptionARN, &m.TopicARN, &m.MessageType, &m.Body, &m.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
