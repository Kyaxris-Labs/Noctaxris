package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrDocDBClusterExists   = errors.New("DBClusterAlreadyExistsFault")
	ErrDocDBClusterNotFound = errors.New("DBClusterNotFoundFault")
	ErrDocDBBadRequest      = errors.New("InvalidParameterValue")
)

const DefaultDocDBRegion = "us-east-1"

// Nested DocumentDB (Mongo-compatible) port inside the DinD network (never published on the host).
const DocDBNestedPort = 27017

const docdbSchema = `
CREATE TABLE IF NOT EXISTS docdb_clusters (
  account_id TEXT NOT NULL,
  db_cluster_identifier TEXT NOT NULL,
  engine TEXT NOT NULL,
  engine_version TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  endpoint_address TEXT NOT NULL,
  endpoint_port INTEGER NOT NULL DEFAULT 27017,
  master_username TEXT NOT NULL DEFAULT '',
  container_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, db_cluster_identifier)
);
`

// DocDBCluster is a DocumentDB cluster control-plane row (not Neptune).
type DocDBCluster struct {
	DBClusterIdentifier string
	Engine              string
	EngineVersion       string
	Status              string
	EndpointAddress     string
	EndpointPort        int
	MasterUsername      string
	ContainerID         string
	CreatedAt           int64
}

// EnsureDocDBSchema creates DocumentDB tables if missing.
func EnsureDocDBSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure docdb schema: db is nil")
	}
	if _, err := db.Exec(docdbSchema); err != nil {
		return fmt.Errorf("ensure docdb schema: %w", err)
	}
	return nil
}

// EnsureDocDBSchema ensures DocumentDB tables on an open store.
func (s *Store) EnsureDocDBSchema() error {
	return EnsureDocDBSchema(s.db)
}

// DocDBNestedEndpoint returns the nested-network host:port string (no host publish).
func DocDBNestedEndpoint(dbClusterIdentifier string) string {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	return fmt.Sprintf("%s.docdb.noctaxris.internal:%d", id, DocDBNestedPort)
}

// CreateDocDBCluster creates a DocumentDB cluster control-plane row. Engine must be docdb.
func (s *Store) CreateDocDBCluster(accountID, region, dbClusterIdentifier, engine, engineVersion, masterUsername string, port int) (DocDBCluster, error) {
	_ = region
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	if id == "" {
		return DocDBCluster{}, fmt.Errorf("%w: DBClusterIdentifier is required", ErrDocDBBadRequest)
	}
	if err := validateDocDBClusterID(id); err != nil {
		return DocDBCluster{}, err
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		return DocDBCluster{}, fmt.Errorf("%w: Engine is required", ErrDocDBBadRequest)
	}
	if engine != "docdb" {
		return DocDBCluster{}, fmt.Errorf("%w: Engine must be docdb", ErrDocDBBadRequest)
	}
	if engineVersion == "" {
		engineVersion = "5.0.0"
	}
	if port <= 0 {
		port = DocDBNestedPort
	}
	masterUsername = strings.TrimSpace(masterUsername)
	var existing string
	err := s.db.QueryRow(
		`SELECT db_cluster_identifier FROM docdb_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
		accountID, id,
	).Scan(&existing)
	if err == nil {
		return DocDBCluster{}, ErrDocDBClusterExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return DocDBCluster{}, fmt.Errorf("create docdb cluster: %w", err)
	}
	addr := fmt.Sprintf("%s.docdb.noctaxris.internal", id)
	now := time.Now().UTC().UnixMilli()
	// Match RDS/ElastiCache: stay creating until SetDocDBContainerID after nested start.
	_, err = s.db.Exec(
		`INSERT INTO docdb_clusters
		 (account_id, db_cluster_identifier, engine, engine_version, status, endpoint_address, endpoint_port, master_username, container_id, created_at)
		 VALUES (?, ?, ?, ?, 'creating', ?, ?, ?, '', ?)`,
		accountID, id, engine, engineVersion, addr, port, masterUsername, now,
	)
	if err != nil {
		return DocDBCluster{}, fmt.Errorf("create docdb cluster: insert: %w", err)
	}
	return DocDBCluster{
		DBClusterIdentifier: id,
		Engine:              engine,
		EngineVersion:       engineVersion,
		Status:              "creating",
		EndpointAddress:     addr,
		EndpointPort:        port,
		MasterUsername:      masterUsername,
		CreatedAt:           now,
	}, nil
}

// SetDocDBContainerID records a nested engine container id after dataplane start.
// endpointAddress may be empty to leave the existing nested endpoint unchanged.
func (s *Store) SetDocDBContainerID(accountID, dbClusterIdentifier, containerID, status, endpointAddress string) error {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	if status == "" {
		status = "available"
	}
	var res sql.Result
	var err error
	if strings.TrimSpace(endpointAddress) == "" {
		res, err = s.db.Exec(
			`UPDATE docdb_clusters SET container_id = ?, status = ? WHERE account_id = ? AND db_cluster_identifier = ?`,
			containerID, status, accountID, id,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE docdb_clusters SET container_id = ?, status = ?, endpoint_address = ? WHERE account_id = ? AND db_cluster_identifier = ?`,
			containerID, status, endpointAddress, accountID, id,
		)
	}
	if err != nil {
		return fmt.Errorf("set docdb container: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrDocDBClusterNotFound
	}
	return nil
}

