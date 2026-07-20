package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustEventsJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSEvents."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "events", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func eventsPutRule(t *testing.T, handler http.Handler, name, pattern string, now time.Time) {
	t.Helper()
	rec := mustEventsJSON(t, handler, "PutRule", map[string]any{
		"Name":         name,
		"EventPattern": pattern,
		"State":        "ENABLED",
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("PutRule status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func eventsPutTargets(t *testing.T, handler http.Handler, ruleName, targetID, targetARN string, now time.Time) {
	t.Helper()
	rec := mustEventsJSON(t, handler, "PutTargets", map[string]any{
		"Rule": ruleName,
		"Targets": []map[string]any{{
			"Id":  targetID,
			"Arn": targetARN,
		}},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("PutTargets status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func eventsPutTargetsWithRole(t *testing.T, handler http.Handler, ruleName, targetID, targetARN, roleARN string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	return mustEventsJSON(t, handler, "PutTargets", map[string]any{
		"Rule": ruleName,
		"Targets": []map[string]any{{
			"Id":      targetID,
			"Arn":     targetARN,
			"RoleArn": roleARN,
		}},
	}, now)
}

func TestEventBridgePutEventsDeliversToSQS(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "evt-bridge-sqs",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := createOut["QueueUrl"].(string)
	if queueURL == "" {
		t.Fatalf("missing QueueUrl in %q", createRec.Body.String())
	}
	queueARN := "arn:aws:sqs:us-east-1:" + testAccountID + ":evt-bridge-sqs"
	eventsQueuePolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + queueARN + `"}]}`
	setRec := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl":   queueURL,
		"Attributes": map[string]string{"Policy": eventsQueuePolicy},
	}, now)
	if setRec.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes status=%d body=%q", setRec.Code, setRec.Body.String())
	}

	eventsPutRule(t, handler, "sqs-rule", `{"source":["noctaxris.lab"]}`, now)
	eventsPutTargets(t, handler, "sqs-rule", "1", queueARN, now)

	putRec := mustEventsJSON(t, handler, "PutEvents", map[string]any{
		"Entries": []map[string]any{{
			"Source":     "noctaxris.lab",
			"DetailType": "demo",
			"Detail":     `{"ok":true}`,
		}},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutEvents status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	recvRec := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            queueURL,
		"MaxNumberOfMessages": 1,
	}, now)
	if recvRec.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage status=%d body=%q", recvRec.Code, recvRec.Body.String())
	}
	var recvOut map[string]any
	if err := json.Unmarshal(recvRec.Body.Bytes(), &recvOut); err != nil {
		t.Fatal(err)
	}
	msgs, _ := recvOut["Messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("Messages len=%d body=%q", len(msgs), recvRec.Body.String())
	}
	msg, _ := msgs[0].(map[string]any)
	body, _ := msg["Body"].(string)
	if !strings.Contains(body, `"source":"noctaxris.lab"`) && !strings.Contains(body, `"source": "noctaxris.lab"`) {
		t.Fatalf("Body=%q want eventbridge source", body)
	}
	if !strings.Contains(body, `"detail-type"`) {
		t.Fatalf("Body=%q want detail-type", body)
	}
}

