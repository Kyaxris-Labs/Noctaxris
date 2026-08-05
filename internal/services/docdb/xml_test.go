package docdb_test

import (
	"strings"
	"testing"

	docdbsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/docdb"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDocDBXML(t *testing.T) {
	c := store.DocDBCluster{
		DBClusterIdentifier: "docdb-1", Engine: "docdb", EngineVersion: "5.0",
		Status: "available", EndpointAddress: "docdb.local", EndpointPort: 27017, MasterUsername: "admin",
	}
	out, err := docdbsvc.CreateDBClusterXML(c, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "CreateDBClusterResponse") {
		t.Fatalf("create=%s", out)
	}

	out, _ = docdbsvc.DescribeDBClustersXML([]store.DocDBCluster{c}, "req-2")
	out, _ = docdbsvc.DeleteDBClusterXML(c, "req-3")

	errXML, err := docdbsvc.ErrorXML("InvalidParameter", "bad", "req-e")
	if err != nil || !strings.Contains(string(errXML), "InvalidParameter") {
		t.Fatalf("err=%s %v", errXML, err)
	}
}
