package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLogsSubscriptionForeignSQSOwnerAccount(t *testing.T) {
	st := openFabricStore(t)
	a := "000000000001"
	b := "000000000002"
	if err := st.EnsureRoot(a, "AKIAROOT000000000001", "sa"); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureRoot(b, "AKIAROOT000000000002", "sb"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogGroup(a, "us-east-1", "/xa/logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(a, "us-east-1", "/xa/logs", "s1"); err != nil {
		t.Fatal(err)
	}
	qOwner, err := st.CreateQueue(b, "us-east-1", "127.0.0.1:4566", "logs-xa", map[string]string{
		"Policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"logs.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	qSameName, err := st.CreateQueue(a, "us-east-1", "127.0.0.1:4566", "logs-xa", map[string]string{
		"Policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"logs.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutSubscriptionFilter(a, "/xa/logs", "f1", "ERROR", qOwner.QueueARN, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutLogEvents(a, "/xa/logs", "s1", "", []store.LogEvent{
		{Timestamp: 1, Message: "ERROR boom"},
	}); err != nil {
		t.Fatal(err)
	}
	ownerMsgs, err := st.ReceiveMessages(b, qOwner.QueueName, 1)
	if err != nil || len(ownerMsgs) != 1 {
		t.Fatalf("owner msgs=%d err=%v", len(ownerMsgs), err)
	}
	sameMsgs, err := st.ReceiveMessages(a, qSameName.QueueName, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sameMsgs) != 0 {
		t.Fatalf("filter-account same-name queue must not receive; got %d", len(sameMsgs))
	}
}

func TestAwslogsSubscriptionEnvelopeRoundTrip(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"messageType": "DATA_MESSAGE",
		"owner":       "000000000001",
		"logGroup":    "/g",
		"logEvents":   []map[string]any{{"timestamp": 1, "message": "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := store.EncodeAwslogsSubscriptionEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := store.DecodeAwslogsSubscriptionEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var msg map[string]any
	if err := json.Unmarshal(decoded, &msg); err != nil {
		t.Fatal(err)
	}
	if msg["messageType"] != "DATA_MESSAGE" {
		t.Fatalf("decoded=%s", decoded)
	}
}

func TestLogsSubscriptionLambdaAwslogsEnvelope(t *testing.T) {
	st := openFabricStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "logs-sub-fn",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(account, fn.FunctionName, "logs", "lambda:InvokeFunction",
		"logs.amazonaws.com", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogGroup(account, "us-east-1", "/lambda/sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(account, "us-east-1", "/lambda/sub", "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutSubscriptionFilter(account, "/lambda/sub", "f1", "", fn.FunctionARN, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutLogEvents(account, "/lambda/sub", "s1", "", []store.LogEvent{
		{Timestamp: 1, Message: "hello"},
	}); err != nil {
		t.Fatal(err)
	}
	job, err := st.LatestAsyncInvocation(account, "logs-sub-fn")
	if err != nil {
		t.Fatal(err)
	}
	var wrap map[string]any
	if err := json.Unmarshal([]byte(job.EventJSON), &wrap); err != nil {
		t.Fatal(err)
	}
	if _, ok := wrap["awslogs"]; !ok {
		t.Fatalf("payload missing awslogs: %s", job.EventJSON)
	}
	decoded, err := store.DecodeAwslogsSubscriptionEnvelope([]byte(job.EventJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(decoded), "DATA_MESSAGE") {
		t.Fatalf("decoded=%s", decoded)
	}
}

func TestMetricFilterLiteEmitsDatapoints(t *testing.T) {
	st := openFabricStore(t)
	account := "000000000001"
	if _, err := st.CreateLogGroup(account, "us-east-1", "/metrics/lab"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(account, "us-east-1", "/metrics/lab", "s1"); err != nil {
		t.Fatal(err)
	}
	groups, err := st.DescribeLogGroups(account, "/metrics")
	if err != nil || len(groups) != 1 || groups[0].MetricFilterCount != 0 {
		t.Fatalf("before put count: %#v err=%v", groups, err)
	}
	if _, err := st.PutMetricFilter(account, "/metrics/lab", "errors", "ERROR", "ErrorCount", "Lab/Logs", "1"); err != nil {
		t.Fatal(err)
	}
	groups, err = st.DescribeLogGroups(account, "/metrics")
	if err != nil || groups[0].MetricFilterCount != 1 {
		t.Fatalf("after put count: %#v err=%v", groups, err)
	}
	if _, _, err := st.PutLogEvents(account, "/metrics/lab", "s1", "", []store.LogEvent{
		{Timestamp: 10, Message: "INFO ok"},
		{Timestamp: 11, Message: "ERROR boom"},
	}); err != nil {
		t.Fatal(err)
	}
	dps, err := st.GetMetricData(account, "Lab/Logs", "ErrorCount", 0, 0)
	if err != nil || len(dps) != 1 || dps[0].Value != 1 {
		t.Fatalf("datapoints=%v err=%v", dps, err)
	}
}

func TestSNSHTTPPinnedDialerRejectsPrivateIP(t *testing.T) {
	_, err := store.PinnedSNSHTTPDialContext(t.Context(), "tcp", "10.0.0.5:443")
	if err == nil {
		t.Fatal("expected private IP dial reject")
	}
}
