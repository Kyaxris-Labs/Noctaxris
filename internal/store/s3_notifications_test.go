package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3NotificationConfigurationPutGetRoundTrip(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-roundtrip"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "s3-notify-fn",
		RoleARN:      "arn:aws:iam::" + account + ":role/exec",
		Runtime:      "python3.12",
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	bucketARN := store.BucketARN(bucket)
	if _, err := st.AddFunctionPermission(account, fn.FunctionName, "s3-invoke", "lambda:InvokeFunction", "s3.amazonaws.com", account, bucketARN); err != nil {
		t.Fatal(err)
	}

	cfg := store.S3NotificationConfig{
		LambdaConfigs: []store.S3LambdaFunctionConfig{{
			ID:           "lambda-1",
			Events:       []string{"s3:ObjectCreated:*"},
			FilterPrefix: "uploads/",
			FilterSuffix: ".png",
			FunctionARN:  fn.FunctionARN,
		}},
	}
	if err := st.PutBucketNotificationConfiguration(account, bucket, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBucketNotificationConfiguration(account, bucket)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.LambdaConfigs) != 1 {
		t.Fatalf("lambda configs=%d want 1", len(got.LambdaConfigs))
	}
	lc := got.LambdaConfigs[0]
	if lc.FunctionARN != fn.FunctionARN || lc.FilterPrefix != "uploads/" || lc.FilterSuffix != ".png" {
		t.Fatalf("lambda config=%+v", lc)
	}
	if len(lc.Events) != 1 || lc.Events[0] != "s3:ObjectCreated:*" {
		t.Fatalf("events=%v", lc.Events)
	}

	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{}); err != nil {
		t.Fatal(err)
	}
	cleared, err := st.GetBucketNotificationConfiguration(account, bucket)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.LambdaConfigs) != 0 || len(cleared.QueueConfigs) != 0 || len(cleared.TopicConfigs) != 0 || cleared.EventBridgeEnabled {
		t.Fatalf("want empty config, got %+v", cleared)
	}
}

func TestS3NotificationConfigurationDefaultEmpty(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "notify-empty"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBucketNotificationConfiguration(account, "notify-empty")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.LambdaConfigs) != 0 || len(got.QueueConfigs) != 0 || got.EventBridgeEnabled {
		t.Fatalf("default should be empty, got %+v", got)
	}
}

func TestS3NotificationConfigurationRejectsUnknownEvent(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "notify-bad-event"); err != nil {
		t.Fatal(err)
	}
	err := st.PutBucketNotificationConfiguration(account, "notify-bad-event", store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			ID:       "q1",
			Events:   []string{"s3:Replication:OperationFailedReplication"},
			QueueARN: "arn:aws:sqs:us-east-1:" + account + ":missing",
		}},
	})
	if err == nil || !errors.Is(err, store.ErrInvalidS3NotificationConfiguration) {
		t.Fatalf("want ErrInvalidS3NotificationConfiguration, got %v", err)
	}
}

func TestS3NotificationConfigurationRequiresDestinationPolicy(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	bucket := "notify-deny"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "s3-notify-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			ID:       "q1",
			Events:   []string{"s3:ObjectCreated:Put"},
			QueueARN: q.QueueARN,
		}},
	})
	if err == nil || !errors.Is(err, store.ErrInvalidS3NotificationConfiguration) {
		t.Fatalf("want destination policy rejection, got %v", err)
	}
	if !strings.Contains(err.Error(), "destination") {
		t.Fatalf("err=%v", err)
	}

	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"s3.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + q.QueueARN + `","Condition":{"ArnLike":{"aws:SourceArn":"` + store.BucketARN(bucket) + `"}}}]}`
	if err := st.SetQueueAttributes(account, q.QueueName, map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutBucketNotificationConfiguration(account, bucket, store.S3NotificationConfig{
		QueueConfigs: []store.S3QueueConfig{{
			ID:       "q1",
			Events:   []string{"s3:ObjectCreated:Put"},
			QueueARN: q.QueueARN,
		}},
		EventBridgeEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBucketNotificationConfiguration(account, bucket)
	if err != nil {
		t.Fatal(err)
	}
	if !got.EventBridgeEnabled || len(got.QueueConfigs) != 1 {
		t.Fatalf("got=%+v", got)
	}
}

func TestS3NotificationConfigurationRejectsBadARNShape(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "notify-bad-arn"); err != nil {
		t.Fatal(err)
	}
	err := st.PutBucketNotificationConfiguration(account, "notify-bad-arn", store.S3NotificationConfig{
		LambdaConfigs: []store.S3LambdaFunctionConfig{{
			Events:      []string{"s3:ObjectCreated:*"},
			FunctionARN: "https://evil.example/open-proxy",
		}},
	})
	if err == nil || !errors.Is(err, store.ErrInvalidS3NotificationConfiguration) {
		t.Fatalf("want ARN shape rejection, got %v", err)
	}
}
