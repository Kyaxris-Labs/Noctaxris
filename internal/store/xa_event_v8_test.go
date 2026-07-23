package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEventBridgeForeignLambdaRoleArnRequiresResourcePolicy(t *testing.T) {
	st := openXAMessagingStore(t)
	owner := "000000000001"
	foreign := "000000000002"
	if err := st.EnsureRoot(foreign, "AKIAROOT000000000002", "secret-foreign"); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: foreign, Region: "us-east-1", FunctionName: "xa-fn",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + foreign + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(owner, "eb-xa-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "invoke", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"lambda:InvokeFunction","Resource":"`+fn.FunctionARN+`"}]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRule(owner, "us-east-1", "default", "xa-fn-rule", `{"source":["noctaxris.lab"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(owner, "default", "xa-fn-rule", []store.EventTargetInput{{
		ID: "1", ARN: fn.FunctionARN, RoleARN: roleARN,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutEvents(owner, []store.PutEventsEntry{{
		Source: "noctaxris.lab", DetailType: "x", Detail: `{"ok":true}`,
	}}); err != nil {
		t.Fatal(err)
	}
	// RoleArn Allows but foreign function has no resource policy → no async invoke.
	if _, err := st.LatestAsyncInvocation(foreign, fn.FunctionName); err == nil {
		t.Fatal("want 0 invokes without resource policy")
	}

	ruleARN := store.EventRuleARN("us-east-1", owner, "default", "xa-fn-rule")
	if err := st.EnsureRoot(owner, "AKIAROOT000000000001", "secret-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(foreign, fn.FunctionName, "eb", "lambda:InvokeFunction",
		"events.amazonaws.com", owner, ruleARN); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutEvents(owner, []store.PutEventsEntry{{
		Source: "noctaxris.lab", DetailType: "x", Detail: `{"ok":true}`,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LatestAsyncInvocation(foreign, fn.FunctionName); err != nil {
		t.Fatalf("want invoke after resource Allow: %v", err)
	}
}

func TestSNSForeignLambdaDelivery(t *testing.T) {
	st := openXAMessagingStore(t)
	topicOwner := "000000000001"
	fnOwner := "000000000002"
	if err := st.EnsureRoot(fnOwner, "AKIAROOT000000000002", "secret-fn"); err != nil {
		t.Fatal(err)
	}
	topic, err := st.CreateTopic(topicOwner, "us-east-1", "xa-sns", nil)
	if err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: fnOwner, Region: "us-east-1", FunctionName: "sns-fn",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + fnOwner + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureRoot(topicOwner, "AKIAROOT000000000001", "secret-topic"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(fnOwner, fn.FunctionName, "sns", "lambda:InvokeFunction",
		"sns.amazonaws.com", topicOwner, topic.TopicARN); err != nil {
		t.Fatal(err)
	}
	// Same-name function in topic owner must not receive delivery.
	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: topicOwner, Region: "us-east-1", FunctionName: "sns-fn",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + topicOwner + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(fnOwner, topic.TopicARN, "lambda", fn.FunctionARN); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Publish(topicOwner, topic.TopicName, `{"hi":1}`, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LatestAsyncInvocation(fnOwner, fn.FunctionName); err != nil {
		t.Fatalf("fnOwner should receive: %v", err)
	}
	if _, err := st.LatestAsyncInvocation(topicOwner, "sns-fn"); err == nil {
		t.Fatal("topicOwner same-name function must not receive")
	}
}

func TestSNSSubscribeRejectsForeignLambdaEndpoint(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "own-topic", nil)
	if err != nil {
		t.Fatal(err)
	}
	foreignARN := "arn:aws:lambda:us-east-1:000000000002:function:other"
	_, err = st.Subscribe(account, topic.TopicName, "lambda", foreignARN)
	if err == nil || !errors.Is(err, store.ErrSNSInvalidParameter) {
		t.Fatalf("want ErrSNSInvalidParameter got %v", err)
	}
}

func TestSchedulerForeignSQSWrongAccountDoesNotDeliver(t *testing.T) {
	st := openXAMessagingStore(t)
	schedAcct := "000000000001"
	foreign := "000000000002"
	if err := st.EnsureRoot(foreign, "AKIAROOT000000000002", "secret-f"); err != nil {
		t.Fatal(err)
	}
	qForeign, err := st.CreateQueue(foreign, "us-east-1", "127.0.0.1:4566", "sched-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	qLocal, err := st.CreateQueue(schedAcct, "us-east-1", "127.0.0.1:4566", "sched-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"scheduler.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + qForeign.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(foreign, qForeign.QueueName, map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"scheduler.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(schedAcct, "sched-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "send", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sqs:SendMessage","Resource":"`+qForeign.QueueARN+`"}]}`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	if _, err := st.CreateSchedule(schedAcct, "us-east-1", store.CreateScheduleInput{
		Name: "xa-sched", Expression: "rate(1 minutes)", TargetARN: qForeign.QueueARN, RoleARN: roleARN,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ProcessDueSchedules(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(foreign, qForeign.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("foreign msgs=%d err=%v", len(msgs), err)
	}
	localMsgs, err := st.ReceiveMessages(schedAcct, qLocal.QueueName, 1)
	if err != nil || len(localMsgs) != 0 {
		t.Fatalf("local same-name must be empty, got %d err=%v", len(localMsgs), err)
	}
}

func TestSchedulerStateMachineTarget(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sched-sfn-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"states.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	sfnRole, err := st.CreateRole(account, "sched-sfn-exec", trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(sfnRole, "task", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sqs:SendMessage","Resource":"`+q.QueueARN+`"}]}`); err != nil {
		t.Fatal(err)
	}
	def := `{"StartAt":"Send","States":{"Send":{"Type":"Task","Resource":"` + q.QueueARN + `","End":true}}}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "sched-sm", def, sfnRole)
	if err != nil {
		t.Fatal(err)
	}
	schedTrust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"scheduler.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	schedRole, err := st.CreateRole(account, "sched-to-sfn", schedTrust)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(schedRole, "start", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"states:StartExecution","Resource":"`+sm.StateMachineARN+`"}]}`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	if _, err := st.CreateSchedule(account, "us-east-1", store.CreateScheduleInput{
		Name: "to-sfn", Expression: "rate(1 minutes)", TargetARN: sm.StateMachineARN, RoleARN: schedRole,
		Input: `{"from":"scheduler"}`,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ProcessDueSchedules(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
}

func TestESMReportBatchItemFailuresSQS(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "rbif-fn",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "rbif-q", map[string]string{
		"VisibilityTimeout": "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	allowLambdaESMQueuePolicy(t, st, account, q.QueueName)
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID: account, FunctionName: fn.FunctionName, EventSourceARN: q.QueueARN, BatchSize: 2,
		FunctionResponseTypesJSON: `["ReportBatchItemFailures"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	id1, err := st.SendMessage(account, q.QueueName, []byte("a"), false, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := st.SendMessage(account, q.QueueName, []byte("b"), false, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PollEventSourceMappingOnce(m.UUID, func(_, _, _, _ string) (string, error) {
		body, _ := json.Marshal(map[string]any{
			"batchItemFailures": []map[string]string{{"itemIdentifier": id2.MessageID}},
		})
		return string(body), nil
	}); err != nil {
		t.Fatal(err)
	}
	remain, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(remain) != 1 || remain[0].MessageID != id2.MessageID {
		t.Fatalf("want only failed id2 remaining, got %+v (id1=%s)", remain, id1.MessageID)
	}
}

func TestESMReportBatchItemFailuresMalformedFailClosed(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "rbif-bad",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "rbif-bad-q", map[string]string{
		"VisibilityTimeout": "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	allowLambdaESMQueuePolicy(t, st, account, q.QueueName)
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID: account, FunctionName: fn.FunctionName, EventSourceARN: q.QueueARN,
		FunctionResponseTypesJSON: `["ReportBatchItemFailures"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte("x"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	err = st.PollEventSourceMappingOnce(m.UUID, func(_, _, _, _ string) (string, error) {
		return `{"batchItemFailures":[{"itemIdentifier":"not-a-real-id"}]}`, nil
	})
	if !errors.Is(err, store.ErrESMBatchItemFailures) {
		t.Fatalf("want ErrESMBatchItemFailures got %v", err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("fail-closed must leave message, got %d err=%v", len(msgs), err)
	}
}

func TestESMFilterCriteriaJSONBody(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "json-filt",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "json-filt-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowLambdaESMQueuePolicy(t, st, account, q.QueueName)
	fc := `{"Filters":[{"Pattern":"{\"body\":{\"status\":[\"ok\"]}}"}]}`
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID: account, FunctionName: fn.FunctionName, EventSourceARN: q.QueueARN,
		FilterCriteriaJSON: fc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte(`{"status":"drop"}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte(`{"status":"ok"}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	invoked := 0
	if err := st.PollEventSourceMappingOnce(m.UUID, func(_, _, _, eventJSON string) (string, error) {
		invoked++
		if !strings.Contains(eventJSON, "status") || !strings.Contains(eventJSON, "ok") {
			t.Fatalf("event=%s", eventJSON)
		}
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked != 1 {
		t.Fatalf("invoked=%d", invoked)
	}
}

func TestPutEventBusPermissionCondition(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	bus, err := st.CreateEventBus(account, "us-east-1", "cond-bus")
	if err != nil {
		t.Fatal(err)
	}
	cond := map[string]any{
		"StringEquals": map[string]any{"aws:SourceAccount": "000000000002"},
	}
	if err := st.PutEventBusPermission(account, bus.Name, "xa", "arn:aws:iam::000000000002:root",
		[]string{"events:PutEvents"}, cond); err != nil {
		t.Fatal(err)
	}
	got, err := st.DescribeEventBus(account, bus.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Policy, "aws:SourceAccount") || !strings.Contains(got.Policy, "000000000002") {
		t.Fatalf("policy missing Condition: %s", got.Policy)
	}
}

func TestPipesBusSourceRequiresRoleSessionAllow(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	bus, err := st.CreateEventBus(account, "us-east-1", "pipe-deny-bus")
	if err != nil {
		t.Fatal(err)
	}
	dst, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-deny-dst", map[string]string{
		"Policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"pipes.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"pipes.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "pipe-deny-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	// Role exists but no events:PutEvents Allow on bus.
	p, err := st.CreatePipe(account, "us-east-1", "deny-bus-pipe", "", bus.ARN, dst.QueueARN, roleARN, "RUNNING")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source: "noctaxris.lab", DetailType: "x", Detail: `{"a":1}`, EventBusName: "pipe-deny-bus",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.PollPipeOnce(account, p.Name, nil); err == nil {
		t.Fatal("expected source authz deny without events:PutEvents on RoleArn")
	}
}
