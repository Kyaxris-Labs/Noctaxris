package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrAppSyncNotFound   = errors.New("NotFoundException")
	ErrAppSyncBadRequest = errors.New("BadRequestException")
	ErrAppSyncAuth       = errors.New("UnauthorizedException")
)

const DefaultAppSyncRegion = "us-east-1"

const (
	AppSyncAuthAPIKey   = "API_KEY"
	AppSyncAuthIAM      = "AWS_IAM"
	AppSyncAuthCognito  = "AMAZON_COGNITO_USER_POOLS"
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
	alters := []string{
		`ALTER TABLE appsync_apis ADD COLUMN user_pool_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE appsync_apis ADD COLUMN user_pool_region TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE appsync_apis ADD COLUMN user_pool_client_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE appsync_apis ADD COLUMN user_pool_issuer TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range alters {
		if _, err := db.Exec(stmt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				continue
			}
			return fmt.Errorf("ensure appsync schema alter: %w", err)
		}
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
	_, err = s.db.Exec(
		`INSERT INTO appsync_api_keys (account_id, api_id, id, api_key, expires_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, apiID, id, apiKey, expiresAt,
	)
	if err != nil {
		return AppSyncAPIKey{}, fmt.Errorf("create api key: %w", err)
	}
	return AppSyncAPIKey{ID: id, APIID: apiID, APIKey: apiKey, ExpiresAt: expiresAt}, nil
}

// LookupAppSyncAPIKey finds an API by raw API key value.
func (s *Store) LookupAppSyncAPIKey(apiKey string) (accountID, apiID string, err error) {
	apiKey = strings.TrimSpace(apiKey)
	err = s.db.QueryRow(
		`SELECT account_id, api_id FROM appsync_api_keys WHERE api_key = ? AND expires_at > ?`,
		apiKey, time.Now().UTC().Unix(),
	).Scan(&accountID, &apiID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrAppSyncAuth
	}
	if err != nil {
		return "", "", fmt.Errorf("lookup api key: %w", err)
	}
	return accountID, apiID, nil
}

// CreateAppSyncDataSource creates a Lambda data source.
func (s *Store) CreateAppSyncDataSource(accountID, apiID, name, dsType, lambdaARN string) (AppSyncDataSource, error) {
	name = strings.TrimSpace(name)
	dsType = strings.ToUpper(strings.TrimSpace(dsType))
	lambdaARN = strings.TrimSpace(lambdaARN)
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
		`INSERT INTO appsync_data_sources (account_id, api_id, name, type, lambda_function_arn)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, apiID, name, dsType, lambdaARN,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return AppSyncDataSource{}, fmt.Errorf("%w: data source exists", ErrAppSyncBadRequest)
		}
		return AppSyncDataSource{}, fmt.Errorf("create data source: %w", err)
	}
	return AppSyncDataSource{Name: name, APIID: apiID, Type: dsType, LambdaFunctionARN: lambdaARN}, nil
}

// GetAppSyncDataSource returns a data source.
func (s *Store) GetAppSyncDataSource(accountID, apiID, name string) (AppSyncDataSource, error) {
	var ds AppSyncDataSource
	err := s.db.QueryRow(
		`SELECT name, api_id, type, lambda_function_arn FROM appsync_data_sources
		 WHERE account_id = ? AND api_id = ? AND name = ?`,
		accountID, apiID, name,
	).Scan(&ds.Name, &ds.APIID, &ds.Type, &ds.LambdaFunctionARN)
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
	gqlOpPrefixRe = regexp.MustCompile(`(?is)^\s*(?:query|mutation|subscription)\s*(?:[A-Za-z_][A-Za-z0-9_]*)?\s*`)
	gqlFieldRe    = regexp.MustCompile(`(?is)\{\s*([A-Za-z_][A-Za-z0-9_]*)`)
)

// ParseAppSyncQueryField extracts the first selection field name from a minimal GraphQL query.
// Lab: supports `{ field }` and `query { field }` only. No fragments, aliases, or nested selections.
func ParseAppSyncQueryField(query string) (fieldName string, err error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("%w: query required", ErrAppSyncBadRequest)
	}
	query = gqlOpPrefixRe.ReplaceAllString(query, "")
	m := gqlFieldRe.FindStringSubmatch(query)
	if len(m) < 2 {
		return "", fmt.Errorf("%w: unable to parse query field", ErrAppSyncBadRequest)
	}
	return m[1], nil
}

// ResolveAppSyncQueryField looks up resolver + Lambda ARN for a Query field.
func (s *Store) ResolveAppSyncQueryField(accountID, apiID, fieldName string) (resolver AppSyncResolver, lambdaARN string, err error) {
	resolver, err = s.GetAppSyncResolver(accountID, apiID, "Query", fieldName)
	if err != nil {
		return AppSyncResolver{}, "", err
	}
	ds, err := s.GetAppSyncDataSource(accountID, apiID, resolver.DataSourceName)
	if err != nil {
		return AppSyncResolver{}, "", err
	}
	return resolver, ds.LambdaFunctionARN, nil
}
