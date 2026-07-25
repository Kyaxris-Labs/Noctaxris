package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSESIdentityNotFound = errors.New("MessageRejected")
)

const sesSchema = `
CREATE TABLE IF NOT EXISTS ses_identities (
  account_id TEXT NOT NULL,
  identity TEXT NOT NULL,
  identity_type TEXT NOT NULL,
  verified INTEGER NOT NULL DEFAULT 1,
  bounce_topic_arn TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, identity)
);
CREATE TABLE IF NOT EXISTS ses_messages (
  account_id TEXT NOT NULL,
  message_id TEXT NOT NULL,
  source TEXT NOT NULL,
  destination TEXT NOT NULL,
  subject TEXT NOT NULL,
  body_text TEXT NOT NULL,
  body_html TEXT NOT NULL,
  raw_data TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, message_id)
);
CREATE INDEX IF NOT EXISTS idx_ses_messages_created ON ses_messages(account_id, created_at);
`

// SESIdentity is a verified email or domain identity.
type SESIdentity struct {
	Identity       string
	Type           string // EmailAddress or Domain
	Verified       bool
	BounceTopicARN string
}

// SESMessage is a caught outbound email.
type SESMessage struct {
	MessageID   string
	Source      string
	Destination string // comma-joined recipients
	Subject     string
	BodyText    string
	BodyHTML    string
	RawData     string
	CreatedAt   int64
}

// SESSendStatistics is a GetSendStatistics stub over caught messages.
type SESSendStatistics struct {
	Timestamp            int64
	DeliveryAttempts     int64
	Bounces              int64
	Complaints           int64
	Rejects              int64
}

// EnsureSESSchema creates SES tables if missing.
func EnsureSESSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure ses schema: db is nil")
	}
	if _, err := db.Exec(sesSchema); err != nil {
		return fmt.Errorf("ensure ses schema: %w", err)
	}
	if err := execMigrateStmt(db, `ALTER TABLE ses_identities ADD COLUMN bounce_topic_arn TEXT NOT NULL DEFAULT ''`, nil); err != nil {
		return fmt.Errorf("ensure ses schema: bounce_topic_arn: %w", err)
	}
	return nil
}

// EnsureSESSchema ensures SES tables on an open store.
func (s *Store) EnsureSESSchema() error {
	return EnsureSESSchema(s.db)
}

