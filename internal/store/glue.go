package store

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

var (
	ErrGlueNotFound      = errors.New("EntityNotFoundException")
	ErrGlueAlreadyExists = errors.New("AlreadyExistsException")
	ErrGlueBadRequest    = errors.New("InvalidInputException")
)

const DefaultGlueRegion = "us-east-1"

const glueSchema = `
CREATE TABLE IF NOT EXISTS glue_databases (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS glue_tables (
  account_id TEXT NOT NULL,
  database_name TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  storage_location TEXT NOT NULL DEFAULT '',
  columns_json TEXT NOT NULL DEFAULT '[]',
  partition_keys_json TEXT NOT NULL DEFAULT '[]',
  input_format TEXT NOT NULL DEFAULT '',
  output_format TEXT NOT NULL DEFAULT '',
  serde_name TEXT NOT NULL DEFAULT '',
  serde_library TEXT NOT NULL DEFAULT '',
  serde_params_json TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, database_name, name)
);
CREATE TABLE IF NOT EXISTS glue_crawlers (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  database_name TEXT NOT NULL,
  targets_json TEXT NOT NULL DEFAULT '[]',
  state TEXT NOT NULL DEFAULT 'READY',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
`

// GlueDatabase is a Data Catalog database.
type GlueDatabase struct {
	Name        string
	Description string
	CreatedAt   int64
}

// GlueColumn is a table column or partition key.
type GlueColumn struct {
	Name string `json:"Name"`
	Type string `json:"Type"`
}

// GlueSerDeInfo is a StorageDescriptor SerDeInfo subset for Athena.
type GlueSerDeInfo struct {
	Name                 string            `json:"Name"`
	SerializationLibrary string            `json:"SerializationLibrary"`
	Parameters           map[string]string `json:"Parameters"`
}

// GlueTable is a Data Catalog table.
type GlueTable struct {
	DatabaseName    string
	Name            string
	Description     string
	StorageLocation string
	Columns         []GlueColumn
	PartitionKeys   []GlueColumn
	InputFormat     string
	OutputFormat    string
	SerDeInfo       GlueSerDeInfo
	CreatedAt       int64
	UpdatedAt       int64
}

// GlueTableCreate is CreateTable input fields.
type GlueTableCreate struct {
	DatabaseName    string
	Name            string
	Description     string
	StorageLocation string
	Columns         []GlueColumn
	PartitionKeys   []GlueColumn
	InputFormat     string
	OutputFormat    string
	SerDeInfo       GlueSerDeInfo
}

// EnsureGlueSchema creates Glue catalog tables if missing.
func EnsureGlueSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure glue schema: db is nil")
	}
	if _, err := db.Exec(glueSchema); err != nil {
		return fmt.Errorf("ensure glue schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE glue_tables ADD COLUMN partition_keys_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE glue_tables ADD COLUMN input_format TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE glue_tables ADD COLUMN output_format TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE glue_tables ADD COLUMN serde_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE glue_tables ADD COLUMN serde_library TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE glue_tables ADD COLUMN serde_params_json TEXT NOT NULL DEFAULT '{}'`,
	}); err != nil {
		return fmt.Errorf("ensure glue schema alter: %w", err)
	}
	return nil
}

// EnsureGlueSchema ensures Glue tables on an open store.
func (s *Store) EnsureGlueSchema() error {
	return EnsureGlueSchema(s.db)
}

// CreateGlueDatabase creates a catalog database.
func (s *Store) CreateGlueDatabase(accountID, name, description string) (GlueDatabase, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return GlueDatabase{}, fmt.Errorf("%w: Name required", ErrGlueBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO glue_databases (account_id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		accountID, name, description, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return GlueDatabase{}, ErrGlueAlreadyExists
		}
		return GlueDatabase{}, fmt.Errorf("create database: %w", err)
	}
	return GlueDatabase{Name: name, Description: description, CreatedAt: now}, nil
}

