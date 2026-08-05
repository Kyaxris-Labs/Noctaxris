package store_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCFNLambdaPermissionFunctionUrlAuthType(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "cfn-url-perm-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"index.py": "def handler(e,c): return e"})
	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "cfn-url-perm-fn",
		RoleARN: roleARN, Runtime: store.LambdaRuntimePython312, Handler: "index.handler", Zip: zip,
	}); err != nil {
		t.Fatal(err)
	}
	tpl := `{
	  "Resources": {
	    "Perm": {
	      "Type": "AWS::Lambda::Permission",
	      "Properties": {
	        "FunctionName": "cfn-url-perm-fn",
	        "Action": "lambda:InvokeFunctionUrl",
	        "Principal": "*",
	        "FunctionUrlAuthType": "AWS_IAM",
	        "InvokedViaFunctionUrl": true,
	        "StatementId": "AllowURL"
	      }
	    }
	  }
	}`
	created, err := st.CreateCFNStack(account, "us-east-1", "url-perm-stack", tpl, "")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := st.GetFunctionPolicy(account, "cfn-url-perm-fn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(policy, "lambda:FunctionUrlAuthType") || !strings.Contains(policy, "AWS_IAM") {
		t.Fatalf("policy=%s", policy)
	}
	if !strings.Contains(policy, "lambda:InvokedViaFunctionUrl") {
		t.Fatalf("missing InvokedViaFunctionUrl in %s", policy)
	}
	if err := st.DeleteCFNStack(account, created.StackName); err != nil {
		t.Fatal(err)
	}
}

func TestCFNLambdaPermissionRejectsPrincipalOrgID(t *testing.T) {
	st := openTestStore(t)
	_, err := st.CreateCFNStack("000000000001", "us-east-1", "bad-org-perm", `{
	  "Resources": {
	    "Perm": {
	      "Type": "AWS::Lambda::Permission",
	      "Properties": {
	        "FunctionName": "missing",
	        "Action": "lambda:InvokeFunctionUrl",
	        "Principal": "*",
	        "PrincipalOrgID": "o-abc1234567",
	        "FunctionUrlAuthType": "NONE"
	      }
	    }
	  }
	}`, "")
	if err == nil || !strings.Contains(err.Error(), "PrincipalOrgID") {
		t.Fatalf("want PrincipalOrgID reject, got %v", err)
	}
}

func TestCFNSNSSubscriptionFilterPolicy(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "cfn-filter-topic", nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "cfn-filter-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	qPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	if err := st.SetQueueAttributes(account, q.QueueName, map[string]string{"Policy": qPolicy}); err != nil {
		t.Fatal(err)
	}
	tpl := `{
	  "Resources": {
	    "Sub": {
	      "Type": "AWS::SNS::Subscription",
	      "Properties": {
	        "TopicArn": "` + topic.TopicARN + `",
	        "Protocol": "sqs",
	        "Endpoint": "` + q.QueueARN + `",
	        "FilterPolicy": {"event": ["order"]},
	        "FilterPolicyScope": "MessageAttributes",
	        "RawMessageDelivery": true
	      }
	    }
	  }
	}`
	created, err := st.CreateCFNStack(account, "us-east-1", "filter-sub-stack", tpl, "")
	if err != nil {
		t.Fatal(err)
	}
	subs, err := st.ListSubscriptionsByTopic(account, topic.TopicName)
	if err != nil || len(subs) != 1 {
		t.Fatalf("subs=%+v err=%v", subs, err)
	}
	attrs, err := st.GetSubscriptionAttributes(subs[0].SubscriptionARN)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(attrs["FilterPolicy"], "order") {
		t.Fatalf("attrs=%v", attrs)
	}
	if attrs["RawMessageDelivery"] != "true" {
		t.Fatalf("RawMessageDelivery=%q", attrs["RawMessageDelivery"])
	}

	if _, err := st.Publish(account, topic.TopicName, "skip-me", "", map[string]string{"event": "other"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	msgs, err := st.ReceiveMessages(account, q.QueueName, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected filter drop, got %d", len(msgs))
	}

	if _, err := st.Publish(account, topic.TopicName, "raw-body", "", map[string]string{"event": "order"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		msgs, err = st.ReceiveMessages(account, q.QueueName, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	if string(msgs[0].Body) != "raw-body" {
		t.Fatalf("want raw body, got %q", msgs[0].Body)
	}
	if err := st.DeleteCFNStack(account, created.StackName); err != nil {
		t.Fatal(err)
	}
}

func TestCFNLogGroupRetentionInDays(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{
	  "Resources": {
	    "G": {
	      "Type": "AWS::Logs::LogGroup",
	      "Properties": {
	        "LogGroupName": "/cfn/retain-7",
	        "RetentionInDays": 7
	      }
	    }
	  }
	}`
	created, err := st.CreateCFNStack(account, "us-east-1", "retain-stack", tpl, "")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := st.DescribeLogGroups(account, "/cfn/retain")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].RetentionInDays != 7 {
		t.Fatalf("groups=%+v", groups)
	}
	if err := st.DeleteCFNStack(account, created.StackName); err != nil {
		t.Fatal(err)
	}
}

func TestLogsPutRetentionPolicyAndPurge(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateLogGroup(account, "us-east-1", "/lab/ret"); err != nil {
		t.Fatal(err)
	}
	if err := st.PutRetentionPolicy(account, "/lab/ret", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogStream(account, "us-east-1", "/lab/ret", "s1"); err != nil {
		t.Fatal(err)
	}
	oldTS := time.Now().UTC().Add(-48 * time.Hour).UnixMilli()
	tok, _, err := st.PutLogEvents(account, "/lab/ret", "s1", "", []store.LogEvent{
		{Timestamp: oldTS, Message: "old"},
	})
	if err != nil {
		t.Fatal(err)
	}
	freshTS := time.Now().UTC().UnixMilli()
	if _, _, err := st.PutLogEvents(account, "/lab/ret", "s1", tok, []store.LogEvent{
		{Timestamp: freshTS, Message: "fresh"},
	}); err != nil {
		t.Fatal(err)
	}
	events, err := st.GetLogEvents(account, "/lab/ret", "s1", 0, 0, true, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Message != "fresh" {
		raw, _ := json.Marshal(events)
		t.Fatalf("want purged old event, got %s", raw)
	}
	groups, err := st.DescribeLogGroups(account, "/lab/ret")
	if err != nil || len(groups) != 1 || groups[0].RetentionInDays != 1 {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
	if err := st.DeleteRetentionPolicy(account, "/lab/ret"); err != nil {
		t.Fatal(err)
	}
	groups, err = st.DescribeLogGroups(account, "/lab/ret")
	if err != nil || len(groups) != 1 || groups[0].RetentionInDays != 0 {
		t.Fatalf("after delete retention groups=%+v err=%v", groups, err)
	}
}
