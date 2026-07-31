package store

import (
	"errors"
	"fmt"
	"strings"
)

// LabPathProfile selects JSONPath subset rules for lab services.
type LabPathProfile int

const (
	// LabPathTopLevelOnly allows $.field (SFN InputPath / ResultPath subset).
	LabPathTopLevelOnly LabPathProfile = iota
	// LabPathNested allows $.a.b and hyphenated keys (EventBridge InputPath).
	LabPathNested
)

var errLabJSONPathMissingKey = errors.New("lab jsonpath key not found")

// LabJSONPathMissingKey reports whether err means an absent path segment (SFN maps this to JSON null).
func LabJSONPathMissingKey(err error) bool {
	return errors.Is(err, errLabJSONPathMissingKey)
}

// LabJSONPathExtract walks path on unmarshaled JSON data (map[string]any trees).
// Bracket and wildcard notation is rejected. Missing keys return errLabJSONPathMissingKey.
func LabJSONPathExtract(data any, path string, profile LabPathProfile) (any, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "$" {
		return data, nil
	}
	if !strings.HasPrefix(path, "$") {
		if profile == LabPathNested {
			return nil, fmt.Errorf("InputPath must start with $")
		}
		return nil, fmt.Errorf("InputPath must be $.field")
	}
	if strings.ContainsAny(path, "[]*") {
		return nil, fmt.Errorf("InputPath bracket/wildcard notation is not supported in lab")
	}
	rest := strings.TrimPrefix(path, "$")
	if rest == "" {
		return data, nil
	}
	switch profile {
	case LabPathTopLevelOnly:
		return labJSONPathTopLevel(data, rest)
	case LabPathNested:
		return labJSONPathNested(data, rest)
	default:
		return nil, fmt.Errorf("unknown lab jsonpath profile")
	}
}

func labJSONPathTopLevel(data any, rest string) (any, error) {
	if !strings.HasPrefix(rest, ".") {
		return nil, fmt.Errorf("InputPath must be $.field")
	}
	field := strings.TrimPrefix(rest, ".")
	if field == "" || strings.Contains(field, ".") || strings.ContainsAny(field, "[") {
		return nil, fmt.Errorf("InputPath must be $.field")
	}
	obj, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("InputPath input is not a JSON object")
	}
	v, present := obj[field]
	if !present {
		return nil, errLabJSONPathMissingKey
	}
	return v, nil
}

func labJSONPathNested(data any, rest string) (any, error) {
	if !strings.HasPrefix(rest, ".") {
		return nil, fmt.Errorf("InputPath must use JSONPath dot notation")
	}
	cur := data
	for _, seg := range strings.Split(strings.TrimPrefix(rest, "."), ".") {
		if seg == "" {
			return nil, fmt.Errorf("InputPath has empty path segment")
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("InputPath segment %q is not an object", seg)
		}
		next, ok := obj[seg]
		if !ok {
			return nil, fmt.Errorf("InputPath key %q not found", seg)
		}
		cur = next
	}
	return cur, nil
}
