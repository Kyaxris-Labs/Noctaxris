package memorydb

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func clusterMap(c store.MemoryDBCluster) map[string]any {
	return map[string]any{
		"Name":           c.Name,
		"Status":         c.Status,
		"NumberOfShards": c.NumberOfShards,
		"NodeType":       c.NodeType,
		"Engine":         c.Engine,
		"EngineVersion":  c.EngineVersion,
		"ACLName":        c.ACLName,
		"TLSEnabled":     false,
		"ARN":            c.ARN,
		"ClusterEndpoint": map[string]any{
			"Address": c.EndpointAddress,
			"Port":    c.EndpointPort,
		},
	}
}

// CreateClusterJSON builds a CreateCluster success body.
func CreateClusterJSON(c store.MemoryDBCluster) ([]byte, error) {
	return json.Marshal(map[string]any{"Cluster": clusterMap(c)})
}

// DescribeClustersJSON builds a DescribeClusters success body.
func DescribeClustersJSON(clusters []store.MemoryDBCluster) ([]byte, error) {
	out := make([]map[string]any, 0, len(clusters))
	for _, c := range clusters {
		out = append(out, clusterMap(c))
	}
	return json.Marshal(map[string]any{"Clusters": out})
}

// DeleteClusterJSON builds a DeleteCluster success body.
func DeleteClusterJSON(c store.MemoryDBCluster) ([]byte, error) {
	return json.Marshal(map[string]any{"Cluster": clusterMap(c)})
}

// DescribeUsersJSON returns an empty Users list (lab stub).
func DescribeUsersJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"Users": []any{}})
}

// DescribeACLsJSON returns an empty ACLs list (lab stub).
func DescribeACLsJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"ACLs": []any{}})
}
