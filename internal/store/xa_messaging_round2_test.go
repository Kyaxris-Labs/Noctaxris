package store_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openXAMessagingStore(t *testing.T) *store.Store {
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

func TestAddFunctionPermissionPersistsSourceArnAndAccount(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "src-lock",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: testZip(t, map[string]string{"app.py": "def handler(e,c): return e"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceARN := "arn:aws:events:us-east-1:000000000001:rule/default/lab"
	stmt, err := st.AddFunctionPermission(account, fn.FunctionName, "events-lock", "lambda:InvokeFunction",
		"events.amazonaws.com", account, sourceARN)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stmt, `"aws:SourceArn"`) || !strings.Contains(stmt, sourceARN) {
		t.Fatalf("statement missing SourceArn: %s", stmt)
	}
	if !strings.Contains(stmt, `"aws:SourceAccount"`) || !strings.Contains(stmt, account) {
		t.Fatalf("statement missing SourceAccount: %s", stmt)
	}
	doc, err := st.GetFunctionPolicy(account, fn.FunctionName)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(doc), &parsed); err != nil {
		t.Fatal(err)
	}
}

func TestEventBridgeSourceArnConditionDeniesWrongRule(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "cond-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRule(account, "us-east-1", "default", "allowed-rule", `{"source":["noctaxris.lab"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRule(account, "us-east-1", "default", "other-rule", `{"source":["noctaxris.lab"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	allowedRuleARN := store.EventRuleARN("us-east-1", account, "default", "allowed-rule")
	policy := `{"Version":"2012-10-17","Statement":[{
		"Effect":"Allow",
		"Principal":{"Service":"events.amazonaws.com"},
		"Action":"sqs:SendMessage",
		"Resource":"` + q.QueueARN + `",
		"Condition":{"ArnLike":{"aws:SourceArn":"` + allowedRuleARN + `"}}
	}]}`
	if err := st.SetQueueAttributes(account, q.QueueName, map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(account, "default", "allowed-rule", []store.EventTargetInput{{
		ID: "1", ARN: q.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(account, "default", "other-rule", []store.EventTargetInput{{
		ID: "1", ARN: q.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}

	result, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source: "noctaxris.lab", DetailType: "x", Detail: `{"ok":true}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Two rules match; only allowed-rule may deliver.
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("msgs=%d want 1 (SourceArn lock); entries=%+v", len(msgs), result.Entries)
	}
}

func TestEventBridgeForeignSQSDelivery(t *testing.T) {
	st := openXAMessagingStore(t)
	owner := "000000000001"
	foreign := "000000000002"
	if err := st.EnsureRoot(foreign, "AKIAROOT000000000002", "secret-foreign"); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(foreign, "us-east-1", "127.0.0.1:4566", "xa-eb-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + q.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(foreign, q.QueueName, map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRule(owner, "us-east-1", "default", "xa-rule", `{"source":["noctaxris.lab"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(owner, "default", "xa-rule", []store.EventTargetInput{{
		ID: "1", ARN: q.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutEvents(owner, []store.PutEventsEntry{{
		Source: "noctaxris.lab", DetailType: "x", Detail: `{"ok":true}`,
	}}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(foreign, q.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("foreign msgs=%d err=%v", len(msgs), err)
	}
}

func TestSFNTaskUsesRoleArnSession(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sfn-role-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"states.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "sfn-exec", trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "task", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sqs:SendMessage","Resource":"`+q.QueueARN+`"}]}`); err != nil {
		t.Fatal(err)
	}
	def := `{
  "StartAt": "Send",
  "States": {
    "Send": {"Type":"Task","Resource":"` + q.QueueARN + `","End":true}
  }
}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "sfn-role-sm", def, roleARN)
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartSFNExecution(account, "us-east-1", sm.StateMachineARN, "r1", `{"a":1}`, nil)
	if err != nil || exec.Status != "SUCCEEDED" {
		t.Fatalf("exec=%+v err=%v", exec, err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
}

func TestSFNTaskDeniesWithoutRoleOrPolicy(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "sfn-deny-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	def := `{
  "StartAt": "Send",
  "States": {
    "Send": {"Type":"Task","Resource":"` + q.QueueARN + `","End":true}
  }
}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "sfn-deny-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartSFNExecution(account, "us-east-1", sm.StateMachineARN, "r1", `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != "FAILED" {
		t.Fatalf("want FAILED got %+v", exec)
	}
}

func TestPipesSourceRequiresAuthz(t *testing.T) {
	st := openXAMessagingStore(t)
	account := "000000000001"
	src, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-src-lock", nil)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-dst-lock", nil)
	if err != nil {
		t.Fatal(err)
	}
	dstPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"pipes.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + dst.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(account, dst.QueueName, map[string]string{"Policy": dstPolicy}); err != nil {
		t.Fatal(err)
	}
	p, err := st.CreatePipe(account, "us-east-1", "src-lock-pipe", "", src.QueueARN, dst.QueueARN, "", "RUNNING")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, src.QueueName, []byte(`{"x":1}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.PollPipeOnce(account, p.Name, nil); err == nil {
		t.Fatal("expected source authz deny without RoleArn or source policy")
	}
}
