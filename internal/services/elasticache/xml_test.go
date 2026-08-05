package elasticache_test

import (
	"strings"
	"testing"

	elasticachesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/elasticache"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestElastiCacheXML(t *testing.T) {
	req := "req-ec"
	c := store.ElastiCacheCluster{
		CacheClusterID: "lab-redis", Engine: "redis", CacheNodeType: "cache.t3.micro",
		NumCacheNodes: 1, Status: "available", EndpointPort: 6379,
	}
	create, err := elasticachesvc.CreateCacheClusterXML(c, req)
	if err != nil || !strings.Contains(string(create), "lab-redis") {
		t.Fatalf("create: %s %v", create, err)
	}
	desc, err := elasticachesvc.DescribeCacheClustersXML([]store.ElastiCacheCluster{c}, req)
	if err != nil || !strings.Contains(string(desc), "CacheClusters") {
		t.Fatalf("describe: %s %v", desc, err)
	}
	del, err := elasticachesvc.DeleteCacheClusterXML(c, req)
	if err != nil || !strings.Contains(string(del), "lab-redis") {
		t.Fatalf("delete: %s %v", del, err)
	}
	errXML, err := elasticachesvc.ErrorXML("CacheClusterNotFound", "missing", req)
	if err != nil || !strings.Contains(string(errXML), "CacheClusterNotFound") {
		t.Fatalf("error: %s %v", errXML, err)
	}
}
