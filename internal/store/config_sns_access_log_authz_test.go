package store_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestConfigSNSNotifyRequiresRecorderRolePublish(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "config-alerts-deny", nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "config-alerts-deny-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowSNSSQSPolicy(t, st, account, q.QueueName)
	if _, err := st.Subscribe(account, topic.TopicARN, "sqs", q.QueueARN); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "config-bucket-deny"); err != nil {
		t.Fatal(err)
	}
	// Role trusted by config with s3:PutObject only (no sns:Publish).
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"config.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "config-no-sns", trust)
	if err != nil {
		t.Fatal(err)
	}
	allowS3 := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:PutObject","Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "config-delivery", allowS3); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigRecorder(account, "default", roleARN, "ALL"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigDeliveryChannel(account, "default", "config-bucket-deny", "", topic.TopicARN); err != nil {
		t.Fatal(err)
	}
	if err := st.StartConfigRecorder(account, "default"); err != nil {
		t.Fatal(err)
	}
	st.NotifyConfigDeliveryChannelsSNS(account, "default")
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("want no SNS notify without sns:Publish, got %+v", msgs)
	}
}

func TestS3AccessLogDeliveryHonorsTargetBucketDeny(t *testing.T) {
	st := openForensicsStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "access-src-deny"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "access-logs-deny"); err != nil {
		t.Fatal(err)
	}
	denyAll := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":"*","Action":"s3:PutObject","Resource":"arn:aws:s3:::access-logs-deny/*"}]}`
	if err := st.PutBucketPolicy(account, "access-logs-deny", denyAll); err != nil {
		t.Fatal(err)
	}
	if err := st.PutBucketLogging(account, "access-src-deny", store.S3AccessLoggingConfig{
		Enabled: true, TargetBucket: "access-logs-deny", TargetPrefix: "s3/",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendS3ServerAccessLog(account, "access-src-deny", store.S3ServerAccessLogInput{
		Operation: "REST.PUT.OBJECT", Key: "a.txt", RequestID: "req-1", HTTPStatus: 200, ObjectSize: 3,
		RequestTime: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListObjectsV2(account, "access-logs-deny", "s3/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Contents) != 0 {
		t.Fatalf("denied target policy must block log delivery, got %d objects", len(list.Contents))
	}
	_ = strings.Contains
}
