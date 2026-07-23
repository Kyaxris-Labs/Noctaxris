package dynamodbstreams

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// DescribeStreamJSON builds a DescribeStream success body.
func DescribeStreamJSON(t store.DynamoTable, region string) ([]byte, error) {
	arn := t.StreamARN(region)
	return json.Marshal(map[string]any{
		"StreamDescription": map[string]any{
			"StreamArn":       arn,
			"StreamLabel":     t.StreamLabel,
			"StreamStatus":    store.StreamStatusEnabled,
			"StreamViewType":  t.StreamViewType,
			"TableName":       t.TableName,
			"CreationRequestDateTime": 0,
			"Shards": []map[string]any{
				{
					"ShardId": store.LabDynamoStreamShardID,
					"SequenceNumberRange": map[string]string{
						"StartingSequenceNumber": "0",
					},
				},
			},
		},
	})
}

// ListStreamsJSON builds a ListStreams success body.
func ListStreamsJSON(tables []store.DynamoTable, region string) ([]byte, error) {
	streams := make([]map[string]any, 0, len(tables))
	for _, t := range tables {
		streams = append(streams, map[string]any{
			"StreamArn":   t.StreamARN(region),
			"TableName":   t.TableName,
			"StreamLabel": t.StreamLabel,
		})
	}
	return json.Marshal(map[string]any{
		"Streams": streams,
		"LastEvaluatedStreamArn": nil,
	})
}

// GetShardIteratorJSON builds a GetShardIterator success body.
func GetShardIteratorJSON(iterator string) ([]byte, error) {
	return json.Marshal(map[string]any{"ShardIterator": iterator})
}

// GetRecordsJSON builds a GetRecords success body.
func GetRecordsJSON(records []store.DynamoStreamRecord, nextIterator string) ([]byte, error) {
	out := make([]map[string]any, 0, len(records))
	for _, r := range records {
		var keys any
		_ = json.Unmarshal([]byte(r.KeysJSON), &keys)
		view := r.StreamViewType
		if view == "" {
			view = store.StreamViewNewImage
		}
		dynamodb := map[string]any{
			"Keys":                        keys,
			"ApproximateCreationDateTime": float64(r.ArrivalMS) / 1000.0,
			"SequenceNumber":              r.SequenceNumber,
			"SizeBytes":                   len(r.KeysJSON) + len(r.NewImageJSON) + len(r.OldImageJSON),
			"StreamViewType":              view,
		}
		if r.NewImageJSON != "" {
			var img any
			_ = json.Unmarshal([]byte(r.NewImageJSON), &img)
			dynamodb["NewImage"] = img
		}
		if r.OldImageJSON != "" {
			var img any
			_ = json.Unmarshal([]byte(r.OldImageJSON), &img)
			dynamodb["OldImage"] = img
		}
		out = append(out, map[string]any{
			"eventID":      r.SequenceNumber,
			"eventName":    r.EventName,
			"eventVersion": "1.1",
			"eventSource":  "aws:dynamodb",
			"awsRegion":    store.DefaultDynamoRegion,
			"dynamodb":     dynamodb,
		})
	}
	return json.Marshal(map[string]any{
		"Records":           out,
		"NextShardIterator": nextIterator,
	})
}
