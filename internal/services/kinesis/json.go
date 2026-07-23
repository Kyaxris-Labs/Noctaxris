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
