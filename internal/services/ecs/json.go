package ecs

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const ecsJSONContentType = "application/x-amz-json-1.1"

// JSONContentType is the ECS JSON protocol content type.
func JSONContentType() string {
	return ecsJSONContentType
}

// RegisterTaskDefinitionJSON builds a RegisterTaskDefinition response.
func RegisterTaskDefinitionJSON(td store.ECSTaskDefinition) ([]byte, error) {
	return json.Marshal(map[string]any{
		"taskDefinition": taskDefinitionJSON(td),
	})
}

// DescribeTaskDefinitionJSON builds a DescribeTaskDefinition response.
func DescribeTaskDefinitionJSON(td store.ECSTaskDefinition) ([]byte, error) {
	return json.Marshal(map[string]any{
		"taskDefinition": taskDefinitionJSON(td),
	})
}

// ListTaskDefinitionsJSON builds a ListTaskDefinitions response.
func ListTaskDefinitionsJSON(arns []string) ([]byte, error) {
	return json.Marshal(map[string]any{"taskDefinitionArns": arns})
}

// DeregisterTaskDefinitionJSON builds a DeregisterTaskDefinition response.
func DeregisterTaskDefinitionJSON(td store.ECSTaskDefinition) ([]byte, error) {
	return json.Marshal(map[string]any{
		"taskDefinition": taskDefinitionJSON(td),
	})
}

// RunTaskJSON builds a RunTask response.
func RunTaskJSON(tasks []store.ECSTask) ([]byte, error) {
	entries := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		entries = append(entries, taskJSON(task))
	}
	return json.Marshal(map[string]any{"tasks": entries})
}

// DescribeTasksJSON builds a DescribeTasks response.
func DescribeTasksJSON(tasks []store.ECSTask) ([]byte, error) {
	entries := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		entries = append(entries, taskJSON(task))
	}
	return json.Marshal(map[string]any{"tasks": entries, "failures": []any{}})
}

// ListTasksJSON builds a ListTasks response.
func ListTasksJSON(taskARNs []string) ([]byte, error) {
	return json.Marshal(map[string]any{"taskArns": taskARNs})
}

// StopTaskJSON builds a StopTask response.
func StopTaskJSON(task store.ECSTask) ([]byte, error) {
	return json.Marshal(map[string]any{"task": taskJSON(task)})
}

// DescribeClustersJSON builds a DescribeClusters response.
func DescribeClustersJSON(clusters []store.ECSCluster) ([]byte, error) {
	entries := make([]map[string]any, 0, len(clusters))
	for _, c := range clusters {
		entries = append(entries, clusterJSON(c))
	}
	return json.Marshal(map[string]any{"clusters": entries, "failures": []any{}})
}

// ListClustersJSON builds a ListClusters response.
func ListClustersJSON(clusterARNs []string) ([]byte, error) {
	return json.Marshal(map[string]any{"clusterArns": clusterARNs})
}

func taskDefinitionJSON(td store.ECSTaskDefinition) map[string]any {
	return map[string]any{
		"taskDefinitionArn":  td.ARN,
		"family":             td.Family,
		"revision":           td.Revision,
		"containerDefinitions": td.ContainerDefs,
		"taskRoleArn":        td.TaskRoleARN,
		"executionRoleArn":   td.ExecutionRoleARN,
		"status":             td.Status,
		"registeredAt":       float64(0),
	}
}

func taskJSON(task store.ECSTask) map[string]any {
	out := map[string]any{
		"taskArn":           task.TaskARN,
		"clusterArn":        task.ClusterARN,
		"taskDefinitionArn": task.TaskDefARN,
		"lastStatus":        task.LastStatus,
		"containers":        task.Containers,
		"createdAt":         float64(0),
	}
	if task.StartedAt != "" {
		out["startedAt"] = float64(0)
	}
	if task.StoppedAt != "" {
		out["stoppedAt"] = float64(0)
	}
	return out
}

func clusterJSON(c store.ECSCluster) map[string]any {
	return map[string]any{
		"clusterArn":  c.ClusterARN,
		"clusterName": c.ClusterName,
		"status":      c.Status,
	}
}
