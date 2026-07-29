package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	glueDefaultRegistryName = "default-registry"
	glueSchemaStatusAvail   = "AVAILABLE"
	glueVersionStatusAvail  = "AVAILABLE"
)

const glueSchemaRegistryDDL = `
CREATE TABLE IF NOT EXISTS glue_registries (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  registry_arn TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS glue_schemas (
  account_id TEXT NOT NULL,
  registry_name TEXT NOT NULL,
  schema_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  data_format TEXT NOT NULL,
  compatibility TEXT NOT NULL DEFAULT 'NONE',
  schema_arn TEXT NOT NULL DEFAULT '',
  schema_status TEXT NOT NULL DEFAULT 'AVAILABLE',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, registry_name, schema_name)
);
CREATE TABLE IF NOT EXISTS glue_schema_versions (
  account_id TEXT NOT NULL,
  schema_version_id TEXT NOT NULL,
  registry_name TEXT NOT NULL,
  schema_name TEXT NOT NULL,
  version_number INTEGER NOT NULL,
  definition TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'AVAILABLE',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, schema_version_id),
  UNIQUE (account_id, registry_name, schema_name, version_number)
);
`

// GlueRegistry is a Schema Registry registry.
type GlueRegistry struct {
	Name        string
	Description string
	RegistryARN string
	CreatedAt   int64
	UpdatedAt   int64
}

// GlueSchema is a Schema Registry schema set.
type GlueSchema struct {
	RegistryName        string
	SchemaName          string
	Description         string
	DataFormat          string
	Compatibility       string
	SchemaARN           string
	SchemaStatus        string
	LatestSchemaVersion int64
	NextSchemaVersion   int64
	CreatedAt           int64
	UpdatedAt           int64
}

// GlueSchemaVersion is one registered schema definition version.
type GlueSchemaVersion struct {
	SchemaVersionID string
	RegistryName    string
	SchemaName      string
	VersionNumber   int64
	Definition      string
	DataFormat      string
	Status          string
	CreatedAt       int64
}

// GlueSchemaReference points a catalog table at a registry schema version.
type GlueSchemaReference struct {
	RegistryName        string `json:"RegistryName,omitempty"`
	SchemaName          string `json:"SchemaName,omitempty"`
	SchemaVersionNumber int64  `json:"SchemaVersionNumber,omitempty"`
	SchemaVersionID     string `json:"SchemaVersionId,omitempty"`
}

// GlueCreateSchemaResult is CreateSchema output.
type GlueCreateSchemaResult struct {
	Schema          GlueSchema
	SchemaVersionID string
	VersionNumber   int64
	VersionStatus   string
}

func glueRegistryARN(accountID, region, name string) string {
	if region == "" {
		region = DefaultGlueRegion
	}
	return "arn:aws:glue:" + region + ":" + accountID + ":registry/" + name
}

func glueSchemaARN(accountID, region, registryName, schemaName string) string {
	if region == "" {
		region = DefaultGlueRegion
	}
	return "arn:aws:glue:" + region + ":" + accountID + ":schema/" + registryName + "/" + schemaName
}

func ensureGlueSchemaRegistryTables(db *sql.DB) error {
	if _, err := db.Exec(glueSchemaRegistryDDL); err != nil {
		return fmt.Errorf("ensure glue schema registry: %w", err)
	}
	return nil
}

