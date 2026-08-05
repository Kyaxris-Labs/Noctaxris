package store_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openSNSStore(t *testing.T) *store.Store {
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

func TestCreateTopic(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"

	topic, err := st.CreateTopic(account, "us-east-1", "alerts", nil)
	if err != nil {
		t.Fatal(err)
	}
	wantARN := "arn:aws:sns:us-east-1:000000000001:alerts"
	if topic.TopicARN != wantARN {
		t.Fatalf("arn=%q want %q", topic.TopicARN, wantARN)
	}
	if topic.TopicName != "alerts" || topic.AccountID != account {
		t.Fatalf("topic=%+v", topic)
	}
	if strings.TrimSpace(topic.Policy) == "" {
		t.Fatal("expected default topic Policy on create")
	}
	var policyDoc map[string]any
	if err := json.Unmarshal([]byte(topic.Policy), &policyDoc); err != nil {
		t.Fatalf("default Policy JSON: %v", err)
	}

	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); !errors.Is(err, store.ErrTopicAlreadyExists) {
		t.Fatalf("want ErrTopicAlreadyExists, got %v", err)
	}

	topics, err := st.ListTopics(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 1 || topics[0].TopicName != "alerts" {
		t.Fatalf("topics=%+v", topics)
	}

	if _, err := st.GetTopic(account, "missing"); !errors.Is(err, store.ErrNoSuchTopic) {
		t.Fatalf("want ErrNoSuchTopic, got %v", err)
	}
}

func TestPublishPersistsMessageMetadata(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}

	result, err := st.Publish(account, "alerts", "hello world", "subject", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.MessageID == "" {
		t.Fatal("expected non-empty message id")
	}

	published, err := st.GetPublishedMessage(result.MessageID)
	if err != nil {
		t.Fatal(err)
	}
	if published.Body != "hello world" || published.Subject != "subject" {
		t.Fatalf("published=%+v", published)
	}
	if published.TopicARN != "arn:aws:sns:us-east-1:000000000001:alerts" {
		t.Fatalf("topic arn=%q", published.TopicARN)
	}
}

func TestSubscribeCreatesRow(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "alerts", nil)
	if err != nil {
		t.Fatal(err)
	}

	sub, err := st.Subscribe(account, "alerts", "sqs", "arn:aws:sqs:us-east-1:000000000001:jobs")
	if err != nil {
		t.Fatal(err)
	}
	if sub.SubscriptionARN == "" {
		t.Fatal("expected subscription arn")
	}
	if !strings.HasPrefix(sub.SubscriptionARN, topic.TopicARN+":") {
		t.Fatalf("subscription arn=%q want prefix %q:", sub.SubscriptionARN, topic.TopicARN)
	}
	if sub.Protocol != "sqs" || sub.Endpoint != "arn:aws:sqs:us-east-1:000000000001:jobs" {
		t.Fatalf("sub=%+v", sub)
	}
	if !sub.Confirmed {
		t.Fatalf("expected lab auto-confirm for sqs, sub=%+v", sub)
	}

	byTopic, err := st.ListSubscriptionsByTopic(account, "alerts")
	if err != nil {
		t.Fatal(err)
	}
	if len(byTopic) != 1 || byTopic[0].SubscriptionARN != sub.SubscriptionARN {
		t.Fatalf("byTopic=%+v", byTopic)
	}

	attrs, err := st.GetSubscriptionAttributes(sub.SubscriptionARN)
	if err != nil {
		t.Fatal(err)
	}
	if attrs["TopicArn"] != topic.TopicARN || attrs["Protocol"] != "sqs" {
		t.Fatalf("attrs=%+v", attrs)
	}
}

func TestDeleteTopicRemovesSubscriptions(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}
	sub, err := st.Subscribe(account, "alerts", "sqs", "arn:aws:sqs:us-east-1:000000000001:jobs")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteTopic(account, "alerts"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSubscriptionAttributes(sub.SubscriptionARN); !errors.Is(err, store.ErrNoSuchSubscription) {
		t.Fatalf("want ErrNoSuchSubscription, got %v", err)
	}
}

