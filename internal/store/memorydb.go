package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrMemoryDBClusterExists   = errors.New("ClusterAlreadyExistsFault")
	ErrMemoryDBClusterNotFound = errors.New("ClusterNotFoundFault")
	ErrMemoryDBBadRequest      = errors.New("InvalidParameterValueException")
)

const DefaultMemoryDBRegion = "us-east-1"

// Nested MemoryDB listen port inside the DinD network (never published on the host).
const MemoryDBNestedPort = 6379

const memorydbSchema = `
CREATE TABLE IF NOT EXISTS memorydb_clusters (
  account_id TEXT NOT NULL,
  cluster_name TEXT NOT NULL,
  engine TEXT NOT NULL,
  engine_version TEXT NOT NULL DEFAULT '',
  node_type TEXT NOT NULL DEFAULT 'db.t4g.small',
  num_shards INTEGER NOT NULL DEFAULT 1,
  acl_name TEXT NOT NULL DEFAULT 'open-access',
  status TEXT NOT NULL,
  endpoint_address TEXT NOT NULL,
  endpoint_port INTEGER NOT NULL DEFAULT 6379,
  container_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, cluster_name)
);
`

// MemoryDBCluster is a lab MemoryDB cluster control-plane row.
type MemoryDBCluster struct {
	Name            string
	Engine          string
	EngineVersion   string
	NodeType        string
	NumberOfShards  int
	ACLName         string
	Status          string
	EndpointAddress string
	EndpointPort    int
	ContainerID     string
	ARN             string
	CreatedAt       int64
}

// EnsureMemoryDBSchema creates MemoryDB tables if missing.
func EnsureMemoryDBSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure memorydb schema: db is nil")
	}
	if _, err := db.Exec(memorydbSchema); err != nil {
		return fmt.Errorf("ensure memorydb schema: %w", err)
	}
	return nil
}

// EnsureMemoryDBSchema ensures MemoryDB tables on an open store.
func (s *Store) EnsureMemoryDBSchema() error {
	return EnsureMemoryDBSchema(s.db)
}

// MemoryDBNestedEndpoint returns the nested-network host:port string (no host publish).
func MemoryDBNestedEndpoint(clusterName string) string {
	id := strings.ToLower(strings.TrimSpace(clusterName))
	return fmt.Sprintf("%s.memorydb.noctaxris.internal:%d", id, MemoryDBNestedPort)
}

// MemoryDBClusterARN builds a lab cluster ARN.
func MemoryDBClusterARN(region, accountID, clusterName string) string {
	if region == "" {
		region = DefaultMemoryDBRegion
	}
	return fmt.Sprintf("arn:aws:memorydb:%s:%s:cluster/%s", region, accountID, strings.ToLower(strings.TrimSpace(clusterName)))
}

// CreateMemoryDBCluster creates a MemoryDB cluster control-plane row.
// Engines: redis, valkey. Endpoint is nested-network only.
func (s *Store) CreateMemoryDBCluster(accountID, region, clusterName, engine, engineVersion, nodeType, aclName string, numShards int) (MemoryDBCluster, error) {
	if region == "" {
		region = DefaultMemoryDBRegion
	}
	name := strings.ToLower(strings.TrimSpace(clusterName))
	if name == "" {
		return MemoryDBCluster{}, fmt.Errorf("%w: ClusterName is required", ErrMemoryDBBadRequest)
	}
	if err := validateMemoryDBClusterName(name); err != nil {
		return MemoryDBCluster{}, err
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		engine = "redis"
	}
	if engine != "redis" && engine != "valkey" {
		return MemoryDBCluster{}, fmt.Errorf("%w: Engine must be redis or valkey", ErrMemoryDBBadRequest)
	}
	if nodeType == "" {
		nodeType = "db.t4g.small"
	}
	if aclName == "" {
		aclName = "open-access"
	}
	if numShards <= 0 {
		numShards = 1
	}
	if engineVersion == "" {
		if engine == "valkey" {
			engineVersion = "7.2"
		} else {
			engineVersion = "7.1"
		}
	}
	var existing string
	err := s.db.QueryRow(
		`SELECT cluster_name FROM memorydb_clusters WHERE account_id = ? AND cluster_name = ?`,
		accountID, name,
	).Scan(&existing)
	if err == nil {
		return MemoryDBCluster{}, ErrMemoryDBClusterExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MemoryDBCluster{}, fmt.Errorf("create memorydb cluster: %w", err)
	}
	addr := fmt.Sprintf("%s.memorydb.noctaxris.internal", name)
	now := time.Now().UTC().UnixMilli()
	// Match ElastiCache: stay creating until SetMemoryDBContainerID after nested start.
	_, err = s.db.Exec(
		`INSERT INTO memorydb_clusters
		 (account_id, cluster_name, engine, engine_version, node_type, num_shards, acl_name, status, endpoint_address, endpoint_port, container_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'creating', ?, ?, '', ?)`,
		accountID, name, engine, engineVersion, nodeType, numShards, aclName, addr, MemoryDBNestedPort, now,
	)
	if err != nil {
		return MemoryDBCluster{}, fmt.Errorf("create memorydb cluster: insert: %w", err)
	}
	return MemoryDBCluster{
		Name:            name,
		Engine:          engine,
		EngineVersion:   engineVersion,
		NodeType:        nodeType,
		NumberOfShards:  numShards,
		ACLName:         aclName,
		Status:          "creating",
		EndpointAddress: addr,
		EndpointPort:    MemoryDBNestedPort,
		ARN:             MemoryDBClusterARN(region, accountID, name),
		CreatedAt:       now,
	}, nil
}

