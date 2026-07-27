package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMemoryDBClusterCRUD(t *testing.T) {
	st := openV7StreamBStore(t)
	account := "000000000001"
	c, err := st.CreateMemoryDBCluster(account, "us-east-1", "lab-memdb", "redis", "", "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "creating" || c.EndpointPort != store.MemoryDBNestedPort {
		t.Fatalf("cluster=%+v", c)
	}
	if !strings.HasSuffix(c.EndpointAddress, ".memorydb.noctaxris.internal") {
		t.Fatalf("endpoint=%q", c.EndpointAddress)
	}
	if c.ACLName != "open-access" {
		t.Fatalf("acl=%q", c.ACLName)
	}
	ep := store.MemoryDBNestedEndpoint(c.Name)
	if !strings.Contains(ep, ":6379") || strings.Contains(ep, "0.0.0.0") {
		t.Fatalf("nested endpoint=%q", ep)
	}
	got, err := st.DescribeMemoryDBCluster(account, "us-east-1", "lab-memdb")
	if err != nil || got.Engine != "redis" {
		t.Fatalf("describe=%+v err=%v", got, err)
	}
	list, err := st.DescribeMemoryDBClusters(account, "us-east-1", "")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if _, err := st.CreateMemoryDBCluster(account, "us-east-1", "lab-memdb", "valkey", "", "", "", 1); !errors.Is(err, store.ErrMemoryDBClusterExists) {
		t.Fatalf("dup err=%v", err)
	}
	if _, err := st.CreateMemoryDBCluster(account, "us-east-1", "bad", "memcached", "", "", "", 1); !errors.Is(err, store.ErrMemoryDBBadRequest) {
		t.Fatalf("engine err=%v", err)
	}
	if err := st.SetMemoryDBContainerID(account, "lab-memdb", "ctr-1", "available", ""); err != nil {
		t.Fatal(err)
	}
	ready, err := st.DescribeMemoryDBCluster(account, "us-east-1", "lab-memdb")
	if err != nil || ready.Status != "available" || ready.ContainerID != "ctr-1" {
		t.Fatalf("after nested start: %+v err=%v", ready, err)
	}
	ctr, err := st.DeleteMemoryDBCluster(account, "lab-memdb")
	if err != nil || ctr != "ctr-1" {
		t.Fatalf("delete ctr=%q err=%v", ctr, err)
	}
}
