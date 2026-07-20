package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSNSFIFOTopicPublishAndSQSDelivery(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"

	if _, err := st.CreateTopic(account, "us-east-1", "orders", map[string]string{"FifoTopic": "true"}); !errors.Is(err, store.ErrSNSInvalidParameter) {
		t.Fatalf("want ErrSNSInvalidParameter got %v", err)
	}

	topic, err := st.CreateTopic(account, "us-east-1", "orders.fifo", map[string]string{
		"FifoTopic":                  "true",
		"ContentBasedDeduplication":  "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if topic.Attributes["FifoTopic"] != "true" {
		t.Fatalf("attrs=%v", topic.Attributes)
	}

	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "orders-q.fifo", map[string]string{
		"FifoQueue":                 "true",
		"ContentBasedDeduplication": "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(account, topic.TopicARN, "sqs", q.QueueARN); err != nil {
		t.Fatal(err)
	}

	if _, err := st.PublishWithOpts(account, "orders.fifo", "hello", "", nil, nil); !errors.Is(err, store.ErrSNSMissingMessageGroupID) {
		t.Fatalf("want missing group id got %v", err)
	}

	first, err := st.PublishWithOpts(account, "orders.fifo", "hello", "", nil, &store.PublishOpts{MessageGroupID: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	dup, err := st.PublishWithOpts(account, "orders.fifo", "hello", "", nil, &store.PublishOpts{MessageGroupID: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.MessageID != dup.MessageID {
		t.Fatalf("content-based dedup: first=%s dup=%s", first.MessageID, dup.MessageID)
	}

	msgs, err := st.ReceiveMessages(account, "orders-q.fifo", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 delivered message got %d", len(msgs))
	}
	if msgs[0].MessageGroupID != "g1" {
		t.Fatalf("group=%q", msgs[0].MessageGroupID)
	}
}

func TestSNSHTTPCatcherAllowlistAndDelivery(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "http-alerts", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.Subscribe(account, topic.TopicARN, "http", "https://evil.example/hook"); !errors.Is(err, store.ErrSNSEndpointNotAllowed) {
		t.Fatalf("want not allowlisted got %v", err)
	}

	endpoint := "http://127.0.0.1:4566" + store.LabSNSHTTPCatcherPath
	sub, err := st.Subscribe(account, topic.TopicARN, "http", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Confirmed {
		t.Fatal("http sub should start unconfirmed")
	}
	caught, err := st.ListSNSHTTPCatcher()
	if err != nil || len(caught) == 0 {
		t.Fatalf("want confirmation catcher row got %+v err=%v", caught, err)
	}
	if caught[0].MessageType != "SubscriptionConfirmation" {
		t.Fatalf("type=%q", caught[0].MessageType)
	}
	if !strings.Contains(caught[0].Body, "SubscribeURL") {
		t.Fatalf("body=%q", caught[0].Body)
	}

	confirmed, err := st.ConfirmSubscription(topic.TopicARN, sub.ConfirmToken)
	if err != nil || !confirmed.Confirmed {
		t.Fatalf("confirm=%+v err=%v", confirmed, err)
	}

	if _, err := st.Publish(account, "http-alerts", "payload", "", nil); err != nil {
		t.Fatal(err)
	}
	caught, err = st.ListSNSHTTPCatcher()
	if err != nil {
		t.Fatal(err)
	}
	foundNotify := false
	for _, m := range caught {
		if m.MessageType == "Notification" && strings.Contains(m.Body, "payload") {
			foundNotify = true
		}
	}
	if !foundNotify {
		t.Fatalf("missing notification in %+v", caught)
	}
}