// SetMemoryDBContainerID records a nested engine container id after dataplane start.
// endpointAddress may be empty to leave the existing nested endpoint unchanged.
func (s *Store) SetMemoryDBContainerID(accountID, clusterName, containerID, status, endpointAddress string) error {
	id := strings.ToLower(strings.TrimSpace(clusterName))
	if status == "" {
		status = "available"
	}
	var res sql.Result
	var err error
	if strings.TrimSpace(endpointAddress) == "" {
		res, err = s.db.Exec(
			`UPDATE memorydb_clusters SET container_id = ?, status = ? WHERE account_id = ? AND cluster_name = ?`,
			containerID, status, accountID, id,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE memorydb_clusters SET container_id = ?, status = ?, endpoint_address = ? WHERE account_id = ? AND cluster_name = ?`,
			containerID, status, endpointAddress, accountID, id,
		)
	}
	if err != nil {
		return fmt.Errorf("set memorydb container: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrMemoryDBClusterNotFound
	}
	return nil
}

// DescribeMemoryDBCluster returns one cluster by name.
func (s *Store) DescribeMemoryDBCluster(accountID, region, clusterName string) (MemoryDBCluster, error) {
	id := strings.ToLower(strings.TrimSpace(clusterName))
	if id == "" {
		return MemoryDBCluster{}, fmt.Errorf("%w: ClusterName is required", ErrMemoryDBBadRequest)
	}
	var c MemoryDBCluster
	err := s.db.QueryRow(
		`SELECT cluster_name, engine, engine_version, node_type, num_shards, acl_name, status, endpoint_address, endpoint_port, container_id, created_at
		 FROM memorydb_clusters WHERE account_id = ? AND cluster_name = ?`,
		accountID, id,
	).Scan(&c.Name, &c.Engine, &c.EngineVersion, &c.NodeType, &c.NumberOfShards, &c.ACLName, &c.Status, &c.EndpointAddress, &c.EndpointPort, &c.ContainerID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MemoryDBCluster{}, ErrMemoryDBClusterNotFound
	}
	if err != nil {
		return MemoryDBCluster{}, fmt.Errorf("describe memorydb cluster: %w", err)
	}
	c.ARN = MemoryDBClusterARN(region, accountID, c.Name)
	return c, nil
}

// DescribeMemoryDBClusters lists clusters for an account (optional name filter).
func (s *Store) DescribeMemoryDBClusters(accountID, region, clusterName string) ([]MemoryDBCluster, error) {
	id := strings.ToLower(strings.TrimSpace(clusterName))
	var (
		rows *sql.Rows
		err  error
	)
	if id == "" {
		rows, err = s.db.Query(
			`SELECT cluster_name, engine, engine_version, node_type, num_shards, acl_name, status, endpoint_address, endpoint_port, container_id, created_at
			 FROM memorydb_clusters WHERE account_id = ? ORDER BY cluster_name`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT cluster_name, engine, engine_version, node_type, num_shards, acl_name, status, endpoint_address, endpoint_port, container_id, created_at
			 FROM memorydb_clusters WHERE account_id = ? AND cluster_name = ?`,
			accountID, id,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("describe memorydb clusters: %w", err)
	}
	defer rows.Close()
	out := []MemoryDBCluster{}
	for rows.Next() {
		var c MemoryDBCluster
		if err := rows.Scan(&c.Name, &c.Engine, &c.EngineVersion, &c.NodeType, &c.NumberOfShards, &c.ACLName, &c.Status, &c.EndpointAddress, &c.EndpointPort, &c.ContainerID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("describe memorydb clusters: scan: %w", err)
		}
		c.ARN = MemoryDBClusterARN(region, accountID, c.Name)
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteMemoryDBCluster deletes a cluster by name. Returns container_id if set (for dataplane stop).
func (s *Store) DeleteMemoryDBCluster(accountID, clusterName string) (containerID string, err error) {
	id := strings.ToLower(strings.TrimSpace(clusterName))
	err = s.db.QueryRow(
		`SELECT container_id FROM memorydb_clusters WHERE account_id = ? AND cluster_name = ?`,
		accountID, id,
	).Scan(&containerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrMemoryDBClusterNotFound
	}
	if err != nil {
		return "", fmt.Errorf("delete memorydb cluster: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM memorydb_clusters WHERE account_id = ? AND cluster_name = ?`, accountID, id)
	if err != nil {
		return "", fmt.Errorf("delete memorydb cluster: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", ErrMemoryDBClusterNotFound
	}
	return containerID, nil
}

func validateMemoryDBClusterName(id string) error {
	if len(id) < 1 || len(id) > 40 {
		return fmt.Errorf("%w: ClusterName length must be 1-40", ErrMemoryDBBadRequest)
	}
	if id[0] < 'a' || id[0] > 'z' {
		return fmt.Errorf("%w: ClusterName must start with a letter", ErrMemoryDBBadRequest)
	}
	if strings.HasSuffix(id, "-") || strings.Contains(id, "--") {
		return fmt.Errorf("%w: ClusterName cannot end with hyphen or contain consecutive hyphens", ErrMemoryDBBadRequest)
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("%w: ClusterName must be alphanumeric or hyphen", ErrMemoryDBBadRequest)
	}
	return nil
}
