package eks

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// ClusterJSON builds the AWS-shaped cluster object for Create/Describe/Delete responses.
func ClusterJSON(c store.EKSCluster) map[string]any {
	vpc := map[string]any{}
	_ = json.Unmarshal([]byte(c.ResourcesVpcJSON), &vpc)
	if vpc == nil {
		vpc = map[string]any{}
	}
	// Nested endpoint string only; certificateAuthority is empty (no live kubectl).
	out := map[string]any{
		"name":                 c.Name,
		"arn":                  c.ARN,
		"createdAt":            float64(c.CreatedAt) / 1000.0,
		"version":              c.Version,
		"endpoint":             c.Endpoint,
		"roleArn":              c.RoleARN,
		"resourcesVpcConfig":   vpc,
		"status":               c.Status,
		"certificateAuthority": map[string]any{},
		"platformVersion":      "eks.1",
	}
	return out
}

// CreateClusterJSON builds a CreateCluster success body.
func CreateClusterJSON(c store.EKSCluster) ([]byte, error) {
	return json.Marshal(map[string]any{"cluster": ClusterJSON(c)})
}

// DescribeClusterJSON builds a DescribeCluster success body.
func DescribeClusterJSON(c store.EKSCluster) ([]byte, error) {
	return json.Marshal(map[string]any{"cluster": ClusterJSON(c)})
}

// ListClustersJSON builds a ListClusters success body (name list).
func ListClustersJSON(clusters []store.EKSCluster) ([]byte, error) {
	names := make([]string, 0, len(clusters))
	for _, c := range clusters {
		names = append(names, c.Name)
	}
	return json.Marshal(map[string]any{"clusters": names})
}

// DeleteClusterJSON builds a DeleteCluster success body (cluster enters DELETING).
func DeleteClusterJSON(c store.EKSCluster) ([]byte, error) {
	c.Status = store.EKSClusterStatusDeleting
	return json.Marshal(map[string]any{"cluster": ClusterJSON(c)})
}

// ListNodegroupsJSON returns an empty nodegroup name list stub.
func ListNodegroupsJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"nodegroups": []string{}})
}
