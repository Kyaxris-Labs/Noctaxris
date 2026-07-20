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
	ErrCodeDeployNotFound   = errors.New("ApplicationDoesNotExistException")
	ErrCodeDeployBadRequest = errors.New("InvalidParameterException")
	ErrCodeDeployExists     = errors.New("ApplicationAlreadyExistsException")
	ErrCodeDeployDGNotFound = errors.New("DeploymentGroupDoesNotExistException")
	ErrCodeDeployDGExists   = errors.New("DeploymentGroupAlreadyExistsException")
)

const codeDeploySchema = `
CREATE TABLE IF NOT EXISTS codedeploy_applications (
  account_id TEXT NOT NULL,
  application_name TEXT NOT NULL,
  application_id TEXT NOT NULL,
  compute_platform TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, application_name)
);
CREATE TABLE IF NOT EXISTS codedeploy_deployment_groups (
  account_id TEXT NOT NULL,
  application_name TEXT NOT NULL,
  deployment_group_name TEXT NOT NULL,
  deployment_group_id TEXT NOT NULL,
  service_role_arn TEXT NOT NULL DEFAULT '',
  ecs_service_name TEXT NOT NULL DEFAULT '',
  ecs_cluster_name TEXT NOT NULL DEFAULT '',
  lambda_function_name TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, application_name, deployment_group_name)
);
CREATE TABLE IF NOT EXISTS codedeploy_deployments (
  account_id TEXT NOT NULL,
  deployment_id TEXT NOT NULL,
  application_name TEXT NOT NULL,
  deployment_group_name TEXT NOT NULL,
  status TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, deployment_id)
);
CREATE INDEX IF NOT EXISTS idx_cd_deploy_app ON codedeploy_deployments(account_id, application_name);
`

// CodeDeployApplication is a lab CodeDeploy application.
type CodeDeployApplication struct {
	ApplicationID   string
	ApplicationName string
	ComputePlatform string
	CreatedAt       int64
}

// CodeDeployDeploymentGroup is a lab deployment group.
type CodeDeployDeploymentGroup struct {
	DeploymentGroupID   string
	ApplicationName     string
	DeploymentGroupName string
	ServiceRoleARN      string
	ECSServiceName      string
	ECSClusterName      string
	LambdaFunctionName  string
	CreatedAt           int64
}

// CodeDeployDeployment is a lab deployment record.
type CodeDeployDeployment struct {
	DeploymentID        string
	ApplicationName     string
	DeploymentGroupName string
	Status              string
	Description         string
	CreatedAt           int64
}

// EnsureCodeDeploySchema creates CodeDeploy tables if missing.
func EnsureCodeDeploySchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure codedeploy schema: db is nil")
	}
	if _, err := db.Exec(codeDeploySchema); err != nil {
		return fmt.Errorf("ensure codedeploy schema: %w", err)
	}
	return nil
}

// EnsureCodeDeploySchema ensures CodeDeploy tables on an open store.
func (s *Store) EnsureCodeDeploySchema() error {
	return EnsureCodeDeploySchema(s.db)
}

// CreateCodeDeployApplication creates an application.
func (s *Store) CreateCodeDeployApplication(accountID, name, platform string) (CodeDeployApplication, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CodeDeployApplication{}, fmt.Errorf("%w: applicationName required", ErrCodeDeployBadRequest)
	}
	if platform == "" {
		platform = "Server"
	}
	id := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO codedeploy_applications (account_id, application_name, application_id, compute_platform, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, name, id, platform, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return CodeDeployApplication{}, ErrCodeDeployExists
		}
		return CodeDeployApplication{}, fmt.Errorf("create codedeploy application: %w", err)
	}
	return CodeDeployApplication{ApplicationID: id, ApplicationName: name, ComputePlatform: platform, CreatedAt: now}, nil
}

// GetCodeDeployApplication returns an application by name.
func (s *Store) GetCodeDeployApplication(accountID, name string) (CodeDeployApplication, error) {
	var a CodeDeployApplication
	err := s.db.QueryRow(
		`SELECT application_id, application_name, compute_platform, created_at
		 FROM codedeploy_applications WHERE account_id = ? AND application_name = ?`,
		accountID, strings.TrimSpace(name),
	).Scan(&a.ApplicationID, &a.ApplicationName, &a.ComputePlatform, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeDeployApplication{}, ErrCodeDeployNotFound
	}
	if err != nil {
		return CodeDeployApplication{}, fmt.Errorf("get codedeploy application: %w", err)
	}
	return a, nil
}

