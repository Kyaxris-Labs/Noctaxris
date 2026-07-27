package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	APIGatewayRESTIntegrationAWSProxy = "AWS_PROXY"
	APIGatewayRESTIntegrationMock     = "MOCK"

	APIGatewayRESTAuthNone = "NONE"
	APIGatewayRESTAuthIAM  = "AWS_IAM"
)

const apiGatewayRESTSchema = `
CREATE TABLE IF NOT EXISTS apigw_rest_apis (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  root_resource_id TEXT NOT NULL,
  PRIMARY KEY (account_id, api_id)
);
CREATE TABLE IF NOT EXISTS apigw_rest_resources (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  parent_id TEXT NOT NULL DEFAULT '',
  path_part TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL,
  PRIMARY KEY (account_id, api_id, resource_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_apigw_rest_resource_path
  ON apigw_rest_resources(account_id, api_id, path);
CREATE TABLE IF NOT EXISTS apigw_rest_methods (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  http_method TEXT NOT NULL,
  authorization_type TEXT NOT NULL,
  authorizer_id TEXT NOT NULL DEFAULT '',
  api_key_required INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, api_id, resource_id, http_method)
);
CREATE TABLE IF NOT EXISTS apigw_rest_integrations (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  http_method TEXT NOT NULL,
  integration_type TEXT NOT NULL,
  uri TEXT NOT NULL DEFAULT '',
  integration_http_method TEXT NOT NULL DEFAULT '',
  credentials TEXT NOT NULL DEFAULT '',
  request_templates_json TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY (account_id, api_id, resource_id, http_method)
);
CREATE TABLE IF NOT EXISTS apigw_rest_deployments (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  deployment_id TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, api_id, deployment_id)
);
CREATE TABLE IF NOT EXISTS apigw_rest_stages (
  account_id TEXT NOT NULL,
  api_id TEXT NOT NULL,
  stage_name TEXT NOT NULL,
  deployment_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, api_id, stage_name)
);
`

// RestAPI is an API Gateway REST API (v1) row.
type RestAPI struct {
	APIID          string
	Name           string
	Description    string
	CreatedAt      int64
	RootResourceID string
}

// RestResource is a REST API resource node.
type RestResource struct {
	ResourceID string
	APIID      string
	ParentID   string
	PathPart   string
	Path       string
}

// RestMethod is an HTTP method on a resource.
type RestMethod struct {
	APIID             string
	ResourceID        string
	HTTPMethod        string
	AuthorizationType string
	AuthorizerID      string
	APIKeyRequired    bool
}

// RestIntegration is a method integration (AWS_PROXY or MOCK).
type RestIntegration struct {
	APIID                  string
	ResourceID             string
	HTTPMethod             string
	Type                   string
	URI                    string
	IntegrationHTTPMethod  string
	Credentials            string
	RequestTemplates       map[string]string
}

// RestDeployment is a deployment snapshot id.
type RestDeployment struct {
	APIID        string
	DeploymentID string
	Description  string
	CreatedAt    int64
}

// RestStage binds a stage name to a deployment.
type RestStage struct {
	APIID        string
	StageName    string
	DeploymentID string
	CreatedAt    int64
}

// EnsureAPIGatewayRESTSchema creates REST API Gateway tables if missing.
func EnsureAPIGatewayRESTSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure apigateway rest schema: db is nil")
	}
	if _, err := db.Exec(apiGatewayRESTSchema); err != nil {
		return fmt.Errorf("ensure apigateway rest schema: %w", err)
	}
	return nil
}

// EnsureAPIGatewayRESTSchema ensures tables on an open store.
func (s *Store) EnsureAPIGatewayRESTSchema() error {
	return EnsureAPIGatewayRESTSchema(s.db)
}

func newAPIGatewayRESTShortID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
}

// RestAPIExecuteBase builds the lab execute base URL (without path).
func RestAPIExecuteBase(apiID, stage string) string {
	return "http://127.0.0.1:4566/restapis/" + apiID + "/" + stage + "/_user_request_"
}