// VerifySESEmailIdentity lab-auto-verifies an email identity.
func (s *Store) VerifySESEmailIdentity(accountID, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || !strings.Contains(email, "@") {
		return fmt.Errorf("verify email identity: EmailAddress is required")
	}
	var existing string
	err := s.db.QueryRow(
		`SELECT identity FROM ses_identities WHERE account_id = ? AND identity = ?`,
		accountID, email,
	).Scan(&existing)
	if err == nil {
		return nil // idempotent verify
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("verify email identity: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO ses_identities (account_id, identity, identity_type, verified, created_at) VALUES (?, ?, 'EmailAddress', 1, ?)`,
		accountID, email, now,
	)
	if err != nil {
		return fmt.Errorf("verify email identity: insert: %w", err)
	}
	return nil
}

// ListSESIdentities lists identities, optional type filter (EmailAddress|Domain|"").
func (s *Store) ListSESIdentities(accountID, identityType string) ([]SESIdentity, error) {
	rows, err := s.db.Query(
		`SELECT identity, identity_type, verified, bounce_topic_arn FROM ses_identities WHERE account_id = ? ORDER BY identity`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list ses identities: %w", err)
	}
	defer rows.Close()
	var out []SESIdentity
	for rows.Next() {
		var id SESIdentity
		var verified int
		if err := rows.Scan(&id.Identity, &id.Type, &verified, &id.BounceTopicARN); err != nil {
			return nil, fmt.Errorf("list ses identities: scan: %w", err)
		}
		id.Verified = verified == 1
		if identityType != "" && !strings.EqualFold(id.Type, identityType) {
			continue
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetSESIdentityNotificationTopic stores a Bounce SNS topic ARN for an identity (lab Bounce only).
func (s *Store) SetSESIdentityNotificationTopic(accountID, identity, notificationType, topicARN string) error {
	identity = strings.TrimSpace(strings.ToLower(identity))
	notificationType = strings.TrimSpace(notificationType)
	topicARN = strings.TrimSpace(topicARN)
	if identity == "" {
		return fmt.Errorf("set identity notification topic: Identity is required")
	}
	if !strings.EqualFold(notificationType, "Bounce") {
		return fmt.Errorf("set identity notification topic: only NotificationType Bounce is supported in lab")
	}
	if topicARN != "" && !strings.HasPrefix(topicARN, "arn:aws:sns:") {
		return fmt.Errorf("set identity notification topic: SnsTopic must be an SNS topic ARN")
	}
	if !s.sesIdentityVerified(accountID, identity) {
		return ErrSESIdentityNotFound
	}
	if topicARN != "" {
		topic, err := s.GetTopicByARN(topicARN)
		if err != nil || topic.AccountID != accountID {
			return fmt.Errorf("set identity notification topic: SNS topic not found in account")
		}
	}
	_, err := s.db.Exec(
		`UPDATE ses_identities SET bounce_topic_arn = ? WHERE account_id = ? AND identity = ?`,
		topicARN, accountID, identity,
	)
	if err != nil {
		return fmt.Errorf("set identity notification topic: %w", err)
	}
	return nil
}

// SendSESEmail persists a caught email. Source must be a verified identity (lab).
// Destinations that contain "bounce@" (case-insensitive) trigger a Bounce SNS publish when configured.
func (s *Store) SendSESEmail(accountID, source string, destinations []string, subject, bodyText, bodyHTML string) (string, error) {
	source = strings.TrimSpace(strings.ToLower(source))
	if source == "" {
		return "", fmt.Errorf("send email: Source is required")
	}
	if !s.sesIdentityVerified(accountID, source) {
		return "", ErrSESIdentityNotFound
	}
	dest := make([]string, 0, len(destinations))
	for _, d := range destinations {
		d = strings.TrimSpace(d)
		if d != "" {
			dest = append(dest, d)
		}
	}
	if len(dest) == 0 {
		return "", fmt.Errorf("send email: Destination is required")
	}
	msgID := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO ses_messages (account_id, message_id, source, destination, subject, body_text, body_html, raw_data, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '', ?)`,
		accountID, msgID, source, strings.Join(dest, ","), subject, bodyText, bodyHTML, now,
	)
	if err != nil {
		return "", fmt.Errorf("send email: %w", err)
	}
	s.maybePublishSESBounce(accountID, source, dest, msgID)
	return msgID, nil
}

// SendSESRawEmail persists a raw MIME message.
func (s *Store) SendSESRawEmail(accountID, source string, destinations []string, rawData string) (string, error) {
	source = strings.TrimSpace(strings.ToLower(source))
	if source == "" {
		// Try to parse From: from raw (lab lite: require Source param).
		return "", fmt.Errorf("send raw email: Source is required")
	}
	if !s.sesIdentityVerified(accountID, source) {
		return "", ErrSESIdentityNotFound
	}
	dest := make([]string, 0, len(destinations))
	for _, d := range destinations {
		d = strings.TrimSpace(d)
		if d != "" {
			dest = append(dest, d)
		}
	}
	msgID := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO ses_messages (account_id, message_id, source, destination, subject, body_text, body_html, raw_data, created_at)
		 VALUES (?, ?, ?, ?, '', '', '', ?, ?)`,
		accountID, msgID, source, strings.Join(dest, ","), rawData, now,
	)
	if err != nil {
		return "", fmt.Errorf("send raw email: %w", err)
	}
	return msgID, nil
}

// CountSESMessages returns caught message count for GetSendStatistics.
func (s *Store) CountSESMessages(accountID string) (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM ses_messages WHERE account_id = ?`, accountID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count ses messages: %w", err)
	}
	return n, nil
}

// GetSESSendStatistics returns a single datapoint stub.
func (s *Store) GetSESSendStatistics(accountID string) (SESSendStatistics, error) {
	n, err := s.CountSESMessages(accountID)
	if err != nil {
		return SESSendStatistics{}, err
	}
	return SESSendStatistics{
		Timestamp:        time.Now().UTC().Unix(),
		DeliveryAttempts: n,
	}, nil
}

// ListSESMessages returns caught messages (lab helper for tests).
func (s *Store) ListSESMessages(accountID string) ([]SESMessage, error) {
	rows, err := s.db.Query(
		`SELECT message_id, source, destination, subject, body_text, body_html, raw_data, created_at
		 FROM ses_messages WHERE account_id = ? ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list ses messages: %w", err)
	}
	defer rows.Close()
	var out []SESMessage
	for rows.Next() {
		var m SESMessage
		if err := rows.Scan(&m.MessageID, &m.Source, &m.Destination, &m.Subject, &m.BodyText, &m.BodyHTML, &m.RawData, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("list ses messages: scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) sesIdentityVerified(accountID, email string) bool {
	var verified int
	err := s.db.QueryRow(
		`SELECT verified FROM ses_identities WHERE account_id = ? AND identity = ?`,
		accountID, email,
	).Scan(&verified)
	return err == nil && verified == 1
}

func (s *Store) sesBounceTopicARN(accountID, identity string) string {
	var arn string
	_ = s.db.QueryRow(
		`SELECT bounce_topic_arn FROM ses_identities WHERE account_id = ? AND identity = ?`,
		accountID, identity,
	).Scan(&arn)
	return strings.TrimSpace(arn)
}

func (s *Store) maybePublishSESBounce(accountID, source string, destinations []string, messageID string) {
	var bounced []string
	for _, d := range destinations {
		if strings.Contains(strings.ToLower(d), "bounce@") {
			bounced = append(bounced, d)
		}
	}
	if len(bounced) == 0 {
		return
	}
	topicARN := s.sesBounceTopicARN(accountID, source)
	if topicARN == "" {
		return
	}
	topic, err := s.GetTopicByARN(topicARN)
	if err != nil || topic.AccountID != accountID {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"notificationType": "Bounce",
		"mail": map[string]any{
			"messageId":   messageID,
			"source":      source,
			"destination": bounced,
		},
		"bounce": map[string]any{
			"bounceType":        "Permanent",
			"bouncedRecipients": bounced,
		},
	})
	if err != nil {
		return
	}
	_, _ = s.Publish(accountID, topic.TopicName, string(payload), "Amazon SES Email Event Message", nil)
}
