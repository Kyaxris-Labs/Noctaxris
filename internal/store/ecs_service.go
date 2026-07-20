package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrECSServiceNotFound is returned when a service name is unknown.
	ErrECSServiceNotFound = errors.New("ServiceNotFoundException")
	// ErrECSServiceAlreadyExists is returned on CreateService name collision.
	ErrECSServiceAlreadyExists = errors.New("InvalidParameterException: service already exists")
	// ErrECSInvalidDesiredCount is returned for negative DesiredCount.
	ErrECSInvalidDesiredCount = errors.New("InvalidParameterException: desiredCount must be >= 0")
)

// ECSService is a lab ECS service row.
type ECSService struct {
	ServiceName    string
	ServiceARN     string
	ClusterARN     string
	ClusterName    string
	TaskDefinition string
	DesiredCount   int
	RunningCount   int
	PendingCount   int
	Status         string
	CreatedAt      string
	UpdatedAt      string
}

// CreateServiceInput holds CreateService fields.
type CreateServiceInput struct {
	Cluster        string
	ServiceName    string
	TaskDefinition string
	DesiredCount   int
}

// UpdateServiceInput holds UpdateService fields.
type UpdateServiceInput struct {
	Cluster        string
	Service        string
	TaskDefinition string
	DesiredCount   *int
}

const ecsServiceSchema = `
CREATE TABLE IF NOT EXISTS ecs_services (
  account_id TEXT NOT NULL,
  cluster_name TEXT NOT NULL,
  service_name TEXT NOT NULL,
  service_arn TEXT NOT NULL,
  task_def_arn TEXT NOT NULL,
  desired_count INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (account_id, cluster_name, service_name)
);
CREATE INDEX IF NOT EXISTS idx_ecs_services_cluster ON ecs_services(account_id, cluster_name);
`

// EnsureECSServiceSchema creates ecs_services and adds service_name on ecs_tasks.
func EnsureECSServiceSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure ecs service schema: db is nil")
	}
	if _, err := db.Exec(ecsServiceSchema); err != nil {
		return fmt.Errorf("ensure ecs service schema: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE ecs_tasks ADD COLUMN service_name TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("ensure ecs service schema: %w", err)
		}
	}
	return nil
}

// EnsureECSServiceSchema ensures service tables on an open store.
func (s *Store) EnsureECSServiceSchema() error {
	return EnsureECSServiceSchema(s.db)
}

// ServiceARN builds arn:aws:ecs:REGION:ACCOUNT:service/CLUSTER/NAME.
func ServiceARN(region, accountID, clusterName, serviceName string) string {
	if region == "" {
		region = DefaultECSRegion
	}
	return fmt.Sprintf("arn:aws:ecs:%s:%s:service/%s/%s", region, accountID, clusterName, serviceName)
}

