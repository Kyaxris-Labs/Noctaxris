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
  tags_json TEXT NOT NULL DEFAULT '{}',
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
CREATE TABLE IF NOT EXISTS emr_instance_groups (
  account_id TEXT NOT NULL,
  cluster_id TEXT NOT NULL,
  group_id TEXT NOT NULL,
  group_name TEXT NOT NULL DEFAULT '',
  instance_group_type TEXT NOT NULL DEFAULT '',
  instance_type TEXT NOT NULL DEFAULT '',
  market TEXT NOT NULL DEFAULT '',
  bid_price TEXT NOT NULL DEFAULT '',
  requested_count INTEGER NOT NULL DEFAULT 0,
  running_count INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL DEFAULT 'RUNNING',
  PRIMARY KEY (account_id, cluster_id, group_id)
);
CREATE INDEX IF NOT EXISTS idx_emr_instance_groups_cluster ON emr_instance_groups(account_id, cluster_id);
CREATE TABLE IF NOT EXISTS emr_instance_fleets (
  account_id TEXT NOT NULL,
  cluster_id TEXT NOT NULL,
  fleet_id TEXT NOT NULL,
  fleet_name TEXT NOT NULL DEFAULT '',
  instance_fleet_type TEXT NOT NULL DEFAULT '',
  target_on_demand INTEGER NOT NULL DEFAULT 0,
  target_spot INTEGER NOT NULL DEFAULT 0,
  provisioned_on_demand INTEGER NOT NULL DEFAULT 0,
  provisioned_spot INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL DEFAULT 'RUNNING',
  PRIMARY KEY (account_id, cluster_id, fleet_id)
);
CREATE INDEX IF NOT EXISTS idx_emr_instance_fleets_cluster ON emr_instance_fleets(account_id, cluster_id);
CREATE TABLE IF NOT EXISTS emr_security_configurations (
  account_id TEXT NOT NULL,
  config_name TEXT NOT NULL,
  security_configuration TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, config_name)
);
`

// EMRCluster is an EMR control-plane stub row (no host Spark).
type EMRCluster struct {
	ClusterID    string
	ClusterName  string
	ClusterARN   string
	Status       string
	ReleaseLabel string
	LogURI       string
	Tags         map[string]string
	CreatedAt    int64
}

// EMRRunJobFlowInput is the RunJobFlow request payload used by the store.
type EMRRunJobFlowInput struct {
	Name         string
	ReleaseLabel string
	LogURI       string
	Tags         map[string]string
	Groups       []EMRInstanceGroupInput
	Fleets       []EMRInstanceFleetInput
}

// EMRInstanceGroupInput is InstanceGroups[] metadata from RunJobFlow Instances.
type EMRInstanceGroupInput struct {
	Name              string
	InstanceGroupType string
	InstanceType      string
	Market            string
	BidPrice          string
	InstanceCount     int
}

// EMRInstanceFleetInput is InstanceFleets[] metadata from RunJobFlow Instances.
type EMRInstanceFleetInput struct {
	Name                   string
	InstanceFleetType      string
	TargetOnDemandCapacity int
	TargetSpotCapacity     int
}

// EMRInstanceGroup is a persisted instance group row.
type EMRInstanceGroup struct {
	ID                     string
	Name                   string
	InstanceGroupType      string
	InstanceType           string
	Market                 string
	BidPrice               string
	RequestedInstanceCount int
	RunningInstanceCount   int
	State                  string
}

// EMRInstanceFleet is a persisted instance fleet row.
type EMRInstanceFleet struct {
	ID                          string
	Name                        string
	InstanceFleetType           string
	TargetOnDemandCapacity      int
	TargetSpotCapacity          int
	ProvisionedOnDemandCapacity int
	ProvisionedSpotCapacity     int
	State                       string
}

// EMRSecurityConfiguration is a named security configuration JSON blob.
type EMRSecurityConfiguration struct {
	Name                  string
	SecurityConfiguration string
	CreatedAt             int64
}

// EMRCancelStepInfo is one CancelSteps result entry.
type EMRCancelStepInfo struct {
	StepID string
	Status string
	Reason string
}

// EnsureEMRSchema creates EMR tables if missing.
func EnsureEMRSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure emr schema: db is nil")
	}
	if _, err := db.Exec(emrSchema); err != nil {
		return fmt.Errorf("ensure emr schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE emr_clusters ADD COLUMN tags_json TEXT NOT NULL DEFAULT '{}'`,
	}); err != nil {
		return fmt.Errorf("ensure emr schema: migrate: %w", err)
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

func newEMRInstanceGroupID() string {
	return "ig-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:13])
}

func newEMRInstanceFleetID() string {
	return "if-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:13])
}

func marshalEMRTags(tags map[string]string) (string, error) {
	if tags == nil {
		tags = map[string]string{}
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("marshal emr tags: %w", err)
	}
	return string(b), nil
}

func unmarshalEMRTags(raw string) (map[string]string, error) {
	out := map[string]string{}
	if raw == "" || raw == "{}" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("unmarshal emr tags: %w", err)
	}
	return out, nil
}

