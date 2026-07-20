package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// applyEventBridgeInputPath extracts a JSONPath subset from an EventBridge event JSON body.
// Lab support: "$", "$.a", "$.a.b", and hyphenated keys via "$.detail-type".
// Bracket notation and wildcards are not supported (reject at delivery).
func applyEventBridgeInputPath(eventJSON, inputPath string) (string, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" || inputPath == "$" {
		return eventJSON, nil
	}
	if !strings.HasPrefix(inputPath, "$") {
		return "", fmt.Errorf("InputPath must start with $")
	}
	if strings.ContainsAny(inputPath, "[]*") {
		return "", fmt.Errorf("InputPath bracket/wildcard notation is not supported in lab")
	}
	var root any
	if err := json.Unmarshal([]byte(eventJSON), &root); err != nil {
		return "", fmt.Errorf("InputPath event json: %w", err)
	}
	cur := root
	rest := strings.TrimPrefix(inputPath, "$")
	if rest == "" {
		out, err := json.Marshal(cur)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	if !strings.HasPrefix(rest, ".") {
		return "", fmt.Errorf("InputPath must use JSONPath dot notation")
	}
	for _, seg := range strings.Split(strings.TrimPrefix(rest, "."), ".") {
		if seg == "" {
			return "", fmt.Errorf("InputPath has empty path segment")
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("InputPath segment %q is not an object", seg)
		}
		next, ok := obj[seg]
		if !ok {
			return "", fmt.Errorf("InputPath key %q not found", seg)
		}
		cur = next
	}
	out, err := json.Marshal(cur)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
