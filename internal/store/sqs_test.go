package store_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

	sent, err := st.SendMessage(account, "jobs", []byte("hello"), false, nil, "", nil)
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
	if _, err := st.SendMessage(account, "jobs", []byte("payload"), false, nil, "", nil); err != nil {
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
	if _, err := st.SendMessage(account, "secure", cipher, true, dek, `{"trace":{"StringValue":"x"}}`, nil); err != nil {
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
	if _, err := st.SendMessage(account, "jobs", []byte("x"), false, nil, "", nil); err != nil {
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

func TestCreateFIFOQueueRequiresFifoSuffix(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"

	_, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs.fifo", nil)
	if !errors.Is(err, store.ErrInvalidFIFOQueueName) {
		t.Fatalf("want ErrInvalidFIFOQueueName, got %v", err)
	}

	_, err = st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", map[string]string{"FifoQueue": "true"})
	if !errors.Is(err, store.ErrInvalidFIFOQueueName) {
		t.Fatalf("want ErrInvalidFIFOQueueName for missing suffix, got %v", err)
	}
}

func TestFIFOSendReceiveOrderingAndDedup(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "orders.fifo", map[string]string{
		"FifoQueue":                 "true",
		"ContentBasedDeduplication": "true",
		"VisibilityTimeout":         "300",
	}); err != nil {
		t.Fatal(err)
	}

	opts := &store.SendMessageOpts{MessageGroupID: "group-a"}
	first, err := st.SendMessage(account, "orders.fifo", []byte("a1"), false, nil, "", opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "orders.fifo", []byte("a2"), false, nil, "", opts); err != nil {
		t.Fatal(err)
	}
	dup, err := st.SendMessage(account, "orders.fifo", []byte("a1"), false, nil, "", opts)
	if err != nil {
		t.Fatal(err)
	}
	if dup.MessageID != first.MessageID {
		t.Fatalf("dedup message id=%q want %q", dup.MessageID, first.MessageID)
	}

	got, err := st.ReceiveMessages(account, "orders.fifo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !bytes.Equal(got[0].Body, []byte("a1")) {
		t.Fatalf("first receive=%+v", got)
	}
	if err := st.DeleteMessage(account, "orders.fifo", got[0].ReceiptHandle); err != nil {
		t.Fatal(err)
	}

	again, err := st.ReceiveMessages(account, "orders.fifo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || !bytes.Equal(again[0].Body, []byte("a2")) {
		t.Fatalf("want a2 after a1 delete, got %+v", again)
	}
}

func TestFIFOSendRequiresMessageGroupId(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs.fifo", map[string]string{"FifoQueue": "true"}); err != nil {
		t.Fatal(err)
	}
	_, err := st.SendMessage(account, "jobs.fifo", []byte("x"), false, nil, "", nil)
	if !errors.Is(err, store.ErrMissingMessageGroupID) {
		t.Fatalf("want ErrMissingMessageGroupID, got %v", err)
	}
}

func TestRedrivePolicyMovesMessageToDLQ(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "dlq.fifo", map[string]string{"FifoQueue": "true"}); err != nil {
		t.Fatal(err)
	}
	dlqARN := store.QueueARN("us-east-1", account, "dlq.fifo")
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "source", map[string]string{
		"VisibilityTimeout": "0",
		"RedrivePolicy":     `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":"1"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "source", []byte("fail-me"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}

	first, err := st.ReceiveMessages(account, "source", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("first receive=%+v", first)
	}
	if err := st.ChangeMessageVisibility(account, "source", first[0].ReceiptHandle, 0); err != nil {
		t.Fatal(err)
	}

	second, err := st.ReceiveMessages(account, "source", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("expected redrive on second receive, got %+v", second)
	}

	dlqMsgs, err := st.ReceiveMessages(account, "dlq.fifo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(dlqMsgs) != 1 || !bytes.Equal(dlqMsgs[0].Body, []byte("fail-me")) {
		t.Fatalf("dlq message=%+v", dlqMsgs)
	}
}

func TestRedrivePolicySetsDlqSourceArnAttribute(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "dlq-src", nil); err != nil {
		t.Fatal(err)
	}
	dlqARN := store.QueueARN("us-east-1", account, "dlq-src")
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "src-q", map[string]string{
		"VisibilityTimeout": "0",
		"RedrivePolicy":     `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":"1"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "src-q", []byte("poison"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReceiveMessages(account, "src-q", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("receive=%+v", got)
	}
	if err := st.ChangeMessageVisibility(account, "src-q", got[0].ReceiptHandle, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReceiveMessages(account, "src-q", 1); err != nil {
		t.Fatal(err)
	}
	dlqMsgs, err := st.ReceiveMessages(account, "dlq-src", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(dlqMsgs) != 1 {
		t.Fatalf("dlq=%+v", dlqMsgs)
	}
	var attrs map[string]any
	if err := json.Unmarshal([]byte(dlqMsgs[0].AttributesJSON), &attrs); err != nil {
		t.Fatal(err)
	}
	provenance, ok := attrs["NoctaxrisDlqSourceArn"].(map[string]any)
	if !ok {
		t.Fatalf("attrs=%v", attrs)
	}
	if provenance["StringValue"] != store.QueueARN("us-east-1", account, "src-q") {
		t.Fatalf("provenance=%v", provenance)
	}
}

func TestSQSDelaySecondsHidesUntilVisible(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "delayed", map[string]string{
		"DelaySeconds": "60",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "delayed", []byte("later"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReceiveMessages(account, "delayed", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected queue DelaySeconds to hide message, got %+v", got)
	}

	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "permsg", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "permsg", []byte("soon"), false, nil, "", &store.SendMessageOpts{DelaySeconds: 900}); err != nil {
		t.Fatal(err)
	}
	got, err = st.ReceiveMessages(account, "permsg", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected per-message DelaySeconds to hide message, got %+v", got)
	}
	if _, err := st.SendMessage(account, "permsg", []byte("now"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	got, err = st.ReceiveMessages(account, "permsg", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || string(got[0].Body) != "now" {
		t.Fatalf("want immediate message, got %+v", got)
	}
}

func TestSQSRedriveAllowPolicyDenies(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "dlq", nil); err != nil {
		t.Fatal(err)
	}
	dlqARN := store.QueueARN("us-east-1", account, "dlq")
	allow := `{"redrivePermission":"byQueue","sourceQueueArns":["arn:aws:sqs:us-east-1:000000000001:other"]}`
	if err := st.SetQueueAttributes(account, "dlq", map[string]string{"RedriveAllowPolicy": allow}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "source", map[string]string{
		"VisibilityTimeout": "0",
		"RedrivePolicy":     `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":"1"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "source", []byte("fail-me"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	first, err := st.ReceiveMessages(account, "source", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("first receive=%+v", first)
	}
	if err := st.ChangeMessageVisibility(account, "source", first[0].ReceiptHandle, 0); err != nil {
		t.Fatal(err)
	}
	_, err = st.ReceiveMessages(account, "source", 1)
	if err == nil || !strings.Contains(err.Error(), "RedriveAllowPolicy") {
		t.Fatalf("want RedriveAllowPolicy error, got %v", err)
	}
}

func TestReceiveMessagesConcurrentNoDuplicate(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", map[string]string{
		"VisibilityTimeout": "30",
	}); err != nil {
		t.Fatal(err)
	}
	const n = 20
	for i := 0; i < n; i++ {
		if _, err := st.SendMessage(account, "jobs", []byte("m"), false, nil, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	type result struct {
		ids []string
		err error
	}
	ch := make(chan result, 8)
	for g := 0; g < 8; g++ {
		go func() {
			got, err := st.ReceiveMessages(account, "jobs", 5)
			ids := make([]string, 0, len(got))
			for _, m := range got {
				ids = append(ids, m.MessageID)
			}
			ch <- result{ids: ids, err: err}
		}()
	}
	seen := map[string]struct{}{}
	total := 0
	for g := 0; g < 8; g++ {
		r := <-ch
		if r.err != nil {
			t.Fatal(r.err)
		}
		for _, id := range r.ids {
			if _, ok := seen[id]; ok {
				t.Fatalf("duplicate delivery of %s", id)
			}
			seen[id] = struct{}{}
			total++
		}
	}
	if total != n {
		t.Fatalf("delivered=%d want %d", total, n)
	}
}

func TestFIFOSendConcurrentUniqueSequence(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "orders.fifo", map[string]string{
		"FifoQueue":         "true",
		"VisibilityTimeout": "0",
	}); err != nil {
		t.Fatal(err)
	}
	const n = 30
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			_, err := st.SendMessage(account, "orders.fifo", []byte("x"), false, nil, "", &store.SendMessageOpts{
				MessageGroupID:         "g1",
				MessageDeduplicationID: "uniq-" + strconv.Itoa(i),
			})
			errs <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	seqs := map[int64]struct{}{}
	for i := 0; i < n; i++ {
		msgs, err := st.ReceiveMessages(account, "orders.fifo", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) != 1 {
			t.Fatalf("receive %d: got %d msgs", i, len(msgs))
		}
		if _, ok := seqs[msgs[0].SequenceNumber]; ok {
			t.Fatalf("duplicate sequence %d", msgs[0].SequenceNumber)
		}
		seqs[msgs[0].SequenceNumber] = struct{}{}
		if err := st.DeleteMessage(account, "orders.fifo", msgs[0].ReceiptHandle); err != nil {
			t.Fatal(err)
		}
	}
	if len(seqs) != n {
		t.Fatalf("sequences=%d want %d", len(seqs), n)
	}
}

func TestFIFOReceiveSameGroup(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	queue := "orders.fifo"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", queue, map[string]string{
		"FifoQueue":         "true",
		"VisibilityTimeout": "300",
	}); err != nil {
		t.Fatal(err)
	}
	const n = 12
	for i := 0; i < n; i++ {
		if _, err := st.SendMessage(account, queue, []byte(fmt.Sprintf("m-%d", i)), false, nil, "", &store.SendMessageOpts{
			MessageGroupID:         "g1",
			MessageDeduplicationID: fmt.Sprintf("d-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	var (
		inflight    atomic.Int32
		maxInflight atomic.Int32
		completed   atomic.Int32
		orderMu     sync.Mutex
		order       []int64
		errCh       = make(chan error, 16)
		stop        = make(chan struct{})
	)
	for g := 0; g < 8; g++ {
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				msgs, err := st.ReceiveMessages(account, queue, 10)
				if err != nil {
					errCh <- err
					return
				}
				if len(msgs) == 0 {
					if completed.Load() >= int32(n) {
						return
					}
					continue
				}
				if len(msgs) != 1 {
					errCh <- fmt.Errorf("same-group receive returned %d messages", len(msgs))
					return
				}
				cur := inflight.Add(1)
				for {
					old := maxInflight.Load()
					if cur <= old || maxInflight.CompareAndSwap(old, cur) {
						break
					}
				}
				if cur > 1 {
					errCh <- fmt.Errorf("two in-flight for same MessageGroupId: inflight=%d", cur)
					inflight.Add(-1)
					return
				}
				orderMu.Lock()
				order = append(order, msgs[0].SequenceNumber)
				orderMu.Unlock()
				time.Sleep(2 * time.Millisecond)
				if err := st.DeleteMessage(account, queue, msgs[0].ReceiptHandle); err != nil {
					errCh <- err
					inflight.Add(-1)
					return
				}
				inflight.Add(-1)
				if completed.Add(1) >= int32(n) {
					return
				}
			}
		}()
	}

	deadline := time.After(15 * time.Second)
	for completed.Load() < int32(n) {
		select {
		case err := <-errCh:
			t.Fatal(err)
		case <-deadline:
			t.Fatalf("timeout: completed=%d want %d", completed.Load(), n)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	close(stop)

	if maxInflight.Load() > 1 {
		t.Fatalf("max concurrent in-flight=%d want <=1", maxInflight.Load())
	}
	orderMu.Lock()
	defer orderMu.Unlock()
	if len(order) != n {
		t.Fatalf("received=%d want %d", len(order), n)
	}
	for i := 1; i < len(order); i++ {
		if order[i] <= order[i-1] {
			t.Fatalf("sequence order broken: %v", order)
		}
	}
}

func TestSendReceiveCMVDeleteStress(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	queue := "stress"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", queue, map[string]string{
		"VisibilityTimeout": "30",
	}); err != nil {
		t.Fatal(err)
	}

	const (
		workers = 6
		rounds  = 40
	)
	var (
		wg       sync.WaitGroup
		errCh    = make(chan error, workers*4)
		sent     atomic.Int64
		deleted  atomic.Int64
		received atomic.Int64
	)

	wg.Add(4)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			if _, err := st.SendMessage(account, queue, []byte(fmt.Sprintf("s-%d", i)), false, nil, "", nil); err != nil {
				errCh <- err
				return
			}
			sent.Add(1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			msgs, err := st.ReceiveMessages(account, queue, 5)
			if err != nil {
				errCh <- err
				return
			}
			received.Add(int64(len(msgs)))
			for _, m := range msgs {
				_ = st.ChangeMessageVisibility(account, queue, m.ReceiptHandle, 0)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			msgs, err := st.ReceiveMessages(account, queue, 5)
			if err != nil {
				errCh <- err
				return
			}
			for _, m := range msgs {
				if err := st.ChangeMessageVisibility(account, queue, m.ReceiptHandle, 1); err != nil && !errors.Is(err, store.ErrNoSuchMessage) {
					errCh <- err
					return
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			msgs, err := st.ReceiveMessages(account, queue, 5)
			if err != nil {
				errCh <- err
				return
			}
			for _, m := range msgs {
				if err := st.DeleteMessage(account, queue, m.ReceiptHandle); err != nil && !errors.Is(err, store.ErrNoSuchMessage) {
					errCh <- err
					return
				}
				deleted.Add(1)
			}
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Drain remaining visible messages; no duplicate deliveries across a final sweep.
	seen := map[string]struct{}{}
	for {
		msgs, err := st.ReceiveMessages(account, queue, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) == 0 {
			break
		}
		for _, m := range msgs {
			if _, ok := seen[m.MessageID]; ok {
				t.Fatalf("duplicate delivery of %s during drain", m.MessageID)
			}
			seen[m.MessageID] = struct{}{}
			if err := st.DeleteMessage(account, queue, m.ReceiptHandle); err != nil {
				t.Fatal(err)
			}
		}
	}
	if sent.Load() != rounds {
		t.Fatalf("sent=%d want %d", sent.Load(), rounds)
	}
	_ = received
	_ = deleted
}

func TestDelayedNotEarly(t *testing.T) {
	st := openSQSStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "delayed", map[string]string{
		"DelaySeconds": "60",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "delayed", []byte("later"), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReceiveMessages(account, "delayed", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("delayed message received early: %+v", got)
	}

	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "permsg", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, "permsg", []byte("soon"), false, nil, "", &store.SendMessageOpts{DelaySeconds: 900}); err != nil {
		t.Fatal(err)
	}
	got, err = st.ReceiveMessages(account, "permsg", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("per-message delay received early: %+v", got)
	}
}
