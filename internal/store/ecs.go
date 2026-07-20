package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	// DefaultECSRegion is the lab region embedded in ECS ARNs.
	DefaultECSRegion = "us-east-1"

	// DefaultECSClusterName is the per-account default ECS cluster.
	DefaultECSClusterName = "default"

	// ECSTaskDefinitionActive is an active task definition revision.
	ECSTaskDefinitionActive = "ACTIVE"

	// ECSTaskDefinitionInactive is a deregistered task definition revision.
	ECSTaskDefinitionInactive = "INACTIVE"

	// ECSTaskStatusPending is the initial task status before transition.
	ECSTaskStatusPending = "PENDING"

	// ECSTaskStatusRunning is the lab running task status.
	ECSTaskStatusRunning = "RUNNING"

	// ECSTaskStatusStopped is the terminal stopped task status.
	ECSTaskStatusStopped = "STOPPED"
)

var (
	ErrECSMissingTaskRoleARN      = errors.New("ClientException: taskRoleArn is required")
	ErrECSMissingExecutionRoleARN = errors.New("ClientException: executionRoleArn is required")
	ErrECSTaskDefinitionNotFound  = errors.New("ClientException: task definition not found")
	ErrECSClusterNotFound         = errors.New("ClusterNotFoundException")
	ErrECSTaskNotFound            = errors.New("ClientException: task not found")
)

// ECSCluster is cluster metadata for an account.
type ECSCluster struct {
	ClusterName string
	ClusterARN  string
	Status      string
	CreatedAt   string
}

// ECSTaskDefinition is a registered task definition revision.
type ECSTaskDefinition struct {
	Family           string
	Revision         int
	ARN              string
	ContainerDefs    []map[string]any
	TaskRoleARN      string
	ExecutionRoleARN string
	Status           string
	RegisteredAt     string
}

// RegisterTaskDefinitionInput holds RegisterTaskDefinition fields.
// AWS allows omitting taskRoleArn; the Noctaxris lab requires both roles.
type RegisterTaskDefinitionInput struct {
	Family           string
	ContainerDefs    []map[string]any
	TaskRoleARN      string
	ExecutionRoleARN string
}

// ECSTask is a task row in a cluster.
type ECSTask struct {
	TaskARN       string
	ClusterARN    string
	TaskDefARN    string
	LastStatus    string
	Containers    []map[string]any
	StartedAt     string
	StoppedAt     string
	CreatedAt     string
}

// RunTaskInput holds RunTask fields.
type RunTaskInput struct {
	Cluster        string
	TaskDefinition string
}

// StopTaskInput holds StopTask fields.
type StopTaskInput struct {
	Cluster string
	Task    string
}

const ecsSchema = `
CREATE TABLE IF NOT EXISTS ecs_clusters (
  account_id TEXT NOT NULL,
  cluster_name TEXT NOT NULL,
  cluster_arn TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  created_at TEXT NOT NULL,
  PRIMARY KEY (account_id, cluster_name)
);
CREATE TABLE IF NOT EXISTS ecs_task_definitions (
  account_id TEXT NOT NULL,
  family TEXT NOT NULL,
  revision INTEGER NOT NULL,
  task_def_arn TEXT NOT NULL,
  container_defs_json TEXT NOT NULL,
  task_role_arn TEXT NOT NULL,
  execution_role_arn TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  registered_at TEXT NOT NULL,
  PRIMARY KEY (account_id, family, revision)
);
CREATE TABLE IF NOT EXISTS ecs_tasks (
  task_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  cluster_name TEXT NOT NULL,
  task_arn TEXT NOT NULL,
  task_def_arn TEXT NOT NULL,
  last_status TEXT NOT NULL,
  containers_json TEXT NOT NULL DEFAULT '[]',
  started_at TEXT NOT NULL DEFAULT '',
  stopped_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  PRIMARY KEY (account_id, task_id)
);
CREATE INDEX IF NOT EXISTS idx_ecs_tasks_cluster ON ecs_tasks(account_id, cluster_name);
`

// EnsureECSSchema creates ECS tables if missing.
func EnsureECSSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure ecs schema: db is nil")
	}
	if _, err := db.Exec(ecsSchema); err != nil {
		return fmt.Errorf("ensure ecs schema: %w", err)
	}
	return nil
}

// EnsureECSSchema ensures ECS tables on an open store (tests and Open wiring).
func (s *Store) EnsureECSSchema() error {
	return EnsureECSSchema(s.db)
}

