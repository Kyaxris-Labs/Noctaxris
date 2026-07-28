package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrEMRValidation = errors.New("ValidationException")
	ErrEMRNotFound   = errors.New("InvalidRequestException")
)

const DefaultEMRRegion = "us-east-1"

const emrSchema = `
CREATE TABLE IF NOT EXISTS emr_clusters (
  account_id TEXT NOT NULL,
  cluster_id TEXT NOT NULL,
  cluster_name TEXT NOT NULL,
  cluster_arn TEXT NOT NULL,
  status TEXT NOT NULL,
  release_label TEXT NOT NULL DEFAULT 'emr-7.0.0',
  log_uri TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, cluster_id)
);
CREATE INDEX IF NOT EXISTS idx_emr_clusters_name ON emr_clusters(account_id, cluster_name);
CREATE TABLE IF NOT EXISTS emr_steps (
  account_id TEXT NOT NULL,
  cluster_id TEXT NOT NULL,
  step_id TEXT NOT NULL,
  step_seq INTEGER NOT NULL,
  step_name TEXT NOT NULL DEFAULT '',
  jar TEXT NOT NULL DEFAULT '',
  main_class TEXT NOT NULL DEFAULT '',
  args_json TEXT NOT NULL DEFAULT '[]',
  properties_json TEXT NOT NULL DEFAULT '{}',
  action_on_failure TEXT NOT NULL DEFAULT 'TERMINATE_CLUSTER',
  execution_role_arn TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  start_at INTEGER NOT NULL DEFAULT 0,
  end_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, cluster_id, step_id)
);
CREATE INDEX IF NOT EXISTS idx_emr_steps_cluster ON emr_steps(account_id, cluster_id, step_seq);
`

// EMRCluster is an EMR control-plane stub row (no host Spark).
type EMRCluster struct {
	ClusterID    string
	ClusterName  string
	ClusterARN   string
	Status       string
	ReleaseLabel string
	LogURI       string
	CreatedAt    int64
}

// EnsureEMRSchema creates EMR tables if missing.
func EnsureEMRSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure emr schema: db is nil")
	}
	if _, err := db.Exec(emrSchema); err != nil {
		return fmt.Errorf("ensure emr schema: %w", err)
	}
	return nil
}

// EnsureEMRSchema ensures EMR tables on an open store.
func (s *Store) EnsureEMRSchema() error {
	return EnsureEMRSchema(s.db)
}

// EMRClusterARN builds arn:aws:elasticmapreduce:REGION:ACCOUNT:cluster/ID
func EMRClusterARN(region, accountID, clusterID string) string {
	if region == "" {
		region = DefaultEMRRegion
	}
	return fmt.Sprintf("arn:aws:elasticmapreduce:%s:%s:cluster/%s", region, accountID, clusterID)
}

