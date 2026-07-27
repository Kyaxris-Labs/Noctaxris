package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openRDSTestStore(t *testing.T) *store.Store {
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

func TestRDSCreateDescribeDelete(t *testing.T) {
	st := openRDSTestStore(t)
	account := "000000000001"

	inst, err := st.CreateRDSDBInstance(account, "us-east-1", store.CreateRDSDBInstanceInput{
		DBInstanceIdentifier: "lab-pg-1",
		Engine:               "postgres",
		MasterUsername:       "postgres",
		MasterUserPassword:   "lab-secret-pass",
		DBName:               "appdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.DBInstanceStatus != "creating" {
		t.Fatalf("status=%q", inst.DBInstanceStatus)
	}
	if inst.MasterUserSecretARN == "" {
		t.Fatal("expected master secret ARN")
	}
	if inst.EndpointAddress == "" || inst.EndpointPort != 5432 {
		t.Fatalf("endpoint=%s:%d", inst.EndpointAddress, inst.EndpointPort)
	}

	sec, err := st.GetSecretValue(account, inst.MasterUserSecretARN)
	if err != nil {
		t.Fatal(err)
	}
	user, pass, err := store.ParseRDSMasterSecret(sec.SecretString)
	if err != nil || user != "postgres" || pass != "lab-secret-pass" {
		t.Fatalf("secret parse user=%q pass=%q err=%v", user, pass, err)
	}

	got, err := st.DescribeRDSDBInstance(account, "lab-pg-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.DBInstanceARN != inst.DBInstanceARN {
		t.Fatalf("arn mismatch %q vs %q", got.DBInstanceARN, inst.DBInstanceARN)
	}

	if err := st.UpdateRDSDBInstanceRuntime(account, "lab-pg-1", "available", "cid-1", "noctaxris-data-rds-lab-pg-1", 5432); err != nil {
		t.Fatal(err)
	}
	got, err = st.DescribeRDSDBInstance(account, "LAB-PG-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.DBInstanceStatus != "available" || got.ContainerID != "cid-1" {
		t.Fatalf("runtime update failed: %+v", got)
	}

	list, err := st.ListRDSDBInstances(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}

	if _, err := st.CreateRDSDBInstance(account, "us-east-1", store.CreateRDSDBInstanceInput{
		DBInstanceIdentifier: "lab-pg-1",
		Engine:               "postgres",
	}); !errors.Is(err, store.ErrRDSInstanceExists) {
		t.Fatalf("expected exists, got %v", err)
	}

	if _, err := st.CreateRDSDBInstance(account, "us-east-1", store.CreateRDSDBInstanceInput{
		DBInstanceIdentifier: "lab-oracle",
		Engine:               "oracle-ee",
	}); !errors.Is(err, store.ErrRDSBadRequest) {
		t.Fatalf("expected bad engine, got %v", err)
	}

	del, err := st.DeleteRDSDBInstance(account, "lab-pg-1")
	if err != nil || del.DBInstanceIdentifier != "lab-pg-1" {
		t.Fatalf("delete=%+v err=%v", del, err)
	}
	if _, err := st.DescribeRDSDBInstance(account, "lab-pg-1"); !errors.Is(err, store.ErrRDSInstanceNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestRDSCreateMySQLAndMariaDB(t *testing.T) {
	st := openRDSTestStore(t)
	account := "000000000001"

	mysql, err := st.CreateRDSDBInstance(account, "us-east-1", store.CreateRDSDBInstanceInput{
		DBInstanceIdentifier: "lab-mysql-1",
		Engine:               "mysql",
		MasterUsername:       "root",
		MasterUserPassword:   "lab-mysql-pass",
		DBName:               "appdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mysql.Engine != "mysql" || mysql.EndpointPort != 3306 {
		t.Fatalf("mysql=%+v", mysql)
	}
	if mysql.EngineVersion != "8.0" {
		t.Fatalf("mysql version=%q", mysql.EngineVersion)
	}
	if store.DefaultRDSImage("mysql") != store.DefaultRDSMySQLImage {
		t.Fatalf("mysql image=%q", store.DefaultRDSImage("mysql"))
	}

	maria, err := st.CreateRDSDBInstance(account, "us-east-1", store.CreateRDSDBInstanceInput{
		DBInstanceIdentifier: "lab-maria-1",
		Engine:               "MariaDB",
		MasterUserPassword:   "lab-maria-pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	if maria.Engine != "mariadb" || maria.EndpointPort != 3306 {
		t.Fatalf("mariadb=%+v", maria)
	}
	if maria.MasterUsername != "root" || maria.DBName != "appdb" {
		t.Fatalf("mariadb defaults user=%q db=%q", maria.MasterUsername, maria.DBName)
	}
	if maria.EngineVersion != "11" {
		t.Fatalf("mariadb version=%q", maria.EngineVersion)
	}

	if err := st.UpdateRDSDBInstanceRuntime(account, "lab-mysql-1", "available", "cid-m", "noctaxris-data-rds-lab-mysql-1", 3306); err != nil {
		t.Fatal(err)
	}
	got, err := st.DescribeRDSDBInstance(account, "lab-mysql-1")
	if err != nil || got.DBInstanceStatus != "available" || got.EndpointPort != 3306 {
		t.Fatalf("mysql runtime=%+v err=%v", got, err)
	}

	if _, err := st.DeleteRDSDBInstance(account, "lab-mysql-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteRDSDBInstance(account, "lab-maria-1"); err != nil {
		t.Fatal(err)
	}
}

func TestRDSEngineHelpers(t *testing.T) {
	if !store.IsValidRDSEngine("postgres") || !store.IsValidRDSEngine("mysql") || !store.IsValidRDSEngine("mariadb") {
		t.Fatal("expected postgres/mysql/mariadb valid")
	}
	if store.IsValidRDSEngine("aurora-mysql") || store.IsValidRDSEngine("oracle-ee") {
		t.Fatal("expected aurora/oracle rejected")
	}
	if store.DefaultRDSPort("mysql") != 3306 || store.DefaultRDSPort("postgres") != 5432 {
		t.Fatal("unexpected default ports")
	}
	if store.DefaultRDSImage("mariadb") != "mariadb:11" || store.DefaultRDSImage("mysql") != "mysql:8.0" {
		t.Fatal("unexpected default images")
	}
}

func TestRDSRejectsMissingSecretARN(t *testing.T) {
	st := openRDSTestStore(t)
	_, err := st.CreateRDSDBInstance("000000000001", "us-east-1", store.CreateRDSDBInstanceInput{
		DBInstanceIdentifier: "lab-pg-2",
		Engine:               "postgres",
		MasterUserSecretARN:  "arn:aws:secretsmanager:us-east-1:000000000001:secret:missing-xxxxxx",
	})
	if !errors.Is(err, store.ErrRDSBadRequest) {
		t.Fatalf("expected bad request for missing secret, got %v", err)
	}
}

func TestDataPlaneSecretHelpers(t *testing.T) {
	st := openRDSTestStore(t)
	arn, err := st.EnsureDataPlaneMasterSecret(
		"000000000001", "us-east-1", store.DataPlaneSecretElastiCache, "cache-1", "default", "pw",
	)
	if err != nil || arn == "" {
		t.Fatalf("arn=%q err=%v", arn, err)
	}
	arn2, err := st.EnsureDataPlaneMasterSecret(
		"000000000001", "us-east-1", store.DataPlaneSecretElastiCache, "cache-1", "default", "pw2",
	)
	if err != nil || arn2 != arn {
		t.Fatalf("update arn=%q vs %q err=%v", arn2, arn, err)
	}
	if _, err := st.ResolveDataPlaneSecretARN("000000000001", "arn:aws:secretsmanager:us-east-1:000000000001:secret:nope"); err == nil {
		t.Fatal("expected resolve failure")
	}
}
