package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustDynamoStreamsJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "DynamoDBStreams_20120810."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "dynamodb", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestDynamoDBStreamsOldAndNewImages(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName":            "stream-old-new",
		"AttributeDefinitions": []map[string]string{{"AttributeName": "pk", "AttributeType": "S"}},
		"KeySchema":            []map[string]string{{"AttributeName": "pk", "KeyType": "HASH"}},
		"BillingMode":          "PAY_PER_REQUEST",
		"StreamSpecification": map[string]any{
			"StreamEnabled":  true,
			"StreamViewType": "NEW_AND_OLD_IMAGES",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", create.Code, create.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	desc, _ := createOut["TableDescription"].(map[string]any)
	streamARN, _ := desc["LatestStreamArn"].(string)

	put1 := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "stream-old-new",
		"Item":      map[string]any{"pk": map[string]string{"S": "1"}, "v": map[string]string{"S": "old"}},
	}, now)
	if put1.Code != http.StatusOK {
		t.Fatalf("PutItem1 status=%d body=%q", put1.Code, put1.Body.String())
	}
	put2 := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "stream-old-new",
		"Item":      map[string]any{"pk": map[string]string{"S": "1"}, "v": map[string]string{"S": "new"}},
	}, now)
	if put2.Code != http.StatusOK {
		t.Fatalf("PutItem2 status=%d body=%q", put2.Code, put2.Body.String())
	}

	itRec := mustDynamoStreamsJSON(t, handler, "GetShardIterator", map[string]any{
		"StreamArn":         streamARN,
		"ShardId":           store.LabDynamoStreamShardID,
		"ShardIteratorType": "TRIM_HORIZON",
	}, now)
	var itOut map[string]any
	if err := json.Unmarshal(itRec.Body.Bytes(), &itOut); err != nil {
		t.Fatal(err)
	}
	get := mustDynamoStreamsJSON(t, handler, "GetRecords", map[string]any{
		"ShardIterator": itOut["ShardIterator"],
		"Limit":         10,
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetRecords status=%d body=%q", get.Code, get.Body.String())
	}
	body := get.Body.String()
	if !strings.Contains(body, `"MODIFY"`) || !strings.Contains(body, `"OldImage"`) || !strings.Contains(body, `"old"`) {
		t.Fatalf("want MODIFY with OldImage, body=%q", body)
	}
	if !strings.Contains(body, `"NewImage"`) || !strings.Contains(body, `"new"`) {
		t.Fatalf("want NewImage, body=%q", body)
	}
}

func TestDynamoDBStreamsRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName":            "stream-lab",
		"AttributeDefinitions": []map[string]string{{"AttributeName": "pk", "AttributeType": "S"}},
		"KeySchema":            []map[string]string{{"AttributeName": "pk", "KeyType": "HASH"}},
		"BillingMode":          "PAY_PER_REQUEST",
		"StreamSpecification": map[string]any{
			"StreamEnabled":  true,
			"StreamViewType": "NEW_IMAGE",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", create.Code, create.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	desc, _ := createOut["TableDescription"].(map[string]any)
	streamARN, _ := desc["LatestStreamArn"].(string)
	if streamARN == "" {
		t.Fatalf("missing stream arn: %q", create.Body.String())
	}

	put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "stream-lab",
		"Item":      map[string]any{"pk": map[string]string{"S": "1"}, "v": map[string]string{"S": "hello"}},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", put.Code, put.Body.String())
	}

	list := mustDynamoStreamsJSON(t, handler, "ListStreams", map[string]any{"TableName": "stream-lab"}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListStreams status=%d body=%q", list.Code, list.Body.String())
	}

	descStream := mustDynamoStreamsJSON(t, handler, "DescribeStream", map[string]any{"StreamArn": streamARN}, now)
	if descStream.Code != http.StatusOK {
		t.Fatalf("DescribeStream status=%d body=%q", descStream.Code, descStream.Body.String())
	}

	itRec := mustDynamoStreamsJSON(t, handler, "GetShardIterator", map[string]any{
		"StreamArn":         streamARN,
		"ShardId":           store.LabDynamoStreamShardID,
		"ShardIteratorType": "TRIM_HORIZON",
	}, now)
	if itRec.Code != http.StatusOK {
		t.Fatalf("GetShardIterator status=%d body=%q", itRec.Code, itRec.Body.String())
	}
	var itOut map[string]any
	if err := json.Unmarshal(itRec.Body.Bytes(), &itOut); err != nil {
		t.Fatal(err)
	}
	iterator, _ := itOut["ShardIterator"].(string)
	get := mustDynamoStreamsJSON(t, handler, "GetRecords", map[string]any{
		"ShardIterator": iterator,
		"Limit":         10,
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetRecords status=%d body=%q", get.Code, get.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	recs, _ := getOut["Records"].([]any)
	if len(recs) != 1 {
		t.Fatalf("records=%v", getOut["Records"])
	}
}

func mustPipesJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "Pipes."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "pipes", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestPipesCreateListDelete(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	account := "000000000001"
	src, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-h-src", nil)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-h-dst", nil)
	if err != nil {
		t.Fatal(err)
	}
	create := mustPipesJSON(t, handler, "CreatePipe", map[string]any{
		"Name":   "handler-pipe",
		"Source": src.QueueARN,
		"Target": dst.QueueARN,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreatePipe status=%d body=%q", create.Code, create.Body.String())
	}
	list := mustPipesJSON(t, handler, "ListPipes", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListPipes status=%d body=%q", list.Code, list.Body.String())
	}
	del := mustPipesJSON(t, handler, "DeletePipe", map[string]any{"Name": "handler-pipe"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeletePipe status=%d body=%q", del.Code, del.Body.String())
	}
}

func mustMQJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "mq.CreateBroker")
	if target != "CreateBroker" {
		req.Header.Set("X-Amz-Target", "mq."+target)
	}
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "mq", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestMQBrokerHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	create := mustMQJSON(t, handler, "CreateBroker", map[string]any{
		"BrokerName": "h-broker",
		"EngineType": "RABBITMQ",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateBroker status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	id, _ := out["BrokerId"].(string)
	desc := mustMQJSON(t, handler, "DescribeBroker", map[string]any{"BrokerId": id}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeBroker status=%d body=%q", desc.Code, desc.Body.String())
	}
	// Unit test server has no DinD: RabbitMQ must fail closed, not invent RUNNING.
	body := desc.Body.String()
	if !strings.Contains(body, `"BrokerState":"CREATION_FAILED"`) && !strings.Contains(body, `"BrokerState": "CREATION_FAILED"`) {
		t.Fatalf("want CREATION_FAILED without DinD; body=%q", body)
	}
	if strings.Contains(body, `"BrokerState":"RUNNING"`) {
		t.Fatalf("must not claim RUNNING without nested broker; body=%q", body)
	}
	list := mustMQJSON(t, handler, "ListBrokers", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListBrokers status=%d body=%q", list.Code, list.Body.String())
	}
	del := mustMQJSON(t, handler, "DeleteBroker", map[string]any{"BrokerId": id}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteBroker status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestCreateBrokerActiveMQWithoutComputeFailsClosed(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	create := mustMQJSON(t, handler, "CreateBroker", map[string]any{
		"BrokerName": "amq-broker",
		"EngineType": "ACTIVEMQ",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateBroker status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	id, _ := out["BrokerId"].(string)
	if id == "" {
		t.Fatalf("missing BrokerId: %v", out)
	}
	desc := mustMQJSON(t, handler, "DescribeBroker", map[string]any{"BrokerId": id}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeBroker status=%d body=%q", desc.Code, desc.Body.String())
	}
	// Unit test server has no DinD: ActiveMQ must fail closed after start attempt, not invent RUNNING.
	body := desc.Body.String()
	if !strings.Contains(body, `"BrokerState":"CREATION_FAILED"`) && !strings.Contains(body, `"BrokerState": "CREATION_FAILED"`) {
		t.Fatalf("want CREATION_FAILED without DinD; body=%q", body)
	}
	if !strings.Contains(body, "stub://") {
		t.Fatalf("want stub:// endpoint without DinD; body=%q", body)
	}
	if strings.Contains(body, `"BrokerState":"RUNNING"`) || strings.Contains(body, `"BrokerState": "RUNNING"`) {
		t.Fatalf("must not claim RUNNING without nested ActiveMQ; body=%q", body)
	}
	pub := mustMQJSON(t, handler, "CreateBroker", map[string]any{
		"BrokerName":         "amq-public",
		"EngineType":         "ACTIVEMQ",
		"PubliclyAccessible": true,
	}, now)
	if pub.Code != http.StatusBadRequest {
		t.Fatalf("PubliclyAccessible=true status=%d want 400 body=%q", pub.Code, pub.Body.String())
	}
}

func mustTransferJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "TransferService."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "transfer", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestTransferFamilyHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	create := mustTransferJSON(t, handler, "CreateServer", map[string]any{
		"Protocols": []string{"SFTP"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateServer status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	id, _ := out["ServerId"].(string)
	user := mustTransferJSON(t, handler, "CreateUser", map[string]any{
		"ServerId": id,
		"UserName": "bob",
	}, now)
	if user.Code != http.StatusOK {
		t.Fatalf("CreateUser status=%d body=%q", user.Code, user.Body.String())
	}
	list := mustTransferJSON(t, handler, "ListServers", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListServers status=%d body=%q", list.Code, list.Body.String())
	}
	delUser := mustTransferJSON(t, handler, "DeleteUser", map[string]any{
		"ServerId": id,
		"UserName": "bob",
	}, now)
	if delUser.Code != http.StatusOK {
		t.Fatalf("DeleteUser status=%d body=%q", delUser.Code, delUser.Body.String())
	}
	del := mustTransferJSON(t, handler, "DeleteServer", map[string]any{"ServerId": id}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteServer status=%d body=%q", del.Code, del.Body.String())
	}
}