// CreateGlueRegistry creates a schema registry.
func (s *Store) CreateGlueRegistry(accountID, region, name, description string) (GlueRegistry, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return GlueRegistry{}, fmt.Errorf("%w: RegistryName required", ErrGlueBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	arn := glueRegistryARN(accountID, region, name)
	_, err := s.db.Exec(
		`INSERT INTO glue_registries (account_id, name, description, registry_arn, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, name, description, arn, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return GlueRegistry{}, ErrGlueAlreadyExists
		}
		return GlueRegistry{}, fmt.Errorf("create registry: %w", err)
	}
	return GlueRegistry{Name: name, Description: description, RegistryARN: arn, CreatedAt: now, UpdatedAt: now}, nil
}

// GetGlueRegistry returns a registry by name.
func (s *Store) GetGlueRegistry(accountID, name string) (GlueRegistry, error) {
	name = strings.TrimSpace(name)
	var r GlueRegistry
	err := s.db.QueryRow(
		`SELECT name, description, registry_arn, created_at, updated_at
		 FROM glue_registries WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&r.Name, &r.Description, &r.RegistryARN, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GlueRegistry{}, ErrGlueNotFound
	}
	if err != nil {
		return GlueRegistry{}, fmt.Errorf("get registry: %w", err)
	}
	return r, nil
}

// ListGlueRegistries lists registries for an account.
func (s *Store) ListGlueRegistries(accountID string) ([]GlueRegistry, error) {
	rows, err := s.db.Query(
		`SELECT name, description, registry_arn, created_at, updated_at
		 FROM glue_registries WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list registries: %w", err)
	}
	defer rows.Close()
	var out []GlueRegistry
	for rows.Next() {
		var r GlueRegistry
		if err := rows.Scan(&r.Name, &r.Description, &r.RegistryARN, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list registries scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteGlueRegistry deletes a registry and its schemas/versions.
func (s *Store) DeleteGlueRegistry(accountID, name string) error {
	if _, err := s.GetGlueRegistry(accountID, name); err != nil {
		return err
	}
	_, _ = s.db.Exec(
		`DELETE FROM glue_schema_versions WHERE account_id = ? AND registry_name = ?`,
		accountID, name,
	)
	_, _ = s.db.Exec(
		`DELETE FROM glue_schemas WHERE account_id = ? AND registry_name = ?`,
		accountID, name,
	)
	_, err := s.db.Exec(`DELETE FROM glue_registries WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete registry: %w", err)
	}
	return nil
}

func (s *Store) ensureGlueRegistry(accountID, region, name string) (GlueRegistry, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = glueDefaultRegistryName
	}
	reg, err := s.GetGlueRegistry(accountID, name)
	if err == nil {
		return reg, nil
	}
	if !errors.Is(err, ErrGlueNotFound) {
		return GlueRegistry{}, err
	}
	reg, err = s.CreateGlueRegistry(accountID, region, name, "")
	if errors.Is(err, ErrGlueAlreadyExists) {
		return s.GetGlueRegistry(accountID, name)
	}
	return reg, err
}

func normalizeGlueDataFormat(format string) (string, error) {
	format = strings.ToUpper(strings.TrimSpace(format))
	switch format {
	case "AVRO", "JSON":
		return format, nil
	case "PROTOBUF":
		return "", fmt.Errorf("%w: PROTOBUF schemas are not supported in this lab", ErrGlueBadRequest)
	default:
		return "", fmt.Errorf("%w: DataFormat must be AVRO or JSON", ErrGlueBadRequest)
	}
}

