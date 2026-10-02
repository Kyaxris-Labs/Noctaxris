package server_test

import (
	"archive/zip"
	"bytes"
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

func iotRuleTrustDoc() string {
	return `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"iot.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
}

func mustIoTDeliveryRole(t *testing.T, st *store.Store, roleName, allowAction, resourceARN string) string {
	t.Helper()
	roleARN, err := st.CreateRole(testAccountID, roleName, iotRuleTrustDoc())
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	allow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"` + allowAction + `","Resource":"` + resourceARN + `"}]}`
	if err := st.PutInlinePolicy(roleARN, "deliver", allow); err != nil {
		t.Fatalf("PutInlinePolicy: %v", err)
	}
	return roleARN
}

func TestIoTTopicRuleSQSDispatchWithoutMQTT(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	q, err := st.CreateQueue(testAccountID, testRegion, "127.0.0.1:4566", "iot-rule-q", nil)
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	roleARN := mustIoTDeliveryRole(t, st, "iot-sqs-deliver", "sqs:SendMessage", q.QueueARN)

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "sqs-rule",
		"topicRulePayload": map[string]any{
			"sql":         "SELECT * FROM 'dt/device/+'",
			"description": "sqs lite",
			"actions": []any{
				map[string]any{"sqs": map[string]any{
					"queueUrl": q.QueueURL,
					"roleArn":  roleARN,
				}},
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

func TestIoTTopicRuleSQSDispatchDeniedWithoutRolePermission(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	q, err := st.CreateQueue(testAccountID, testRegion, "127.0.0.1:4566", "iot-rule-deny-q", nil)
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	roleARN, err := st.CreateRole(testAccountID, "iot-sqs-deny", iotRuleTrustDoc())
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	// Trust only; no sqs:SendMessage identity Allow → EvaluateFull denies delivery.
	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "sqs-deny-rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/deny/+'",
			"actions": []any{
				map[string]any{"sqs": map[string]any{
					"queueUrl": q.QueueURL,
					"roleArn":  roleARN,
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}

	srv.PublishTopic(testAccountID, testRegion, "dt/deny/1", []byte(`{"temp":1}`), true)
	msgs, err := st.ReceiveMessages(testAccountID, "iot-rule-deny-q", 10)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 SQS messages when role lacks permission, got %d", len(msgs))
	}
}

func TestIoTTopicRuleS3DispatchWithoutMQTT(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	const bucket = "iot-rule-bucket"
	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	objectARN := store.ObjectARN(bucket, "from-rule.json")
	roleARN := mustIoTDeliveryRole(t, st, "iot-s3-deliver", "s3:PutObject", objectARN)

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "s3-rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'lab/data'",
			"actions": []any{
				map[string]any{"s3": map[string]any{
					"bucketName": bucket,
					"key":        "from-rule.json",
					"roleArn":    roleARN,
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}

	payload := []byte(`{"ok":true}`)
	srv.PublishTopic(testAccountID, testRegion, "lab/data", payload, true)

	_, data, err := st.GetObject(testAccountID, bucket, "from-rule.json")
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

func TestIoTTopicRuleS3DispatchDeniedMissingRoleArn(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	const bucket = "iot-rule-norole-bucket"
	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "s3-norole",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'lab/norole'",
			"actions": []any{
				map[string]any{"s3": map[string]any{
					"bucketName": bucket,
					"key":        "nope.json",
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}
	srv.PublishTopic(testAccountID, testRegion, "lab/norole", []byte(`{"x":1}`), true)
	if _, _, err := st.GetObject(testAccountID, bucket, "nope.json"); err == nil {
		t.Fatal("expected no object when roleArn missing")
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

func TestIoTTopicRuleSQSDispatchDeniedMissingRoleArn(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	q, err := st.CreateQueue(testAccountID, testRegion, "127.0.0.1:4566", "iot-sqs-noroIe", nil)
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "sqs-noroIe",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/sqs/noroIe'",
			"actions": []any{
				map[string]any{"sqs": map[string]any{"queueUrl": q.QueueURL}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}
	srv.PublishTopic(testAccountID, testRegion, "dt/sqs/noroIe", []byte(`{"x":1}`), true)
	msgs, err := st.ReceiveMessages(testAccountID, "iot-sqs-noroIe", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 SQS messages when roleArn missing, got %d", len(msgs))
	}
}

func TestIoTTopicRuleSNSDispatchAuthz(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	topic, err := st.CreateTopic(testAccountID, testRegion, "iot-rule-sns", nil)
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	q, err := st.CreateQueue(testAccountID, testRegion, "127.0.0.1:4566", "iot-sns-q", nil)
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	if _, err := st.Subscribe(testAccountID, topic.TopicARN, "sqs", q.QueueURL); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	roleARN := mustIoTDeliveryRole(t, st, "iot-sns-deliver", "sns:Publish", topic.TopicARN)

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "sns-rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/sns/ok'",
			"actions": []any{
				map[string]any{"sns": map[string]any{
					"targetArn": topic.TopicARN,
					"roleArn":   roleARN,
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}
	qPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	if err := st.SetQueueAttributes(testAccountID, "iot-sns-q", map[string]string{"Policy": qPolicy}); err != nil {
		t.Fatalf("SetQueueAttributes: %v", err)
	}
	srv.PublishTopic(testAccountID, testRegion, "dt/sns/ok", []byte(`{"ok":true}`), true)
	msgs, err := st.ReceiveMessages(testAccountID, "iot-sns-q", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 SNS->SQS message, got %d", len(msgs))
	}

	// Missing roleArn must skip.
	create2 := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "sns-noroIe",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/sns/noroIe'",
			"actions": []any{
				map[string]any{"sns": map[string]any{"targetArn": topic.TopicARN}},
			},
		},
	}, now)
	if create2.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create2.Code, create2.Body.String())
	}
	srv.PublishTopic(testAccountID, testRegion, "dt/sns/noroIe", []byte(`{"x":1}`), true)
	msgs2, err := st.ReceiveMessages(testAccountID, "iot-sns-q", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs2) != 0 {
		t.Fatalf("expected 0 messages when SNS roleArn missing, got %d", len(msgs2))
	}
}

func TestIoTTopicRuleDynamoDispatchAuthz(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	tbl, err := st.CreateTable(testAccountID, testRegion, "iot-rule-ddb", "id", "S", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	roleARN := mustIoTDeliveryRole(t, st, "iot-ddb-deliver", "dynamodb:PutItem", tbl.TableARN)

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "ddb-rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/ddb'",
			"actions": []any{
				map[string]any{"dynamoDB": map[string]any{
					"tableName": tbl.TableName,
					"roleArn":   roleARN,
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}
	srv.PublishTopic(testAccountID, testRegion, "dt/ddb", []byte(`{"id":"row-1","temp":9}`), true)
	page, err := st.ScanItems(testAccountID, tbl.TableName, 10, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 dynamo item, got %d", len(page.Items))
	}

	create2 := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "ddb-noroIe",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/ddb/noroIe'",
			"actions": []any{
				map[string]any{"dynamoDB": map[string]any{"tableName": tbl.TableName}},
			},
		},
	}, now)
	if create2.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create2.Code, create2.Body.String())
	}
	srv.PublishTopic(testAccountID, testRegion, "dt/ddb/noroIe", []byte(`{"id":"row-2"}`), true)
	page2, err := st.ScanItems(testAccountID, tbl.TableName, 10, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("expected still 1 item when roleArn missing, got %d", len(page2.Items))
	}
}

func TestIoTTopicRuleKinesisDispatchAuthz(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	stream, err := st.CreateKinesisStream(testAccountID, testRegion, "iot-rule-kinesis", 1)
	if err != nil {
		t.Fatalf("CreateKinesisStream: %v", err)
	}
	roleARN := mustIoTDeliveryRole(t, st, "iot-kinesis-deliver", "kinesis:PutRecord", stream.StreamARN)

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "kinesis-rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/kinesis'",
			"actions": []any{
				map[string]any{"kinesis": map[string]any{
					"streamName": stream.StreamName,
					"roleArn":    roleARN,
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}
	srv.PublishTopic(testAccountID, testRegion, "dt/kinesis", []byte(`{"n":1}`), true)
	it, err := st.GetKinesisShardIterator(testAccountID, stream.StreamName, store.LabKinesisShardID(0), "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	recs, _, err := st.GetKinesisRecords(testAccountID, it, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || string(recs[0].Data) != `{"n":1}` {
		t.Fatalf("kinesis records=%v", recs)
	}

	create2 := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "kinesis-noroIe",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/kinesis/noroIe'",
			"actions": []any{
				map[string]any{"kinesis": map[string]any{"streamName": stream.StreamName}},
			},
		},
	}, now)
	if create2.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create2.Code, create2.Body.String())
	}
	srv.PublishTopic(testAccountID, testRegion, "dt/kinesis/noroIe", []byte(`{"n":2}`), true)
	it2, err := st.GetKinesisShardIterator(testAccountID, stream.StreamName, store.LabKinesisShardID(0), "LATEST", "")
	if err != nil {
		t.Fatal(err)
	}
	recs2, _, err := st.GetKinesisRecords(testAccountID, it2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs2) != 0 {
		t.Fatalf("expected 0 kinesis records when roleArn missing, got %d", len(recs2))
	}
}

func TestIoTTopicRuleLambdaDispatchAuthz(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC()
	handler := srv.Handler()

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("app.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("def handler(e,c): return e")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    testAccountID,
		Region:       testRegion,
		FunctionName: "iot-rule-fn",
		RoleARN:      "arn:aws:iam::" + testAccountID + ":role/lambda-exec",
		Runtime:      store.LambdaRuntimePython312,
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		Zip:          zipBuf.Bytes(),
	})
	if err != nil {
		t.Fatalf("CreateFunction: %v", err)
	}
	roleARN := mustIoTDeliveryRole(t, st, "iot-lambda-deliver", "lambda:InvokeFunction", fn.FunctionARN)

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "lambda-rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/lambda'",
			"actions": []any{
				map[string]any{"lambda": map[string]any{
					"functionArn": fn.FunctionARN,
					"roleArn":     roleARN,
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create.Code, create.Body.String())
	}
	srv.PublishTopic(testAccountID, testRegion, "dt/lambda", []byte(`{"ok":true}`), true)
	job, err := st.LatestAsyncInvocation(testAccountID, fn.FunctionName)
	if err != nil {
		t.Fatalf("expected async invoke after role delivery: %v", err)
	}
	if job.EventJSON != `{"ok":true}` {
		t.Fatalf("event=%q", job.EventJSON)
	}

	// No roleArn and no function resource policy → deny.
	create2 := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "lambda-deny",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/lambda/deny'",
			"actions": []any{
				map[string]any{"lambda": map[string]any{"functionArn": fn.FunctionARN}},
			},
		},
	}, now)
	if create2.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create2.Code, create2.Body.String())
	}
	beforeID := job.InvocationID
	srv.PublishTopic(testAccountID, testRegion, "dt/lambda/deny", []byte(`{"denied":true}`), true)
	job2, err := st.LatestAsyncInvocation(testAccountID, fn.FunctionName)
	if err != nil {
		t.Fatal(err)
	}
	if job2.InvocationID != beforeID {
		t.Fatalf("expected no new invoke without role/policy, got %s", job2.InvocationID)
	}

	// Resource-policy path (AWS Lambda action shape) without roleArn.
	if _, err := st.AddFunctionPermission(testAccountID, fn.FunctionName, "iot-invoke", "lambda:InvokeFunction", "iot.amazonaws.com", "", ""); err != nil {
		t.Fatalf("AddFunctionPermission: %v", err)
	}
	create3 := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "lambda-policy",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'dt/lambda/policy'",
			"actions": []any{
				map[string]any{"lambda": map[string]any{"functionArn": fn.FunctionARN}},
			},
		},
	}, now)
	if create3.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%s", create3.Code, create3.Body.String())
	}
	gotPolicy := make(chan store.LambdaAsyncInvocation, 1)
	st.SetOnAsyncEnqueue(func(job store.LambdaAsyncInvocation) {
		if job.FunctionName == fn.FunctionName && job.EventJSON == `{"via":"policy"}` {
			select {
			case gotPolicy <- job:
			default:
			}
		}
	})
	srv.PublishTopic(testAccountID, testRegion, "dt/lambda/policy", []byte(`{"via":"policy"}`), true)
	select {
	case job3 := <-gotPolicy:
		if job3.EventJSON != `{"via":"policy"}` {
			t.Fatalf("expected resource-policy invoke, event=%q", job3.EventJSON)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected resource-policy invoke (iot.amazonaws.com function policy)")
	}
}
