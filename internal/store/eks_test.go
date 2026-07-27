package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEKSClusterCRUD(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	c, err := st.CreateEKSCluster(account, "us-east-1", "lab-eks-1",
		"arn:aws:iam::000000000001:role/eks", "1.29",
		`{"subnetIds":["subnet-1"],"securityGroupIds":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != store.EKSClusterStatusActive {
		t.Fatalf("status=%q want ACTIVE (metadata-only)", c.Status)
	}
	if c.Endpoint != "https://noctaxris-eks-lab-eks-1:6443" {
		t.Fatalf("endpoint=%q", c.Endpoint)
	}
	if c.ARN != "arn:aws:eks:us-east-1:000000000001:cluster/lab-eks-1" {
		t.Fatalf("arn=%q", c.ARN)
	}
	if _, err := st.CreateEKSCluster(account, "us-east-1", "lab-eks-1",
		"arn:aws:iam::000000000001:role/eks", "", "{}"); !errors.Is(err, store.ErrEKSClusterExists) {
		t.Fatalf("exists? err=%v", err)
	}
	if _, err := st.CreateEKSCluster(account, "us-east-1", "", "arn:aws:iam::000000000001:role/eks", "", "{}"); !errors.Is(err, store.ErrEKSBadRequest) {
		t.Fatalf("empty name? err=%v", err)
	}
	if _, err := st.CreateEKSCluster(account, "us-east-1", "x", "", "", "{}"); !errors.Is(err, store.ErrEKSBadRequest) {
		t.Fatalf("empty role? err=%v", err)
	}
	got, err := st.DescribeEKSCluster(account, "lab-eks-1")
	if err != nil || got.Status != store.EKSClusterStatusActive {
		t.Fatalf("describe %+v err=%v", got, err)
	}
	list, err := st.ListEKSClusters(account)
	if err != nil || len(list) != 1 || list[0].Name != "lab-eks-1" {
		t.Fatalf("list=%v err=%v", list, err)
	}
	del, err := st.DeleteEKSCluster(account, "lab-eks-1")
	if err != nil || del.Status != store.EKSClusterStatusDeleting {
		t.Fatalf("delete %+v err=%v", del, err)
	}
	if _, err := st.DescribeEKSCluster(account, "lab-eks-1"); !errors.Is(err, store.ErrEKSClusterNotFound) {
		t.Fatalf("after delete err=%v", err)
	}
}
