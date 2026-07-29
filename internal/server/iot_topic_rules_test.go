package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIoTTopicSQLMatch(t *testing.T) {
	if got := store.ExtractIoTTopicFilter(`SELECT * FROM 'dt/device/+'`); got != "dt/device/+" {
		t.Fatalf("filter=%q", got)
	}
	if !store.IoTTopicMatches("dt/device/+", "dt/device/1") {
		t.Fatal("expected + match")
	}
	if store.IoTTopicMatches("dt/device/+", "dt/device/1/x") {
		t.Fatal("expected + non-match for extra level")
	}
	if !store.IoTTopicMatches("sensors/#", "sensors/a/b") {
		t.Fatal("expected # match")
	}
	if store.IoTTopicMatches("sensors/#", "other/a") {
		t.Fatal("expected # non-match")
	}
}

func TestIoTTopicRuleSQSDispatchWithoutMQTT(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	q, err := st.CreateQueue(testAccountID, testRegion, "127.0.0.1:4566", "iot-rule-q", nil)
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "sqs-rule",
		"topicRulePayload": map[string]any{
			"sql":         "SELECT * FROM 'dt/device/+'",
			"description": "sqs lite",
			"actions": []any{
				map[string]any{"sqs": map[string]any{"queueUrl": q.QueueURL}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}

	srv.PublishTopic(testAccountID, testRegion, "dt/device/42", []byte(`{"temp":21}`), true)

	msgs, err := st.ReceiveMessages(testAccountID, "iot-rule-q", 10)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 SQS message, got %d", len(msgs))
	}
	if string(msgs[0].Body) != `{"temp":21}` {
		t.Fatalf("body=%q", msgs[0].Body)
	}

	// Non-matching topic must not dispatch.
	srv.PublishTopic(testAccountID, testRegion, "other/topic", []byte(`{"x":1}`), true)
	msgs2, err := st.ReceiveMessages(testAccountID, "iot-rule-q", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs2) != 0 {
		t.Fatalf("expected no extra messages, got %d", len(msgs2))
	}
}

func TestIoTTopicRuleS3DispatchWithoutMQTT(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	if _, err := st.CreateBucket(testAccountID, "iot-rule-bucket"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "s3-rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'lab/data'",
			"actions": []any{
				map[string]any{"s3": map[string]any{
					"bucketName": "iot-rule-bucket",
					"key":        "from-rule.json",
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}

	payload := []byte(`{"ok":true}`)
	srv.PublishTopic(testAccountID, testRegion, "lab/data", payload, true)

	_, data, err := st.GetObject(testAccountID, "iot-rule-bucket", "from-rule.json")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("object=%q", data)
	}

	get := mustJSONTarget(t, handler, "AWSIotService.GetTopicRule", "iot", map[string]any{
		"ruleName": "s3-rule",
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetTopicRule status=%d body=%s", get.Code, get.Body.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	rule, _ := parsed["rule"].(map[string]any)
	if rule["sql"] != "SELECT * FROM 'lab/data'" {
		t.Fatalf("rule=%v", rule)
	}
}

func TestIoTTopicRuleMissingTargetFailClosed(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "missing-sqs",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'x/y'",
			"actions": []any{
				map[string]any{"sqs": map[string]any{
					"queueUrl": "http://127.0.0.1:4566/" + testAccountID + "/no-such-queue",
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}

	// Must not panic when target is missing.
	srv.PublishTopic(testAccountID, testRegion, "x/y", []byte(`{}`), true)

	list, err := st.ListIoTTopicRules(testAccountID, testRegion)
	if err != nil || len(list) != 1 {
		t.Fatalf("rules still stored: %v %v", list, err)
	}
	_ = store.ExtractIoTTopicFilter(list[0].SQL)
}
