package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestParseAlterAddColumn(t *testing.T) {
	table, col, ok := parseAlterAddColumn(`ALTER TABLE access_keys ADD COLUMN role_arn TEXT`)
	if !ok || table != "access_keys" || col != "role_arn" {
		t.Fatalf("got table=%q col=%q ok=%v", table, col, ok)
	}
	table, col, ok = parseAlterAddColumn(`ALTER TABLE s3_buckets ADD COLUMN canned_acl TEXT NOT NULL DEFAULT 'private'`)
	if !ok || table != "s3_buckets" || col != "canned_acl" {
		t.Fatalf("got table=%q col=%q ok=%v", table, col, ok)
	}
	if _, _, ok := parseAlterAddColumn(`CREATE UNIQUE INDEX IF NOT EXISTS idx ON t(a)`); ok {
		t.Fatal("expected non-ALTER to fail parse")
	}
}

func TestExecMigrateStmtSkipsExistingColumn(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	cache := map[string]map[string]bool{}
	stmt := `ALTER TABLE t ADD COLUMN name TEXT NOT NULL DEFAULT ''`
	if err := execMigrateStmt(db, stmt, cache); err != nil {
		t.Fatalf("skip existing: %v", err)
	}
	if !cache["t"]["name"] {
		t.Fatal("expected name cached as present")
	}
	// Adding a missing column must succeed and update cache.
	if err := execMigrateStmt(db, `ALTER TABLE t ADD COLUMN extra TEXT NOT NULL DEFAULT ''`, cache); err != nil {
		t.Fatalf("add missing: %v", err)
	}
	if !cache["t"]["extra"] {
		t.Fatal("expected extra cached after add")
	}
	var col string
	if err := db.QueryRow(`SELECT name FROM pragma_table_info('t') WHERE name = 'extra'`).Scan(&col); err != nil || col != "extra" {
		t.Fatalf("extra missing: col=%q err=%v", col, err)
	}
	// Second Open-style pass stays idempotent.
	if err := execMigrateStmt(db, `ALTER TABLE t ADD COLUMN extra TEXT NOT NULL DEFAULT ''`, map[string]map[string]bool{}); err != nil {
		t.Fatalf("idempotent: %v", err)
	}
}

func TestOpenSkipsRedundantMigrateAlters(t *testing.T) {
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// Re-open same data root; migrateSchema + Ensure* ALTER paths must stay cheap/idempotent.
	st2, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
}