// CreateCodeDeployDeploymentGroup creates a deployment group.
func (s *Store) CreateCodeDeployDeploymentGroup(
	accountID, appName, dgName, roleARN, ecsService, ecsCluster, lambdaFn string,
) (CodeDeployDeploymentGroup, error) {
	appName = strings.TrimSpace(appName)
	dgName = strings.TrimSpace(dgName)
	if appName == "" || dgName == "" {
		return CodeDeployDeploymentGroup{}, fmt.Errorf("%w: applicationName and deploymentGroupName required", ErrCodeDeployBadRequest)
	}
	if _, err := s.GetCodeDeployApplication(accountID, appName); err != nil {
		return CodeDeployDeploymentGroup{}, err
	}
	id := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO codedeploy_deployment_groups
		 (account_id, application_name, deployment_group_name, deployment_group_id, service_role_arn,
		  ecs_service_name, ecs_cluster_name, lambda_function_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, appName, dgName, id, strings.TrimSpace(roleARN),
		strings.TrimSpace(ecsService), strings.TrimSpace(ecsCluster), strings.TrimSpace(lambdaFn), now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return CodeDeployDeploymentGroup{}, ErrCodeDeployDGExists
		}
		return CodeDeployDeploymentGroup{}, fmt.Errorf("create deployment group: %w", err)
	}
	return CodeDeployDeploymentGroup{
		DeploymentGroupID: id, ApplicationName: appName, DeploymentGroupName: dgName,
		ServiceRoleARN: roleARN, ECSServiceName: ecsService, ECSClusterName: ecsCluster,
		LambdaFunctionName: lambdaFn, CreatedAt: now,
	}, nil
}

// GetCodeDeployDeploymentGroup returns a deployment group.
func (s *Store) GetCodeDeployDeploymentGroup(accountID, appName, dgName string) (CodeDeployDeploymentGroup, error) {
	var g CodeDeployDeploymentGroup
	err := s.db.QueryRow(
		`SELECT deployment_group_id, application_name, deployment_group_name, service_role_arn,
		        ecs_service_name, ecs_cluster_name, lambda_function_name, created_at
		 FROM codedeploy_deployment_groups
		 WHERE account_id = ? AND application_name = ? AND deployment_group_name = ?`,
		accountID, strings.TrimSpace(appName), strings.TrimSpace(dgName),
	).Scan(&g.DeploymentGroupID, &g.ApplicationName, &g.DeploymentGroupName, &g.ServiceRoleARN,
		&g.ECSServiceName, &g.ECSClusterName, &g.LambdaFunctionName, &g.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeDeployDeploymentGroup{}, ErrCodeDeployDGNotFound
	}
	if err != nil {
		return CodeDeployDeploymentGroup{}, fmt.Errorf("get deployment group: %w", err)
	}
	return g, nil
}

// CreateCodeDeployDeployment records a deployment as Succeeded (lab sync complete).
func (s *Store) CreateCodeDeployDeployment(accountID, appName, dgName, description string) (CodeDeployDeployment, error) {
	if _, err := s.GetCodeDeployDeploymentGroup(accountID, appName, dgName); err != nil {
		return CodeDeployDeployment{}, err
	}
	id := "d-" + uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO codedeploy_deployments
		 (account_id, deployment_id, application_name, deployment_group_name, status, description, created_at)
		 VALUES (?, ?, ?, ?, 'Succeeded', ?, ?)`,
		accountID, id, strings.TrimSpace(appName), strings.TrimSpace(dgName), strings.TrimSpace(description), now,
	)
	if err != nil {
		return CodeDeployDeployment{}, fmt.Errorf("create deployment: %w", err)
	}
	return CodeDeployDeployment{
		DeploymentID: id, ApplicationName: appName, DeploymentGroupName: dgName,
		Status: "Succeeded", Description: description, CreatedAt: now,
	}, nil
}

// GetCodeDeployDeployment returns a deployment by id.
func (s *Store) GetCodeDeployDeployment(accountID, deploymentID string) (CodeDeployDeployment, error) {
	var d CodeDeployDeployment
	err := s.db.QueryRow(
		`SELECT deployment_id, application_name, deployment_group_name, status, description, created_at
		 FROM codedeploy_deployments WHERE account_id = ? AND deployment_id = ?`,
		accountID, strings.TrimSpace(deploymentID),
	).Scan(&d.DeploymentID, &d.ApplicationName, &d.DeploymentGroupName, &d.Status, &d.Description, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeDeployDeployment{}, ErrCodeDeployNotFound
	}
	if err != nil {
		return CodeDeployDeployment{}, fmt.Errorf("get deployment: %w", err)
	}
	return d, nil
}

// ListCodeDeployDeployments lists deployments, optionally filtered by application.
func (s *Store) ListCodeDeployDeployments(accountID, appName string) ([]CodeDeployDeployment, error) {
	appName = strings.TrimSpace(appName)
	var rows *sql.Rows
	var err error
	if appName == "" {
		rows, err = s.db.Query(
			`SELECT deployment_id, application_name, deployment_group_name, status, description, created_at
			 FROM codedeploy_deployments WHERE account_id = ? ORDER BY created_at DESC`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT deployment_id, application_name, deployment_group_name, status, description, created_at
			 FROM codedeploy_deployments WHERE account_id = ? AND application_name = ? ORDER BY created_at DESC`,
			accountID, appName,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	defer rows.Close()
	var out []CodeDeployDeployment
	for rows.Next() {
		var d CodeDeployDeployment
		if err := rows.Scan(&d.DeploymentID, &d.ApplicationName, &d.DeploymentGroupName, &d.Status, &d.Description, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("list deployments scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
