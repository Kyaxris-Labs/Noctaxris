package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwksfetch"
	"github.com/google/uuid"
)

var (
	ErrAPIGatewayNotFound   = errors.New("NotFoundException")
	ErrAPIGatewayBadRequest = errors.New("BadRequestException")
	ErrAPIGatewayConflict   = errors.New("ConflictException")
)

const DefaultAPIGatewayRegion = "us-east-1"

const (
	APIGatewayProtocolHTTP = "HTTP"

	APIGatewayAuthNone = "NONE"
	APIGatewayAuthJWT  = "JWT"
	APIGatewayAuthIAM  = "AWS_IAM"

	APIGatewayAuthorizerJWT = "JWT"

	APIGatewayIntegrationAWSProxy = "AWS_PROXY"
)

const apiGatewayV2Schema = `
CREATE TABLE IF NOT EXISTS apigwv2_apis (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  name TEXT NOT NULL,
  protocol_type TEXT NOT NULL,
  api_endpoint TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, api_id)
);
CREATE TABLE IF NOT EXISTS apigwv2_integrations (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  integration_id TEXT NOT NULL,
  integration_type TEXT NOT NULL,
  integration_uri TEXT NOT NULL,
  payload_format_version TEXT NOT NULL DEFAULT '2.0',
  credentials_arn TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, api_id, integration_id)
);
CREATE TABLE IF NOT EXISTS apigwv2_authorizers (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  authorizer_id TEXT NOT NULL,
  name TEXT NOT NULL,
  authorizer_type TEXT NOT NULL,
  identity_source TEXT NOT NULL,
  jwt_issuer TEXT NOT NULL DEFAULT '',
  jwt_audience_json TEXT NOT NULL DEFAULT '[]',
  PRIMARY KEY (account_id, api_id, authorizer_id)
);
CREATE TABLE IF NOT EXISTS apigwv2_routes (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  route_id TEXT NOT NULL,
  route_key TEXT NOT NULL,
  target TEXT NOT NULL,
  authorization_type TEXT NOT NULL,
  authorizer_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, api_id, route_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_apigwv2_route_key ON apigwv2_routes(account_id, api_id, route_key);
CREATE TABLE IF NOT EXISTS apigwv2_stages (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  stage_name TEXT NOT NULL,
  auto_deploy INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (account_id, api_id, stage_name)
);
`

// APIGatewayAPI is an HTTP API row.
type APIGatewayAPI struct {
	APIID        string
	Name         string
	ProtocolType string
	APIEndpoint  string
	CreatedAt    int64
}

// APIGatewayIntegration is a Lambda AWS_PROXY integration.
type APIGatewayIntegration struct {
	IntegrationID        string
	APIID                string
	IntegrationType      string
	IntegrationURI       string
	PayloadFormatVersion string
	CredentialsArn       string
}

// APIGatewayAuthorizer is a JWT authorizer.
type APIGatewayAuthorizer struct {
	AuthorizerID   string
	APIID          string
	Name           string
	AuthorizerType string
	IdentitySource string
	JWTIssuer      string
	JWTAudience    []string
}

// APIGatewayRoute is a route row.
type APIGatewayRoute struct {
	RouteID           string
	APIID             string
	RouteKey          string
	Target            string
	AuthorizationType string
	AuthorizerID      string
}

// APIGatewayStage is a stage row.
type APIGatewayStage struct {
	APIID      string
	StageName  string
	AutoDeploy bool
}

