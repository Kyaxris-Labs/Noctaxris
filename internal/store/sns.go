package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
}

// PublishResult is the outcome of Publish (message id for fan-out and delivery).
type PublishResult struct {
	MessageID string
}

// PublishedMessage is a persisted publish metadata row.
type PublishedMessage struct {
	MessageID  string
	TopicARN   string
	Body       string
	Subject    string
	Attributes map[string]string
	CreatedAt  string
}

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
  owner TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS sns_published_messages (
  message_id TEXT PRIMARY KEY,
  topic_arn TEXT NOT NULL,
  body TEXT NOT NULL,
  subject TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sns_subscriptions_topic ON sns_subscriptions(topic_arn);
`

// EnsureSNSSchema creates SNS tables if missing.
func EnsureSNSSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure sns schema: db is nil")
	}
	if _, err := db.Exec(snsSchema); err != nil {
		return fmt.Errorf("ensure sns schema: %w", err)
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

func scanTopic(row *sql.Row) (Topic, error) {
	var (
		t       Topic
		attrs   string
		policy  string
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
	topic, err := s.GetTopic(accountID, topicName)
	if err != nil {
		return PublishResult{}, err
	}
	messageID := uuid.NewString()
	created := nowRFC3339()
	attrsJSON, err := marshalAttributes(messageAttrs)
	if err != nil {
		return PublishResult{}, err
	}
	_, err = s.db.Exec(
		`INSERT INTO sns_published_messages
		 (message_id, topic_arn, body, subject, attributes_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		messageID, topic.TopicARN, message, subject, attrsJSON, created,
	)
	if err != nil {
		return PublishResult{}, fmt.Errorf("publish: %w", err)
	}
	if err := s.fanOutTopicPublish(topic, PublishedMessage{
		MessageID:  messageID,
		TopicARN:   topic.TopicARN,
		Body:       message,
		Subject:    subject,
		Attributes: messageAttrs,
		CreatedAt:  created,
	}); err != nil {
		return PublishResult{}, fmt.Errorf("publish fan-out: %w", err)
	}
	return PublishResult{MessageID: messageID}, nil
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
		`SELECT subscription_arn, topic_arn, protocol, endpoint, confirmed, owner
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
	for attempt := 0; attempt < snsDeliveryMaxAttempts; attempt++ {
		if err := s.deliverSNSSubscriptionOnce(msg, sub); err == nil {
			return
		}
	}
}

func (s *Store) deliverSNSSubscriptionOnce(msg PublishedMessage, sub Subscription) error {
	switch strings.ToLower(strings.TrimSpace(sub.Protocol)) {
	case "sqs":
		return s.deliverSNSToSQS(sub, msg)
	case "lambda":
		return s.deliverSNSToLambda(sub, msg)
	default:
		return nil
	}
}

func (s *Store) deliverSNSToSQS(sub Subscription, msg PublishedMessage) error {
	queueName, err := s.resolveSQSQueueName(sub.Owner, sub.Endpoint)
	if err != nil {
		return err
	}
	body, err := json.Marshal(snsNotificationEnvelope(msg))
	if err != nil {
		return fmt.Errorf("marshal sns sqs envelope: %w", err)
	}
	if _, err := s.SendMessage(sub.Owner, queueName, body, false, nil, "", nil); err != nil {
		return fmt.Errorf("sns sqs delivery: %w", err)
	}
	return nil
}

func (s *Store) deliverSNSToLambda(sub Subscription, msg PublishedMessage) error {
	functionName, qualifier := ParseFunctionQualifier(sub.Endpoint)
	if functionName == "" {
		return fmt.Errorf("sns lambda delivery: empty function endpoint")
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
	endpoint = strings.TrimSpace(endpoint)
	switch {
	case strings.HasPrefix(endpoint, "arn:aws:sqs:"):
		return queueNameFromARN(endpoint)
	case strings.Contains(endpoint, "://"):
		q, err := s.GetQueueByURL(endpoint)
		if err != nil {
			return "", err
		}
		if q.AccountID != accountID {
			return "", ErrNoSuchQueue
		}
		return q.QueueName, nil
	default:
		if _, err := s.GetQueue(accountID, endpoint); err != nil {
			return "", err
		}
		return endpoint, nil
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
		`SELECT message_id, topic_arn, body, subject, attributes_json, created_at
		 FROM sns_published_messages WHERE message_id = ?`,
		messageID,
	).Scan(&m.MessageID, &m.TopicARN, &m.Body, &m.Subject, &attrs, &m.CreatedAt)
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
	err := row.Scan(&sub.SubscriptionARN, &sub.TopicARN, &sub.Protocol, &sub.Endpoint, &confirmed, &sub.Owner)
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
	if topic.AccountID != accountID {
		return Subscription{}, ErrNoSuchTopic
	}
	protocol = strings.TrimSpace(protocol)
	endpoint = strings.TrimSpace(endpoint)
	if protocol == "" || endpoint == "" {
		return Subscription{}, fmt.Errorf("subscribe: protocol and endpoint are required")
	}
	subARN := SubscriptionARN(topic.TopicARN)
	confirmed := 0
	if labAutoConfirmProtocol(protocol) {
		confirmed = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO sns_subscriptions
		 (subscription_arn, topic_arn, protocol, endpoint, confirmed, owner)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		subARN, topic.TopicARN, protocol, endpoint, confirmed, accountID,
	)
	if err != nil {
		return Subscription{}, fmt.Errorf("subscribe: %w", err)
	}
	return Subscription{
		SubscriptionARN: subARN,
		TopicARN:        topic.TopicARN,
		Protocol:        protocol,
		Endpoint:        endpoint,
		Confirmed:       confirmed == 1,
		Owner:           accountID,
	}, nil
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
		`SELECT subscription_arn, topic_arn, protocol, endpoint, confirmed, owner
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
		"SubscriptionArn": sub.SubscriptionARN,
		"TopicArn":        sub.TopicARN,
		"Protocol":        sub.Protocol,
		"Endpoint":        sub.Endpoint,
		"Owner":           sub.Owner,
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
		`SELECT subscription_arn, topic_arn, protocol, endpoint, confirmed, owner
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
		`SELECT subscription_arn, topic_arn, protocol, endpoint, confirmed, owner
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
		if err := rows.Scan(&sub.SubscriptionARN, &sub.TopicARN, &sub.Protocol, &sub.Endpoint, &confirmed, &sub.Owner); err != nil {
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
