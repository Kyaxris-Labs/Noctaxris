package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwksfetch"
	"github.com/google/uuid"
)

var (
	ErrAppSyncNotFound   = errors.New("NotFoundException")
	ErrAppSyncBadRequest = errors.New("BadRequestException")
	ErrAppSyncAuth       = errors.New("UnauthorizedException")
)

const DefaultAppSyncRegion = "us-east-1"

const (
	AppSyncAuthAPIKey  = "API_KEY"
	AppSyncAuthIAM     = "AWS_IAM"
	AppSyncAuthCognito = "AMAZON_COGNITO_USER_POOLS"
)

const appSyncSchema = `
CREATE TABLE IF NOT EXISTS appsync_apis (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  authentication_type TEXT NOT NULL,
  schema_sdl TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  user_pool_id TEXT NOT NULL DEFAULT '',
  user_pool_region TEXT NOT NULL DEFAULT '',
  user_pool_client_id TEXT NOT NULL DEFAULT '',
  user_pool_issuer TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, api_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_appsync_name ON appsync_apis(account_id, name);
CREATE TABLE IF NOT EXISTS appsync_api_keys (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  id TEXT NOT NULL,
  api_key TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, api_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_appsync_key ON appsync_api_keys(api_key);
CREATE TABLE IF NOT EXISTS appsync_data_sources (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  lambda_function_arn TEXT NOT NULL DEFAULT '',
  service_role_arn TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, api_id, name)
);
CREATE TABLE IF NOT EXISTS appsync_resolvers (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  type_name TEXT NOT NULL,
  field_name TEXT NOT NULL,
  data_source_name TEXT NOT NULL,
  PRIMARY KEY (account_id, api_id, type_name, field_name)
);
`

// AppSyncAPI is a GraphQL API row.
type AppSyncAPI struct {
	APIID              string
	Name               string
	ARN                string
	AuthenticationType string
	SchemaSDL          string
	CreatedAt          int64
	UserPoolID         string
	UserPoolRegion     string
	UserPoolClientID   string
	UserPoolIssuer     string
}

// AppSyncUserPoolConfig is Cognito User Pools auth config for AMAZON_COGNITO_USER_POOLS.
type AppSyncUserPoolConfig struct {
	UserPoolID string
	AwsRegion  string
	ClientID   string // lab audience / app client id for JWT verify
	Issuer     string // optional override; default lab issuer on :4566
}

// AppSyncAPIKey is an API key for API_KEY auth.
type AppSyncAPIKey struct {
	ID        string
	APIID     string
	APIKey    string
	ExpiresAt int64
}

// AppSyncDataSource is a Lambda (or none) data source.
type AppSyncDataSource struct {
	Name              string
	APIID             string
	Type              string // AWS_LAMBDA
	LambdaFunctionARN string
	ServiceRoleArn    string // optional; when set, GraphQL invoke uses role session
}

// AppSyncResolver maps a type/field to a data source.
type AppSyncResolver struct {
	APIID          string
	TypeName       string
	FieldName      string
	DataSourceName string
}

// EnsureAppSyncSchema creates AppSync tables if missing.
func EnsureAppSyncSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure appsync schema: db is nil")
	}
	if _, err := db.Exec(appSyncSchema); err != nil {
		return fmt.Errorf("ensure appsync schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE appsync_apis ADD COLUMN user_pool_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE appsync_apis ADD COLUMN user_pool_region TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE appsync_apis ADD COLUMN user_pool_client_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE appsync_apis ADD COLUMN user_pool_issuer TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE appsync_data_sources ADD COLUMN service_role_arn TEXT NOT NULL DEFAULT ''`,
	}); err != nil {
		return fmt.Errorf("ensure appsync schema alter: %w", err)
	}
	return nil
}

// EnsureAppSyncSchema ensures AppSync tables on an open store.
func (s *Store) EnsureAppSyncSchema() error {
	return EnsureAppSyncSchema(s.db)
}

