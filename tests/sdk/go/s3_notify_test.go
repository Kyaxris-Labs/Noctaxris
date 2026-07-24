package sdk_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func TestS3NotificationPutObjectToSQS(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	sqsc := newSQS(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower("sdk-s3n-" + prefix)
	qName := "sdk-s3n-q-" + prefix
	key := "notify.txt"

	_, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	qOut, err := sqsc.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(qName)})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	qURL := aws.ToString(qOut.QueueUrl)
	t.Cleanup(func() {
		_, _ = sqsc.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
	})

	attrs, err := sqsc.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(qURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		t.Fatalf("GetQueueAttributes: %v", err)
	}
	qARN := attrs.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]
	if qARN == "" {
		t.Fatal("GetQueueAttributes missing QueueArn")
	}

	bucketARN := "arn:aws:s3:::" + bucket
	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"s3.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"%s","Condition":{"ArnLike":{"aws:SourceArn":"%s"}}}]}`,
		qARN, bucketARN,
	)
	_, err = sqsc.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl: aws.String(qURL),
		Attributes: map[string]string{
			"Policy": policy,
		},
	})
	if err != nil {
		t.Fatalf("SetQueueAttributes: %v", err)
	}

	_, err = s3c.PutBucketNotificationConfiguration(ctx, &s3.PutBucketNotificationConfigurationInput{
		Bucket: aws.String(bucket),
		NotificationConfiguration: &s3types.NotificationConfiguration{
			QueueConfigurations: []s3types.QueueConfiguration{
				{
					Id:       aws.String("q1"),
					QueueArn: aws.String(qARN),
					Events:   []s3types.Event{s3types.EventS3ObjectCreated},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("PutBucketNotificationConfiguration: %v", err)
	}

	_, err = s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader([]byte("hello-notify")),
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		recv, err := sqsc.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(qURL),
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     1,
		})
		if err != nil {
			t.Fatalf("ReceiveMessage: %v", err)
		}
		if len(recv.Messages) == 0 {
			continue
		}
		body = aws.ToString(recv.Messages[0].Body)
		break
	}
	if body == "" {
		t.Fatal("expected S3 notification message on SQS")
	}
	if !strings.Contains(body, `"eventSource":"aws:s3"`) {
		t.Fatalf("missing eventSource: %s", body)
	}
	if !strings.Contains(body, bucket) || !strings.Contains(body, key) {
		t.Fatalf("want bucket %q and key %q in body=%s", bucket, key, body)
	}
}

func TestS3NotificationDenyWithoutQueuePolicy(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	sqsc := newSQS(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower("sdk-s3n-deny-" + prefix)
	qName := "sdk-s3n-deny-q-" + prefix
	key := "deny.txt"

	_, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	qOut, err := sqsc.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(qName)})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	qURL := aws.ToString(qOut.QueueUrl)
	t.Cleanup(func() {
		_, _ = sqsc.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
	})

	attrs, err := sqsc.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(qURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		t.Fatalf("GetQueueAttributes: %v", err)
	}
	qARN := attrs.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]
	if qARN == "" {
		t.Fatal("GetQueueAttributes missing QueueArn")
	}

	bucketARN := "arn:aws:s3:::" + bucket
	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"s3.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"%s","Condition":{"ArnLike":{"aws:SourceArn":"%s"}}}]}`,
		qARN, bucketARN,
	)
	_, err = sqsc.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl: aws.String(qURL),
		Attributes: map[string]string{
			"Policy": policy,
		},
	})
	if err != nil {
		t.Fatalf("SetQueueAttributes: %v", err)
	}

	_, err = s3c.PutBucketNotificationConfiguration(ctx, &s3.PutBucketNotificationConfigurationInput{
		Bucket: aws.String(bucket),
		NotificationConfiguration: &s3types.NotificationConfiguration{
			QueueConfigurations: []s3types.QueueConfiguration{
				{
					QueueArn: aws.String(qARN),
					Events:   []s3types.Event{s3types.EventS3ObjectCreatedPut},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("PutBucketNotificationConfiguration: %v", err)
	}

	// Clear policy after configure so emit-time authz re-check fails closed.
	_, err = sqsc.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl: aws.String(qURL),
		Attributes: map[string]string{
			"Policy": "",
		},
	})
	if err != nil {
		t.Fatalf("SetQueueAttributes clear policy: %v", err)
	}

	_, err = s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader([]byte("no-notify")),
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	recv, err := sqsc.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(qURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     2,
	})
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if len(recv.Messages) != 0 {
		t.Fatalf("emit must re-check queue policy; got %d messages", len(recv.Messages))
	}
}
