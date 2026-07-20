package emr

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// RunJobFlowJSON builds a RunJobFlow success body.
func RunJobFlowJSON(c store.EMRCluster) ([]byte, error) {
	return json.Marshal(map[string]any{
		"JobFlowId":  c.ClusterID,
		"ClusterArn": c.ClusterARN,
	})
}

// DescribeClusterJSON builds a DescribeCluster success body.
func DescribeClusterJSON(c store.EMRCluster) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Cluster": map[string]any{
			"Id":           c.ClusterID,
			"Name":         c.ClusterName,
			"ClusterArn":   c.ClusterARN,
			"Status":       map[string]any{"State": c.Status},
			"ReleaseLabel": c.ReleaseLabel,
			"LogUri":       c.LogURI,
			"StatusTimeline": map[string]any{
				"CreationDateTime": time.UnixMilli(c.CreatedAt).UTC().Format(time.RFC3339),
			},
		},
	})
}

// ListClustersJSON builds a ListClusters success body.
func ListClustersJSON(clusters []store.EMRCluster) ([]byte, error) {
	summaries := make([]map[string]any, 0, len(clusters))
	for _, c := range clusters {
		summaries = append(summaries, map[string]any{
			"Id":         c.ClusterID,
			"Name":       c.ClusterName,
			"ClusterArn": c.ClusterARN,
			"Status":     map[string]any{"State": c.Status},
		})
	}
	return json.Marshal(map[string]any{"Clusters": summaries})
}
