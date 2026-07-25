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
	// DefaultBatchRegion is the lab region embedded in Batch ARNs.
	DefaultBatchRegion = "us-east-1"

	BatchCEStatusValid      = "VALID"
	BatchJQStatusValid      = "VALID"
	BatchJobStatusSubmitted = "SUBMITTED"
	BatchJobStatusRunning   = "RUNNING"
	BatchJobStatusSucceeded = "SUCCEEDED"
	BatchJobStatusFailed    = "FAILED"
)

var (
	ErrBatchAlreadyExists = errors.New("ClientException: already exists")
	ErrBatchNotFound      = errors.New("ClientException: not found")
	ErrBatchInvalidInput  = errors.New("ClientException: invalid input")
)

// BatchComputeEnvironment is a CreateComputeEnvironment row.
type BatchComputeEnvironment struct {
	Name        string
	ARN         string
	Type        string
	State       string
	Status      string
	ServiceRole string
	CreatedAt   string
}

// BatchJobQueue is a CreateJobQueue row.
type BatchJobQueue struct {
	Name      string
	ARN       string
	State     string
	Status    string
	Priority  int
	CEOrders  string // JSON array of compute environment order
	CreatedAt string
}

// BatchJobDefinition is a RegisterJobDefinition revision.
type BatchJobDefinition struct {
	Name              string
	ARN               string
	Revision          int
	Type              string
	Image             string
	Command           []string
	JobRoleARN        string
	ExecutionRoleARN  string
	EnvJSON           string
	Status            string
	CreatedAt         string
}

// BatchJob is a SubmitJob row.
type BatchJob struct {
	JobID       string
	JobName     string
	JobQueue    string
	JobDefARN   string
	Status      string
	ContainerID string
	CreatedAt   string
	StartedAt   string
	StoppedAt   string
}

// CreateBatchComputeEnvironmentInput holds CreateComputeEnvironment fields.
type CreateBatchComputeEnvironmentInput struct {
	Name        string
	Type        string
	State       string
	ServiceRole string
}

// CreateBatchJobQueueInput holds CreateJobQueue fields.
type CreateBatchJobQueueInput struct {
	Name                 string
	State                string
	Priority             int
	ComputeEnvironmentOrder []map[string]any
}

// RegisterBatchJobDefinitionInput holds RegisterJobDefinition fields.
type RegisterBatchJobDefinitionInput struct {
	Name             string
	Type             string
	Image            string
	Command          []string
	JobRoleARN       string
	ExecutionRoleARN string
	Env              map[string]string
}

const batchSchema = `
CREATE TABLE IF NOT EXISTS batch_compute_environments (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  ce_arn TEXT NOT NULL,
  type TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'ENABLED',
  status TEXT NOT NULL DEFAULT 'VALID',
  service_role TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS batch_job_queues (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  jq_arn TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'ENABLED',
  status TEXT NOT NULL DEFAULT 'VALID',
  priority INTEGER NOT NULL DEFAULT 1,
  ce_order_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS batch_job_definitions (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  revision INTEGER NOT NULL,
  jd_arn TEXT NOT NULL,
  type TEXT NOT NULL,
  image TEXT NOT NULL,
  command_json TEXT NOT NULL DEFAULT '[]',
  job_role_arn TEXT NOT NULL DEFAULT '',
  env_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  created_at TEXT NOT NULL,
  PRIMARY KEY (account_id, name, revision)
);
CREATE TABLE IF NOT EXISTS batch_jobs (
  account_id TEXT NOT NULL,
  job_id TEXT NOT NULL,
  job_name TEXT NOT NULL,
  job_queue TEXT NOT NULL,
  job_def_arn TEXT NOT NULL,
  status TEXT NOT NULL,
  container_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  started_at TEXT NOT NULL DEFAULT '',
  stopped_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, job_id)
);
CREATE INDEX IF NOT EXISTS idx_batch_jobs_queue ON batch_jobs(account_id, job_queue);
`

// EnsureBatchSchema creates Batch tables if missing.
func EnsureBatchSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure batch schema: db is nil")
	}
	if _, err := db.Exec(batchSchema); err != nil {
		return fmt.Errorf("ensure batch schema: %w", err)
	}
	if err := execMigrateStmt(db, `ALTER TABLE batch_job_definitions ADD COLUMN execution_role_arn TEXT NOT NULL DEFAULT ''`, nil); err != nil {
		return fmt.Errorf("ensure batch schema: execution_role_arn: %w", err)
	}
	return nil
}

