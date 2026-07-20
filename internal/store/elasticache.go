package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrElastiCacheClusterExists   = errors.New("CacheClusterAlreadyExists")
	ErrElastiCacheClusterNotFound = errors.New("CacheClusterNotFound")
	ErrElastiCacheBadRequest      = errors.New("InvalidParameterValue")
)

const DefaultElastiCacheRegion = "us-east-1"

// Nested cache listen port inside the DinD network (never published on the host).
const ElastiCacheNestedPort = 6379

const elasticacheSchema = `
CREATE TABLE IF NOT EXISTS elasticache_clusters (
  account_id TEXT NOT NULL,
  cache_cluster_id TEXT NOT NULL,
  engine TEXT NOT NULL,
  engine_version TEXT NOT NULL DEFAULT '',
  cache_node_type TEXT NOT NULL DEFAULT 'cache.t3.micro',
  num_cache_nodes INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL,
  endpoint_address TEXT NOT NULL,
  endpoint_port INTEGER NOT NULL DEFAULT 6379,
  container_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, cache_cluster_id)
);
`

// ElastiCacheCluster is a lab cache cluster control-plane row.
type ElastiCacheCluster struct {
	CacheClusterID  string
	Engine          string
	EngineVersion   string
	CacheNodeType   string
	NumCacheNodes   int
	Status          string
	EndpointAddress string
	EndpointPort    int
	ContainerID     string
	CreatedAt       int64
}

// EnsureElastiCacheSchema creates ElastiCache tables if missing.
func EnsureElastiCacheSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure elasticache schema: db is nil")
	}
	if _, err := db.Exec(elasticacheSchema); err != nil {
		return fmt.Errorf("ensure elasticache schema: %w", err)
	}
	return nil
}

// EnsureElastiCacheSchema ensures ElastiCache tables on an open store.
func (s *Store) EnsureElastiCacheSchema() error {
	return EnsureElastiCacheSchema(s.db)
}

// ElastiCacheNestedEndpoint returns the nested-network host:port string (no host publish).
func ElastiCacheNestedEndpoint(cacheClusterID string) string {
	id := strings.ToLower(strings.TrimSpace(cacheClusterID))
	return fmt.Sprintf("%s.cache.noctaxris.internal:%d", id, ElastiCacheNestedPort)
}

// CreateElastiCacheCluster creates a cache cluster control-plane row.
// Engines: redis, valkey. Endpoint is nested-network only.
func (s *Store) CreateElastiCacheCluster(accountID, region, cacheClusterID, engine, engineVersion, nodeType string, numNodes int) (ElastiCacheCluster, error) {
	_ = region
	id := strings.ToLower(strings.TrimSpace(cacheClusterID))
	if id == "" {
		return ElastiCacheCluster{}, fmt.Errorf("%w: CacheClusterId is required", ErrElastiCacheBadRequest)
	}
	if err := validateElastiCacheID(id); err != nil {
		return ElastiCacheCluster{}, err
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		engine = "redis"
	}
	if engine != "redis" && engine != "valkey" {
		return ElastiCacheCluster{}, fmt.Errorf("%w: Engine must be redis or valkey", ErrElastiCacheBadRequest)
	}
	if nodeType == "" {
		nodeType = "cache.t3.micro"
	}
	if numNodes <= 0 {
		numNodes = 1
	}
	if engineVersion == "" {
		if engine == "valkey" {
			engineVersion = "8.0"
		} else {
			engineVersion = "7.1"
		}
	}
	var existing string
	err := s.db.QueryRow(
		`SELECT cache_cluster_id FROM elasticache_clusters WHERE account_id = ? AND cache_cluster_id = ?`,
		accountID, id,
	).Scan(&existing)
	if err == nil {
		return ElastiCacheCluster{}, ErrElastiCacheClusterExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ElastiCacheCluster{}, fmt.Errorf("create elasticache cluster: %w", err)
	}
	addr := fmt.Sprintf("%s.cache.noctaxris.internal", id)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO elasticache_clusters
		 (account_id, cache_cluster_id, engine, engine_version, cache_node_type, num_cache_nodes, status, endpoint_address, endpoint_port, container_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'available', ?, ?, '', ?)`,
		accountID, id, engine, engineVersion, nodeType, numNodes, addr, ElastiCacheNestedPort, now,
	)
	if err != nil {
		return ElastiCacheCluster{}, fmt.Errorf("create elasticache cluster: insert: %w", err)
	}
	return ElastiCacheCluster{
		CacheClusterID:  id,
		Engine:          engine,
		EngineVersion:   engineVersion,
		CacheNodeType:   nodeType,
		NumCacheNodes:   numNodes,
		Status:          "available",
		EndpointAddress: addr,
		EndpointPort:    ElastiCacheNestedPort,
		CreatedAt:       now,
	}, nil
}

// SetElastiCacheContainerID records a nested engine container id after dataplane start.
// endpointAddress may be empty to leave the existing nested endpoint unchanged.
func (s *Store) SetElastiCacheContainerID(accountID, cacheClusterID, containerID, status, endpointAddress string) error {
	id := strings.ToLower(strings.TrimSpace(cacheClusterID))
	if status == "" {
		status = "available"
	}
	var res sql.Result
	var err error
	if strings.TrimSpace(endpointAddress) == "" {
		res, err = s.db.Exec(
			`UPDATE elasticache_clusters SET container_id = ?, status = ? WHERE account_id = ? AND cache_cluster_id = ?`,
			containerID, status, accountID, id,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE elasticache_clusters SET container_id = ?, status = ?, endpoint_address = ? WHERE account_id = ? AND cache_cluster_id = ?`,
			containerID, status, endpointAddress, accountID, id,
		)
	}
	if err != nil {
		return fmt.Errorf("set elasticache container: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrElastiCacheClusterNotFound
	}
	return nil
}

