package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNeptuneClusterExists   = errors.New("DBClusterAlreadyExistsFault")
	ErrNeptuneClusterNotFound = errors.New("DBClusterNotFoundFault")
	ErrNeptuneBadRequest      = errors.New("InvalidParameterValue")
)

const DefaultNeptuneRegion = "us-east-1"

// Nested Neptune (Gremlin Server) port inside the DinD network (never published on the host).
const NeptuneNestedPort = 8182

const neptuneSchema = `
CREATE TABLE IF NOT EXISTS neptune_clusters (
  account_id TEXT NOT NULL,
  db_cluster_identifier TEXT NOT NULL,
  engine TEXT NOT NULL,
  engine_version TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  endpoint_address TEXT NOT NULL,
  endpoint_port INTEGER NOT NULL DEFAULT 8182,
  container_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, db_cluster_identifier)
);
`

// NeptuneCluster is a Neptune cluster control-plane row (not DocumentDB).
type NeptuneCluster struct {
	DBClusterIdentifier string
	Engine              string
	EngineVersion       string
	Status              string
	EndpointAddress     string
	EndpointPort        int
	ContainerID         string
	CreatedAt           int64
}

// EnsureNeptuneSchema creates Neptune tables if missing.
func EnsureNeptuneSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure neptune schema: db is nil")
	}
	if _, err := db.Exec(neptuneSchema); err != nil {
		return fmt.Errorf("ensure neptune schema: %w", err)
	}
	return nil
}

// EnsureNeptuneSchema ensures Neptune tables on an open store.
func (s *Store) EnsureNeptuneSchema() error {
	return EnsureNeptuneSchema(s.db)
}

// NeptuneNestedEndpoint returns the nested-network host:port string (no host publish).
func NeptuneNestedEndpoint(dbClusterIdentifier string) string {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	return fmt.Sprintf("%s.neptune.noctaxris.internal:%d", id, NeptuneNestedPort)
}

// CreateNeptuneCluster creates a Neptune cluster control-plane row. Engine must be neptune.
func (s *Store) CreateNeptuneCluster(accountID, region, dbClusterIdentifier, engine, engineVersion string, port int) (NeptuneCluster, error) {
	_ = region
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	if id == "" {
		return NeptuneCluster{}, fmt.Errorf("%w: DBClusterIdentifier is required", ErrNeptuneBadRequest)
	}
	if err := validateNeptuneClusterID(id); err != nil {
		return NeptuneCluster{}, err
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		return NeptuneCluster{}, fmt.Errorf("%w: Engine is required", ErrNeptuneBadRequest)
	}
	if engine != "neptune" {
		return NeptuneCluster{}, fmt.Errorf("%w: Engine must be neptune", ErrNeptuneBadRequest)
	}
	if engineVersion == "" {
		engineVersion = "1.3.0.0"
	}
	if port <= 0 {
		port = NeptuneNestedPort
	}
	var existing string
	err := s.db.QueryRow(
		`SELECT db_cluster_identifier FROM neptune_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
		accountID, id,
	).Scan(&existing)
	if err == nil {
		return NeptuneCluster{}, ErrNeptuneClusterExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return NeptuneCluster{}, fmt.Errorf("create neptune cluster: %w", err)
	}
	addr := fmt.Sprintf("%s.neptune.noctaxris.internal", id)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO neptune_clusters
		 (account_id, db_cluster_identifier, engine, engine_version, status, endpoint_address, endpoint_port, container_id, created_at)
		 VALUES (?, ?, ?, ?, 'creating', ?, ?, '', ?)`,
		accountID, id, engine, engineVersion, addr, port, now,
	)
	if err != nil {
		return NeptuneCluster{}, fmt.Errorf("create neptune cluster: insert: %w", err)
	}
	return NeptuneCluster{
		DBClusterIdentifier: id,
		Engine:              engine,
		EngineVersion:       engineVersion,
		Status:              "creating",
		EndpointAddress:     addr,
		EndpointPort:        port,
		CreatedAt:           now,
	}, nil
}

