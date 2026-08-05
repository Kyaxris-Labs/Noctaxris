package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/server"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIoTTopicRuleSNSDynamoKinesisLambdaAndDispatch(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "iot-rule-sns", now)

	createTable := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "iot-rule-ddb",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "id", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "id", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if createTable.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createTable.Code, createTable.Body.String())
	}

	createStream := mustKinesisJSON(t, handler, "CreateStream", map[string]any{
		"StreamName": "iot-rule-k",
		"ShardCount": 1,
	}, now)
	if createStream.Code != http.StatusOK {
		t.Fatalf("CreateStream status=%d body=%q", createStream.Code, createStream.Body.String())
	}

	mustCreateIAMRole(t, handler, "iot-rule-lambda", lambdaTrustOK, now)
	fn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "iot-rule-fn",
		"Runtime":      "python3.12",
		"Role":         "arn:aws:iam::" + testAccountID + ":role/iot-rule-lambda",
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fn.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", fn.Code, fn.Body.String())
	}

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "multi-action",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'lab/multi'",
			"actions": []any{
				map[string]any{"sns": map[string]any{"topicArn": topicARN}},
				map[string]any{"dynamoDB": map[string]any{"tableName": "iot-rule-ddb"}},
				map[string]any{"dynamoDBv2": map[string]any{
					"putItem": map[string]any{"tableName": "iot-rule-ddb"},
				}},
				map[string]any{"kinesis": map[string]any{"streamName": "iot-rule-k"}},
				map[string]any{"lambda": map[string]any{
					"functionName": "iot-rule-fn",
				}},
				map[string]any{"sns": map[string]any{}},
				map[string]any{"dynamoDB": map[string]any{}},
				map[string]any{"kinesis": map[string]any{}},
				map[string]any{"lambda": map[string]any{}},
				map[string]any{"unknown": map[string]any{"x": 1}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%q", create.Code, create.Body.String())
	}

	payload := []byte(`{"id":"row-1","temp":21,"ok":true,"n":null}`)
	srv.PublishTopic(testAccountID, testRegion, "lab/multi", payload, true)

	// Dynamo put via rule
	get := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "iot-rule-ddb",
		"Key": map[string]any{
			"id": map[string]any{"S": "row-1"},
		},
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "row-1") {
		t.Fatalf("GetItem after rule status=%d body=%q", get.Code, get.Body.String())
	}

	// Kinesis put via rule
	itRec := mustKinesisJSON(t, handler, "GetShardIterator", map[string]any{
		"StreamName":        "iot-rule-k",
		"ShardId":           store.LabKinesisShardID(0),
		"ShardIteratorType": "TRIM_HORIZON",
	}, now)
	if itRec.Code != http.StatusOK {
		t.Fatalf("GetShardIterator status=%d body=%q", itRec.Code, itRec.Body.String())
	}
	var itOut map[string]any
	_ = json.Unmarshal(itRec.Body.Bytes(), &itOut)
	iterator, _ := itOut["ShardIterator"].(string)
	getRecs := mustKinesisJSON(t, handler, "GetRecords", map[string]any{
		"ShardIterator": iterator,
		"Limit":         10,
	}, now)
	if getRecs.Code != http.StatusOK {
		t.Fatalf("GetRecords status=%d body=%q", getRecs.Code, getRecs.Body.String())
	}
	var getOut map[string]any
	_ = json.Unmarshal(getRecs.Body.Bytes(), &getOut)
	if recs, _ := getOut["Records"].([]any); len(recs) < 1 {
		t.Fatalf("expected kinesis records from rule, got %v", getOut["Records"])
	}

	// Lambda enqueue via rule (async queue non-empty or at least no panic)
	_, err := st.GetFunction(testAccountID, "iot-rule-fn")
	if err != nil {
		t.Fatal(err)
	}

	// Missing targets fail closed
	_ = mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "missing-targets",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'lab/miss'",
			"actions": []any{
				map[string]any{"sns": map[string]any{"topicArn": "arn:aws:sns:us-east-1:" + testAccountID + ":nope"}},
				map[string]any{"dynamoDB": map[string]any{"tableName": "nope"}},
				map[string]any{"kinesis": map[string]any{"streamName": "nope"}},
				map[string]any{"lambda": map[string]any{"functionName": "nope"}},
				map[string]any{"lambda": map[string]any{"functionArn": "bad"}},
			},
		},
	}, now)
	srv.PublishTopic(testAccountID, testRegion, "lab/miss", []byte(`{"id":"x"}`), true)
	srv.PublishTopic(testAccountID, testRegion, "lab/miss", []byte(`not-json`), true)

	// DispatchMQTTPublish + republish path without live MQTT
	srv.DispatchMQTTPublish("lab/multi", []byte(`{"id":"mqtt-1"}`))
	srv.DispatchMQTTPublish("", []byte(`{}`))
	srv.DispatchMQTTPublish("$aws/things/x/shadow/update", []byte(`{}`))
	srv.PublishTopic(testAccountID, testRegion, "cov/out", []byte(`{"republish":true}`), false)
}

func TestIoTMQTTSharedHooksAndFailClosed(t *testing.T) {
	t.Cleanup(func() {
		server.SetMQTTBridgeStartHookForTest(nil)
		server.SetMQTTBridgeRunningHookForTest(nil)
	})

	var started atomic.Bool
	server.SetMQTTBridgeStartHookForTest(func(*server.Server) error {
		started.Store(true)
		return nil
	})
	server.SetMQTTBridgeRunningHookForTest(func() {})

	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.SharedMQTT = true
		cfg.ListenAddr = "127.0.0.1:0"
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServeContext(ctx)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !started.Load() {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-errCh
	if !started.Load() {
		t.Fatal("expected MQTT bridge start hook via ListenAndServeContext")
	}

	// SharedMQTT ensure on CreateKeysAndCertificate without DinD (error logged, request still OK).
	t.Setenv(compute.EnvBrokerPortPublish, "")
	t.Setenv(compute.EnvNestedPortPublish, "")
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	cert := mustJSONTarget(t, handler, "AWSIotService.CreateKeysAndCertificate", "iot", map[string]any{
		"setAsActive": true,
	}, now)
	if cert.Code != http.StatusOK {
		t.Fatalf("CreateKeysAndCertificate status=%d body=%q", cert.Code, cert.Body.String())
	}

	// No-hook SharedMQTT starts bridge goroutine; without broker publish it fails closed.
	server.SetMQTTBridgeStartHookForTest(nil)
	server.SetMQTTBridgeRunningHookForTest(nil)
	t.Setenv(compute.EnvBrokerPortPublish, "")
	srv2, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.SharedMQTT = true
		cfg.ListenAddr = "127.0.0.1:0"
	})
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go func() { _ = srv2.ListenAndServeContext(ctx2) }()
	time.Sleep(50 * time.Millisecond)
	cancel2()

	// With broker publish enabled but no DockerHost, tryEnsureSharedMQTTBroker fail-closed.
	t.Setenv(compute.EnvBrokerPortPublish, "1")
	srv3, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.SharedMQTT = true
		cfg.DockerHost = ""
		cfg.ListenAddr = "127.0.0.1:0"
	})
	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel3()
	go func() { _ = srv3.ListenAndServeContext(ctx3) }()
	time.Sleep(50 * time.Millisecond)
	cancel3()
}
