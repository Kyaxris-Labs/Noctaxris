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
	ErrBeanstalkNotFound   = errors.New("InvalidParameterValue")
	ErrBeanstalkExists     = errors.New("InvalidParameterValue")
	ErrBeanstalkBadRequest = errors.New("ValidationError")
)

const DefaultBeanstalkRegion = "us-east-1"

const DefaultBeanstalkSolutionStack = "64bit Amazon Linux 2023 v4.3.0 running Docker"

const beanstalkSchema = `
CREATE TABLE IF NOT EXISTS beanstalk_applications (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, name)
);
CREATE TABLE IF NOT EXISTS beanstalk_versions (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  application_name TEXT NOT NULL,
  version_label TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  s3_bucket TEXT NOT NULL DEFAULT '',
  s3_key TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'Processed',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, application_name, version_label)
);
CREATE TABLE IF NOT EXISTS beanstalk_environments (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  environment_name TEXT NOT NULL,
  arn TEXT NOT NULL,
  application_name TEXT NOT NULL,
  version_label TEXT NOT NULL DEFAULT '',
  solution_stack_name TEXT NOT NULL,
  cname TEXT NOT NULL,
  status TEXT NOT NULL,
  health TEXT NOT NULL DEFAULT 'Green',
  health_status TEXT NOT NULL DEFAULT 'Ok',
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, environment_id),
  UNIQUE (account_id, region, environment_name)
);
`

// BeanstalkApplication is an Elastic Beanstalk application row.
type BeanstalkApplication struct {
	ApplicationName string
	ARN             string
	Description     string
	CreatedAt       int64
	UpdatedAt       int64
	Region          string
}

// BeanstalkApplicationVersion is an application version row.
type BeanstalkApplicationVersion struct {
	ApplicationName string
	VersionLabel    string
	Description     string
	S3Bucket        string
	S3Key           string
	Status          string
	CreatedAt       int64
	Region          string
}

// BeanstalkEnvironment is an environment row (Ready/Green immediately).
type BeanstalkEnvironment struct {
	EnvironmentID     string
	EnvironmentName   string
	ARN               string
	ApplicationName   string
	VersionLabel      string
	SolutionStackName string
	CNAME             string
	Status            string
	Health            string
	HealthStatus      string
	Description       string
	CreatedAt         int64
	UpdatedAt         int64
	Region            string
}

// EnsureBeanstalkSchema creates Elastic Beanstalk tables if missing.
func EnsureBeanstalkSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure beanstalk schema: db is nil")
	}
	if _, err := db.Exec(beanstalkSchema); err != nil {
		return fmt.Errorf("ensure beanstalk schema: %w", err)
	}
	return nil
}

// EnsureBeanstalkSchema ensures Beanstalk tables on an open store.
func (s *Store) EnsureBeanstalkSchema() error {
	return EnsureBeanstalkSchema(s.db)
}

// BeanstalkSolutionStacks returns the static lab solution stack list.
func BeanstalkSolutionStacks() []string {
	return []string{
		DefaultBeanstalkSolutionStack,
		"64bit Amazon Linux 2023 v4.1.0 running Python 3.11",
		"64bit Amazon Linux 2023 v4.1.0 running Node.js 20",
	}
}

func BeanstalkApplicationARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultBeanstalkRegion
	}
	return fmt.Sprintf("arn:aws:elasticbeanstalk:%s:%s:application/%s", region, accountID, name)
}

func BeanstalkEnvironmentARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultBeanstalkRegion
	}
	return fmt.Sprintf("arn:aws:elasticbeanstalk:%s:%s:environment/%s", region, accountID, name)
}

