package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestEnsureSNSSchemaUpgradesLegacyPublishedMessages(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Pre-FIFO SNS shape: table exists without message_deduplication_id.
	_, err = db.Exec(`
CREATE TABLE sns_topics (
  account_id TEXT NOT NULL,
  topic_name TEXT NOT NULL,
  topic_arn TEXT NOT NULL,
  policy_json TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  creation_date TEXT NOT NULL,
  PRIMARY KEY (account_id, topic_name)
);
CREATE TABLE sns_subscriptions (
  subscription_arn TEXT PRIMARY KEY,
  topic_arn TEXT NOT NULL,
  protocol TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  confirmed INTEGER NOT NULL DEFAULT 0,
  owner TEXT NOT NULL DEFAULT ''
);
CREATE TABLE sns_published_messages (
  message_id TEXT PRIMARY KEY,
  topic_arn TEXT NOT NULL,
  body TEXT NOT NULL,
  subject TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE TABLE sns_http_catcher (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  subscription_arn TEXT NOT NULL DEFAULT '',
  topic_arn TEXT NOT NULL DEFAULT '',
  message_type TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  received_at TEXT NOT NULL
);
`)
	if err != nil {
		t.Fatal(err)
	}

	if err := EnsureSNSSchema(db); err != nil {
		t.Fatalf("EnsureSNSSchema on legacy volume: %v", err)
	}
	var col string
	err = db.QueryRow(
		`SELECT name FROM pragma_table_info('sns_published_messages') WHERE name = 'message_deduplication_id'`,
	).Scan(&col)
	if err != nil || col != "message_deduplication_id" {
		t.Fatalf("column missing after migrate: col=%q err=%v", col, err)
	}
	// Second call must stay idempotent (duplicate ALTER + index).
	if err := EnsureSNSSchema(db); err != nil {
		t.Fatalf("EnsureSNSSchema idempotent: %v", err)
	}
}