func TestSetTopicAttributesMergesPolicy(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", map[string]string{"DisplayName": "Alerts"}); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[]}`
	if err := st.SetTopicAttributes(account, "alerts", map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	attrs, err := st.GetTopicAttributes(account, "alerts")
	if err != nil {
		t.Fatal(err)
	}
	if attrs["Policy"] != policy {
		t.Fatalf("policy=%q", attrs["Policy"])
	}
	if attrs["DisplayName"] != "Alerts" {
		t.Fatalf("attrs=%+v", attrs)
	}
}

func TestGetTopicAttributesReturnsDefaultPolicy(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "with-default", map[string]string{
		"DisplayName": "DefaultPolicy",
	}); err != nil {
		t.Fatal(err)
	}
	attrs, err := st.GetTopicAttributes(account, "with-default")
	if err != nil {
		t.Fatal(err)
	}
	policy := attrs["Policy"]
	if strings.TrimSpace(policy) == "" {
		t.Fatal("Policy missing; Terraform cannot parse empty/omitted Policy")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(policy), &doc); err != nil {
		t.Fatalf("Policy JSON: %v raw=%q", err, policy)
	}
	if attrs["DisplayName"] != "DefaultPolicy" {
		t.Fatalf("attrs=%+v", attrs)
	}
}

func TestGetPublishedMessageMissing(t *testing.T) {
	st := openSNSStore(t)

	_, err := st.GetPublishedMessage("missing-message-id")
	if !errors.Is(err, store.ErrNoSuchSNSMessage) {
		t.Fatalf("want ErrNoSuchSNSMessage, got %v", err)
	}
	if errors.Is(err, store.ErrNoSuchTopic) {
		t.Fatal("missing message must not return ErrNoSuchTopic")
	}
}

func TestAddTopicPermissionSeedsPolicy(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "alerts", nil)
	if err != nil {
		t.Fatal(err)
	}
	principal := "arn:aws:iam::000000000001:root"
	action := "sns:Publish"

	if err := st.AddTopicPermission(account, "alerts", "pub-label", principal, action); err != nil {
		t.Fatal(err)
	}

	attrs, err := st.GetTopicAttributes(account, "alerts")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(attrs["Policy"]) == "" {
		t.Fatal("expected seeded policy")
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(attrs["Policy"]), &doc); err != nil {
		t.Fatal(err)
	}
	statements, ok := doc["Statement"].([]any)
	if !ok || len(statements) < 2 {
		t.Fatalf("statements=%v want default + AddPermission", doc["Statement"])
	}
	var found map[string]any
	for _, raw := range statements {
		stmt, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if stmt["Sid"] == "pub-label" {
			found = stmt
			break
		}
	}
	if found == nil {
		t.Fatalf("missing pub-label statement in %v", statements)
	}
	if found["Action"] != action || found["Resource"] != topic.TopicARN {
		t.Fatalf("statement=%+v", found)
	}
}

func TestAddTopicPermissionMergesExistingPolicy(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "alerts", nil)
	if err != nil {
		t.Fatal(err)
	}
	existing := `{"Version":"2012-10-17","Statement":[{"Sid":"existing","Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sns:Subscribe","Resource":"` + topic.TopicARN + `"}]}`
	if err := st.SetTopicAttributes(account, "alerts", map[string]string{"Policy": existing}); err != nil {
		t.Fatal(err)
	}

	principal := "arn:aws:iam::000000000001:role/publisher"
	action := "sns:Publish"
	if err := st.AddTopicPermission(account, "alerts", "publish-label", principal, action); err != nil {
		t.Fatal(err)
	}

	attrs, err := st.GetTopicAttributes(account, "alerts")
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(attrs["Policy"]), &doc); err != nil {
		t.Fatal(err)
	}
	statements, ok := doc["Statement"].([]any)
	if !ok || len(statements) != 2 {
		t.Fatalf("want 2 statements, got %v", doc["Statement"])
	}
	first, _ := statements[0].(map[string]any)
	second, _ := statements[1].(map[string]any)
	if first["Sid"] != "existing" {
		t.Fatalf("first statement=%+v", first)
	}
	if second["Sid"] != "publish-label" || second["Action"] != action {
		t.Fatalf("second statement=%+v", second)
	}
}