func TestEventBridgePutEventsDeliversToLambda(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "evt-lambda-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/evt-lambda-exec"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "evt-target-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	fnARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:evt-target-fn"
	addRec := mustLambdaJSON(t, handler, "AddPermission", map[string]any{
		"FunctionName": "evt-target-fn",
		"StatementId":  "events-root",
		"Action":       "lambda:InvokeFunction",
		"Principal":    "arn:aws:iam::" + testAccountID + ":root",
	}, now)
	if addRec.Code != http.StatusOK {
		t.Fatalf("AddPermission status=%d body=%q", addRec.Code, addRec.Body.String())
	}

	eventsPutRule(t, handler, "lambda-rule", `{"source":["noctaxris.lab"]}`, now)
	eventsPutTargets(t, handler, "lambda-rule", "1", fnARN, now)

	putRec := mustEventsJSON(t, handler, "PutEvents", map[string]any{
		"Entries": []map[string]any{{
			"Source":     "noctaxris.lab",
			"DetailType": "lambda-demo",
			"Detail":     `{"n":42}`,
		}},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutEvents status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	job, err := st.LatestAsyncInvocation(testAccountID, "evt-target-fn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(job.EventJSON, `"source":"noctaxris.lab"`) && !strings.Contains(job.EventJSON, `"source": "noctaxris.lab"`) {
		t.Fatalf("EventJSON=%q want source noctaxris.lab", job.EventJSON)
	}
	if !strings.Contains(job.EventJSON, `"detail-type"`) {
		t.Fatalf("EventJSON=%q want detail-type", job.EventJSON)
	}
}

const eventsTrustBad = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}`

func TestEventBridgePutEventsSkipsDeliveryWithoutResourcePolicy(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "evt-bridge-deny",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := createOut["QueueUrl"].(string)
	if queueURL == "" {
		t.Fatalf("missing QueueUrl in %q", createRec.Body.String())
	}
	queueARN := "arn:aws:sqs:us-east-1:" + testAccountID + ":evt-bridge-deny"

	eventsPutRule(t, handler, "sqs-deny-rule", `{"source":["noctaxris.lab"]}`, now)
	eventsPutTargets(t, handler, "sqs-deny-rule", "1", queueARN, now)

	putRec := mustEventsJSON(t, handler, "PutEvents", map[string]any{
		"Entries": []map[string]any{{
			"Source":     "noctaxris.lab",
			"DetailType": "demo",
			"Detail":     `{"blocked":true}`,
		}},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutEvents status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	recvRec := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            queueURL,
		"MaxNumberOfMessages": 1,
	}, now)
	if recvRec.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage status=%d body=%q", recvRec.Code, recvRec.Body.String())
	}
	var recvOut map[string]any
	if err := json.Unmarshal(recvRec.Body.Bytes(), &recvOut); err != nil {
		t.Fatal(err)
	}
	msgs, _ := recvOut["Messages"].([]any)
	if len(msgs) != 0 {
		t.Fatalf("Messages len=%d want 0 without resource policy", len(msgs))
	}
}

func TestEventBridgePutTargetsPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "no-events-trust", eventsTrustBad, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/no-events-trust"
	targetARN := "arn:aws:sqs:us-east-1:" + testAccountID + ":noop"

	eventsPutRule(t, handler, "passrole-rule", `{"source":["x"]}`, now)

	rec := eventsPutTargetsWithRole(t, handler, "passrole-rule", "1", targetARN, roleARN, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PutTargets status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("expected AccessDeniedException in %q", rec.Body.String())
	}
}

func TestEventBridgePutEventsDeliversToSNS(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "evt-bridge-sns", now)
	topicPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sns:Publish","Resource":"` + topicARN + `"}]}`
	setRec := mustSNSQuery(t, handler,
		"Action=SetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&AttributeName=Policy&AttributeValue="+url.QueryEscape(topicPolicy),
		testAccessKey, testSecret, now)
	if setRec.Code != http.StatusOK {
		t.Fatalf("SetTopicAttributes status=%d body=%q", setRec.Code, setRec.Body.String())
	}

	createRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "evt-bridge-sns-q",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := createOut["QueueUrl"].(string)
	queueARN := "arn:aws:sqs:us-east-1:" + testAccountID + ":evt-bridge-sns-q"

	sqsPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	setQ := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl": queueURL,
		"Attributes": map[string]string{
			"Policy": sqsPolicy,
		},
	}, now)
	if setQ.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes status=%d body=%q", setQ.Code, setQ.Body.String())
	}

	subRec := mustSNSQuery(t, handler,
		"Action=Subscribe&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Protocol=sqs&Endpoint="+url.QueryEscape(queueARN),
		testAccessKey, testSecret, now)
	if subRec.Code != http.StatusOK {
		t.Fatalf("Subscribe status=%d body=%q", subRec.Code, subRec.Body.String())
	}

	eventsPutRule(t, handler, "sns-rule", `{"source":["noctaxris.lab"]}`, now)
	eventsPutTargets(t, handler, "sns-rule", "1", topicARN, now)

	putRec := mustEventsJSON(t, handler, "PutEvents", map[string]any{
		"Entries": []map[string]any{{
			"Source":     "noctaxris.lab",
			"DetailType": "sns-demo",
			"Detail":     `{"ok":true}`,
		}},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutEvents status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	recvRec := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            queueURL,
		"MaxNumberOfMessages": 1,
	}, now)
	if recvRec.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage status=%d body=%q", recvRec.Code, recvRec.Body.String())
	}
	var recvOut map[string]any
	if err := json.Unmarshal(recvRec.Body.Bytes(), &recvOut); err != nil {
		t.Fatal(err)
	}
	msgs, _ := recvOut["Messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("Messages len=%d body=%q", len(msgs), recvRec.Body.String())
	}
}
