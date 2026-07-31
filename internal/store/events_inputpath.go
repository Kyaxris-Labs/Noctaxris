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
	var root any
	if err := json.Unmarshal([]byte(eventJSON), &root); err != nil {
		return "", fmt.Errorf("InputPath event json: %w", err)
	}
	cur, err := LabJSONPathExtract(root, inputPath, LabPathNested)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(cur)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