// CreateBeanstalkApplication creates an application.
func (s *Store) CreateBeanstalkApplication(accountID, region, name, description string) (BeanstalkApplication, error) {
	if region == "" {
		region = DefaultBeanstalkRegion
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return BeanstalkApplication{}, fmt.Errorf("%w: ApplicationName required", ErrBeanstalkBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	arn := BeanstalkApplicationARN(region, accountID, name)
	_, err := s.db.Exec(
		`INSERT INTO beanstalk_applications (account_id, region, name, arn, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, name, arn, description, now, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return BeanstalkApplication{}, ErrBeanstalkExists
		}
		return BeanstalkApplication{}, fmt.Errorf("create beanstalk application: %w", err)
	}
	return BeanstalkApplication{ApplicationName: name, ARN: arn, Description: description, CreatedAt: now, UpdatedAt: now, Region: region}, nil
}

// DescribeBeanstalkApplications lists applications, optionally filtered by names.
func (s *Store) DescribeBeanstalkApplications(accountID, region string, names []string) ([]BeanstalkApplication, error) {
	if region == "" {
		region = DefaultBeanstalkRegion
	}
	rows, err := s.db.Query(
		`SELECT name, arn, description, created_at, updated_at, region FROM beanstalk_applications
		 WHERE account_id = ? AND region = ? ORDER BY created_at, name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("describe beanstalk applications: %w", err)
	}
	defer rows.Close()
	want := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			want[n] = struct{}{}
		}
	}
	var out []BeanstalkApplication
	for rows.Next() {
		var a BeanstalkApplication
		if err := rows.Scan(&a.ApplicationName, &a.ARN, &a.Description, &a.CreatedAt, &a.UpdatedAt, &a.Region); err != nil {
			return nil, fmt.Errorf("describe beanstalk applications scan: %w", err)
		}
		if len(want) > 0 {
			if _, ok := want[a.ApplicationName]; !ok {
				continue
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteBeanstalkApplication deletes an application and its versions/environments when force is set.
func (s *Store) DeleteBeanstalkApplication(accountID, region, name string, terminateEnvByForce bool) error {
	if region == "" {
		region = DefaultBeanstalkRegion
	}
	name = strings.TrimSpace(name)
	envs, err := s.DescribeBeanstalkEnvironments(accountID, region, name, nil, false)
	if err != nil {
		return err
	}
	active := 0
	for _, e := range envs {
		if e.Status != "Terminated" {
			active++
		}
	}
	if active > 0 && !terminateEnvByForce {
		return fmt.Errorf("%w: application has active environments", ErrBeanstalkBadRequest)
	}
	if terminateEnvByForce {
		for _, e := range envs {
			if e.Status != "Terminated" {
				_, _ = s.TerminateBeanstalkEnvironment(accountID, region, e.EnvironmentName, e.EnvironmentID)
			}
		}
	}
	_, _ = s.db.Exec(`DELETE FROM beanstalk_versions WHERE account_id = ? AND region = ? AND application_name = ?`, accountID, region, name)
	res, err := s.db.Exec(`DELETE FROM beanstalk_applications WHERE account_id = ? AND region = ? AND name = ?`, accountID, region, name)
	if err != nil {
		return fmt.Errorf("delete beanstalk application: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete beanstalk application: %w", err)
	}
	if n == 0 {
		return ErrBeanstalkNotFound
	}
	return nil
}

// CreateBeanstalkApplicationVersion stores version metadata.
func (s *Store) CreateBeanstalkApplicationVersion(accountID, region, appName, label, description, s3Bucket, s3Key string) (BeanstalkApplicationVersion, error) {
	if region == "" {
		region = DefaultBeanstalkRegion
	}
	appName = strings.TrimSpace(appName)
	label = strings.TrimSpace(label)
	if appName == "" || label == "" {
		return BeanstalkApplicationVersion{}, fmt.Errorf("%w: ApplicationName and VersionLabel required", ErrBeanstalkBadRequest)
	}
	apps, err := s.DescribeBeanstalkApplications(accountID, region, []string{appName})
	if err != nil {
		return BeanstalkApplicationVersion{}, err
	}
	if len(apps) == 0 {
		return BeanstalkApplicationVersion{}, fmt.Errorf("%w: application not found", ErrBeanstalkNotFound)
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO beanstalk_versions
		 (account_id, region, application_name, version_label, description, s3_bucket, s3_key, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'Processed', ?)`,
		accountID, region, appName, label, description, s3Bucket, s3Key, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return BeanstalkApplicationVersion{}, ErrBeanstalkExists
		}
		return BeanstalkApplicationVersion{}, fmt.Errorf("create beanstalk version: %w", err)
	}
	return BeanstalkApplicationVersion{
		ApplicationName: appName, VersionLabel: label, Description: description,
		S3Bucket: s3Bucket, S3Key: s3Key, Status: "Processed", CreatedAt: now, Region: region,
	}, nil
}

// CreateBeanstalkEnvironment creates an environment in Ready/Green state.
func (s *Store) CreateBeanstalkEnvironment(accountID, region, appName, envName, versionLabel, solutionStack, description, cnamePrefix string) (BeanstalkEnvironment, error) {
	if region == "" {
		region = DefaultBeanstalkRegion
	}
	appName = strings.TrimSpace(appName)
	envName = strings.TrimSpace(envName)
	if appName == "" || envName == "" {
		return BeanstalkEnvironment{}, fmt.Errorf("%w: ApplicationName and EnvironmentName required", ErrBeanstalkBadRequest)
	}
	apps, err := s.DescribeBeanstalkApplications(accountID, region, []string{appName})
	if err != nil {
		return BeanstalkEnvironment{}, err
	}
	if len(apps) == 0 {
		return BeanstalkEnvironment{}, fmt.Errorf("%w: application not found", ErrBeanstalkNotFound)
	}
	if solutionStack == "" {
		solutionStack = DefaultBeanstalkSolutionStack
	}
	if cnamePrefix == "" {
		cnamePrefix = envName
	}
	now := time.Now().UTC().UnixMilli()
	envID := "e-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	cname := cnamePrefix + ".elasticbeanstalk.local"
	arn := BeanstalkEnvironmentARN(region, accountID, envName)
	_, err = s.db.Exec(
		`INSERT INTO beanstalk_environments
		 (account_id, region, environment_id, environment_name, arn, application_name, version_label,
		  solution_stack_name, cname, status, health, health_status, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'Ready', 'Green', 'Ok', ?, ?, ?)`,
		accountID, region, envID, envName, arn, appName, versionLabel, solutionStack, cname, description, now, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return BeanstalkEnvironment{}, ErrBeanstalkExists
		}
		return BeanstalkEnvironment{}, fmt.Errorf("create beanstalk environment: %w", err)
	}
	return BeanstalkEnvironment{
		EnvironmentID: envID, EnvironmentName: envName, ARN: arn, ApplicationName: appName,
		VersionLabel: versionLabel, SolutionStackName: solutionStack, CNAME: cname,
		Status: "Ready", Health: "Green", HealthStatus: "Ok", Description: description,
		CreatedAt: now, UpdatedAt: now, Region: region,
	}, nil
}

// DescribeBeanstalkEnvironments lists environments.
func (s *Store) DescribeBeanstalkEnvironments(accountID, region, appName string, envNames []string, includeDeleted bool) ([]BeanstalkEnvironment, error) {
	if region == "" {
		region = DefaultBeanstalkRegion
	}
	rows, err := s.db.Query(
		`SELECT environment_id, environment_name, arn, application_name, version_label, solution_stack_name,
		        cname, status, health, health_status, description, created_at, updated_at, region
		 FROM beanstalk_environments WHERE account_id = ? AND region = ? ORDER BY created_at, environment_name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("describe beanstalk environments: %w", err)
	}
	defer rows.Close()
	want := map[string]struct{}{}
	for _, n := range envNames {
		n = strings.TrimSpace(n)
		if n != "" {
			want[n] = struct{}{}
		}
	}
	appName = strings.TrimSpace(appName)
	var out []BeanstalkEnvironment
	for rows.Next() {
		var e BeanstalkEnvironment
		if err := rows.Scan(&e.EnvironmentID, &e.EnvironmentName, &e.ARN, &e.ApplicationName, &e.VersionLabel,
			&e.SolutionStackName, &e.CNAME, &e.Status, &e.Health, &e.HealthStatus, &e.Description,
			&e.CreatedAt, &e.UpdatedAt, &e.Region); err != nil {
			return nil, fmt.Errorf("describe beanstalk environments scan: %w", err)
		}
		if appName != "" && e.ApplicationName != appName {
			continue
		}
		if len(want) > 0 {
			if _, ok := want[e.EnvironmentName]; !ok {
				continue
			}
		}
		if !includeDeleted && e.Status == "Terminated" {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// TerminateBeanstalkEnvironment marks an environment Terminated.
func (s *Store) TerminateBeanstalkEnvironment(accountID, region, envName, envID string) (BeanstalkEnvironment, error) {
	if region == "" {
		region = DefaultBeanstalkRegion
	}
	envs, err := s.DescribeBeanstalkEnvironments(accountID, region, "", nil, true)
	if err != nil {
		return BeanstalkEnvironment{}, err
	}
	var found *BeanstalkEnvironment
	for i := range envs {
		if (envID != "" && envs[i].EnvironmentID == envID) || (envName != "" && envs[i].EnvironmentName == envName) {
			found = &envs[i]
			break
		}
	}
	if found == nil {
		return BeanstalkEnvironment{}, ErrBeanstalkNotFound
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`UPDATE beanstalk_environments SET status = 'Terminated', health = 'Grey', health_status = 'Suspended', updated_at = ?
		 WHERE account_id = ? AND region = ? AND environment_id = ?`,
		now, accountID, region, found.EnvironmentID,
	)
	if err != nil {
		return BeanstalkEnvironment{}, fmt.Errorf("terminate beanstalk environment: %w", err)
	}
	found.Status = "Terminated"
	found.Health = "Grey"
	found.HealthStatus = "Suspended"
	found.UpdatedAt = now
	return *found, nil
}