// EnsureBatchSchema ensures Batch tables on an open store.
func (s *Store) EnsureBatchSchema() error {
	return EnsureBatchSchema(s.db)
}

// BatchComputeEnvironmentARN builds arn:aws:batch:REGION:ACCOUNT:compute-environment/NAME.
func BatchComputeEnvironmentARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultBatchRegion
	}
	return fmt.Sprintf("arn:aws:batch:%s:%s:compute-environment/%s", region, accountID, name)
}

// BatchJobQueueARN builds arn:aws:batch:REGION:ACCOUNT:job-queue/NAME.
func BatchJobQueueARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultBatchRegion
	}
	return fmt.Sprintf("arn:aws:batch:%s:%s:job-queue/%s", region, accountID, name)
}

// BatchJobDefinitionARN builds arn:aws:batch:REGION:ACCOUNT:job-definition/NAME:REVISION.
func BatchJobDefinitionARN(region, accountID, name string, revision int) string {
	if region == "" {
		region = DefaultBatchRegion
	}
	return fmt.Sprintf("arn:aws:batch:%s:%s:job-definition/%s:%d", region, accountID, name, revision)
}

// CreateBatchComputeEnvironment inserts a compute environment.
func (s *Store) CreateBatchComputeEnvironment(accountID, region string, in CreateBatchComputeEnvironmentInput) (BatchComputeEnvironment, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return BatchComputeEnvironment{}, fmt.Errorf("%w: computeEnvironmentName is required", ErrBatchInvalidInput)
	}
	typ := strings.TrimSpace(in.Type)
	if typ == "" {
		typ = "MANAGED"
	}
	state := strings.TrimSpace(in.State)
	if state == "" {
		state = "ENABLED"
	}
	arn := BatchComputeEnvironmentARN(region, accountID, name)
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO batch_compute_environments (
			account_id, name, ce_arn, type, state, status, service_role, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, arn, typ, state, BatchCEStatusValid, strings.TrimSpace(in.ServiceRole), created,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return BatchComputeEnvironment{}, ErrBatchAlreadyExists
		}
		return BatchComputeEnvironment{}, fmt.Errorf("create batch compute environment: %w", err)
	}
	return BatchComputeEnvironment{
		Name: name, ARN: arn, Type: typ, State: state, Status: BatchCEStatusValid,
		ServiceRole: strings.TrimSpace(in.ServiceRole), CreatedAt: created,
	}, nil
}

// CreateBatchJobQueue inserts a job queue.
func (s *Store) CreateBatchJobQueue(accountID, region string, in CreateBatchJobQueueInput) (BatchJobQueue, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return BatchJobQueue{}, fmt.Errorf("%w: jobQueueName is required", ErrBatchInvalidInput)
	}
	if len(in.ComputeEnvironmentOrder) == 0 {
		return BatchJobQueue{}, fmt.Errorf("%w: computeEnvironmentOrder is required", ErrBatchInvalidInput)
	}
	for _, order := range in.ComputeEnvironmentOrder {
		ceName := stringFromAny(order["computeEnvironment"])
		if ceName == "" {
			return BatchJobQueue{}, fmt.Errorf("%w: computeEnvironment is required", ErrBatchInvalidInput)
		}
		if strings.Contains(ceName, "/") {
			ceName = ceName[strings.LastIndex(ceName, "/")+1:]
		}
		if _, err := s.getBatchCE(accountID, ceName); err != nil {
			return BatchJobQueue{}, fmt.Errorf("%w: compute environment not found", ErrBatchInvalidInput)
		}
	}
	state := strings.TrimSpace(in.State)
	if state == "" {
		state = "ENABLED"
	}
	priority := in.Priority
	if priority == 0 {
		priority = 1
	}
	orderJSON, err := json.Marshal(in.ComputeEnvironmentOrder)
	if err != nil {
		return BatchJobQueue{}, err
	}
	arn := BatchJobQueueARN(region, accountID, name)
	created := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO batch_job_queues (
			account_id, name, jq_arn, state, status, priority, ce_order_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, arn, state, BatchJQStatusValid, priority, string(orderJSON), created,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return BatchJobQueue{}, ErrBatchAlreadyExists
		}
		return BatchJobQueue{}, fmt.Errorf("create batch job queue: %w", err)
	}
	return BatchJobQueue{
		Name: name, ARN: arn, State: state, Status: BatchJQStatusValid,
		Priority: priority, CEOrders: string(orderJSON), CreatedAt: created,
	}, nil
}