// RunEMRJobFlow creates a control-plane cluster in WAITING state.
func (s *Store) RunEMRJobFlow(accountID, region string, in EMRRunJobFlowInput) (EMRCluster, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return EMRCluster{}, fmt.Errorf("%w: Name is required", ErrEMRValidation)
	}
	releaseLabel := in.ReleaseLabel
	if releaseLabel == "" {
		releaseLabel = "emr-7.0.0"
	}
	tags := in.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	tagsJSON, err := marshalEMRTags(tags)
	if err != nil {
		return EMRCluster{}, err
	}

	id := "j-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:13])
	arn := EMRClusterARN(region, accountID, id)
	now := time.Now().UTC().UnixMilli()

	tx, err := s.db.Begin()
	if err != nil {
		return EMRCluster{}, fmt.Errorf("run emr job flow: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(
		`INSERT INTO emr_clusters
		 (account_id, cluster_id, cluster_name, cluster_arn, status, release_label, log_uri, tags_json, created_at)
		 VALUES (?, ?, ?, ?, 'WAITING', ?, ?, ?, ?)`,
		accountID, id, name, arn, releaseLabel, strings.TrimSpace(in.LogURI), tagsJSON, now,
	)
	if err != nil {
		return EMRCluster{}, fmt.Errorf("run emr job flow: insert: %w", err)
	}

	for _, g := range in.Groups {
		groupID := newEMRInstanceGroupID()
		count := g.InstanceCount
		_, err = tx.Exec(
			`INSERT INTO emr_instance_groups
			 (account_id, cluster_id, group_id, group_name, instance_group_type, instance_type,
			  market, bid_price, requested_count, running_count, state)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'RUNNING')`,
			accountID, id, groupID, strings.TrimSpace(g.Name), strings.TrimSpace(g.InstanceGroupType),
			strings.TrimSpace(g.InstanceType), strings.TrimSpace(g.Market), strings.TrimSpace(g.BidPrice),
			count, count,
		)
		if err != nil {
			return EMRCluster{}, fmt.Errorf("run emr job flow: insert group: %w", err)
		}
	}
	for _, f := range in.Fleets {
		fleetID := newEMRInstanceFleetID()
		_, err = tx.Exec(
			`INSERT INTO emr_instance_fleets
			 (account_id, cluster_id, fleet_id, fleet_name, instance_fleet_type,
			  target_on_demand, target_spot, provisioned_on_demand, provisioned_spot, state)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'RUNNING')`,
			accountID, id, fleetID, strings.TrimSpace(f.Name), strings.TrimSpace(f.InstanceFleetType),
			f.TargetOnDemandCapacity, f.TargetSpotCapacity, f.TargetOnDemandCapacity, f.TargetSpotCapacity,
		)
		if err != nil {
			return EMRCluster{}, fmt.Errorf("run emr job flow: insert fleet: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return EMRCluster{}, fmt.Errorf("run emr job flow: commit: %w", err)
	}
	return EMRCluster{
		ClusterID: id, ClusterName: name, ClusterARN: arn,
		Status: "WAITING", ReleaseLabel: releaseLabel, LogURI: in.LogURI,
		Tags: tags, CreatedAt: now,
	}, nil
}

func scanEMRCluster(row scanner) (EMRCluster, error) {
	var c EMRCluster
	var tagsJSON string
	err := row.Scan(
		&c.ClusterID, &c.ClusterName, &c.ClusterARN, &c.Status,
		&c.ReleaseLabel, &c.LogURI, &tagsJSON, &c.CreatedAt,
	)
	if err != nil {
		return EMRCluster{}, err
	}
	tags, err := unmarshalEMRTags(tagsJSON)
	if err != nil {
		return EMRCluster{}, err
	}
	c.Tags = tags
	return c, nil
}

// DescribeEMRCluster returns a cluster by id.
func (s *Store) DescribeEMRCluster(accountID, clusterID string) (EMRCluster, error) {
	row := s.db.QueryRow(
		`SELECT cluster_id, cluster_name, cluster_arn, status, release_label, log_uri, tags_json, created_at
		 FROM emr_clusters WHERE account_id = ? AND cluster_id = ?`,
		accountID, clusterID,
	)
	c, err := scanEMRCluster(row)
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
		`SELECT cluster_id, cluster_name, cluster_arn, status, release_label, log_uri, tags_json, created_at
		 FROM emr_clusters WHERE account_id = ? ORDER BY cluster_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list emr clusters: %w", err)
	}
	defer rows.Close()
	out := []EMRCluster{}
	for rows.Next() {
		c, err := scanEMRCluster(rows)
		if err != nil {
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

// CancelEMRSteps marks matching steps CANCELLED (lab stub; works on COMPLETED steps).
func (s *Store) CancelEMRSteps(accountID, clusterID string, stepIDs []string) ([]EMRCancelStepInfo, error) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterId is required", ErrEMRValidation)
	}
	if len(stepIDs) == 0 {
		return nil, fmt.Errorf("%w: StepIds is required", ErrEMRValidation)
	}
	if _, err := s.DescribeEMRCluster(accountID, clusterID); err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("cancel emr steps: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().UnixMilli()
	out := make([]EMRCancelStepInfo, 0, len(stepIDs))
	for _, rawID := range stepIDs {
		stepID := strings.TrimSpace(rawID)
		if stepID == "" {
			continue
		}
		var state string
		err := tx.QueryRow(
			`SELECT state FROM emr_steps WHERE account_id = ? AND cluster_id = ? AND step_id = ?`,
			accountID, clusterID, stepID,
		).Scan(&state)
		if errors.Is(err, sql.ErrNoRows) {
			out = append(out, EMRCancelStepInfo{StepID: stepID, Status: "FAILED", Reason: "Step does not exist."})
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cancel emr steps: select: %w", err)
		}
		if state == "CANCELLED" {
			out = append(out, EMRCancelStepInfo{
				StepID: stepID, Status: "FAILED", Reason: "Cannot cancel the step. It is already CANCELLED.",
			})
			continue
		}
		_, err = tx.Exec(
			`UPDATE emr_steps SET state = 'CANCELLED', end_at = ? WHERE account_id = ? AND cluster_id = ? AND step_id = ?`,
			now, accountID, clusterID, stepID,
		)
		if err != nil {
			return nil, fmt.Errorf("cancel emr steps: update: %w", err)
		}
		out = append(out, EMRCancelStepInfo{StepID: stepID, Status: "SUBMITTED"})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cancel emr steps: commit: %w", err)
	}
	return out, nil
}

// ListEMRInstanceGroups returns instance groups for a cluster.
func (s *Store) ListEMRInstanceGroups(accountID, clusterID string) ([]EMRInstanceGroup, error) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterId is required", ErrEMRValidation)
	}
	if _, err := s.DescribeEMRCluster(accountID, clusterID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT group_id, group_name, instance_group_type, instance_type, market, bid_price,
		        requested_count, running_count, state
		 FROM emr_instance_groups WHERE account_id = ? AND cluster_id = ? ORDER BY group_id`,
		accountID, clusterID,
	)
	if err != nil {
		return nil, fmt.Errorf("list emr instance groups: %w", err)
	}
	defer rows.Close()
	out := []EMRInstanceGroup{}
	for rows.Next() {
		var g EMRInstanceGroup
		if err := rows.Scan(
			&g.ID, &g.Name, &g.InstanceGroupType, &g.InstanceType, &g.Market, &g.BidPrice,
			&g.RequestedInstanceCount, &g.RunningInstanceCount, &g.State,
		); err != nil {
			return nil, fmt.Errorf("list emr instance groups: scan: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListEMRInstanceFleets returns instance fleets for a cluster.
func (s *Store) ListEMRInstanceFleets(accountID, clusterID string) ([]EMRInstanceFleet, error) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterId is required", ErrEMRValidation)
	}
	if _, err := s.DescribeEMRCluster(accountID, clusterID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT fleet_id, fleet_name, instance_fleet_type, target_on_demand, target_spot,
		        provisioned_on_demand, provisioned_spot, state
		 FROM emr_instance_fleets WHERE account_id = ? AND cluster_id = ? ORDER BY fleet_id`,
		accountID, clusterID,
	)
	if err != nil {
		return nil, fmt.Errorf("list emr instance fleets: %w", err)
	}
	defer rows.Close()
	out := []EMRInstanceFleet{}
	for rows.Next() {
		var f EMRInstanceFleet
		if err := rows.Scan(
			&f.ID, &f.Name, &f.InstanceFleetType, &f.TargetOnDemandCapacity, &f.TargetSpotCapacity,
			&f.ProvisionedOnDemandCapacity, &f.ProvisionedSpotCapacity, &f.State,
		); err != nil {
			return nil, fmt.Errorf("list emr instance fleets: scan: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// AddEMRTags merges tags onto a cluster (ResourceId = cluster id).
func (s *Store) AddEMRTags(accountID, resourceID string, tags map[string]string) error {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return fmt.Errorf("%w: ResourceId is required", ErrEMRValidation)
	}
	if len(tags) == 0 {
		return fmt.Errorf("%w: Tags is required", ErrEMRValidation)
	}
	c, err := s.DescribeEMRCluster(accountID, resourceID)
	if err != nil {
		return err
	}
	merged := map[string]string{}
	for k, v := range c.Tags {
		merged[k] = v
	}
	for k, v := range tags {
		if strings.TrimSpace(k) == "" {
			continue
		}
		merged[k] = v
	}
	tagsJSON, err := marshalEMRTags(merged)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE emr_clusters SET tags_json = ? WHERE account_id = ? AND cluster_id = ?`,
		tagsJSON, accountID, resourceID,
	)
	if err != nil {
		return fmt.Errorf("add emr tags: %w", err)
	}
	return nil
}

// RemoveEMRTags deletes tag keys from a cluster.
func (s *Store) RemoveEMRTags(accountID, resourceID string, keys []string) error {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return fmt.Errorf("%w: ResourceId is required", ErrEMRValidation)
	}
	c, err := s.DescribeEMRCluster(accountID, resourceID)
	if err != nil {
		return err
	}
	for _, k := range keys {
		delete(c.Tags, k)
	}
	tagsJSON, err := marshalEMRTags(c.Tags)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE emr_clusters SET tags_json = ? WHERE account_id = ? AND cluster_id = ?`,
		tagsJSON, accountID, resourceID,
	)
	if err != nil {
		return fmt.Errorf("remove emr tags: %w", err)
	}
	return nil
}

// CreateEMRSecurityConfiguration stores a named security configuration JSON string.
func (s *Store) CreateEMRSecurityConfiguration(accountID, name, securityConfiguration string) (EMRSecurityConfiguration, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return EMRSecurityConfiguration{}, fmt.Errorf("%w: Name is required", ErrEMRValidation)
	}
	if strings.TrimSpace(securityConfiguration) == "" {
		return EMRSecurityConfiguration{}, fmt.Errorf("%w: SecurityConfiguration is required", ErrEMRValidation)
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO emr_security_configurations (account_id, config_name, security_configuration, created_at)
		 VALUES (?, ?, ?, ?)`,
		accountID, name, securityConfiguration, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return EMRSecurityConfiguration{}, fmt.Errorf("%w: Security configuration already exists: %s", ErrEMRNotFound, name)
		}
		return EMRSecurityConfiguration{}, fmt.Errorf("create emr security configuration: %w", err)
	}
	return EMRSecurityConfiguration{Name: name, SecurityConfiguration: securityConfiguration, CreatedAt: now}, nil
}

