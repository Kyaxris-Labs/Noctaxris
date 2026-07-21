package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EventBridgeInputTransformer is the lab PutTargets InputTransformer shape.
type EventBridgeInputTransformer struct {
	InputPathsMap map[string]string `json:"InputPathsMap"`
	InputTemplate string            `json:"InputTemplate"`
}

// applyEventBridgeInputTransformer builds target input from InputPathsMap + InputTemplate.
// Lab support: same JSONPath subset as InputPath for map values; template placeholders
// are <varname> (AWS shape). Bracket/wildcard paths are rejected.
func applyEventBridgeInputTransformer(eventJSON string, tr EventBridgeInputTransformer) (string, error) {
	template := strings.TrimSpace(tr.InputTemplate)
	if template == "" {
		return "", fmt.Errorf("InputTransformer.InputTemplate is required")
	}
	vars := map[string]string{}
	for name, path := range tr.InputPathsMap {
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		if name == "" || path == "" {
			return "", fmt.Errorf("InputTransformer.InputPathsMap entries must be non-empty")
		}
		extracted, err := applyEventBridgeInputPath(eventJSON, path)
		if err != nil {
			return "", fmt.Errorf("InputTransformer path %q: %w", name, err)
		}
		vars[name] = extracted
	}
	out := template
	for name, value := range vars {
		placeholder := "<" + name + ">"
		if !strings.Contains(out, placeholder) {
			continue
		}
		out = strings.ReplaceAll(out, placeholder, value)
	}
	if strings.Contains(out, "<") && strings.Contains(out, ">") {
		// Leave unresolved static <...> only when not from InputPathsMap; reject dangling map vars.
		for name := range vars {
			if strings.Contains(out, "<"+name+">") {
				return "", fmt.Errorf("InputTransformer unresolved placeholder %q", name)
			}
		}
	}
	if !json.Valid([]byte(out)) {
		// AWS allows non-JSON string templates; wrap as JSON string for lab targets that expect JSON.
		raw, err := json.Marshal(out)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
	return out, nil
}

func parseEventBridgeInputTransformerJSON(raw string) (EventBridgeInputTransformer, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return EventBridgeInputTransformer{}, false, nil
	}
	var tr EventBridgeInputTransformer
	if err := json.Unmarshal([]byte(raw), &tr); err != nil {
		return EventBridgeInputTransformer{}, false, fmt.Errorf("InputTransformer json: %w", err)
	}
	if strings.TrimSpace(tr.InputTemplate) == "" {
		return EventBridgeInputTransformer{}, false, fmt.Errorf("InputTransformer.InputTemplate is required")
	}
	if tr.InputPathsMap == nil {
		tr.InputPathsMap = map[string]string{}
	}
	return tr, true, nil
}

func marshalEventBridgeInputTransformer(tr *EventBridgeInputTransformer) (string, error) {
	if tr == nil || strings.TrimSpace(tr.InputTemplate) == "" {
		return "", nil
	}
	if tr.InputPathsMap == nil {
		tr.InputPathsMap = map[string]string{}
	}
	raw, err := json.Marshal(tr)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