func (s *Store) getBatchCE(accountID, name string) (BatchComputeEnvironment, error) {
	var ce BatchComputeEnvironment
	err := s.db.QueryRow(
		`SELECT name, ce_arn, type, state, status, service_role, created_at
		 FROM batch_compute_environments WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&ce.Name, &ce.ARN, &ce.Type, &ce.State, &ce.Status, &ce.ServiceRole, &ce.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BatchComputeEnvironment{}, ErrBatchNotFound
	}
	if err != nil {
		return BatchComputeEnvironment{}, err
	}
	return ce, nil
}

// RegisterBatchJobDefinition inserts a new revision.
func (s *Store) RegisterBatchJobDefinition(accountID, region string, in RegisterBatchJobDefinitionInput) (BatchJobDefinition, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return BatchJobDefinition{}, fmt.Errorf("%w: jobDefinitionName is required", ErrBatchInvalidInput)
	}
	typ := strings.TrimSpace(in.Type)
	if typ == "" {
		typ = "container"
	}
	image := strings.TrimSpace(in.Image)
	if image == "" {
		return BatchJobDefinition{}, fmt.Errorf("%w: containerProperties.image is required", ErrBatchInvalidInput)
	}
	var maxRev sql.NullInt64
	_ = s.db.QueryRow(
		`SELECT MAX(revision) FROM batch_job_definitions WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&maxRev)
	rev := 1
	if maxRev.Valid {
		rev = int(maxRev.Int64) + 1
	}
	cmdJSON, _ := json.Marshal(in.Command)
	if in.Command == nil {
		cmdJSON = []byte("[]")
	}
	envJSON, _ := json.Marshal(in.Env)
	if in.Env == nil {
		envJSON = []byte("{}")
	}
	arn := BatchJobDefinitionARN(region, accountID, name, rev)
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO batch_job_definitions (
			account_id, name, revision, jd_arn, type, image, command_json, job_role_arn, env_json, status, created_at, execution_role_arn
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'ACTIVE', ?, ?)`,
		accountID, name, rev, arn, typ, image, string(cmdJSON), strings.TrimSpace(in.JobRoleARN), string(envJSON), created,
		strings.TrimSpace(in.ExecutionRoleARN),
	)
	if err != nil {
		return BatchJobDefinition{}, fmt.Errorf("register batch job definition: %w", err)
	}
	return BatchJobDefinition{
		Name: name, ARN: arn, Revision: rev, Type: typ, Image: image,
		Command: in.Command, JobRoleARN: strings.TrimSpace(in.JobRoleARN),
		ExecutionRoleARN: strings.TrimSpace(in.ExecutionRoleARN),
		EnvJSON: string(envJSON), Status: "ACTIVE", CreatedAt: created,
	}, nil
}

// GetBatchJobDefinition resolves name, name:rev, or ARN.
func (s *Store) GetBatchJobDefinition(accountID, ref string) (BatchJobDefinition, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return BatchJobDefinition{}, ErrBatchNotFound
	}
	if strings.Contains(ref, "job-definition/") {
		var jd BatchJobDefinition
		var cmdJSON string
		err := s.db.QueryRow(
			`SELECT name, revision, jd_arn, type, image, command_json, job_role_arn, env_json, status, created_at,
			        COALESCE(execution_role_arn, '')
			 FROM batch_job_definitions WHERE account_id = ? AND jd_arn = ?`,
			accountID, ref,
		).Scan(&jd.Name, &jd.Revision, &jd.ARN, &jd.Type, &jd.Image, &cmdJSON, &jd.JobRoleARN, &jd.EnvJSON, &jd.Status, &jd.CreatedAt, &jd.ExecutionRoleARN)
		if errors.Is(err, sql.ErrNoRows) {
			return BatchJobDefinition{}, ErrBatchNotFound
		}
		if err != nil {
			return BatchJobDefinition{}, err
		}
		_ = json.Unmarshal([]byte(cmdJSON), &jd.Command)
		return jd, nil
	}
	name := ref
	rev := 0
	if i := strings.LastIndex(ref, ":"); i > 0 && !strings.Contains(ref[i+1:], "/") {
		name = ref[:i]
		fmt.Sscanf(ref[i+1:], "%d", &rev)
	}
	if rev > 0 {
		return s.getBatchJobDefinitionRev(accountID, name, rev)
	}
	var maxRev int
	err := s.db.QueryRow(
		`SELECT MAX(revision) FROM batch_job_definitions WHERE account_id = ? AND name = ? AND status = 'ACTIVE'`,
		accountID, name,
	).Scan(&maxRev)
	if errors.Is(err, sql.ErrNoRows) || maxRev == 0 {
		return BatchJobDefinition{}, ErrBatchNotFound
	}
	if err != nil {
		return BatchJobDefinition{}, err
	}
	return s.getBatchJobDefinitionRev(accountID, name, maxRev)
}

func (s *Store) getBatchJobDefinitionRev(accountID, name string, rev int) (BatchJobDefinition, error) {
	var jd BatchJobDefinition
	var cmdJSON string
	err := s.db.QueryRow(
		`SELECT name, revision, jd_arn, type, image, command_json, job_role_arn, env_json, status, created_at,
		        COALESCE(execution_role_arn, '')
		 FROM batch_job_definitions WHERE account_id = ? AND name = ? AND revision = ?`,
		accountID, name, rev,
	).Scan(&jd.Name, &jd.Revision, &jd.ARN, &jd.Type, &jd.Image, &cmdJSON, &jd.JobRoleARN, &jd.EnvJSON, &jd.Status, &jd.CreatedAt, &jd.ExecutionRoleARN)
	if errors.Is(err, sql.ErrNoRows) {
		return BatchJobDefinition{}, ErrBatchNotFound
	}
	if err != nil {
		return BatchJobDefinition{}, err
	}
	_ = json.Unmarshal([]byte(cmdJSON), &jd.Command)
	return jd, nil
}

func (s *Store) getBatchJobQueue(accountID, name string) (BatchJobQueue, error) {
	name = strings.TrimSpace(name)
	if strings.Contains(name, "/") {
		name = name[strings.LastIndex(name, "/")+1:]
	}
	var jq BatchJobQueue
	err := s.db.QueryRow(
		`SELECT name, jq_arn, state, status, priority, ce_order_json, created_at
		 FROM batch_job_queues WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&jq.Name, &jq.ARN, &jq.State, &jq.Status, &jq.Priority, &jq.CEOrders, &jq.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BatchJobQueue{}, ErrBatchNotFound
	}
	if err != nil {
		return BatchJobQueue{}, err
	}
	return jq, nil
}

