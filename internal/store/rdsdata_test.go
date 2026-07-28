package store_test

import (
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
