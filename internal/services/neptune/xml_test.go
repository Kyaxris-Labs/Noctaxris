package neptune_test

import (
	"strings"
	"testing"

	neptunesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/neptune"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestNeptuneXML(t *testing.T) {
	c := store.NeptuneCluster{
		DBClusterIdentifier: "neptune-1", Engine: "neptune", EngineVersion: "1.3",
		Status: "available", EndpointAddress: "neptune.local", EndpointPort: 8182,
	}
	out, err := neptunesvc.CreateDBClusterXML(c, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "CreateDBClusterResponse") {
		t.Fatalf("create=%s", out)
	}

	out, _ = neptunesvc.DescribeDBClustersXML([]store.NeptuneCluster{c}, "req-2")
	if !strings.Contains(string(out), "neptune-1") {
		t.Fatal()
	}
	_, _ = neptunesvc.DescribeDBClustersXML(nil, "req-3")

	out, _ = neptunesvc.DeleteDBClusterXML(c, "req-4")
	if !strings.Contains(string(out), "deleting") {
		t.Fatal()
	}

	errXML, err := neptunesvc.ErrorXML("InvalidParameter", "bad", "req-e")
	if err != nil || !strings.Contains(string(errXML), "InvalidParameter") {
		t.Fatalf("err=%s %v", errXML, err)
	}
}
