package eks_test

import (
	"encoding/json"
	"testing"

	ekssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/eks"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEKSJSON(t *testing.T) {
	c := store.EKSCluster{
		Name: "lab", ARN: "arn:aws:eks:us-east-1:1:cluster/lab",
		CreatedAt: 1_700_000_000_000, Version: "1.29",
		Endpoint: "https://lab.eks.noctaxris.local", RoleARN: "arn:aws:iam::1:role/eks",
		ResourcesVpcJSON: `{"subnetIds":["subnet-1"],"securityGroupIds":["sg-1"]}`,
		Status: store.EKSClusterStatusActive,
	}
	createRaw, err := ekssvc.CreateClusterJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRaw, &createOut); err != nil {
		t.Fatal(err)
	}
	cluster, _ := createOut["cluster"].(map[string]any)
	vpc, _ := cluster["resourcesVpcConfig"].(map[string]any)
	if vpc["subnetIds"] == nil {
		t.Fatalf("create=%v", createOut)
	}

	c.ResourcesVpcJSON = "not-json"
	obj := ekssvc.ClusterJSON(c)
	if obj["resourcesVpcConfig"] == nil {
		t.Fatal("expected empty vpc map")
	}

	descRaw, err := ekssvc.DescribeClusterJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(descRaw, &createOut); err != nil {
		t.Fatal(err)
	}

	listRaw, err := ekssvc.ListClustersJSON([]store.EKSCluster{c, {Name: "other"}})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	names, _ := listOut["clusters"].([]any)
	if len(names) != 2 {
		t.Fatalf("list=%v", listOut)
	}

	delRaw, err := ekssvc.DeleteClusterJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(delRaw, &createOut); err != nil {
		t.Fatal(err)
	}
	cluster, _ = createOut["cluster"].(map[string]any)
	if cluster["status"] != store.EKSClusterStatusDeleting {
		t.Fatalf("delete status=%v", cluster["status"])
	}

	ngRaw, err := ekssvc.ListNodegroupsJSON()
	if err != nil {
		t.Fatal(err)
	}
	var ngOut map[string]any
	if err := json.Unmarshal(ngRaw, &ngOut); err != nil {
		t.Fatal(err)
	}
	ng, _ := ngOut["nodegroups"].([]any)
	if len(ng) != 0 {
		t.Fatalf("nodegroups=%v", ngOut)
	}
}
