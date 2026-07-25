package store

import (
	"archive/zip"
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
)

func dlqTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func openDLQHelperStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSendLabDLQMessageSameAccountAllowed(t *testing.T) {
	st := openDLQHelperStore(t)
	owner := "000000000001"
	q, err := st.CreateQueue(owner, "us-east-1", "127.0.0.1:4566", "same-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.sendLabDLQMessage(owner, q.QueueARN, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("same-account dlq: %v", err)
	}
	msgs, err := st.ReceiveMessages(owner, q.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
}

func TestSendLabDLQMessageForeignWithoutPolicyDenied(t *testing.T) {
	st := openDLQHelperStore(t)
	owner := "000000000001"
	foreign := "000000000002"
	if err := st.EnsureRoot(foreign, "AKIAROOT000000000002", "secret-foreign"); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(foreign, "us-east-1", "127.0.0.1:4566", "xa-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = st.sendLabDLQMessage(owner, q.QueueARN, []byte(`{"leak":true}`))
	if err == nil {
		t.Fatal("expected foreign dlq without policy to fail")
	}
	if !strings.Contains(err.Error(), "policy") && !strings.Contains(err.Error(), "RedriveAllow") {
		t.Fatalf("want policy/allow error, got %v", err)
	}
	msgs, err := st.ReceiveMessages(foreign, q.QueueName, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("foreign dlq received %d messages", len(msgs))
	}
}

func TestSendLabDLQMessageForeignWithServicePolicyAllowed(t *testing.T) {
	st := openDLQHelperStore(t)
	owner := "000000000001"
	foreign := "000000000002"
	if err := st.EnsureRoot(foreign, "AKIAROOT000000000002", "secret-foreign"); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(foreign, "us-east-1", "127.0.0.1:4566", "xa-dlq-ok", nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"` + authz.ServicePrincipalEvents + `"},"Action":"sqs:SendMessage","Resource":"` + q.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(foreign, q.QueueName, map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	if err := st.sendLabDLQMessage(owner, q.QueueARN, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("foreign dlq with policy: %v", err)
	}
	msgs, err := st.ReceiveMessages(foreign, q.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
}

func TestSendLabDLQMessageRedriveAllowDenyAll(t *testing.T) {
	st := openDLQHelperStore(t)
	owner := "000000000001"
	q, err := st.CreateQueue(owner, "us-east-1", "127.0.0.1:4566", "deny-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	allow := `{"redrivePermission":"denyAll"}`
	if err := st.SetQueueAttributes(owner, q.QueueName, map[string]string{"RedriveAllowPolicy": allow}); err != nil {
		t.Fatal(err)
	}
	err = st.sendLabDLQMessage(owner, q.QueueARN, []byte(`{"x":1}`))
	if err == nil || !strings.Contains(err.Error(), "RedriveAllowPolicy") {
		t.Fatalf("want RedriveAllowPolicy error, got %v", err)
	}
}

func TestSNSForeignDLQWithoutAllow(t *testing.T) {
	st := openDLQHelperStore(t)
	owner := "000000000001"
	foreign := "000000000002"
	if err := st.EnsureRoot(foreign, "AKIAROOT000000000002", "secret-foreign"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTopic(owner, "us-east-1", "alerts", nil); err != nil {
		t.Fatal(err)
	}
	dlq, err := st.CreateQueue(foreign, "us-east-1", "127.0.0.1:4566", "sns-xa-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	zip := map[string]string{"app.py": "def handler(e,c): return e"}
	fn, err := st.CreateFunction(CreateFunctionMeta{
		AccountID: owner, Region: "us-east-1", FunctionName: "sns-no-policy",
		RoleARN: "arn:aws:iam::000000000001:role/lambda-exec", Runtime: LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: dlqTestZip(t, zip),
	})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := st.Subscribe(owner, "alerts", "lambda", fn.FunctionARN)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSubscriptionAttributes(sub.SubscriptionARN, map[string]string{
		"RedrivePolicy": `{"deadLetterTargetArn":"` + dlq.QueueARN + `"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Publish(owner, "alerts", "hello", "", nil); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(foreign, dlq.QueueName, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("foreign sns dlq got %d messages without policy", len(msgs))
	}
}

func TestPipesForeignDLQWithoutAllow(t *testing.T) {
	st := openDLQHelperStore(t)
	owner := "000000000001"
	foreign := "000000000002"
	if err := st.EnsureRoot(foreign, "AKIAROOT000000000002", "secret-foreign"); err != nil {
		t.Fatal(err)
	}
	src, err := st.CreateQueue(owner, "us-east-1", "127.0.0.1:4566", "pipe-src", nil)
	if err != nil {
		t.Fatal(err)
	}
	dlq, err := st.CreateQueue(foreign, "us-east-1", "127.0.0.1:4566", "pipe-xa-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	zip := map[string]string{"app.py": "def handler(e,c): return e"}
	fn, err := st.CreateFunction(CreateFunctionMeta{
		AccountID: owner, Region: "us-east-1", FunctionName: "pipe-target",
		RoleARN: "arn:aws:iam::000000000001:role/lambda-exec", Runtime: LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: dlqTestZip(t, zip),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(owner, "pipe-target", "pipes-allow", "lambda:InvokeFunction", "pipes.amazonaws.com", "", ""); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"pipes.amazonaws.com"},"Action":["sqs:ReceiveMessage","sqs:DeleteMessage"],"Resource":"` + src.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(owner, "pipe-src", map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	p, err := st.CreatePipeWithEnrichment(owner, "us-east-1", "lab-pipe", "", src.QueueARN, fn.FunctionARN, "", "", dlq.QueueARN, "RUNNING")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(owner, "pipe-src", []byte(`{"x":1}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	_ = st.PollPipeOnce(owner, p.Name, func(accountID, functionName, payloadJSON string) (string, error) {
		return "", errors.New("target invoke failed")
	})
	msgs, err := st.ReceiveMessages(foreign, dlq.QueueName, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("foreign pipes dlq got %d messages without policy", len(msgs))
	}
}

func TestEventBridgeForeignDLQWithoutAllow(t *testing.T) {
	st := openDLQHelperStore(t)
	owner := "000000000001"
	foreign := "000000000002"
	if err := st.EnsureRoot(foreign, "AKIAROOT000000000002", "secret-foreign"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRule(owner, "us-east-1", "default", "xa-dlq-rule", `{"source":["lab.dlq"]}`, "", RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	dlq, err := st.CreateQueue(foreign, "us-east-1", "127.0.0.1:4566", "eb-xa-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	logGroup := "/eb/xa-dlq"
	if _, err := st.CreateLogGroup(owner, "us-east-1", logGroup); err != nil {
		t.Fatal(err)
	}
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":["logs:PutLogEvents","logs:CreateLogStream"],"Resource":"*"}]}`
	if _, err := st.PutLogsResourcePolicy(owner, "eb-to-logs", doc); err != nil {
		t.Fatal(err)
	}
	logARN := "arn:aws:logs:us-east-1:" + owner + ":log-group:" + logGroup
	if err := st.PutTargets(owner, "default", "xa-dlq-rule", []EventTargetInput{{
		ID: "t1", ARN: logARN, DeadLetterARN: dlq.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteLogGroup(owner, logGroup); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutEvents(owner, []PutEventsEntry{{
		Source: "lab.dlq", DetailType: "Test", Detail: `{"ok":true}`,
	}}); err != nil {
		t.Fatal(err)
	}
	history, err := st.ListEventTargetDeliveryHistory(owner, "default", "xa-dlq-rule", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("history=%+v", history)
	}
	if history[0].DLQSent {
		t.Fatalf("foreign dlq without allow must not mark DLQSent: %+v", history[0])
	}
	msgs, err := st.ReceiveMessages(foreign, dlq.QueueName, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("foreign dlq got %d messages without policy", len(msgs))
	}
}