// CreateAppSyncGraphqlAPI creates a GraphQL API. AuthType: API_KEY, AWS_IAM, or AMAZON_COGNITO_USER_POOLS.
func (s *Store) CreateAppSyncGraphqlAPI(accountID, region, name, authType string) (AppSyncAPI, error) {
	return s.CreateAppSyncGraphqlAPIWithConfig(accountID, region, name, authType, AppSyncUserPoolConfig{})
}

// CreateAppSyncGraphqlAPIWithConfig creates a GraphQL API with optional Cognito User Pool config.
func (s *Store) CreateAppSyncGraphqlAPIWithConfig(accountID, region, name, authType string, pool AppSyncUserPoolConfig) (AppSyncAPI, error) {
	name = strings.TrimSpace(name)
	authType = strings.ToUpper(strings.TrimSpace(authType))
	if name == "" {
		return AppSyncAPI{}, fmt.Errorf("%w: name required", ErrAppSyncBadRequest)
	}
	if authType == "" {
		authType = AppSyncAuthAPIKey
	}
	switch authType {
	case AppSyncAuthAPIKey, AppSyncAuthIAM:
		pool = AppSyncUserPoolConfig{}
	case AppSyncAuthCognito:
		pool.UserPoolID = strings.TrimSpace(pool.UserPoolID)
		pool.AwsRegion = strings.TrimSpace(pool.AwsRegion)
		pool.ClientID = strings.TrimSpace(pool.ClientID)
		pool.Issuer = strings.TrimSpace(pool.Issuer)
		if pool.UserPoolID == "" {
			return AppSyncAPI{}, fmt.Errorf("%w: userPoolConfig.userPoolId required", ErrAppSyncBadRequest)
		}
		if pool.ClientID == "" {
			return AppSyncAPI{}, fmt.Errorf("%w: userPoolConfig clientId (audience) required", ErrAppSyncBadRequest)
		}
		if pool.AwsRegion == "" {
			pool.AwsRegion = DefaultAppSyncRegion
		}
		if pool.Issuer == "" {
			pool.Issuer = AppSyncCognitoIssuer(pool.AwsRegion, pool.UserPoolID)
		}
		if err := jwksfetch.ValidateIssuerForConfig(pool.Issuer); err != nil {
			return AppSyncAPI{}, fmt.Errorf("%w: userPoolConfig.issuer: %v", ErrAppSyncBadRequest, err)
		}
	default:
		return AppSyncAPI{}, fmt.Errorf("%w: authenticationType must be API_KEY, AWS_IAM, or AMAZON_COGNITO_USER_POOLS", ErrAppSyncBadRequest)
	}
	if region == "" {
		region = DefaultAppSyncRegion
	}
	apiID := uuid.NewString()
	arn := fmt.Sprintf("arn:aws:appsync:%s:%s:apis/%s", region, accountID, apiID)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO appsync_apis (account_id, api_id, name, arn, authentication_type, schema_sdl, created_at,
		 user_pool_id, user_pool_region, user_pool_client_id, user_pool_issuer)
		 VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?)`,
		accountID, apiID, name, arn, authType, now,
		pool.UserPoolID, pool.AwsRegion, pool.ClientID, pool.Issuer,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return AppSyncAPI{}, fmt.Errorf("%w: API name already exists", ErrAppSyncBadRequest)
		}
		return AppSyncAPI{}, fmt.Errorf("create graphql api: %w", err)
	}
	return AppSyncAPI{
		APIID: apiID, Name: name, ARN: arn, AuthenticationType: authType, CreatedAt: now,
		UserPoolID: pool.UserPoolID, UserPoolRegion: pool.AwsRegion,
		UserPoolClientID: pool.ClientID, UserPoolIssuer: pool.Issuer,
	}, nil
}

// AppSyncCognitoIssuer builds the lab Cognito issuer URL (ADR-0009 / same as CognitoIssuerURL).
func AppSyncCognitoIssuer(region, userPoolID string) string {
	return CognitoIssuerURL(region, userPoolID)
}

// DeleteAppSyncGraphqlAPI deletes an API and related rows.
func (s *Store) DeleteAppSyncGraphqlAPI(accountID, apiID string) error {
	apiID = strings.TrimSpace(apiID)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete graphql api begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM appsync_apis WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	if err != nil {
		return fmt.Errorf("delete graphql api: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAppSyncNotFound
	}
	_, _ = tx.Exec(`DELETE FROM appsync_api_keys WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	_, _ = tx.Exec(`DELETE FROM appsync_data_sources WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	_, _ = tx.Exec(`DELETE FROM appsync_resolvers WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	return tx.Commit()
}

