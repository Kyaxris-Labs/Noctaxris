package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEventSourceMappingCRUDAndPollDelete(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "esm-fn",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      "arn:aws:iam::" + account + ":role/lambda",
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "esm-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fn.FunctionName,
		EventSourceARN: q.QueueARN,
		BatchSize:      5,
		Enabled:        &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.UUID == "" || m.BatchSize != 5 || !m.Enabled || m.State != "Enabled" {
		t.Fatalf("mapping=%+v", m)
	}
	got, err := st.GetEventSourceMapping(account, m.UUID)
	if err != nil || got.UUID != m.UUID {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	listed, err := st.ListEventSourceMappings(account, fn.FunctionName)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}

	if _, err := st.SendMessage(account, q.QueueName, []byte(`{"ping":1}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	invoked := false
	err = st.PollEventSourceMappingOnce(m.UUID, func(acct, name, eventJSON string) error {
		invoked = true
		if acct != account || name != fn.FunctionName {
			t.Fatalf("invoke acct=%s name=%s", acct, name)
		}
		if !strings.Contains(eventJSON, "Records") || !strings.Contains(eventJSON, "aws:sqs") || !strings.Contains(eventJSON, q.QueueARN) {
			t.Fatalf("event=%s", eventJSON)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !invoked {
		t.Fatal("expected invoke")
	}
	remain, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(remain) != 0 {
		t.Fatalf("expected deleted messages, got %d", len(remain))
	}

	disabled := false
	updated, err := st.UpdateEventSourceMapping(store.UpdateEventSourceMappingInput{
		AccountID: account,
		UUID:      m.UUID,
		Enabled:   &disabled,
	})
	if err != nil || updated.Enabled || updated.State != "Disabled" {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	if err := st.DeleteEventSourceMapping(account, m.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetEventSourceMapping(account, m.UUID); !errors.Is(err, store.ErrNoSuchEventSourceMapping) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestEventSourceMappingPollLeavesOnInvokeError(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "esm-fail",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      "arn:aws:iam::" + account + ":role/lambda",
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "esm-fail-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fn.FunctionName,
		EventSourceARN: q.QueueARN,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte("x"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	invokeErr := errors.New("invoke failed")
	err = st.PollEventSourceMappingOnce(m.UUID, func(string, string, string) error {
		return invokeErr
	})
	if !errors.Is(err, invokeErr) {
		t.Fatalf("want invoke err, got %v", err)
	}
	// Message remains invisible under the queue visibility timeout (not deleted).
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected invisible (0 receive), got %d", len(msgs))
	}
}

func TestFunctionURLConfigCRUD(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaFunctionURLSchema(); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "url-fn",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      "arn:aws:iam::" + account + ":role/lambda",
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateFunctionURLConfig(store.CreateFunctionURLInput{
		AccountID:    account,
		FunctionName: fn.FunctionName,
		AuthType:     store.FunctionURLAuthNone,
		EndpointHost: "127.0.0.1:4566",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "http://127.0.0.1:4566/lambda-url/" + account + "/url-fn"
	if u.FunctionURL != wantURL || u.AuthType != store.FunctionURLAuthNone {
		t.Fatalf("url=%+v", u)
	}
	if _, err := st.CreateFunctionURLConfig(store.CreateFunctionURLInput{
		AccountID: account, FunctionName: fn.FunctionName, AuthType: store.FunctionURLAuthIAM,
	}); !errors.Is(err, store.ErrFunctionURLExists) {
		t.Fatalf("want conflict, got %v", err)
	}
	got, err := st.GetFunctionURLConfig(account, fn.FunctionName)
	if err != nil || got.FunctionURL != wantURL {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	listed, err := st.ListFunctionURLConfigs(account, "")
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	if err := st.DeleteFunctionURLConfig(account, fn.FunctionName); err != nil {
		t.Fatal(err)
	}
}

func TestECSServiceCRUDAndScaleDown(t *testing.T) {
	st := openECSStore(t)
	account := "000000000001"
	if err := st.EnsureECSServiceSchema(); err != nil {
		t.Fatal(err)
	}
	td, err := st.RegisterTaskDefinition(account, "us-east-1", store.RegisterTaskDefinitionInput{
		Family: "svc-family",
		ContainerDefs: []map[string]any{
			{"name": "app", "image": "public.ecr.aws/docker/library/alpine:3.20", "essential": true},
		},
		TaskRoleARN:      "arn:aws:iam::" + account + ":role/task",
		ExecutionRoleARN: "arn:aws:iam::" + account + ":role/exec",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := st.CreateService(account, "us-east-1", store.CreateServiceInput{
		Cluster:        "default",
		ServiceName:    "web",
		TaskDefinition: td.ARN,
		DesiredCount:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if svc.DesiredCount != 2 || svc.Status != "ACTIVE" {
		t.Fatalf("svc=%+v", svc)
	}
	task1, err := st.RunServiceTask(account, "us-east-1", "default", "web", td.ARN)
	if err != nil {
		t.Fatal(err)
	}
	task2, err := st.RunServiceTask(account, "us-east-1", "default", "web", td.ARN)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetService(account, "default", "web")
	if err != nil || got.RunningCount != 2 {
		t.Fatalf("running=%d err=%v", got.RunningCount, err)
	}
	zero := 0
	updated, err := st.UpdateService(account, "us-east-1", store.UpdateServiceInput{
		Cluster: "default", Service: "web", DesiredCount: &zero,
	})
	if err != nil || updated.DesiredCount != 0 {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	arns, err := st.ListServiceTaskARNs(account, "default", "web", store.ECSTaskStatusRunning)
	if err != nil || len(arns) != 2 {
		t.Fatalf("arns=%v err=%v", arns, err)
	}
	if _, err := st.StopTask(account, "us-east-1", store.StopTaskInput{Cluster: "default", Task: task1.TaskARN}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopTask(account, "us-east-1", store.StopTaskInput{Cluster: "default", Task: task2.TaskARN}); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetService(account, "default", "web")
	if err != nil || got.RunningCount != 0 {
		t.Fatalf("after stop running=%d err=%v", got.RunningCount, err)
	}
	listed, err := st.ListServices(account, "default")
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	if _, err := st.DeleteService(account, "default", "web"); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetService(account, "default", "web")
	if err != nil || got.Status != "INACTIVE" {
		t.Fatalf("deleted=%+v err=%v", got, err)
	}
}
