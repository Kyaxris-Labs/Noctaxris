package kinesis_test

import (
	"encoding/json"
	"testing"

	kinesissvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kinesis"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKinesisJSON(t *testing.T) {
	if _, err := kinesissvc.EmptyOKJSON(); err != nil {
		t.Fatal(err)
	}
	st := store.KinesisStream{
		StreamName: "lab-stream", StreamARN: "arn:aws:kinesis:us-east-1:1:stream/lab-stream",
		StreamStatus: "ACTIVE", ShardCount: 2,
	}
	desc, err := kinesissvc.DescribeStreamJSON(st)
	if err != nil {
		t.Fatal(err)
	}
	var descOut map[string]any
	if err := json.Unmarshal(desc, &descOut); err != nil {
		t.Fatal(err)
	}
	if _, err := kinesissvc.ListStreamsJSON([]string{st.StreamName}); err != nil {
		t.Fatal(err)
	}
	if _, err := kinesissvc.PutRecordJSON("seq-1", "shard-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := kinesissvc.PutRecordsJSON([]store.PutKinesisRecordsResultEntry{{SequenceNumber: "1", ShardID: "s"}}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := kinesissvc.GetShardIteratorJSON("iter-1"); err != nil {
		t.Fatal(err)
	}
	rec := store.KinesisRecord{SequenceNumber: "1", PartitionKey: "pk", Data: []byte("data")}
	if _, err := kinesissvc.GetRecordsJSON([]store.KinesisRecord{rec}, "next"); err != nil {
		t.Fatal(err)
	}
	cons := store.KinesisConsumer{ConsumerName: "c1", ConsumerARN: "arn:aws:kinesis:us-east-1:1:stream/lab-stream/consumer/c1:123"}
	if _, err := kinesissvc.RegisterStreamConsumerJSON(cons); err != nil {
		t.Fatal(err)
	}
	if _, err := kinesissvc.DescribeStreamConsumerJSON(cons); err != nil {
		t.Fatal(err)
	}
	if _, err := kinesissvc.ListStreamConsumersJSON([]store.KinesisConsumer{cons}); err != nil {
		t.Fatal(err)
	}
	if _, err := kinesissvc.EmptyConsumerOKJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := kinesissvc.UpdateShardCountJSON(st, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := kinesissvc.SubscribeToShardLabJSON([]store.KinesisRecord{rec}, "cont"); err != nil {
		t.Fatal(err)
	}
}