// ClusterARN builds arn:aws:ecs:REGION:ACCOUNT:cluster/NAME.
func ClusterARN(region, accountID, clusterName string) string {
	if region == "" {
		region = DefaultECSRegion
	}
	return fmt.Sprintf("arn:aws:ecs:%s:%s:cluster/%s", region, accountID, clusterName)
}

// TaskDefinitionARN builds arn:aws:ecs:REGION:ACCOUNT:task-definition/FAMILY:REVISION.
func TaskDefinitionARN(region, accountID, family string, revision int) string {
	if region == "" {
		region = DefaultECSRegion
	}
	return fmt.Sprintf("arn:aws:ecs:%s:%s:task-definition/%s:%d", region, accountID, family, revision)
}

// TaskARN builds arn:aws:ecs:REGION:ACCOUNT:task/CLUSTER/TASK_ID.
func TaskARN(region, accountID, clusterName, taskID string) string {
	if region == "" {
		region = DefaultECSRegion
	}
	return fmt.Sprintf("arn:aws:ecs:%s:%s:task/%s/%s", region, accountID, clusterName, taskID)
}

func normalizeECSClusterName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultECSClusterName
	}
	return name
}

// NormalizeECSClusterNamePublic exposes cluster name normalization for handlers.
func NormalizeECSClusterNamePublic(name string) string {
	return normalizeECSClusterName(name)
}

func (s *Store) ensureDefaultCluster(accountID, region string) error {
	if region == "" {
		region = DefaultECSRegion
	}
	_, err := s.getClusterRow(accountID, DefaultECSClusterName)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrECSClusterNotFound) {
		return err
	}
	created := nowRFC3339()
	arn := ClusterARN(region, accountID, DefaultECSClusterName)
	_, err = s.db.Exec(
		`INSERT INTO ecs_clusters (account_id, cluster_name, cluster_arn, status, created_at)
		 VALUES (?, ?, ?, 'ACTIVE', ?)`,
		accountID, DefaultECSClusterName, arn, created,
	)
	if err != nil {
		return fmt.Errorf("ensure default cluster: %w", err)
	}
	return nil
}

type ecsClusterRow struct {
	Name      string
	ARN       string
	Status    string
	CreatedAt string
}

func (s *Store) getClusterRow(accountID, clusterName string) (ecsClusterRow, error) {
	clusterName = normalizeECSClusterName(clusterName)
	var row ecsClusterRow
	err := s.db.QueryRow(
		`SELECT cluster_name, cluster_arn, status, created_at
		 FROM ecs_clusters WHERE account_id = ? AND cluster_name = ?`,
		accountID, clusterName,
	).Scan(&row.Name, &row.ARN, &row.Status, &row.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ecsClusterRow{}, ErrECSClusterNotFound
		}
		return ecsClusterRow{}, fmt.Errorf("get cluster row %s: %w", clusterName, err)
	}
	return row, nil
}

func clusterFromRow(row ecsClusterRow) ECSCluster {
	return ECSCluster{
		ClusterName: row.Name,
		ClusterARN:  row.ARN,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
	}
}

// DescribeClusters returns cluster metadata, seeding default when needed.
func (s *Store) DescribeClusters(accountID, region string, names []string) ([]ECSCluster, error) {
	if len(names) == 0 {
		if err := s.ensureDefaultCluster(accountID, region); err != nil {
			return nil, err
		}
		return s.ListClusters(accountID, region)
	}
	out := make([]ECSCluster, 0, len(names))
	for _, name := range names {
		if normalizeECSClusterName(name) == DefaultECSClusterName {
			if err := s.ensureDefaultCluster(accountID, region); err != nil {
				return nil, err
			}
		}
		row, err := s.getClusterRow(accountID, name)
		if errors.Is(err, ErrECSClusterNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, clusterFromRow(row))
	}
	return out, nil
}

// ListClusters returns all clusters for an account, ensuring default exists.
func (s *Store) ListClusters(accountID, region string) ([]ECSCluster, error) {
	if err := s.ensureDefaultCluster(accountID, region); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT cluster_name, cluster_arn, status, created_at
		 FROM ecs_clusters WHERE account_id = ? ORDER BY cluster_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list clusters %s: %w", accountID, err)
	}
	defer rows.Close()
	var out []ECSCluster
	for rows.Next() {
		var row ecsClusterRow
		if err := rows.Scan(&row.Name, &row.ARN, &row.Status, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		out = append(out, clusterFromRow(row))
	}
	return out, rows.Err()
}

