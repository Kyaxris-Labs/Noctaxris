package store_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openSQSStore(t *testing.T) *store.Store {
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

func TestCreateQueueGetList(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"

	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", map[string]string{"VisibilityTimeout": "30"})
	if err != nil {
		t.Fatal(err)
	}
	if q.QueueURL != "http://127.0.0.1:4566/000000000001/jobs" {
		t.Fatalf("url=%q", q.QueueURL)
	}
	if q.QueueARN != "arn:aws:sqs:us-east-1:000000000001:jobs" {
		t.Fatalf("arn=%q", q.QueueARN)
	}
	if q.Attributes["VisibilityTimeout"] != "30" {
		t.Fatalf("attrs=%+v", q.Attributes)
	}

	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", nil); !errors.Is(err, store.ErrQueueAlreadyExists) {
		t.Fatalf("want ErrQueueAlreadyExists, got %v", err)
	}

	byURL, err := st.GetQueueByURL(q.QueueURL)
	if err != nil {
		t.Fatal(err)
	}
	if byURL.QueueName != "jobs" {
		t.Fatalf("byURL=%+v", byURL)
	}

	if _, err := st.GetQueue(account, "missing"); !errors.Is(err, store.ErrNoSuchQueue) {
		t.Fatalf("want ErrNoSuchQueue, got %v", err)
	}

	queues, err := st.ListQueues(account, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(queues) != 1 {
		t.Fatalf("queues=%+v", queues)
	}
}

func TestSendReceiveDeleteMessage(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", nil); err != nil {
		t.Fatal(err)
	}

	sent, err := st.SendMessage(account, "jobs", []byte("hello"), false, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if sent.MessageID == "" {
		t.Fatalf("sent=%+v", sent)
	}

	got, err := st.ReceiveMessages(account, "jobs", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 message, got %d", len(got))
	}
	msg := got[0]
	if !bytes.Equal(msg.Body, []byte("hello")) || msg.ReceiptHandle == "" || msg.ReceiveCount != 1 {
		t.Fatalf("msg=%+v", msg)
	}

	if again, err := st.ReceiveMessages(account, "jobs", 10); err != nil {
		t.Fatal(err)
	} else if len(again) != 0 {
		t.Fatalf("expected message hidden, got %d", len(again))
	}

	if err := st.DeleteMessage(account, "jobs", msg.ReceiptHandle); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteMessage(account, "jobs", "stale-handle"); !errors.Is(err, store.ErrNoSuchMessage) {
		t.Fatalf("want ErrNoSuchMessage, got %v", err)
	}
}

func TestChangeMessageVisibilityMakesVisible(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", map[string]string{"VisibilityTimeout": "300"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "jobs", []byte("payload"), false, nil, ""); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReceiveMessages(account, "jobs", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	if err := st.ChangeMessageVisibility(account, "jobs", got[0].ReceiptHandle, 0); err != nil {
		t.Fatal(err)
	}
	again, err := st.ReceiveMessages(account, "jobs", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].ReceiveCount != 2 {
		t.Fatalf("again=%+v", again)
	}
}

func TestPurgeQueue(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessageBatch(account, "jobs", [][]byte{[]byte("a"), []byte("b"), []byte("c")}); err != nil {
		t.Fatal(err)
	}
	if err := st.PurgeQueue(account, "jobs"); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReceiveMessages(account, "jobs", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty queue, got %d", len(got))
	}
}

func TestSetQueueAttributesMergesPolicy(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", map[string]string{"VisibilityTimeout": "30"}); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[]}`
	if err := st.SetQueueAttributes(account, "jobs", map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	attrs, err := st.GetQueueAttributes(account, "jobs")
	if err != nil {
		t.Fatal(err)
	}
	if attrs["Policy"] != policy {
		t.Fatalf("policy attr=%q", attrs["Policy"])
	}
	if attrs["VisibilityTimeout"] != "30" {
		t.Fatalf("expected merge to keep VisibilityTimeout, got %+v", attrs)
	}
}

func TestSealedMessageRoundTrip(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "secure", map[string]string{"KmsMasterKeyId": "key-1"}); err != nil {
		t.Fatal(err)
	}
	cipher := []byte{0x10, 0x20, 0x30}
	dek := []byte{0x99}
	if _, err := st.SendMessage(account, "secure", cipher, true, dek, `{"trace":{"StringValue":"x"}}`); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReceiveMessages(account, "secure", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	m := got[0]
	if !m.Sealed || !bytes.Equal(m.Body, cipher) || !bytes.Equal(m.SealedDEK, dek) {
		t.Fatalf("message=%+v", m)
	}
	if m.AttributesJSON != `{"trace":{"StringValue":"x"}}` {
		t.Fatalf("attrs json=%q", m.AttributesJSON)
	}
}

func TestDeleteQueueRemovesMessages(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "jobs", []byte("x"), false, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteQueue(account, "jobs"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetQueue(account, "jobs"); !errors.Is(err, store.ErrNoSuchQueue) {
		t.Fatalf("want ErrNoSuchQueue, got %v", err)
	}
	if err := st.DeleteQueue(account, "jobs"); !errors.Is(err, store.ErrNoSuchQueue) {
		t.Fatalf("want ErrNoSuchQueue on second delete, got %v", err)
	}
}
