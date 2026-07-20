package store_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openV7StreamBStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestElastiCacheClusterCRUD(t *testing.T) {
	st := openV7StreamBStore(t)
	account := "000000000001"
	c, err := st.CreateElastiCacheCluster(account, "us-east-1", "lab-cache", "redis", "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "available" || c.EndpointPort != store.ElastiCacheNestedPort {
		t.Fatalf("cluster=%+v", c)
	}
	if !strings.HasSuffix(c.EndpointAddress, ".cache.noctaxris.internal") {
		t.Fatalf("endpoint=%q", c.EndpointAddress)
	}
	ep := store.ElastiCacheNestedEndpoint(c.CacheClusterID)
	if !strings.Contains(ep, ":6379") || strings.Contains(ep, "0.0.0.0") {
		t.Fatalf("nested endpoint=%q", ep)
	}
	got, err := st.DescribeElastiCacheCluster(account, "lab-cache")
	if err != nil || got.Engine != "redis" {
		t.Fatalf("describe=%+v err=%v", got, err)
	}
	list, err := st.DescribeElastiCacheClusters(account, "")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if _, err := st.CreateElastiCacheCluster(account, "us-east-1", "lab-cache", "valkey", "", "", 1); !errors.Is(err, store.ErrElastiCacheClusterExists) {
		t.Fatalf("dup err=%v", err)
	}
	if _, err := st.CreateElastiCacheCluster(account, "us-east-1", "bad", "memcached", "", "", 1); !errors.Is(err, store.ErrElastiCacheBadRequest) {
		t.Fatalf("engine err=%v", err)
	}
	if err := st.SetElastiCacheContainerID(account, "lab-cache", "ctr-1", "available", ""); err != nil {
		t.Fatal(err)
	}
	ctr, err := st.DeleteElastiCacheCluster(account, "lab-cache")
	if err != nil || ctr != "ctr-1" {
		t.Fatalf("delete ctr=%q err=%v", ctr, err)
	}
}

func TestDocDBClusterCRUD(t *testing.T) {
	st := openV7StreamBStore(t)
	account := "000000000001"
	c, err := st.CreateDocDBCluster(account, "us-east-1", "lab-docdb", "docdb", "", "labadmin", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "available" || c.EndpointPort != store.DocDBNestedPort {
		t.Fatalf("cluster=%+v", c)
	}
	if !strings.HasSuffix(c.EndpointAddress, ".docdb.noctaxris.internal") {
		t.Fatalf("endpoint=%q", c.EndpointAddress)
	}
	ep := store.DocDBNestedEndpoint(c.DBClusterIdentifier)
	if !strings.Contains(ep, ":27017") {
		t.Fatalf("nested endpoint=%q", ep)
	}
	if _, err := st.CreateDocDBCluster(account, "us-east-1", "neo", "neptune", "", "", 0); !errors.Is(err, store.ErrDocDBBadRequest) {
		t.Fatalf("neptune rejected? err=%v", err)
	}
	list, err := st.DescribeDocDBClusters(account, "")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.SetDocDBContainerID(account, "lab-docdb", "mongo-1", "available", ""); err != nil {
		t.Fatal(err)
	}
	ctr, err := st.DeleteDocDBCluster(account, "lab-docdb")
	if err != nil || ctr != "mongo-1" {
		t.Fatalf("delete ctr=%q err=%v", ctr, err)
	}
}
