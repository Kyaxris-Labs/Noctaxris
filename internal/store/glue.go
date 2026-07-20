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
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, database_name, name)
);
`

// GlueDatabase is a Data Catalog database.
type GlueDatabase struct {
	Name        string
	Description string
	CreatedAt   int64
}

// GlueColumn is a table column.
type GlueColumn struct {
	Name string `json:"Name"`
	Type string `json:"Type"`
}

// GlueTable is a Data Catalog table.
type GlueTable struct {
	DatabaseName    string
	Name            string
	Description     string
	StorageLocation string
	Columns         []GlueColumn
	CreatedAt       int64
	UpdatedAt       int64
}

// EnsureGlueSchema creates Glue catalog tables if missing.
func EnsureGlueSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure glue schema: db is nil")
	}
	if _, err := db.Exec(glueSchema); err != nil {
		return fmt.Errorf("ensure glue schema: %w", err)
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
func (s *Store) CreateGlueTable(accountID, databaseName, name, description, location string, columns []GlueColumn) (GlueTable, error) {
	databaseName = strings.TrimSpace(databaseName)
	name = strings.TrimSpace(name)
	if databaseName == "" || name == "" {
		return GlueTable{}, fmt.Errorf("%w: DatabaseName and Name required", ErrGlueBadRequest)
	}
	if _, err := s.GetGlueDatabase(accountID, databaseName); err != nil {
		return GlueTable{}, err
	}
	if columns == nil {
		columns = []GlueColumn{}
	}
	colJSON, err := json.Marshal(columns)
	if err != nil {
		return GlueTable{}, fmt.Errorf("marshal columns: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO glue_tables (account_id, database_name, name, description, storage_location, columns_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, databaseName, name, description, location, string(colJSON), now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return GlueTable{}, ErrGlueAlreadyExists
		}
		return GlueTable{}, fmt.Errorf("create table: %w", err)
	}
	return GlueTable{
		DatabaseName: databaseName, Name: name, Description: description,
		StorageLocation: location, Columns: columns, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// GetGlueTable returns a table.
func (s *Store) GetGlueTable(accountID, databaseName, name string) (GlueTable, error) {
	var t GlueTable
	var colJSON string
	err := s.db.QueryRow(
		`SELECT database_name, name, description, storage_location, columns_json, created_at, updated_at
		 FROM glue_tables WHERE account_id = ? AND database_name = ? AND name = ?`,
		accountID, databaseName, name,
	).Scan(&t.DatabaseName, &t.Name, &t.Description, &t.StorageLocation, &colJSON, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GlueTable{}, ErrGlueNotFound
	}
	if err != nil {
		return GlueTable{}, fmt.Errorf("get table: %w", err)
	}
	_ = json.Unmarshal([]byte(colJSON), &t.Columns)
	return t, nil
}

// GetGlueTables lists tables in a database.
func (s *Store) GetGlueTables(accountID, databaseName string) ([]GlueTable, error) {
	if _, err := s.GetGlueDatabase(accountID, databaseName); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT database_name, name, description, storage_location, columns_json, created_at, updated_at
		 FROM glue_tables WHERE account_id = ? AND database_name = ? ORDER BY name`,
		accountID, databaseName,
	)
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	defer rows.Close()
	var out []GlueTable
	for rows.Next() {
		var t GlueTable
		var colJSON string
		if err := rows.Scan(&t.DatabaseName, &t.Name, &t.Description, &t.StorageLocation, &colJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("get tables scan: %w", err)
		}
		_ = json.Unmarshal([]byte(colJSON), &t.Columns)
		out = append(out, t)
	}
	return out, rows.Err()
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