// GetAppSyncGraphqlAPI returns an API by ID.
func (s *Store) GetAppSyncGraphqlAPI(accountID, apiID string) (AppSyncAPI, error) {
	var a AppSyncAPI
	err := s.db.QueryRow(
		`SELECT api_id, name, arn, authentication_type, schema_sdl, created_at,
		 COALESCE(user_pool_id,''), COALESCE(user_pool_region,''), COALESCE(user_pool_client_id,''), COALESCE(user_pool_issuer,'')
		 FROM appsync_apis WHERE account_id = ? AND api_id = ?`,
		accountID, apiID,
	).Scan(&a.APIID, &a.Name, &a.ARN, &a.AuthenticationType, &a.SchemaSDL, &a.CreatedAt,
		&a.UserPoolID, &a.UserPoolRegion, &a.UserPoolClientID, &a.UserPoolIssuer)
	if errors.Is(err, sql.ErrNoRows) {
		return AppSyncAPI{}, ErrAppSyncNotFound
	}
	if err != nil {
		return AppSyncAPI{}, fmt.Errorf("get graphql api: %w", err)
	}
	return a, nil
}

// GetAppSyncGraphqlAPIByID returns an API and owning account by api_id (lab: api_id is globally unique).
func (s *Store) GetAppSyncGraphqlAPIByID(apiID string) (accountID string, api AppSyncAPI, err error) {
	err = s.db.QueryRow(
		`SELECT account_id, api_id, name, arn, authentication_type, schema_sdl, created_at,
		 COALESCE(user_pool_id,''), COALESCE(user_pool_region,''), COALESCE(user_pool_client_id,''), COALESCE(user_pool_issuer,'')
		 FROM appsync_apis WHERE api_id = ?`,
		apiID,
	).Scan(&accountID, &api.APIID, &api.Name, &api.ARN, &api.AuthenticationType, &api.SchemaSDL, &api.CreatedAt,
		&api.UserPoolID, &api.UserPoolRegion, &api.UserPoolClientID, &api.UserPoolIssuer)
	if errors.Is(err, sql.ErrNoRows) {
		return "", AppSyncAPI{}, ErrAppSyncNotFound
	}
	if err != nil {
		return "", AppSyncAPI{}, fmt.Errorf("get graphql api by id: %w", err)
	}
	return accountID, api, nil
}

