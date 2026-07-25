package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrCognitoInvalidLambdaResponse is returned when a trigger response fails lab validation.
var ErrCognitoInvalidLambdaResponse = errors.New("InvalidLambdaResponseException")

// CognitoCustomMessage is the lab-rendered CustomMessage body (no SES send).
type CognitoCustomMessage struct {
	AccountID     string
	PoolID        string
	Username      string
	TriggerSource string
	SMSMessage    string
	EmailMessage  string
	EmailSubject  string
	CreatedAt     int64
}

const cognitoCustomMessageSchema = `
CREATE TABLE IF NOT EXISTS cognito_custom_messages (
  account_id TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  username TEXT NOT NULL,
  trigger_source TEXT NOT NULL,
  sms_message TEXT NOT NULL DEFAULT '',
  email_message TEXT NOT NULL DEFAULT '',
  email_subject TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, pool_id, username)
);
`

// EnsureCognitoCustomMessageSchema stores last rendered CustomMessage per user.
func EnsureCognitoCustomMessageSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cognito custom message schema: db is nil")
	}
	if _, err := db.Exec(cognitoCustomMessageSchema); err != nil {
		return fmt.Errorf("ensure cognito custom message schema: %w", err)
	}
	return nil
}

func (s *Store) EnsureCognitoCustomMessageSchema() error {
	return EnsureCognitoCustomMessageSchema(s.db)
}

// CognitoCustomMessageBodies holds parsed CustomMessage response fields.
type CognitoCustomMessageBodies struct {
	SMSMessage   string
	EmailMessage string
	EmailSubject string
}

// ParseCognitoCustomMessageBodies reads response smsMessage / emailMessage / emailSubject.
func ParseCognitoCustomMessageBodies(payload []byte) (CognitoCustomMessageBodies, error) {
	out := CognitoCustomMessageBodies{}
	if len(strings.TrimSpace(string(payload))) == 0 {
		return out, nil
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return CognitoCustomMessageBodies{}, fmt.Errorf("parse custom message payload: %w", err)
	}
	resp := root
	if nested, ok := root["response"].(map[string]any); ok {
		resp = nested
	}
	if v, ok := resp["smsMessage"].(string); ok {
		out.SMSMessage = v
	}
	if v, ok := resp["emailMessage"].(string); ok {
		out.EmailMessage = v
	}
	if v, ok := resp["emailSubject"].(string); ok {
		out.EmailSubject = v
	}
	return out, nil
}

// RenderCognitoCustomMessage substitutes codeParameter and username placeholders
// with confirmationCode (the issued code value).
func RenderCognitoCustomMessage(bodies CognitoCustomMessageBodies, codeParameter, username, confirmationCode string) CognitoCustomMessageBodies {
	code := strings.TrimSpace(codeParameter)
	if code == "" {
		code = "{####}"
	}
	issued := strings.TrimSpace(confirmationCode)
	if issued == "" {
		issued = CognitoLabConfirmationCode
	}
	userParam := "{username}"
	repl := func(s string) string {
		s = strings.ReplaceAll(s, code, issued)
		s = strings.ReplaceAll(s, "{####}", issued)
		s = strings.ReplaceAll(s, userParam, username)
		return s
	}
	return CognitoCustomMessageBodies{
		SMSMessage:   repl(bodies.SMSMessage),
		EmailMessage: repl(bodies.EmailMessage),
		EmailSubject: repl(bodies.EmailSubject),
	}
}

// ValidateCognitoCustomMessageBodies enforces AWS-shaped codeParameter inclusion when a body is set.
func ValidateCognitoCustomMessageBodies(bodies CognitoCustomMessageBodies, codeParameter string) error {
	code := strings.TrimSpace(codeParameter)
	if code == "" {
		code = "{####}"
	}
	check := func(field, value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if !strings.Contains(value, code) && !strings.Contains(value, "{####}") {
			return fmt.Errorf("%w: %s must include codeParameter", ErrCognitoInvalidLambdaResponse, field)
		}
		return nil
	}
	if err := check("smsMessage", bodies.SMSMessage); err != nil {
		return err
	}
	if err := check("emailMessage", bodies.EmailMessage); err != nil {
		return err
	}
	return nil
}

func (s *Store) storeCognitoCustomMessage(
	accountID, poolID, username, triggerSource string, bodies CognitoCustomMessageBodies,
) error {
	if err := s.EnsureCognitoCustomMessageSchema(); err != nil {
		return err
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO cognito_custom_messages
		 (account_id, pool_id, username, trigger_source, sms_message, email_message, email_subject, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, pool_id, username) DO UPDATE SET
		   trigger_source = excluded.trigger_source,
		   sms_message = excluded.sms_message,
		   email_message = excluded.email_message,
		   email_subject = excluded.email_subject,
		   created_at = excluded.created_at`,
		accountID, poolID, username, triggerSource,
		bodies.SMSMessage, bodies.EmailMessage, bodies.EmailSubject, now,
	)
	if err != nil {
		return fmt.Errorf("store custom message: %w", err)
	}
	return nil
}

// GetLastCognitoCustomMessage returns the last rendered CustomMessage for a user (lab test helper).
func (s *Store) GetLastCognitoCustomMessage(accountID, poolID, username string) (CognitoCustomMessage, error) {
	if err := s.EnsureCognitoCustomMessageSchema(); err != nil {
		return CognitoCustomMessage{}, err
	}
	var row CognitoCustomMessage
	err := s.db.QueryRow(
		`SELECT account_id, pool_id, username, trigger_source, sms_message, email_message, email_subject, created_at
		 FROM cognito_custom_messages
		 WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	).Scan(
		&row.AccountID, &row.PoolID, &row.Username, &row.TriggerSource,
		&row.SMSMessage, &row.EmailMessage, &row.EmailSubject, &row.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CognitoCustomMessage{}, ErrCognitoNotFound
	}
	if err != nil {
		return CognitoCustomMessage{}, fmt.Errorf("get custom message: %w", err)
	}
	return row, nil
}

// applyCustomMessagePayload validates, renders, and stores CustomMessage response bodies.
func (s *Store) applyCustomMessagePayload(
	accountID, poolID, username, triggerSource, codeParameter, confirmationCode string, payload []byte,
) error {
	if len(payload) == 0 {
		return nil
	}
	bodies, err := ParseCognitoCustomMessageBodies(payload)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCognitoInvalidLambdaResponse, err)
	}
	if bodies.SMSMessage == "" && bodies.EmailMessage == "" && bodies.EmailSubject == "" {
		return nil
	}
	if err := ValidateCognitoCustomMessageBodies(bodies, codeParameter); err != nil {
		return err
	}
	rendered := RenderCognitoCustomMessage(bodies, codeParameter, username, confirmationCode)
	return s.storeCognitoCustomMessage(accountID, poolID, username, triggerSource, rendered)
}
