package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrBedrockValidation      = errors.New("ValidationException")
	ErrBedrockResourceNotFound = errors.New("ResourceNotFoundException")
)

const DefaultBedrockRegion = "us-east-1"

// BedrockAllowlistedModels maps modelId to canned InvokeModel response body (JSON).
// Unknown modelIds are rejected fail-closed. No real model calls.
var BedrockAllowlistedModels = map[string]string{
	"amazon.titan-text-express-v1": `{"inputTextTokenCount":1,"results":[{"tokenCount":8,"outputText":"Noctaxris Bedrock stub response.","completionReason":"FINISH"}]}`,
	"amazon.titan-embed-text-v1":   `{"embedding":[0.01,0.02,0.03],"inputTextTokenCount":1}`,
	"anthropic.claude-3-haiku-20240307-v1:0": `{"id":"msg_stub","type":"message","role":"assistant","content":[{"type":"text","text":"Noctaxris Bedrock stub response."}],"model":"anthropic.claude-3-haiku-20240307-v1:0","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":8}}`,
}

const bedrockSchema = `
CREATE TABLE IF NOT EXISTS bedrock_invocations (
  account_id TEXT NOT NULL,
  invocation_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT 'application/json',
  request_bytes INTEGER NOT NULL DEFAULT 0,
  response_body TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, invocation_id)
);
CREATE INDEX IF NOT EXISTS idx_bedrock_invocations_model ON bedrock_invocations(account_id, model_id);
`

// BedrockInvocation is a persisted InvokeModel stub record.
type BedrockInvocation struct {
	InvocationID string
	ModelID      string
	ContentType  string
	RequestBytes int
	ResponseBody string
	CreatedAt    int64
}

// EnsureBedrockSchema creates Bedrock Runtime tables if missing.
func EnsureBedrockSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure bedrock schema: db is nil")
	}
	if _, err := db.Exec(bedrockSchema); err != nil {
		return fmt.Errorf("ensure bedrock schema: %w", err)
	}
	return nil
}

// EnsureBedrockSchema ensures Bedrock tables on an open store.
func (s *Store) EnsureBedrockSchema() error {
	return EnsureBedrockSchema(s.db)
}

// BedrockCannedBody returns the canned response for an allowlisted modelId.
func BedrockCannedBody(modelID string) (string, bool) {
	body, ok := BedrockAllowlistedModels[strings.TrimSpace(modelID)]
	return body, ok
}

// InvokeBedrockModel validates modelId against the allowlist, persists metadata,
// and returns the canned response body. Never calls external model APIs.
func (s *Store) InvokeBedrockModel(accountID, modelID, contentType string, requestBody []byte) (BedrockInvocation, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return BedrockInvocation{}, fmt.Errorf("%w: modelId is required", ErrBedrockValidation)
	}
	canned, ok := BedrockCannedBody(modelID)
	if !ok {
		return BedrockInvocation{}, fmt.Errorf("%w: modelId %q is not allowlisted", ErrBedrockResourceNotFound, modelID)
	}
	if contentType == "" {
		contentType = "application/json"
	}
	id := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO bedrock_invocations
		 (account_id, invocation_id, model_id, content_type, request_bytes, response_body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, id, modelID, contentType, len(requestBody), canned, now,
	)
	if err != nil {
		return BedrockInvocation{}, fmt.Errorf("invoke bedrock model: insert: %w", err)
	}
	return BedrockInvocation{
		InvocationID: id,
		ModelID:      modelID,
		ContentType:  contentType,
		RequestBytes: len(requestBody),
		ResponseBody: canned,
		CreatedAt:    now,
	}, nil
}

// GetBedrockInvocation returns a stored invocation by id.
func (s *Store) GetBedrockInvocation(accountID, invocationID string) (BedrockInvocation, error) {
	var inv BedrockInvocation
	err := s.db.QueryRow(
		`SELECT invocation_id, model_id, content_type, request_bytes, response_body, created_at
		 FROM bedrock_invocations WHERE account_id = ? AND invocation_id = ?`,
		accountID, invocationID,
	).Scan(&inv.InvocationID, &inv.ModelID, &inv.ContentType, &inv.RequestBytes, &inv.ResponseBody, &inv.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BedrockInvocation{}, ErrBedrockResourceNotFound
	}
	if err != nil {
		return BedrockInvocation{}, fmt.Errorf("get bedrock invocation: %w", err)
	}
	return inv, nil
}
