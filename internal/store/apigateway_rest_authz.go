package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	APIGatewayRESTAuthCUSTOM  = "CUSTOM"
	APIGatewayRESTAuthTOKEN   = "TOKEN"
	APIGatewayRESTAuthREQUEST = "REQUEST"
)

const apiGatewayRESTAuthzSchema = `
CREATE TABLE IF NOT EXISTS apigw_rest_authorizers (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  authorizer_id TEXT NOT NULL,
  name TEXT NOT NULL,
  authorizer_type TEXT NOT NULL,
  authorizer_uri TEXT NOT NULL,
  identity_source TEXT NOT NULL DEFAULT '',
  authorizer_credentials TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, api_id, authorizer_id)
);
CREATE TABLE IF NOT EXISTS apigw_api_keys (
  account_id TEXT NOT NULL,
  api_key_id TEXT NOT NULL,
  name TEXT NOT NULL,
  key_hash TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, api_key_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_apigw_api_keys_hash ON apigw_api_keys(key_hash);
CREATE TABLE IF NOT EXISTS apigw_usage_plans (
  account_id TEXT NOT NULL,
  usage_plan_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, usage_plan_id)
);
CREATE TABLE IF NOT EXISTS apigw_usage_plan_stages (
  account_id TEXT NOT NULL,
  usage_plan_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  stage TEXT NOT NULL,
  PRIMARY KEY (account_id, usage_plan_id, api_id, stage)
);
CREATE TABLE IF NOT EXISTS apigw_usage_plan_keys (
  account_id TEXT NOT NULL,
  usage_plan_id TEXT NOT NULL,
  api_key_id TEXT NOT NULL,
  PRIMARY KEY (account_id, usage_plan_id, api_key_id)
);
`

func ensureAPIGatewayRESTAuthzSchema(db *sql.DB) error {
	if _, err := db.Exec(apiGatewayRESTAuthzSchema); err != nil {
		return fmt.Errorf("ensure apigateway rest authz schema: %w", err)
	}
	return nil
}

// RestAuthorizer is a REST API TOKEN or REQUEST Lambda authorizer.
type RestAuthorizer struct {
	AuthorizerID          string
	APIID                 string
	Name                  string
	Type                  string
	AuthorizerURI         string
	IdentitySource        string
	AuthorizerCredentials string
}

// CreateRestAuthorizerInput holds CreateAuthorizer fields for REST APIs.
type CreateRestAuthorizerInput struct {
	AccountID             string
	APIID                 string
	Name                  string
	Type                  string
	AuthorizerURI         string
	IdentitySource        string
	AuthorizerCredentials string
}

// RestAPIKey is an account-scoped API Gateway API key.
type RestAPIKey struct {
	ID        string
	Name      string
	Value     string // plaintext only on Create; empty on Get/List
	Enabled   bool
	CreatedAt int64
}

// RestUsagePlan is a usage plan with associated API stages.
type RestUsagePlan struct {
	ID          string
	Name        string
	Description string
	CreatedAt   int64
	APIStages   []RestUsagePlanStage
}

// RestUsagePlanStage binds a usage plan to an API stage.
type RestUsagePlanStage struct {
	APIID string
	Stage string
}

// RestUsagePlanKey associates an API key with a usage plan.
type RestUsagePlanKey struct {
	ID    string // api key id
	Type  string
	Value string // api key id (AWS shape)
}

