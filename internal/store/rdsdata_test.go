package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openRDSDataTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestIsRDSDataSupportedEngine(t *testing.T) {
	for _, eng := range []string{"postgres", "mysql", "mariadb", "POSTGRES"} {
		if !store.IsRDSDataSupportedEngine(eng) {
			t.Fatalf("expected supported %q", eng)
		}
	}
	for _, eng := range []string{"oracle-ee", "aurora-mysql"} {
		if store.IsRDSDataSupportedEngine(eng) {
			t.Fatalf("expected unsupported %q", eng)
		}
	}
}

func TestResolveRDSDataResourceMySQLAccepted(t *testing.T) {
	st := openRDSDataTestStore(t)
	account := "123456789012"
	inst, err := st.CreateRDSDBInstance(account, "us-east-1", store.CreateRDSDBInstanceInput{
		DBInstanceIdentifier: "data-mysql",
		Engine:               "mysql",
		MasterUsername:       "root",
		MasterUserPassword:   "lab-pass-12345",
		DBName:               "appdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.ResolveRDSDataResource(account, inst.DBInstanceARN, inst.MasterUserSecretARN)
	if err != nil {
		t.Fatalf("ResolveRDSDataResource mysql: %v", err)
	}
	if got.Engine != "mysql" {
		t.Fatalf("engine=%q", got.Engine)
	}
}

func TestRDSDataTransactionCRUDAndNegatives(t *testing.T) {
	st := openRDSDataTestStore(t)
	account := "123456789012"
	if err := st.EnsureRDSDataSchema(); err != nil {
		t.Fatal(err)
	}

	inst, err := st.CreateRDSDBInstance(account, "us-east-1", store.CreateRDSDBInstanceInput{
		DBInstanceIdentifier: "data-pg",
		Engine:               "postgres",
		MasterUsername:       "root",
		MasterUserPassword:   "lab-pass-12345",
		DBName:               "appdb",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.ResolveRDSDataResource(account, "", inst.MasterUserSecretARN); !errors.Is(err, store.ErrRDSDataBadRequest) {
		t.Fatalf("empty arn: %v", err)
	}
	if _, err := st.ResolveRDSDataResource(account, "arn:aws:rds:us-east-1:"+account+":db:missing", inst.MasterUserSecretARN); !errors.Is(err, store.ErrRDSDataNotFound) {
		t.Fatalf("missing db: %v", err)
	}
	if store.IsRDSDataSupportedEngine("oracle-ee") {
		t.Fatal("oracle should be unsupported for Data API")
	}

	if _, err := st.ResolveRDSDataResource(account, inst.DBInstanceARN, "arn:aws:secretsmanager:us-east-1:"+account+":secret:missing-xxxxxx"); err == nil {
		t.Fatal("expected secret error")
	}

	txnID, err := st.BeginRDSDataTransaction(account, inst.DBInstanceARN, inst.MasterUserSecretARN, "appdb")
	if err != nil || txnID == "" {
		t.Fatalf("begin=%q err=%v", txnID, err)
	}
	txn, err := st.GetRDSDataTransaction(account, txnID)
	if err != nil || txn.Status != "active" || txn.Database != "appdb" {
		t.Fatalf("get=%+v err=%v", txn, err)
	}
	if _, err := st.GetRDSDataTransaction(account, ""); !errors.Is(err, store.ErrRDSDataBadRequest) {
		t.Fatalf("empty txn: %v", err)
	}
	if _, err := st.GetRDSDataTransaction(account, "missing"); !errors.Is(err, store.ErrRDSDataTxnNotFound) {
		t.Fatalf("missing txn: %v", err)
	}

	if err := st.RecordRDSDataStatement(account, store.RDSDataExecuteRequest{
		ResourceARN: inst.DBInstanceARN, SecretARN: inst.MasterUserSecretARN,
		Database: "appdb", SQL: "SELECT 1", TransactionID: txnID,
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.FinishRDSDataTransaction(account, txnID, "committed"); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishRDSDataTransaction(account, txnID, "committed"); !errors.Is(err, store.ErrRDSDataBadRequest) {
		t.Fatalf("inactive finish: %v", err)
	}
	if err := st.FinishRDSDataTransaction(account, "", "committed"); !errors.Is(err, store.ErrRDSDataBadRequest) {
		t.Fatalf("empty finish: %v", err)
	}
	if err := st.FinishRDSDataTransaction(account, "missing", "committed"); !errors.Is(err, store.ErrRDSDataTxnNotFound) {
		t.Fatalf("missing finish: %v", err)
	}
}

func TestStubRDSDataExecutor(t *testing.T) {
	var e store.StubRDSDataExecutor
	sel, err := e.Execute(store.RDSDataExecuteRequest{SQL: "SELECT 1"})
	if err != nil || len(sel.Records) != 1 || sel.FormattedRecords == "" {
		t.Fatalf("select=%+v err=%v", sel, err)
	}
	upd, err := e.Execute(store.RDSDataExecuteRequest{SQL: "UPDATE t SET a=1"})
	if err != nil || upd.NumberOfRecordsUpdated != 1 {
		t.Fatalf("update=%+v err=%v", upd, err)
	}
}