func parseTaskDefinitionARN(arn string) (family string, revision int, err error) {
	const prefix = "task-definition/"
	i := strings.LastIndex(arn, prefix)
	if i < 0 {
		return "", 0, fmt.Errorf("parse task definition arn: invalid arn %q", arn)
	}
	rest := arn[i+len(prefix):]
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("parse task definition arn: invalid arn %q", arn)
	}
	family = parts[0]
	if _, err := fmt.Sscanf(parts[1], "%d", &revision); err != nil {
		return "", 0, fmt.Errorf("parse task definition arn: invalid revision in %q", arn)
	}
	return family, revision, nil
}

func (s *Store) nextTaskDefinitionRevision(accountID, family string) (int, error) {
	var maxRev sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(revision) FROM ecs_task_definitions WHERE account_id = ? AND family = ?`,
		accountID, family,
	).Scan(&maxRev)
	if err != nil {
		return 0, fmt.Errorf("next task definition revision %s: %w", family, err)
	}
	if !maxRev.Valid {
		return 1, nil
	}
	return int(maxRev.Int64) + 1, nil
}

type ecsTaskDefinitionRow struct {
	Family           string
	Revision         int
	ARN              string
	ContainerDefs    string
	TaskRoleARN      string
	ExecutionRoleARN string
	Status           string
	RegisteredAt     string
}

func (s *Store) getTaskDefinitionRow(accountID, family string, revision int) (ecsTaskDefinitionRow, error) {
	var row ecsTaskDefinitionRow
	err := s.db.QueryRow(
		`SELECT family, revision, task_def_arn, container_defs_json, task_role_arn, execution_role_arn, status, registered_at
		 FROM ecs_task_definitions WHERE account_id = ? AND family = ? AND revision = ?`,
		accountID, family, revision,
	).Scan(&row.Family, &row.Revision, &row.ARN, &row.ContainerDefs, &row.TaskRoleARN, &row.ExecutionRoleARN, &row.Status, &row.RegisteredAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ecsTaskDefinitionRow{}, ErrECSTaskDefinitionNotFound
		}
		return ecsTaskDefinitionRow{}, fmt.Errorf("get task definition %s:%d: %w", family, revision, err)
	}
	return row, nil
}

func taskDefinitionFromRow(row ecsTaskDefinitionRow) (ECSTaskDefinition, error) {
	var defs []map[string]any
	if err := json.Unmarshal([]byte(row.ContainerDefs), &defs); err != nil {
		return ECSTaskDefinition{}, fmt.Errorf("unmarshal container defs: %w", err)
	}
	return ECSTaskDefinition{
		Family:           row.Family,
		Revision:         row.Revision,
		ARN:              row.ARN,
		ContainerDefs:    defs,
		TaskRoleARN:      row.TaskRoleARN,
		ExecutionRoleARN: row.ExecutionRoleARN,
		Status:           row.Status,
		RegisteredAt:     row.RegisteredAt,
	}, nil
}

// RegisterTaskDefinition stores a new task definition revision.
// Lab bar: both taskRoleArn and executionRoleArn are required (AWS allows omitting task role).
func (s *Store) RegisterTaskDefinition(accountID, region string, in RegisterTaskDefinitionInput) (ECSTaskDefinition, error) {
	family := strings.TrimSpace(in.Family)
	if family == "" {
		return ECSTaskDefinition{}, fmt.Errorf("register task definition: family is required")
	}
	if strings.TrimSpace(in.TaskRoleARN) == "" {
		return ECSTaskDefinition{}, ErrECSMissingTaskRoleARN
	}
	if strings.TrimSpace(in.ExecutionRoleARN) == "" {
		return ECSTaskDefinition{}, ErrECSMissingExecutionRoleARN
	}
	if region == "" {
		region = DefaultECSRegion
	}
	if in.ContainerDefs == nil {
		in.ContainerDefs = []map[string]any{}
	}
	containerJSON, err := json.Marshal(in.ContainerDefs)
	if err != nil {
		return ECSTaskDefinition{}, fmt.Errorf("register task definition: marshal containers: %w", err)
	}

	revision, err := s.nextTaskDefinitionRevision(accountID, family)
	if err != nil {
		return ECSTaskDefinition{}, err
	}
	arn := TaskDefinitionARN(region, accountID, family, revision)
	registered := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO ecs_task_definitions
		 (account_id, family, revision, task_def_arn, container_defs_json, task_role_arn, execution_role_arn, status, registered_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, family, revision, arn, string(containerJSON), strings.TrimSpace(in.TaskRoleARN), strings.TrimSpace(in.ExecutionRoleARN), ECSTaskDefinitionActive, registered,
	)
	if err != nil {
		return ECSTaskDefinition{}, fmt.Errorf("register task definition %s: %w", family, err)
	}
	return ECSTaskDefinition{
		Family:           family,
		Revision:         revision,
		ARN:              arn,
		ContainerDefs:    in.ContainerDefs,
		TaskRoleARN:      strings.TrimSpace(in.TaskRoleARN),
		ExecutionRoleARN: strings.TrimSpace(in.ExecutionRoleARN),
		Status:           ECSTaskDefinitionActive,
		RegisteredAt:     registered,
	}, nil
}

// DescribeTaskDefinition returns a task definition by family and revision.
func (s *Store) DescribeTaskDefinition(accountID, family string, revision int) (ECSTaskDefinition, error) {
	row, err := s.getTaskDefinitionRow(accountID, family, revision)
	if err != nil {
		return ECSTaskDefinition{}, err
	}
	return taskDefinitionFromRow(row)
}

// DescribeTaskDefinitionByARN returns a task definition by ARN.
func (s *Store) DescribeTaskDefinitionByARN(accountID, arn string) (ECSTaskDefinition, error) {
	family, revision, err := parseTaskDefinitionARN(arn)
	if err != nil {
		return ECSTaskDefinition{}, err
	}
	return s.DescribeTaskDefinition(accountID, family, revision)
}

// ListTaskDefinitions returns task definition ARNs for a family, or all families when family is empty.
func (s *Store) ListTaskDefinitions(accountID, family string) ([]string, error) {
	family = strings.TrimSpace(family)
	var (
		rows *sql.Rows
		err  error
	)
	if family == "" {
		rows, err = s.db.Query(
			`SELECT task_def_arn FROM ecs_task_definitions WHERE account_id = ? ORDER BY family, revision`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT task_def_arn FROM ecs_task_definitions WHERE account_id = ? AND family = ? ORDER BY revision`,
			accountID, family,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list task definitions %s: %w", accountID, err)
	}
	defer rows.Close()
	var arns []string
	for rows.Next() {
		var arn string
		if err := rows.Scan(&arn); err != nil {
			return nil, fmt.Errorf("scan task definition arn: %w", err)
		}
		arns = append(arns, arn)
	}
	return arns, rows.Err()
}

