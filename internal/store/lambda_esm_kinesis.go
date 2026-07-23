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
	if _, err := s.DescribeKinesisStream(acct, streamName); err != nil {
		return err
	}
	// SourceCursor stores the last successfully processed sequence number (not a
	// shard iterator). GetRecords consumes iterators, so sequence cursors keep
	// invoke-failure redelivery reliable.
	var (
		iterator string
		err      error
	)
	if strings.TrimSpace(m.SourceCursor) == "" {
		iterator, err = s.GetKinesisShardIterator(acct, streamName, LabKinesisShardID, "TRIM_HORIZON", "")
	} else {
		iterator, err = s.GetKinesisShardIterator(acct, streamName, LabKinesisShardID, "AFTER_SEQUENCE_NUMBER", m.SourceCursor)
	}
	if err != nil {
		return err
	}
	records, _, err := s.GetKinesisRecords(iterator, m.BatchSize)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	batchEndSeq := records[len(records)-1].SequenceNumber
	fc, _ := parseESMFilterCriteriaJSON(m.FilterCriteriaJSON)
	matched := filterKinesisRecords(fc, records)
	if len(matched) == 0 {
		// Advance cursor past filtered records (lab: no invoke, no redelivery).
		return s.setESMSourceCursor(m.UUID, batchEndSeq)
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
	return s.setESMSourceCursor(m.UUID, batchEndSeq)
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
		out = append(out, map[string]any{
			"eventID":        LabKinesisShardID + ":" + rec.SequenceNumber,
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
