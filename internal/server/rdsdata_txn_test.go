package server

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLookupRDSDataTxnSessionExpired(t *testing.T) {
	const accountID = "acct-txn"
	const txnID = "txn-expired-1"
	registerRDSDataTxnSession(txnID, &rdsDataTxnSession{
		conn:      nil,
		accountID: accountID,
		resource:  "arn:aws:rds:us-east-1:1:db:x",
		secret:    "arn:aws:secretsmanager:us-east-1:1:secret:x",
		database:  "postgres",
		expires:   time.Now().UTC().Add(-time.Minute),
	})
	t.Cleanup(func() {
		removeRDSDataTxnSession(accountID, txnID)
	})

	srv := &Server{}
	_, err := srv.lookupRDSDataTxnSession(accountID, txnID)
	if !errors.Is(err, store.ErrRDSDataTxnNotFound) {
		t.Fatalf("want TransactionNotFoundException, got %v", err)
	}
}

func TestRDSDataTxnLive(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NOCTAXRIS_TEST_PGX_DSN"))
	if dsn == "" {
		t.Skip("NOCTAXRIS_TEST_PGX_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupConn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer setupConn.Close(context.Background())
	if _, err := setupConn.Exec(ctx, `CREATE TABLE IF NOT EXISTS noctaxris_txn_lab (id int PRIMARY KEY, v text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := setupConn.Exec(ctx, `DELETE FROM noctaxris_txn_lab WHERE id IN (1, 2)`); err != nil {
		t.Fatal(err)
	}

	srv := &Server{}
	const accountID = "live-acct"
	resource := "arn:aws:rds:us-east-1:1:db:live"
	secret := "arn:aws:secretsmanager:us-east-1:1:secret:live"

	// Rollback path: insert then rollback; row must be absent.
	conn1, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn1.Exec(ctx, "BEGIN"); err != nil {
		_ = conn1.Close(context.Background())
		t.Fatal(err)
	}
	const txnRoll = "live-txn-roll"
	registerRDSDataTxnSession(txnRoll, &rdsDataTxnSession{
		conn:      conn1,
		accountID: accountID,
		resource:  resource,
		secret:    secret,
		database:  "postgres",
		expires:   time.Now().UTC().Add(rdsDataTxnIdleTimeout),
	})
	_, err = srv.executeRDSDataOnTransaction(ctx, accountID, store.RDSDataExecuteRequest{
		ResourceARN:   resource,
		SecretARN:     secret,
		SQL:           "INSERT INTO noctaxris_txn_lab (id, v) VALUES (1, 'roll')",
		TransactionID: txnRoll,
	})
	if err != nil {
		t.Fatal(err)
	}
	removed := removeRDSDataTxnSession(accountID, txnRoll)
	if removed == nil || removed.conn == nil {
		t.Fatal("missing rollback session")
	}
	if _, err := removed.conn.Exec(ctx, "ROLLBACK"); err != nil {
		removed.close()
		t.Fatal(err)
	}
	removed.close()

	var n int
	if err := setupConn.QueryRow(ctx, `SELECT COUNT(*) FROM noctaxris_txn_lab WHERE id = 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rollback left row count=%d", n)
	}

	// Commit path.
	conn2, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn2.Exec(ctx, "BEGIN"); err != nil {
		_ = conn2.Close(context.Background())
		t.Fatal(err)
	}
	const txnCommit = "live-txn-commit"
	registerRDSDataTxnSession(txnCommit, &rdsDataTxnSession{
		conn:      conn2,
		accountID: accountID,
		resource:  resource,
		secret:    secret,
		database:  "postgres",
		expires:   time.Now().UTC().Add(rdsDataTxnIdleTimeout),
	})
	_, err = srv.executeRDSDataOnTransaction(ctx, accountID, store.RDSDataExecuteRequest{
		ResourceARN:   resource,
		SecretARN:     secret,
		SQL:           "INSERT INTO noctaxris_txn_lab (id, v) VALUES (2, 'commit')",
		TransactionID: txnCommit,
	})
	if err != nil {
		t.Fatal(err)
	}
	removed = removeRDSDataTxnSession(accountID, txnCommit)
	if removed == nil || removed.conn == nil {
		t.Fatal("missing commit session")
	}
	if _, err := removed.conn.Exec(ctx, "COMMIT"); err != nil {
		removed.close()
		t.Fatal(err)
	}
	removed.close()
	if err := setupConn.QueryRow(ctx, `SELECT COUNT(*) FROM noctaxris_txn_lab WHERE id = 2`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("commit row count=%d", n)
	}
	_, _ = setupConn.Exec(ctx, `DELETE FROM noctaxris_txn_lab WHERE id IN (1, 2)`)
}

func TestMatchRDSDataTxnResource(t *testing.T) {
	sess := &rdsDataTxnSession{resource: "arn:rds:db", secret: "arn:secret:a"}
	if err := matchRDSDataTxnResource(sess, "arn:rds:other", ""); err == nil {
		t.Fatal("want resource mismatch error")
	}
	if err := matchRDSDataTxnResource(sess, "", "arn:secret:b"); err == nil {
		t.Fatal("want secret mismatch error")
	}
	if err := matchRDSDataTxnResource(sess, "arn:rds:db", "arn:secret:a"); err != nil {
		t.Fatalf("match: %v", err)
	}
}

func TestLookupRDSDataTxnSessionEmptyAndMissing(t *testing.T) {
	srv := &Server{}
	_, err := srv.lookupRDSDataTxnSession("acct", "  ")
	if err == nil {
		t.Fatal("want error for empty transaction id")
	}
	_, err = srv.lookupRDSDataTxnSession("acct", "missing-txn")
	if !errors.Is(err, store.ErrRDSDataTxnNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestRDSDataTxnSessionCloseNilSafe(t *testing.T) {
	var sess *rdsDataTxnSession
	sess.close()
	sess = &rdsDataTxnSession{}
	sess.close()
}

func TestCommitRollbackRDSDataTxnNotFound(t *testing.T) {
	srv := &Server{store: mustOpenRDSDataTxnTestStore(t)}
	ctx := context.Background()
	err := srv.CommitRDSDataSQLTransaction(ctx, testAccountID, "nope", "", "")
	if !errors.Is(err, store.ErrRDSDataTxnNotFound) {
		t.Fatalf("Commit want not found got %v", err)
	}
	err = srv.RollbackRDSDataSQLTransaction(ctx, testAccountID, "nope", "", "")
	if !errors.Is(err, store.ErrRDSDataTxnNotFound) {
		t.Fatalf("Rollback want not found got %v", err)
	}
}

func mustOpenRDSDataTxnTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(dir + "/master.key")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.EnsureRoot(testAccountID, "AKIAROOTEXAMPLE01", "secret-root-value"); err != nil {
		t.Fatal(err)
	}
	return st
}

const testAccountID = "000000000001"
