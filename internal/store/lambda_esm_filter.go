package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// LambdaESMLabMaxFilters matches the AWS default FilterCriteria Filters quota.
	LambdaESMLabMaxFilters = 5
)

// ErrInvalidESMFilterCriteria is returned when FilterCriteria JSON is malformed
// or uses unsupported content-filtering operators.
var ErrInvalidESMFilterCriteria = errors.New("InvalidParameterValueException: FilterCriteria")

// LambdaESMFilterCriteria is the AWS FilterCriteria shape (Filters[].Pattern).
type LambdaESMFilterCriteria struct {
	Filters []LambdaESMFilter `json:"Filters"`
}

// LambdaESMFilter is one filter pattern entry.
type LambdaESMFilter struct {
	Pattern string `json:"Pattern"`
}

func parseESMFilterCriteriaJSON(raw string) (LambdaESMFilterCriteria, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return LambdaESMFilterCriteria{}, nil
	}
	var fc LambdaESMFilterCriteria
	if err := json.Unmarshal([]byte(raw), &fc); err != nil {
		return LambdaESMFilterCriteria{}, fmt.Errorf("%w: must be a JSON object with Filters", ErrInvalidESMFilterCriteria)
	}
	if len(fc.Filters) > LambdaESMLabMaxFilters {
		return LambdaESMFilterCriteria{}, fmt.Errorf("%w: at most %d Filters", ErrInvalidESMFilterCriteria, LambdaESMLabMaxFilters)
	}
	for i, f := range fc.Filters {
		if strings.TrimSpace(f.Pattern) == "" {
			return LambdaESMFilterCriteria{}, fmt.Errorf("%w: Filters[%d].Pattern is required", ErrInvalidESMFilterCriteria, i)
		}
		if err := validateESMFilterPattern(f.Pattern); err != nil {
			return LambdaESMFilterCriteria{}, err
		}
	}
	return fc, nil
}

func validateESMFilterPattern(pattern string) error {
	var doc any
	if err := json.Unmarshal([]byte(pattern), &doc); err != nil {
		return fmt.Errorf("%w: pattern must be JSON", ErrInvalidESMFilterCriteria)
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: pattern must be a JSON object", ErrInvalidESMFilterCriteria)
	}
	if err := validatePatternValue(obj, true); err != nil {
		// Reuse EventBridge operator validation; surface as FilterCriteria.
		msg := strings.ReplaceAll(err.Error(), "event pattern", "FilterCriteria pattern")
		return fmt.Errorf("%w: %s", ErrInvalidESMFilterCriteria, msg)
	}
	return nil
}

// esmRecordMatchesFilters returns true when Filters is empty or any filter pattern matches (OR).
// Patterns use the same EventBridge content-filtering operators as AWS Lambda event filtering.
func esmRecordMatchesFilters(fc LambdaESMFilterCriteria, record map[string]any) bool {
	if len(fc.Filters) == 0 {
		return true
	}
	for _, f := range fc.Filters {
		var pattern map[string]any
		if err := json.Unmarshal([]byte(f.Pattern), &pattern); err != nil {
			continue
		}
		if matchPatternObject(pattern, record, true) {
			return true
		}
	}
	return false
}

func filterSQSMessages(fc LambdaESMFilterCriteria, msgs []Message) []Message {
	if len(fc.Filters) == 0 {
		return msgs
	}
	out := make([]Message, 0, len(msgs))
	for _, msg := range msgs {
		rec := esmSQSRecordForFilter(msg)
		if esmRecordMatchesFilters(fc, rec) {
			out = append(out, msg)
		}
	}
	return out
}

// esmSQSRecordForFilter builds the SQS filter evaluation record.
// JSON object bodies are nested under body (AWS content filtering); non-object bodies stay strings.
func esmSQSRecordForFilter(msg Message) map[string]any {
	return map[string]any{
		"messageId": msg.MessageID,
		"body":      esmSQSBodyForFilter(msg.Body),
	}
}

// esmSQSBodyForFilter nests JSON object bodies under body for FilterCriteria
// (AWS content filtering). Non-object payloads stay as a string.
func esmSQSBodyForFilter(body []byte) any {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		return obj
	}
	return string(body)
}

func filterDynamoStreamRecords(fc LambdaESMFilterCriteria, records []DynamoStreamRecord) []DynamoStreamRecord {
	if len(fc.Filters) == 0 {
		return records
	}
	out := make([]DynamoStreamRecord, 0, len(records))
	for _, rec := range records {
		if esmRecordMatchesFilters(fc, esmDynamoRecordForFilter(rec)) {
			out = append(out, rec)
		}
	}
	return out
}

// esmDynamoRecordForFilter builds the DynamoDB Streams filter evaluation record
// (eventName plus dynamodb.Keys / dynamodb.NewImage when present).
func esmDynamoRecordForFilter(rec DynamoStreamRecord) map[string]any {
	m := map[string]any{"eventName": rec.EventName}
	ddb := map[string]any{}
	if strings.TrimSpace(rec.KeysJSON) != "" {
		var keys any
		if err := json.Unmarshal([]byte(rec.KeysJSON), &keys); err == nil {
			ddb["Keys"] = keys
		}
	}
	if strings.TrimSpace(rec.NewImageJSON) != "" {
		var img any
		if err := json.Unmarshal([]byte(rec.NewImageJSON), &img); err == nil {
			ddb["NewImage"] = img
		}
	}
	if len(ddb) > 0 {
		m["dynamodb"] = ddb
	}
	return m
}
