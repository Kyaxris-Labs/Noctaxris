package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMemoryDBHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AmazonMemoryDB.CreateCluster", "memorydb", map[string]any{
		"ClusterName": "lab-memdb-1",
		"NodeType":    "db.t4g.small",
		"ACLName":     "open-access",
		"Engine":      "redis",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateCluster status=%d body=%q", create.Code, create.Body.String())
	}
	var createBody map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &createBody); err != nil {
		t.Fatal(err)
	}
	cluster, _ := createBody["Cluster"].(map[string]any)
	if cluster == nil {
		t.Fatalf("create missing Cluster: %q", create.Body.String())
	}
	if cluster["Status"] != "creating" {
		t.Fatalf("without DinD expect creating, got %v", cluster["Status"])
	}
	ep, _ := cluster["ClusterEndpoint"].(map[string]any)
	addr, _ := ep["Address"].(string)
	if !strings.Contains(addr, "memorydb.noctaxris.internal") {
		t.Fatalf("endpoint=%v", ep)
	}
	if strings.Contains(addr, "0.0.0.0") || strings.Contains(addr, "127.0.0.1") {
		t.Fatalf("must not host-publish endpoint: %v", ep)
	}

	desc := mustJSONTarget(t, handler, "AmazonMemoryDB.DescribeClusters", "memorydb", map[string]any{
		"ClusterName": "lab-memdb-1",
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeClusters status=%d body=%q", desc.Code, desc.Body.String())
	}
	if !strings.Contains(desc.Body.String(), `"creating"`) || !strings.Contains(desc.Body.String(), "6379") {
		t.Fatalf("describe body=%q", desc.Body.String())
	}

	list := mustJSONTarget(t, handler, "AmazonMemoryDB.DescribeClusters", "memorydb", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "lab-memdb-1") {
		t.Fatalf("list status=%d body=%q", list.Code, list.Body.String())
	}

	users := mustJSONTarget(t, handler, "AmazonMemoryDB.DescribeUsers", "memorydb", map[string]any{}, now)
	if users.Code != http.StatusOK || !strings.Contains(users.Body.String(), `"Users"`) {
		t.Fatalf("DescribeUsers status=%d body=%q", users.Code, users.Body.String())
	}

	acls := mustJSONTarget(t, handler, "AmazonMemoryDB.DescribeACLs", "memorydb", map[string]any{}, now)
	if acls.Code != http.StatusOK || !strings.Contains(acls.Body.String(), `"ACLs"`) {
		t.Fatalf("DescribeACLs status=%d body=%q", acls.Code, acls.Body.String())
	}

	del := mustJSONTarget(t, handler, "AmazonMemoryDB.DeleteCluster", "memorydb", map[string]any{
		"ClusterName": "lab-memdb-1",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteCluster status=%d body=%q", del.Code, del.Body.String())
	}
}
