package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// LambdaESMFilterCriteria is the lab FilterCriteria shape.
type LambdaESMFilterCriteria struct {
	Filters []LambdaESMFilter `json:"Filters"`
}

// LambdaESMFilter is one filter pattern entry.
type LambdaESMFilter struct {
	Pattern string `json:"Pattern"`
}

func parseESMFilterCriteriaJSON(raw string) (LambdaESMFilterCriteria, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return LambdaESMFilterCriteria{}, nil
	}
	var fc LambdaESMFilterCriteria
	if err := json.Unmarshal([]byte(raw), &fc); err != nil {
		return LambdaESMFilterCriteria{}, fmt.Errorf("FilterCriteria: %w", err)
	}
	for _, f := range fc.Filters {
		if strings.TrimSpace(f.Pattern) == "" {
			return LambdaESMFilterCriteria{}, fmt.Errorf("FilterCriteria.Filters[].Pattern is required")
		}
		if !json.Valid([]byte(f.Pattern)) {
			return LambdaESMFilterCriteria{}, fmt.Errorf("FilterCriteria pattern must be JSON")
		}
	}
	return fc, nil
}

// esmRecordMatchesFilters returns true when Filters is empty or any filter pattern matches.
// Lab pattern language: same subset as EventBridge detail match on top-level keys
// (string equality / string list). SQS body is matched as a string field "body".
func esmRecordMatchesFilters(fc LambdaESMFilterCriteria, record map[string]any) bool {
	if len(fc.Filters) == 0 {
		return true
	}
	for _, f := range fc.Filters {
		var pattern map[string]any
		if err := json.Unmarshal([]byte(f.Pattern), &pattern); err != nil {
			continue
		}
		if esmPatternMatches(pattern, record) {
			return true
		}
	}
	return false
}

func esmPatternMatches(pattern, record map[string]any) bool {
	for key, expected := range pattern {
		actual, ok := record[key]
		if !ok {
			return false
		}
		if !patternValueMatches(expected, actual) {
			return false
		}
	}
	return true
}

func filterSQSMessages(fc LambdaESMFilterCriteria, msgs []Message) []Message {
	if len(fc.Filters) == 0 {
		return msgs
	}
	out := make([]Message, 0, len(msgs))
	for _, msg := range msgs {
		rec := map[string]any{"body": string(msg.Body)}
		if esmRecordMatchesFilters(fc, rec) {
			out = append(out, msg)
		}
	}
	return out
}

func filterDynamoStreamRecords(fc LambdaESMFilterCriteria, records []DynamoStreamRecord) []DynamoStreamRecord {
	if len(fc.Filters) == 0 {
		return records
	}
	out := make([]DynamoStreamRecord, 0, len(records))
	for _, rec := range records {
		m := map[string]any{"eventName": rec.EventName}
		if esmRecordMatchesFilters(fc, m) {
			out = append(out, rec)
		}
	}
	return out
}
