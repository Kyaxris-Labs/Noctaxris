package sns_test

import (
	"strings"
	"testing"

	snssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/sns"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSNSXML(t *testing.T) {
	req := "req-sns"
	if _, err := snssvc.ListTagsForResourceXML([]store.ResourceTag{{Key: "k", Value: "v"}}, req); err != nil {
		t.Fatal(err)
	}
	if _, err := snssvc.TagResourceXML(req); err != nil {
		t.Fatal(err)
	}
	if _, err := snssvc.UntagResourceXML(req); err != nil {
		t.Fatal(err)
	}
	topicARN := "arn:aws:sns:us-east-1:1:lab-topic"
	if _, err := snssvc.CreateTopicXML(topicARN, req); err != nil {
		t.Fatal(err)
	}
	if _, err := snssvc.PublishXML("msg-1", req); err != nil {
		t.Fatal(err)
	}
	topic := store.Topic{TopicARN: topicARN, TopicName: "lab-topic"}
	list, err := snssvc.ListTopicsXML([]store.Topic{topic}, req)
	if err != nil || !strings.Contains(string(list), topicARN) {
		t.Fatalf("list topics: %s %v", list, err)
	}
	attrs := map[string]string{"DisplayName": "Lab", "Owner": "000000000001"}
	if _, err := snssvc.GetTopicAttributesXML(attrs, req); err != nil {
		t.Fatal(err)
	}
	subARN := "arn:aws:sns:us-east-1:1:lab-topic:sub-1"
	if _, err := snssvc.SubscribeXML(subARN, req); err != nil {
		t.Fatal(err)
	}
	if _, err := snssvc.ConfirmSubscriptionXML(subARN, req); err != nil {
		t.Fatal(err)
	}
	sub := store.Subscription{SubscriptionARN: subARN, TopicARN: topicARN, Protocol: "email", Endpoint: "a@example.com"}
	if _, err := snssvc.ListSubscriptionsXML([]store.Subscription{sub}, req); err != nil {
		t.Fatal(err)
	}
	if _, err := snssvc.ListSubscriptionsByTopicXML([]store.Subscription{sub}, req); err != nil {
		t.Fatal(err)
	}
	subAttrs := map[string]string{"Owner": "000000000001", "PendingConfirmation": "false"}
	if _, err := snssvc.GetSubscriptionAttributesXML(subAttrs, req); err != nil {
		t.Fatal(err)
	}
	for _, fn := range []func(string) ([]byte, error){
		snssvc.DeleteTopicXML, snssvc.SetTopicAttributesXML, snssvc.UnsubscribeXML,
		snssvc.AddPermissionXML, snssvc.RemovePermissionXML,
	} {
		if _, err := fn(req); err != nil {
			t.Fatal(err)
		}
	}
}