// DescribeDocDBCluster returns one cluster by id.
func (s *Store) DescribeDocDBCluster(accountID, dbClusterIdentifier string) (DocDBCluster, error) {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	var c DocDBCluster
	err := s.db.QueryRow(
		`SELECT db_cluster_identifier, engine, engine_version, status, endpoint_address, endpoint_port, master_username, container_id, created_at
		 FROM docdb_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
		accountID, id,
	).Scan(&c.DBClusterIdentifier, &c.Engine, &c.EngineVersion, &c.Status, &c.EndpointAddress, &c.EndpointPort, &c.MasterUsername, &c.ContainerID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DocDBCluster{}, ErrDocDBClusterNotFound
	}
	if err != nil {
		return DocDBCluster{}, fmt.Errorf("describe docdb cluster: %w", err)
	}
	return c, nil
}

// DescribeDocDBClusters lists clusters (optional id filter).
func (s *Store) DescribeDocDBClusters(accountID, dbClusterIdentifier string) ([]DocDBCluster, error) {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	var (
		rows *sql.Rows
		err  error
	)
	if id == "" {
		rows, err = s.db.Query(
			`SELECT db_cluster_identifier, engine, engine_version, status, endpoint_address, endpoint_port, master_username, container_id, created_at
			 FROM docdb_clusters WHERE account_id = ? ORDER BY db_cluster_identifier`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT db_cluster_identifier, engine, engine_version, status, endpoint_address, endpoint_port, master_username, container_id, created_at
			 FROM docdb_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
			accountID, id,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("describe docdb clusters: %w", err)
	}
	defer rows.Close()
	out := []DocDBCluster{}
	for rows.Next() {
		var c DocDBCluster
		if err := rows.Scan(&c.DBClusterIdentifier, &c.Engine, &c.EngineVersion, &c.Status, &c.EndpointAddress, &c.EndpointPort, &c.MasterUsername, &c.ContainerID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("describe docdb clusters: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteDocDBCluster deletes a cluster by id. Returns container_id if set.
func (s *Store) DeleteDocDBCluster(accountID, dbClusterIdentifier string) (containerID string, err error) {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	err = s.db.QueryRow(
		`SELECT container_id FROM docdb_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
		accountID, id,
	).Scan(&containerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrDocDBClusterNotFound
	}
	if err != nil {
		return "", fmt.Errorf("delete docdb cluster: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM docdb_clusters WHERE account_id = ? AND db_cluster_identifier = ?`, accountID, id)
	if err != nil {
		return "", fmt.Errorf("delete docdb cluster: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", ErrDocDBClusterNotFound
	}
	return containerID, nil
}

func validateDocDBClusterID(id string) error {
	if len(id) < 1 || len(id) > 63 {
		return fmt.Errorf("%w: DBClusterIdentifier length must be 1-63", ErrDocDBBadRequest)
	}
	if id[0] < 'a' || id[0] > 'z' {
		return fmt.Errorf("%w: DBClusterIdentifier must start with a letter", ErrDocDBBadRequest)
	}
	if strings.HasSuffix(id, "-") || strings.Contains(id, "--") {
		return fmt.Errorf("%w: DBClusterIdentifier cannot end with hyphen or contain consecutive hyphens", ErrDocDBBadRequest)
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("%w: DBClusterIdentifier must be alphanumeric or hyphen", ErrDocDBBadRequest)
	}
	return nil
}