// RunEMRJobFlow creates a control-plane cluster in WAITING state.
func (s *Store) RunEMRJobFlow(accountID, region, name, releaseLabel, logURI string) (EMRCluster, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return EMRCluster{}, fmt.Errorf("%w: Name is required", ErrEMRValidation)
	}
	if releaseLabel == "" {
		releaseLabel = "emr-7.0.0"
	}
	id := "j-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:13])
	arn := EMRClusterARN(region, accountID, id)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO emr_clusters
		 (account_id, cluster_id, cluster_name, cluster_arn, status, release_label, log_uri, created_at)
		 VALUES (?, ?, ?, ?, 'WAITING', ?, ?, ?)`,
		accountID, id, name, arn, releaseLabel, strings.TrimSpace(logURI), now,
	)
	if err != nil {
		return EMRCluster{}, fmt.Errorf("run emr job flow: insert: %w", err)
	}
	return EMRCluster{
		ClusterID: id, ClusterName: name, ClusterARN: arn,
		Status: "WAITING", ReleaseLabel: releaseLabel, LogURI: logURI, CreatedAt: now,
	}, nil
}

// DescribeEMRCluster returns a cluster by id.
func (s *Store) DescribeEMRCluster(accountID, clusterID string) (EMRCluster, error) {
	var c EMRCluster
	err := s.db.QueryRow(
		`SELECT cluster_id, cluster_name, cluster_arn, status, release_label, log_uri, created_at
		 FROM emr_clusters WHERE account_id = ? AND cluster_id = ?`,
		accountID, clusterID,
	).Scan(&c.ClusterID, &c.ClusterName, &c.ClusterARN, &c.Status, &c.ReleaseLabel, &c.LogURI, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EMRCluster{}, ErrEMRNotFound
	}
	if err != nil {
		return EMRCluster{}, fmt.Errorf("describe emr cluster: %w", err)
	}
	return c, nil
}

// ListEMRClusters lists clusters for an account.
func (s *Store) ListEMRClusters(accountID string) ([]EMRCluster, error) {
	rows, err := s.db.Query(
		`SELECT cluster_id, cluster_name, cluster_arn, status, release_label, log_uri, created_at
		 FROM emr_clusters WHERE account_id = ? ORDER BY cluster_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list emr clusters: %w", err)
	}
	defer rows.Close()
	out := []EMRCluster{}
	for rows.Next() {
		var c EMRCluster
		if err := rows.Scan(&c.ClusterID, &c.ClusterName, &c.ClusterARN, &c.Status, &c.ReleaseLabel, &c.LogURI, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("list emr clusters: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// EMRStepInput is a HadoopJarStep-shaped step submitted via AddJobFlowSteps.
type EMRStepInput struct {
	Name             string
	Jar              string
	MainClass        string
	Args             []string
	Properties       map[string]string
	ActionOnFailure  string
	ExecutionRoleArn string
}

// EMRStep is a persisted EMR step row (lab state machine; no Spark execution).
type EMRStep struct {
	StepID           string
	ClusterID        string
	Name             string
	Jar              string
	MainClass        string
	Args             []string
	Properties       map[string]string
	ActionOnFailure  string
	ExecutionRoleArn string
	State            string
	CreatedAt        int64
	StartAt          int64
	EndAt            int64
	StepSeq          int
}

func newEMRStepID() string {
	return "s-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:13])
}

func (s *Store) nextEMRStepSeq(tx *sql.Tx, accountID, clusterID string) (int, error) {
	var maxSeq sql.NullInt64
	err := tx.QueryRow(
		`SELECT MAX(step_seq) FROM emr_steps WHERE account_id = ? AND cluster_id = ?`,
		accountID, clusterID,
	).Scan(&maxSeq)
	if err != nil {
		return 0, fmt.Errorf("emr step seq: %w", err)
	}
	if !maxSeq.Valid {
		return 1, nil
	}
	return int(maxSeq.Int64) + 1, nil
}

// AddEMRJobFlowSteps appends steps and immediately marks them COMPLETED (lab stub).
func (s *Store) AddEMRJobFlowSteps(accountID, clusterID string, inputs []EMRStepInput) ([]string, error) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, fmt.Errorf("%w: JobFlowId is required", ErrEMRValidation)
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("%w: Steps is required", ErrEMRValidation)
	}
	cluster, err := s.DescribeEMRCluster(accountID, clusterID)
	if errors.Is(err, ErrEMRNotFound) {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrEMRNotFound, clusterID)
	}
	if err != nil {
		return nil, fmt.Errorf("add emr job flow steps: describe cluster: %w", err)
	}
	if cluster.Status == "TERMINATED" {
		return nil, fmt.Errorf("%w: cluster %s is terminated", ErrEMRNotFound, clusterID)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("add emr job flow steps: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ids := make([]string, 0, len(inputs))
	now := time.Now().UTC().UnixMilli()
	for _, in := range inputs {
		seq, err := s.nextEMRStepSeq(tx, accountID, clusterID)
		if err != nil {
			return nil, fmt.Errorf("add emr job flow steps: %w", err)
		}
		stepID := newEMRStepID()
		actionOnFailure := strings.TrimSpace(in.ActionOnFailure)
		if actionOnFailure == "" {
			actionOnFailure = "TERMINATE_CLUSTER"
		}
		args := in.Args
		if args == nil {
			args = []string{}
		}
		props := in.Properties
		if props == nil {
			props = map[string]string{}
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("add emr job flow steps: marshal args: %w", err)
		}
		propsJSON, err := json.Marshal(props)
		if err != nil {
			return nil, fmt.Errorf("add emr job flow steps: marshal properties: %w", err)
		}
		_, err = tx.Exec(
			`INSERT INTO emr_steps
			 (account_id, cluster_id, step_id, step_seq, step_name, jar, main_class, args_json, properties_json,
			  action_on_failure, execution_role_arn, state, created_at, start_at, end_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'COMPLETED', ?, ?, ?)`,
			accountID, clusterID, stepID, seq, strings.TrimSpace(in.Name), strings.TrimSpace(in.Jar),
			strings.TrimSpace(in.MainClass), string(argsJSON), string(propsJSON),
			actionOnFailure, strings.TrimSpace(in.ExecutionRoleArn), now, now, now,
		)
		if err != nil {
			return nil, fmt.Errorf("add emr job flow steps: insert: %w", err)
		}
		ids = append(ids, stepID)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("add emr job flow steps: commit: %w", err)
	}
	return ids, nil
}

func scanEMRStep(clusterID string, row scanner) (EMRStep, error) {
	var st EMRStep
	var argsJSON, propsJSON string
	st.ClusterID = clusterID
	err := row.Scan(
		&st.StepID, &st.StepSeq, &st.Name, &st.Jar, &st.MainClass, &argsJSON, &propsJSON,
		&st.ActionOnFailure, &st.ExecutionRoleArn, &st.State, &st.CreatedAt, &st.StartAt, &st.EndAt,
	)
	if err != nil {
		return EMRStep{}, err
	}
	st.Args = []string{}
	if argsJSON != "" && argsJSON != "[]" {
		if err := json.Unmarshal([]byte(argsJSON), &st.Args); err != nil {
			return EMRStep{}, fmt.Errorf("scan emr step args: %w", err)
		}
	}
	st.Properties = map[string]string{}
	if propsJSON != "" && propsJSON != "{}" {
		if err := json.Unmarshal([]byte(propsJSON), &st.Properties); err != nil {
			return EMRStep{}, fmt.Errorf("scan emr step properties: %w", err)
		}
	}
	return st, nil
}

type scanner interface {
	Scan(dest ...any) error
}

// DescribeEMRStep returns a step by cluster and step id.
func (s *Store) DescribeEMRStep(accountID, clusterID, stepID string) (EMRStep, error) {
	clusterID = strings.TrimSpace(clusterID)
	stepID = strings.TrimSpace(stepID)
	if clusterID == "" || stepID == "" {
		return EMRStep{}, fmt.Errorf("%w: ClusterId and StepId are required", ErrEMRValidation)
	}
	if _, err := s.DescribeEMRCluster(accountID, clusterID); err != nil {
		return EMRStep{}, err
	}
	row := s.db.QueryRow(
		`SELECT step_id, step_seq, step_name, jar, main_class, args_json, properties_json,
		        action_on_failure, execution_role_arn, state, created_at, start_at, end_at
		 FROM emr_steps WHERE account_id = ? AND cluster_id = ? AND step_id = ?`,
		accountID, clusterID, stepID,
	)
	st, err := scanEMRStep(clusterID, row)
	if errors.Is(err, sql.ErrNoRows) {
		return EMRStep{}, fmt.Errorf("%w: step %s does not exist", ErrEMRNotFound, stepID)
	}
	if err != nil {
		return EMRStep{}, fmt.Errorf("describe emr step: %w", err)
	}
	return st, nil
}

// ListEMRSteps returns steps newest-first, optionally filtered by state or step id.
func (s *Store) ListEMRSteps(accountID, clusterID string, states, stepIDs []string) ([]EMRStep, error) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterId is required", ErrEMRValidation)
	}
	if _, err := s.DescribeEMRCluster(accountID, clusterID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT step_id, step_seq, step_name, jar, main_class, args_json, properties_json,
		        action_on_failure, execution_role_arn, state, created_at, start_at, end_at
		 FROM emr_steps WHERE account_id = ? AND cluster_id = ?
		 ORDER BY step_seq DESC`,
		accountID, clusterID,
	)
	if err != nil {
		return nil, fmt.Errorf("list emr steps: %w", err)
	}
	defer rows.Close()

	stateSet := map[string]struct{}{}
	for _, st := range states {
		if t := strings.TrimSpace(st); t != "" {
			stateSet[t] = struct{}{}
		}
	}
	idSet := map[string]struct{}{}
	for _, id := range stepIDs {
		if t := strings.TrimSpace(id); t != "" {
			idSet[t] = struct{}{}
		}
	}
	filterStates := len(stateSet) > 0
	filterIDs := len(idSet) > 0

	out := []EMRStep{}
	for rows.Next() {
		st, err := scanEMRStep(clusterID, rows)
		if err != nil {
			return nil, fmt.Errorf("list emr steps: scan: %w", err)
		}
		if filterStates {
			if _, ok := stateSet[st.State]; !ok {
				continue
			}
		}
		if filterIDs {
			if _, ok := idSet[st.StepID]; !ok {
				continue
			}
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// TerminateEMRJobFlows marks clusters as TERMINATED.
func (s *Store) TerminateEMRJobFlows(accountID string, clusterIDs []string) error {
	if len(clusterIDs) == 0 {
		return fmt.Errorf("%w: JobFlowIds is required", ErrEMRValidation)
	}
	for _, id := range clusterIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		res, err := s.db.Exec(
			`UPDATE emr_clusters SET status = 'TERMINATED' WHERE account_id = ? AND cluster_id = ?`,
			accountID, id,
		)
		if err != nil {
			return fmt.Errorf("terminate emr job flows: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("%w: cluster %s not found", ErrEMRNotFound, id)
		}
	}
	return nil
}