// DescribeElastiCacheCluster returns one cluster by id.
func (s *Store) DescribeElastiCacheCluster(accountID, cacheClusterID string) (ElastiCacheCluster, error) {
	id := strings.ToLower(strings.TrimSpace(cacheClusterID))
	var c ElastiCacheCluster
	err := s.db.QueryRow(
		`SELECT cache_cluster_id, engine, engine_version, cache_node_type, num_cache_nodes, status, endpoint_address, endpoint_port, container_id, created_at
		 FROM elasticache_clusters WHERE account_id = ? AND cache_cluster_id = ?`,
		accountID, id,
	).Scan(&c.CacheClusterID, &c.Engine, &c.EngineVersion, &c.CacheNodeType, &c.NumCacheNodes, &c.Status, &c.EndpointAddress, &c.EndpointPort, &c.ContainerID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ElastiCacheCluster{}, ErrElastiCacheClusterNotFound
	}
	if err != nil {
		return ElastiCacheCluster{}, fmt.Errorf("describe elasticache cluster: %w", err)
	}
	return c, nil
}

// DescribeElastiCacheClusters lists clusters for an account (optional id filter).
func (s *Store) DescribeElastiCacheClusters(accountID, cacheClusterID string) ([]ElastiCacheCluster, error) {
	id := strings.ToLower(strings.TrimSpace(cacheClusterID))
	var (
		rows *sql.Rows
		err  error
	)
	if id == "" {
		rows, err = s.db.Query(
			`SELECT cache_cluster_id, engine, engine_version, cache_node_type, num_cache_nodes, status, endpoint_address, endpoint_port, container_id, created_at
			 FROM elasticache_clusters WHERE account_id = ? ORDER BY cache_cluster_id`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT cache_cluster_id, engine, engine_version, cache_node_type, num_cache_nodes, status, endpoint_address, endpoint_port, container_id, created_at
			 FROM elasticache_clusters WHERE account_id = ? AND cache_cluster_id = ?`,
			accountID, id,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("describe elasticache clusters: %w", err)
	}
	defer rows.Close()
	out := []ElastiCacheCluster{}
	for rows.Next() {
		var c ElastiCacheCluster
		if err := rows.Scan(&c.CacheClusterID, &c.Engine, &c.EngineVersion, &c.CacheNodeType, &c.NumCacheNodes, &c.Status, &c.EndpointAddress, &c.EndpointPort, &c.ContainerID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("describe elasticache clusters: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteElastiCacheCluster deletes a cluster by id. Returns container_id if set (for dataplane stop).
func (s *Store) DeleteElastiCacheCluster(accountID, cacheClusterID string) (containerID string, err error) {
	id := strings.ToLower(strings.TrimSpace(cacheClusterID))
	err = s.db.QueryRow(
		`SELECT container_id FROM elasticache_clusters WHERE account_id = ? AND cache_cluster_id = ?`,
		accountID, id,
	).Scan(&containerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrElastiCacheClusterNotFound
	}
	if err != nil {
		return "", fmt.Errorf("delete elasticache cluster: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM elasticache_clusters WHERE account_id = ? AND cache_cluster_id = ?`, accountID, id)
	if err != nil {
		return "", fmt.Errorf("delete elasticache cluster: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", ErrElastiCacheClusterNotFound
	}
	return containerID, nil
}

func validateElastiCacheID(id string) error {
	if len(id) < 1 || len(id) > 50 {
		return fmt.Errorf("%w: CacheClusterId length must be 1-50", ErrElastiCacheBadRequest)
	}
	if id[0] < 'a' || id[0] > 'z' {
		return fmt.Errorf("%w: CacheClusterId must start with a letter", ErrElastiCacheBadRequest)
	}
	if strings.HasSuffix(id, "-") || strings.Contains(id, "--") {
		return fmt.Errorf("%w: CacheClusterId cannot end with hyphen or contain consecutive hyphens", ErrElastiCacheBadRequest)
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("%w: CacheClusterId must be alphanumeric or hyphen", ErrElastiCacheBadRequest)
	}
	return nil
}
