package ecs_test

import (
	"encoding/json"
	"testing"

	ecssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ecs"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestECSJSON(t *testing.T) {
	if ecssvc.JSONContentType() == "" {
		t.Fatal("content type")
	}
	td := store.ECSTaskDefinition{
		Family: "web", Revision: 1, ARN: "arn:aws:ecs:us-east-1:1:task-definition/web:1",
		TaskRoleARN: "arn:aws:iam::1:role/task", ExecutionRoleARN: "arn:aws:iam::1:role/exec",
		Status: store.ECSTaskDefinitionActive, RegisteredAt: "2024-01-01T00:00:00Z",
		ContainerDefs: []map[string]any{{"name": "app", "image": "nginx"}},
	}
	if _, err := ecssvc.RegisterTaskDefinitionJSON(td); err != nil {
		t.Fatal(err)
	}
	desc, err := ecssvc.DescribeTaskDefinitionJSON(td)
	if err != nil {
		t.Fatal(err)
	}
	var tdOut map[string]any
	if err := json.Unmarshal(desc, &tdOut); err != nil {
		t.Fatal(err)
	}
	if tdOut["taskDefinition"] == nil {
		t.Fatalf("taskDefinition missing: %v", tdOut)
	}
	if _, err := ecssvc.ListTaskDefinitionsJSON([]string{td.ARN}); err != nil {
		t.Fatal(err)
	}
	if _, err := ecssvc.DeregisterTaskDefinitionJSON(td); err != nil {
		t.Fatal(err)
	}

	task := store.ECSTask{
		TaskARN: "arn:aws:ecs:us-east-1:1:task/cluster/t1", ClusterARN: "arn:aws:ecs:us-east-1:1:cluster/default",
		TaskDefARN: td.ARN, LastStatus: store.ECSTaskStatusRunning,
	}
	if _, err := ecssvc.RunTaskJSON([]store.ECSTask{task}); err != nil {
		t.Fatal(err)
	}
	if _, err := ecssvc.DescribeTasksJSON([]store.ECSTask{task}); err != nil {
		t.Fatal(err)
	}
	if _, err := ecssvc.ListTasksJSON([]string{task.TaskARN}); err != nil {
		t.Fatal(err)
	}
	if _, err := ecssvc.StopTaskJSON(task); err != nil {
		t.Fatal(err)
	}

	cluster := store.ECSCluster{ClusterName: "default", ClusterARN: "arn:aws:ecs:us-east-1:1:cluster/default", Status: "ACTIVE"}
	if _, err := ecssvc.DescribeClustersJSON([]store.ECSCluster{cluster}); err != nil {
		t.Fatal(err)
	}
	if _, err := ecssvc.ListClustersJSON([]string{cluster.ClusterARN}); err != nil {
		t.Fatal(err)
	}

	svc := store.ECSService{
		ServiceName: "api", ServiceARN: "arn:aws:ecs:us-east-1:1:service/default/api",
		ClusterARN: cluster.ClusterARN, TaskDefinition: td.ARN, DesiredCount: 1,
	}
	if _, err := ecssvc.CreateServiceJSON(svc); err != nil {
		t.Fatal(err)
	}
	if _, err := ecssvc.DescribeServicesJSON([]store.ECSService{svc}); err != nil {
		t.Fatal(err)
	}
	if _, err := ecssvc.ListServicesJSON([]string{svc.ServiceARN}); err != nil {
		t.Fatal(err)
	}
	if _, err := ecssvc.ListTagsForResourceJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := ecssvc.EmptyOKJSON(); err != nil {
		t.Fatal(err)
	}
}
