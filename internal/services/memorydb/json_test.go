package memorydb_test

import (
	"encoding/json"
	"testing"

	memorydbsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/memorydb"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMemoryDBJSON(t *testing.T) {
	c := store.MemoryDBCluster{
		Name: "lab", Status: "available", NumberOfShards: 2,
		NodeType: "db.t4g.small", Engine: "redis", EngineVersion: "7.0",
		ACLName: "open-access", ARN: "arn:aws:memorydb:us-east-1:1:cluster/lab",
		EndpointAddress: "lab.noctaxris.local", EndpointPort: 6379,
	}
	createRaw, err := memorydbsvc.CreateClusterJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRaw, &createOut); err != nil {
		t.Fatal(err)
	}
	cluster, _ := createOut["Cluster"].(map[string]any)
	ep, _ := cluster["ClusterEndpoint"].(map[string]any)
	if ep["Port"].(float64) != 6379 {
		t.Fatalf("create=%v", createOut)
	}

	descRaw, err := memorydbsvc.DescribeClustersJSON([]store.MemoryDBCluster{c})
	if err != nil {
		t.Fatal(err)
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRaw, &descOut); err != nil {
		t.Fatal(err)
	}
	clusters, _ := descOut["Clusters"].([]any)
	if len(clusters) != 1 {
		t.Fatalf("describe=%v", descOut)
	}

	delRaw, err := memorydbsvc.DeleteClusterJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(delRaw, &createOut); err != nil {
		t.Fatal(err)
	}

	usersRaw, err := memorydbsvc.DescribeUsersJSON()
	if err != nil {
		t.Fatal(err)
	}
	var usersOut map[string]any
	if err := json.Unmarshal(usersRaw, &usersOut); err != nil {
		t.Fatal(err)
	}
	u, _ := usersOut["Users"].([]any)
	if len(u) != 0 {
		t.Fatalf("users=%v", usersOut)
	}

	aclsRaw, err := memorydbsvc.DescribeACLsJSON()
	if err != nil {
		t.Fatal(err)
	}
	var aclsOut map[string]any
	if err := json.Unmarshal(aclsRaw, &aclsOut); err != nil {
		t.Fatal(err)
	}
	a, _ := aclsOut["ACLs"].([]any)
	if len(a) != 0 {
		t.Fatalf("acls=%v", aclsOut)
	}
}