func (s *Store) latestGlueSchemaVersionNumber(accountID, registryName, schemaName string) (int64, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(version_number) FROM glue_schema_versions
		 WHERE account_id = ? AND registry_name = ? AND schema_name = ?`,
		accountID, registryName, schemaName,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("latest schema version: %w", err)
	}
	if !n.Valid {
		return 0, nil
	}
	return n.Int64, nil
}

func (s *Store) loadGlueSchema(accountID, registryName, schemaName string) (GlueSchema, error) {
	var sc GlueSchema
	err := s.db.QueryRow(
		`SELECT registry_name, schema_name, description, data_format, compatibility, schema_arn, schema_status, created_at, updated_at
		 FROM glue_schemas WHERE account_id = ? AND registry_name = ? AND schema_name = ?`,
		accountID, registryName, schemaName,
	).Scan(
		&sc.RegistryName, &sc.SchemaName, &sc.Description, &sc.DataFormat, &sc.Compatibility,
		&sc.SchemaARN, &sc.SchemaStatus, &sc.CreatedAt, &sc.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return GlueSchema{}, ErrGlueNotFound
	}
	if err != nil {
		return GlueSchema{}, fmt.Errorf("get schema: %w", err)
	}
	latest, err := s.latestGlueSchemaVersionNumber(accountID, registryName, schemaName)
	if err != nil {
		return GlueSchema{}, err
	}
	sc.LatestSchemaVersion = latest
	sc.NextSchemaVersion = latest + 1
	return sc, nil
}

// CreateGlueSchema creates a schema set and registers version 1.
func (s *Store) CreateGlueSchema(
	accountID, region, registryName, schemaName, dataFormat, compatibility, description, definition string,
) (GlueCreateSchemaResult, error) {
	schemaName = strings.TrimSpace(schemaName)
	if schemaName == "" {
		return GlueCreateSchemaResult{}, fmt.Errorf("%w: SchemaName required", ErrGlueBadRequest)
	}
	format, err := normalizeGlueDataFormat(dataFormat)
	if err != nil {
		return GlueCreateSchemaResult{}, err
	}
	if strings.TrimSpace(definition) == "" {
		return GlueCreateSchemaResult{}, fmt.Errorf("%w: SchemaDefinition required", ErrGlueBadRequest)
	}
	if err := validateGlueSchemaDefinition(format, definition); err != nil {
		return GlueCreateSchemaResult{}, err
	}
	compat := strings.ToUpper(strings.TrimSpace(compatibility))
	if compat == "" {
		compat = "NONE"
	}
	reg, err := s.ensureGlueRegistry(accountID, region, registryName)
	if err != nil {
		return GlueCreateSchemaResult{}, err
	}
	if _, err := s.loadGlueSchema(accountID, reg.Name, schemaName); err == nil {
		return GlueCreateSchemaResult{}, ErrGlueAlreadyExists
	} else if !errors.Is(err, ErrGlueNotFound) {
		return GlueCreateSchemaResult{}, err
	}
	now := time.Now().UTC().UnixMilli()
	schemaARN := glueSchemaARN(accountID, region, reg.Name, schemaName)
	_, err = s.db.Exec(
		`INSERT INTO glue_schemas (
		   account_id, registry_name, schema_name, description, data_format, compatibility,
		   schema_arn, schema_status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, reg.Name, schemaName, description, format, compat, schemaARN, glueSchemaStatusAvail, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return GlueCreateSchemaResult{}, ErrGlueAlreadyExists
		}
		return GlueCreateSchemaResult{}, fmt.Errorf("create schema: %w", err)
	}
	versionID := uuid.NewString()
	_, err = s.db.Exec(
		`INSERT INTO glue_schema_versions (
		   account_id, schema_version_id, registry_name, schema_name, version_number, definition, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, versionID, reg.Name, schemaName, int64(1), definition, glueVersionStatusAvail, now,
	)
	if err != nil {
		return GlueCreateSchemaResult{}, fmt.Errorf("create schema version: %w", err)
	}
	sc := GlueSchema{
		RegistryName: reg.Name, SchemaName: schemaName, Description: description,
		DataFormat: format, Compatibility: compat, SchemaARN: schemaARN,
		SchemaStatus: glueSchemaStatusAvail, LatestSchemaVersion: 1, NextSchemaVersion: 2,
		CreatedAt: now, UpdatedAt: now,
	}
	return GlueCreateSchemaResult{
		Schema: sc, SchemaVersionID: versionID, VersionNumber: 1, VersionStatus: glueVersionStatusAvail,
	}, nil
}

// GetGlueSchema returns a schema set.
func (s *Store) GetGlueSchema(accountID, registryName, schemaName string) (GlueSchema, error) {
	registryName = strings.TrimSpace(registryName)
	if registryName == "" {
		registryName = glueDefaultRegistryName
	}
	return s.loadGlueSchema(accountID, registryName, schemaName)
}

// ListGlueSchemas lists schemas, optionally filtered by registry.
func (s *Store) ListGlueSchemas(accountID, registryName string) ([]GlueSchema, error) {
	registryName = strings.TrimSpace(registryName)
	var (
		rows *sql.Rows
		err  error
	)
	if registryName == "" {
		rows, err = s.db.Query(
			`SELECT registry_name, schema_name, description, data_format, compatibility, schema_arn, schema_status, created_at, updated_at
			 FROM glue_schemas WHERE account_id = ? ORDER BY registry_name, schema_name`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT registry_name, schema_name, description, data_format, compatibility, schema_arn, schema_status, created_at, updated_at
			 FROM glue_schemas WHERE account_id = ? AND registry_name = ? ORDER BY schema_name`,
			accountID, registryName,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	defer rows.Close()
	var out []GlueSchema
	for rows.Next() {
		var sc GlueSchema
		if err := rows.Scan(
			&sc.RegistryName, &sc.SchemaName, &sc.Description, &sc.DataFormat, &sc.Compatibility,
			&sc.SchemaARN, &sc.SchemaStatus, &sc.CreatedAt, &sc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list schemas scan: %w", err)
		}
		latest, err := s.latestGlueSchemaVersionNumber(accountID, sc.RegistryName, sc.SchemaName)
		if err != nil {
			return nil, err
		}
		sc.LatestSchemaVersion = latest
		sc.NextSchemaVersion = latest + 1
		out = append(out, sc)
	}
	return out, rows.Err()
}

