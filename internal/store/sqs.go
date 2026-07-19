package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrQueueAlreadyExists = errors.New("QueueNameExists")
	ErrNoSuchQueue        = errors.New("QueueDoesNotExist")
	ErrNoSuchMessage      = errors.New("ReceiptHandleIsInvalid")
)

const (
	// DefaultSQSRegion is the lab region embedded in SQS ARNs.
	DefaultSQSRegion = "us-east-1"

	// DefaultVisibilityTimeout is the fallback visibility timeout in seconds when
	// the queue does not set VisibilityTimeout.
	DefaultVisibilityTimeout = 30

	attrVisibilityTimeout = "VisibilityTimeout"
)

// Queue is an SQS queue metadata row.
type Queue struct {
	AccountID    string
	QueueName    string
	QueueURL     string
	QueueARN     string
	Attributes   map[string]string
	CreationDate string
}

// Message is an SQS message row. Body is plaintext when Sealed is false, or
// ciphertext when Sealed is true (with optional SealedDEK).
type Message struct {
	MessageID      string
	AccountID      string
	QueueName      string
	Body           []byte
	Sealed         bool
	SealedDEK      []byte
	ReceiptHandle  string
	VisibleAfter   string
	ReceiveCount   int
	CreatedAt      string
	AttributesJSON string
}

// QueueARN builds arn:aws:sqs:REGION:ACCOUNT:NAME.
func QueueARN(region, accountID, queueName string) string {
	if region == "" {
		region = DefaultSQSRegion
	}
	return fmt.Sprintf("arn:aws:sqs:%s:%s:%s", region, accountID, queueName)
}

// QueueURL builds a path-style queue URL: SCHEME://HOST/ACCOUNT/NAME. endpointHost
// may include a scheme; http is assumed when none is present.
func QueueURL(endpointHost, accountID, queueName string) string {
	base := endpointHost
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	base = strings.TrimRight(base, "/")
	return base + "/" + accountID + "/" + queueName
}

func marshalAttributes(attrs map[string]string) (string, error) {
	if len(attrs) == 0 {
		return "{}", nil
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		return "", fmt.Errorf("marshal attributes: %w", err)
	}
	return string(raw), nil
}

func unmarshalAttributes(raw string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("unmarshal attributes: %w", err)
	}
	return out, nil
}

// CreateQueue inserts a new queue with the given attributes.
func (s *Store) CreateQueue(accountID, region, endpointHost, queueName string, attributes map[string]string) (Queue, error) {
	if strings.TrimSpace(queueName) == "" {
		return Queue{}, fmt.Errorf("create queue: name is required")
	}
	attrsJSON, err := marshalAttributes(attributes)
	if err != nil {
		return Queue{}, err
	}
	url := QueueURL(endpointHost, accountID, queueName)
	arn := QueueARN(region, accountID, queueName)
	created := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO sqs_queues
		 (account_id, queue_name, queue_url, queue_arn, attributes_json, creation_date)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, queueName, url, arn, attrsJSON, created,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Queue{}, ErrQueueAlreadyExists
		}
		return Queue{}, fmt.Errorf("create queue: %w", err)
	}
	attrsCopy, err := unmarshalAttributes(attrsJSON)
	if err != nil {
		return Queue{}, err
	}
	return Queue{
		AccountID:    accountID,
		QueueName:    queueName,
		QueueURL:     url,
		QueueARN:     arn,
		Attributes:   attrsCopy,
		CreationDate: created,
	}, nil
}

func scanQueue(row *sql.Row) (Queue, error) {
	var (
		q     Queue
		attrs string
	)
	err := row.Scan(&q.AccountID, &q.QueueName, &q.QueueURL, &q.QueueARN, &attrs, &q.CreationDate)
	if errors.Is(err, sql.ErrNoRows) {
		return Queue{}, ErrNoSuchQueue
	}
	if err != nil {
		return Queue{}, fmt.Errorf("scan queue: %w", err)
	}
	q.Attributes, err = unmarshalAttributes(attrs)
	if err != nil {
		return Queue{}, err
	}
	return q, nil
}

// GetQueue returns a queue by name or ErrNoSuchQueue.
func (s *Store) GetQueue(accountID, queueName string) (Queue, error) {
	row := s.db.QueryRow(
		`SELECT account_id, queue_name, queue_url, queue_arn, attributes_json, creation_date
		 FROM sqs_queues WHERE account_id = ? AND queue_name = ?`,
		accountID, queueName,
	)
	return scanQueue(row)
}

