package sdk_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
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

func TestSQSRedrivePolicyDlqSourceArn(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newSQS(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	dlqName := prefix + "-dlq"
	srcName := prefix + "-src"

	dlqCreated, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(dlqName)})
	if err != nil {
		t.Fatalf("CreateQueue dlq: %v", err)
	}
	dlqURL := aws.ToString(dlqCreated.QueueUrl)
	t.Cleanup(func() {
		_, _ = client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(dlqURL)})
	})

	dlqAttrs, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(dlqURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		t.Fatalf("GetQueueAttributes dlq: %v", err)
	}
	dlqARN := dlqAttrs.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]
	if dlqARN == "" {
		t.Fatal("GetQueueAttributes missing QueueArn")
	}

	redrive, err := json.Marshal(map[string]any{
		"deadLetterTargetArn": dlqARN,
		"maxReceiveCount":      "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	srcCreated, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(srcName),
		Attributes: map[string]string{
			"VisibilityTimeout": "0",
			"RedrivePolicy":     string(redrive),
		},
	})
	if err != nil {
		t.Fatalf("CreateQueue src: %v", err)
	}
	srcURL := aws.ToString(srcCreated.QueueUrl)
	t.Cleanup(func() {
		_, _ = client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(srcURL)})
	})

	srcAttrs, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(srcURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		t.Fatalf("GetQueueAttributes src: %v", err)
	}
	srcARN := srcAttrs.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]
	if srcARN == "" {
		t.Fatal("src QueueArn missing")
	}

	_, err = client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(srcURL),
		MessageBody: aws.String("poison"),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	first, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(srcURL),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     1,
	})
	if err != nil {
		t.Fatalf("ReceiveMessage first: %v", err)
	}
	if len(first.Messages) != 1 || first.Messages[0].ReceiptHandle == nil {
		t.Fatalf("first receive unexpected: %+v", first.Messages)
	}
	_, err = client.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(srcURL),
		ReceiptHandle:     first.Messages[0].ReceiptHandle,
		VisibilityTimeout: 0,
	})
	if err != nil {
		t.Fatalf("ChangeMessageVisibility: %v", err)
	}

	second, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(srcURL),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     1,
	})
	if err != nil {
		t.Fatalf("ReceiveMessage second: %v", err)
	}
	if len(second.Messages) != 0 {
		t.Fatalf("expected redrive on second receive, got %+v", second.Messages)
	}

	dlqRecv, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(dlqURL),
		MaxNumberOfMessages:   1,
		WaitTimeSeconds:       1,
		MessageAttributeNames: []string{"All"},
	})
	if err != nil {
		t.Fatalf("ReceiveMessage dlq: %v", err)
	}
	if len(dlqRecv.Messages) != 1 {
		t.Fatalf("dlq messages=%+v", dlqRecv.Messages)
	}
	if aws.ToString(dlqRecv.Messages[0].Body) != "poison" {
		t.Fatalf("dlq body=%q", aws.ToString(dlqRecv.Messages[0].Body))
	}
	prov, ok := dlqRecv.Messages[0].MessageAttributes["NoctaxrisDlqSourceArn"]
	if !ok {
		t.Fatalf("missing NoctaxrisDlqSourceArn attrs=%v", dlqRecv.Messages[0].MessageAttributes)
	}
	if aws.ToString(prov.StringValue) != srcARN {
		t.Fatalf("NoctaxrisDlqSourceArn=%q want %s", aws.ToString(prov.StringValue), srcARN)
	}
	if aws.ToString(prov.DataType) != "String" {
		t.Fatalf("DataType=%q", aws.ToString(prov.DataType))
	}
}