// ListAppSyncGraphqlAPIs lists APIs for an account.
func (s *Store) ListAppSyncGraphqlAPIs(accountID string) ([]AppSyncAPI, error) {
	rows, err := s.db.Query(
		`SELECT api_id, name, arn, authentication_type, schema_sdl, created_at,
		 COALESCE(user_pool_id,''), COALESCE(user_pool_region,''), COALESCE(user_pool_client_id,''), COALESCE(user_pool_issuer,'')
		 FROM appsync_apis WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list graphql apis: %w", err)
	}
	defer rows.Close()
	var out []AppSyncAPI
	for rows.Next() {
		var a AppSyncAPI
		if err := rows.Scan(&a.APIID, &a.Name, &a.ARN, &a.AuthenticationType, &a.SchemaSDL, &a.CreatedAt,
			&a.UserPoolID, &a.UserPoolRegion, &a.UserPoolClientID, &a.UserPoolIssuer); err != nil {
			return nil, fmt.Errorf("list graphql apis scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// StartAppSyncSchemaCreation stores the schema SDL (lab: immediate success, no async job).
func (s *Store) StartAppSyncSchemaCreation(accountID, apiID, definition string) error {
	definition = strings.TrimSpace(definition)
	if definition == "" {
		return fmt.Errorf("%w: definition required", ErrAppSyncBadRequest)
	}
	res, err := s.db.Exec(
		`UPDATE appsync_apis SET schema_sdl = ? WHERE account_id = ? AND api_id = ?`,
		definition, accountID, apiID,
	)
	if err != nil {
		return fmt.Errorf("start schema creation: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAppSyncNotFound
	}
	return nil
}

// CreateAppSyncAPIKey creates an API key for an API_KEY authenticated API.
func (s *Store) CreateAppSyncAPIKey(accountID, apiID string, expiresAt int64) (AppSyncAPIKey, error) {
	api, err := s.GetAppSyncGraphqlAPI(accountID, apiID)
	if err != nil {
		return AppSyncAPIKey{}, err
	}
	if api.AuthenticationType != AppSyncAuthAPIKey {
		return AppSyncAPIKey{}, fmt.Errorf("%w: API keys require authenticationType API_KEY", ErrAppSyncBadRequest)
	}
	if expiresAt <= 0 {
		expiresAt = time.Now().UTC().Add(365 * 24 * time.Hour).Unix()
	}
	id := uuid.NewString()
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return AppSyncAPIKey{}, fmt.Errorf("generate api key: %w", err)
	}
	apiKey := "da2-" + hex.EncodeToString(keyBytes)
	keyHash := s.hashAppSyncAPIKey(apiKey)
	_, err = s.db.Exec(
		`INSERT INTO appsync_api_keys (account_id, api_id, id, api_key, expires_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, apiID, id, keyHash, expiresAt,
	)
	if err != nil {
		return AppSyncAPIKey{}, fmt.Errorf("create api key: %w", err)
	}
	// Return plaintext once; only the HMAC hash is stored at rest.
	return AppSyncAPIKey{ID: id, APIID: apiID, APIKey: apiKey, ExpiresAt: expiresAt}, nil
}

// LookupAppSyncAPIKey finds an API by raw API key value (HMAC lookup).
func (s *Store) LookupAppSyncAPIKey(apiKey string) (accountID, apiID string, err error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", "", ErrAppSyncAuth
	}
	keyHash := s.hashAppSyncAPIKey(apiKey)
	err = s.db.QueryRow(
		`SELECT account_id, api_id FROM appsync_api_keys WHERE api_key = ? AND expires_at > ?`,
		keyHash, time.Now().UTC().Unix(),
	).Scan(&accountID, &apiID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrAppSyncAuth
	}
	if err != nil {
		return "", "", fmt.Errorf("lookup api key: %w", err)
	}
	return accountID, apiID, nil
}

func (s *Store) hashAppSyncAPIKey(apiKey string) string {
	mac := hmac.New(sha256.New, s.master[:])
	_, _ = mac.Write([]byte(apiKey))
	return hex.EncodeToString(mac.Sum(nil))
}

