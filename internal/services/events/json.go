package events

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const eventsJSONContentType = "application/x-amz-json-1.1"

// JSONContentType is the EventBridge JSON protocol content type.
func JSONContentType() string {
	return eventsJSONContentType
}

// PutEventsJSON builds a PutEvents response.
func PutEventsJSON(result store.PutEventsResult) ([]byte, error) {
	entries := make([]map[string]any, 0, len(result.Entries))
	for _, e := range result.Entries {
		entry := map[string]any{}
		if e.EventID != "" {
			entry["EventId"] = e.EventID
		}
		if e.ErrorCode != "" {
			entry["ErrorCode"] = e.ErrorCode
			entry["ErrorMessage"] = e.ErrorMessage
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]any{
		"FailedEntryCount": result.FailedEntryCount,
		"Entries":          entries,
	})
}

// PutRuleJSON builds a PutRule response.
func PutRuleJSON(rule store.EventRule) ([]byte, error) {
	return json.Marshal(map[string]any{"RuleArn": rule.ARN})
}

// DescribeRuleJSON builds a DescribeRule response.
func DescribeRuleJSON(rule store.EventRule) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Name":          rule.Name,
		"Arn":           rule.ARN,
		"EventPattern":  rule.Pattern,
		"State":         rule.State,
		"Description":   rule.Description,
		"EventBusName":  rule.BusName,
		"ManagedBy":     "",
		"CreatedBy":     "",
	})
}

// ListRulesJSON builds a ListRules response.
func ListRulesJSON(rules []store.EventRule) ([]byte, error) {
	entries := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		entries = append(entries, map[string]any{
			"Name":         rule.Name,
			"Arn":          rule.ARN,
			"EventPattern": rule.Pattern,
			"State":        rule.State,
			"Description":  rule.Description,
			"EventBusName": rule.BusName,
		})
	}
	return json.Marshal(map[string]any{"Rules": entries})
}

// CreateEventBusJSON builds a CreateEventBus response.
func CreateEventBusJSON(bus store.EventBus) ([]byte, error) {
	return json.Marshal(map[string]any{"EventBusArn": bus.ARN})
}

// DescribeEventBusJSON builds a DescribeEventBus response.
func DescribeEventBusJSON(bus store.EventBus) ([]byte, error) {
	ts, err := creationDateUnix(bus.CreationDate)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"Name":         bus.Name,
		"Arn":          bus.ARN,
		"Policy":       bus.Policy,
		"CreationTime": ts,
	})
}

// ListEventBusesJSON builds a ListEventBuses response.
func ListEventBusesJSON(buses []store.EventBus) ([]byte, error) {
	entries := make([]map[string]any, 0, len(buses))
	for _, bus := range buses {
		ts, err := creationDateUnix(bus.CreationDate)
		if err != nil {
			return nil, err
		}
		entries = append(entries, map[string]any{
			"Name":         bus.Name,
			"Arn":          bus.ARN,
			"Policy":       bus.Policy,
			"CreationTime": ts,
		})
	}
	return json.Marshal(map[string]any{"EventBuses": entries})
}

// PutTargetsJSON builds a PutTargets response.
func PutTargetsJSON(failed []map[string]any) ([]byte, error) {
	if failed == nil {
		failed = []map[string]any{}
	}
	return json.Marshal(map[string]any{
		"FailedEntryCount": len(failed),
		"FailedEntries":    failed,
	})
}

// ListTargetsByRuleJSON builds a ListTargetsByRule response.
func ListTargetsByRuleJSON(targets []store.EventTarget) ([]byte, error) {
	entries := make([]map[string]any, 0, len(targets))
	for _, tgt := range targets {
		entry := map[string]any{
			"Id":  tgt.ID,
			"Arn": tgt.ARN,
		}
		if tgt.RoleARN != "" {
			entry["RoleArn"] = tgt.RoleARN
		}
		if tgt.Input != "" {
			entry["Input"] = tgt.Input
		}
		if tgt.InputPath != "" {
			entry["InputPath"] = tgt.InputPath
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]any{"Targets": entries})
}

// EmptyOKJSON is an empty success body for delete/enable/disable APIs.
func EmptyOKJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

func creationDateUnix(created string) (float64, error) {
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return 0, err
	}
	return float64(t.Unix()), nil
}
