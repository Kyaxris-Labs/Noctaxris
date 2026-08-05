package dynamodbstreams_test

import (
	"encoding/json"
	"testing"

	dynamodbstreamssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/dynamodbstreams"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDynamoDBStreamsJSON(t *testing.T) {
	tbl := store.DynamoTable{
		TableName: "items", StreamLabel: "2024-01-01T00:00:00.000",
		StreamViewType: store.StreamViewNewAndOldImages,
	}
	region := "us-east-1"

	descRaw, err := dynamodbstreamssvc.DescribeStreamJSON(tbl, region)
	if err != nil {
		t.Fatal(err)
	}
	var desc map[string]any
	if err := json.Unmarshal(descRaw, &desc); err != nil {
		t.Fatal(err)
	}
	sd, _ := desc["StreamDescription"].(map[string]any)
	if sd["TableName"] != "items" {
		t.Fatalf("describe=%v", desc)
	}

	listRaw, err := dynamodbstreamssvc.ListStreamsJSON([]store.DynamoTable{tbl}, region)
	if err != nil {
		t.Fatal(err)
	}
	var list map[string]any
	_ = json.Unmarshal(listRaw, &list)
	streams, _ := list["Streams"].([]any)
	if len(streams) != 1 {
		t.Fatalf("list=%v", list)
	}
	_, _ = dynamodbstreamssvc.ListStreamsJSON(nil, region)

	iterRaw, err := dynamodbstreamssvc.GetShardIteratorJSON("iter-1")
	if err != nil {
		t.Fatal(err)
	}
	var iter map[string]any
	_ = json.Unmarshal(iterRaw, &iter)
	if iter["ShardIterator"] != "iter-1" {
		t.Fatalf("iter=%v", iter)
	}

	rec := store.DynamoStreamRecord{
		SequenceNumber: "1", EventName: "INSERT", ArrivalMS: 1_700_000_000_000,
		KeysJSON: `{"pk":{"S":"a"}}`, NewImageJSON: `{"pk":{"S":"a"}}`,
		OldImageJSON: `{not-json`, StreamViewType: "",
	}
	recRaw, err := dynamodbstreamssvc.GetRecordsJSON([]store.DynamoStreamRecord{rec}, "next")
	if err != nil {
		t.Fatal(err)
	}
	var recOut map[string]any
	_ = json.Unmarshal(recRaw, &recOut)
	rows, _ := recOut["Records"].([]any)
	if len(rows) != 1 || recOut["NextShardIterator"] != "next" {
		t.Fatalf("records=%v", recOut)
	}
	_, _ = dynamodbstreamssvc.GetRecordsJSON(nil, "")
}
