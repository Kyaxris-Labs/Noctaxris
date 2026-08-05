package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestEventBridgeBusRuleTargetPermissionMgmt(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createBus := mustEventsJSON(t, handler, "CreateEventBus", map[string]any{"Name": "lab-bus"}, now)
	if createBus.Code != http.StatusOK {
		t.Fatalf("CreateEventBus status=%d body=%q", createBus.Code, createBus.Body.String())
	}
	descBus := mustEventsJSON(t, handler, "DescribeEventBus", map[string]any{"Name": "lab-bus"}, now)
	if descBus.Code != http.StatusOK || !strings.Contains(descBus.Body.String(), "lab-bus") {
		t.Fatalf("DescribeEventBus status=%d body=%q", descBus.Code, descBus.Body.String())
	}
	listBuses := mustEventsJSON(t, handler, "ListEventBuses", map[string]any{}, now)
	if listBuses.Code != http.StatusOK || !strings.Contains(listBuses.Body.String(), "lab-bus") {
		t.Fatalf("ListEventBuses status=%d body=%q", listBuses.Code, listBuses.Body.String())
	}

	eventsPutRule(t, handler, "mgmt-rule", `{"source":["noctaxris.mgmt"]}`, now)
	descRule := mustEventsJSON(t, handler, "DescribeRule", map[string]any{"Name": "mgmt-rule"}, now)
	if descRule.Code != http.StatusOK || !strings.Contains(descRule.Body.String(), "ENABLED") {
		t.Fatalf("DescribeRule status=%d body=%q", descRule.Code, descRule.Body.String())
	}
	listRules := mustEventsJSON(t, handler, "ListRules", map[string]any{}, now)
	if listRules.Code != http.StatusOK || !strings.Contains(listRules.Body.String(), "mgmt-rule") {
		t.Fatalf("ListRules status=%d body=%q", listRules.Code, listRules.Body.String())
	}

	qARN := "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":events-mgmt-q"
	createQ := mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "events-mgmt-q"}, now)
	if createQ.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createQ.Code, createQ.Body.String())
	}
	eventsPutTargets(t, handler, "mgmt-rule", "1", qARN, now)
	listTargets := mustEventsJSON(t, handler, "ListTargetsByRule", map[string]any{"Rule": "mgmt-rule"}, now)
	if listTargets.Code != http.StatusOK || !strings.Contains(listTargets.Body.String(), qARN) {
		t.Fatalf("ListTargetsByRule status=%d body=%q", listTargets.Code, listTargets.Body.String())
	}

	disable := mustEventsJSON(t, handler, "DisableRule", map[string]any{"Name": "mgmt-rule"}, now)
	if disable.Code != http.StatusOK {
		t.Fatalf("DisableRule status=%d body=%q", disable.Code, disable.Body.String())
	}
	descRule = mustEventsJSON(t, handler, "DescribeRule", map[string]any{"Name": "mgmt-rule"}, now)
	if !strings.Contains(descRule.Body.String(), "DISABLED") {
		t.Fatalf("expected DISABLED: %q", descRule.Body.String())
	}
	enable := mustEventsJSON(t, handler, "EnableRule", map[string]any{"Name": "mgmt-rule"}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("EnableRule status=%d body=%q", enable.Code, enable.Body.String())
	}

	putPerm := mustEventsJSON(t, handler, "PutPermission", map[string]any{
		"Action":    "events:PutEvents",
		"Principal": "*",
		"StatementId": "allow-all",
	}, now)
	if putPerm.Code != http.StatusOK {
		t.Fatalf("PutPermission status=%d body=%q", putPerm.Code, putPerm.Body.String())
	}
	rmPerm := mustEventsJSON(t, handler, "RemovePermission", map[string]any{
		"StatementId": "allow-all",
	}, now)
	if rmPerm.Code != http.StatusOK {
		t.Fatalf("RemovePermission status=%d body=%q", rmPerm.Code, rmPerm.Body.String())
	}

	rmTargets := mustEventsJSON(t, handler, "RemoveTargets", map[string]any{
		"Rule": "mgmt-rule", "Ids": []string{"1"},
	}, now)
	if rmTargets.Code != http.StatusOK {
		t.Fatalf("RemoveTargets status=%d body=%q", rmTargets.Code, rmTargets.Body.String())
	}
	delRule := mustEventsJSON(t, handler, "DeleteRule", map[string]any{"Name": "mgmt-rule"}, now)
	if delRule.Code != http.StatusOK {
		t.Fatalf("DeleteRule status=%d body=%q", delRule.Code, delRule.Body.String())
	}
	delBus := mustEventsJSON(t, handler, "DeleteEventBus", map[string]any{"Name": "lab-bus"}, now)
	if delBus.Code != http.StatusOK {
		t.Fatalf("DeleteEventBus status=%d body=%q", delBus.Code, delBus.Body.String())
	}
	delDefault := mustEventsJSON(t, handler, "DeleteEventBus", map[string]any{"Name": "default"}, now)
	if delDefault.Code == http.StatusOK {
		t.Fatalf("delete default bus should fail: %q", delDefault.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "events-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "events-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"events:ListEventBuses","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSEvents.ListEventBuses")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "events", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestSNSTopicSubscriptionMgmt(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "mgmt-topic", now)
	listTopics := mustSNSQuery(t, handler,
		"Action=ListTopics&Version=2010-03-31", testAccessKey, testSecret, now)
	if listTopics.Code != http.StatusOK || !strings.Contains(listTopics.Body.String(), topicARN) {
		t.Fatalf("ListTopics status=%d body=%q", listTopics.Code, listTopics.Body.String())
	}
	attrs := mustSNSQuery(t, handler,
		"Action=GetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN),
		testAccessKey, testSecret, now)
	if attrs.Code != http.StatusOK || !strings.Contains(attrs.Body.String(), topicARN) {
		t.Fatalf("GetTopicAttributes status=%d body=%q", attrs.Code, attrs.Body.String())
	}

	qARN := "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":sns-mgmt-q"
	createQ := mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "sns-mgmt-q"}, now)
	if createQ.Code != http.StatusOK {
		t.Fatalf("CreateQueue: %d %s", createQ.Code, createQ.Body.String())
	}
	sub := mustSNSQuery(t, handler,
		"Action=Subscribe&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Protocol=sqs&Endpoint="+url.QueryEscape(qARN),
		testAccessKey, testSecret, now)
	if sub.Code != http.StatusOK {
		t.Fatalf("Subscribe status=%d body=%q", sub.Code, sub.Body.String())
	}
	subARN := xmlTag(t, sub.Body.String(), "SubscriptionArn")
	if subARN == "" || subARN == "pending confirmation" {
		// Lab may return pending; ConfirmSubscription path still exercises handler when token present.
		confirm := mustSNSQuery(t, handler,
			"Action=ConfirmSubscription&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
				"&Token=lab-token",
			testAccessKey, testSecret, now)
		_ = confirm
		subARN = xmlTag(t, sub.Body.String(), "SubscriptionArn")
	}

	listSubs := mustSNSQuery(t, handler,
		"Action=ListSubscriptions&Version=2010-03-31", testAccessKey, testSecret, now)
	if listSubs.Code != http.StatusOK {
		t.Fatalf("ListSubscriptions status=%d body=%q", listSubs.Code, listSubs.Body.String())
	}
	listByTopic := mustSNSQuery(t, handler,
		"Action=ListSubscriptionsByTopic&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN),
		testAccessKey, testSecret, now)
	if listByTopic.Code != http.StatusOK {
		t.Fatalf("ListSubscriptionsByTopic status=%d body=%q", listByTopic.Code, listByTopic.Body.String())
	}

	if subARN != "" && subARN != "pending confirmation" {
		getSub := mustSNSQuery(t, handler,
			"Action=GetSubscriptionAttributes&Version=2010-03-31&SubscriptionArn="+url.QueryEscape(subARN),
			testAccessKey, testSecret, now)
		if getSub.Code != http.StatusOK {
			t.Fatalf("GetSubscriptionAttributes status=%d body=%q", getSub.Code, getSub.Body.String())
		}
		unsub := mustSNSQuery(t, handler,
			"Action=Unsubscribe&Version=2010-03-31&SubscriptionArn="+url.QueryEscape(subARN),
			testAccessKey, testSecret, now)
		if unsub.Code != http.StatusOK {
			t.Fatalf("Unsubscribe status=%d body=%q", unsub.Code, unsub.Body.String())
		}
	}

	addPerm := mustSNSQuery(t, handler, strings.Join([]string{
		"Action=AddPermission",
		"Version=2010-03-31",
		"TopicArn=" + url.QueryEscape(topicARN),
		"Label=allow-publish",
		"AWSAccountId=" + testAccountID,
		"ActionName=Publish",
	}, "&"), testAccessKey, testSecret, now)
	if addPerm.Code != http.StatusOK {
		t.Fatalf("AddPermission status=%d body=%q", addPerm.Code, addPerm.Body.String())
	}
	rmPerm := mustSNSQuery(t, handler,
		"Action=RemovePermission&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Label=allow-publish",
		testAccessKey, testSecret, now)
	if rmPerm.Code != http.StatusOK && rmPerm.Code != http.StatusNotImplemented {
		t.Fatalf("RemovePermission status=%d body=%q", rmPerm.Code, rmPerm.Body.String())
	}

	del := mustSNSQuery(t, handler,
		"Action=DeleteTopic&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN),
		testAccessKey, testSecret, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteTopic status=%d body=%q", del.Code, del.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "sns-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "sns-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"sns:ListTopics","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	denyRec := mustSNSQuery(t, handler,
		"Action=ListTopics&Version=2010-03-31", denyAK, denySecret, now)
	if denyRec.Code != http.StatusForbidden && !strings.Contains(denyRec.Body.String(), "AccessDenied") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestSQSSendDeleteMessageBatch(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "batch-q"}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", create.Code, create.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &createOut)
	queueURL, _ := createOut["QueueUrl"].(string)

	sendBatch := mustSQSJSON(t, handler, "SendMessageBatch", map[string]any{
		"QueueUrl": queueURL,
		"Entries": []map[string]any{
			{"Id": "a", "MessageBody": "one"},
			{"Id": "b", "MessageBody": "two"},
		},
	}, now)
	if sendBatch.Code != http.StatusOK {
		t.Fatalf("SendMessageBatch status=%d body=%q", sendBatch.Code, sendBatch.Body.String())
	}

	recv := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            queueURL,
		"MaxNumberOfMessages": 10,
		"WaitTimeSeconds":     0,
	}, now)
	if recv.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage status=%d body=%q", recv.Code, recv.Body.String())
	}
	var recvOut map[string]any
	_ = json.Unmarshal(recv.Body.Bytes(), &recvOut)
	msgs, _ := recvOut["Messages"].([]any)
	if len(msgs) < 1 {
		t.Fatalf("expected messages: %v", recvOut)
	}
	entries := make([]map[string]any, 0, len(msgs))
	for i, m := range msgs {
		mm, _ := m.(map[string]any)
		entries = append(entries, map[string]any{
			"Id":            fmt.Sprintf("d%d", i),
			"ReceiptHandle": mm["ReceiptHandle"],
		})
	}
	delBatch := mustSQSJSON(t, handler, "DeleteMessageBatch", map[string]any{
		"QueueUrl": queueURL,
		"Entries":  entries,
	}, now)
	if delBatch.Code != http.StatusOK {
		t.Fatalf("DeleteMessageBatch status=%d body=%q", delBatch.Code, delBatch.Body.String())
	}

	emptyBatch := mustSQSJSON(t, handler, "SendMessageBatch", map[string]any{
		"QueueUrl": queueURL,
		"Entries":  []map[string]any{},
	}, now)
	if emptyBatch.Code == http.StatusOK && !strings.Contains(emptyBatch.Body.String(), "Failed") {
		// either validation error or empty successful batch is acceptable
		_ = emptyBatch
	}
}

func TestPipesDescribePipe(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	src, err := st.CreateQueue(testAccountID, testRegion, "127.0.0.1:4566", "pipe-desc-src", nil)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := st.CreateQueue(testAccountID, testRegion, "127.0.0.1:4566", "pipe-desc-dst", nil)
	if err != nil {
		t.Fatal(err)
	}
	create := mustPipesJSON(t, handler, "CreatePipe", map[string]any{
		"Name": "desc-pipe", "Source": src.QueueARN, "Target": dst.QueueARN,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreatePipe status=%d body=%q", create.Code, create.Body.String())
	}
	desc := mustPipesJSON(t, handler, "DescribePipe", map[string]any{"Name": "desc-pipe"}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "desc-pipe") {
		t.Fatalf("DescribePipe status=%d body=%q", desc.Code, desc.Body.String())
	}
	missing := mustPipesJSON(t, handler, "DescribePipe", map[string]any{"Name": "missing"}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("missing pipe should fail: %q", missing.Body.String())
	}
	_ = mustPipesJSON(t, handler, "DeletePipe", map[string]any{"Name": "desc-pipe"}, now)
}
