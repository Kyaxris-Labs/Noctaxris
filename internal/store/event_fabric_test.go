package store_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openFabricStore(t *testing.T) *store.Store {
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

func TestEventBridgeInputTransformerDelivery(t *testing.T) {
	st := openFabricStore(t)
	account := "000000000001"
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "xfmr-q", map[string]string{
		"Policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRule(account, "us-east-1", "default", "xfmr-rule", `{"source":["noctaxris.lab"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(account, "default", "xfmr-rule", []store.EventTargetInput{{
		ID:  "1",
		ARN: q.QueueARN,
		InputTransformer: &store.EventBridgeInputTransformer{
			InputPathsMap: map[string]string{"status": "$.detail.status"},
			InputTemplate: `{"status": <status>}`,
		},
	}}); err != nil {
		t.Fatal(err)
	}
	result, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source: "noctaxris.lab", DetailType: "demo", Detail: `{"status":"ok"}`,
	}})
	if err != nil || result.FailedEntryCount != 0 {
		t.Fatalf("PutEvents err=%v result=%+v", err, result)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
	var body map[string]any
	if err := json.Unmarshal(msgs[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body=%s", msgs[0].Body)
	}
}

func TestEventBridgeLogsAndKinesisTargets(t *testing.T) {
	st := openFabricStore(t)
	account := "000000000001"
	if _, err := st.CreateLogGroup(account, "us-east-1", "/eb/lab"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateKinesisStream(account, "us-east-1", "eb-stream", 1); err != nil {
		t.Fatal(err)
	}
	roleARN, err := st.CreateRole(account, "eb-deliver", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	pol := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["logs:PutLogEvents","logs:CreateLogStream","kinesis:PutRecord"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "deliver", pol); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRule(account, "us-east-1", "default", "multi-tgt", `{"source":["noctaxris.lab"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	logARN := store.LogGroupARN("us-east-1", account, "/eb/lab")
	kinesisARN := store.KinesisStreamARN("us-east-1", account, "eb-stream")
	if err := st.PutTargets(account, "default", "multi-tgt", []store.EventTargetInput{
		{ID: "logs", ARN: logARN, RoleARN: roleARN},
		{ID: "kinesis", ARN: kinesisARN, RoleARN: roleARN},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source: "noctaxris.lab", DetailType: "demo", Detail: `{"n":1}`,
	}}); err != nil {
		t.Fatal(err)
	}
	events, err := st.GetLogEvents(account, "/eb/lab", store.LabEventBridgeLogStream, 0, 0, true, 10)
	if err != nil || len(events) == 0 {
		t.Fatalf("log events=%d err=%v", len(events), err)
	}
	it, err := st.GetKinesisShardIterator(account, "eb-stream", store.LabKinesisShardID, "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	recs, _, err := st.GetKinesisRecords(it, 10)
	if err != nil || len(recs) == 0 {
		t.Fatalf("kinesis recs=%d err=%v", len(recs), err)
	}
}

func TestEventBusPutPermissionXA(t *testing.T) {
	st := openFabricStore(t)
	owner := "000000000001"
	caller := "000000000002"
	if err := st.EnsureRoot(owner, "AKIAROOT000000000001", "secret-owner"); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureRoot(caller, "AKIAROOT000000000002", "secret-caller"); err != nil {
		t.Fatal(err)
	}
	bus, err := st.CreateEventBus(owner, "us-east-1", "shared")
	if err != nil {
		t.Fatal(err)
	}
	principal := "arn:aws:iam::" + caller + ":root"
	if err := st.PutEventBusPermission(owner, "shared", "allow-caller", principal, []string{"events:PutEvents"}); err != nil {
		t.Fatal(err)
	}
	bus, err = st.DescribeEventBus(owner, "shared")
	if err != nil || bus.Policy == "" {
		t.Fatalf("policy missing: %+v err=%v", bus, err)
	}
	if err := st.RemoveEventBusPermission(owner, "shared", "allow-caller"); err != nil {
		t.Fatal(err)
	}
}

func TestSFNTaskSQSAndEventBridge(t *testing.T) {
	st := openFabricStore(t)
	account := "000000000001"
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sfn-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	def := `{
  "StartAt": "Send",
  "States": {
    "Send": {
      "Type": "Task",
      "Resource": "` + q.QueueARN + `",
      "End": true
    }
  }
}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "sfn-sqs", def, "")
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartSFNExecution(account, "us-east-1", sm.StateMachineARN, "r1", `{"hello":true}`, nil)
	if err != nil || exec.Status != "SUCCEEDED" {
		t.Fatalf("exec=%+v err=%v", exec, err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
}

func TestPipesEventBusSource(t *testing.T) {
	st := openFabricStore(t)
	account := "000000000001"
	bus, err := st.CreateEventBus(account, "us-east-1", "pipe-bus")
	if err != nil {
		t.Fatal(err)
	}
	dst, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-dst", map[string]string{
		"Policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"pipes.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreatePipe(account, "us-east-1", "bus-pipe", "", bus.ARN, dst.QueueARN, "", "RUNNING")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source: "noctaxris.lab", DetailType: "x", Detail: `{"a":1}`, EventBusName: "pipe-bus",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.PollPipeOnce(account, p.Name, nil); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, dst.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
}

func TestESMFilterCriteriaSQS(t *testing.T) {
	st := openFabricStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "filt-fn",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "esm-filt", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowLambdaESMQueuePolicy(t, st, account, q.QueueName)
	fc := `{"Filters":[{"Pattern":"{\"body\":[\"keep-me\"]}"}]}`
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID: account, FunctionName: fn.FunctionName, EventSourceARN: q.QueueARN,
		FilterCriteriaJSON: fc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte("drop-me"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte("keep-me"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	invoked := 0
	if err := st.PollEventSourceMappingOnce(m.UUID, func(accountID, functionName, eventJSON string) error {
		invoked++
		var payload map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &payload)
		recs, _ := payload["Records"].([]any)
		if len(recs) != 1 {
			t.Fatalf("want 1 record got %d body=%s", len(recs), eventJSON)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked != 1 {
		t.Fatalf("invoked=%d", invoked)
	}
}

func TestLogsSubscriptionFilterToSQS(t *testing.T) {
	st := openFabricStore(t)
	account := "000000000001"
	if _, err := st.CreateLogGroup(account, "us-east-1", "/sub/lab"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(account, "us-east-1", "/sub/lab", "s1"); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "logs-sub", map[string]string{
		"Policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"logs.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutSubscriptionFilter(account, "/sub/lab", "f1", "ERROR", q.QueueARN, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutLogEvents(account, "/sub/lab", "s1", "", []store.LogEvent{
		{Timestamp: 1, Message: "INFO ok"},
		{Timestamp: 2, Message: "ERROR boom"},
	}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
}

func TestSNSXASubscribeForeignQueueDelivery(t *testing.T) {
	st := openFabricStore(t)
	a := "000000000001"
	b := "000000000002"
	if err := st.EnsureRoot(a, "AKIAROOT000000000001", "sa"); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureRoot(b, "AKIAROOT000000000002", "sb"); err != nil {
		t.Fatal(err)
	}
	topic, err := st.CreateTopic(a, "us-east-1", "xa-topic", nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(b, "us-east-1", "127.0.0.1:4566", "xa-q", map[string]string{
		"Policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(b, topic.TopicARN, "sqs", q.QueueARN); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Publish(a, topic.TopicName, "hello-xa", "", nil); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(b, q.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
}
