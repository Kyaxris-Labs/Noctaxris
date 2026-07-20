package sfn

import (
	"encoding/json"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateStateMachineJSON builds a CreateStateMachine response.
func CreateStateMachineJSON(sm store.SFNStateMachine) ([]byte, error) {
	return json.Marshal(map[string]any{
		"stateMachineArn": sm.StateMachineARN,
		"creationDate":    float64(sm.CreationDate) / 1000.0,
	})
}

// DescribeStateMachineJSON builds a DescribeStateMachine response.
func DescribeStateMachineJSON(sm store.SFNStateMachine) ([]byte, error) {
	return json.Marshal(map[string]any{
		"stateMachineArn": sm.StateMachineARN,
		"name":            sm.Name,
		"definition":      sm.Definition,
		"roleArn":         sm.RoleARN,
		"type":            "STANDARD",
		"creationDate":    float64(sm.CreationDate) / 1000.0,
		"status":          "ACTIVE",
	})
}

// ListStateMachinesJSON builds a ListStateMachines response.
func ListStateMachinesJSON(machines []store.SFNStateMachine) ([]byte, error) {
	items := make([]map[string]any, 0, len(machines))
	for _, sm := range machines {
		items = append(items, map[string]any{
			"stateMachineArn": sm.StateMachineARN,
			"name":            sm.Name,
			"type":            "STANDARD",
			"creationDate":    float64(sm.CreationDate) / 1000.0,
		})
	}
	return json.Marshal(map[string]any{"stateMachines": items})
}

// DeleteStateMachineJSON is an empty OK body.
func DeleteStateMachineJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// StartExecutionJSON builds a StartExecution response.
func StartExecutionJSON(e store.SFNExecution) ([]byte, error) {
	return json.Marshal(map[string]any{
		"executionArn": e.ExecutionARN,
		"startDate":    float64(e.StartDate) / 1000.0,
	})
}

// DescribeExecutionJSON builds a DescribeExecution response.
func DescribeExecutionJSON(e store.SFNExecution) ([]byte, error) {
	m := map[string]any{
		"executionArn":    e.ExecutionARN,
		"stateMachineArn": e.StateMachineARN,
		"name":            e.Name,
		"status":          e.Status,
		"startDate":       float64(e.StartDate) / 1000.0,
		"input":           e.Input,
	}
	if e.StopDate > 0 {
		m["stopDate"] = float64(e.StopDate) / 1000.0
	}
	if e.Output != "" {
		m["output"] = e.Output
	}
	if e.Error != "" {
		m["error"] = e.Error
		m["cause"] = e.Cause
	}
	return json.Marshal(m)
}

// GetExecutionHistoryJSON builds a GetExecutionHistory response.
func GetExecutionHistoryJSON(events []store.SFNHistoryEvent) ([]byte, error) {
	items := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		item := map[string]any{
			"id":        ev.ID,
			"type":      ev.Type,
			"timestamp": float64(ev.Timestamp) / 1000.0,
		}
		var details map[string]any
		if err := json.Unmarshal([]byte(ev.Details), &details); err == nil {
			switch {
			case strings.Contains(ev.Type, "ExecutionStarted"):
				item["executionStartedEventDetails"] = details
			case strings.Contains(ev.Type, "ExecutionSucceeded"):
				item["executionSucceededEventDetails"] = details
			case strings.Contains(ev.Type, "ExecutionFailed"):
				item["executionFailedEventDetails"] = details
			default:
				item["stateEnteredEventDetails"] = details
			}
		}
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"events": items})
}
