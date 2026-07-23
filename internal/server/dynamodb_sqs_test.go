package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustDynamoJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "dynamodb", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustSQSJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "sqs", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestDynamoDBPutGetRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "lab-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	putRec := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "lab-items",
		"Item": map[string]any{
			"pk":  map[string]any{"S": "row-1"},
			"val": map[string]any{"S": "hello-ddb"},
		},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	getRec := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "lab-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "row-1"},
		},
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetItem status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	item, _ := getOut["Item"].(map[string]any)
	if item == nil {
		t.Fatalf("missing Item in %q", getRec.Body.String())
	}
	valAV, _ := item["val"].(map[string]any)
	got, _ := valAV["S"].(string)
	if got != "hello-ddb" {
		t.Fatalf("val=%q want hello-ddb body=%q", got, getRec.Body.String())
	}
}

func TestDynamoDBResourcePolicyDenyGetItem(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "deny-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	putRec := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "deny-items",
		"Item": map[string]any{
			"pk": map[string]any{"S": "secret"},
		},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	denyPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":{"AWS":"*"},"Action":"dynamodb:GetItem","Resource":"*"}]}`
	polRec := mustDynamoJSON(t, handler, "PutResourcePolicy", map[string]any{
		"TableName": "deny-items",
		"Policy":    denyPolicy,
	}, now)
	if polRec.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", polRec.Code, polRec.Body.String())
	}

	getRec := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "deny-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "secret"},
		},
	}, now)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetItem status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("expected AccessDeniedException in %q", getRec.Body.String())
	}
}

func TestSQSSendReceiveDeleteRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "lab-queue",
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

	sendRec := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    queueURL,
		"MessageBody": "hello-sqs",
	}, now)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("SendMessage status=%d body=%q", sendRec.Code, sendRec.Body.String())
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
	if body != "hello-sqs" {
		t.Fatalf("Body=%q want hello-sqs", body)
	}
	handle, _ := msg["ReceiptHandle"].(string)
	if handle == "" {
		t.Fatalf("missing ReceiptHandle in %q", recvRec.Body.String())
	}

	delRec := mustSQSJSON(t, handler, "DeleteMessage", map[string]any{
		"QueueUrl":      queueURL,
		"ReceiptHandle": handle,
	}, now)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DeleteMessage status=%d body=%q", delRec.Code, delRec.Body.String())
	}
}

func TestSQSQueuePolicyDenySendMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "deny-queue",
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

	// Queue policy is resource-based: Principal required (match-none if omitted).
	denyPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":{"AWS":"*"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	setRec := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl": queueURL,
		"Attributes": map[string]string{
			"Policy": denyPolicy,
		},
	}, now)
	if setRec.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes status=%d body=%q", setRec.Code, setRec.Body.String())
	}

	sendRec := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    queueURL,
		"MessageBody": "nope",
	}, now)
	if sendRec.Code != http.StatusForbidden {
		t.Fatalf("SendMessage status=%d want 403 body=%q", sendRec.Code, sendRec.Body.String())
	}
	if !strings.Contains(sendRec.Body.String(), "AccessDenied") {
		t.Fatalf("expected AccessDenied in %q", sendRec.Body.String())
	}
}

func TestDynamoDBSSEKMSPutGet(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createKeyRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createKeyRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createKeyRec.Code, createKeyRec.Body.String())
	}
	var createKeyOut map[string]any
	if err := json.Unmarshal(createKeyRec.Body.Bytes(), &createKeyOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createKeyOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	if keyID == "" {
		t.Fatal("missing KeyId")
	}

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "kms-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"SSESpecification": map[string]any{
			"Enabled":        true,
			"SSEType":        "KMS",
			"KMSMasterKeyId": keyID,
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable SSE-KMS status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	putRec := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "kms-items",
		"Item": map[string]any{
			"pk":  map[string]any{"S": "enc-1"},
			"val": map[string]any{"S": "sse-kms-secret"},
		},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	getRec := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "kms-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "enc-1"},
		},
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetItem status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	item, _ := getOut["Item"].(map[string]any)
	valAV, _ := item["val"].(map[string]any)
	got, _ := valAV["S"].(string)
	if got != "sse-kms-secret" {
		t.Fatalf("val=%q want sse-kms-secret body=%q", got, getRec.Body.String())
	}
}

func TestDynamoDBConditionExpressionPutItem(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "cond-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	first := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "cond-items",
		"Item": map[string]any{
			"pk": map[string]any{"S": "a"},
		},
		"ConditionExpression": "attribute_not_exists(pk)",
	}, now)
	if first.Code != http.StatusOK {
		t.Fatalf("first PutItem status=%d body=%q", first.Code, first.Body.String())
	}

	second := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "cond-items",
		"Item": map[string]any{
			"pk": map[string]any{"S": "a"},
		},
		"ConditionExpression": "attribute_not_exists(pk)",
	}, now)
	if second.Code != http.StatusBadRequest || !strings.Contains(second.Body.String(), "ConditionalCheckFailedException") {
		t.Fatalf("second PutItem want ConditionalCheckFailed, status=%d body=%q", second.Code, second.Body.String())
	}

	unsupported := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "cond-items",
		"Item": map[string]any{
			"pk": map[string]any{"S": "b"},
		},
		"ConditionExpression": "size(tags) > :n",
		"ExpressionAttributeValues": map[string]any{
			":n": map[string]any{"N": "1"},
		},
	}, now)
	if unsupported.Code != http.StatusBadRequest || !strings.Contains(unsupported.Body.String(), "ValidationException") {
		t.Fatalf("unsupported want ValidationException, status=%d body=%q", unsupported.Code, unsupported.Body.String())
	}
}
