package msk

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateClusterJSON builds a CreateCluster success body.
func CreateClusterJSON(c store.MSKCluster) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ClusterArn":  c.ClusterARN,
		"ClusterName": c.ClusterName,
		"State":       c.State,
	})
}

// DescribeClusterJSON builds a DescribeCluster success body.
func DescribeClusterJSON(c store.MSKCluster) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ClusterInfo": map[string]any{
			"ClusterArn":            c.ClusterARN,
			"ClusterName":           c.ClusterName,
			"State":                 c.State,
			"CurrentVersion":        "K1",
			"KafkaVersion":          c.KafkaVersion,
			"NumberOfBrokerNodes":   c.NumberOfBrokerNodes,
			"ZookeeperConnectString": "",
		},
	})
}

// ListClustersJSON builds a ListClusters success body.
func ListClustersJSON(clusters []store.MSKCluster) ([]byte, error) {
	list := make([]map[string]any, 0, len(clusters))
	for _, c := range clusters {
		list = append(list, map[string]any{
			"ClusterArn":  c.ClusterARN,
			"ClusterName": c.ClusterName,
			"State":       c.State,
			"KafkaVersion": c.KafkaVersion,
			"NumberOfBrokerNodes": c.NumberOfBrokerNodes,
		})
	}
	return json.Marshal(map[string]any{"ClusterInfoList": list})
}

// GetBootstrapBrokersJSON builds a GetBootstrapBrokers success body.
// Returns nested-network brokers only (never host-published).
func GetBootstrapBrokersJSON(c store.MSKCluster) ([]byte, error) {
	brokers := c.BootstrapBrokers
	if c.State != store.MSKClusterStateActive || c.ContainerID == "" {
		brokers = ""
	}
	return json.Marshal(map[string]any{
		"BootstrapBrokerString": brokers,
	})
}

// DeleteClusterJSON builds a DeleteCluster success body.
func DeleteClusterJSON(arn string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ClusterArn": arn,
		"State":      store.MSKClusterStateDeleting,
	})
}