// CreateRestAPI creates a REST API with a root resource.
func (s *Store) CreateRestAPI(accountID, name, description string) (RestAPI, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return RestAPI{}, fmt.Errorf("%w: name required", ErrAPIGatewayBadRequest)
	}
	apiID := newAPIGatewayRESTShortID()
	rootID := newAPIGatewayRESTShortID()
	now := time.Now().UTC().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return RestAPI{}, fmt.Errorf("create rest api begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`INSERT INTO apigw_rest_apis (account_id, api_id, name, description, created_at, root_resource_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, apiID, name, description, now, rootID,
	); err != nil {
		return RestAPI{}, fmt.Errorf("create rest api: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO apigw_rest_resources (account_id, api_id, resource_id, parent_id, path_part, path)
		 VALUES (?, ?, ?, '', '', '/')`,
		accountID, apiID, rootID,
	); err != nil {
		return RestAPI{}, fmt.Errorf("create rest root resource: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RestAPI{}, fmt.Errorf("create rest api commit: %w", err)
	}
	return RestAPI{
		APIID: apiID, Name: name, Description: description,
		CreatedAt: now, RootResourceID: rootID,
	}, nil
}

// GetRestAPI returns a REST API by id for an account.
func (s *Store) GetRestAPI(accountID, apiID string) (RestAPI, error) {
	var a RestAPI
	err := s.db.QueryRow(
		`SELECT api_id, name, description, created_at, root_resource_id FROM apigw_rest_apis
		 WHERE account_id = ? AND api_id = ?`,
		accountID, apiID,
	).Scan(&a.APIID, &a.Name, &a.Description, &a.CreatedAt, &a.RootResourceID)
	if err == sql.ErrNoRows {
		return RestAPI{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return RestAPI{}, fmt.Errorf("get rest api: %w", err)
	}
	return a, nil
}

// GetRestAPIByID returns account and API for an api id (execute path).
func (s *Store) GetRestAPIByID(apiID string) (accountID string, api RestAPI, err error) {
	err = s.db.QueryRow(
		`SELECT account_id, api_id, name, description, created_at, root_resource_id FROM apigw_rest_apis
		 WHERE api_id = ?`,
		apiID,
	).Scan(&accountID, &api.APIID, &api.Name, &api.Description, &api.CreatedAt, &api.RootResourceID)
	if err == sql.ErrNoRows {
		return "", RestAPI{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return "", RestAPI{}, fmt.Errorf("get rest api by id: %w", err)
	}
	return accountID, api, nil
}

// ListRestAPIs lists REST APIs for an account.
func (s *Store) ListRestAPIs(accountID string) ([]RestAPI, error) {
	rows, err := s.db.Query(
		`SELECT api_id, name, description, created_at, root_resource_id FROM apigw_rest_apis
		 WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list rest apis: %w", err)
	}
	defer rows.Close()
	var out []RestAPI
	for rows.Next() {
		var a RestAPI
		if err := rows.Scan(&a.APIID, &a.Name, &a.Description, &a.CreatedAt, &a.RootResourceID); err != nil {
			return nil, fmt.Errorf("list rest apis scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteRestAPI deletes an API and related rows.
func (s *Store) DeleteRestAPI(accountID, apiID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete rest api begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM apigw_rest_apis WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	if err != nil {
		return fmt.Errorf("delete rest api: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAPIGatewayNotFound
	}
	_, _ = tx.Exec(`DELETE FROM apigw_rest_resources WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	_, _ = tx.Exec(`DELETE FROM apigw_rest_methods WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	_, _ = tx.Exec(`DELETE FROM apigw_rest_integrations WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	_, _ = tx.Exec(`DELETE FROM apigw_rest_deployments WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	_, _ = tx.Exec(`DELETE FROM apigw_rest_stages WHERE account_id = ? AND api_id = ?`, accountID, apiID)
	return tx.Commit()
}

// CreateRestResource creates a child resource under parentID.
func (s *Store) CreateRestResource(accountID, apiID, parentID, pathPart string) (RestResource, error) {
	pathPart = strings.TrimSpace(pathPart)
	parentID = strings.TrimSpace(parentID)
	if pathPart == "" {
		return RestResource{}, fmt.Errorf("%w: pathPart required", ErrAPIGatewayBadRequest)
	}
	if strings.Contains(pathPart, "/") {
		return RestResource{}, fmt.Errorf("%w: pathPart must not contain /", ErrAPIGatewayBadRequest)
	}
	if _, err := s.GetRestAPI(accountID, apiID); err != nil {
		return RestResource{}, err
	}
	parent, err := s.GetRestResource(accountID, apiID, parentID)
	if err != nil {
		return RestResource{}, err
	}
	path := parent.Path
	if path == "/" {
		path = "/" + pathPart
	} else {
		path = path + "/" + pathPart
	}
	id := newAPIGatewayRESTShortID()
	_, err = s.db.Exec(
		`INSERT INTO apigw_rest_resources (account_id, api_id, resource_id, parent_id, path_part, path)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, apiID, id, parentID, pathPart, path,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return RestResource{}, fmt.Errorf("%w: resource path already exists", ErrAPIGatewayConflict)
		}
		return RestResource{}, fmt.Errorf("create rest resource: %w", err)
	}
	return RestResource{
		ResourceID: id, APIID: apiID, ParentID: parentID, PathPart: pathPart, Path: path,
	}, nil
}

// GetRestResource returns a resource by id.
func (s *Store) GetRestResource(accountID, apiID, resourceID string) (RestResource, error) {
	var r RestResource
	err := s.db.QueryRow(
		`SELECT resource_id, api_id, parent_id, path_part, path FROM apigw_rest_resources
		 WHERE account_id = ? AND api_id = ? AND resource_id = ?`,
		accountID, apiID, resourceID,
	).Scan(&r.ResourceID, &r.APIID, &r.ParentID, &r.PathPart, &r.Path)
	if err == sql.ErrNoRows {
		return RestResource{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return RestResource{}, fmt.Errorf("get rest resource: %w", err)
	}
	return r, nil
}

// ListRestResources lists resources for an API.
func (s *Store) ListRestResources(accountID, apiID string) ([]RestResource, error) {
	if _, err := s.GetRestAPI(accountID, apiID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT resource_id, api_id, parent_id, path_part, path FROM apigw_rest_resources
		 WHERE account_id = ? AND api_id = ? ORDER BY path`,
		accountID, apiID,
	)
	if err != nil {
		return nil, fmt.Errorf("list rest resources: %w", err)
	}
	defer rows.Close()
	var out []RestResource
	for rows.Next() {
		var r RestResource
		if err := rows.Scan(&r.ResourceID, &r.APIID, &r.ParentID, &r.PathPart, &r.Path); err != nil {
			return nil, fmt.Errorf("list rest resources scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRestResource deletes a non-root resource and its methods/integrations.
func (s *Store) DeleteRestResource(accountID, apiID, resourceID string) error {
	api, err := s.GetRestAPI(accountID, apiID)
	if err != nil {
		return err
	}
	if resourceID == api.RootResourceID {
		return fmt.Errorf("%w: cannot delete root resource", ErrAPIGatewayBadRequest)
	}
	res, err := s.db.Exec(
		`DELETE FROM apigw_rest_resources WHERE account_id = ? AND api_id = ? AND resource_id = ?`,
		accountID, apiID, resourceID,
	)
	if err != nil {
		return fmt.Errorf("delete rest resource: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAPIGatewayNotFound
	}
	_, _ = s.db.Exec(
		`DELETE FROM apigw_rest_methods WHERE account_id = ? AND api_id = ? AND resource_id = ?`,
		accountID, apiID, resourceID,
	)
	_, _ = s.db.Exec(
		`DELETE FROM apigw_rest_integrations WHERE account_id = ? AND api_id = ? AND resource_id = ?`,
		accountID, apiID, resourceID,
	)
	return nil
}

// PutRestMethod creates or replaces a method on a resource.
func (s *Store) PutRestMethod(accountID, apiID, resourceID, httpMethod, authorizationType, authorizerID string, apiKeyRequired bool) (RestMethod, error) {
	httpMethod = strings.ToUpper(strings.TrimSpace(httpMethod))
	authorizationType = strings.ToUpper(strings.TrimSpace(authorizationType))
	authorizerID = strings.TrimSpace(authorizerID)
	if httpMethod == "" {
		return RestMethod{}, fmt.Errorf("%w: httpMethod required", ErrAPIGatewayBadRequest)
	}
	if authorizationType == "" {
		authorizationType = APIGatewayRESTAuthNone
	}
	switch authorizationType {
	case APIGatewayRESTAuthNone, APIGatewayRESTAuthIAM:
		// lab core
	default:
		return RestMethod{}, fmt.Errorf("%w: authorizationType must be NONE or AWS_IAM in lab core", ErrAPIGatewayBadRequest)
	}
	if _, err := s.GetRestResource(accountID, apiID, resourceID); err != nil {
		return RestMethod{}, err
	}
	keyInt := 0
	if apiKeyRequired {
		keyInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO apigw_rest_methods
		 (account_id, api_id, resource_id, http_method, authorization_type, authorizer_id, api_key_required)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, api_id, resource_id, http_method) DO UPDATE SET
		   authorization_type = excluded.authorization_type,
		   authorizer_id = excluded.authorizer_id,
		   api_key_required = excluded.api_key_required`,
		accountID, apiID, resourceID, httpMethod, authorizationType, authorizerID, keyInt,
	)
	if err != nil {
		return RestMethod{}, fmt.Errorf("put rest method: %w", err)
	}
	return RestMethod{
		APIID: apiID, ResourceID: resourceID, HTTPMethod: httpMethod,
		AuthorizationType: authorizationType, AuthorizerID: authorizerID, APIKeyRequired: apiKeyRequired,
	}, nil
}

// GetRestMethod returns a method.
func (s *Store) GetRestMethod(accountID, apiID, resourceID, httpMethod string) (RestMethod, error) {
	httpMethod = strings.ToUpper(strings.TrimSpace(httpMethod))
	var m RestMethod
	var keyInt int
	err := s.db.QueryRow(
		`SELECT api_id, resource_id, http_method, authorization_type, authorizer_id, api_key_required
		 FROM apigw_rest_methods
		 WHERE account_id = ? AND api_id = ? AND resource_id = ? AND http_method = ?`,
		accountID, apiID, resourceID, httpMethod,
	).Scan(&m.APIID, &m.ResourceID, &m.HTTPMethod, &m.AuthorizationType, &m.AuthorizerID, &keyInt)
	if err == sql.ErrNoRows {
		return RestMethod{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return RestMethod{}, fmt.Errorf("get rest method: %w", err)
	}
	m.APIKeyRequired = keyInt != 0
	return m, nil
}

// DeleteRestMethod deletes a method and its integration.
func (s *Store) DeleteRestMethod(accountID, apiID, resourceID, httpMethod string) error {
	httpMethod = strings.ToUpper(strings.TrimSpace(httpMethod))
	res, err := s.db.Exec(
		`DELETE FROM apigw_rest_methods WHERE account_id = ? AND api_id = ? AND resource_id = ? AND http_method = ?`,
		accountID, apiID, resourceID, httpMethod,
	)
	if err != nil {
		return fmt.Errorf("delete rest method: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAPIGatewayNotFound
	}
	_, _ = s.db.Exec(
		`DELETE FROM apigw_rest_integrations WHERE account_id = ? AND api_id = ? AND resource_id = ? AND http_method = ?`,
		accountID, apiID, resourceID, httpMethod,
	)
	return nil
}

// PutRestIntegration creates or replaces an integration (AWS_PROXY Lambda, MOCK, or opt-in HTTP_PROXY / VPC_LINK).
func (s *Store) PutRestIntegration(
	accountID, apiID, resourceID, httpMethod, integrationType, uri, integrationHTTPMethod, credentials string,
	requestTemplates map[string]string,
) (RestIntegration, error) {
	httpMethod = strings.ToUpper(strings.TrimSpace(httpMethod))
	integrationType = strings.ToUpper(strings.TrimSpace(integrationType))
	uri = strings.TrimSpace(uri)
	integrationHTTPMethod = strings.ToUpper(strings.TrimSpace(integrationHTTPMethod))
	credentials = strings.TrimSpace(credentials)
	if httpMethod == "" {
		return RestIntegration{}, fmt.Errorf("%w: httpMethod required", ErrAPIGatewayBadRequest)
	}
	if _, err := s.GetRestMethod(accountID, apiID, resourceID, httpMethod); err != nil {
		return RestIntegration{}, err
	}
	switch integrationType {
	case APIGatewayRESTIntegrationAWSProxy:
		normalized, err := NormalizeRestAPILambdaURI(uri)
		if err != nil {
			return RestIntegration{}, fmt.Errorf("%w: %s", ErrAPIGatewayBadRequest, err.Error())
		}
		uri = normalized
		if integrationHTTPMethod == "" {
			integrationHTTPMethod = "POST"
		}
	case APIGatewayRESTIntegrationMock:
		uri = ""
	case APIGatewayIntegrationHTTPProxy, APIGatewayIntegrationVPCLink:
		if err := ValidateAPIGatewayHTTPProxyURI(uri); err != nil {
			return RestIntegration{}, err
		}
		if credentials != "" {
			return RestIntegration{}, fmt.Errorf("%w: credentials are not supported for HTTP_PROXY / VPC_LINK", ErrAPIGatewayBadRequest)
		}
		if integrationHTTPMethod == "" {
			integrationHTTPMethod = httpMethod
		}
	default:
		return RestIntegration{}, fmt.Errorf("%w: only AWS_PROXY, MOCK, or HTTP_PROXY / VPC_LINK (when %s=1) integrations",
			ErrAPIGatewayBadRequest, EnvAPIGatewayHTTPProxy)
	}
	if requestTemplates == nil {
		requestTemplates = map[string]string{}
	}
	tplJSON, err := json.Marshal(requestTemplates)
	if err != nil {
		return RestIntegration{}, fmt.Errorf("marshal request templates: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO apigw_rest_integrations
		 (account_id, api_id, resource_id, http_method, integration_type, uri, integration_http_method, credentials, request_templates_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, api_id, resource_id, http_method) DO UPDATE SET
		   integration_type = excluded.integration_type,
		   uri = excluded.uri,
		   integration_http_method = excluded.integration_http_method,
		   credentials = excluded.credentials,
		   request_templates_json = excluded.request_templates_json`,
		accountID, apiID, resourceID, httpMethod, integrationType, uri, integrationHTTPMethod, credentials, string(tplJSON),
	)
	if err != nil {
		return RestIntegration{}, fmt.Errorf("put rest integration: %w", err)
	}
	return RestIntegration{
		APIID: apiID, ResourceID: resourceID, HTTPMethod: httpMethod,
		Type: integrationType, URI: uri, IntegrationHTTPMethod: integrationHTTPMethod,
		Credentials: credentials, RequestTemplates: requestTemplates,
	}, nil
}

// GetRestIntegration returns an integration.
func (s *Store) GetRestIntegration(accountID, apiID, resourceID, httpMethod string) (RestIntegration, error) {
	httpMethod = strings.ToUpper(strings.TrimSpace(httpMethod))
	var in RestIntegration
	var tplJSON string
	err := s.db.QueryRow(
		`SELECT api_id, resource_id, http_method, integration_type, uri, integration_http_method, credentials, request_templates_json
		 FROM apigw_rest_integrations
		 WHERE account_id = ? AND api_id = ? AND resource_id = ? AND http_method = ?`,
		accountID, apiID, resourceID, httpMethod,
	).Scan(
		&in.APIID, &in.ResourceID, &in.HTTPMethod, &in.Type, &in.URI,
		&in.IntegrationHTTPMethod, &in.Credentials, &tplJSON,
	)
	if err == sql.ErrNoRows {
		return RestIntegration{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return RestIntegration{}, fmt.Errorf("get rest integration: %w", err)
	}
	in.RequestTemplates = map[string]string{}
	_ = json.Unmarshal([]byte(tplJSON), &in.RequestTemplates)
	return in, nil
}

// NormalizeRestAPILambdaURI accepts a Lambda ARN or API Gateway Lambda invoke URI.
func NormalizeRestAPILambdaURI(uri string) (string, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", fmt.Errorf("uri required for AWS_PROXY")
	}
	if strings.HasPrefix(uri, "arn:aws:lambda:") {
		parts := strings.Split(uri, ":")
		if len(parts) >= 7 && parts[5] == "function" && parts[3] != "" && parts[4] != "" && parts[6] != "" {
			return fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", parts[3], parts[4], parts[6]), nil
		}
		return "", fmt.Errorf("uri must be a Lambda ARN or apigateway Lambda invoke URI")
	}
	normalized, err := NormalizeAPIGatewayAuthorizerURI(uri)
	if err != nil {
		return "", fmt.Errorf("uri must be a Lambda ARN or apigateway Lambda invoke URI")
	}
	return normalized, nil
}

// CreateRestDeployment creates a deployment; if stageName is set, creates/updates the stage.
func (s *Store) CreateRestDeployment(accountID, apiID, description, stageName string) (RestDeployment, error) {
	if _, err := s.GetRestAPI(accountID, apiID); err != nil {
		return RestDeployment{}, err
	}
	id := newAPIGatewayRESTShortID()
	now := time.Now().UTC().UnixMilli()
	description = strings.TrimSpace(description)
	stageName = strings.TrimSpace(stageName)
	tx, err := s.db.Begin()
	if err != nil {
		return RestDeployment{}, fmt.Errorf("create rest deployment begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`INSERT INTO apigw_rest_deployments (account_id, api_id, deployment_id, description, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, apiID, id, description, now,
	); err != nil {
		return RestDeployment{}, fmt.Errorf("create rest deployment: %w", err)
	}
	if stageName != "" {
		if _, err := tx.Exec(
			`INSERT INTO apigw_rest_stages (account_id, api_id, stage_name, deployment_id, created_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, api_id, stage_name) DO UPDATE SET
			   deployment_id = excluded.deployment_id`,
			accountID, apiID, stageName, id, now,
		); err != nil {
			return RestDeployment{}, fmt.Errorf("create rest deployment stage: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return RestDeployment{}, fmt.Errorf("create rest deployment commit: %w", err)
	}
	return RestDeployment{APIID: apiID, DeploymentID: id, Description: description, CreatedAt: now}, nil
}

// CreateRestStage creates a stage bound to a deployment.
func (s *Store) CreateRestStage(accountID, apiID, stageName, deploymentID string) (RestStage, error) {
	stageName = strings.TrimSpace(stageName)
	deploymentID = strings.TrimSpace(deploymentID)
	if stageName == "" {
		return RestStage{}, fmt.Errorf("%w: stageName required", ErrAPIGatewayBadRequest)
	}
	if deploymentID == "" {
		return RestStage{}, fmt.Errorf("%w: deploymentId required", ErrAPIGatewayBadRequest)
	}
	if _, err := s.GetRestAPI(accountID, apiID); err != nil {
		return RestStage{}, err
	}
	var exists string
	err := s.db.QueryRow(
		`SELECT deployment_id FROM apigw_rest_deployments WHERE account_id = ? AND api_id = ? AND deployment_id = ?`,
		accountID, apiID, deploymentID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return RestStage{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return RestStage{}, fmt.Errorf("get rest deployment: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO apigw_rest_stages (account_id, api_id, stage_name, deployment_id, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, api_id, stage_name) DO UPDATE SET
		   deployment_id = excluded.deployment_id`,
		accountID, apiID, stageName, deploymentID, now,
	)
	if err != nil {
		return RestStage{}, fmt.Errorf("create rest stage: %w", err)
	}
	return RestStage{APIID: apiID, StageName: stageName, DeploymentID: deploymentID, CreatedAt: now}, nil
}

// GetRestStage returns a stage.
func (s *Store) GetRestStage(accountID, apiID, stageName string) (RestStage, error) {
	var st RestStage
	err := s.db.QueryRow(
		`SELECT api_id, stage_name, deployment_id, created_at FROM apigw_rest_stages
		 WHERE account_id = ? AND api_id = ? AND stage_name = ?`,
		accountID, apiID, stageName,
	).Scan(&st.APIID, &st.StageName, &st.DeploymentID, &st.CreatedAt)
	if err == sql.ErrNoRows {
		return RestStage{}, ErrAPIGatewayNotFound
	}
	if err != nil {
		return RestStage{}, fmt.Errorf("get rest stage: %w", err)
	}
	return st, nil
}

// MatchRestAPIRoute finds the best resource+method for an invoke path.
func (s *Store) MatchRestAPIRoute(accountID, apiID, httpMethod, requestPath string) (RestResource, RestMethod, error) {
	httpMethod = strings.ToUpper(strings.TrimSpace(httpMethod))
	requestPath = normalizeRestInvokePath(requestPath)
	resources, err := s.ListRestResources(accountID, apiID)
	if err != nil {
		return RestResource{}, RestMethod{}, err
	}
	var best RestResource
	bestScore := -1
	for _, r := range resources {
		score, ok := restResourceMatchScore(r.Path, requestPath)
		if !ok {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = r
		}
	}
	if bestScore < 0 {
		return RestResource{}, RestMethod{}, ErrAPIGatewayNotFound
	}
	method, err := s.GetRestMethod(accountID, apiID, best.ResourceID, httpMethod)
	if err != nil {
		if httpMethod != "ANY" {
			method, err = s.GetRestMethod(accountID, apiID, best.ResourceID, "ANY")
		}
		if err != nil {
			return RestResource{}, RestMethod{}, err
		}
	}
	return best, method, nil
}

func normalizeRestInvokePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimSuffix(p, "/")
}

// restResourceMatchScore returns specificity score; greedy {proxy+} allowed.
func restResourceMatchScore(resourcePath, requestPath string) (int, bool) {
	resourcePath = normalizeRestInvokePath(resourcePath)
	requestPath = normalizeRestInvokePath(requestPath)
	rParts := splitRestPath(resourcePath)
	qParts := splitRestPath(requestPath)
	score := 0
	ri, qi := 0, 0
	for ri < len(rParts) {
		rp := rParts[ri]
		if rp == "{proxy+}" {
			if qi > len(qParts) {
				return 0, false
			}
			score += 1
			return score, true
		}
		if qi >= len(qParts) {
			return 0, false
		}
		qp := qParts[qi]
		if strings.HasPrefix(rp, "{") && strings.HasSuffix(rp, "}") {
			score += 1
		} else if rp == qp {
			score += 10
		} else {
			return 0, false
		}
		ri++
		qi++
	}
	if qi != len(qParts) {
		return 0, false
	}
	return score, true
}

func splitRestPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// RestAPIMethodARN builds an execute-api resource ARN for IAM auth.
func RestAPIMethodARN(region, accountID, apiID, stage, httpMethod, resourcePath string) string {
	if region == "" {
		region = DefaultAPIGatewayRegion
	}
	resourcePath = normalizeRestInvokePath(resourcePath)
	return fmt.Sprintf("arn:aws:execute-api:%s:%s:%s/%s/%s%s",
		region, accountID, apiID, stage, strings.ToUpper(httpMethod), resourcePath)
}
