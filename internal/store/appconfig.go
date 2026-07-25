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
	ErrAppConfigNotFound      = errors.New("ResourceNotFoundException")
	ErrAppConfigAlreadyExists = errors.New("ConflictException")
	ErrAppConfigBadRequest    = errors.New("BadRequestException")
)

const (
	DefaultAppConfigRegion = "us-east-1"
)

const appconfigSchema = `
CREATE TABLE IF NOT EXISTS appconfig_applications (
  account_id TEXT NOT NULL,
  application_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, application_id),
  UNIQUE (account_id, name)
);
CREATE TABLE IF NOT EXISTS appconfig_environments (
  account_id TEXT NOT NULL,
  application_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, application_id, environment_id),
  UNIQUE (account_id, application_id, name)
);
CREATE TABLE IF NOT EXISTS appconfig_profiles (
  account_id TEXT NOT NULL,
  application_id TEXT NOT NULL,
  profile_id TEXT NOT NULL,
  name TEXT NOT NULL,
  location_uri TEXT NOT NULL DEFAULT 'hosted',
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, application_id, profile_id),
  UNIQUE (account_id, application_id, name)
);
CREATE TABLE IF NOT EXISTS appconfig_hosted_versions (
  account_id TEXT NOT NULL,
  application_id TEXT NOT NULL,
  profile_id TEXT NOT NULL,
  version_number INTEGER NOT NULL,
  content BLOB NOT NULL,
  content_type TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, application_id, profile_id, version_number)
);
CREATE TABLE IF NOT EXISTS appconfig_sessions (
  token TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  application_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  profile_id TEXT NOT NULL,
  next_version INTEGER NOT NULL DEFAULT 0,
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS appconfig_deployments (
  account_id TEXT NOT NULL,
  application_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  profile_id TEXT NOT NULL,
  deployment_id TEXT NOT NULL,
  deployment_number INTEGER NOT NULL,
  configuration_version INTEGER NOT NULL,
  state TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, deployment_id),
  UNIQUE (account_id, application_id, deployment_number)
);
CREATE TABLE IF NOT EXISTS appconfig_deployed_pointers (
  account_id TEXT NOT NULL,
  application_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  profile_id TEXT NOT NULL,
  configuration_version INTEGER NOT NULL,
  deployment_id TEXT NOT NULL,
  PRIMARY KEY (account_id, application_id, environment_id, profile_id)
);
`

// AppConfigApplication is an application row.
type AppConfigApplication struct {
	ID          string
	Name        string
	Description string
}

// AppConfigEnvironment is an environment row.
type AppConfigEnvironment struct {
	ApplicationID string
	ID            string
	Name          string
	Description   string
}

// AppConfigProfile is a configuration profile row.
type AppConfigProfile struct {
	ApplicationID string
	ID            string
	Name          string
	LocationURI   string
	Description   string
}

// AppConfigHostedVersion is a hosted configuration version.
type AppConfigHostedVersion struct {
	ApplicationID string
	ProfileID     string
	VersionNumber int
	Content       []byte
	ContentType   string
}

// AppConfigDeployment is a synchronous lab deployment record.
type AppConfigDeployment struct {
	ApplicationID        string
	EnvironmentID        string
	ProfileID            string
	DeploymentID         string
	DeploymentNumber     int
	ConfigurationVersion int
	State                string
}

// EnsureAppConfigSchema creates AppConfig tables if missing.
func EnsureAppConfigSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure appconfig schema: db is nil")
	}
	if _, err := db.Exec(appconfigSchema); err != nil {
		return fmt.Errorf("ensure appconfig schema: %w", err)
	}
	return nil
}

// EnsureAppConfigSchema ensures AppConfig tables on an open store.
func (s *Store) EnsureAppConfigSchema() error {
	return EnsureAppConfigSchema(s.db)
}

// CreateAppConfigApplication creates an application.
func (s *Store) CreateAppConfigApplication(accountID, name, description string) (AppConfigApplication, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return AppConfigApplication{}, fmt.Errorf("create application: Name is required")
	}
	id := shortID()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO appconfig_applications (account_id, application_id, name, description, created_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, id, name, description, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return AppConfigApplication{}, ErrAppConfigAlreadyExists
		}
		return AppConfigApplication{}, fmt.Errorf("create application: %w", err)
	}
	return AppConfigApplication{ID: id, Name: name, Description: description}, nil
}

