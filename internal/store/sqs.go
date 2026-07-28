package store

import (
	"crypto/md5"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrQueueAlreadyExists          = errors.New("QueueNameExists")
	ErrNoSuchQueue                 = errors.New("QueueDoesNotExist")
	ErrNoSuchMessage               = errors.New("ReceiptHandleIsInvalid")
	ErrInvalidFIFOQueueName        = errors.New("InvalidFIFOQueueName")
	ErrMissingMessageGroupID       = errors.New("MissingMessageGroupId")
	ErrMissingMessageDeduplication = errors.New("MissingMessageDeduplicationId")
)

const (
	// DefaultSQSRegion is the lab region embedded in SQS ARNs.
	DefaultSQSRegion = "us-east-1"

	// DefaultVisibilityTimeout is the fallback visibility timeout in seconds when
	// the queue does not set VisibilityTimeout.
	DefaultVisibilityTimeout = 30

	// FIFODedupWindow is the lab deduplication window for FIFO queues.
	FIFODedupWindow = 5 * time.Minute

	attrVisibilityTimeout          = "VisibilityTimeout"
	attrFifoQueue                  = "FifoQueue"
	attrContentBasedDeduplication  = "ContentBasedDeduplication"
	attrRedrivePolicy              = "RedrivePolicy"
	attrRedriveAllowPolicy         = "RedriveAllowPolicy"
	attrDelaySeconds               = "DelaySeconds"

	// sqsAttrNoctaxrisDlqSourceArn is set on messages redriven to a DLQ (lab provenance).
	sqsAttrNoctaxrisDlqSourceArn = "NoctaxrisDlqSourceArn"

	maxDelaySeconds = 900
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
	MessageID              string
	AccountID              string
	QueueName              string
	Body                   []byte
	Sealed                 bool
	SealedDEK              []byte
	ReceiptHandle          string
	VisibleAfter           string
	ReceiveCount           int
	CreatedAt              string
	AttributesJSON         string
	MessageGroupID         string
	MessageDeduplicationID string
	SequenceNumber         int64
}

