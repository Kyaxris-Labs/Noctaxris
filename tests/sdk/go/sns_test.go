package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

func TestSNSTopicPublishRoundTrip(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newSNS(t, cfg)
	ctx := context.Background()

	name := uniquePrefix(t) + "-topic"
	created, err := client.CreateTopic(ctx, &sns.CreateTopicInput{Name: aws.String(name)})
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	if created.TopicArn == nil || *created.TopicArn == "" {
		t.Fatal("CreateTopic missing TopicArn")
	}
	topicARN := *created.TopicArn
	t.Cleanup(func() {
		_, _ = client.DeleteTopic(ctx, &sns.DeleteTopicInput{TopicArn: aws.String(topicARN)})
	})

	list, err := client.ListTopics(ctx, &sns.ListTopicsInput{})
	if err != nil {
		t.Fatalf("ListTopics: %v", err)
	}
	found := false
	for _, topic := range list.Topics {
		if topic.TopicArn != nil && *topic.TopicArn == topicARN {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListTopics missing %s", topicARN)
	}

	pub, err := client.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(topicARN),
		Message:  aws.String("sdk-sns"),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if pub.MessageId == nil || *pub.MessageId == "" {
		t.Fatal("Publish missing MessageId")
	}

	_, err = client.DeleteTopic(ctx, &sns.DeleteTopicInput{TopicArn: aws.String(topicARN)})
	if err != nil {
		t.Fatalf("DeleteTopic: %v", err)
	}
}
