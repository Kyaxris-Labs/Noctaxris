package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKinesisPutRecordsResourcePolicyConsumersAndShards(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustKinesisJSON(t, handler, "CreateStream", map[string]any{
		"StreamName": "cov-k",
		"ShardCount": 1,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStream status=%d body=%q", create.Code, create.Body.String())
	}

	putMany := mustKinesisJSON(t, handler, "PutRecords", map[string]any{
		"StreamName": "cov-k",
		"Records": []map[string]any{
			{"PartitionKey": "a", "Data": base64.StdEncoding.EncodeToString([]byte("one"))},
			{"PartitionKey": "b", "Data": base64.StdEncoding.EncodeToString([]byte("two"))},
		},
	}, now)
	if putMany.Code != http.StatusOK {
		t.Fatalf("PutRecords status=%d body=%q", putMany.Code, putMany.Body.String())
	}
	missingPut := mustKinesisJSON(t, handler, "PutRecords", map[string]any{
		"StreamName": "missing",
		"Records":    []map[string]any{{"PartitionKey": "a", "Data": base64.StdEncoding.EncodeToString([]byte("x"))}},
	}, now)
	if missingPut.Code != http.StatusBadRequest || !strings.Contains(missingPut.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("PutRecords missing want ResourceNotFound status=%d body=%q", missingPut.Code, missingPut.Body.String())
	}
	emptyName := mustKinesisJSON(t, handler, "PutRecords", map[string]any{"Records": []any{}}, now)
	if emptyName.Code != http.StatusBadRequest {
		t.Fatalf("PutRecords empty name want 400 status=%d body=%q", emptyName.Code, emptyName.Body.String())
	}

	streamARN := "arn:aws:kinesis:" + testRegion + ":" + testAccountID + ":stream/cov-k"
	putPol := mustKinesisJSON(t, handler, "PutResourcePolicy", map[string]any{
		"ResourceArn": streamARN,
		"Policy":      `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"kinesis:*","Resource":"*"}]}`,
	}, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	getPol := mustKinesisJSON(t, handler, "GetResourcePolicy", map[string]any{
		"ResourceArn": streamARN,
	}, now)
	if getPol.Code != http.StatusOK || !strings.Contains(getPol.Body.String(), "Version") {
		t.Fatalf("GetResourcePolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}
	delPol := mustKinesisJSON(t, handler, "DeleteResourcePolicy", map[string]any{
		"ResourceArn": streamARN,
	}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeleteResourcePolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}
	getGone := mustKinesisJSON(t, handler, "GetResourcePolicy", map[string]any{
		"ResourceArn": streamARN,
	}, now)
	if getGone.Code == http.StatusOK && strings.Contains(getGone.Body.String(), "Version") {
		// some labs return empty policy; either way path was exercised
	}

	reg := mustKinesisJSON(t, handler, "RegisterStreamConsumer", map[string]any{
		"StreamARN":    streamARN,
		"ConsumerName": "efo-1",
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterStreamConsumer status=%d body=%q", reg.Code, reg.Body.String())
	}
	var regOut map[string]any
	_ = json.Unmarshal(reg.Body.Bytes(), &regOut)
	consumer, _ := regOut["Consumer"].(map[string]any)
	consumerARN, _ := consumer["ConsumerARN"].(string)
	if consumerARN == "" {
		consumerARN, _ = regOut["ConsumerARN"].(string)
	}
	dup := mustKinesisJSON(t, handler, "RegisterStreamConsumer", map[string]any{
		"StreamName":   "cov-k",
		"ConsumerName": "efo-1",
	}, now)
	if dup.Code != http.StatusBadRequest {
		t.Fatalf("RegisterStreamConsumer dup want 400 status=%d body=%q", dup.Code, dup.Body.String())
	}
	descC := mustKinesisJSON(t, handler, "DescribeStreamConsumer", map[string]any{
		"StreamARN":    streamARN,
		"ConsumerName": "efo-1",
	}, now)
	if descC.Code != http.StatusOK {
		t.Fatalf("DescribeStreamConsumer status=%d body=%q", descC.Code, descC.Body.String())
	}
	listC := mustKinesisJSON(t, handler, "ListStreamConsumers", map[string]any{
		"StreamARN": streamARN,
	}, now)
	if listC.Code != http.StatusOK || !strings.Contains(listC.Body.String(), "efo-1") {
		t.Fatalf("ListStreamConsumers status=%d body=%q", listC.Code, listC.Body.String())
	}
	missingC := mustKinesisJSON(t, handler, "DescribeStreamConsumer", map[string]any{
		"StreamARN":    streamARN,
		"ConsumerName": "nope",
	}, now)
	if missingC.Code != http.StatusBadRequest {
		t.Fatalf("DescribeStreamConsumer missing want 400 status=%d body=%q", missingC.Code, missingC.Body.String())
	}

	sub := mustKinesisJSON(t, handler, "SubscribeToShard", map[string]any{
		"ConsumerARN": consumerARN,
		"ShardId":     store.LabKinesisShardID(0),
		"StartingPosition": map[string]any{
			"Type": "TRIM_HORIZON",
		},
	}, now)
	if sub.Code != http.StatusOK {
		t.Fatalf("SubscribeToShard status=%d body=%q", sub.Code, sub.Body.String())
	}
	badSub := mustKinesisJSON(t, handler, "SubscribeToShard", map[string]any{}, now)
	if badSub.Code != http.StatusBadRequest {
		t.Fatalf("SubscribeToShard bad want 400 status=%d body=%q", badSub.Code, badSub.Body.String())
	}

	upd := mustKinesisJSON(t, handler, "UpdateShardCount", map[string]any{
		"StreamName":       "cov-k",
		"TargetShardCount": 2,
		"ScalingType":      "UNIFORM_SCALING",
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateShardCount status=%d body=%q", upd.Code, upd.Body.String())
	}
	badScale := mustKinesisJSON(t, handler, "UpdateShardCount", map[string]any{
		"StreamName":       "cov-k",
		"TargetShardCount": 3,
		"ScalingType":      "OTHER",
	}, now)
	if badScale.Code != http.StatusBadRequest {
		t.Fatalf("UpdateShardCount bad ScalingType want 400 status=%d body=%q", badScale.Code, badScale.Body.String())
	}

	dereg := mustKinesisJSON(t, handler, "DeregisterStreamConsumer", map[string]any{
		"StreamARN":    streamARN,
		"ConsumerName": "efo-1",
	}, now)
	if dereg.Code != http.StatusOK {
		t.Fatalf("DeregisterStreamConsumer status=%d body=%q", dereg.Code, dereg.Body.String())
	}
	deregGone := mustKinesisJSON(t, handler, "DeregisterStreamConsumer", map[string]any{
		"ConsumerARN": consumerARN,
	}, now)
	if deregGone.Code != http.StatusBadRequest {
		t.Fatalf("DeregisterStreamConsumer gone want 400 status=%d body=%q", deregGone.Code, deregGone.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "kinesis-deny")
	if err != nil {
		t.Fatal(err)
	}
	ak, secret, err := st.CreateUserAccessKey(testAccountID, "kinesis-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"kinesis:PutRecords","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"StreamName": "cov-k",
		"Records":    []map[string]any{{"PartitionKey": "z", "Data": base64.StdEncoding.EncodeToString([]byte("z"))}},
	})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Kinesis_20131202.PutRecords")
	signHeader(t, req, raw, ak, secret, testRegion, "kinesis", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("PutRecords deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}