// GetGlueDatabase returns a database.
func (s *Store) GetGlueDatabase(accountID, name string) (GlueDatabase, error) {
	var d GlueDatabase
	err := s.db.QueryRow(
		`SELECT name, description, created_at FROM glue_databases WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&d.Name, &d.Description, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GlueDatabase{}, ErrGlueNotFound
	}
	if err != nil {
		return GlueDatabase{}, fmt.Errorf("get database: %w", err)
	}
	return d, nil
}

// GetGlueDatabases lists databases.
func (s *Store) GetGlueDatabases(accountID string) ([]GlueDatabase, error) {
	rows, err := s.db.Query(
		`SELECT name, description, created_at FROM glue_databases WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("get databases: %w", err)
	}
	defer rows.Close()
	var out []GlueDatabase
	for rows.Next() {
		var d GlueDatabase
		if err := rows.Scan(&d.Name, &d.Description, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("get databases scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteGlueDatabase deletes a database and its tables.
func (s *Store) DeleteGlueDatabase(accountID, name string) error {
	if _, err := s.GetGlueDatabase(accountID, name); err != nil {
		return err
	}
	_, _ = s.db.Exec(`DELETE FROM glue_tables WHERE account_id = ? AND database_name = ?`, accountID, name)
	_, err := s.db.Exec(`DELETE FROM glue_databases WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete database: %w", err)
	}
	return nil
}

// CreateGlueTable creates a table under a database.
func (s *Store) CreateGlueTable(accountID string, in GlueTableCreate) (GlueTable, error) {
	databaseName := strings.TrimSpace(in.DatabaseName)
	name := strings.TrimSpace(in.Name)
	if databaseName == "" || name == "" {
		return GlueTable{}, fmt.Errorf("%w: DatabaseName and Name required", ErrGlueBadRequest)
	}
	if _, err := s.GetGlueDatabase(accountID, databaseName); err != nil {
		return GlueTable{}, err
	}
	columns := in.Columns
	if columns == nil {
		columns = []GlueColumn{}
	}
	partitionKeys := in.PartitionKeys
	if partitionKeys == nil {
		partitionKeys = []GlueColumn{}
	}
	serde := in.SerDeInfo
	if serde.Parameters == nil {
		serde.Parameters = map[string]string{}
	}
	colJSON, err := json.Marshal(columns)
	if err != nil {
		return GlueTable{}, fmt.Errorf("marshal columns: %w", err)
	}
	pkJSON, err := json.Marshal(partitionKeys)
	if err != nil {
		return GlueTable{}, fmt.Errorf("marshal partition keys: %w", err)
	}
	serdeParamsJSON, err := json.Marshal(serde.Parameters)
	if err != nil {
		return GlueTable{}, fmt.Errorf("marshal serde params: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO glue_tables (
		   account_id, database_name, name, description, storage_location, columns_json,
		   partition_keys_json, input_format, output_format, serde_name, serde_library, serde_params_json,
		   created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, databaseName, name, in.Description, in.StorageLocation, string(colJSON),
		string(pkJSON), in.InputFormat, in.OutputFormat, serde.Name, serde.SerializationLibrary, string(serdeParamsJSON),
		now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return GlueTable{}, ErrGlueAlreadyExists
		}
		return GlueTable{}, fmt.Errorf("create table: %w", err)
	}
	return GlueTable{
		DatabaseName: databaseName, Name: name, Description: in.Description,
		StorageLocation: in.StorageLocation, Columns: columns, PartitionKeys: partitionKeys,
		InputFormat: in.InputFormat, OutputFormat: in.OutputFormat, SerDeInfo: serde,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func scanGlueTable(
	databaseName, name, description, location, colJSON, pkJSON, inputFmt, outputFmt, serdeName, serdeLib, serdeParamsJSON string,
	createdAt, updatedAt int64,
) GlueTable {
	t := GlueTable{
		DatabaseName: databaseName, Name: name, Description: description,
		StorageLocation: location, InputFormat: inputFmt, OutputFormat: outputFmt,
		SerDeInfo: GlueSerDeInfo{Name: serdeName, SerializationLibrary: serdeLib, Parameters: map[string]string{}},
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	_ = json.Unmarshal([]byte(colJSON), &t.Columns)
	_ = json.Unmarshal([]byte(pkJSON), &t.PartitionKeys)
	_ = json.Unmarshal([]byte(serdeParamsJSON), &t.SerDeInfo.Parameters)
	if t.Columns == nil {
		t.Columns = []GlueColumn{}
	}
	if t.PartitionKeys == nil {
		t.PartitionKeys = []GlueColumn{}
	}
	if t.SerDeInfo.Parameters == nil {
		t.SerDeInfo.Parameters = map[string]string{}
	}
	return t
}

// GetGlueTable returns a table.
func (s *Store) GetGlueTable(accountID, databaseName, name string) (GlueTable, error) {
	var (
		dbName, tblName, desc, location, colJSON, pkJSON string
		inputFmt, outputFmt, serdeName, serdeLib, serdeParamsJSON string
		createdAt, updatedAt int64
	)
	err := s.db.QueryRow(
		`SELECT database_name, name, description, storage_location, columns_json,
		        partition_keys_json, input_format, output_format, serde_name, serde_library, serde_params_json,
		        created_at, updated_at
		 FROM glue_tables WHERE account_id = ? AND database_name = ? AND name = ?`,
		accountID, databaseName, name,
	).Scan(&dbName, &tblName, &desc, &location, &colJSON, &pkJSON, &inputFmt, &outputFmt, &serdeName, &serdeLib, &serdeParamsJSON, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GlueTable{}, ErrGlueNotFound
	}
	if err != nil {
		return GlueTable{}, fmt.Errorf("get table: %w", err)
	}
	return scanGlueTable(dbName, tblName, desc, location, colJSON, pkJSON, inputFmt, outputFmt, serdeName, serdeLib, serdeParamsJSON, createdAt, updatedAt), nil
}

// GetGlueTables lists tables in a database.
func (s *Store) GetGlueTables(accountID, databaseName string) ([]GlueTable, error) {
	if _, err := s.GetGlueDatabase(accountID, databaseName); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT database_name, name, description, storage_location, columns_json,
		        partition_keys_json, input_format, output_format, serde_name, serde_library, serde_params_json,
		        created_at, updated_at
		 FROM glue_tables WHERE account_id = ? AND database_name = ? ORDER BY name`,
		accountID, databaseName,
	)
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	defer rows.Close()
	var out []GlueTable
	for rows.Next() {
		var (
			dbName, tblName, desc, location, colJSON, pkJSON string
			inputFmt, outputFmt, serdeName, serdeLib, serdeParamsJSON string
			createdAt, updatedAt int64
		)
		if err := rows.Scan(&dbName, &tblName, &desc, &location, &colJSON, &pkJSON, &inputFmt, &outputFmt, &serdeName, &serdeLib, &serdeParamsJSON, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("get tables scan: %w", err)
		}
		out = append(out, scanGlueTable(dbName, tblName, desc, location, colJSON, pkJSON, inputFmt, outputFmt, serdeName, serdeLib, serdeParamsJSON, createdAt, updatedAt))
	}
	return out, rows.Err()
}

// GlueS3Target is an S3 path target for a crawler.
type GlueS3Target struct {
	Path string `json:"Path"`
}

// GlueCrawler is a lab Glue crawler record.
type GlueCrawler struct {
	Name         string
	Role         string
	DatabaseName string
	Targets      []GlueS3Target
	State        string
	CreatedAt    int64
	UpdatedAt    int64
}

// GlueCrawlerCreate is CreateCrawler input.
type GlueCrawlerCreate struct {
	Name         string
	Role         string
	DatabaseName string
	Targets      []GlueS3Target
}

const (
	glueCrawlerStateReady   = "READY"
	glueCrawlerStateRunning = "RUNNING"
)

// CreateGlueCrawler registers a crawler.
func (s *Store) CreateGlueCrawler(accountID string, in GlueCrawlerCreate) (GlueCrawler, error) {
	name := strings.TrimSpace(in.Name)
	databaseName := strings.TrimSpace(in.DatabaseName)
	if name == "" || databaseName == "" {
		return GlueCrawler{}, fmt.Errorf("%w: Name and DatabaseName required", ErrGlueBadRequest)
	}
	if len(in.Targets) == 0 {
		return GlueCrawler{}, fmt.Errorf("%w: at least one S3 target required", ErrGlueBadRequest)
	}
	for _, tgt := range in.Targets {
		if _, _, err := parseS3Location(tgt.Path); err != nil {
			return GlueCrawler{}, fmt.Errorf("%w: invalid S3 target path", ErrGlueBadRequest)
		}
	}
	if _, err := s.GetGlueDatabase(accountID, databaseName); err != nil {
		return GlueCrawler{}, err
	}
	targets := in.Targets
	if targets == nil {
		targets = []GlueS3Target{}
	}
	targetsJSON, err := json.Marshal(targets)
	if err != nil {
		return GlueCrawler{}, fmt.Errorf("marshal targets: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO glue_crawlers (account_id, name, role_arn, database_name, targets_json, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, strings.TrimSpace(in.Role), databaseName, string(targetsJSON), glueCrawlerStateReady, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return GlueCrawler{}, ErrGlueAlreadyExists
		}
		return GlueCrawler{}, fmt.Errorf("create crawler: %w", err)
	}
	return GlueCrawler{
		Name: name, Role: strings.TrimSpace(in.Role), DatabaseName: databaseName,
		Targets: targets, State: glueCrawlerStateReady, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func scanGlueCrawler(name, role, databaseName, targetsJSON, state string, createdAt, updatedAt int64) GlueCrawler {
	cr := GlueCrawler{
		Name: name, Role: role, DatabaseName: databaseName, State: state,
		CreatedAt: createdAt, UpdatedAt: updatedAt, Targets: []GlueS3Target{},
	}
	_ = json.Unmarshal([]byte(targetsJSON), &cr.Targets)
	if cr.Targets == nil {
		cr.Targets = []GlueS3Target{}
	}
	return cr
}

// GetGlueCrawler returns a crawler.
func (s *Store) GetGlueCrawler(accountID, name string) (GlueCrawler, error) {
	var role, databaseName, targetsJSON, state string
	var createdAt, updatedAt int64
	err := s.db.QueryRow(
		`SELECT name, role_arn, database_name, targets_json, state, created_at, updated_at
		 FROM glue_crawlers WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&name, &role, &databaseName, &targetsJSON, &state, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GlueCrawler{}, ErrGlueNotFound
	}
	if err != nil {
		return GlueCrawler{}, fmt.Errorf("get crawler: %w", err)
	}
	return scanGlueCrawler(name, role, databaseName, targetsJSON, state, createdAt, updatedAt), nil
}

// ListGlueCrawlers lists crawlers for an account.
func (s *Store) ListGlueCrawlers(accountID string) ([]GlueCrawler, error) {
	rows, err := s.db.Query(
		`SELECT name, role_arn, database_name, targets_json, state, created_at, updated_at
		 FROM glue_crawlers WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list crawlers: %w", err)
	}
	defer rows.Close()
	var out []GlueCrawler
	for rows.Next() {
		var name, role, databaseName, targetsJSON, state string
		var createdAt, updatedAt int64
		if err := rows.Scan(&name, &role, &databaseName, &targetsJSON, &state, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("list crawlers scan: %w", err)
		}
		out = append(out, scanGlueCrawler(name, role, databaseName, targetsJSON, state, createdAt, updatedAt))
	}
	return out, rows.Err()
}

// DeleteGlueCrawler deletes a crawler.
func (s *Store) DeleteGlueCrawler(accountID, name string) error {
	res, err := s.db.Exec(`DELETE FROM glue_crawlers WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete crawler: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrGlueNotFound
	}
	return nil
}

func (s *Store) setGlueCrawlerState(accountID, name, state string) error {
	now := time.Now().UTC().UnixMilli()
	res, err := s.db.Exec(
		`UPDATE glue_crawlers SET state = ?, updated_at = ? WHERE account_id = ? AND name = ?`,
		state, now, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("set crawler state: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrGlueNotFound
	}
	return nil
}

// StartGlueCrawler runs a synchronous crawl: READY → RUNNING → READY.
func (s *Store) StartGlueCrawler(accountID, name string) (GlueCrawler, error) {
	cr, err := s.GetGlueCrawler(accountID, name)
	if err != nil {
		return GlueCrawler{}, err
	}
	if cr.State == glueCrawlerStateRunning {
		return GlueCrawler{}, fmt.Errorf("%w: crawler is already running", ErrGlueBadRequest)
	}
	if err := s.setGlueCrawlerState(accountID, name, glueCrawlerStateRunning); err != nil {
		return GlueCrawler{}, err
	}
	crawlErr := s.runGlueCrawler(accountID, cr)
	_ = s.setGlueCrawlerState(accountID, name, glueCrawlerStateReady)
	if crawlErr != nil {
		return GlueCrawler{}, crawlErr
	}
	return s.GetGlueCrawler(accountID, name)
}

func (s *Store) runGlueCrawler(accountID string, cr GlueCrawler) error {
	for _, tgt := range cr.Targets {
		bucket, prefix, err := parseS3Location(tgt.Path)
		if err != nil {
			return err
		}
		listed, err := s.ListObjectsV2(accountID, bucket, prefix, "")
		if err != nil {
			if errors.Is(err, ErrNoSuchBucket) {
				return fmt.Errorf("%w: S3 bucket does not exist: %s", ErrGlueBadRequest, bucket)
			}
			return err
		}
		for _, obj := range listed.Contents {
			if err := s.crawlS3Object(accountID, cr.DatabaseName, bucket, prefix, obj.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) crawlS3Object(accountID, databaseName, bucket, targetPrefix, key string) error {
	_, data, err := s.GetObject(accountID, bucket, key)
	if err != nil {
		return fmt.Errorf("get object s3://%s/%s: %w", bucket, key, err)
	}
	if len(data) == 0 {
		return nil
	}
	format := detectCrawlDataFormat(key, data)
	var columns []GlueColumn
	var in GlueTableCreate
	switch format {
	case "json":
		columns, err = inferJSONCrawlColumns(data)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrGlueBadRequest, err)
		}
		in = GlueTableCreate{
			DatabaseName:    databaseName,
			Name:            tableNameFromObjectKey(key),
			StorageLocation: crawlStorageLocation(bucket, targetPrefix, key),
			Columns:         columns,
			InputFormat:     "org.apache.hive.hcatalog.data.JsonSerDe",
			SerDeInfo: GlueSerDeInfo{
				SerializationLibrary: "org.openx.data.jsonserde.JsonSerDe",
				Parameters:           map[string]string{},
			},
		}
	default:
		columns, err = inferCSVCrawlColumns(data)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrGlueBadRequest, err)
		}
		in = GlueTableCreate{
			DatabaseName:    databaseName,
			Name:            tableNameFromObjectKey(key),
			StorageLocation: crawlStorageLocation(bucket, targetPrefix, key),
			Columns:         columns,
			InputFormat:     "org.apache.hadoop.mapred.TextInputFormat",
			OutputFormat:    "org.apache.hadoop.hive.ql.io.HiveIgnoreKeyTextOutputFormat",
			SerDeInfo: GlueSerDeInfo{
				SerializationLibrary: "org.apache.hadoop.hive.serde2.lazy.LazySimpleSerDe",
				Parameters:           map[string]string{"field.delim": ","},
			},
		}
	}
	return s.upsertGlueCrawlTable(accountID, in)
}

func crawlStorageLocation(bucket, targetPrefix, key string) string {
	locPrefix := targetPrefix
	if locPrefix != "" && !strings.HasSuffix(locPrefix, "/") {
		locPrefix += "/"
	}
	if locPrefix == "" || strings.HasPrefix(key, locPrefix) {
		return "s3://" + bucket + "/" + locPrefix
	}
	dir := path.Dir(key)
	if dir == "." {
		return "s3://" + bucket + "/"
	}
	return "s3://" + bucket + "/" + dir + "/"
}

func tableNameFromObjectKey(key string) string {
	base := path.Base(key)
	if dot := strings.LastIndex(base, "."); dot > 0 {
		base = base[:dot]
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return "data"
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "data"
	}
	return out
}

func detectCrawlDataFormat(key string, data []byte) string {
	lk := strings.ToLower(key)
	if strings.HasSuffix(lk, ".json") || strings.HasSuffix(lk, ".jsonl") {
		return "json"
	}
	if strings.HasSuffix(lk, ".csv") {
		return "csv"
	}
	trimmed := strings.TrimLeft(string(data), " \t\r\n")
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return "json"
	}
	return "csv"
}

func inferCSVCrawlColumns(data []byte) ([]GlueColumn, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("csv header: %w", err)
	}
	if len(header) == 0 {
		return nil, errors.New("csv header empty")
	}
	cols := make([]GlueColumn, 0, len(header))
	for _, h := range header {
		name := strings.TrimSpace(h)
		if name == "" {
			continue
		}
		cols = append(cols, GlueColumn{Name: name, Type: "string"})
	}
	if len(cols) == 0 {
		return nil, errors.New("csv header has no column names")
	}
	return cols, nil
}

func inferJSONCrawlColumns(data []byte) ([]GlueColumn, error) {
	line := firstNonEmptyLine(data)
	if line == "" {
		return nil, errors.New("json object empty")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return nil, fmt.Errorf("json object: %w", err)
	}
	if len(obj) == 0 {
		return nil, errors.New("json object has no keys")
	}
	names := make([]string, 0, len(obj))
	for k := range obj {
		names = append(names, k)
	}
	sort.Strings(names)
	cols := make([]GlueColumn, 0, len(names))
	for _, name := range names {
		cols = append(cols, GlueColumn{Name: name, Type: "string"})
	}
	return cols, nil
}

func firstNonEmptyLine(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func (s *Store) upsertGlueCrawlTable(accountID string, in GlueTableCreate) error {
	_, err := s.GetGlueTable(accountID, in.DatabaseName, in.Name)
	if errors.Is(err, ErrGlueNotFound) {
		_, err = s.CreateGlueTable(accountID, in)
		return err
	}
	if err != nil {
		return err
	}
	return s.updateGlueTableFromCrawl(accountID, in)
}

func (s *Store) updateGlueTableFromCrawl(accountID string, in GlueTableCreate) error {
	columns := in.Columns
	if columns == nil {
		columns = []GlueColumn{}
	}
	serde := in.SerDeInfo
	if serde.Parameters == nil {
		serde.Parameters = map[string]string{}
	}
	colJSON, err := json.Marshal(columns)
	if err != nil {
		return fmt.Errorf("marshal columns: %w", err)
	}
	serdeParamsJSON, err := json.Marshal(serde.Parameters)
	if err != nil {
		return fmt.Errorf("marshal serde params: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	res, err := s.db.Exec(
		`UPDATE glue_tables SET storage_location = ?, columns_json = ?, input_format = ?, output_format = ?,
		        serde_name = ?, serde_library = ?, serde_params_json = ?, updated_at = ?
		 WHERE account_id = ? AND database_name = ? AND name = ?`,
		in.StorageLocation, string(colJSON), in.InputFormat, in.OutputFormat,
		serde.Name, serde.SerializationLibrary, string(serdeParamsJSON), now,
		accountID, in.DatabaseName, in.Name,
	)
	if err != nil {
		return fmt.Errorf("update table from crawl: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrGlueNotFound
	}
	return nil
}

// DeleteGlueTable deletes a table.
func (s *Store) DeleteGlueTable(accountID, databaseName, name string) error {
	res, err := s.db.Exec(
		`DELETE FROM glue_tables WHERE account_id = ? AND database_name = ? AND name = ?`,
		accountID, databaseName, name,
	)
	if err != nil {
		return fmt.Errorf("delete table: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrGlueNotFound
	}
	return nil
}
