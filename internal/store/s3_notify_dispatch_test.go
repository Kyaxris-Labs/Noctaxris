package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func allowS3QueueNotify(t *testing.T, st *store.Store, account, bucket, queueName, queueARN string) {
	t.Helper()
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"s3.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + queueARN + `","Condition":{"ArnLike":{"aws:SourceArn":"` + store.BucketARN(bucket) + `"}}}]}`
	if err := st.SetQueueAttributes(account, queueName, map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
}

func TestS3NotifyDispatchSQSPutObject(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-sqs-emit"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-notify-emit", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowS3QueueNotify(t, st, account, bucket, q.QueueName, q.QueueARN)
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			ID:           "q1",
			Events:       []string{"s3:ObjectCreated:*"},
			FilterPrefix: "uploads/",
			FilterSuffix: ".txt",
			QueueARN:     q.QueueARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := st.PutObject(account, bucket, "other/nope.txt", store.PutObjectMeta{Data: []byte("x"), PlainSize: 1}); err != nil {
		t.Fatal(err)
	}
	if msgs, err := st.ReceiveMessages(account, q.QueueName, 10); err != nil {
		t.Fatal(err)
	} else if len(msgs) != 0 {
		t.Fatalf("prefix miss should not notify, got %d", len(msgs))
	}

	if _, err := st.PutObject(account, bucket, "uploads/hi.txt", store.PutObjectMeta{Data: []byte("hello"), PlainSize: 5, ETag: "abc"}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	body := string(msgs[0].Body)
	if !strings.Contains(body, `"eventSource":"aws:s3"`) || !strings.Contains(body, `"eventName":"ObjectCreated:Put"`) {
		t.Fatalf("body=%s", body)
	}
	if !strings.Contains(body, `"name":"`+bucket+`"`) || !strings.Contains(body, `uploads%2Fhi.txt`) {
		t.Fatalf("bucket/key missing body=%s", body)
	}
}

func TestS3NotifyDispatchDenyWithoutPolicyOnEmit(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-emit-deny"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-notify-deny", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowS3QueueNotify(t, st, account, bucket, q.QueueName, q.QueueARN)
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			Events:   []string{"s3:ObjectCreated:Put"},
			QueueARN: q.QueueARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetQueueAttributes(account, q.QueueName, map[string]string{"Policy": ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(account, bucket, "a.txt", store.PutObjectMeta{Data: []byte("z"), PlainSize: 1}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("emit must re-check authz; got %d msgs", len(msgs))
	}
}

func TestS3NotifyForeignLambdaServicePrincipalXA(t *testing.T) {
	st := openS3Store(t)
	bucketOwner := "000000000001"
	fnOwner := "000000000002"
	if err := st.EnsureRoot(bucketOwner, "AKIAROOT000000000001", "secret-bucket"); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureRoot(fnOwner, "AKIAROOT000000000002", "secret-fn"); err != nil {
		t.Fatal(err)
	}
	bucket := "xa-notify-bucket"
	if _, err := st.CreateBucket(bucketOwner, bucket); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    fnOwner,
		Region:       "us-east-1",
		FunctionName: "xa-s3-notify",
		RoleARN:      "arn:aws:iam::" + fnOwner + ":role/exec",
		Runtime:      "python3.12",
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(fnOwner, fn.FunctionName, "s3-xa", "lambda:InvokeFunction",
		"s3.amazonaws.com", bucketOwner, store.BucketARN(bucket)); err != nil {
		t.Fatal(err)
	}
	if err := st.PutBucketNotificationConfiguration(bucketOwner, bucket, store.S3NotificationConfig{
		LambdaConfigs: []store.S3LambdaFunctionConfig{{
			Events:      []string{"s3:ObjectCreated:Put"},
			FunctionARN: fn.FunctionARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(bucketOwner, bucket, "xa.bin", store.PutObjectMeta{Data: []byte("1"), PlainSize: 1}); err != nil {
		t.Fatal(err)
	}
	job, err := st.LatestAsyncInvocation(fnOwner, fn.FunctionName)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(job.EventJSON, `"eventName":"ObjectCreated:Put"`) || !strings.Contains(job.EventJSON, bucket) {
		t.Fatalf("async event=%s", job.EventJSON)
	}
}

func TestS3NotifyDispatchLambdaAsync(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-lambda-emit"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "s3-notify-async",
		RoleARN:      "arn:aws:iam::" + account + ":role/exec",
		Runtime:      "python3.12",
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(account, fn.FunctionName, "s3-invoke", "lambda:InvokeFunction", "s3.amazonaws.com", account, store.BucketARN(bucket)); err != nil {
		t.Fatal(err)
	}
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		LambdaConfigs: []store.S3LambdaFunctionConfig{{
			Events:      []string{"s3:ObjectCreated:Put"},
			FunctionARN: fn.FunctionARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(account, bucket, "obj.bin", store.PutObjectMeta{Data: []byte("1"), PlainSize: 1}); err != nil {
		t.Fatal(err)
	}
	job, err := st.LatestAsyncInvocation(account, fn.FunctionName)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(job.EventJSON, `"eventName":"ObjectCreated:Put"`) || !strings.Contains(job.EventJSON, bucket) {
		t.Fatalf("async event=%s", job.EventJSON)
	}
}

func TestS3NotifyDispatchEventBridgePattern(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "lab-notify-eb"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-eb-target", nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + q.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(account, q.QueueName, map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	pat := `{"source":["aws.s3"],"detail-type":["Object Created"],"detail":{"bucket":{"name":[{"prefix":"lab-"}]},"object":{"key":[{"suffix":".png"}]}}}`
	if _, err := st.PutRule(account, "us-east-1", "default", "s3-created", pat, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(account, "default", "s3-created", []store.EventTargetInput{{
		ID:  "t1",
		ARN: q.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		EventBridgeEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := st.PutObject(account, bucket, "skip.txt", store.PutObjectMeta{Data: []byte("x"), PlainSize: 1}); err != nil {
		t.Fatal(err)
	}
	if msgs, err := st.ReceiveMessages(account, q.QueueName, 10); err != nil {
		t.Fatal(err)
	} else if len(msgs) != 0 {
		t.Fatalf("suffix miss should not match rule, got %d", len(msgs))
	}

	if _, err := st.PutObject(account, bucket, "pic.png", store.PutObjectMeta{Data: []byte("img"), PlainSize: 3}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 eventbridge delivery, got %d", len(msgs))
	}
	var envelope map[string]any
	if err := json.Unmarshal(msgs[0].Body, &envelope); err != nil {
		// EventBridge may deliver the detail or full event depending on target input.
		if !strings.Contains(string(msgs[0].Body), bucket) && !strings.Contains(string(msgs[0].Body), "pic.png") {
			t.Fatalf("body=%s", msgs[0].Body)
		}
		return
	}
	raw := string(msgs[0].Body)
	if !strings.Contains(raw, "aws.s3") && !strings.Contains(raw, bucket) {
		t.Fatalf("unexpected eb body=%s", raw)
	}
}

func TestS3NotifyDispatchDeleteObject(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-delete"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-notify-del", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowS3QueueNotify(t, st, account, bucket, q.QueueName, q.QueueARN)
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			Events:   []string{"s3:ObjectRemoved:Delete"},
			QueueARN: q.QueueARN,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(account, bucket, "gone.txt", store.PutObjectMeta{Data: []byte("x"), PlainSize: 1}); err != nil {
		t.Fatal(err)
	}
	if msgs, err := st.ReceiveMessages(account, q.QueueName, 10); err != nil {
		t.Fatal(err)
	} else if len(msgs) != 0 {
		t.Fatalf("create should not match ObjectRemoved, got %d", len(msgs))
	}
	if err := st.DeleteObject(account, bucket, "gone.txt"); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || !strings.Contains(string(msgs[0].Body), `"eventName":"ObjectRemoved:Delete"`) {
		t.Fatalf("msgs=%v", msgs)
	}
}

func TestS3NotifyEmptyConfigNoEmit(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-off"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-notify-off", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowS3QueueNotify(t, st, account, bucket, q.QueueName, q.QueueARN)
	if _, err := st.PutObject(account, bucket, "x.txt", store.PutObjectMeta{Data: []byte("1"), PlainSize: 1}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("empty config must not emit, got %d", len(msgs))
	}
}