// DeleteGlueSchema deletes a schema and its versions.
func (s *Store) DeleteGlueSchema(accountID, registryName, schemaName string) error {
	registryName = strings.TrimSpace(registryName)
	if registryName == "" {
		registryName = glueDefaultRegistryName
	}
	if _, err := s.loadGlueSchema(accountID, registryName, schemaName); err != nil {
		return err
	}
	_, _ = s.db.Exec(
		`DELETE FROM glue_schema_versions WHERE account_id = ? AND registry_name = ? AND schema_name = ?`,
		accountID, registryName, schemaName,
	)
	_, err := s.db.Exec(
		`DELETE FROM glue_schemas WHERE account_id = ? AND registry_name = ? AND schema_name = ?`,
		accountID, registryName, schemaName,
	)
	if err != nil {
		return fmt.Errorf("delete schema: %w", err)
	}
	return nil
}

// RegisterGlueSchemaVersion adds a new schema version.
func (s *Store) RegisterGlueSchemaVersion(
	accountID, registryName, schemaName, definition string,
) (GlueSchemaVersion, error) {
	registryName = strings.TrimSpace(registryName)
	if registryName == "" {
		registryName = glueDefaultRegistryName
	}
	schemaName = strings.TrimSpace(schemaName)
	if schemaName == "" {
		return GlueSchemaVersion{}, fmt.Errorf("%w: SchemaName required", ErrGlueBadRequest)
	}
	if strings.TrimSpace(definition) == "" {
		return GlueSchemaVersion{}, fmt.Errorf("%w: SchemaDefinition required", ErrGlueBadRequest)
	}
	sc, err := s.loadGlueSchema(accountID, registryName, schemaName)
	if err != nil {
		return GlueSchemaVersion{}, err
	}
	if strings.EqualFold(sc.Compatibility, "DISABLED") && sc.LatestSchemaVersion >= 1 {
		return GlueSchemaVersion{}, fmt.Errorf("%w: schema versioning is DISABLED", ErrGlueBadRequest)
	}
	if err := validateGlueSchemaDefinition(sc.DataFormat, definition); err != nil {
		return GlueSchemaVersion{}, err
	}
	next := sc.LatestSchemaVersion + 1
	now := time.Now().UTC().UnixMilli()
	versionID := uuid.NewString()
	_, err = s.db.Exec(
		`INSERT INTO glue_schema_versions (
		   account_id, schema_version_id, registry_name, schema_name, version_number, definition, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, versionID, registryName, schemaName, next, definition, glueVersionStatusAvail, now,
	)
	if err != nil {
		return GlueSchemaVersion{}, fmt.Errorf("register schema version: %w", err)
	}
	_, _ = s.db.Exec(
		`UPDATE glue_schemas SET updated_at = ? WHERE account_id = ? AND registry_name = ? AND schema_name = ?`,
		now, accountID, registryName, schemaName,
	)
	return GlueSchemaVersion{
		SchemaVersionID: versionID, RegistryName: registryName, SchemaName: schemaName,
		VersionNumber: next, Definition: definition, DataFormat: sc.DataFormat,
		Status: glueVersionStatusAvail, CreatedAt: now,
	}, nil
}

// GetGlueSchemaVersionByID returns a version by SchemaVersionId.
func (s *Store) GetGlueSchemaVersionByID(accountID, schemaVersionID string) (GlueSchemaVersion, error) {
	schemaVersionID = strings.TrimSpace(schemaVersionID)
	var v GlueSchemaVersion
	err := s.db.QueryRow(
		`SELECT v.schema_version_id, v.registry_name, v.schema_name, v.version_number, v.definition, v.status, v.created_at,
		        s.data_format
		 FROM glue_schema_versions v
		 JOIN glue_schemas s ON s.account_id = v.account_id AND s.registry_name = v.registry_name AND s.schema_name = v.schema_name
		 WHERE v.account_id = ? AND v.schema_version_id = ?`,
		accountID, schemaVersionID,
	).Scan(
		&v.SchemaVersionID, &v.RegistryName, &v.SchemaName, &v.VersionNumber, &v.Definition, &v.Status, &v.CreatedAt,
		&v.DataFormat,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return GlueSchemaVersion{}, ErrGlueNotFound
	}
	if err != nil {
		return GlueSchemaVersion{}, fmt.Errorf("get schema version: %w", err)
	}
	return v, nil
}

// GetGlueSchemaVersionByNumber returns a version by registry/schema/number.
func (s *Store) GetGlueSchemaVersionByNumber(
	accountID, registryName, schemaName string, versionNumber int64,
) (GlueSchemaVersion, error) {
	registryName = strings.TrimSpace(registryName)
	if registryName == "" {
		registryName = glueDefaultRegistryName
	}
	if versionNumber < 1 {
		return GlueSchemaVersion{}, fmt.Errorf("%w: SchemaVersionNumber required", ErrGlueBadRequest)
	}
	var v GlueSchemaVersion
	err := s.db.QueryRow(
		`SELECT v.schema_version_id, v.registry_name, v.schema_name, v.version_number, v.definition, v.status, v.created_at,
		        s.data_format
		 FROM glue_schema_versions v
		 JOIN glue_schemas s ON s.account_id = v.account_id AND s.registry_name = v.registry_name AND s.schema_name = v.schema_name
		 WHERE v.account_id = ? AND v.registry_name = ? AND v.schema_name = ? AND v.version_number = ?`,
		accountID, registryName, schemaName, versionNumber,
	).Scan(
		&v.SchemaVersionID, &v.RegistryName, &v.SchemaName, &v.VersionNumber, &v.Definition, &v.Status, &v.CreatedAt,
		&v.DataFormat,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return GlueSchemaVersion{}, ErrGlueNotFound
	}
	if err != nil {
		return GlueSchemaVersion{}, fmt.Errorf("get schema version by number: %w", err)
	}
	return v, nil
}

// ListGlueSchemaVersions lists versions for a schema.
func (s *Store) ListGlueSchemaVersions(accountID, registryName, schemaName string) ([]GlueSchemaVersion, error) {
	registryName = strings.TrimSpace(registryName)
	if registryName == "" {
		registryName = glueDefaultRegistryName
	}
	if _, err := s.loadGlueSchema(accountID, registryName, schemaName); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT v.schema_version_id, v.registry_name, v.schema_name, v.version_number, v.definition, v.status, v.created_at,
		        s.data_format
		 FROM glue_schema_versions v
		 JOIN glue_schemas s ON s.account_id = v.account_id AND s.registry_name = v.registry_name AND s.schema_name = v.schema_name
		 WHERE v.account_id = ? AND v.registry_name = ? AND v.schema_name = ?
		 ORDER BY v.version_number`,
		accountID, registryName, schemaName,
	)
	if err != nil {
		return nil, fmt.Errorf("list schema versions: %w", err)
	}
	defer rows.Close()
	var out []GlueSchemaVersion
	for rows.Next() {
		var v GlueSchemaVersion
		if err := rows.Scan(
			&v.SchemaVersionID, &v.RegistryName, &v.SchemaName, &v.VersionNumber, &v.Definition, &v.Status, &v.CreatedAt,
			&v.DataFormat,
		); err != nil {
			return nil, fmt.Errorf("list schema versions scan: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ResolveGlueSchemaReference loads the referenced schema version.
func (s *Store) ResolveGlueSchemaReference(accountID string, ref GlueSchemaReference) (GlueSchemaVersion, error) {
	if id := strings.TrimSpace(ref.SchemaVersionID); id != "" {
		return s.GetGlueSchemaVersionByID(accountID, id)
	}
	registryName := strings.TrimSpace(ref.RegistryName)
	schemaName := strings.TrimSpace(ref.SchemaName)
	if registryName == "" || schemaName == "" {
		return GlueSchemaVersion{}, fmt.Errorf("%w: SchemaReference requires SchemaVersionId or RegistryName+SchemaName", ErrGlueBadRequest)
	}
	versionNumber := ref.SchemaVersionNumber
	if versionNumber < 1 {
		latest, err := s.latestGlueSchemaVersionNumber(accountID, registryName, schemaName)
		if err != nil {
			return GlueSchemaVersion{}, err
		}
		if latest < 1 {
			return GlueSchemaVersion{}, ErrGlueNotFound
		}
		versionNumber = latest
	}
	return s.GetGlueSchemaVersionByNumber(accountID, registryName, schemaName, versionNumber)
}

// SchemaReferenceFromTableParams builds a SchemaReference from table Parameters keys.
func SchemaReferenceFromTableParams(params map[string]string) (GlueSchemaReference, bool) {
	if params == nil {
		return GlueSchemaReference{}, false
	}
	ref := GlueSchemaReference{
		RegistryName:    strings.TrimSpace(params["RegistryName"]),
		SchemaName:      strings.TrimSpace(params["SchemaName"]),
		SchemaVersionID: strings.TrimSpace(params["SchemaVersionId"]),
	}
	if v := strings.TrimSpace(params["SchemaVersionNumber"]); v != "" {
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			ref.SchemaVersionNumber = n
		}
	}
	if ref.SchemaVersionID == "" && (ref.RegistryName == "" || ref.SchemaName == "") {
		return GlueSchemaReference{}, false
	}
	return ref, true
}

func (s *Store) resolveGlueTableColumns(accountID string, t *GlueTable) {
	if t == nil || len(t.Columns) > 0 {
		return
	}
	ref := t.SchemaReference
	if ref.SchemaVersionID == "" && ref.RegistryName == "" && ref.SchemaName == "" {
		if fromParams, ok := SchemaReferenceFromTableParams(t.Parameters); ok {
			ref = fromParams
		}
	}
	if ref.SchemaVersionID == "" && ref.RegistryName == "" && ref.SchemaName == "" {
		return
	}
	ver, err := s.ResolveGlueSchemaReference(accountID, ref)
	if err != nil {
		return
	}
	cols, err := glueSchemaDefinitionToColumns(ver.DataFormat, ver.Definition)
	if err != nil || len(cols) == 0 {
		return
	}
	t.Columns = cols
}