// GetQueueByURL returns a queue by its URL or ErrNoSuchQueue.
func (s *Store) GetQueueByURL(queueURL string) (Queue, error) {
	row := s.db.QueryRow(
		`SELECT account_id, queue_name, queue_url, queue_arn, attributes_json, creation_date
		 FROM sqs_queues WHERE queue_url = ?`,
		queueURL,
	)
	return scanQueue(row)
}

// ListQueues returns queues for an account, optionally filtered by name prefix.
func (s *Store) ListQueues(accountID, prefix string) ([]Queue, error) {
	rows, err := s.db.Query(
		`SELECT account_id, queue_name, queue_url, queue_arn, attributes_json, creation_date
		 FROM sqs_queues WHERE account_id = ? AND queue_name LIKE ? ORDER BY queue_name`,
		accountID, prefix+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("list queues: %w", err)
	}
	defer rows.Close()
	out := []Queue{}
	for rows.Next() {
		var (
			q     Queue
			attrs string
		)
		if err := rows.Scan(&q.AccountID, &q.QueueName, &q.QueueURL, &q.QueueARN, &attrs, &q.CreationDate); err != nil {
			return nil, fmt.Errorf("list queues: %w", err)
		}
		q.Attributes, err = unmarshalAttributes(attrs)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// DeleteQueue removes a queue and all of its messages.
func (s *Store) DeleteQueue(accountID, queueName string) error {
	if _, err := s.GetQueue(accountID, queueName); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete queue: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM sqs_messages WHERE account_id = ? AND queue_name = ?`,
		accountID, queueName,
	); err != nil {
		return fmt.Errorf("delete queue messages: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM sqs_queues WHERE account_id = ? AND queue_name = ?`,
		accountID, queueName,
	); err != nil {
		return fmt.Errorf("delete queue: %w", err)
	}
	return tx.Commit()
}

// GetQueueAttributes returns the queue attribute map.
func (s *Store) GetQueueAttributes(accountID, queueName string) (map[string]string, error) {
	q, err := s.GetQueue(accountID, queueName)
	if err != nil {
		return nil, err
	}
	return q.Attributes, nil
}

// SetQueueAttributes merges the provided attributes into the stored map.
func (s *Store) SetQueueAttributes(accountID, queueName string, attrs map[string]string) error {
	q, err := s.GetQueue(accountID, queueName)
	if err != nil {
		return err
	}
	merged := q.Attributes
	if merged == nil {
		merged = map[string]string{}
	}
	for k, v := range attrs {
		merged[k] = v
	}
	attrsJSON, err := marshalAttributes(merged)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE sqs_queues SET attributes_json = ? WHERE account_id = ? AND queue_name = ?`,
		attrsJSON, accountID, queueName,
	)
	if err != nil {
		return fmt.Errorf("set queue attributes: %w", err)
	}
	return nil
}

func queueVisibilityTimeout(attrs map[string]string) int {
	if v, ok := attrs[attrVisibilityTimeout]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return n
		}
	}
	return DefaultVisibilityTimeout
}

// SendMessage stores a message on the queue. body is plaintext when sealed is
// false, or ciphertext when sealed is true. messageAttributesJSON is stored as-is.
func (s *Store) SendMessage(accountID, queueName string, body []byte, sealed bool, sealedDEK []byte, messageAttributesJSON string) (Message, error) {
	if _, err := s.GetQueue(accountID, queueName); err != nil {
		return Message{}, err
	}
	messageID := uuid.NewString()
	created := nowRFC3339()
	sealedFlag := 0
	if sealed {
		sealedFlag = 1
	}
	attrsJSON := messageAttributesJSON
	if strings.TrimSpace(attrsJSON) == "" {
		attrsJSON = "{}"
	}
	_, err := s.db.Exec(
		`INSERT INTO sqs_messages
		 (message_id, account_id, queue_name, body, sealed, sealed_dek,
		  receipt_handle, visible_after, receive_count, created_at, attributes_json)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, '', 0, ?, ?)`,
		messageID, accountID, queueName, body, sealedFlag, sealedDEK, created, attrsJSON,
	)
	if err != nil {
		return Message{}, fmt.Errorf("send message: %w", err)
	}
	return Message{
		MessageID:      messageID,
		AccountID:      accountID,
		QueueName:      queueName,
		Body:           body,
		Sealed:         sealed,
		SealedDEK:      sealedDEK,
		ReceiveCount:   0,
		CreatedAt:      created,
		AttributesJSON: attrsJSON,
	}, nil
}

// SendMessageBatch sends each body in order and returns the stored messages.
func (s *Store) SendMessageBatch(accountID, queueName string, bodies [][]byte) ([]Message, error) {
	out := make([]Message, 0, len(bodies))
	for _, body := range bodies {
		m, err := s.SendMessage(accountID, queueName, body, false, nil, "")
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// ReceiveMessages returns up to max currently visible messages. Each returned
// message gets a fresh receipt handle, an incremented receive count, and a
// visibility deadline of now plus the queue VisibilityTimeout.
func (s *Store) ReceiveMessages(accountID, queueName string, max int) ([]Message, error) {
	q, err := s.GetQueue(accountID, queueName)
	if err != nil {
		return nil, err
	}
	if max <= 0 {
		max = 1
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	rows, err := s.db.Query(
		`SELECT message_id, body, sealed, sealed_dek, receive_count, created_at, attributes_json
		 FROM sqs_messages
		 WHERE account_id = ? AND queue_name = ? AND (visible_after = '' OR visible_after <= ?)
		 ORDER BY created_at, message_id
		 LIMIT ?`,
		accountID, queueName, nowStr, max,
	)
	if err != nil {
		return nil, fmt.Errorf("receive messages: %w", err)
	}
	defer rows.Close()

	visTimeout := queueVisibilityTimeout(q.Attributes)
	visibleAfter := now.Add(time.Duration(visTimeout) * time.Second).Format(time.RFC3339)

	var pending []Message
	for rows.Next() {
		var (
			m         Message
			sealed    int
			sealedDEK []byte
		)
		if err := rows.Scan(&m.MessageID, &m.Body, &sealed, &sealedDEK, &m.ReceiveCount, &m.CreatedAt, &m.AttributesJSON); err != nil {
			return nil, fmt.Errorf("receive messages: %w", err)
		}
		m.AccountID = accountID
		m.QueueName = queueName
		m.Sealed = sealed == 1
		m.SealedDEK = sealedDEK
		pending = append(pending, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("receive messages: %w", err)
	}

	out := make([]Message, 0, len(pending))
	for _, m := range pending {
		handle := uuid.NewString()
		res, err := s.db.Exec(
			`UPDATE sqs_messages
			 SET receipt_handle = ?, visible_after = ?, receive_count = receive_count + 1
			 WHERE message_id = ?`,
			handle, visibleAfter, m.MessageID,
		)
		if err != nil {
			return nil, fmt.Errorf("receive messages: %w", err)
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			continue
		}
		m.ReceiptHandle = handle
		m.VisibleAfter = visibleAfter
		m.ReceiveCount++
		out = append(out, m)
	}
	return out, nil
}

// DeleteMessage removes a message by receipt handle. A stale or unknown handle
// returns ErrNoSuchMessage.
func (s *Store) DeleteMessage(accountID, queueName, receiptHandle string) error {
	if _, err := s.GetQueue(accountID, queueName); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`DELETE FROM sqs_messages WHERE account_id = ? AND queue_name = ? AND receipt_handle = ?`,
		accountID, queueName, receiptHandle,
	)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNoSuchMessage
	}
	return nil
}

// ChangeMessageVisibility resets the visibility deadline for a received message
// to now plus visibilityTimeoutSeconds.
func (s *Store) ChangeMessageVisibility(accountID, queueName, receiptHandle string, visibilityTimeoutSeconds int) error {
	if _, err := s.GetQueue(accountID, queueName); err != nil {
		return err
	}
	if visibilityTimeoutSeconds < 0 {
		visibilityTimeoutSeconds = 0
	}
	visibleAfter := time.Now().UTC().Add(time.Duration(visibilityTimeoutSeconds) * time.Second).Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE sqs_messages SET visible_after = ?
		 WHERE account_id = ? AND queue_name = ? AND receipt_handle = ?`,
		visibleAfter, accountID, queueName, receiptHandle,
	)
	if err != nil {
		return fmt.Errorf("change message visibility: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNoSuchMessage
	}
	return nil
}

// PurgeQueue deletes all messages from a queue.
func (s *Store) PurgeQueue(accountID, queueName string) error {
	if _, err := s.GetQueue(accountID, queueName); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`DELETE FROM sqs_messages WHERE account_id = ? AND queue_name = ?`,
		accountID, queueName,
	)
	if err != nil {
		return fmt.Errorf("purge queue: %w", err)
	}
	return nil
}