// DescribeEMRSecurityConfiguration returns a security configuration by name.
func (s *Store) DescribeEMRSecurityConfiguration(accountID, name string) (EMRSecurityConfiguration, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return EMRSecurityConfiguration{}, fmt.Errorf("%w: Name is required", ErrEMRValidation)
	}
	var sc EMRSecurityConfiguration
	err := s.db.QueryRow(
		`SELECT config_name, security_configuration, created_at
		 FROM emr_security_configurations WHERE account_id = ? AND config_name = ?`,
		accountID, name,
	).Scan(&sc.Name, &sc.SecurityConfiguration, &sc.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EMRSecurityConfiguration{}, fmt.Errorf("%w: Security configuration does not exist: %s", ErrEMRNotFound, name)
	}
	if err != nil {
		return EMRSecurityConfiguration{}, fmt.Errorf("describe emr security configuration: %w", err)
	}
	return sc, nil
}

// DeleteEMRSecurityConfiguration deletes a security configuration by name.
func (s *Store) DeleteEMRSecurityConfiguration(accountID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: Name is required", ErrEMRValidation)
	}
	res, err := s.db.Exec(
		`DELETE FROM emr_security_configurations WHERE account_id = ? AND config_name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete emr security configuration: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: Security configuration does not exist: %s", ErrEMRNotFound, name)
	}
	return nil
}

// ListEMRSecurityConfigurations lists security configurations for an account.
func (s *Store) ListEMRSecurityConfigurations(accountID string) ([]EMRSecurityConfiguration, error) {
	rows, err := s.db.Query(
		`SELECT config_name, security_configuration, created_at
		 FROM emr_security_configurations WHERE account_id = ? ORDER BY config_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list emr security configurations: %w", err)
	}
	defer rows.Close()
	out := []EMRSecurityConfiguration{}
	for rows.Next() {
		var sc EMRSecurityConfiguration
		if err := rows.Scan(&sc.Name, &sc.SecurityConfiguration, &sc.CreatedAt); err != nil {
			return nil, fmt.Errorf("list emr security configurations: scan: %w", err)
		}
		out = append(out, sc)
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
