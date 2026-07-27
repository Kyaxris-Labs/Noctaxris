package kinesis

import (
	"encoding/base64"
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// EmptyOKJSON is the empty success body used by CreateStream / DeleteStream.
func EmptyOKJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// DescribeStreamJSON builds a DescribeStream response with lab shard list (1..4).
func DescribeStreamJSON(st store.KinesisStream) ([]byte, error) {
	shardCount := st.ShardCount
	if shardCount <= 0 {
		shardCount = 1
	}
	shards := make([]map[string]any, 0, shardCount)
	for i, shardID := range store.LabKinesisShardIDs(shardCount) {
		start, end := store.LabKinesisHashKeyRange(i, shardCount)
		shards = append(shards, map[string]any{
			"ShardId": shardID,
			"HashKeyRange": map[string]string{
				"StartingHashKey": start,
				"EndingHashKey":   end,
			},
			"SequenceNumberRange": map[string]string{
				"StartingSequenceNumber": "0",
			},
		})
	}
	return json.Marshal(map[string]any{
		"StreamDescription": map[string]any{
			"StreamName":   st.StreamName,
			"StreamARN":    st.StreamARN,
			"StreamStatus": st.StreamStatus,
			"Shards":       shards,
			"RetentionPeriodHours": 24,
			"EnhancedMonitoring":   []any{},
			"EncryptionType":       "NONE",
			"KeyId":                nil,
			"StreamCreationTimestamp": float64(st.CreatedAt) / 1000.0,
		},
	})
}

// ListStreamsJSON builds a ListStreams response.
func ListStreamsJSON(names []string) ([]byte, error) {
	if names == nil {
		names = []string{}
	}
	return json.Marshal(map[string]any{
		"StreamNames": names,
		"HasMoreStreams": false,
	})
}

// PutRecordJSON builds a PutRecord response.
func PutRecordJSON(seq, shardID string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"SequenceNumber": seq,
		"ShardId":        shardID,
	})
}

// PutRecordsJSON builds a PutRecords response.
func PutRecordsJSON(entries []store.PutKinesisRecordsResultEntry, failed int) ([]byte, error) {
	recs := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		m := map[string]any{}
		if e.ErrorCode != "" {
			m["ErrorCode"] = e.ErrorCode
			m["ErrorMessage"] = e.ErrorMessage
		} else {
			m["SequenceNumber"] = e.SequenceNumber
			m["ShardId"] = e.ShardID
		}
		recs = append(recs, m)
	}
	return json.Marshal(map[string]any{
		"FailedRecordCount": failed,
		"Records":           recs,
	})
}

// GetShardIteratorJSON builds a GetShardIterator response.
func GetShardIteratorJSON(iterator string) ([]byte, error) {
	return json.Marshal(map[string]any{"ShardIterator": iterator})
}

// GetRecordsJSON builds a GetRecords response.
func GetRecordsJSON(records []store.KinesisRecord, nextIterator string) ([]byte, error) {
	recs := make([]map[string]any, 0, len(records))
	for _, r := range records {
		recs = append(recs, map[string]any{
			"SequenceNumber":              r.SequenceNumber,
			"ApproximateArrivalTimestamp": float64(r.ApproximateArrivalTimestamp) / 1000.0,
			"Data":                        base64.StdEncoding.EncodeToString(r.Data),
			"PartitionKey":                r.PartitionKey,
		})
	}
	return json.Marshal(map[string]any{
		"Records":            recs,
		"NextShardIterator":  nextIterator,
		"MillisBehindLatest": 0,
	})
}

func consumerJSON(c store.KinesisConsumer) map[string]any {
	return map[string]any{
		"ConsumerName":              c.ConsumerName,
		"ConsumerARN":               c.ConsumerARN,
		"ConsumerStatus":            c.ConsumerStatus,
		"ConsumerCreationTimestamp": float64(c.ConsumerCreationTimestamp) / 1000.0,
	}
}

// RegisterStreamConsumerJSON builds a RegisterStreamConsumer response.
func RegisterStreamConsumerJSON(c store.KinesisConsumer) ([]byte, error) {
	return json.Marshal(map[string]any{"Consumer": consumerJSON(c)})
}

// DescribeStreamConsumerJSON builds a DescribeStreamConsumer response.
func DescribeStreamConsumerJSON(c store.KinesisConsumer) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ConsumerDescription": map[string]any{
			"ConsumerName":              c.ConsumerName,
			"ConsumerARN":               c.ConsumerARN,
			"ConsumerStatus":            c.ConsumerStatus,
			"ConsumerCreationTimestamp": float64(c.ConsumerCreationTimestamp) / 1000.0,
			"StreamARN":                 c.StreamARN,
		},
	})
}

// ListStreamConsumersJSON builds a ListStreamConsumers response.
func ListStreamConsumersJSON(consumers []store.KinesisConsumer) ([]byte, error) {
	list := make([]map[string]any, 0, len(consumers))
	for _, c := range consumers {
		list = append(list, consumerJSON(c))
	}
	return json.Marshal(map[string]any{"Consumers": list})
}

// EmptyConsumerOKJSON is used by DeregisterStreamConsumer.
func EmptyConsumerOKJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// UpdateShardCountJSON builds an UpdateShardCount response.
func UpdateShardCountJSON(st store.KinesisStream, currentCount int) ([]byte, error) {
	return json.Marshal(map[string]any{
		"StreamName":           st.StreamName,
		"CurrentShardCount":    currentCount,
		"TargetShardCount":     st.ShardCount,
		"StreamARN":            st.StreamARN,
	})
}

// SubscribeToShardLabJSON builds a lab long-poll SubscribeToShard body (not HTTP/2 event stream).
func SubscribeToShardLabJSON(records []store.KinesisRecord, continuation string) ([]byte, error) {
	recs := make([]map[string]any, 0, len(records))
	for _, r := range records {
		recs = append(recs, map[string]any{
			"SequenceNumber":              r.SequenceNumber,
			"ApproximateArrivalTimestamp": float64(r.ApproximateArrivalTimestamp) / 1000.0,
			"Data":                        base64.StdEncoding.EncodeToString(r.Data),
			"PartitionKey":                r.PartitionKey,
		})
	}
	return json.Marshal(map[string]any{
		"SubscribeToShardEvent": map[string]any{
			"Records":                     recs,
			"ContinuationSequenceNumber":  continuation,
			"MillisBehindLatest":          0,
		},
	})
}