// SubmitBatchJob creates a SUBMITTED job row (container start is async).
func (s *Store) SubmitBatchJob(accountID, region, jobName, queueRef, jobDefRef string) (BatchJob, BatchJobDefinition, error) {
	_ = region
	jq, err := s.getBatchJobQueue(accountID, queueRef)
	if err != nil {
		return BatchJob{}, BatchJobDefinition{}, fmt.Errorf("%w: job queue not found", ErrBatchInvalidInput)
	}
	jd, err := s.GetBatchJobDefinition(accountID, jobDefRef)
	if err != nil {
		return BatchJob{}, BatchJobDefinition{}, fmt.Errorf("%w: job definition not found", ErrBatchInvalidInput)
	}
	jobName = strings.TrimSpace(jobName)
	if jobName == "" {
		return BatchJob{}, BatchJobDefinition{}, fmt.Errorf("%w: jobName is required", ErrBatchInvalidInput)
	}
	id := uuid.NewString()
	created := nowRFC3339()
	job := BatchJob{
		JobID: id, JobName: jobName, JobQueue: jq.Name, JobDefARN: jd.ARN,
		Status: BatchJobStatusSubmitted, CreatedAt: created, StartedAt: "",
	}
	_, err = s.db.Exec(
		`INSERT INTO batch_jobs (
			account_id, job_id, job_name, job_queue, job_def_arn, status, container_id, created_at, started_at, stopped_at
		) VALUES (?, ?, ?, ?, ?, ?, '', ?, '', '')`,
		accountID, job.JobID, job.JobName, job.JobQueue, job.JobDefARN, job.Status, job.CreatedAt,
	)
	if err != nil {
		return BatchJob{}, BatchJobDefinition{}, fmt.Errorf("submit batch job: %w", err)
	}
	return job, jd, nil
}

