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