func TestPublishDeliversToSubscribedSQSQueue(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}
	queue, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sns-target", nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	if err := st.SetQueueAttributes(account, "sns-target", map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(account, "alerts", "sqs", queue.QueueARN); err != nil {
		t.Fatal(err)
	}

	result, err := st.Publish(account, "alerts", "payload-body", "payload-subject", nil)
	if err != nil {
		t.Fatal(err)
	}

	msgs, err := st.ReceiveMessages(account, "sns-target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages=%d want 1", len(msgs))
	}
	var envelope map[string]any
	if err := json.Unmarshal(msgs[0].Body, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["Type"] != "Notification" {
		t.Fatalf("Type=%v", envelope["Type"])
	}
	if envelope["MessageId"] != result.MessageID {
		t.Fatalf("MessageId=%v want %q", envelope["MessageId"], result.MessageID)
	}
	if envelope["TopicArn"] != "arn:aws:sns:us-east-1:000000000001:alerts" {
		t.Fatalf("TopicArn=%v", envelope["TopicArn"])
	}
	if envelope["Message"] != "payload-body" || envelope["Subject"] != "payload-subject" {
		t.Fatalf("envelope=%+v", envelope)
	}
}

func TestPublishSkipsSQSWithoutQueuePolicy(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sns-deny", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(account, "alerts", "sqs", "arn:aws:sqs:us-east-1:"+account+":sns-deny"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Publish(account, "alerts", "no-policy", "", nil); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, "sns-deny", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected no delivery without queue policy, got %d", len(msgs))
	}
}

func TestPublishDeliversToSubscribedLambda(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "sns-handler",
		RoleARN:      "arn:aws:iam::000000000001:role/lambda-exec",
		Runtime:      store.LambdaRuntimePython312,
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(account, "sns-handler", "sns-allow", "lambda:InvokeFunction", "sns.amazonaws.com", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(account, "alerts", "lambda", fn.FunctionARN); err != nil {
		t.Fatal(err)
	}

	if _, err := st.Publish(account, "alerts", "lambda-payload", "", nil); err != nil {
		t.Fatal(err)
	}

	job, err := st.LatestAsyncInvocation(account, "sns-handler")
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(job.EventJSON), &event); err != nil {
		t.Fatal(err)
	}
	records, ok := event["Records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("Records=%v", event["Records"])
	}
	record, ok := records[0].(map[string]any)
	if !ok || record["EventSource"] != "aws:sns" {
		t.Fatalf("record=%+v", records[0])
	}
	sns, ok := record["Sns"].(map[string]any)
	if !ok || sns["Message"] != "lambda-payload" {
		t.Fatalf("sns=%+v", record["Sns"])
	}
}

func TestAddTopicPermissionDuplicateLabel(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}
	principal := "arn:aws:iam::000000000001:root"
	if err := st.AddTopicPermission(account, "alerts", "dup-label", principal, "sns:Publish"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddTopicPermission(account, "alerts", "dup-label", principal, "sns:Subscribe"); !errors.Is(err, store.ErrSNSPolicyStatementExists) {
		t.Fatalf("want ErrSNSPolicyStatementExists, got %v", err)
	}
}

func TestPublishRejectsSQSARNAccountMismatch(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "jobs", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(account, "alerts", "sqs", "arn:aws:sqs:us-east-1:999999999999:jobs"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Publish(account, "alerts", "hello", "", nil); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, "jobs", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected no delivery for cross-account SQS ARN, got %d", len(msgs))
	}
}

func sqliteBusy(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "database is locked") || strings.Contains(s, "SQLITE_BUSY")
}

