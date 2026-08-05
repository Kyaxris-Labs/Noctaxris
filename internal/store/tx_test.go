package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestWithTxCommitAndRollback(t *testing.T) {
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := withTx(st.db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS with_tx_probe (id INTEGER PRIMARY KEY)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("force rollback")
	err = withTx(st.db, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO with_tx_probe (id) VALUES (1)`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM with_tx_probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rollback failed, count=%d", n)
	}

	if err := withTx(st.db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO with_tx_probe (id) VALUES (2)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM with_tx_probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("commit count=%d", n)
	}
}