// CreateService inserts an ACTIVE service.
func (s *Store) CreateService(accountID, region string, in CreateServiceInput) (ECSService, error) {
	clusterName := normalizeECSClusterName(in.Cluster)
	serviceName := strings.TrimSpace(in.ServiceName)
	if serviceName == "" {
		return ECSService{}, fmt.Errorf("create service: serviceName is required")
	}
	if in.DesiredCount < 0 {
		return ECSService{}, ErrECSInvalidDesiredCount
	}
	if err := s.ensureDefaultCluster(accountID, region); err != nil {
		return ECSService{}, err
	}
	clusterRow, err := s.getClusterRow(accountID, clusterName)
	if err != nil {
		return ECSService{}, err
	}
	td, err := s.resolveTaskDefinitionForService(accountID, in.TaskDefinition)
	if err != nil {
		return ECSService{}, err
	}
	if _, err := s.GetService(accountID, clusterName, serviceName); err == nil {
		return ECSService{}, ErrECSServiceAlreadyExists
	} else if !errors.Is(err, ErrECSServiceNotFound) {
		return ECSService{}, err
	}
	if region == "" {
		region = DefaultECSRegion
	}
	now := nowRFC3339()
	arn := ServiceARN(region, accountID, clusterName, serviceName)
	_, err = s.db.Exec(
		`INSERT INTO ecs_services
		 (account_id, cluster_name, service_name, service_arn, task_def_arn, desired_count, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE', ?, ?)`,
		accountID, clusterName, serviceName, arn, td.ARN, in.DesiredCount, now, now,
	)
	if err != nil {
		return ECSService{}, fmt.Errorf("create service: %w", err)
	}
	running, pending, err := s.countServiceTasks(accountID, clusterName, serviceName)
	if err != nil {
		return ECSService{}, err
	}
	return ECSService{
		ServiceName:    serviceName,
		ServiceARN:     arn,
		ClusterARN:     clusterRow.ARN,
		ClusterName:    clusterName,
		TaskDefinition: td.ARN,
		DesiredCount:   in.DesiredCount,
		RunningCount:   running,
		PendingCount:   pending,
		Status:         "ACTIVE",
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// GetService returns a service by cluster and name.
func (s *Store) GetService(accountID, cluster, serviceName string) (ECSService, error) {
	clusterName := normalizeECSClusterName(cluster)
	serviceName = strings.TrimSpace(serviceName)
	row := s.db.QueryRow(
		`SELECT account_id, cluster_name, service_name, service_arn, task_def_arn, desired_count, status, created_at, updated_at
		 FROM ecs_services WHERE account_id = ? AND cluster_name = ? AND service_name = ?`,
		accountID, clusterName, serviceName,
	)
	var (
		acct string
		svc  ECSService
	)
	err := row.Scan(&acct, &svc.ClusterName, &svc.ServiceName, &svc.ServiceARN, &svc.TaskDefinition,
		&svc.DesiredCount, &svc.Status, &svc.CreatedAt, &svc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ECSService{}, ErrECSServiceNotFound
	}
	if err != nil {
		return ECSService{}, fmt.Errorf("get service: %w", err)
	}
	clusterRow, err := s.getClusterRow(accountID, clusterName)
	if err != nil {
		return ECSService{}, err
	}
	svc.ClusterARN = clusterRow.ARN
	running, pending, err := s.countServiceTasks(accountID, clusterName, serviceName)
	if err != nil {
		return ECSService{}, err
	}
	svc.RunningCount = running
	svc.PendingCount = pending
	return svc, nil
}

// UpdateService updates DesiredCount and/or TaskDefinition.
func (s *Store) UpdateService(accountID, region string, in UpdateServiceInput) (ECSService, error) {
	clusterName := normalizeECSClusterName(in.Cluster)
	svc, err := s.GetService(accountID, clusterName, in.Service)
	if err != nil {
		return ECSService{}, err
	}
	taskDefARN := svc.TaskDefinition
	if strings.TrimSpace(in.TaskDefinition) != "" {
		td, err := s.resolveTaskDefinitionForService(accountID, in.TaskDefinition)
		if err != nil {
			return ECSService{}, err
		}
		taskDefARN = td.ARN
	}
	desired := svc.DesiredCount
	if in.DesiredCount != nil {
		if *in.DesiredCount < 0 {
			return ECSService{}, ErrECSInvalidDesiredCount
		}
		desired = *in.DesiredCount
	}
	now := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE ecs_services SET task_def_arn = ?, desired_count = ?, updated_at = ?
		 WHERE account_id = ? AND cluster_name = ? AND service_name = ?`,
		taskDefARN, desired, now, accountID, clusterName, svc.ServiceName,
	)
	if err != nil {
		return ECSService{}, fmt.Errorf("update service: %w", err)
	}
	return s.GetService(accountID, clusterName, svc.ServiceName)
}

// DeleteService marks a service INACTIVE and clears DesiredCount (tasks stopped by reconciler).
func (s *Store) DeleteService(accountID, cluster, serviceName string) (ECSService, error) {
	if _, err := s.GetService(accountID, cluster, serviceName); err != nil {
		return ECSService{}, err
	}
	now := nowRFC3339()
	_, err := s.db.Exec(
		`UPDATE ecs_services SET status = 'INACTIVE', desired_count = 0, updated_at = ?
		 WHERE account_id = ? AND cluster_name = ? AND service_name = ?`,
		now, accountID, normalizeECSClusterName(cluster), strings.TrimSpace(serviceName),
	)
	if err != nil {
		return ECSService{}, fmt.Errorf("delete service: %w", err)
	}
	return s.GetService(accountID, cluster, serviceName)
}

// DescribeServices returns services by name in a cluster. Missing names are omitted.
func (s *Store) DescribeServices(accountID, cluster string, serviceNames []string) ([]ECSService, error) {
	out := make([]ECSService, 0, len(serviceNames))
	for _, name := range serviceNames {
		svc, err := s.GetService(accountID, cluster, name)
		if errors.Is(err, ErrECSServiceNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, svc)
	}
	return out, nil
}

// ListServices returns ACTIVE service ARNs in a cluster.
func (s *Store) ListServices(accountID, cluster string) ([]string, error) {
	clusterName := normalizeECSClusterName(cluster)
	rows, err := s.db.Query(
		`SELECT service_arn FROM ecs_services
		 WHERE account_id = ? AND cluster_name = ? AND status = 'ACTIVE'
		 ORDER BY service_name`,
		accountID, clusterName,
	)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var arn string
		if err := rows.Scan(&arn); err != nil {
			return nil, fmt.Errorf("list services: %w", err)
		}
		out = append(out, arn)
	}
	return out, rows.Err()
}

// ListActiveServicesWithAccount returns ACTIVE services with owning account IDs.
func (s *Store) ListActiveServicesWithAccount() ([]ECSServiceAccount, error) {
	rows, err := s.db.Query(
		`SELECT account_id, cluster_name, service_name, service_arn, task_def_arn, desired_count, status, created_at, updated_at
		 FROM ecs_services WHERE status = 'ACTIVE' ORDER BY account_id, cluster_name, service_name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list active services: %w", err)
	}
	defer rows.Close()
	out := []ECSServiceAccount{}
	for rows.Next() {
		var (
			acct string
			svc  ECSService
		)
		if err := rows.Scan(&acct, &svc.ClusterName, &svc.ServiceName, &svc.ServiceARN, &svc.TaskDefinition,
			&svc.DesiredCount, &svc.Status, &svc.CreatedAt, &svc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list active services: %w", err)
		}
		clusterRow, err := s.getClusterRow(acct, svc.ClusterName)
		if err != nil {
			return nil, err
		}
		svc.ClusterARN = clusterRow.ARN
		running, pending, err := s.countServiceTasks(acct, svc.ClusterName, svc.ServiceName)
		if err != nil {
			return nil, err
		}
		svc.RunningCount = running
		svc.PendingCount = pending
		out = append(out, ECSServiceAccount{AccountID: acct, Service: svc})
	}
	return out, rows.Err()
}

// ECSServiceAccount carries account + service for the reconciler.
type ECSServiceAccount struct {
	AccountID string
	Service   ECSService
}

// ListServiceTaskARNs returns RUNNING task ARNs owned by a service (oldest first).
func (s *Store) ListServiceTaskARNs(accountID, cluster, serviceName, status string) ([]string, error) {
	clusterName := normalizeECSClusterName(cluster)
	serviceName = strings.TrimSpace(serviceName)
	rows, err := s.db.Query(
		`SELECT task_arn FROM ecs_tasks
		 WHERE account_id = ? AND cluster_name = ? AND service_name = ? AND last_status = ?
		 ORDER BY created_at, task_arn`,
		accountID, clusterName, serviceName, status,
	)
	if err != nil {
		return nil, fmt.Errorf("list service tasks: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var arn string
		if err := rows.Scan(&arn); err != nil {
			return nil, fmt.Errorf("list service tasks: %w", err)
		}
		out = append(out, arn)
	}
	return out, rows.Err()
}

// RunServiceTask creates a RUNNING task owned by a service (store row only).
func (s *Store) RunServiceTask(accountID, region, cluster, serviceName, taskDefARN string) (ECSTask, error) {
	task, err := s.RunTask(accountID, region, RunTaskInput{
		Cluster:        cluster,
		TaskDefinition: taskDefARN,
	})
	if err != nil {
		return ECSTask{}, err
	}
	_, err = s.db.Exec(
		`UPDATE ecs_tasks SET service_name = ? WHERE account_id = ? AND task_arn = ?`,
		strings.TrimSpace(serviceName), accountID, task.TaskARN,
	)
	if err != nil {
		_ = s.DeleteTask(accountID, task.TaskARN)
		return ECSTask{}, fmt.Errorf("run service task: %w", err)
	}
	return task, nil
}

func (s *Store) resolveTaskDefinitionForService(accountID, taskDef string) (ECSTaskDefinition, error) {
	taskDef = strings.TrimSpace(taskDef)
	if taskDef == "" {
		return ECSTaskDefinition{}, ErrECSTaskDefinitionNotFound
	}
	if strings.Contains(taskDef, "task-definition/") {
		return s.DescribeTaskDefinitionByARN(accountID, taskDef)
	}
	arns, err := s.ListTaskDefinitions(accountID, taskDef)
	if err != nil {
		return ECSTaskDefinition{}, err
	}
	if len(arns) == 0 {
		return ECSTaskDefinition{}, ErrECSTaskDefinitionNotFound
	}
	return s.DescribeTaskDefinitionByARN(accountID, arns[len(arns)-1])
}

func (s *Store) countServiceTasks(accountID, clusterName, serviceName string) (running, pending int, err error) {
	err = s.db.QueryRow(
		`SELECT
		   COALESCE(SUM(CASE WHEN last_status = ? THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN last_status = ? THEN 1 ELSE 0 END), 0)
		 FROM ecs_tasks
		 WHERE account_id = ? AND cluster_name = ? AND service_name = ?`,
		ECSTaskStatusRunning, ECSTaskStatusPending, accountID, clusterName, serviceName,
	).Scan(&running, &pending)
	if err != nil {
		return 0, 0, fmt.Errorf("count service tasks: %w", err)
	}
	return running, pending, nil
}
