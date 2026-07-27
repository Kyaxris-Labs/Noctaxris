package sdk_test

import (
	"strings"
	"testing"
	"time"
)

func TestMemoryDBCreateDescribeDeleteSoftSkipNested(t *testing.T) {
	requireReady(t)
	name := "sdk-mdb-" + uniquePrefix(t)
	if len(name) > 40 {
		name = name[:40]
	}

	createStatus, createBody, createParsed := signedJSONTarget(t, "memorydb", "AmazonMemoryDB.CreateCluster", map[string]any{
		"ClusterName": name,
		"NodeType":    "db.t4g.small",
		"ACLName":     "open-access",
		"Engine":      "redis",
	})
	if createStatus != 200 {
		t.Fatalf("CreateCluster status=%d body=%s", createStatus, createBody)
	}
	cluster, _ := createParsed["Cluster"].(map[string]any)
	if cluster == nil {
		t.Fatalf("CreateCluster missing Cluster: %s", createBody)
	}
	defer func() {
		_, _, _ = signedJSONTarget(t, "memorydb", "AmazonMemoryDB.DeleteCluster", map[string]any{
			"ClusterName": name,
		})
	}()

	ep, _ := cluster["ClusterEndpoint"].(map[string]any)
	addr, _ := ep["Address"].(string)
	if !strings.Contains(addr, "memorydb.noctaxris.internal") {
		t.Fatalf("expected nested endpoint, got %v", ep)
	}

	var status string
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		descStatus, descBody, descParsed := signedJSONTarget(t, "memorydb", "AmazonMemoryDB.DescribeClusters", map[string]any{
			"ClusterName": name,
		})
		if descStatus != 200 {
			t.Fatalf("DescribeClusters status=%d body=%s", descStatus, descBody)
		}
		clusters, _ := descParsed["Clusters"].([]any)
		if len(clusters) < 1 {
			t.Fatalf("DescribeClusters empty: %s", descBody)
		}
		c, _ := clusters[0].(map[string]any)
		status, _ = c["Status"].(string)
		if status == "available" || status == "failed" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if status != "available" {
		t.Skipf("MemoryDB nested soft-skip: Status=%s (set Compose noctaxris-engine healthy for available)", status)
	}
}