// CreateRestAuthorizer creates a TOKEN or REQUEST Lambda authorizer on a REST API.
func (s *Store) CreateRestAuthorizer(in CreateRestAuthorizerInput) (RestAuthorizer, error) {
	name := strings.TrimSpace(in.Name)
	authType := strings.ToUpper(strings.TrimSpace(in.Type))
	uri := strings.TrimSpace(in.AuthorizerURI)
	identity := strings.TrimSpace(in.IdentitySource)
	cred := strings.TrimSpace(in.AuthorizerCredentials)
	if name == "" {
		return RestAuthorizer{}, fmt.Errorf("%w: name required", ErrAPIGatewayBadRequest)
	}
	switch authType {
	case APIGatewayAuthorizerTOKEN, APIGatewayAuthorizerREQUEST:
	default:
		return RestAuthorizer{}, fmt.Errorf("%w: type must be TOKEN or REQUEST", ErrAPIGatewayBadRequest)
	}
	normalized, err := NormalizeAPIGatewayAuthorizerURI(uri)
	if err != nil {
		return RestAuthorizer{}, fmt.Errorf("%w: %v", ErrAPIGatewayBadRequest, err)
	}
	if identity == "" {
		identity = "method.request.header.Authorization"
	}
	if !isValidRestAuthorizerIdentitySource(authType, identity) {
		return RestAuthorizer{}, fmt.Errorf("%w: identitySource must be method.request.header.* or method.request.querystring.* (REQUEST)", ErrAPIGatewayBadRequest)
	}
	if _, err := s.GetRestAPI(in.AccountID, in.APIID); err != nil {
		return RestAuthorizer{}, err
	}
	id := newAPIGatewayRESTShortID()
	_, err = s.db.Exec(
		`INSERT INTO apigw_rest_authorizers
		 (account_id, api_id, authorizer_id, name, authorizer_type, authorizer_uri, identity_source, authorizer_credentials)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		in.AccountID, in.APIID, id, name, authType, normalized, identity, cred,
	)
	if err != nil {
		return RestAuthorizer{}, fmt.Errorf("create rest authorizer: %w", err)
	}
	return RestAuthorizer{
		AuthorizerID: id, APIID: in.APIID, Name: name, Type: authType,
		AuthorizerURI: normalized, IdentitySource: identity, AuthorizerCredentials: cred,
	}, nil
}

func isValidRestAuthorizerIdentitySource(authType, src string) bool {
	src = strings.TrimSpace(src)
	switch {
	case strings.HasPrefix(src, "method.request.header.") && len(src) > len("method.request.header."):
		return true
	case strings.HasPrefix(src, "$request.header.") && len(src) > len("$request.header."):
		return true
	case authType == APIGatewayAuthorizerREQUEST &&
		strings.HasPrefix(src, "method.request.querystring.") && len(src) > len("method.request.querystring."):
		return true
	case authType == APIGatewayAuthorizerREQUEST &&
		strings.HasPrefix(src, "$request.querystring.") && len(src) > len("$request.querystring."):
		return true
	case authType == APIGatewayAuthorizerTOKEN && !strings.Contains(src, ".") && src != "":
		// bare header name (e.g. Authorization)
		return true
	default:
		return false
	}
}

// GetRestAuthorizer returns one authorizer.
func (s *Store) GetRestAuthorizer(accountID, apiID, authorizerID string) (RestAuthorizer, error) {
	var a RestAuthorizer
	err := s.db.QueryRow(
		`SELECT authorizer_id, api_id, name, authorizer_type, authorizer_uri, identity_source, authorizer_credentials
		 FROM apigw_rest_authorizers
		 WHERE account_id = ? AND api_id = ? AND authorizer_id = ?`,
		accountID, apiID, authorizerID,
	).Scan(&a.AuthorizerID, &a.APIID, &a.Name, &a.Type, &a.AuthorizerURI, &a.IdentitySource, &a.AuthorizerCredentials)
	if err == sql.ErrNoRows {
		return RestAuthorizer{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return RestAuthorizer{}, fmt.Errorf("get rest authorizer: %w", err)
	}
	return a, nil
}

// ListRestAuthorizers lists authorizers for a REST API.
func (s *Store) ListRestAuthorizers(accountID, apiID string) ([]RestAuthorizer, error) {
	if _, err := s.GetRestAPI(accountID, apiID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT authorizer_id, api_id, name, authorizer_type, authorizer_uri, identity_source, authorizer_credentials
		 FROM apigw_rest_authorizers WHERE account_id = ? AND api_id = ? ORDER BY name`,
		accountID, apiID,
	)
	if err != nil {
		return nil, fmt.Errorf("list rest authorizers: %w", err)
	}
	defer rows.Close()
	var out []RestAuthorizer
	for rows.Next() {
		var a RestAuthorizer
		if err := rows.Scan(&a.AuthorizerID, &a.APIID, &a.Name, &a.Type, &a.AuthorizerURI, &a.IdentitySource, &a.AuthorizerCredentials); err != nil {
			return nil, fmt.Errorf("list rest authorizers scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteRestAuthorizer deletes an authorizer.
func (s *Store) DeleteRestAuthorizer(accountID, apiID, authorizerID string) error {
	res, err := s.db.Exec(
		`DELETE FROM apigw_rest_authorizers WHERE account_id = ? AND api_id = ? AND authorizer_id = ?`,
		accountID, apiID, authorizerID,
	)
	if err != nil {
		return fmt.Errorf("delete rest authorizer: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAPIGatewayNotFound
	}
	return nil
}

// CreateRestAPIKey creates an API key; plaintext value is returned once.
func (s *Store) CreateRestAPIKey(accountID, name string, enabled bool) (RestAPIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return RestAPIKey{}, fmt.Errorf("%w: name required", ErrAPIGatewayBadRequest)
	}
	id := newAPIGatewayRESTShortID()
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return RestAPIKey{}, fmt.Errorf("generate api key: %w", err)
	}
	value := hex.EncodeToString(raw)
	now := time.Now().UTC().UnixMilli()
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO apigw_api_keys (account_id, api_key_id, name, key_hash, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, id, name, s.hashRestAPIKey(value), en, now,
	)
	if err != nil {
		return RestAPIKey{}, fmt.Errorf("create api key: %w", err)
	}
	return RestAPIKey{ID: id, Name: name, Value: value, Enabled: enabled, CreatedAt: now}, nil
}

func (s *Store) hashRestAPIKey(value string) string {
	mac := hmac.New(sha256.New, s.master[:])
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// GetRestAPIKey returns an API key without the plaintext value.
func (s *Store) GetRestAPIKey(accountID, apiKeyID string) (RestAPIKey, error) {
	var k RestAPIKey
	var en int
	err := s.db.QueryRow(
		`SELECT api_key_id, name, enabled, created_at FROM apigw_api_keys
		 WHERE account_id = ? AND api_key_id = ?`,
		accountID, apiKeyID,
	).Scan(&k.ID, &k.Name, &en, &k.CreatedAt)
	if err == sql.ErrNoRows {
		return RestAPIKey{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return RestAPIKey{}, fmt.Errorf("get api key: %w", err)
	}
	k.Enabled = en != 0
	return k, nil
}

// ListRestAPIKeys lists API keys for an account (no plaintext values).
func (s *Store) ListRestAPIKeys(accountID string) ([]RestAPIKey, error) {
	rows, err := s.db.Query(
		`SELECT api_key_id, name, enabled, created_at FROM apigw_api_keys
		 WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()
	var out []RestAPIKey
	for rows.Next() {
		var k RestAPIKey
		var en int
		if err := rows.Scan(&k.ID, &k.Name, &en, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("list api keys scan: %w", err)
		}
		k.Enabled = en != 0
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteRestAPIKey deletes an API key and usage-plan associations.
func (s *Store) DeleteRestAPIKey(accountID, apiKeyID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete api key begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM apigw_api_keys WHERE account_id = ? AND api_key_id = ?`, accountID, apiKeyID)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAPIGatewayNotFound
	}
	_, _ = tx.Exec(`DELETE FROM apigw_usage_plan_keys WHERE account_id = ? AND api_key_id = ?`, accountID, apiKeyID)
	return tx.Commit()
}

// CreateRestUsagePlan creates a usage plan with optional API stages.
func (s *Store) CreateRestUsagePlan(accountID, name, description string, stages []RestUsagePlanStage) (RestUsagePlan, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return RestUsagePlan{}, fmt.Errorf("%w: name required", ErrAPIGatewayBadRequest)
	}
	for _, st := range stages {
		apiID := strings.TrimSpace(st.APIID)
		stage := strings.TrimSpace(st.Stage)
		if apiID == "" || stage == "" {
			return RestUsagePlan{}, fmt.Errorf("%w: apiStages require apiId and stage", ErrAPIGatewayBadRequest)
		}
		if _, err := s.GetRestAPI(accountID, apiID); err != nil {
			return RestUsagePlan{}, err
		}
		if _, err := s.GetRestStage(accountID, apiID, stage); err != nil {
			return RestUsagePlan{}, err
		}
	}
	id := newAPIGatewayRESTShortID()
	now := time.Now().UTC().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return RestUsagePlan{}, fmt.Errorf("create usage plan begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`INSERT INTO apigw_usage_plans (account_id, usage_plan_id, name, description, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, id, name, description, now,
	); err != nil {
		return RestUsagePlan{}, fmt.Errorf("create usage plan: %w", err)
	}
	normStages := make([]RestUsagePlanStage, 0, len(stages))
	for _, st := range stages {
		apiID := strings.TrimSpace(st.APIID)
		stage := strings.TrimSpace(st.Stage)
		if _, err := tx.Exec(
			`INSERT INTO apigw_usage_plan_stages (account_id, usage_plan_id, api_id, stage)
			 VALUES (?, ?, ?, ?)`,
			accountID, id, apiID, stage,
		); err != nil {
			return RestUsagePlan{}, fmt.Errorf("create usage plan stage: %w", err)
		}
		normStages = append(normStages, RestUsagePlanStage{APIID: apiID, Stage: stage})
	}
	if err := tx.Commit(); err != nil {
		return RestUsagePlan{}, fmt.Errorf("create usage plan commit: %w", err)
	}
	return RestUsagePlan{ID: id, Name: name, Description: description, CreatedAt: now, APIStages: normStages}, nil
}

func (s *Store) loadRestUsagePlanStages(accountID, usagePlanID string) ([]RestUsagePlanStage, error) {
	rows, err := s.db.Query(
		`SELECT api_id, stage FROM apigw_usage_plan_stages
		 WHERE account_id = ? AND usage_plan_id = ? ORDER BY api_id, stage`,
		accountID, usagePlanID,
	)
	if err != nil {
		return nil, fmt.Errorf("list usage plan stages: %w", err)
	}
	defer rows.Close()
	var out []RestUsagePlanStage
	for rows.Next() {
		var st RestUsagePlanStage
		if err := rows.Scan(&st.APIID, &st.Stage); err != nil {
			return nil, fmt.Errorf("list usage plan stages scan: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// GetRestUsagePlan returns a usage plan.
func (s *Store) GetRestUsagePlan(accountID, usagePlanID string) (RestUsagePlan, error) {
	var p RestUsagePlan
	err := s.db.QueryRow(
		`SELECT usage_plan_id, name, description, created_at FROM apigw_usage_plans
		 WHERE account_id = ? AND usage_plan_id = ?`,
		accountID, usagePlanID,
	).Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return RestUsagePlan{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return RestUsagePlan{}, fmt.Errorf("get usage plan: %w", err)
	}
	stages, err := s.loadRestUsagePlanStages(accountID, usagePlanID)
	if err != nil {
		return RestUsagePlan{}, err
	}
	p.APIStages = stages
	return p, nil
}

// ListRestUsagePlans lists usage plans for an account.
func (s *Store) ListRestUsagePlans(accountID string) ([]RestUsagePlan, error) {
	rows, err := s.db.Query(
		`SELECT usage_plan_id, name, description, created_at FROM apigw_usage_plans
		 WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list usage plans: %w", err)
	}
	defer rows.Close()
	var out []RestUsagePlan
	for rows.Next() {
		var p RestUsagePlan
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list usage plans scan: %w", err)
		}
		stages, err := s.loadRestUsagePlanStages(accountID, p.ID)
		if err != nil {
			return nil, err
		}
		p.APIStages = stages
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteRestUsagePlan deletes a usage plan and associations.
func (s *Store) DeleteRestUsagePlan(accountID, usagePlanID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete usage plan begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM apigw_usage_plans WHERE account_id = ? AND usage_plan_id = ?`, accountID, usagePlanID)
	if err != nil {
		return fmt.Errorf("delete usage plan: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAPIGatewayNotFound
	}
	_, _ = tx.Exec(`DELETE FROM apigw_usage_plan_stages WHERE account_id = ? AND usage_plan_id = ?`, accountID, usagePlanID)
	_, _ = tx.Exec(`DELETE FROM apigw_usage_plan_keys WHERE account_id = ? AND usage_plan_id = ?`, accountID, usagePlanID)
	return tx.Commit()
}

// CreateRestUsagePlanKey associates an API key with a usage plan.
func (s *Store) CreateRestUsagePlanKey(accountID, usagePlanID, apiKeyID string) (RestUsagePlanKey, error) {
	usagePlanID = strings.TrimSpace(usagePlanID)
	apiKeyID = strings.TrimSpace(apiKeyID)
	if usagePlanID == "" || apiKeyID == "" {
		return RestUsagePlanKey{}, fmt.Errorf("%w: usagePlanId and keyId required", ErrAPIGatewayBadRequest)
	}
	if _, err := s.GetRestUsagePlan(accountID, usagePlanID); err != nil {
		return RestUsagePlanKey{}, err
	}
	if _, err := s.GetRestAPIKey(accountID, apiKeyID); err != nil {
		return RestUsagePlanKey{}, err
	}
	_, err := s.db.Exec(
		`INSERT INTO apigw_usage_plan_keys (account_id, usage_plan_id, api_key_id) VALUES (?, ?, ?)`,
		accountID, usagePlanID, apiKeyID,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return RestUsagePlanKey{}, fmt.Errorf("%w: usage plan key already exists", ErrAPIGatewayConflict)
		}
		return RestUsagePlanKey{}, fmt.Errorf("create usage plan key: %w", err)
	}
	return RestUsagePlanKey{ID: apiKeyID, Type: "API_KEY", Value: apiKeyID}, nil
}

// ListRestUsagePlanKeys lists keys associated with a usage plan.
func (s *Store) ListRestUsagePlanKeys(accountID, usagePlanID string) ([]RestUsagePlanKey, error) {
	if _, err := s.GetRestUsagePlan(accountID, usagePlanID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT api_key_id FROM apigw_usage_plan_keys
		 WHERE account_id = ? AND usage_plan_id = ? ORDER BY api_key_id`,
		accountID, usagePlanID,
	)
	if err != nil {
		return nil, fmt.Errorf("list usage plan keys: %w", err)
	}
	defer rows.Close()
	var out []RestUsagePlanKey
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list usage plan keys scan: %w", err)
		}
		out = append(out, RestUsagePlanKey{ID: id, Type: "API_KEY", Value: id})
	}
	return out, rows.Err()
}

// DeleteRestUsagePlanKey removes an API key from a usage plan.
func (s *Store) DeleteRestUsagePlanKey(accountID, usagePlanID, apiKeyID string) error {
	res, err := s.db.Exec(
		`DELETE FROM apigw_usage_plan_keys WHERE account_id = ? AND usage_plan_id = ? AND api_key_id = ?`,
		accountID, usagePlanID, apiKeyID,
	)
	if err != nil {
		return fmt.Errorf("delete usage plan key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAPIGatewayNotFound
	}
	return nil
}

// RestStageHasUsagePlan reports whether any usage plan covers the API stage.
func (s *Store) RestStageHasUsagePlan(accountID, apiID, stage string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM apigw_usage_plan_stages
		 WHERE account_id = ? AND api_id = ? AND stage = ?`,
		accountID, apiID, stage,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("usage plan stage check: %w", err)
	}
	return n > 0, nil
}

// ValidateRestAPIKeyForStage checks x-api-key against an enabled key linked to a
// usage plan that covers the given API stage. Fail-closed when unmatched.
func (s *Store) ValidateRestAPIKeyForStage(accountID, apiID, stage, apiKeyValue string) bool {
	apiKeyValue = strings.TrimSpace(apiKeyValue)
	if apiKeyValue == "" {
		return false
	}
	keyHash := s.hashRestAPIKey(apiKeyValue)
	var keyID string
	var enabled int
	err := s.db.QueryRow(
		`SELECT api_key_id, enabled FROM apigw_api_keys WHERE account_id = ? AND key_hash = ?`,
		accountID, keyHash,
	).Scan(&keyID, &enabled)
	if err != nil || enabled == 0 {
		return false
	}
	var n int
	err = s.db.QueryRow(
		`SELECT COUNT(1) FROM apigw_usage_plan_keys k
		 INNER JOIN apigw_usage_plan_stages st
		   ON st.account_id = k.account_id AND st.usage_plan_id = k.usage_plan_id
		 WHERE k.account_id = ? AND k.api_key_id = ? AND st.api_id = ? AND st.stage = ?`,
		accountID, keyID, apiID, stage,
	).Scan(&n)
	return err == nil && n > 0
}

// ParseRestAPIStagesJSON parses CreateUsagePlan apiStages.
func ParseRestAPIStagesJSON(raw any) ([]RestUsagePlanStage, error) {
	if raw == nil {
		return nil, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: apiStages", ErrAPIGatewayBadRequest)
	}
	var items []map[string]any
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("%w: apiStages must be an array", ErrAPIGatewayBadRequest)
	}
	out := make([]RestUsagePlanStage, 0, len(items))
	for _, m := range items {
		apiID, _ := m["apiId"].(string)
		if apiID == "" {
			apiID, _ = m["ApiId"].(string)
		}
		stage, _ := m["stage"].(string)
		if stage == "" {
			stage, _ = m["Stage"].(string)
		}
		out = append(out, RestUsagePlanStage{APIID: apiID, Stage: stage})
	}
	return out, nil
}

// RestAPIControlPlaneARN is the management ARN used for PassRole SourceArn.
func RestAPIControlPlaneARN(region, apiID string) string {
	if region == "" {
		region = DefaultAPIGatewayRegion
	}
	return "arn:aws:apigateway:" + region + "::/restapis/" + apiID
}
