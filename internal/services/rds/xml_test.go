package rds_test

import (
	"strings"
	"testing"

	rdssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/rds"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRDSXML(t *testing.T) {
	inst := store.RDSDBInstance{
		DBInstanceIdentifier: "lab-db", DBInstanceARN: "arn:db", Engine: "postgres", EngineVersion: "16",
		DBInstanceClass: "db.t3.micro", DBName: "app", MasterUsername: "admin",
		DBInstanceStatus: "available", AllocatedStorage: 20, MasterUserSecretARN: "arn:secret",
		CreatedAt: 1_700_000_000_000, EndpointAddress: "lab.local", EndpointPort: 5432,
	}
	out, err := rdssvc.CreateDBInstanceXML(inst, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "CreateDBInstanceResponse") || !strings.Contains(s, "lab-db") {
		t.Fatalf("create=%q", s)
	}

	inst.EndpointAddress = ""
	out, _ = rdssvc.CreateDBInstanceXML(inst, "req-2")

	out, _ = rdssvc.DeleteDBInstanceXML(inst, "req-3")
	if !strings.Contains(string(out), "DeleteDBInstanceResponse") {
		t.Fatal()
	}

	out, _ = rdssvc.DescribeDBInstancesXML([]store.RDSDBInstance{inst}, "req-4")
	if !strings.Contains(string(out), "DescribeDBInstancesResult") {
		t.Fatal()
	}
	_, _ = rdssvc.DescribeDBInstancesXML(nil, "req-5")

	errBody := rdssvc.ErrorXML("DBInstanceNotFound", "missing", "req-err")
	if len(errBody) == 0 {
		t.Fatal("error xml empty")
	}
}
