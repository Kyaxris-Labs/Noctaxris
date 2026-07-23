package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	actionKinesisGetRecords       = "kinesis:GetRecords"
	actionKinesisGetShardIterator = "kinesis:GetShardIterator"
)

// parseKinesisStreamARN parses arn:aws:kinesis:region:account:stream/name.
func parseKinesisStreamARN(arn string) (accountID, streamName string, ok bool) {
	arn = strings.TrimSpace(arn)
	parts := strings.Split(arn, ":")
	if len(parts) < 6 || parts[0] != "arn" || parts[1] != "aws" || parts[2] != "kinesis" {
		return "", "", false
	}
	accountID = strings.TrimSpace(parts[4])
	if accountID == "" {
		return "", "", false
	}
	const marker = "stream/"
	rest := strings.Join(parts[5:], ":")
	if !strings.HasPrefix(rest, marker) {
		return "", "", false
	}
	streamName = strings.TrimSpace(rest[len(marker):])
	if streamName == "" {
		return "", "", false
	}
	return accountID, streamName, true
}

func isKinesisEventSourceARN(arn string) bool {
	_, _, ok := parseKinesisStreamARN(arn)
	return ok
}

func (s *Store) pollKinesisEventSourceMappingOnce(m LambdaEventSourceMapping, invoke ESMInvokeFunc) error {
	acct, streamName, ok := parseKinesisStreamARN(m.EventSourceARN)
	if !ok || acct != m.AccountID {
		return ErrInvalidEventSourceARN
	}
	stream, err := s.DescribeKinesisStream(acct, streamName)
	if err != nil {
		return err
	}
	// SourceCursor is a JSON map shardId → last successfully processed sequence
	// (not a shard iterator). Legacy plain sequence strings mean shard 0 only.
	cursors := parseKinesisESMCursor(m.SourceCursor)
	shardIDs := LabKinesisShardIDs(stream.ShardCount)
	remaining := m.BatchSize
	if remaining <= 0 {
		remaining = 10
	}
	var (
		batch       []KinesisRecord
		nextCursors = cloneKinesisESMCursor(cursors)
	)
	for _, shardID := range shardIDs {
		if remaining <= 0 {
			break
		}
		var iterator string
		if seq := strings.TrimSpace(cursors[shardID]); seq == "" {
			iterator, err = s.GetKinesisShardIterator(acct, streamName, shardID, "TRIM_HORIZON", "")
		} else {
			iterator, err = s.GetKinesisShardIterator(acct, streamName, shardID, "AFTER_SEQUENCE_NUMBER", seq)
		}
		if err != nil {
			return err
		}
		recs, _, err := s.GetKinesisRecords(iterator, remaining)
		if err != nil {
			return err
		}
		if len(recs) == 0 {
			continue
		}
		for i := range recs {
			if recs[i].ShardID == "" {
				recs[i].ShardID = shardID
			}
		}
		batch = append(batch, recs...)
		nextCursors[shardID] = recs[len(recs)-1].SequenceNumber
		remaining -= len(recs)
	}
	if len(batch) == 0 {
		return nil
	}
	cursorJSON, err := marshalKinesisESMCursor(nextCursors)
	if err != nil {
		return err
	}
	fc, _ := parseESMFilterCriteriaJSON(m.FilterCriteriaJSON)
	matched := filterKinesisRecords(fc, batch)
	if len(matched) == 0 {
		// Advance cursor past filtered records (lab: no invoke, no redelivery).
		return s.setESMSourceCursor(m.UUID, cursorJSON)
	}
	eventJSON, err := buildKinesisLambdaEventJSON(m.EventSourceARN, matched)
	if err != nil {
		return err
	}
	respJSON, err := invoke(m.AccountID, m.FunctionName, esmMappingQualifier(m), eventJSON)
	if err != nil {
		return err
	}
	if esmReportsBatchItemFailures(m.FunctionResponseTypesJSON) {
		valid := make(map[string]struct{}, len(matched))
		for _, rec := range matched {
			valid[rec.SequenceNumber] = struct{}{}
		}
		failedIDs, perr := parseBatchItemFailureIDs(respJSON, valid)
		if perr != nil {
			return perr
		}
		// Lab: any reported failure keeps the cursor (retry whole matched set).
		if len(failedIDs) > 0 {
			return nil
		}
	}
	return s.setESMSourceCursor(m.UUID, cursorJSON)
}

func parseKinesisESMCursor(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	out := map[string]string{}
	if raw == "" {
		return out
	}
	if strings.HasPrefix(raw, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil && m != nil {
			for k, v := range m {
				if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
					out[k] = v
				}
			}
			return out
		}
	}
	// Legacy single-shard cursor: plain sequence on shard 0.
	out[LabKinesisShardID(0)] = raw
	return out
}

func cloneKinesisESMCursor(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func marshalKinesisESMCursor(cursors map[string]string) (string, error) {
	if len(cursors) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(cursors)
	if err != nil {
		return "", fmt.Errorf("marshal kinesis esm cursor: %w", err)
	}
	return string(raw), nil
}

func filterKinesisRecords(fc LambdaESMFilterCriteria, records []KinesisRecord) []KinesisRecord {
	if len(fc.Filters) == 0 {
		return records
	}
	out := make([]KinesisRecord, 0, len(records))
	for _, rec := range records {
		if esmRecordMatchesFilters(fc, esmKinesisRecordForFilter(rec)) {
			out = append(out, rec)
		}
	}
	return out
}

// esmKinesisRecordForFilter builds the Kinesis filter evaluation record.
// AWS filters primarily on decoded data (JSON object when valid); lab also exposes
// kinesis.partitionKey for partition-key patterns.
func esmKinesisRecordForFilter(rec KinesisRecord) map[string]any {
	return map[string]any{
		"data": esmKinesisDataForFilter(rec.Data),
		"kinesis": map[string]any{
			"partitionKey": rec.PartitionKey,
		},
	}
}

// esmKinesisDataForFilter nests JSON object payloads under data (AWS content filtering).
// Non-object payloads stay as strings. When Filters require JSON data properties and the
// payload is non-JSON, the record does not match (lab drop, AWS-shaped).
func esmKinesisDataForFilter(data []byte) any {
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		return obj
	}
	return string(data)
}

func buildKinesisLambdaEventJSON(streamARN string, records []KinesisRecord) (string, error) {
	region := DefaultKinesisRegion
	if parts := strings.Split(streamARN, ":"); len(parts) >= 4 {
		region = parts[3]
	}
	out := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		shardID := rec.ShardID
		if shardID == "" {
			shardID = LabKinesisShardID(0)
		}
		out = append(out, map[string]any{
			"eventID":        shardID + ":" + rec.SequenceNumber,
			"eventName":      "aws:kinesis:record",
			"eventVersion":   "1.0",
			"eventSource":    "aws:kinesis",
			"awsRegion":      region,
			"eventSourceARN": streamARN,
			"kinesis": map[string]any{
				"kinesisSchemaVersion":        "1.0",
				"partitionKey":                rec.PartitionKey,
				"sequenceNumber":              rec.SequenceNumber,
				"data":                        base64.StdEncoding.EncodeToString(rec.Data),
				"approximateArrivalTimestamp": float64(rec.ApproximateArrivalTimestamp) / 1000.0,
			},
		})
	}
	raw, err := json.Marshal(map[string]any{"Records": out})
	if err != nil {
		return "", fmt.Errorf("marshal kinesis lambda event: %w", err)
	}
	return string(raw), nil
}