// CreateAppConfigEnvironment creates an environment under an application.
func (s *Store) CreateAppConfigEnvironment(accountID, applicationID, name, description string) (AppConfigEnvironment, error) {
	name = strings.TrimSpace(name)
	applicationID = strings.TrimSpace(applicationID)
	if name == "" || applicationID == "" {
		return AppConfigEnvironment{}, fmt.Errorf("create environment: ApplicationId and Name are required")
	}
	if _, err := s.getAppConfigApplication(accountID, applicationID); err != nil {
		return AppConfigEnvironment{}, err
	}
	id := shortID()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO appconfig_environments (account_id, application_id, environment_id, name, description, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, applicationID, id, name, description, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return AppConfigEnvironment{}, ErrAppConfigAlreadyExists
		}
		return AppConfigEnvironment{}, fmt.Errorf("create environment: %w", err)
	}
	return AppConfigEnvironment{ApplicationID: applicationID, ID: id, Name: name, Description: description}, nil
}

// CreateAppConfigProfile creates a configuration profile (hosted).
func (s *Store) CreateAppConfigProfile(accountID, applicationID, name, description string) (AppConfigProfile, error) {
	name = strings.TrimSpace(name)
	applicationID = strings.TrimSpace(applicationID)
	if name == "" || applicationID == "" {
		return AppConfigProfile{}, fmt.Errorf("create profile: ApplicationId and Name are required")
	}
	if _, err := s.getAppConfigApplication(accountID, applicationID); err != nil {
		return AppConfigProfile{}, err
	}
	id := shortID()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO appconfig_profiles (account_id, application_id, profile_id, name, location_uri, description, created_at)
		 VALUES (?, ?, ?, ?, 'hosted', ?, ?)`,
		accountID, applicationID, id, name, description, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return AppConfigProfile{}, ErrAppConfigAlreadyExists
		}
		return AppConfigProfile{}, fmt.Errorf("create profile: %w", err)
	}
	return AppConfigProfile{ApplicationID: applicationID, ID: id, Name: name, LocationURI: "hosted", Description: description}, nil
}

// CreateAppConfigHostedVersion writes a new hosted configuration version.
func (s *Store) CreateAppConfigHostedVersion(accountID, applicationID, profileID, contentType string, content []byte) (AppConfigHostedVersion, error) {
	if _, err := s.getAppConfigProfile(accountID, applicationID, profileID); err != nil {
		return AppConfigHostedVersion{}, err
	}
	if contentType == "" {
		contentType = "application/json"
	}
	var max sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(version_number) FROM appconfig_hosted_versions
		 WHERE account_id = ? AND application_id = ? AND profile_id = ?`,
		accountID, applicationID, profileID,
	).Scan(&max)
	if err != nil {
		return AppConfigHostedVersion{}, fmt.Errorf("hosted version max: %w", err)
	}
	next := 1
	if max.Valid {
		next = int(max.Int64) + 1
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO appconfig_hosted_versions (account_id, application_id, profile_id, version_number, content, content_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, applicationID, profileID, next, content, contentType, now,
	)
	if err != nil {
		return AppConfigHostedVersion{}, fmt.Errorf("create hosted version: %w", err)
	}
	return AppConfigHostedVersion{
		ApplicationID: applicationID, ProfileID: profileID, VersionNumber: next,
		Content: content, ContentType: contentType,
	}, nil
}

// GetAppConfigConfiguration returns deployed hosted content for an environment (GetConfiguration lite).
func (s *Store) GetAppConfigConfiguration(accountID, applicationID, environmentID, profileID string) (AppConfigHostedVersion, error) {
	if _, err := s.getAppConfigEnvironment(accountID, applicationID, environmentID); err != nil {
		return AppConfigHostedVersion{}, err
	}
	if _, err := s.getAppConfigProfile(accountID, applicationID, profileID); err != nil {
		return AppConfigHostedVersion{}, err
	}
	return s.deployedHostedVersion(accountID, applicationID, environmentID, profileID)
}

