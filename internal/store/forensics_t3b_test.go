package store_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openForensicsStore(t *testing.T) *store.Store {
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

func TestSNSRedrivePolicySendsFailedDeliveryToDLQ(t *testing.T) {
	st := openForensicsStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}
	dlq, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sns-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "sns-no-policy",
		RoleARN: "arn:aws:iam::000000000001:role/lambda-exec", Runtime: store.LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := st.Subscribe(account, "alerts", "lambda", fn.FunctionARN)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSubscriptionAttributes(sub.SubscriptionARN, map[string]string{
		"RedrivePolicy": `{"deadLetterTargetArn":"` + dlq.QueueARN + `"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Publish(account, "alerts", "hello", "", nil); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, "sns-dlq", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("dlq messages=%d want 1", len(msgs))
	}
	if !strings.Contains(string(msgs[0].Body), "hello") {
		t.Fatalf("dlq body=%q", string(msgs[0].Body))
	}
}

func TestEventBridgeTargetDLQAndDeliveryHistory(t *testing.T) {
	st := openForensicsStore(t)
	account := "000000000001"
	if _, err := st.PutRule(account, "us-east-1", "default", "dlq-rule", `{"source":["lab.t3"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	dlq, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "eb-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	logGroup := "/eb/dlq-test"
	if _, err := st.CreateLogGroup(account, "us-east-1", logGroup); err != nil {
		t.Fatal(err)
	}
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":["logs:PutLogEvents","logs:CreateLogStream"],"Resource":"*"}]}`
	if _, err := st.PutLogsResourcePolicy(account, "eb-to-logs", doc); err != nil {
		t.Fatal(err)
	}
	logARN := "arn:aws:logs:us-east-1:" + account + ":log-group:" + logGroup
	if err := st.PutTargets(account, "default", "dlq-rule", []store.EventTargetInput{{
		ID:            "t1",
		ARN:           logARN,
		DeadLetterARN: dlq.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteLogGroup(account, logGroup); err != nil {
		t.Fatal(err)
	}
	res, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source:     "lab.t3",
		DetailType: "Test",
		Detail:     `{"ok":true}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 || res.Entries[0].EventID == "" {
		t.Fatalf("put events=%+v", res)
	}
	history, err := st.ListEventTargetDeliveryHistory(account, "default", "dlq-rule", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("history=%+v", history)
	}
	if history[0].Status != "FAILED" {
		t.Fatalf("status=%q want FAILED", history[0].Status)
	}
	if !history[0].DLQSent {
		t.Fatalf("expected DLQ delivery history[0]=%+v", history[0])
	}
	msgs, err := st.ReceiveMessages(account, "eb-dlq", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("dlq len=%d", len(msgs))
	}
}

func TestPipesDLQOnTargetFailure(t *testing.T) {
	st := openForensicsStore(t)
	account := "000000000001"
	src, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-src", nil)
	if err != nil {
		t.Fatal(err)
	}
	dlq, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "pipe-target",
		RoleARN: "arn:aws:iam::000000000001:role/lambda-exec", Runtime: store.LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(account, "pipe-target", "pipes-allow", "lambda:InvokeFunction", "pipes.amazonaws.com", "", ""); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"pipes.amazonaws.com"},"Action":["sqs:ReceiveMessage","sqs:DeleteMessage"],"Resource":"` + src.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(account, "pipe-src", map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	p, err := st.CreatePipeWithEnrichment(account, "us-east-1", "lab-pipe", "", src.QueueARN, fn.FunctionARN, "", "", dlq.QueueARN, "RUNNING")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "pipe-src", []byte(`{"x":1}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	err = st.PollPipeOnce(account, p.Name, func(accountID, functionName, payloadJSON string) (string, error) {
		return "", errors.New("target invoke failed")
	})
	if err == nil {
		t.Fatal("expected target delivery failure")
	}
	msgs, err := st.ReceiveMessages(account, "pipe-dlq", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("pipe dlq len=%d", len(msgs))
	}
	var payload map[string]any
	if err := json.Unmarshal(msgs[0].Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["pipeArn"] != p.ARN {
		t.Fatalf("payload=%v", payload)
	}
}

func TestS3ObjectLockBlocksDeleteUntilRetainExpires(t *testing.T) {
	st := openForensicsStore(t)
	account := "000000000001"
	if _, err := st.CreateBucketWithOptions(account, "lock-bucket", store.CreateBucketOptions{ObjectLockEnabled: true}); err != nil {
		t.Fatal(err)
	}
	retain := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	_, err := st.PutObject(account, "lock-bucket", "obj.txt", store.PutObjectMeta{
		Data:                  []byte("secret"),
		ObjectLockMode:        "GOVERNANCE",
		ObjectLockRetainUntil: retain,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteObject(account, "lock-bucket", "obj.txt"); err == nil {
		t.Fatal("expected delete blocked by object lock")
	} else if !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("err=%v", err)
	}
	if err := st.DeleteObjectWithOptions(account, "lock-bucket", "obj.txt", store.DeleteObjectOptions{BypassGovernanceRetention: true}); err != nil {
		t.Fatalf("bypass delete: %v", err)
	}
}

func TestS3ServerAccessLoggingWritesToTargetBucket(t *testing.T) {
	st := openForensicsStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "access-src"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "access-logs"); err != nil {
		t.Fatal(err)
	}
	if err := st.PutBucketLogging(account, "access-src", store.S3AccessLoggingConfig{
		Enabled: true, TargetBucket: "access-logs", TargetPrefix: "s3/",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendS3ServerAccessLog(account, "access-src", store.S3ServerAccessLogInput{
		Operation: "REST.PUT.OBJECT", Key: "a.txt", RequestID: "req-1", HTTPStatus: 200, ObjectSize: 3,
	}); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListObjectsV2(account, "access-logs", "s3/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Contents) != 1 {
		t.Fatalf("log objects=%d", len(list.Contents))
	}
	_, data, err := st.GetObject(account, "access-logs", list.Contents[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "REST.PUT.OBJECT") || !strings.Contains(string(data), "a.txt") {
		t.Fatalf("log line=%q", string(data))
	}
}
