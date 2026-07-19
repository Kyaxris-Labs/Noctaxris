package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS accounts (
  account_id TEXT PRIMARY KEY
);
CREATE TABLE IF NOT EXISTS access_keys (
  access_key_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  secret_ciphertext BLOB NOT NULL,
  is_root INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS policies (
  policy_id TEXT PRIMARY KEY,
  document TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS policy_attachments (
  principal_arn TEXT NOT NULL,
  policy_id TEXT NOT NULL,
  PRIMARY KEY (principal_arn, policy_id)
);
`

type Store struct {
	db     *sql.DB
	master MasterKey
}

func Open(dataRoot string, master MasterKey) (*Store, error) {
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataRoot, "state.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, master: master}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) EnsureRoot(accountID, accessKeyID, secret string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT OR IGNORE INTO accounts (account_id) VALUES (?)`, accountID); err != nil {
		return err
	}

	var existingAccountID string
	var ciphertext []byte
	var isRoot int
	rowErr := tx.QueryRow(
		`SELECT account_id, secret_ciphertext, is_root FROM access_keys WHERE access_key_id = ?`,
		accessKeyID,
	).Scan(&existingAccountID, &ciphertext, &isRoot)

	if rowErr == nil {
		if existingAccountID != accountID || isRoot != 1 {
			return fmt.Errorf("access key %s already exists", accessKeyID)
		}
		plaintext, err := Unseal(s.master, ciphertext)
		if err != nil {
			return err
		}
		if string(plaintext) != secret {
			return fmt.Errorf("access key %s already exists with different secret", accessKeyID)
		}
		return tx.Commit()
	}
	if !errors.Is(rowErr, sql.ErrNoRows) {
		return rowErr
	}

	sealed, err := Seal(s.master, []byte(secret))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO access_keys (access_key_id, account_id, secret_ciphertext, is_root) VALUES (?, ?, ?, 1)`,
		accessKeyID, accountID, sealed,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LookupAccessKey(accessKeyID string) (accountID, secret string, isRoot bool, err error) {
	var ciphertext []byte
	var rootFlag int
	err = s.db.QueryRow(
		`SELECT account_id, secret_ciphertext, is_root FROM access_keys WHERE access_key_id = ?`,
		accessKeyID,
	).Scan(&accountID, &ciphertext, &rootFlag)
	if err != nil {
		return "", "", false, err
	}
	plaintext, err := Unseal(s.master, ciphertext)
	if err != nil {
		return "", "", false, err
	}
	return accountID, string(plaintext), rootFlag == 1, nil
}
