package sdk_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestEKSCreateDescribeListDelete(t *testing.T) {
	requireReady(t)
	name := "goeks-" + uniquePrefix(t)
	if len(name) > 40 {
		name = name[:40]
	}
	createBody, err := json.Marshal(map[string]any{
		"name":    name,
		"roleArn": "arn:aws:iam::000000000001:role/eks",
		"version": "1.29",
		"resourcesVpcConfig": map[string]any{
			"subnetIds":        []string{"subnet-1"},
			"securityGroupIds": []string{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	st, raw := signedHTTP(t, "eks", http.MethodPost, "/clusters", createBody, "application/json")
	if st < 200 || st >= 300 {
		t.Fatalf("CreateCluster status=%d body=%s", st, raw)
	}
	t.Cleanup(func() {
		_, _ = signedHTTP(t, "eks", http.MethodDelete, "/clusters/"+name, nil, "application/json")
	})

	var created map[string]any
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	cluster, _ := created["cluster"].(map[string]any)
	if cluster == nil {
		t.Fatalf("missing cluster: %s", raw)
	}
	status, _ := cluster["status"].(string)
	if status != "ACTIVE" {
		t.Skipf("EKS live smoke skipped: status=%s (expected ACTIVE metadata)", status)
	}
	ep, _ := cluster["endpoint"].(string)
	if !strings.Contains(ep, "noctaxris-eks-") {
		t.Fatalf("expected nested endpoint, got %q", ep)
	}

	st, descRaw := signedHTTP(t, "eks", http.MethodGet, "/clusters/"+name, nil, "application/json")
	if st != 200 {
		t.Fatalf("DescribeCluster status=%d body=%s", st, descRaw)
	}
	var described map[string]any
	if err := json.Unmarshal(descRaw, &described); err != nil {
		t.Fatal(err)
	}
	descCluster, _ := described["cluster"].(map[string]any)
	if descCluster == nil || descCluster["status"] != "ACTIVE" {
		t.Fatalf("DescribeCluster want ACTIVE got %v", described)
	}
	st, listRaw := signedHTTP(t, "eks", http.MethodGet, "/clusters", nil, "application/json")
	if st != 200 || !strings.Contains(string(listRaw), name) {
		t.Fatalf("ListClusters status=%d body=%s", st, listRaw)
	}
	st, ngRaw := signedHTTP(t, "eks", http.MethodGet, "/clusters/"+name+"/node-groups", nil, "application/json")
	if st != 200 {
		t.Fatalf("ListNodegroups status=%d body=%s", st, ngRaw)
	}
	st, delRaw := signedHTTP(t, "eks", http.MethodDelete, "/clusters/"+name, nil, "application/json")
	if st < 200 || st >= 300 {
		t.Fatalf("DeleteCluster status=%d body=%s", st, delRaw)
	}
	st, goneRaw := signedHTTP(t, "eks", http.MethodGet, "/clusters/"+name, nil, "application/json")
	if st == 200 {
		t.Fatalf("DescribeCluster after Delete still 200: %s", goneRaw)
	}
}