// DeregisterTaskDefinition marks a task definition revision INACTIVE.
func (s *Store) DeregisterTaskDefinition(accountID, taskDefARN string) error {
	family, revision, err := parseTaskDefinitionARN(taskDefARN)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE ecs_task_definitions SET status = ? WHERE account_id = ? AND family = ? AND revision = ?`,
		ECSTaskDefinitionInactive, accountID, family, revision,
	)
	if err != nil {
		return fmt.Errorf("deregister task definition %s: %w", taskDefARN, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("deregister task definition rows affected: %w", err)
	}
	if n == 0 {
		return ErrECSTaskDefinitionNotFound
	}
	return nil
}

type ecsTaskRow struct {
	TaskID        string
	ClusterName   string
	TaskARN       string
	TaskDefARN    string
	LastStatus    string
	Containers    string
	StartedAt     string
	StoppedAt     string
	CreatedAt     string
}

func (s *Store) getTaskRow(accountID, taskARN string) (ecsTaskRow, error) {
	var row ecsTaskRow
	err := s.db.QueryRow(
		`SELECT task_id, cluster_name, task_arn, task_def_arn, last_status, containers_json, started_at, stopped_at, created_at
		 FROM ecs_tasks WHERE account_id = ? AND task_arn = ?`,
		accountID, taskARN,
	).Scan(&row.TaskID, &row.ClusterName, &row.TaskARN, &row.TaskDefARN, &row.LastStatus, &row.Containers, &row.StartedAt, &row.StoppedAt, &row.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ecsTaskRow{}, ErrECSTaskNotFound
		}
		return ecsTaskRow{}, fmt.Errorf("get task row %s: %w", taskARN, err)
	}
	return row, nil
}

func taskFromRow(row ecsTaskRow, clusterARN string) (ECSTask, error) {
	var containers []map[string]any
	if row.Containers != "" {
		if err := json.Unmarshal([]byte(row.Containers), &containers); err != nil {
			return ECSTask{}, fmt.Errorf("unmarshal task containers: %w", err)
		}
	}
	return ECSTask{
		TaskARN:    row.TaskARN,
		ClusterARN: clusterARN,
		TaskDefARN: row.TaskDefARN,
		LastStatus: row.LastStatus,
		Containers: containers,
		StartedAt:  row.StartedAt,
		StoppedAt:  row.StoppedAt,
		CreatedAt:  row.CreatedAt,
	}, nil
}

func containersFromTaskDefinition(td ECSTaskDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(td.ContainerDefs))
	for _, def := range td.ContainerDefs {
		name, _ := def["name"].(string)
		out = append(out, map[string]any{
			"name":   name,
			"status": ECSTaskStatusRunning,
		})
	}
	return out
}

// RunTask creates a task in the cluster and transitions PENDING to RUNNING.
func (s *Store) RunTask(accountID, region string, in RunTaskInput) (ECSTask, error) {
	clusterName := normalizeECSClusterName(in.Cluster)
	if err := s.ensureDefaultCluster(accountID, region); err != nil {
		return ECSTask{}, err
	}
	clusterRow, err := s.getClusterRow(accountID, clusterName)
	if err != nil {
		return ECSTask{}, err
	}
	if region == "" {
		region = DefaultECSRegion
	}

	td, err := s.DescribeTaskDefinitionByARN(accountID, strings.TrimSpace(in.TaskDefinition))
	if err != nil {
		return ECSTask{}, err
	}
	if td.Status != ECSTaskDefinitionActive {
		return ECSTask{}, ErrECSTaskDefinitionNotFound
	}

	taskID := uuid.NewString()
	taskARN := TaskARN(region, accountID, clusterName, taskID)
	created := nowRFC3339()
	containers := containersFromTaskDefinition(td)
	containersJSON, err := json.Marshal(containers)
	if err != nil {
		return ECSTask{}, fmt.Errorf("run task: marshal containers: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO ecs_tasks
		 (task_id, account_id, cluster_name, task_arn, task_def_arn, last_status, containers_json, started_at, stopped_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '', '', ?)`,
		taskID, accountID, clusterName, taskARN, td.ARN, ECSTaskStatusPending, string(containersJSON), created,
	)
	if err != nil {
		return ECSTask{}, fmt.Errorf("run task insert %s: %w", taskARN, err)
	}

	started := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE ecs_tasks SET last_status = ?, started_at = ? WHERE account_id = ? AND task_arn = ?`,
		ECSTaskStatusRunning, started, accountID, taskARN,
	)
	if err != nil {
		return ECSTask{}, fmt.Errorf("run task transition %s: %w", taskARN, err)
	}

	return ECSTask{
		TaskARN:    taskARN,
		ClusterARN: clusterRow.ARN,
		TaskDefARN: td.ARN,
		LastStatus: ECSTaskStatusRunning,
		Containers: containers,
		StartedAt:  started,
		CreatedAt:  created,
	}, nil
}

// DescribeTasks returns tasks by ARN in the cluster. Missing tasks are omitted.
func (s *Store) DescribeTasks(accountID, cluster string, taskARNs []string) ([]ECSTask, error) {
	clusterName := normalizeECSClusterName(cluster)
	clusterRow, err := s.getClusterRow(accountID, clusterName)
	if err != nil {
		if clusterName == DefaultECSClusterName {
			if ensureErr := s.ensureDefaultCluster(accountID, DefaultECSRegion); ensureErr != nil {
				return nil, ensureErr
			}
			clusterRow, err = s.getClusterRow(accountID, clusterName)
		}
		if err != nil {
			return nil, err
		}
	}
	out := make([]ECSTask, 0, len(taskARNs))
	for _, arn := range taskARNs {
		row, err := s.getTaskRow(accountID, arn)
		if errors.Is(err, ErrECSTaskNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if row.ClusterName != clusterName {
			continue
		}
		task, err := taskFromRow(row, clusterRow.ARN)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, nil
}

// ListTasks returns all tasks in a cluster.
func (s *Store) ListTasks(accountID, cluster string) ([]ECSTask, error) {
	clusterName := normalizeECSClusterName(cluster)
	if err := s.ensureDefaultCluster(accountID, DefaultECSRegion); err != nil {
		return nil, err
	}
	clusterRow, err := s.getClusterRow(accountID, clusterName)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT task_id, cluster_name, task_arn, task_def_arn, last_status, containers_json, started_at, stopped_at, created_at
		 FROM ecs_tasks WHERE account_id = ? AND cluster_name = ? ORDER BY created_at`,
		accountID, clusterName,
	)
	if err != nil {
		return nil, fmt.Errorf("list tasks %s/%s: %w", accountID, clusterName, err)
	}
	defer rows.Close()
	var out []ECSTask
	for rows.Next() {
		var row ecsTaskRow
		if err := rows.Scan(&row.TaskID, &row.ClusterName, &row.TaskARN, &row.TaskDefARN, &row.LastStatus, &row.Containers, &row.StartedAt, &row.StoppedAt, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		task, err := taskFromRow(row, clusterRow.ARN)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

// SetTaskRuntimeID records the DinD container ID on the first container row for StopTask.
func (s *Store) SetTaskRuntimeID(accountID, taskARN, runtimeID string) error {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return fmt.Errorf("set task runtime id: runtime id is required")
	}
	row, err := s.getTaskRow(accountID, taskARN)
	if err != nil {
		return err
	}
	var containers []map[string]any
	if row.Containers != "" {
		if err := json.Unmarshal([]byte(row.Containers), &containers); err != nil {
			return fmt.Errorf("set task runtime id: unmarshal containers: %w", err)
		}
	}
	if len(containers) == 0 {
		containers = []map[string]any{{"name": "app"}}
	}
	containers[0]["runtimeId"] = runtimeID
	containersJSON, err := json.Marshal(containers)
	if err != nil {
		return fmt.Errorf("set task runtime id: marshal containers: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE ecs_tasks SET containers_json = ? WHERE account_id = ? AND task_arn = ?`,
		string(containersJSON), accountID, taskARN,
	)
	if err != nil {
		return fmt.Errorf("set task runtime id %s: %w", taskARN, err)
	}
	return nil
}

// TaskRuntimeID returns the DinD container ID stored on the task, if any.
func (s *Store) TaskRuntimeID(accountID, taskARN string) (string, error) {
	row, err := s.getTaskRow(accountID, taskARN)
	if err != nil {
		return "", err
	}
	var containers []map[string]any
	if row.Containers == "" {
		return "", nil
	}
	if err := json.Unmarshal([]byte(row.Containers), &containers); err != nil {
		return "", fmt.Errorf("task runtime id: unmarshal containers: %w", err)
	}
	for _, c := range containers {
		if id, ok := c["runtimeId"].(string); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id), nil
		}
	}
	return "", nil
}

