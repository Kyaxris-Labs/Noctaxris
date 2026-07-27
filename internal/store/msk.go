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
	ErrMSKClusterExists   = errors.New("ConflictException")
	ErrMSKClusterNotFound = errors.New("NotFoundException")
	ErrMSKBadRequest      = errors.New("BadRequestException")
)

const DefaultMSKRegion = "us-east-1"

// Nested MSK (Redpanda Kafka) port inside the DinD network (never published on the host).
const MSKNestedPort = 9092

// Amazon MSK cluster states used by the lab control plane.
const (
	MSKClusterStateCreating = "CREATING"
	MSKClusterStateActive   = "ACTIVE"
	MSKClusterStateFailed   = "FAILED"
	MSKClusterStateDeleting = "DELETING"
)

const mskSchema = `
CREATE TABLE IF NOT EXISTS msk_clusters (
  account_id TEXT NOT NULL,
  cluster_uuid TEXT NOT NULL,
  cluster_name TEXT NOT NULL,
  cluster_arn TEXT NOT NULL,
  kafka_version TEXT NOT NULL DEFAULT '',
  number_of_broker_nodes INTEGER NOT NULL DEFAULT 1,
  state TEXT NOT NULL,
  bootstrap_brokers TEXT NOT NULL DEFAULT '',
  container_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, cluster_uuid)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_msk_clusters_name ON msk_clusters(account_id, cluster_name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_msk_clusters_arn ON msk_clusters(account_id, cluster_arn);
`

// MSKCluster is an Amazon MSK cluster control-plane row.
// BootstrapBrokers is nested-network only (or empty / stub until ACTIVE).
type MSKCluster struct {
	ClusterUUID          string
	ClusterName          string
	ClusterARN           string
	KafkaVersion         string
	NumberOfBrokerNodes  int
	State                string
	BootstrapBrokers     string
	ContainerID          string
	CreatedAt            int64
}

// EnsureMSKSchema creates MSK tables if missing.
func EnsureMSKSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure msk schema: db is nil")
	}
	if _, err := db.Exec(mskSchema); err != nil {
		return fmt.Errorf("ensure msk schema: %w", err)
	}
	return nil
}

// EnsureMSKSchema ensures MSK tables on an open store.
func (s *Store) EnsureMSKSchema() error {
	return EnsureMSKSchema(s.db)
}

// MSKClusterARN builds arn:aws:kafka:REGION:ACCOUNT:cluster/NAME/UUID
func MSKClusterARN(region, accountID, name, id string) string {
	if region == "" {
		region = DefaultMSKRegion
	}
	return fmt.Sprintf("arn:aws:kafka:%s:%s:cluster/%s/%s", region, accountID, name, id)
}

// MSKNestedBootstrap returns the nested-network bootstrap broker string (no host publish).
func MSKNestedBootstrap(clusterName string) string {
	name := strings.ToLower(strings.TrimSpace(clusterName))
	return fmt.Sprintf("noctaxris-msk-%s:%d", name, MSKNestedPort)
}

// CreateMSKCluster creates a control-plane MSK cluster.
// Starts CREATING; nested DinD may promote to ACTIVE. Without engine, handlers fail closed to FAILED.
func (s *Store) CreateMSKCluster(accountID, region, clusterName, kafkaVersion string, numberOfBrokerNodes int) (MSKCluster, error) {
	name := strings.ToLower(strings.TrimSpace(clusterName))
	if name == "" {
		return MSKCluster{}, fmt.Errorf("%w: ClusterName is required", ErrMSKBadRequest)
	}
	if err := validateMSKClusterName(name); err != nil {
		return MSKCluster{}, err
	}
	if kafkaVersion == "" {
		kafkaVersion = "3.6.0"
	}
	if numberOfBrokerNodes < 1 {
		numberOfBrokerNodes = 1
	}
	var existing string
	err := s.db.QueryRow(
		`SELECT cluster_uuid FROM msk_clusters WHERE account_id = ? AND cluster_name = ?`,
		accountID, name,
	).Scan(&existing)
	if err == nil {
		return MSKCluster{}, ErrMSKClusterExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MSKCluster{}, fmt.Errorf("create msk cluster: %w", err)
	}
	id := uuid.NewString()
	arn := MSKClusterARN(region, accountID, name, id)
	now := time.Now().UTC().UnixMilli()
	bootstrap := MSKNestedBootstrap(name)
	_, err = s.db.Exec(
		`INSERT INTO msk_clusters
		 (account_id, cluster_uuid, cluster_name, cluster_arn, kafka_version, number_of_broker_nodes, state, bootstrap_brokers, container_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		accountID, id, name, arn, kafkaVersion, numberOfBrokerNodes, MSKClusterStateCreating, bootstrap, now,
	)
	if err != nil {
		return MSKCluster{}, fmt.Errorf("create msk cluster: insert: %w", err)
	}
	return MSKCluster{
		ClusterUUID: id, ClusterName: name, ClusterARN: arn,
		KafkaVersion: kafkaVersion, NumberOfBrokerNodes: numberOfBrokerNodes,
		State: MSKClusterStateCreating, BootstrapBrokers: bootstrap, CreatedAt: now,
	}, nil
}

// SetMSKContainerID records a nested Redpanda container after dataplane start.
// ACTIVE requires a non-empty containerID and must not use stub:// brokers.
func (s *Store) SetMSKContainerID(accountID, clusterName, containerID, status, bootstrap string) error {
	name := strings.ToLower(strings.TrimSpace(clusterName))
	if status == "" {
		status = MSKClusterStateActive
	}
	if status == MSKClusterStateActive {
		if strings.TrimSpace(containerID) == "" {
			return fmt.Errorf("%w: ACTIVE requires container_id", ErrMSKBadRequest)
		}
		if strings.HasPrefix(strings.TrimSpace(bootstrap), "stub://") {
			return fmt.Errorf("%w: ACTIVE must not use stub:// brokers", ErrMSKBadRequest)
		}
	}
	var res sql.Result
	var err error
	if strings.TrimSpace(bootstrap) == "" {
		res, err = s.db.Exec(
			`UPDATE msk_clusters SET container_id = ?, state = ? WHERE account_id = ? AND cluster_name = ?`,
			containerID, status, accountID, name,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE msk_clusters SET container_id = ?, state = ?, bootstrap_brokers = ? WHERE account_id = ? AND cluster_name = ?`,
			containerID, status, bootstrap, accountID, name,
		)
	}
	if err != nil {
		return fmt.Errorf("set msk container: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrMSKClusterNotFound
	}
	return nil
}