// SetNeptuneContainerID records a nested Gremlin container id after dataplane start.
// endpointAddress may be empty to leave the existing nested endpoint unchanged.
func (s *Store) SetNeptuneContainerID(accountID, dbClusterIdentifier, containerID, status, endpointAddress string) error {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	if status == "" {
		status = "available"
	}
	var res sql.Result
	var err error
	if strings.TrimSpace(endpointAddress) == "" {
		res, err = s.db.Exec(
			`UPDATE neptune_clusters SET container_id = ?, status = ? WHERE account_id = ? AND db_cluster_identifier = ?`,
			containerID, status, accountID, id,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE neptune_clusters SET container_id = ?, status = ?, endpoint_address = ? WHERE account_id = ? AND db_cluster_identifier = ?`,
			containerID, status, endpointAddress, accountID, id,
		)
	}
	if err != nil {
		return fmt.Errorf("set neptune container: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNeptuneClusterNotFound
	}
	return nil
}

// DescribeNeptuneCluster returns one cluster by id.
func (s *Store) DescribeNeptuneCluster(accountID, dbClusterIdentifier string) (NeptuneCluster, error) {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	var c NeptuneCluster
	err := s.db.QueryRow(
		`SELECT db_cluster_identifier, engine, engine_version, status, endpoint_address, endpoint_port, container_id, created_at
		 FROM neptune_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
		accountID, id,
	).Scan(&c.DBClusterIdentifier, &c.Engine, &c.EngineVersion, &c.Status, &c.EndpointAddress, &c.EndpointPort, &c.ContainerID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return NeptuneCluster{}, ErrNeptuneClusterNotFound
	}
	if err != nil {
		return NeptuneCluster{}, fmt.Errorf("describe neptune cluster: %w", err)
	}
	return c, nil
}

// DescribeNeptuneClusters lists clusters (optional id filter).
func (s *Store) DescribeNeptuneClusters(accountID, dbClusterIdentifier string) ([]NeptuneCluster, error) {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	var (
		rows *sql.Rows
		err  error
	)
	if id == "" {
		rows, err = s.db.Query(
			`SELECT db_cluster_identifier, engine, engine_version, status, endpoint_address, endpoint_port, container_id, created_at
			 FROM neptune_clusters WHERE account_id = ? ORDER BY db_cluster_identifier`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT db_cluster_identifier, engine, engine_version, status, endpoint_address, endpoint_port, container_id, created_at
			 FROM neptune_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
			accountID, id,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("describe neptune clusters: %w", err)
	}
	defer rows.Close()
	out := []NeptuneCluster{}
	for rows.Next() {
		var c NeptuneCluster
		if err := rows.Scan(&c.DBClusterIdentifier, &c.Engine, &c.EngineVersion, &c.Status, &c.EndpointAddress, &c.EndpointPort, &c.ContainerID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("describe neptune clusters: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteNeptuneCluster deletes a cluster by id. Returns container_id if set.
func (s *Store) DeleteNeptuneCluster(accountID, dbClusterIdentifier string) (containerID string, err error) {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	err = s.db.QueryRow(
		`SELECT container_id FROM neptune_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
		accountID, id,
	).Scan(&containerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNeptuneClusterNotFound
	}
	if err != nil {
		return "", fmt.Errorf("delete neptune cluster: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM neptune_clusters WHERE account_id = ? AND db_cluster_identifier = ?`, accountID, id)
	if err != nil {
		return "", fmt.Errorf("delete neptune cluster: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", ErrNeptuneClusterNotFound
	}
	return containerID, nil
}

func validateNeptuneClusterID(id string) error {
	if len(id) < 1 || len(id) > 63 {
		return fmt.Errorf("%w: DBClusterIdentifier length must be 1-63", ErrNeptuneBadRequest)
	}
	if id[0] < 'a' || id[0] > 'z' {
		return fmt.Errorf("%w: DBClusterIdentifier must start with a letter", ErrNeptuneBadRequest)
	}
	if strings.HasSuffix(id, "-") || strings.Contains(id, "--") {
		return fmt.Errorf("%w: DBClusterIdentifier cannot end with hyphen or contain consecutive hyphens", ErrNeptuneBadRequest)
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("%w: DBClusterIdentifier must be alphanumeric or hyphen", ErrNeptuneBadRequest)
	}
	return nil
}