// CreateAppSyncDataSource creates a Lambda data source.
// serviceRoleARN is optional; when empty, GraphQL invoke uses Lambda resource policy only.
func (s *Store) CreateAppSyncDataSource(accountID, apiID, name, dsType, lambdaARN, serviceRoleARN string) (AppSyncDataSource, error) {
	name = strings.TrimSpace(name)
	dsType = strings.ToUpper(strings.TrimSpace(dsType))
	lambdaARN = strings.TrimSpace(lambdaARN)
	serviceRoleARN = strings.TrimSpace(serviceRoleARN)
	if name == "" {
		return AppSyncDataSource{}, fmt.Errorf("%w: name required", ErrAppSyncBadRequest)
	}
	if dsType == "" {
		dsType = "AWS_LAMBDA"
	}
	if dsType != "AWS_LAMBDA" {
		return AppSyncDataSource{}, fmt.Errorf("%w: only AWS_LAMBDA data sources in lab core", ErrAppSyncBadRequest)
	}
	if lambdaARN == "" {
		return AppSyncDataSource{}, fmt.Errorf("%w: lambdaFunctionArn required", ErrAppSyncBadRequest)
	}
	if _, err := s.GetAppSyncGraphqlAPI(accountID, apiID); err != nil {
		return AppSyncDataSource{}, err
	}
	_, err := s.db.Exec(
		`INSERT INTO appsync_data_sources (account_id, api_id, name, type, lambda_function_arn, service_role_arn)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, apiID, name, dsType, lambdaARN, serviceRoleARN,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return AppSyncDataSource{}, fmt.Errorf("%w: data source exists", ErrAppSyncBadRequest)
		}
		return AppSyncDataSource{}, fmt.Errorf("create data source: %w", err)
	}
	return AppSyncDataSource{
		Name: name, APIID: apiID, Type: dsType,
		LambdaFunctionARN: lambdaARN, ServiceRoleArn: serviceRoleARN,
	}, nil
}

// GetAppSyncDataSource returns a data source.
func (s *Store) GetAppSyncDataSource(accountID, apiID, name string) (AppSyncDataSource, error) {
	var ds AppSyncDataSource
	err := s.db.QueryRow(
		`SELECT name, api_id, type, lambda_function_arn, COALESCE(service_role_arn,'')
		 FROM appsync_data_sources
		 WHERE account_id = ? AND api_id = ? AND name = ?`,
		accountID, apiID, name,
	).Scan(&ds.Name, &ds.APIID, &ds.Type, &ds.LambdaFunctionARN, &ds.ServiceRoleArn)
	if errors.Is(err, sql.ErrNoRows) {
		return AppSyncDataSource{}, ErrAppSyncNotFound
	}
	if err != nil {
		return AppSyncDataSource{}, fmt.Errorf("get data source: %w", err)
	}
	return ds, nil
}

// CreateAppSyncResolver creates a resolver for one type/field.
func (s *Store) CreateAppSyncResolver(accountID, apiID, typeName, fieldName, dataSourceName string) (AppSyncResolver, error) {
	typeName = strings.TrimSpace(typeName)
	fieldName = strings.TrimSpace(fieldName)
	dataSourceName = strings.TrimSpace(dataSourceName)
	if typeName == "" || fieldName == "" || dataSourceName == "" {
		return AppSyncResolver{}, fmt.Errorf("%w: typeName, fieldName, dataSourceName required", ErrAppSyncBadRequest)
	}
	if _, err := s.GetAppSyncDataSource(accountID, apiID, dataSourceName); err != nil {
		return AppSyncResolver{}, err
	}
	_, err := s.db.Exec(
		`INSERT INTO appsync_resolvers (account_id, api_id, type_name, field_name, data_source_name)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, apiID, typeName, fieldName, dataSourceName,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return AppSyncResolver{}, fmt.Errorf("%w: resolver exists", ErrAppSyncBadRequest)
		}
		return AppSyncResolver{}, fmt.Errorf("create resolver: %w", err)
	}
	return AppSyncResolver{APIID: apiID, TypeName: typeName, FieldName: fieldName, DataSourceName: dataSourceName}, nil
}