// DescribeMSKClusterByARN returns a cluster by ARN.
func (s *Store) DescribeMSKClusterByARN(accountID, clusterARN string) (MSKCluster, error) {
	arn := strings.TrimSpace(clusterARN)
	var c MSKCluster
	err := s.db.QueryRow(
		`SELECT cluster_uuid, cluster_name, cluster_arn, kafka_version, number_of_broker_nodes, state, bootstrap_brokers, container_id, created_at
		 FROM msk_clusters WHERE account_id = ? AND cluster_arn = ?`,
		accountID, arn,
	).Scan(&c.ClusterUUID, &c.ClusterName, &c.ClusterARN, &c.KafkaVersion, &c.NumberOfBrokerNodes, &c.State, &c.BootstrapBrokers, &c.ContainerID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MSKCluster{}, ErrMSKClusterNotFound
	}
	if err != nil {
		return MSKCluster{}, fmt.Errorf("describe msk cluster: %w", err)
	}
	return c, nil
}

// DescribeMSKClusterByName returns a cluster by name.
func (s *Store) DescribeMSKClusterByName(accountID, clusterName string) (MSKCluster, error) {
	name := strings.ToLower(strings.TrimSpace(clusterName))
	var c MSKCluster
	err := s.db.QueryRow(
		`SELECT cluster_uuid, cluster_name, cluster_arn, kafka_version, number_of_broker_nodes, state, bootstrap_brokers, container_id, created_at
		 FROM msk_clusters WHERE account_id = ? AND cluster_name = ?`,
		accountID, name,
	).Scan(&c.ClusterUUID, &c.ClusterName, &c.ClusterARN, &c.KafkaVersion, &c.NumberOfBrokerNodes, &c.State, &c.BootstrapBrokers, &c.ContainerID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MSKCluster{}, ErrMSKClusterNotFound
	}
	if err != nil {
		return MSKCluster{}, fmt.Errorf("describe msk cluster by name: %w", err)
	}
	return c, nil
}

// ListMSKClusters lists clusters for an account.
func (s *Store) ListMSKClusters(accountID string) ([]MSKCluster, error) {
	rows, err := s.db.Query(
		`SELECT cluster_uuid, cluster_name, cluster_arn, kafka_version, number_of_broker_nodes, state, bootstrap_brokers, container_id, created_at
		 FROM msk_clusters WHERE account_id = ? ORDER BY cluster_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list msk clusters: %w", err)
	}
	defer rows.Close()
	out := []MSKCluster{}
	for rows.Next() {
		var c MSKCluster
		if err := rows.Scan(&c.ClusterUUID, &c.ClusterName, &c.ClusterARN, &c.KafkaVersion, &c.NumberOfBrokerNodes, &c.State, &c.BootstrapBrokers, &c.ContainerID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("list msk clusters: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteMSKCluster deletes a cluster by ARN. Returns container_id if set.
func (s *Store) DeleteMSKCluster(accountID, clusterARN string) (containerID string, err error) {
	arn := strings.TrimSpace(clusterARN)
	err = s.db.QueryRow(
		`SELECT container_id FROM msk_clusters WHERE account_id = ? AND cluster_arn = ?`,
		accountID, arn,
	).Scan(&containerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrMSKClusterNotFound
	}
	if err != nil {
		return "", fmt.Errorf("delete msk cluster: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM msk_clusters WHERE account_id = ? AND cluster_arn = ?`, accountID, arn)
	if err != nil {
		return "", fmt.Errorf("delete msk cluster: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", ErrMSKClusterNotFound
	}
	return containerID, nil
}

func validateMSKClusterName(name string) error {
	if len(name) < 1 || len(name) > 64 {
		return fmt.Errorf("%w: ClusterName length must be 1-64", ErrMSKBadRequest)
	}
	if name[0] < 'a' || name[0] > 'z' {
		return fmt.Errorf("%w: ClusterName must start with a letter", ErrMSKBadRequest)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("%w: ClusterName must be alphanumeric or hyphen", ErrMSKBadRequest)
	}
	return nil
}