func TestSNSPublishConcurrentReceive(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"
	if _, err := st.CreateTopic(account, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}
	target, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sns-target", nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	if err := st.SetQueueAttributes(account, "sns-target", map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(account, "alerts", "sqs", target.QueueARN); err != nil {
		t.Fatal(err)
	}

	const otherN = 30
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "other", map[string]string{
		"VisibilityTimeout": "30",
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < otherN; i++ {
		if _, err := st.SendMessage(account, "other", []byte(fmt.Sprintf("o-%d", i)), false, nil, "", nil); err != nil {
			t.Fatal(err)
		}
	}

	const publishes = 20
	var wg sync.WaitGroup
	errCh := make(chan error, publishes+8)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < publishes; i++ {
			var err error
			for attempt := 0; attempt < 8; attempt++ {
				_, err = st.Publish(account, "alerts", fmt.Sprintf("p-%d", i), "", nil)
				if err == nil || !sqliteBusy(err) {
					break
				}
				time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()

	seen := sync.Map{}
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var msgs []store.Message
				var err error
				for attempt := 0; attempt < 8; attempt++ {
					msgs, err = st.ReceiveMessages(account, "other", 5)
					if err == nil || !sqliteBusy(err) {
						break
					}
					time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
				}
				if err != nil {
					errCh <- err
					return
				}
				if len(msgs) == 0 {
					return
				}
				for _, m := range msgs {
					if _, loaded := seen.LoadOrStore(m.MessageID, struct{}{}); loaded {
						errCh <- fmt.Errorf("duplicate receive of %s on other queue", m.MessageID)
						return
					}
					var delErr error
					for attempt := 0; attempt < 8; attempt++ {
						delErr = st.DeleteMessage(account, "other", m.ReceiptHandle)
						if delErr == nil || !sqliteBusy(delErr) {
							break
						}
						time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
					}
					if delErr != nil {
						errCh <- delErr
						return
					}
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Drain remaining other-queue messages without duplicates.
	for {
		msgs, err := st.ReceiveMessages(account, "other", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) == 0 {
			break
		}
		for _, m := range msgs {
			if _, loaded := seen.LoadOrStore(m.MessageID, struct{}{}); loaded {
				t.Fatalf("duplicate receive of %s during drain", m.MessageID)
			}
			if err := st.DeleteMessage(account, "other", m.ReceiptHandle); err != nil {
				t.Fatal(err)
			}
		}
	}
	count := 0
	seen.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != otherN {
		t.Fatalf("other queue delivered=%d want %d", count, otherN)
	}

	targetSeen := map[string]struct{}{}
	for {
		msgs, err := st.ReceiveMessages(account, "sns-target", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) == 0 {
			break
		}
		for _, m := range msgs {
			if _, ok := targetSeen[m.MessageID]; ok {
				t.Fatalf("duplicate SNS delivery %s", m.MessageID)
			}
			targetSeen[m.MessageID] = struct{}{}
		}
	}
	if len(targetSeen) != publishes {
		t.Fatalf("sns-target messages=%d want %d", len(targetSeen), publishes)
	}
}

func TestSNSCoverageWave2ListSubsPermissionsTopicName(t *testing.T) {
	st := openSNSStore(t)
	account := "000000000001"

	if err := store.EnsureSNSSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}
	if err := st.EnsureSNSSchema(); err != nil {
		t.Fatal(err)
	}

	if _, err := store.TopicNameFromARN("not-an-arn"); err == nil {
		t.Fatal("invalid ARN must fail")
	}
	if _, err := store.TopicNameFromARN("arn:aws:sns:us-east-1:"); err == nil {
		t.Fatal("empty name segment must fail")
	}
	// Lenient parser: last colon segment is the name (including short ARNs).
	if name, err := store.TopicNameFromARN("arn:aws:sns:us-east-1:1"); err != nil || name != "1" {
		t.Fatalf("short ARN name=%q err=%v", name, err)
	}
	name, err := store.TopicNameFromARN("arn:aws:sns:us-east-1:000000000001:wave2-topic")
	if err != nil || name != "wave2-topic" {
		t.Fatalf("name=%q err=%v", name, err)
	}

	empty, err := st.ListSubscriptions(account)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty subs=%v err=%v", empty, err)
	}

	topic, err := st.CreateTopic(account, "us-east-1", "wave2-topic", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Lab RemoveTopicPermission is a stub that only validates the topic exists.
	if err := st.RemoveTopicPermission(account, "wave2-topic", "missing-label"); err != nil {
		t.Fatal(err)
	}

	principal := "arn:aws:iam::" + account + ":root"
	if err := st.AddTopicPermission(account, "wave2-topic", "wave2-label", principal, "sns:Publish"); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveTopicPermission(account, "wave2-topic", "wave2-label"); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveTopicPermission(account, "missing-topic", "x"); !errors.Is(err, store.ErrNoSuchTopic) {
		t.Fatalf("remove on missing topic: %v", err)
	}

	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "wave2-sns-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	pol := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	if err := st.SetQueueAttributes(account, q.QueueName, map[string]string{"Policy": pol}); err != nil {
		t.Fatal(err)
	}
	sub, err := st.Subscribe(account, topic.TopicARN, "sqs", q.QueueARN)
	if err != nil {
		t.Fatal(err)
	}
	subs, err := st.ListSubscriptions(account)
	if err != nil || len(subs) != 1 || subs[0].SubscriptionARN != sub.SubscriptionARN {
		t.Fatalf("subs=%v err=%v", subs, err)
	}
	if _, err := st.Publish(account, "wave2-topic", "hello-wave2", "", nil); err != nil {
		t.Fatal(err)
	}
}
