package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKinesisPutRecordsConsumersShardCountAndPolicy(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	stream := "cov-kinesis"

	create := mustKinesisJSON(t, handler, "CreateStream", map[string]any{
		"StreamName": stream,
		"ShardCount": 1,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStream status=%d body=%q", create.Code, create.Body.String())
	}

	emptyName := mustKinesisJSON(t, handler, "PutRecords", map[string]any{
		"Records": []map[string]any{{
			"PartitionKey": "pk",
			"Data":         base64.StdEncoding.EncodeToString([]byte("x")),
		}},
	}, now)
	if emptyName.Code != http.StatusBadRequest || !strings.Contains(emptyName.Body.String(), "InvalidArgumentException") {
		t.Fatalf("PutRecords empty StreamName want InvalidArgument status=%d body=%q", emptyName.Code, emptyName.Body.String())
	}

	missing := mustKinesisJSON(t, handler, "PutRecords", map[string]any{
		"StreamName": "no-such-stream",
		"Records": []map[string]any{{
			"PartitionKey": "pk",
			"Data":         base64.StdEncoding.EncodeToString([]byte("x")),
		}},
	}, now)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("PutRecords missing stream want ResourceNotFound status=%d body=%q", missing.Code, missing.Body.String())
	}

	putMany := mustKinesisJSON(t, handler, "PutRecords", map[string]any{
		"StreamName": stream,
		"Records": []map[string]any{
			{
				"PartitionKey": "a",
				"Data":         base64.StdEncoding.EncodeToString([]byte("one")),
			},
			{
				"PartitionKey": "b",
				"Data":         base64.StdEncoding.EncodeToString([]byte("two")),
			},
			nil,
		},
	}, now)
	if putMany.Code != http.StatusOK {
		t.Fatalf("PutRecords status=%d body=%q", putMany.Code, putMany.Body.String())
	}

	streamARN := "arn:aws:kinesis:" + testRegion + ":" + testAccountID + ":stream/" + stream
	reg := mustKinesisJSON(t, handler, "RegisterStreamConsumer", map[string]any{
		"StreamARN":    streamARN,
		"ConsumerName": "efo-1",
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterStreamConsumer status=%d body=%q", reg.Code, reg.Body.String())
	}
	var regOut map[string]any
	if err := json.Unmarshal(reg.Body.Bytes(), &regOut); err != nil {
		t.Fatal(err)
	}
	consumer, _ := regOut["Consumer"].(map[string]any)
	consumerARN, _ := consumer["ConsumerARN"].(string)
	if consumerARN == "" {
		t.Fatalf("missing ConsumerARN in %q", reg.Body.String())
	}

	dup := mustKinesisJSON(t, handler, "RegisterStreamConsumer", map[string]any{
		"StreamName":   stream,
		"ConsumerName": "efo-1",
	}, now)
	if dup.Code != http.StatusBadRequest || !strings.Contains(dup.Body.String(), "ResourceInUseException") {
		t.Fatalf("RegisterStreamConsumer dup want ResourceInUse status=%d body=%q", dup.Code, dup.Body.String())
	}
	badReg := mustKinesisJSON(t, handler, "RegisterStreamConsumer", map[string]any{
		"StreamARN": streamARN,
	}, now)
	if badReg.Code != http.StatusBadRequest {
		t.Fatalf("RegisterStreamConsumer missing name want 400 status=%d body=%q", badReg.Code, badReg.Body.String())
	}

	desc := mustKinesisJSON(t, handler, "DescribeStreamConsumer", map[string]any{
		"StreamARN":    streamARN,
		"ConsumerName": "efo-1",
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "efo-1") {
		t.Fatalf("DescribeStreamConsumer status=%d body=%q", desc.Code, desc.Body.String())
	}
	missingConsumer := mustKinesisJSON(t, handler, "DescribeStreamConsumer", map[string]any{
		"StreamARN":    streamARN,
		"ConsumerName": "nope",
	}, now)
	if missingConsumer.Code != http.StatusBadRequest || !strings.Contains(missingConsumer.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DescribeStreamConsumer missing want ResourceNotFound status=%d body=%q", missingConsumer.Code, missingConsumer.Body.String())
	}

	list := mustKinesisJSON(t, handler, "ListStreamConsumers", map[string]any{
		"StreamARN": streamARN,
	}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "efo-1") {
		t.Fatalf("ListStreamConsumers status=%d body=%q", list.Code, list.Body.String())
	}
	listEmpty := mustKinesisJSON(t, handler, "ListStreamConsumers", map[string]any{}, now)
	if listEmpty.Code != http.StatusBadRequest {
		t.Fatalf("ListStreamConsumers empty want 400 status=%d body=%q", listEmpty.Code, listEmpty.Body.String())
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
	subBad := mustKinesisJSON(t, handler, "SubscribeToShard", map[string]any{
		"ConsumerARN": consumerARN,
	}, now)
	if subBad.Code != http.StatusBadRequest {
		t.Fatalf("SubscribeToShard missing shard want 400 status=%d body=%q", subBad.Code, subBad.Body.String())
	}
	subMissing := mustKinesisJSON(t, handler, "SubscribeToShard", map[string]any{
		"ConsumerARN": "arn:aws:kinesis:" + testRegion + ":" + testAccountID + ":stream/" + stream + "/consumer/missing:123",
		"ShardId":     store.LabKinesisShardID(0),
	}, now)
	if subMissing.Code != http.StatusBadRequest {
		t.Fatalf("SubscribeToShard missing consumer want 400 status=%d body=%q", subMissing.Code, subMissing.Body.String())
	}

	scale := mustKinesisJSON(t, handler, "UpdateShardCount", map[string]any{
		"StreamName":       stream,
		"TargetShardCount": 2,
		"ScalingType":      "UNIFORM_SCALING",
	}, now)
	if scale.Code != http.StatusOK {
		t.Fatalf("UpdateShardCount status=%d body=%q", scale.Code, scale.Body.String())
	}
	badScaleType := mustKinesisJSON(t, handler, "UpdateShardCount", map[string]any{
		"StreamName":       stream,
		"TargetShardCount": 3,
		"ScalingType":      "OTHER",
	}, now)
	if badScaleType.Code != http.StatusBadRequest {
		t.Fatalf("UpdateShardCount bad ScalingType want 400 status=%d body=%q", badScaleType.Code, badScaleType.Body.String())
	}
	scaleEmpty := mustKinesisJSON(t, handler, "UpdateShardCount", map[string]any{
		"TargetShardCount": 2,
	}, now)
	if scaleEmpty.Code != http.StatusBadRequest {
		t.Fatalf("UpdateShardCount empty name want 400 status=%d body=%q", scaleEmpty.Code, scaleEmpty.Body.String())
	}

	policyDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"kinesis:DescribeStream","Resource":"*"}]}`
	putPol := mustKinesisJSON(t, handler, "PutResourcePolicy", map[string]any{
		"ResourceArn": streamARN,
		"Policy":      policyDoc,
	}, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	putPolByName := mustKinesisJSON(t, handler, "PutResourcePolicy", map[string]any{
		"StreamName": stream,
		"Policy":     policyDoc,
	}, now)
	if putPolByName.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy by name status=%d body=%q", putPolByName.Code, putPolByName.Body.String())
	}
	badPol := mustKinesisJSON(t, handler, "PutResourcePolicy", map[string]any{
		"ResourceArn": streamARN,
	}, now)
	if badPol.Code != http.StatusBadRequest {
		t.Fatalf("PutResourcePolicy empty policy want 400 status=%d body=%q", badPol.Code, badPol.Body.String())
	}

	getPol := mustKinesisJSON(t, handler, "GetResourcePolicy", map[string]any{
		"ResourceArn": streamARN,
	}, now)
	if getPol.Code != http.StatusOK || !strings.Contains(getPol.Body.String(), "Version") {
		t.Fatalf("GetResourcePolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}
	getPolEmpty := mustKinesisJSON(t, handler, "GetResourcePolicy", map[string]any{}, now)
	if getPolEmpty.Code != http.StatusBadRequest {
		t.Fatalf("GetResourcePolicy empty want 400 status=%d body=%q", getPolEmpty.Code, getPolEmpty.Body.String())
	}

	delPol := mustKinesisJSON(t, handler, "DeleteResourcePolicy", map[string]any{
		"StreamName": stream,
	}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeleteResourcePolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}
	delPolEmpty := mustKinesisJSON(t, handler, "DeleteResourcePolicy", map[string]any{}, now)
	if delPolEmpty.Code != http.StatusBadRequest {
		t.Fatalf("DeleteResourcePolicy empty want 400 status=%d body=%q", delPolEmpty.Code, delPolEmpty.Body.String())
	}

	dereg := mustKinesisJSON(t, handler, "DeregisterStreamConsumer", map[string]any{
		"ConsumerARN": consumerARN,
	}, now)
	if dereg.Code != http.StatusOK {
		t.Fatalf("DeregisterStreamConsumer status=%d body=%q", dereg.Code, dereg.Body.String())
	}
	deregGone := mustKinesisJSON(t, handler, "DeregisterStreamConsumer", map[string]any{
		"StreamARN":    streamARN,
		"ConsumerName": "efo-1",
	}, now)
	if deregGone.Code != http.StatusBadRequest || !strings.Contains(deregGone.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DeregisterStreamConsumer gone want ResourceNotFound status=%d body=%q", deregGone.Code, deregGone.Body.String())
	}

	unknown := mustKinesisJSON(t, handler, "NotARealKinesisAction", map[string]any{}, now)
	if unknown.Code == http.StatusOK {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(unknown.Body.String(), "NotImplemented") && !strings.Contains(unknown.Body.String(), "__type") {
		t.Fatalf("expected error payload body=%q", unknown.Body.String())
	}

	del := mustKinesisJSON(t, handler, "DeleteStream", map[string]any{"StreamName": stream}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteStream status=%d body=%q", del.Code, del.Body.String())
	}
}
