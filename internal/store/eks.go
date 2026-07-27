package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrEKSClusterExists   = errors.New("ResourceInUseException")
	ErrEKSClusterNotFound = errors.New("ResourceNotFoundException")
	ErrEKSBadRequest      = errors.New("InvalidParameterException")
)

const DefaultEKSRegion = "us-east-1"

// Nested EKS API-server port string (metadata only; no live kubectl).
const EKSNestedPort = 6443

// Amazon EKS cluster states used by the lab control plane.
const (
	EKSClusterStatusCreating = "CREATING"
	EKSClusterStatusActive   = "ACTIVE"
	EKSClusterStatusDeleting = "DELETING"
)

const eksSchema = `
CREATE TABLE IF NOT EXISTS eks_clusters (
  account_id TEXT NOT NULL,
  cluster_name TEXT NOT NULL,
  cluster_arn TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '1.29',
  status TEXT NOT NULL,
  endpoint TEXT NOT NULL DEFAULT '',
  vpc_config_json TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, cluster_name)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_eks_clusters_arn ON eks_clusters(account_id, cluster_arn);
`

// EKSCluster is an Amazon EKS cluster control-plane row.
// Endpoint is a nested-network string only; there is no live Kubernetes API server.
type EKSCluster struct {
	Name             string
	ARN              string
	RoleARN          string
	Version          string
	Status           string
	Endpoint         string
	ResourcesVpcJSON string
	CreatedAt        int64
}

// EnsureEKSSchema creates EKS tables if missing.
func EnsureEKSSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure eks schema: db is nil")
	}
	if _, err := db.Exec(eksSchema); err != nil {
		return fmt.Errorf("ensure eks schema: %w", err)
	}
	return nil
}

// EnsureEKSSchema ensures EKS tables on an open store.
func (s *Store) EnsureEKSSchema() error {
	return EnsureEKSSchema(s.db)
}

// EKSClusterARN builds arn:aws:eks:REGION:ACCOUNT:cluster/NAME
func EKSClusterARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultEKSRegion
	}
	return fmt.Sprintf("arn:aws:eks:%s:%s:cluster/%s", region, accountID, name)
}

// EKSNestedEndpoint returns the nested-network API endpoint string (no host publish, no live kubectl).
func EKSNestedEndpoint(clusterName string) string {
	name := strings.ToLower(strings.TrimSpace(clusterName))
	return fmt.Sprintf("https://noctaxris-eks-%s:%d", name, EKSNestedPort)
}

// CreateEKSCluster creates a control-plane EKS cluster in ACTIVE status (metadata-only).
// Nested k3s is not started; the endpoint string is nested-network only.
func (s *Store) CreateEKSCluster(accountID, region, name, roleARN, version, vpcConfigJSON string) (EKSCluster, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return EKSCluster{}, fmt.Errorf("%w: name is required", ErrEKSBadRequest)
	}
	if err := validateEKSClusterName(n); err != nil {
		return EKSCluster{}, err
	}
	role := strings.TrimSpace(roleARN)
	if role == "" {
		return EKSCluster{}, fmt.Errorf("%w: roleArn is required", ErrEKSBadRequest)
	}
	if version == "" {
		version = "1.29"
	}
	if strings.TrimSpace(vpcConfigJSON) == "" {
		vpcConfigJSON = "{}"
	}
	if !json.Valid([]byte(vpcConfigJSON)) {
		return EKSCluster{}, fmt.Errorf("%w: resourcesVpcConfig must be JSON", ErrEKSBadRequest)
	}
	var existing string
	err := s.db.QueryRow(
		`SELECT cluster_name FROM eks_clusters WHERE account_id = ? AND cluster_name = ?`,
		accountID, n,
	).Scan(&existing)
	if err == nil {
		return EKSCluster{}, ErrEKSClusterExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return EKSCluster{}, fmt.Errorf("create eks cluster: %w", err)
	}
	arn := EKSClusterARN(region, accountID, n)
	now := time.Now().UTC().UnixMilli()
	endpoint := EKSNestedEndpoint(n)
	_, err = s.db.Exec(
		`INSERT INTO eks_clusters
		 (account_id, cluster_name, cluster_arn, role_arn, version, status, endpoint, vpc_config_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, n, arn, role, version, EKSClusterStatusActive, endpoint, vpcConfigJSON, now,
	)
	if err != nil {
		return EKSCluster{}, fmt.Errorf("create eks cluster: insert: %w", err)
	}
	return EKSCluster{
		Name: n, ARN: arn, RoleARN: role, Version: version,
		Status: EKSClusterStatusActive, Endpoint: endpoint,
		ResourcesVpcJSON: vpcConfigJSON, CreatedAt: now,
	}, nil
}

// DescribeEKSCluster returns a cluster by name.
func (s *Store) DescribeEKSCluster(accountID, name string) (EKSCluster, error) {
	n := strings.TrimSpace(name)
	var c EKSCluster
	err := s.db.QueryRow(
		`SELECT cluster_name, cluster_arn, role_arn, version, status, endpoint, vpc_config_json, created_at
		 FROM eks_clusters WHERE account_id = ? AND cluster_name = ?`,
		accountID, n,
	).Scan(&c.Name, &c.ARN, &c.RoleARN, &c.Version, &c.Status, &c.Endpoint, &c.ResourcesVpcJSON, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EKSCluster{}, ErrEKSClusterNotFound
	}
	if err != nil {
		return EKSCluster{}, fmt.Errorf("describe eks cluster: %w", err)
	}
	return c, nil
}

// ListEKSClusters lists cluster names for an account.
func (s *Store) ListEKSClusters(accountID string) ([]EKSCluster, error) {
	rows, err := s.db.Query(
		`SELECT cluster_name, cluster_arn, role_arn, version, status, endpoint, vpc_config_json, created_at
		 FROM eks_clusters WHERE account_id = ? ORDER BY cluster_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list eks clusters: %w", err)
	}
	defer rows.Close()
	out := []EKSCluster{}
	for rows.Next() {
		var c EKSCluster
		if err := rows.Scan(&c.Name, &c.ARN, &c.RoleARN, &c.Version, &c.Status, &c.Endpoint, &c.ResourcesVpcJSON, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("list eks clusters: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteEKSCluster deletes a cluster by name.
func (s *Store) DeleteEKSCluster(accountID, name string) (EKSCluster, error) {
	c, err := s.DescribeEKSCluster(accountID, name)
	if err != nil {
		return EKSCluster{}, err
	}
	res, err := s.db.Exec(`DELETE FROM eks_clusters WHERE account_id = ? AND cluster_name = ?`, accountID, strings.TrimSpace(name))
	if err != nil {
		return EKSCluster{}, fmt.Errorf("delete eks cluster: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return EKSCluster{}, ErrEKSClusterNotFound
	}
	c.Status = EKSClusterStatusDeleting
	return c, nil
}

func validateEKSClusterName(name string) error {
	if len(name) < 1 || len(name) > 100 {
		return fmt.Errorf("%w: name length must be 1-100", ErrEKSBadRequest)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("%w: name must be alphanumeric, hyphen, or underscore", ErrEKSBadRequest)
	}
	return nil
}