// StartAppConfigDeployment marks a hosted version as deployed to an environment (immediate DEPLOYED).
func (s *Store) StartAppConfigDeployment(
	accountID, applicationID, environmentID, profileID string, configurationVersion int,
) (AppConfigDeployment, error) {
	if configurationVersion < 1 {
		return AppConfigDeployment{}, ErrAppConfigBadRequest
	}
	if _, err := s.getAppConfigEnvironment(accountID, applicationID, environmentID); err != nil {
		return AppConfigDeployment{}, err
	}
	if _, err := s.getAppConfigProfile(accountID, applicationID, profileID); err != nil {
		return AppConfigDeployment{}, err
	}
	if _, err := s.getAppConfigHostedVersion(accountID, applicationID, profileID, configurationVersion); err != nil {
		return AppConfigDeployment{}, err
	}
	var maxNum sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(deployment_number) FROM appconfig_deployments
		 WHERE account_id = ? AND application_id = ?`,
		accountID, applicationID,
	).Scan(&maxNum)
	if err != nil {
		return AppConfigDeployment{}, fmt.Errorf("deployment number max: %w", err)
	}
	deploymentNumber := 1
	if maxNum.Valid {
		deploymentNumber = int(maxNum.Int64) + 1
	}
	deploymentID := shortID()
	now := time.Now().UTC().UnixMilli()
	state := "DEPLOYED"
	tx, err := s.db.Begin()
	if err != nil {
		return AppConfigDeployment{}, fmt.Errorf("start deployment tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(
		`INSERT INTO appconfig_deployments
		 (account_id, application_id, environment_id, profile_id, deployment_id, deployment_number, configuration_version, state, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, applicationID, environmentID, profileID, deploymentID, deploymentNumber, configurationVersion, state, now,
	)
	if err != nil {
		return AppConfigDeployment{}, fmt.Errorf("insert deployment: %w", err)
	}
	_, err = tx.Exec(
		`INSERT INTO appconfig_deployed_pointers
		 (account_id, application_id, environment_id, profile_id, configuration_version, deployment_id)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, application_id, environment_id, profile_id) DO UPDATE SET
		   configuration_version = excluded.configuration_version,
		   deployment_id = excluded.deployment_id`,
		accountID, applicationID, environmentID, profileID, configurationVersion, deploymentID,
	)
	if err != nil {
		return AppConfigDeployment{}, fmt.Errorf("upsert deployed pointer: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AppConfigDeployment{}, fmt.Errorf("commit deployment: %w", err)
	}
	return AppConfigDeployment{
		ApplicationID:        applicationID,
		EnvironmentID:        environmentID,
		ProfileID:            profileID,
		DeploymentID:         deploymentID,
		DeploymentNumber:     deploymentNumber,
		ConfigurationVersion: configurationVersion,
		State:                state,
	}, nil
}

// GetAppConfigDeployment returns a deployment by id.
func (s *Store) GetAppConfigDeployment(accountID, deploymentID string) (AppConfigDeployment, error) {
	deploymentID = strings.TrimSpace(deploymentID)
	var d AppConfigDeployment
	err := s.db.QueryRow(
		`SELECT application_id, environment_id, profile_id, deployment_id, deployment_number, configuration_version, state
		 FROM appconfig_deployments WHERE account_id = ? AND deployment_id = ?`,
		accountID, deploymentID,
	).Scan(&d.ApplicationID, &d.EnvironmentID, &d.ProfileID, &d.DeploymentID, &d.DeploymentNumber, &d.ConfigurationVersion, &d.State)
	if errors.Is(err, sql.ErrNoRows) {
		return AppConfigDeployment{}, ErrAppConfigNotFound
	}
	if err != nil {
		return AppConfigDeployment{}, fmt.Errorf("get deployment: %w", err)
	}
	return d, nil
}

// ListAppConfigDeployments lists deployments for an application, optionally filtered by environment and profile.
func (s *Store) ListAppConfigDeployments(accountID, applicationID, environmentID, profileID string) ([]AppConfigDeployment, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return nil, fmt.Errorf("list deployments: ApplicationId is required")
	}
	if _, err := s.getAppConfigApplication(accountID, applicationID); err != nil {
		return nil, err
	}
	query := `SELECT application_id, environment_id, profile_id, deployment_id, deployment_number, configuration_version, state
		 FROM appconfig_deployments WHERE account_id = ? AND application_id = ?`
	args := []any{accountID, applicationID}
	if strings.TrimSpace(environmentID) != "" {
		query += ` AND environment_id = ?`
		args = append(args, environmentID)
	}
	if strings.TrimSpace(profileID) != "" {
		query += ` AND profile_id = ?`
		args = append(args, profileID)
	}
	query += ` ORDER BY deployment_number DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	defer rows.Close()
	var out []AppConfigDeployment
	for rows.Next() {
		var d AppConfigDeployment
		if err := rows.Scan(&d.ApplicationID, &d.EnvironmentID, &d.ProfileID, &d.DeploymentID, &d.DeploymentNumber, &d.ConfigurationVersion, &d.State); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list deployments rows: %w", err)
	}
	return out, nil
}

// StartAppConfigSession creates an AppConfigData session token.
func (s *Store) StartAppConfigSession(accountID, applicationID, environmentID, profileID string) (token string, err error) {
	if _, err := s.getAppConfigEnvironment(accountID, applicationID, environmentID); err != nil {
		return "", err
	}
	if _, err := s.getAppConfigProfile(accountID, applicationID, profileID); err != nil {
		return "", err
	}
	token = uuid.NewString()
	expires := time.Now().UTC().Add(24 * time.Hour).Unix()
	_, err = s.db.Exec(
		`INSERT INTO appconfig_sessions (token, account_id, application_id, environment_id, profile_id, next_version, expires_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?)`,
		token, accountID, applicationID, environmentID, profileID, expires,
	)
	if err != nil {
		return "", fmt.Errorf("start config session: %w", err)
	}
	return token, nil
}

// GetLatestAppConfigConfiguration returns content for a session and advances the token.
func (s *Store) GetLatestAppConfigConfiguration(token string) (content []byte, contentType, nextToken string, versionLabel string, err error) {
	token = strings.TrimSpace(token)
	var accountID, applicationID, environmentID, profileID string
	var nextVersion int
	var expires int64
	err = s.db.QueryRow(
		`SELECT account_id, application_id, environment_id, profile_id, next_version, expires_at
		 FROM appconfig_sessions WHERE token = ?`,
		token,
	).Scan(&accountID, &applicationID, &environmentID, &profileID, &nextVersion, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", "", "", ErrAppConfigNotFound
	}
	if err != nil {
		return nil, "", "", "", fmt.Errorf("get latest configuration: %w", err)
	}
	if time.Now().UTC().Unix() > expires {
		return nil, "", "", "", ErrAppConfigBadRequest
	}
	ver, err := s.deployedHostedVersion(accountID, applicationID, environmentID, profileID)
	if err != nil {
		return nil, "", "", "", err
	}
	// If client already has this version (next_version == ver), return empty (304-shaped).
	if nextVersion == ver.VersionNumber {
		newToken := uuid.NewString()
		_, _ = s.db.Exec(`DELETE FROM appconfig_sessions WHERE token = ?`, token)
		_, err = s.db.Exec(
			`INSERT INTO appconfig_sessions (token, account_id, application_id, environment_id, profile_id, next_version, expires_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			newToken, accountID, applicationID, environmentID, profileID, nextVersion, expires,
		)
		if err != nil {
			return nil, "", "", "", fmt.Errorf("rotate session: %w", err)
		}
		return nil, ver.ContentType, newToken, fmt.Sprintf("%d", ver.VersionNumber), nil
	}
	newToken := uuid.NewString()
	_, _ = s.db.Exec(`DELETE FROM appconfig_sessions WHERE token = ?`, token)
	_, err = s.db.Exec(
		`INSERT INTO appconfig_sessions (token, account_id, application_id, environment_id, profile_id, next_version, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		newToken, accountID, applicationID, environmentID, profileID, ver.VersionNumber, expires,
	)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("rotate session after fetch: %w", err)
	}
	return ver.Content, ver.ContentType, newToken, fmt.Sprintf("%d", ver.VersionNumber), nil
}

func (s *Store) deployedHostedVersion(accountID, applicationID, environmentID, profileID string) (AppConfigHostedVersion, error) {
	var version int
	err := s.db.QueryRow(
		`SELECT configuration_version FROM appconfig_deployed_pointers
		 WHERE account_id = ? AND application_id = ? AND environment_id = ? AND profile_id = ?`,
		accountID, applicationID, environmentID, profileID,
	).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return AppConfigHostedVersion{}, ErrAppConfigNotFound
	}
	if err != nil {
		return AppConfigHostedVersion{}, fmt.Errorf("deployed pointer: %w", err)
	}
	return s.getAppConfigHostedVersion(accountID, applicationID, profileID, version)
}

func (s *Store) getAppConfigHostedVersion(accountID, applicationID, profileID string, versionNumber int) (AppConfigHostedVersion, error) {
	var v AppConfigHostedVersion
	err := s.db.QueryRow(
		`SELECT application_id, profile_id, version_number, content, content_type FROM appconfig_hosted_versions
		 WHERE account_id = ? AND application_id = ? AND profile_id = ? AND version_number = ?`,
		accountID, applicationID, profileID, versionNumber,
	).Scan(&v.ApplicationID, &v.ProfileID, &v.VersionNumber, &v.Content, &v.ContentType)
	if errors.Is(err, sql.ErrNoRows) {
		return AppConfigHostedVersion{}, ErrAppConfigNotFound
	}
	if err != nil {
		return AppConfigHostedVersion{}, fmt.Errorf("get hosted version: %w", err)
	}
	return v, nil
}

func (s *Store) latestHostedVersion(accountID, applicationID, profileID string) (AppConfigHostedVersion, error) {
	var v AppConfigHostedVersion
	err := s.db.QueryRow(
		`SELECT application_id, profile_id, version_number, content, content_type FROM appconfig_hosted_versions
		 WHERE account_id = ? AND application_id = ? AND profile_id = ?
		 ORDER BY version_number DESC LIMIT 1`,
		accountID, applicationID, profileID,
	).Scan(&v.ApplicationID, &v.ProfileID, &v.VersionNumber, &v.Content, &v.ContentType)
	if errors.Is(err, sql.ErrNoRows) {
		return AppConfigHostedVersion{}, ErrAppConfigNotFound
	}
	if err != nil {
		return AppConfigHostedVersion{}, fmt.Errorf("latest hosted version: %w", err)
	}
	return v, nil
}

func (s *Store) getAppConfigApplication(accountID, id string) (AppConfigApplication, error) {
	var a AppConfigApplication
	err := s.db.QueryRow(
		`SELECT application_id, name, description FROM appconfig_applications WHERE account_id = ? AND application_id = ?`,
		accountID, id,
	).Scan(&a.ID, &a.Name, &a.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return AppConfigApplication{}, ErrAppConfigNotFound
	}
	if err != nil {
		return AppConfigApplication{}, fmt.Errorf("get application: %w", err)
	}
	return a, nil
}

func (s *Store) getAppConfigEnvironment(accountID, applicationID, environmentID string) (AppConfigEnvironment, error) {
	var e AppConfigEnvironment
	err := s.db.QueryRow(
		`SELECT application_id, environment_id, name, description FROM appconfig_environments
		 WHERE account_id = ? AND application_id = ? AND environment_id = ?`,
		accountID, applicationID, environmentID,
	).Scan(&e.ApplicationID, &e.ID, &e.Name, &e.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return AppConfigEnvironment{}, ErrAppConfigNotFound
	}
	if err != nil {
		return AppConfigEnvironment{}, fmt.Errorf("get environment: %w", err)
	}
	return e, nil
}

func (s *Store) getAppConfigProfile(accountID, applicationID, profileID string) (AppConfigProfile, error) {
	var p AppConfigProfile
	err := s.db.QueryRow(
		`SELECT application_id, profile_id, name, location_uri, description FROM appconfig_profiles
		 WHERE account_id = ? AND application_id = ? AND profile_id = ?`,
		accountID, applicationID, profileID,
	).Scan(&p.ApplicationID, &p.ID, &p.Name, &p.LocationURI, &p.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return AppConfigProfile{}, ErrAppConfigNotFound
	}
	if err != nil {
		return AppConfigProfile{}, fmt.Errorf("get profile: %w", err)
	}
	return p, nil
}

func shortID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
}
