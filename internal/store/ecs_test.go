package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openECSStore(t *testing.T) *store.Store {
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

func TestRegisterTaskDefinitionRequiresBothRoles(t *testing.T) {
	st := openECSStore(t)
	account := "000000000001"
	containers := []map[string]any{
		{"name": "app", "image": "alpine:3.20"},
	}

	_, err := st.RegisterTaskDefinition(account, "us-east-1", store.RegisterTaskDefinitionInput{
		Family:           "lab-task",
		ContainerDefs:    containers,
		TaskRoleARN:      "",
		ExecutionRoleARN: "arn:aws:iam::000000000001:role/ecsExecutionRole",
	})
	if !errors.Is(err, store.ErrECSMissingTaskRoleARN) {
		t.Fatalf("missing task role: got %v want ErrECSMissingTaskRoleARN", err)
	}

	_, err = st.RegisterTaskDefinition(account, "us-east-1", store.RegisterTaskDefinitionInput{
		Family:           "lab-task",
		ContainerDefs:    containers,
		TaskRoleARN:      "arn:aws:iam::000000000001:role/ecsTaskRole",
		ExecutionRoleARN: "",
	})
	if !errors.Is(err, store.ErrECSMissingExecutionRoleARN) {
		t.Fatalf("missing execution role: got %v want ErrECSMissingExecutionRoleARN", err)
	}
}

func TestRegisterTaskDefinitionHappyPath(t *testing.T) {
	st := openECSStore(t)
	account := "000000000001"
	containers := []map[string]any{
		{"name": "app", "image": "alpine:3.20"},
	}

	td, err := st.RegisterTaskDefinition(account, "us-east-1", store.RegisterTaskDefinitionInput{
		Family:           "lab-task",
		ContainerDefs:    containers,
		TaskRoleARN:      "arn:aws:iam::000000000001:role/ecsTaskRole",
		ExecutionRoleARN: "arn:aws:iam::000000000001:role/ecsExecutionRole",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantARN := "arn:aws:ecs:us-east-1:000000000001:task-definition/lab-task:1"
	if td.Family != "lab-task" || td.Revision != 1 || td.ARN != wantARN || td.Status != store.ECSTaskDefinitionActive {
		t.Fatalf("registered=%+v want family=lab-task revision=1 arn=%q status=ACTIVE", td, wantARN)
	}
	if td.TaskRoleARN == "" || td.ExecutionRoleARN == "" {
		t.Fatalf("roles missing: %+v", td)
	}

	got, err := st.DescribeTaskDefinition(account, "lab-task", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.ARN != wantARN {
		t.Fatalf("describe=%+v", got)
	}

	arns, err := st.ListTaskDefinitions(account, "lab-task")
	if err != nil {
		t.Fatal(err)
	}
	if len(arns) != 1 || arns[0] != wantARN {
		t.Fatalf("list=%v", arns)
	}
}

func TestRunTaskLifecycle(t *testing.T) {
	st := openECSStore(t)
	account := "000000000001"
	containers := []map[string]any{
		{"name": "app", "image": "alpine:3.20"},
	}

	td, err := st.RegisterTaskDefinition(account, "us-east-1", store.RegisterTaskDefinitionInput{
		Family:           "run-task",
		ContainerDefs:    containers,
		TaskRoleARN:      "arn:aws:iam::000000000001:role/ecsTaskRole",
		ExecutionRoleARN: "arn:aws:iam::000000000001:role/ecsExecutionRole",
	})
	if err != nil {
		t.Fatal(err)
	}

	task, err := st.RunTask(account, "us-east-1", store.RunTaskInput{
		Cluster:        "default",
		TaskDefinition: td.ARN,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.LastStatus != store.ECSTaskStatusRunning {
		t.Fatalf("run status=%q want RUNNING", task.LastStatus)
	}
	if task.TaskARN == "" || task.StartedAt == "" {
		t.Fatalf("task=%+v", task)
	}
	wantPrefix := "arn:aws:ecs:us-east-1:000000000001:task/default/"
	if len(task.TaskARN) <= len(wantPrefix) || task.TaskARN[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("task arn=%q want prefix %q", task.TaskARN, wantPrefix)
	}

	described, err := st.DescribeTasks(account, "default", []string{task.TaskARN})
	if err != nil {
		t.Fatal(err)
	}
	if len(described) != 1 || described[0].LastStatus != store.ECSTaskStatusRunning {
		t.Fatalf("describe=%+v", described)
	}

	listed, err := st.ListTasks(account, "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].TaskARN != task.TaskARN {
		t.Fatalf("list=%+v", listed)
	}

	stopped, err := st.StopTask(account, "us-east-1", store.StopTaskInput{
		Cluster: "default",
		Task:    task.TaskARN,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.LastStatus != store.ECSTaskStatusStopped {
		t.Fatalf("stop status=%q want STOPPED", stopped.LastStatus)
	}
	if stopped.StoppedAt == "" {
		t.Fatal("stopped_at missing")
	}
}

func TestDeleteTask(t *testing.T) {
	st := openECSStore(t)
	account := "000000000001"
	containers := []map[string]any{
		{"name": "app", "image": "alpine:3.20"},
	}

	td, err := st.RegisterTaskDefinition(account, "us-east-1", store.RegisterTaskDefinitionInput{
		Family:           "delete-task",
		ContainerDefs:    containers,
		TaskRoleARN:      "arn:aws:iam::000000000001:role/ecsTaskRole",
		ExecutionRoleARN: "arn:aws:iam::000000000001:role/ecsExecutionRole",
	})
	if err != nil {
		t.Fatal(err)
	}

	task, err := st.RunTask(account, "us-east-1", store.RunTaskInput{
		Cluster:        "default",
		TaskDefinition: td.ARN,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteTask(account, task.TaskARN); err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListTasks(account, "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("list after delete=%+v want empty", listed)
	}
}

func TestDescribeListClustersDefault(t *testing.T) {
	st := openECSStore(t)
	account := "000000000001"

	clusters, err := st.ListClusters(account, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 || clusters[0].ClusterName != store.DefaultECSClusterName {
		t.Fatalf("list=%+v", clusters)
	}
	wantARN := "arn:aws:ecs:us-east-1:000000000001:cluster/default"
	if clusters[0].ClusterARN != wantARN {
		t.Fatalf("cluster arn=%q want %q", clusters[0].ClusterARN, wantARN)
	}

	described, err := st.DescribeClusters(account, "us-east-1", []string{"default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(described) != 1 || described[0].ClusterARN != wantARN {
		t.Fatalf("describe=%+v", described)
	}
}

func TestDeregisterTaskDefinition(t *testing.T) {
	st := openECSStore(t)
	account := "000000000001"
	containers := []map[string]any{
		{"name": "app", "image": "alpine:3.20"},
	}

	td, err := st.RegisterTaskDefinition(account, "us-east-1", store.RegisterTaskDefinitionInput{
		Family:           "deregister-me",
		ContainerDefs:    containers,
		TaskRoleARN:      "arn:aws:iam::000000000001:role/ecsTaskRole",
		ExecutionRoleARN: "arn:aws:iam::000000000001:role/ecsExecutionRole",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := st.DeregisterTaskDefinition(account, td.ARN); err != nil {
		t.Fatal(err)
	}
	got, err := st.DescribeTaskDefinition(account, "deregister-me", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.ECSTaskDefinitionInactive {
		t.Fatalf("status=%q want INACTIVE", got.Status)
	}
}

func TestDescribeServicesAcceptsServiceARN(t *testing.T) {
	st := openECSStore(t)
	account := "000000000001"
	containers := []map[string]any{
		{"name": "app", "image": "alpine:3.20"},
	}
	td, err := st.RegisterTaskDefinition(account, "us-east-1", store.RegisterTaskDefinitionInput{
		Family:           "svc-arn-lookup",
		ContainerDefs:    containers,
		TaskRoleARN:      "arn:aws:iam::000000000001:role/ecsTaskRole",
		ExecutionRoleARN: "arn:aws:iam::000000000001:role/ecsExecutionRole",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := st.CreateService(account, "us-east-1", store.CreateServiceInput{
		Cluster:        "default",
		ServiceName:    "lab-svc",
		TaskDefinition: td.ARN,
		DesiredCount:   0,
	})
	if err != nil {
		t.Fatal(err)
	}
	clusterARN := "arn:aws:ecs:us-east-1:000000000001:cluster/default"
	got, err := st.DescribeServices(account, clusterARN, []string{svc.ServiceARN})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ServiceName != "lab-svc" || got[0].ServiceARN != svc.ServiceARN {
		t.Fatalf("describe by ARN=%+v want lab-svc %q", got, svc.ServiceARN)
	}
}