// GetAppSyncResolver returns a resolver.
func (s *Store) GetAppSyncResolver(accountID, apiID, typeName, fieldName string) (AppSyncResolver, error) {
	var r AppSyncResolver
	err := s.db.QueryRow(
		`SELECT api_id, type_name, field_name, data_source_name FROM appsync_resolvers
		 WHERE account_id = ? AND api_id = ? AND type_name = ? AND field_name = ?`,
		accountID, apiID, typeName, fieldName,
	).Scan(&r.APIID, &r.TypeName, &r.FieldName, &r.DataSourceName)
	if errors.Is(err, sql.ErrNoRows) {
		return AppSyncResolver{}, ErrAppSyncNotFound
	}
	if err != nil {
		return AppSyncResolver{}, fmt.Errorf("get resolver: %w", err)
	}
	return r, nil
}

var (
	gqlOpPrefixRe  = regexp.MustCompile(`(?is)^\s*(?:query|mutation|subscription)\s*(?:[A-Za-z_][A-Za-z0-9_]*)?\s*`)
	gqlIdentRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	gqlTypeDeclRe  = regexp.MustCompile(`(?is)\btype\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{([^}]*)\}`)
	gqlFieldDeclRe = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*(?:\([^)]*\))?\s*:\s*(\[(?:[A-Za-z_][A-Za-z0-9_]*!?)\]!?|[A-Za-z_][A-Za-z0-9_]*!?)`)
)

// MaxAppSyncSelectionDepth is the max nesting of selection sets (Query field = depth 1).
const MaxAppSyncSelectionDepth = 3

// AppSyncSelection is one GraphQL selection field with optional nested children.
type AppSyncSelection struct {
	Name     string
	Children []AppSyncSelection
}

// ParseAppSyncQuerySelections parses a lab GraphQL query into a selection tree.
// Supports `{ hello world }`, `query { parent { child } }`, and nesting up to MaxAppSyncSelectionDepth.
// Rejects field arguments, aliases, fragments, and deeper nesting.
func ParseAppSyncQuerySelections(query string) ([]AppSyncSelection, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: query required", ErrAppSyncBadRequest)
	}
	query = gqlOpPrefixRe.ReplaceAllString(query, "")
	query = strings.TrimSpace(query)
	if !strings.HasPrefix(query, "{") {
		return nil, fmt.Errorf("%w: unable to parse query field", ErrAppSyncBadRequest)
	}
	inner, rest, err := takeGQLSelectionSet(query)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rest) != "" {
		return nil, fmt.Errorf("%w: unable to parse query field", ErrAppSyncBadRequest)
	}
	return parseAppSyncSelections(inner, 1)
}

func takeGQLSelectionSet(s string) (inner, rest string, err error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return "", "", fmt.Errorf("%w: unable to parse query field", ErrAppSyncBadRequest)
	}
	depth := 0
	end := -1
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return "", "", fmt.Errorf("%w: unable to parse query field", ErrAppSyncBadRequest)
	}
	return s[1:end], s[end+1:], nil
}

func parseAppSyncSelections(inner string, depth int) ([]AppSyncSelection, error) {
	if depth > MaxAppSyncSelectionDepth {
		return nil, fmt.Errorf("%w: selection set depth exceeds %d", ErrAppSyncBadRequest, MaxAppSyncSelectionDepth)
	}
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return nil, fmt.Errorf("%w: unable to parse query field", ErrAppSyncBadRequest)
	}
	var out []AppSyncSelection
	i := 0
	for i < len(inner) {
		for i < len(inner) && (inner[i] == ' ' || inner[i] == '\t' || inner[i] == '\n' || inner[i] == '\r' || inner[i] == ',') {
			i++
		}
		if i >= len(inner) {
			break
		}
		start := i
		for i < len(inner) && gqlIdentByte(inner[i]) {
			i++
		}
		if start == i {
			return nil, fmt.Errorf("%w: unable to parse query field", ErrAppSyncBadRequest)
		}
		name := inner[start:i]
		if !gqlIdentRe.MatchString(name) {
			return nil, fmt.Errorf("%w: unable to parse query field", ErrAppSyncBadRequest)
		}
		for i < len(inner) && (inner[i] == ' ' || inner[i] == '\t' || inner[i] == '\n' || inner[i] == '\r') {
			i++
		}
		if i < len(inner) && inner[i] == '(' {
			return nil, fmt.Errorf("%w: field arguments are not supported", ErrAppSyncBadRequest)
		}
		sel := AppSyncSelection{Name: name}
		if i < len(inner) && inner[i] == '{' {
			if depth >= MaxAppSyncSelectionDepth {
				return nil, fmt.Errorf("%w: selection set depth exceeds %d", ErrAppSyncBadRequest, MaxAppSyncSelectionDepth)
			}
			childInner, rest, err := takeGQLSelectionSet(inner[i:])
			if err != nil {
				return nil, err
			}
			children, err := parseAppSyncSelections(childInner, depth+1)
			if err != nil {
				return nil, err
			}
			sel.Children = children
			// rest is relative to inner[i:]; map back to absolute index in inner
			consumed := len(inner[i:]) - len(rest)
			i += consumed
		}
		out = append(out, sel)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: unable to parse query field", ErrAppSyncBadRequest)
	}
	return out, nil
}

func gqlIdentByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

// ParseAppSyncQueryFields extracts top-level Query selection field names (ignores nested children).
func ParseAppSyncQueryFields(query string) ([]string, error) {
	sels, err := ParseAppSyncQuerySelections(query)
	if err != nil {
		return nil, err
	}
	fields := make([]string, len(sels))
	for i, s := range sels {
		fields[i] = s.Name
	}
	return fields, nil
}

// ParseAppSyncQueryField extracts the first selection field name from a minimal GraphQL query.
func ParseAppSyncQueryField(query string) (fieldName string, err error) {
	fields, err := ParseAppSyncQueryFields(query)
	if err != nil {
		return "", err
	}
	return fields[0], nil
}

// ParseAppSyncSchemaFieldReturnTypes maps typeName -> fieldName -> bare return type (list/non-null stripped).
func ParseAppSyncSchemaFieldReturnTypes(sdl string) map[string]map[string]string {
	out := make(map[string]map[string]string)
	for _, m := range gqlTypeDeclRe.FindAllStringSubmatch(sdl, -1) {
		typeName := m[1]
		body := m[2]
		fields := make(map[string]string)
		for _, fm := range gqlFieldDeclRe.FindAllStringSubmatch(body, -1) {
			fields[fm[1]] = stripGQLTypeWrappers(fm[2])
		}
		if len(fields) > 0 {
			out[typeName] = fields
		}
	}
	return out
}

func stripGQLTypeWrappers(t string) string {
	t = strings.TrimSpace(t)
	for {
		changed := false
		if strings.HasSuffix(t, "!") {
			t = strings.TrimSuffix(t, "!")
			t = strings.TrimSpace(t)
			changed = true
		}
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			t = strings.TrimPrefix(t, "[")
			t = strings.TrimSuffix(t, "]")
			t = strings.TrimSpace(t)
			changed = true
		}
		if !changed {
			break
		}
	}
	return t
}

// ResolveAppSyncField looks up resolver + data source for typeName.fieldName.
func (s *Store) ResolveAppSyncField(accountID, apiID, typeName, fieldName string) (resolver AppSyncResolver, ds AppSyncDataSource, err error) {
	resolver, err = s.GetAppSyncResolver(accountID, apiID, typeName, fieldName)
	if err != nil {
		return AppSyncResolver{}, AppSyncDataSource{}, err
	}
	ds, err = s.GetAppSyncDataSource(accountID, apiID, resolver.DataSourceName)
	if err != nil {
		return AppSyncResolver{}, AppSyncDataSource{}, err
	}
	return resolver, ds, nil
}

// ResolveAppSyncQueryField looks up resolver + data source for a Query field.
func (s *Store) ResolveAppSyncQueryField(accountID, apiID, fieldName string) (resolver AppSyncResolver, ds AppSyncDataSource, err error) {
	return s.ResolveAppSyncField(accountID, apiID, "Query", fieldName)
}