// EnsureAPIGatewayV2Schema creates API Gateway HTTP API tables if missing.
func EnsureAPIGatewayV2Schema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure apigatewayv2 schema: db is nil")
	}
	if _, err := db.Exec(apiGatewayV2Schema); err != nil {
		return fmt.Errorf("ensure apigatewayv2 schema: %w", err)
	}
	alters := []string{
		`ALTER TABLE apigwv2_integrations ADD COLUMN credentials_arn TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range alters {
		if _, err := db.Exec(stmt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				continue
			}
			return fmt.Errorf("ensure apigatewayv2 schema alter: %w", err)
		}
	}
	return nil
}

// EnsureAPIGatewayV2Schema ensures tables on an open store.
func (s *Store) EnsureAPIGatewayV2Schema() error {
	return EnsureAPIGatewayV2Schema(s.db)
}

// APIGatewayAPIEndpoint builds the lab invoke base URL for an API.
func APIGatewayAPIEndpoint(apiID string) string {
	return "http://127.0.0.1:4566/http-api/" + apiID
}

// CreateAPIGatewayAPI creates an HTTP API.
func (s *Store) CreateAPIGatewayAPI(accountID, region, name, protocolType string) (APIGatewayAPI, error) {
	name = strings.TrimSpace(name)
	protocolType = strings.ToUpper(strings.TrimSpace(protocolType))
	if name == "" {
		return APIGatewayAPI{}, fmt.Errorf("%w: Name required", ErrAPIGatewayBadRequest)
	}
	if protocolType == "" {
		protocolType = APIGatewayProtocolHTTP
	}
	if protocolType != APIGatewayProtocolHTTP {
		return APIGatewayAPI{}, fmt.Errorf("%w: ProtocolType must be HTTP", ErrAPIGatewayBadRequest)
	}
	if region == "" {
		region = DefaultAPIGatewayRegion
	}
	_ = region
	apiID := uuid.NewString()
	endpoint := APIGatewayAPIEndpoint(apiID)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO apigwv2_apis (account_id, api_id, name, protocol_type, api_endpoint, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, apiID, name, protocolType, endpoint, now,
	)
	if err != nil {
		return APIGatewayAPI{}, fmt.Errorf("create api: %w", err)
	}
	return APIGatewayAPI{
		APIID: apiID, Name: name, ProtocolType: protocolType,
		APIEndpoint: endpoint, CreatedAt: now,
	}, nil
}

// GetAPIGatewayAPI returns an API by id.
func (s *Store) GetAPIGatewayAPI(accountID, apiID string) (APIGatewayAPI, error) {
	var a APIGatewayAPI
	err := s.db.QueryRow(
		`SELECT api_id, name, protocol_type, api_endpoint, created_at FROM apigwv2_apis
		 WHERE account_id = ? AND api_id = ?`,
		accountID, apiID,
	).Scan(&a.APIID, &a.Name, &a.ProtocolType, &a.APIEndpoint, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return APIGatewayAPI{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return APIGatewayAPI{}, fmt.Errorf("get api: %w", err)
	}
	return a, nil
}

// GetAPIGatewayAPIByID returns API and owning account (lab: api_id globally unique).
func (s *Store) GetAPIGatewayAPIByID(apiID string) (accountID string, api APIGatewayAPI, err error) {
	err = s.db.QueryRow(
		`SELECT account_id, api_id, name, protocol_type, api_endpoint, created_at FROM apigwv2_apis
		 WHERE api_id = ?`,
		apiID,
	).Scan(&accountID, &api.APIID, &api.Name, &api.ProtocolType, &api.APIEndpoint, &api.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", APIGatewayAPI{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return "", APIGatewayAPI{}, fmt.Errorf("get api by id: %w", err)
	}
	return accountID, api, nil
}

// DeleteAPIGatewayAPI deletes an API and related rows.
func (s *Store) DeleteAPIGatewayAPI(accountID, apiID string) error {
	apiID = strings.TrimSpace(apiID)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete api begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM apigwv2_apis WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	if err != nil {
		return fmt.Errorf("delete api: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAPIGatewayNotFound
	}
	_, _ = tx.Exec(`DELETE FROM apigwv2_integrations WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	_, _ = tx.Exec(`DELETE FROM apigwv2_authorizers WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	_, _ = tx.Exec(`DELETE FROM apigwv2_routes WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	_, _ = tx.Exec(`DELETE FROM apigwv2_stages WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	return tx.Commit()
}

// ListAPIGatewayAPIs lists APIs for an account.
func (s *Store) ListAPIGatewayAPIs(accountID string) ([]APIGatewayAPI, error) {
	rows, err := s.db.Query(
		`SELECT api_id, name, protocol_type, api_endpoint, created_at FROM apigwv2_apis
		 WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list apis: %w", err)
	}
	defer rows.Close()
	var out []APIGatewayAPI
	for rows.Next() {
		var a APIGatewayAPI
		if err := rows.Scan(&a.APIID, &a.Name, &a.ProtocolType, &a.APIEndpoint, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("list apis scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateAPIGatewayIntegration creates an AWS_PROXY Lambda integration.
func (s *Store) CreateAPIGatewayIntegration(accountID, apiID, integrationType, integrationURI, payloadFormat, credentialsArn string) (APIGatewayIntegration, error) {
	integrationType = strings.ToUpper(strings.TrimSpace(integrationType))
	integrationURI = strings.TrimSpace(integrationURI)
	payloadFormat = strings.TrimSpace(payloadFormat)
	credentialsArn = strings.TrimSpace(credentialsArn)
	if integrationType == "" {
		integrationType = APIGatewayIntegrationAWSProxy
	}
	if integrationType != APIGatewayIntegrationAWSProxy {
		return APIGatewayIntegration{}, fmt.Errorf("%w: only AWS_PROXY integrations in lab core", ErrAPIGatewayBadRequest)
	}
	if integrationURI == "" {
		return APIGatewayIntegration{}, fmt.Errorf("%w: IntegrationUri required", ErrAPIGatewayBadRequest)
	}
	if _, _, ok := ParseLambdaARNFromSFNResource(integrationURI); !ok {
		return APIGatewayIntegration{}, fmt.Errorf("%w: IntegrationUri must be a Lambda ARN", ErrAPIGatewayBadRequest)
	}
	if payloadFormat == "" {
		payloadFormat = "2.0"
	}
	if _, err := s.GetAPIGatewayAPI(accountID, apiID); err != nil {
		return APIGatewayIntegration{}, err
	}
	id := uuid.NewString()
	_, err := s.db.Exec(
		`INSERT INTO apigwv2_integrations (account_id, api_id, integration_id, integration_type, integration_uri, payload_format_version, credentials_arn)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, apiID, id, integrationType, integrationURI, payloadFormat, credentialsArn,
	)
	if err != nil {
		return APIGatewayIntegration{}, fmt.Errorf("create integration: %w", err)
	}
	return APIGatewayIntegration{
		IntegrationID: id, APIID: apiID, IntegrationType: integrationType,
		IntegrationURI: integrationURI, PayloadFormatVersion: payloadFormat,
		CredentialsArn: credentialsArn,
	}, nil
}

// GetAPIGatewayIntegration returns an integration.
func (s *Store) GetAPIGatewayIntegration(accountID, apiID, integrationID string) (APIGatewayIntegration, error) {
	var in APIGatewayIntegration
	err := s.db.QueryRow(
		`SELECT integration_id, api_id, integration_type, integration_uri, payload_format_version, COALESCE(credentials_arn, '')
		 FROM apigwv2_integrations WHERE account_id = ? AND api_id = ? AND integration_id = ?`,
		accountID, apiID, integrationID,
	).Scan(&in.IntegrationID, &in.APIID, &in.IntegrationType, &in.IntegrationURI, &in.PayloadFormatVersion, &in.CredentialsArn)
	if errors.Is(err, sql.ErrNoRows) {
		return APIGatewayIntegration{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return APIGatewayIntegration{}, fmt.Errorf("get integration: %w", err)
	}
	return in, nil
}

// CreateAPIGatewayAuthorizer creates a JWT authorizer.
func (s *Store) CreateAPIGatewayAuthorizer(accountID, apiID, name, authorizerType, identitySource, issuer string, audience []string) (APIGatewayAuthorizer, error) {
	name = strings.TrimSpace(name)
	authorizerType = strings.ToUpper(strings.TrimSpace(authorizerType))
	identitySource = strings.TrimSpace(identitySource)
	issuer = strings.TrimSpace(issuer)
	if name == "" {
		return APIGatewayAuthorizer{}, fmt.Errorf("%w: Name required", ErrAPIGatewayBadRequest)
	}
	if authorizerType == "" {
		authorizerType = APIGatewayAuthorizerJWT
	}
	if authorizerType != APIGatewayAuthorizerJWT {
		return APIGatewayAuthorizer{}, fmt.Errorf("%w: AuthorizerType must be JWT", ErrAPIGatewayBadRequest)
	}
	if issuer == "" {
		return APIGatewayAuthorizer{}, fmt.Errorf("%w: JwtConfiguration.Issuer required", ErrAPIGatewayBadRequest)
	}
	if err := jwksfetch.ValidateIssuerForConfig(issuer); err != nil {
		return APIGatewayAuthorizer{}, fmt.Errorf("%w: JwtConfiguration.Issuer: %v", ErrAPIGatewayBadRequest, err)
	}
	if len(audience) == 0 {
		return APIGatewayAuthorizer{}, fmt.Errorf("%w: JwtConfiguration.Audience required", ErrAPIGatewayBadRequest)
	}
	if identitySource == "" {
		identitySource = "$request.header.Authorization"
	}
	if _, err := s.GetAPIGatewayAPI(accountID, apiID); err != nil {
		return APIGatewayAuthorizer{}, err
	}
	audJSON, err := json.Marshal(audience)
	if err != nil {
		return APIGatewayAuthorizer{}, fmt.Errorf("marshal audience: %w", err)
	}
	id := uuid.NewString()
	_, err = s.db.Exec(
		`INSERT INTO apigwv2_authorizers (account_id, api_id, authorizer_id, name, authorizer_type, identity_source, jwt_issuer, jwt_audience_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, apiID, id, name, authorizerType, identitySource, issuer, string(audJSON),
	)
	if err != nil {
		return APIGatewayAuthorizer{}, fmt.Errorf("create authorizer: %w", err)
	}
	return APIGatewayAuthorizer{
		AuthorizerID: id, APIID: apiID, Name: name, AuthorizerType: authorizerType,
		IdentitySource: identitySource, JWTIssuer: issuer, JWTAudience: audience,
	}, nil
}

// GetAPIGatewayAuthorizer returns an authorizer.
func (s *Store) GetAPIGatewayAuthorizer(accountID, apiID, authorizerID string) (APIGatewayAuthorizer, error) {
	var a APIGatewayAuthorizer
	var audJSON string
	err := s.db.QueryRow(
		`SELECT authorizer_id, api_id, name, authorizer_type, identity_source, jwt_issuer, jwt_audience_json
		 FROM apigwv2_authorizers WHERE account_id = ? AND api_id = ? AND authorizer_id = ?`,
		accountID, apiID, authorizerID,
	).Scan(&a.AuthorizerID, &a.APIID, &a.Name, &a.AuthorizerType, &a.IdentitySource, &a.JWTIssuer, &audJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return APIGatewayAuthorizer{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return APIGatewayAuthorizer{}, fmt.Errorf("get authorizer: %w", err)
	}
	_ = json.Unmarshal([]byte(audJSON), &a.JWTAudience)
	return a, nil
}

// CreateAPIGatewayRoute creates a route.
func (s *Store) CreateAPIGatewayRoute(accountID, apiID, routeKey, target, authType, authorizerID string) (APIGatewayRoute, error) {
	routeKey = strings.TrimSpace(routeKey)
	target = strings.TrimSpace(target)
	authType = strings.ToUpper(strings.TrimSpace(authType))
	authorizerID = strings.TrimSpace(authorizerID)
	if routeKey == "" {
		return APIGatewayRoute{}, fmt.Errorf("%w: RouteKey required", ErrAPIGatewayBadRequest)
	}
	if target == "" {
		return APIGatewayRoute{}, fmt.Errorf("%w: Target required", ErrAPIGatewayBadRequest)
	}
	if !strings.HasPrefix(target, "integrations/") {
		return APIGatewayRoute{}, fmt.Errorf("%w: Target must be integrations/{integrationId}", ErrAPIGatewayBadRequest)
	}
	if authType == "" {
		authType = APIGatewayAuthNone
	}
	switch authType {
	case APIGatewayAuthNone, APIGatewayAuthJWT, APIGatewayAuthIAM:
	default:
		return APIGatewayRoute{}, fmt.Errorf("%w: AuthorizationType must be NONE, JWT, or AWS_IAM", ErrAPIGatewayBadRequest)
	}
	if authType == APIGatewayAuthJWT {
		if authorizerID == "" {
			return APIGatewayRoute{}, fmt.Errorf("%w: AuthorizerId required for JWT", ErrAPIGatewayBadRequest)
		}
		if _, err := s.GetAPIGatewayAuthorizer(accountID, apiID, authorizerID); err != nil {
			return APIGatewayRoute{}, err
		}
	}
	if authType != APIGatewayAuthJWT {
		authorizerID = ""
	}
	integrationID := strings.TrimPrefix(target, "integrations/")
	if _, err := s.GetAPIGatewayIntegration(accountID, apiID, integrationID); err != nil {
		return APIGatewayRoute{}, err
	}
	if _, err := s.GetAPIGatewayAPI(accountID, apiID); err != nil {
		return APIGatewayRoute{}, err
	}
	id := uuid.NewString()
	_, err := s.db.Exec(
		`INSERT INTO apigwv2_routes (account_id, api_id, route_id, route_key, target, authorization_type, authorizer_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, apiID, id, routeKey, target, authType, authorizerID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return APIGatewayRoute{}, fmt.Errorf("%w: route key already exists", ErrAPIGatewayConflict)
		}
		return APIGatewayRoute{}, fmt.Errorf("create route: %w", err)
	}
	return APIGatewayRoute{
		RouteID: id, APIID: apiID, RouteKey: routeKey, Target: target,
		AuthorizationType: authType, AuthorizerID: authorizerID,
	}, nil
}

// CreateAPIGatewayStage creates a stage.
func (s *Store) CreateAPIGatewayStage(accountID, apiID, stageName string, autoDeploy bool) (APIGatewayStage, error) {
	stageName = strings.TrimSpace(stageName)
	if stageName == "" {
		stageName = "$default"
	}
	if _, err := s.GetAPIGatewayAPI(accountID, apiID); err != nil {
		return APIGatewayStage{}, err
	}
	ad := 0
	if autoDeploy {
		ad = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO apigwv2_stages (account_id, api_id, stage_name, auto_deploy) VALUES (?, ?, ?, ?)`,
		accountID, apiID, stageName, ad,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return APIGatewayStage{}, fmt.Errorf("%w: stage already exists", ErrAPIGatewayConflict)
		}
		return APIGatewayStage{}, fmt.Errorf("create stage: %w", err)
	}
	return APIGatewayStage{APIID: apiID, StageName: stageName, AutoDeploy: autoDeploy}, nil
}

// GetAPIGatewayStage returns a stage.
func (s *Store) GetAPIGatewayStage(accountID, apiID, stageName string) (APIGatewayStage, error) {
	var st APIGatewayStage
	var ad int
	err := s.db.QueryRow(
		`SELECT api_id, stage_name, auto_deploy FROM apigwv2_stages
		 WHERE account_id = ? AND api_id = ? AND stage_name = ?`,
		accountID, apiID, stageName,
	).Scan(&st.APIID, &st.StageName, &ad)
	if errors.Is(err, sql.ErrNoRows) {
		return APIGatewayStage{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return APIGatewayStage{}, fmt.Errorf("get stage: %w", err)
	}
	st.AutoDeploy = ad != 0
	return st, nil
}

// MatchAPIGatewayRoute finds the best route for method + path.
// Prefers exact method+path, then ANY + path, then $default.
func (s *Store) MatchAPIGatewayRoute(accountID, apiID, method, path string) (APIGatewayRoute, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = normalizeAPIGatewayPath(path)
	rows, err := s.db.Query(
		`SELECT route_id, api_id, route_key, target, authorization_type, authorizer_id
		 FROM apigwv2_routes WHERE account_id = ? AND api_id = ?`,
		accountID, apiID,
	)
	if err != nil {
		return APIGatewayRoute{}, fmt.Errorf("match route: %w", err)
	}
	defer rows.Close()
	var exact, anyMatch, def *APIGatewayRoute
	for rows.Next() {
		var r APIGatewayRoute
		if err := rows.Scan(&r.RouteID, &r.APIID, &r.RouteKey, &r.Target, &r.AuthorizationType, &r.AuthorizerID); err != nil {
			return APIGatewayRoute{}, fmt.Errorf("match route scan: %w", err)
		}
		rkMethod, rkPath, ok := ParseAPIGatewayRouteKey(r.RouteKey)
		if !ok {
			continue
		}
		if rkMethod == "$default" {
			cp := r
			def = &cp
			continue
		}
		if normalizeAPIGatewayPath(rkPath) != path {
			continue
		}
		if rkMethod == method {
			cp := r
			exact = &cp
			continue
		}
		if rkMethod == "ANY" {
			cp := r
			anyMatch = &cp
		}
	}
	if err := rows.Err(); err != nil {
		return APIGatewayRoute{}, err
	}
	if exact != nil {
		return *exact, nil
	}
	if anyMatch != nil {
		return *anyMatch, nil
	}
	if def != nil {
		return *def, nil
	}
	return APIGatewayRoute{}, ErrAPIGatewayNotFound
}

// ParseAPIGatewayRouteKey splits "GET /path" or "$default".
func ParseAPIGatewayRouteKey(routeKey string) (method, path string, ok bool) {
	routeKey = strings.TrimSpace(routeKey)
	if routeKey == "$default" {
		return "$default", "/", true
	}
	parts := strings.SplitN(routeKey, " ", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	method = strings.ToUpper(strings.TrimSpace(parts[0]))
	path = strings.TrimSpace(parts[1])
	if method == "" || path == "" {
		return "", "", false
	}
	return method, path, true
}

func normalizeAPIGatewayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

// APIGatewayRouteARN builds execute-api invoke ARN for a route.
// arn:aws:execute-api:region:account:api-id/stage/METHOD/path
func APIGatewayRouteARN(region, accountID, apiID, stage, method, path string) string {
	if region == "" {
		region = DefaultAPIGatewayRegion
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimPrefix(normalizeAPIGatewayPath(path), "/")
	if path == "" {
		return fmt.Sprintf("arn:aws:execute-api:%s:%s:%s/%s/%s/", region, accountID, apiID, stage, method)
	}
	return fmt.Sprintf("arn:aws:execute-api:%s:%s:%s/%s/%s/%s", region, accountID, apiID, stage, method, path)
}

// IntegrationIDFromTarget extracts integration id from integrations/{id}.
func IntegrationIDFromTarget(target string) string {
	return strings.TrimPrefix(strings.TrimSpace(target), "integrations/")
}
