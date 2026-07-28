package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	ErrNeptuneClusterExists   = errors.New("DBClusterAlreadyExistsFault")
	ErrNeptuneClusterNotFound = errors.New("DBClusterNotFoundFault")
	ErrNeptuneBadRequest      = errors.New("InvalidParameterValue")
)

const DefaultNeptuneRegion = "us-east-1"

// Nested Neptune Gremlin Server port inside the DinD network (never published on the host by default).
const NeptuneNestedPort = 8182

// Nested Neptune Neo4j Bolt port inside the DinD network (never published on the host by default).
const NeptuneNestedBoltPort = 7687

// EnvNeptuneEngine selects the nested graph backend (gremlin default, neo4j opt-in).
const EnvNeptuneEngine = "NOCTAXRIS_NEPTUNE_ENGINE"

// Nested graph backend identifiers (AWS Engine remains "neptune").
const (
	NeptuneGraphEngineGremlin = "gremlin"
	NeptuneGraphEngineNeo4j   = "neo4j"
)

const neptuneSchema = `
CREATE TABLE IF NOT EXISTS neptune_clusters (
  account_id TEXT NOT NULL,
  db_cluster_identifier TEXT NOT NULL,
  engine TEXT NOT NULL,
  engine_version TEXT NOT NULL DEFAULT '',
  graph_engine TEXT NOT NULL DEFAULT 'gremlin',
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
	GraphEngine         string
	Status              string
	EndpointAddress     string
	EndpointPort        int
	ContainerID         string
	CreatedAt           int64
}

// EnsureNeptuneSchema creates Neptune tables if missing and migrates columns.
func EnsureNeptuneSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure neptune schema: db is nil")
	}
	if _, err := db.Exec(neptuneSchema); err != nil {
		return fmt.Errorf("ensure neptune schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE neptune_clusters ADD COLUMN graph_engine TEXT NOT NULL DEFAULT 'gremlin'`,
	}); err != nil {
		return fmt.Errorf("ensure neptune schema: migrate: %w", err)
	}
	return nil
}

// EnsureNeptuneSchema ensures Neptune tables on an open store.
func (s *Store) EnsureNeptuneSchema() error {
	return EnsureNeptuneSchema(s.db)
}

// NormalizeNeptuneGraphEngine maps config/param/tag values to gremlin or neo4j.
// Empty defaults to gremlin. Unknown values fail closed.
func NormalizeNeptuneGraphEngine(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "gremlin", "tinkerpop":
		return NeptuneGraphEngineGremlin, nil
	case "neo4j", "opencypher", "cypher", "bolt":
		return NeptuneGraphEngineNeo4j, nil
	default:
		return "", fmt.Errorf("%w: Neptune graph engine must be gremlin or neo4j", ErrNeptuneBadRequest)
	}
}

// NeptuneGraphEngineFromEnv reads NOCTAXRIS_NEPTUNE_ENGINE (default gremlin; unknown fails closed).
func NeptuneGraphEngineFromEnv() (string, error) {
	return NormalizeNeptuneGraphEngine(os.Getenv(EnvNeptuneEngine))
}

// NeptunePortForGraphEngine returns the nested listen port for a graph backend.
func NeptunePortForGraphEngine(graphEngine string) int {
	ge, err := NormalizeNeptuneGraphEngine(graphEngine)
	if err == nil && ge == NeptuneGraphEngineNeo4j {
		return NeptuneNestedBoltPort
	}
	return NeptuneNestedPort
}

// NeptuneNestedEndpoint returns the nested-network host:port string (no host publish).
func NeptuneNestedEndpoint(dbClusterIdentifier, graphEngine string) string {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	return fmt.Sprintf("%s.neptune.noctaxris.internal:%d", id, NeptunePortForGraphEngine(graphEngine))
}

// CreateNeptuneCluster creates a Neptune cluster control-plane row. Engine must be neptune.
// graphEngine selects nested Gremlin (default) or Neo4j; empty uses Normalize defaults.
func (s *Store) CreateNeptuneCluster(accountID, region, dbClusterIdentifier, engine, engineVersion, graphEngine string, port int) (NeptuneCluster, error) {
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
	ge, err := NormalizeNeptuneGraphEngine(graphEngine)
	if err != nil {
		return NeptuneCluster{}, err
	}
	if engineVersion == "" {
		engineVersion = "1.3.0.0"
	}
	if port <= 0 {
		port = NeptunePortForGraphEngine(ge)
	}
	var existing string
	err = s.db.QueryRow(
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
		 (account_id, db_cluster_identifier, engine, engine_version, graph_engine, status, endpoint_address, endpoint_port, container_id, created_at)
		 VALUES (?, ?, ?, ?, ?, 'creating', ?, ?, '', ?)`,
		accountID, id, engine, engineVersion, ge, addr, port, now,
	)
	if err != nil {
		return NeptuneCluster{}, fmt.Errorf("create neptune cluster: insert: %w", err)
	}
	return NeptuneCluster{
		DBClusterIdentifier: id,
		Engine:              engine,
		EngineVersion:       engineVersion,
		GraphEngine:         ge,
		Status:              "creating",
		EndpointAddress:     addr,
		EndpointPort:        port,
		CreatedAt:           now,
	}, nil
}

// SetNeptuneContainerID records a nested graph container id after dataplane start.
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

const neptuneSelectCols = `db_cluster_identifier, engine, engine_version, graph_engine, status, endpoint_address, endpoint_port, container_id, created_at`

func scanNeptuneCluster(scan func(dest ...any) error) (NeptuneCluster, error) {
	var c NeptuneCluster
	err := scan(&c.DBClusterIdentifier, &c.Engine, &c.EngineVersion, &c.GraphEngine, &c.Status, &c.EndpointAddress, &c.EndpointPort, &c.ContainerID, &c.CreatedAt)
	if err != nil {
		return NeptuneCluster{}, err
	}
	if c.GraphEngine == "" {
		c.GraphEngine = NeptuneGraphEngineGremlin
	}
	return c, nil
}

// DescribeNeptuneCluster returns one cluster by id.
func (s *Store) DescribeNeptuneCluster(accountID, dbClusterIdentifier string) (NeptuneCluster, error) {
	id := strings.ToLower(strings.TrimSpace(dbClusterIdentifier))
	c, err := scanNeptuneCluster(func(dest ...any) error {
		return s.db.QueryRow(
			`SELECT `+neptuneSelectCols+` FROM neptune_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
			accountID, id,
		).Scan(dest...)
	})
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
			`SELECT `+neptuneSelectCols+` FROM neptune_clusters WHERE account_id = ? ORDER BY db_cluster_identifier`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT `+neptuneSelectCols+` FROM neptune_clusters WHERE account_id = ? AND db_cluster_identifier = ?`,
			accountID, id,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("describe neptune clusters: %w", err)
	}
	defer rows.Close()
	out := []NeptuneCluster{}
	for rows.Next() {
		c, err := scanNeptuneCluster(rows.Scan)
		if err != nil {
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