// DeleteTask removes a task row entirely. Used when RunTask launch fails so
// ListTasks does not surface phantom STOPPED tasks.
func (s *Store) DeleteTask(accountID, taskARN string) error {
	taskARN = strings.TrimSpace(taskARN)
	if taskARN == "" {
		return fmt.Errorf("delete task: task arn is required")
	}
	res, err := s.db.Exec(
		`DELETE FROM ecs_tasks WHERE account_id = ? AND task_arn = ?`,
		accountID, taskARN,
	)
	if err != nil {
		return fmt.Errorf("delete task %s: %w", taskARN, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete task %s rows affected: %w", taskARN, err)
	}
	if n == 0 {
		return ErrECSTaskNotFound
	}
	return nil
}

// StopTask stops a running task.
func (s *Store) StopTask(accountID, region string, in StopTaskInput) (ECSTask, error) {
	clusterName := normalizeECSClusterName(in.Cluster)
	clusterRow, err := s.getClusterRow(accountID, clusterName)
	if err != nil {
		return ECSTask{}, err
	}
	row, err := s.getTaskRow(accountID, strings.TrimSpace(in.Task))
	if err != nil {
		return ECSTask{}, err
	}
	if row.ClusterName != clusterName {
		return ECSTask{}, ErrECSTaskNotFound
	}
	stopped := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE ecs_tasks SET last_status = ?, stopped_at = ? WHERE account_id = ? AND task_arn = ?`,
		ECSTaskStatusStopped, stopped, accountID, row.TaskARN,
	)
	if err != nil {
		return ECSTask{}, fmt.Errorf("stop task %s: %w", row.TaskARN, err)
	}
	row.LastStatus = ECSTaskStatusStopped
	row.StoppedAt = stopped
	return taskFromRow(row, clusterRow.ARN)
}