// SendMessageOpts carries optional FIFO and delay send parameters.
type SendMessageOpts struct {
	MessageGroupID         string
	MessageDeduplicationID string
	DelaySeconds           int
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

func attrTruthy(attrs map[string]string, name string) bool {
	v, ok := attrs[name]
	return ok && strings.EqualFold(strings.TrimSpace(v), "true")
}

func queueIsFIFO(attrs map[string]string) bool {
	return attrTruthy(attrs, attrFifoQueue)
}

func queueContentBasedDedup(attrs map[string]string) bool {
	return attrTruthy(attrs, attrContentBasedDeduplication)
}

func validateFIFOQueueName(queueName string, attrs map[string]string) error {
	fifo := queueIsFIFO(attrs)
	hasSuffix := strings.HasSuffix(queueName, ".fifo")
	if fifo && !hasSuffix {
		return ErrInvalidFIFOQueueName
	}
	if hasSuffix && !fifo {
		return ErrInvalidFIFOQueueName
	}
	return nil
}

type redrivePolicy struct {
	DeadLetterTargetArn string
	MaxReceiveCount     int
}

func queueDelaySeconds(attrs map[string]string) int {
	if v, ok := attrs[attrDelaySeconds]; ok {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil && n > 0 {
			if n > maxDelaySeconds {
				return maxDelaySeconds
			}
			return n
		}
	}
	return 0
}

type redriveAllowPolicy struct {
	RedrivePermission string   // allowAll | denyAll | byQueue
	SourceQueueARNs   []string
}

func parseRedriveAllowPolicy(attrs map[string]string) (redriveAllowPolicy, bool) {
	raw, ok := attrs[attrRedriveAllowPolicy]
	if !ok || strings.TrimSpace(raw) == "" {
		return redriveAllowPolicy{RedrivePermission: "allowAll"}, false
	}
	var parsed struct {
		RedrivePermission string   `json:"redrivePermission"`
		SourceQueueArns   []string `json:"sourceQueueArns"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return redriveAllowPolicy{}, false
	}
	perm := strings.TrimSpace(parsed.RedrivePermission)
	if perm == "" {
		perm = "allowAll"
	}
	return redriveAllowPolicy{RedrivePermission: perm, SourceQueueARNs: parsed.SourceQueueArns}, true
}

func (p redriveAllowPolicy) allowsSource(sourceARN string) bool {
	switch strings.ToLower(p.RedrivePermission) {
	case "denyall":
		return false
	case "byqueue":
		for _, arn := range p.SourceQueueARNs {
			if strings.TrimSpace(arn) == sourceARN {
				return true
			}
		}
		return false
	default: // allowAll
		return true
	}
}

func parseRedrivePolicy(attrs map[string]string) (redrivePolicy, bool) {
	raw, ok := attrs[attrRedrivePolicy]
	if !ok || strings.TrimSpace(raw) == "" {
		return redrivePolicy{}, false
	}
	var parsed struct {
		DeadLetterTargetArn string `json:"deadLetterTargetArn"`
		MaxReceiveCount     any    `json:"maxReceiveCount"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return redrivePolicy{}, false
	}
	max := 0
	switch v := parsed.MaxReceiveCount.(type) {
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n <= 0 {
			return redrivePolicy{}, false
		}
		max = n
	case float64:
		if v <= 0 {
			return redrivePolicy{}, false
		}
		max = int(v)
	default:
		return redrivePolicy{}, false
	}
	if strings.TrimSpace(parsed.DeadLetterTargetArn) == "" {
		return redrivePolicy{}, false
	}
	return redrivePolicy{
		DeadLetterTargetArn: strings.TrimSpace(parsed.DeadLetterTargetArn),
		MaxReceiveCount:     max,
	}, true
}

func queueNameFromARN(arn string) (string, error) {
	i := strings.LastIndex(arn, ":")
	if i < 0 || i == len(arn)-1 {
		return "", fmt.Errorf("invalid queue arn %q", arn)
	}
	return arn[i+1:], nil
}

// queueAccountFromARN extracts the account id from arn:aws:sqs:REGION:ACCOUNT:NAME.
func queueAccountFromARN(arn string) (string, error) {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 || parts[0] != "arn" || parts[1] != "aws" || parts[2] != "sqs" {
		return "", fmt.Errorf("invalid queue arn %q", arn)
	}
	accountID := strings.TrimSpace(parts[4])
	if accountID == "" {
		return "", fmt.Errorf("invalid queue arn %q", arn)
	}
	return accountID, nil
}

func contentBasedDedupID(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// CreateQueue inserts a new queue with the given attributes.
func (s *Store) CreateQueue(accountID, region, endpointHost, queueName string, attributes map[string]string) (Queue, error) {
	if strings.TrimSpace(queueName) == "" {
		return Queue{}, fmt.Errorf("create queue: name is required")
	}
	if attributes == nil {
		attributes = map[string]string{}
	}
	if err := validateFIFOQueueName(queueName, attributes); err != nil {
		return Queue{}, err
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
	if err := validateFIFOQueueName(queueName, merged); err != nil {
		return err
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

func (s *Store) findDedupMessage(accountID, queueName, dedupID string) (Message, bool, error) {
	if dedupID == "" {
		return Message{}, false, nil
	}
	cutoff := time.Now().UTC().Add(-FIFODedupWindow).Format(time.RFC3339)
	row := s.db.QueryRow(
		`SELECT message_id, body, sealed, sealed_dek, receive_count, created_at, attributes_json,
		        message_group_id, message_deduplication_id, sequence_number
		 FROM sqs_messages
		 WHERE account_id = ? AND queue_name = ? AND message_deduplication_id = ? AND created_at >= ?
		 ORDER BY created_at DESC LIMIT 1`,
		accountID, queueName, dedupID, cutoff,
	)
	var (
		m      Message
		sealed int
	)
	err := row.Scan(
		&m.MessageID, &m.Body, &sealed, &m.SealedDEK, &m.ReceiveCount, &m.CreatedAt, &m.AttributesJSON,
		&m.MessageGroupID, &m.MessageDeduplicationID, &m.SequenceNumber,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, false, nil
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("find dedup message: %w", err)
	}
	m.AccountID = accountID
	m.QueueName = queueName
	m.Sealed = sealed == 1
	return m, true, nil
}

func (s *Store) nextSequenceNumberTx(tx *sql.Tx, accountID, queueName, messageGroupID string) (int64, error) {
	var next int64
	err := tx.QueryRow(
		`SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM sqs_messages
		 WHERE account_id = ? AND queue_name = ? AND message_group_id = ?`,
		accountID, queueName, messageGroupID,
	).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("next sequence number: %w", err)
	}
	return next, nil
}

func (s *Store) findDedupMessageTx(tx *sql.Tx, accountID, queueName, dedupID string) (Message, bool, error) {
	if dedupID == "" {
		return Message{}, false, nil
	}
	cutoff := time.Now().UTC().Add(-FIFODedupWindow).Format(time.RFC3339)
	row := tx.QueryRow(
		`SELECT message_id, body, sealed, sealed_dek, receive_count, created_at, attributes_json,
		        message_group_id, message_deduplication_id, sequence_number
		 FROM sqs_messages
		 WHERE account_id = ? AND queue_name = ? AND message_deduplication_id = ? AND created_at >= ?
		 ORDER BY created_at DESC LIMIT 1`,
		accountID, queueName, dedupID, cutoff,
	)
	var (
		m      Message
		sealed int
	)
	err := row.Scan(
		&m.MessageID, &m.Body, &sealed, &m.SealedDEK, &m.ReceiveCount, &m.CreatedAt, &m.AttributesJSON,
		&m.MessageGroupID, &m.MessageDeduplicationID, &m.SequenceNumber,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, false, nil
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("find dedup message: %w", err)
	}
	m.AccountID = accountID
	m.QueueName = queueName
	m.Sealed = sealed == 1
	return m, true, nil
}

func (s *Store) nextSequenceNumber(accountID, queueName, messageGroupID string) (int64, error) {
	var next int64
	err := s.db.QueryRow(
		`SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM sqs_messages
		 WHERE account_id = ? AND queue_name = ? AND message_group_id = ?`,
		accountID, queueName, messageGroupID,
	).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("next sequence number: %w", err)
	}
	return next, nil
}

// prepareSendFIFOFields derives MessageGroupId / MessageDeduplicationId from
// already-loaded queue attributes. Callers may run this outside sqsMu.
func prepareSendFIFOFields(attrs map[string]string, body []byte, opts *SendMessageOpts) (groupID, dedupID string, fifo bool, err error) {
	fifo = queueIsFIFO(attrs)
	if !fifo {
		return "", "", false, nil
	}
	if opts == nil || strings.TrimSpace(opts.MessageGroupID) == "" {
		return "", "", true, ErrMissingMessageGroupID
	}
	groupID = strings.TrimSpace(opts.MessageGroupID)
	if queueContentBasedDedup(attrs) {
		return groupID, contentBasedDedupID(body), true, nil
	}
	if opts != nil && strings.TrimSpace(opts.MessageDeduplicationID) != "" {
		return groupID, strings.TrimSpace(opts.MessageDeduplicationID), true, nil
	}
	return "", "", true, ErrMissingMessageDeduplication
}

func prepareSendVisibleAfter(attrs map[string]string, opts *SendMessageOpts) string {
	delay := queueDelaySeconds(attrs)
	if opts != nil && opts.DelaySeconds > 0 {
		delay = opts.DelaySeconds
		if delay > maxDelaySeconds {
			delay = maxDelaySeconds
		}
	}
	if delay <= 0 {
		return ""
	}
	return time.Now().UTC().Add(time.Duration(delay) * time.Second).Format(time.RFC3339)
}

// SendMessage stores a message on the queue. body is plaintext when sealed is
// false, or ciphertext when sealed is true. messageAttributesJSON is stored as-is.
// UUID, attribute defaulting, delay math, and FIFO field prep run outside sqsMu;
// the critical section re-loads the queue (fail closed) then performs FIFO
// dedup/sequence+INSERT or standard INSERT under the mutex.
func (s *Store) SendMessage(
	accountID, queueName string,
	body []byte,
	sealed bool,
	sealedDEK []byte,
	messageAttributesJSON string,
	opts *SendMessageOpts,
) (Message, error) {
	qSnap, err := s.GetQueue(accountID, queueName)
	if err != nil {
		return Message{}, err
	}

	messageGroupID, dedupID, fifo, err := prepareSendFIFOFields(qSnap.Attributes, body, opts)
	if err != nil {
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
	visibleAfter := prepareSendVisibleAfter(qSnap.Attributes, opts)

	s.sqsMu.Lock()
	defer s.sqsMu.Unlock()

	q, err := s.GetQueue(accountID, queueName)
	if err != nil {
		return Message{}, err
	}
	// Queue attrs may have changed between snapshot and lock; re-derive FIFO fields.
	if queueIsFIFO(q.Attributes) != fifo ||
		(fifo && queueContentBasedDedup(q.Attributes) != queueContentBasedDedup(qSnap.Attributes)) {
		messageGroupID, dedupID, fifo, err = prepareSendFIFOFields(q.Attributes, body, opts)
		if err != nil {
			return Message{}, err
		}
		visibleAfter = prepareSendVisibleAfter(q.Attributes, opts)
	}

	if fifo {
		tx, err := s.beginImmediate()
		if err != nil {
			return Message{}, err
		}
		defer tx.Rollback()

		if existing, found, findErr := s.findDedupMessageTx(tx, accountID, queueName, dedupID); findErr != nil {
			return Message{}, findErr
		} else if found {
			return existing, nil
		}
		sequenceNumber, err := s.nextSequenceNumberTx(tx, accountID, queueName, messageGroupID)
		if err != nil {
			return Message{}, err
		}
		_, err = tx.Exec(
			`INSERT INTO sqs_messages
			 (message_id, account_id, queue_name, body, sealed, sealed_dek,
			  receipt_handle, visible_after, receive_count, created_at, attributes_json,
			  message_group_id, message_deduplication_id, sequence_number)
			 VALUES (?, ?, ?, ?, ?, ?, NULL, ?, 0, ?, ?, ?, ?, ?)`,
			messageID, accountID, queueName, body, sealedFlag, sealedDEK, visibleAfter, created, attrsJSON,
			messageGroupID, dedupID, sequenceNumber,
		)
		if err != nil {
			return Message{}, fmt.Errorf("send message: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return Message{}, fmt.Errorf("send message: %w", err)
		}
		return Message{
			MessageID:              messageID,
			AccountID:              accountID,
			QueueName:              queueName,
			Body:                   body,
			Sealed:                 sealed,
			SealedDEK:              sealedDEK,
			VisibleAfter:           visibleAfter,
			ReceiveCount:           0,
			CreatedAt:              created,
			AttributesJSON:         attrsJSON,
			MessageGroupID:         messageGroupID,
			MessageDeduplicationID: dedupID,
			SequenceNumber:         sequenceNumber,
		}, nil
	}

	_, err = s.db.Exec(
		`INSERT INTO sqs_messages
		 (message_id, account_id, queue_name, body, sealed, sealed_dek,
		  receipt_handle, visible_after, receive_count, created_at, attributes_json,
		  message_group_id, message_deduplication_id, sequence_number)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, ?, 0, ?, ?, ?, ?, ?)`,
		messageID, accountID, queueName, body, sealedFlag, sealedDEK, visibleAfter, created, attrsJSON,
		messageGroupID, dedupID, int64(0),
	)
	if err != nil {
		return Message{}, fmt.Errorf("send message: %w", err)
	}
	return Message{
		MessageID:              messageID,
		AccountID:              accountID,
		QueueName:              queueName,
		Body:                   body,
		Sealed:                 sealed,
		SealedDEK:              sealedDEK,
		VisibleAfter:           visibleAfter,
		ReceiveCount:           0,
		CreatedAt:              created,
		AttributesJSON:         attrsJSON,
		MessageGroupID:         messageGroupID,
		MessageDeduplicationID: dedupID,
		SequenceNumber:         0,
	}, nil
}

// SendMessageBatch sends each body in order and returns the stored messages.
func (s *Store) SendMessageBatch(accountID, queueName string, bodies [][]byte) ([]Message, error) {
	out := make([]Message, 0, len(bodies))
	for _, body := range bodies {
		m, err := s.SendMessage(accountID, queueName, body, false, nil, "", nil)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *Store) beginImmediate() (*sql.Tx, error) {
	// Caller must hold sqsMu. Begin is deferred; sqsMu + CAS on visibility updates
	// keep concurrent receivers from double-delivering without global _txlock.
	return s.db.Begin()
}

func (s *Store) fifoBlockedGroupsTx(tx *sql.Tx, accountID, queueName, nowStr string) (map[string]struct{}, error) {
	rows, err := tx.Query(
		`SELECT DISTINCT message_group_id FROM sqs_messages
		 WHERE account_id = ? AND queue_name = ? AND message_group_id != ''
		   AND receipt_handle IS NOT NULL AND receipt_handle != ''
		   AND visible_after > ?`,
		accountID, queueName, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("fifo blocked groups: %w", err)
	}
	defer rows.Close()
	blocked := map[string]struct{}{}
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return nil, fmt.Errorf("fifo blocked groups: %w", err)
		}
		blocked[groupID] = struct{}{}
	}
	return blocked, rows.Err()
}

func (s *Store) fifoBlockedGroups(accountID, queueName, nowStr string) (map[string]struct{}, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT message_group_id FROM sqs_messages
		 WHERE account_id = ? AND queue_name = ? AND message_group_id != ''
		   AND receipt_handle IS NOT NULL AND receipt_handle != ''
		   AND visible_after > ?`,
		accountID, queueName, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("fifo blocked groups: %w", err)
	}
	defer rows.Close()
	blocked := map[string]struct{}{}
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return nil, fmt.Errorf("fifo blocked groups: %w", err)
		}
		blocked[groupID] = struct{}{}
	}
	return blocked, rows.Err()
}

func (s *Store) redriveMessage(q Queue, policy redrivePolicy, msg Message) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("redrive message: %w", err)
	}
	defer tx.Rollback()
	if err := s.redriveMessageTx(tx, q, policy, msg); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) redriveMessageTx(tx *sql.Tx, q Queue, policy redrivePolicy, msg Message) error {
	dlqName, err := queueNameFromARN(policy.DeadLetterTargetArn)
	if err != nil {
		return err
	}
	var (
		attrsJSON  string
		dlqAccount string
	)
	err = tx.QueryRow(
		`SELECT account_id, attributes_json FROM sqs_queues WHERE account_id = ? AND queue_name = ?`,
		q.AccountID, dlqName,
	).Scan(&dlqAccount, &attrsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoSuchQueue
	}
	if err != nil {
		return err
	}
	if dlqAccount != q.AccountID {
		return fmt.Errorf("dead-letter queue must be in the same account")
	}
	dlqAttrs, err := unmarshalAttributes(attrsJSON)
	if err != nil {
		return err
	}
	allow, hasAllow := parseRedriveAllowPolicy(dlqAttrs)
	if hasAllow && !allow.allowsSource(q.QueueARN) {
		return fmt.Errorf("redrive not allowed by dead-letter queue RedriveAllowPolicy")
	}

	newID := uuid.NewString()
	created := nowRFC3339()
	sealedFlag := 0
	if msg.Sealed {
		sealedFlag = 1
	}

	dlqGroupID := msg.MessageGroupID
	dlqDedupID := msg.MessageDeduplicationID
	dlqSeq := int64(0)
	if queueIsFIFO(dlqAttrs) {
		if dlqGroupID == "" {
			dlqGroupID = "redriven"
		}
		if dlqDedupID == "" {
			sum := md5.Sum([]byte(msg.MessageID))
			dlqDedupID = hex.EncodeToString(sum[:])
		}
		var seqErr error
		dlqSeq, seqErr = s.nextSequenceNumberTx(tx, q.AccountID, dlqName, dlqGroupID)
		if seqErr != nil {
			return seqErr
		}
	}

	dlqAttrsJSON, err := mergeDlqSourceProvenance(msg.AttributesJSON, q.QueueARN)
	if err != nil {
		return fmt.Errorf("redrive dlq provenance: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO sqs_messages
		 (message_id, account_id, queue_name, body, sealed, sealed_dek,
		  receipt_handle, visible_after, receive_count, created_at, attributes_json,
		  message_group_id, message_deduplication_id, sequence_number)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, '', 0, ?, ?, ?, ?, ?)`,
		newID, q.AccountID, dlqName, msg.Body, sealedFlag, msg.SealedDEK, created, dlqAttrsJSON,
		dlqGroupID, dlqDedupID, dlqSeq,
	); err != nil {
		return fmt.Errorf("redrive insert: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM sqs_messages WHERE message_id = ?`, msg.MessageID); err != nil {
		return fmt.Errorf("redrive delete: %w", err)
	}
	return nil
}

// ReceiveMessages returns up to max currently visible messages. Each returned
// message gets a fresh receipt handle, an incremented receive count, and a
// visibility deadline of now plus the queue VisibilityTimeout.
// Claims run in a single write transaction with CAS on visibility so concurrent
// receivers cannot deliver the same message twice.
func (s *Store) ReceiveMessages(accountID, queueName string, max int) ([]Message, error) {
	s.sqsMu.Lock()
	defer s.sqsMu.Unlock()

	q, err := s.GetQueue(accountID, queueName)
	if err != nil {
		return nil, err
	}
	if max <= 0 {
		max = 1
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	fifo := queueIsFIFO(q.Attributes)
	policy, hasRedrive := parseRedrivePolicy(q.Attributes)

	tx, err := s.beginImmediate()
	if err != nil {
		return nil, fmt.Errorf("receive messages: %w", err)
	}
	defer tx.Rollback()

	var blocked map[string]struct{}
	if fifo {
		blocked, err = s.fifoBlockedGroupsTx(tx, accountID, queueName, nowStr)
		if err != nil {
			return nil, err
		}
	}

	orderClause := `ORDER BY created_at, message_id`
	if fifo {
		orderClause = `ORDER BY sequence_number, message_id`
	}

	rows, err := tx.Query(
		`SELECT message_id, body, sealed, sealed_dek, receive_count, created_at, attributes_json,
		        message_group_id, message_deduplication_id, sequence_number, visible_after
		 FROM sqs_messages
		 WHERE account_id = ? AND queue_name = ? AND (visible_after = '' OR visible_after <= ?)
		 `+orderClause,
		accountID, queueName, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("receive messages: %w", err)
	}

	var pending []Message
	seenGroups := map[string]struct{}{}
	for rows.Next() {
		var (
			m      Message
			sealed int
		)
		if err := rows.Scan(
			&m.MessageID, &m.Body, &sealed, &m.SealedDEK, &m.ReceiveCount, &m.CreatedAt, &m.AttributesJSON,
			&m.MessageGroupID, &m.MessageDeduplicationID, &m.SequenceNumber, &m.VisibleAfter,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("receive messages: %w", err)
		}
		m.AccountID = accountID
		m.QueueName = queueName
		m.Sealed = sealed == 1
		if fifo {
			if _, blockedGroup := blocked[m.MessageGroupID]; blockedGroup {
				continue
			}
			if _, already := seenGroups[m.MessageGroupID]; already {
				continue
			}
			seenGroups[m.MessageGroupID] = struct{}{}
		}
		pending = append(pending, m)
		if len(pending) >= max {
			break
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("receive messages: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("receive messages: %w", err)
	}

	visTimeout := queueVisibilityTimeout(q.Attributes)
	visibleAfter := now.Add(time.Duration(visTimeout) * time.Second).Format(time.RFC3339)

	out := make([]Message, 0, len(pending))
	for _, m := range pending {
		newReceiveCount := m.ReceiveCount + 1
		if hasRedrive && newReceiveCount > policy.MaxReceiveCount {
			if err := s.redriveMessageTx(tx, q, policy, m); err != nil {
				return nil, fmt.Errorf("redrive message: %w", err)
			}
			continue
		}

		handle := uuid.NewString()
		res, err := tx.Exec(
			`UPDATE sqs_messages
			 SET receipt_handle = ?, visible_after = ?, receive_count = receive_count + 1
			 WHERE message_id = ?
			   AND (visible_after = '' OR visible_after <= ?)`,
			handle, visibleAfter, m.MessageID, nowStr,
		)
		if err != nil {
			return nil, fmt.Errorf("receive messages: %w", err)
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			continue
		}
		m.ReceiptHandle = handle
		m.VisibleAfter = visibleAfter
		m.ReceiveCount = newReceiveCount
		out = append(out, m)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("receive messages: %w", err)
	}
	return out, nil
}

// DeleteMessage removes a message by receipt handle. A stale or unknown handle
// returns ErrNoSuchMessage.
func (s *Store) DeleteMessage(accountID, queueName, receiptHandle string) error {
	s.sqsMu.Lock()
	defer s.sqsMu.Unlock()

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
	s.sqsMu.Lock()
	defer s.sqsMu.Unlock()

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

// mergeDlqSourceProvenance adds NoctaxrisDlqSourceArn to redriven message attributes.
func mergeDlqSourceProvenance(attrsJSON, sourceQueueARN string) (string, error) {
	attrs := map[string]any{}
	if trimmed := strings.TrimSpace(attrsJSON); trimmed != "" && trimmed != "{}" {
		if err := json.Unmarshal([]byte(trimmed), &attrs); err != nil {
			return "", fmt.Errorf("parse message attributes: %w", err)
		}
	}
	attrs[sqsAttrNoctaxrisDlqSourceArn] = map[string]any{
		"DataType":    "String",
		"StringValue": sourceQueueARN,
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		return "", err
	}
	return string(raw), nil
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
