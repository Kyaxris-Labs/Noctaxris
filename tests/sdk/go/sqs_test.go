package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func TestSQSSendReceiveDeleteRoundTrip(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newSQS(t, cfg)
	ctx := context.Background()

	name := uniquePrefix(t) + "-q"
	created, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(name)})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	if created.QueueUrl == nil || *created.QueueUrl == "" {
		t.Fatal("CreateQueue missing QueueUrl")
	}
	queueURL := *created.QueueUrl
	t.Cleanup(func() {
		_, _ = client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(queueURL)})
	})

	_, err = client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String("sdk-sqs-body"),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	recv, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     1,
	})
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if len(recv.Messages) != 1 || recv.Messages[0].Body == nil || *recv.Messages[0].Body != "sdk-sqs-body" {
		t.Fatalf("ReceiveMessage unexpected: %+v", recv.Messages)
	}
	if recv.Messages[0].ReceiptHandle == nil {
		t.Fatal("missing ReceiptHandle")
	}

	_, err = client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: recv.Messages[0].ReceiptHandle,
	})
	if err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	_, err = client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(queueURL)})
	if err != nil {
		t.Fatalf("DeleteQueue: %v", err)
	}
}