// SetBatchJobRuntime updates container id and status.
func (s *Store) SetBatchJobRuntime(accountID, jobID, containerID, status, stoppedAt string) error {
	startedAt := ""
	if status == BatchJobStatusRunning || status == BatchJobStatusSucceeded || status == BatchJobStatusFailed {
		startedAt = nowRFC3339()
	}
	_, err := s.db.Exec(
		`UPDATE batch_jobs SET container_id = ?, status = ?, stopped_at = ?,
		 started_at = CASE WHEN started_at = '' AND ? != '' THEN ? ELSE started_at END
		 WHERE account_id = ? AND job_id = ?`,
		containerID, status, stoppedAt, startedAt, startedAt, accountID, jobID,
	)
	if err != nil {
		return fmt.Errorf("set batch job runtime: %w", err)
	}
	return nil
}

// DescribeBatchComputeEnvironments returns CEs by name (empty = all).
func (s *Store) DescribeBatchComputeEnvironments(accountID string, names []string) ([]BatchComputeEnvironment, error) {
	if len(names) == 0 {
		rows, err := s.db.Query(
			`SELECT name, ce_arn, type, state, status, service_role, created_at
			 FROM batch_compute_environments WHERE account_id = ? ORDER BY name`,
			accountID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanBatchCEs(rows)
	}
	out := make([]BatchComputeEnvironment, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if strings.Contains(n, "/") {
			n = n[strings.LastIndex(n, "/")+1:]
		}
		ce, err := s.getBatchCE(accountID, n)
		if errors.Is(err, ErrBatchNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, ce)
	}
	return out, nil
}

func scanBatchCEs(rows *sql.Rows) ([]BatchComputeEnvironment, error) {
	var out []BatchComputeEnvironment
	for rows.Next() {
		var ce BatchComputeEnvironment
		if err := rows.Scan(&ce.Name, &ce.ARN, &ce.Type, &ce.State, &ce.Status, &ce.ServiceRole, &ce.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ce)
	}
	return out, rows.Err()
}

// DescribeBatchJobQueues returns queues by name (empty = all).
func (s *Store) DescribeBatchJobQueues(accountID string, names []string) ([]BatchJobQueue, error) {
	if len(names) == 0 {
		rows, err := s.db.Query(
			`SELECT name, jq_arn, state, status, priority, ce_order_json, created_at
			 FROM batch_job_queues WHERE account_id = ? ORDER BY name`,
			accountID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []BatchJobQueue
		for rows.Next() {
			var jq BatchJobQueue
			if err := rows.Scan(&jq.Name, &jq.ARN, &jq.State, &jq.Status, &jq.Priority, &jq.CEOrders, &jq.CreatedAt); err != nil {
				return nil, err
			}
			out = append(out, jq)
		}
		return out, rows.Err()
	}
	out := make([]BatchJobQueue, 0, len(names))
	for _, n := range names {
		jq, err := s.getBatchJobQueue(accountID, n)
		if errors.Is(err, ErrBatchNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, jq)
	}
	return out, nil
}

// DescribeBatchJobDefinitions returns active definitions matching name (empty = all latest).
func (s *Store) DescribeBatchJobDefinitions(accountID, name string) ([]BatchJobDefinition, error) {
	name = strings.TrimSpace(name)
	query := `SELECT name, revision, jd_arn, type, image, command_json, job_role_arn, env_json, status, created_at,
		COALESCE(execution_role_arn, '')
		FROM batch_job_definitions WHERE account_id = ? AND status = 'ACTIVE'`
	args := []any{accountID}
	if name != "" {
		query += ` AND name = ?`
		args = append(args, name)
	}
	query += ` ORDER BY name, revision`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BatchJobDefinition
	for rows.Next() {
		var jd BatchJobDefinition
		var cmdJSON string
		if err := rows.Scan(&jd.Name, &jd.Revision, &jd.ARN, &jd.Type, &jd.Image, &cmdJSON, &jd.JobRoleARN, &jd.EnvJSON, &jd.Status, &jd.CreatedAt, &jd.ExecutionRoleARN); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(cmdJSON), &jd.Command)
		out = append(out, jd)
	}
	return out, rows.Err()
}

// DescribeBatchJobs returns jobs by id.
func (s *Store) DescribeBatchJobs(accountID string, ids []string) ([]BatchJob, error) {
	out := make([]BatchJob, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		var j BatchJob
		err := s.db.QueryRow(
			`SELECT job_id, job_name, job_queue, job_def_arn, status, container_id, created_at, started_at, stopped_at
			 FROM batch_jobs WHERE account_id = ? AND job_id = ?`,
			accountID, id,
		).Scan(&j.JobID, &j.JobName, &j.JobQueue, &j.JobDefARN, &j.Status, &j.ContainerID, &j.CreatedAt, &j.StartedAt, &j.StoppedAt)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
