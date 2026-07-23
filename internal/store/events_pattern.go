package store

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// matchEventPatternJSON reports whether event fields satisfy an EventBridge event pattern.
// Unknown operator objects fail closed (non-match, err=nil). Invalid JSON returns an error.
func matchEventPatternJSON(patternJSON, source, detailType, detailJSON string) (bool, error) {
	var pattern map[string]any
	if err := json.Unmarshal([]byte(patternJSON), &pattern); err != nil {
		return false, fmt.Errorf("match event pattern: %w", err)
	}
	if len(pattern) == 0 {
		return false, nil
	}

	envelope := map[string]any{
		"source":      source,
		"detail-type": detailType,
	}
	if strings.TrimSpace(detailJSON) != "" {
		var eventDetail map[string]any
		if err := json.Unmarshal([]byte(detailJSON), &eventDetail); err != nil {
			return false, fmt.Errorf("match event pattern: event detail: %w", err)
		}
		envelope["detail"] = eventDetail
	} else {
		envelope["detail"] = map[string]any{}
	}

	return matchPatternObject(pattern, envelope, true), nil
}

// validateEventPatternDeep rejects unsupported EventBridge operators at PutRule time.
func validateEventPatternDeep(pattern string) error {
	var doc any
	if err := json.Unmarshal([]byte(pattern), &doc); err != nil {
		return fmt.Errorf("put rule: invalid event pattern: %w", err)
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return fmt.Errorf("put rule: event pattern must be a JSON object")
	}
	if err := validatePatternValue(obj, true); err != nil {
		return fmt.Errorf("put rule: %w", err)
	}
	return nil
}

func validatePatternValue(v any, asPatternObject bool) error {
	typed, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("event pattern must be a JSON object")
	}
	if _, hasOr := typed["$or"]; hasOr {
		return fmt.Errorf("event pattern operator $or is not supported")
	}
	for k, child := range typed {
		if k == "$or" {
			return fmt.Errorf("event pattern operator $or is not supported")
		}
		switch c := child.(type) {
		case map[string]any:
			if err := validatePatternValue(c, true); err != nil {
				return err
			}
		case []any:
			if err := validateMatcherArray(c); err != nil {
				return err
			}
		default:
			return fmt.Errorf("event pattern field %q must be an object or array", k)
		}
	}
	_ = asPatternObject
	return nil
}

func validateMatcherArray(items []any) error {
	for _, item := range items {
		switch typed := item.(type) {
		case map[string]any:
			if len(typed) != 1 {
				return fmt.Errorf("event pattern operator objects must have exactly one key")
			}
			for op, arg := range typed {
				if err := validateOperator(op, arg); err != nil {
					return err
				}
			}
		case string, float64, bool, nil:
			// exact match literals
		default:
			return fmt.Errorf("event pattern array items must be literals or operator objects")
		}
	}
	return nil
}

func validateOperator(op string, arg any) error {
	switch op {
	case "prefix", "suffix":
		switch arg.(type) {
		case string:
			return nil
		case map[string]any:
			return fmt.Errorf("event pattern operator %q with nested equals-ignore-case is not supported", op)
		default:
			return fmt.Errorf("event pattern operator %q requires a string", op)
		}
	case "exists":
		if _, ok := arg.(bool); !ok {
			return fmt.Errorf("event pattern operator exists requires a boolean")
		}
		return nil
	case "equals-ignore-case":
		if _, ok := arg.(string); !ok {
			return fmt.Errorf("event pattern operator equals-ignore-case requires a string")
		}
		return nil
	case "numeric":
		arr, ok := arg.([]any)
		if !ok || len(arr) == 0 || len(arr)%2 != 0 {
			return fmt.Errorf("event pattern operator numeric requires [op, number, ...]")
		}
		for i := 0; i < len(arr); i += 2 {
			opStr, ok := arr[i].(string)
			if !ok {
				return fmt.Errorf("event pattern operator numeric: expected comparison string")
			}
			switch opStr {
			case "=", ">", ">=", "<", "<=":
			default:
				return fmt.Errorf("event pattern operator numeric: unsupported comparison %q", opStr)
			}
			if _, ok := asFloat64(arr[i+1]); !ok {
				return fmt.Errorf("event pattern operator numeric: expected number")
			}
		}
		return nil
	case "anything-but":
		return validateAnythingButArg(arg)
	case "wildcard", "cidr":
		return fmt.Errorf("event pattern operator %q is not supported", op)
	default:
		return fmt.Errorf("event pattern operator %q is not supported", op)
	}
}

func validateAnythingButArg(arg any) error {
	switch typed := arg.(type) {
	case string, float64, bool, nil:
		return nil
	case []any:
		for _, item := range typed {
			switch item.(type) {
			case string, float64, bool, nil:
			default:
				return fmt.Errorf("event pattern anything-but list items must be scalars")
			}
		}
		return nil
	case map[string]any:
		if len(typed) != 1 {
			return fmt.Errorf("event pattern anything-but nested object must have exactly one key")
		}
		for nested, nestedArg := range typed {
			switch nested {
			case "prefix", "suffix":
				switch nestedArg.(type) {
				case string:
					return nil
				case []any:
					for _, item := range nestedArg.([]any) {
						if _, ok := item.(string); !ok {
							return fmt.Errorf("event pattern anything-but %s list items must be strings", nested)
						}
					}
					return nil
				case map[string]any:
					return fmt.Errorf("event pattern anything-but %s with nested equals-ignore-case is not supported", nested)
				default:
					return fmt.Errorf("event pattern anything-but %s requires a string or string list", nested)
				}
			case "wildcard", "equals-ignore-case", "cidr":
				return fmt.Errorf("event pattern anything-but %q is not supported", nested)
			default:
				return fmt.Errorf("event pattern anything-but %q is not supported", nested)
			}
		}
		return nil
	default:
		return fmt.Errorf("event pattern anything-but has unsupported argument type")
	}
}

// matchPatternObject matches an EventBridge pattern object against an event object.
// When topLevel is true, pattern keys absent from the event (except detail nesting) are evaluated via leaf rules.
func matchPatternObject(pattern, event map[string]any, _ bool) bool {
	for key, expected := range pattern {
		actual, present := event[key]
		switch exp := expected.(type) {
		case map[string]any:
			if key == "$or" {
				return false
			}
			actMap, ok := actual.(map[string]any)
			if !present || !ok {
				return false
			}
			if !matchPatternObject(exp, actMap, false) {
				return false
			}
		case []any:
			if !matchLeafMatchers(exp, actual, present) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func matchLeafMatchers(matchers []any, actual any, present bool) bool {
	for _, matcher := range matchers {
		if leafMatcherMatches(matcher, actual, present) {
			return true
		}
	}
	return false
}

func leafMatcherMatches(matcher, actual any, present bool) bool {
	switch typed := matcher.(type) {
	case map[string]any:
		if len(typed) != 1 {
			return false
		}
		for op, arg := range typed {
			return operatorMatches(op, arg, actual, present)
		}
		return false
	default:
		if !present {
			return false
		}
		return jsonValueEqual(typed, actual)
	}
}

func operatorMatches(op string, arg, actual any, present bool) bool {
	switch op {
	case "exists":
		want, ok := arg.(bool)
		if !ok {
			return false
		}
		return present == want
	case "prefix":
		if !present {
			return false
		}
		prefix, ok := arg.(string)
		if !ok {
			return false
		}
		s, ok := actual.(string)
		return ok && strings.HasPrefix(s, prefix)
	case "suffix":
		if !present {
			return false
		}
		suffix, ok := arg.(string)
		if !ok {
			return false
		}
		s, ok := actual.(string)
		return ok && strings.HasSuffix(s, suffix)
	case "equals-ignore-case":
		if !present {
			return false
		}
		want, ok := arg.(string)
		if !ok {
			return false
		}
		s, ok := actual.(string)
		return ok && strings.EqualFold(s, want)
	case "numeric":
		if !present {
			return false
		}
		return numericMatches(arg, actual)
	case "anything-but":
		if !present {
			return false
		}
		return anythingButMatches(arg, actual)
	default:
		// Unknown operators fail closed at match time.
		return false
	}
}

func numericMatches(arg, actual any) bool {
	arr, ok := arg.([]any)
	if !ok || len(arr) == 0 || len(arr)%2 != 0 {
		return false
	}
	value, ok := asFloat64(actual)
	if !ok {
		return false
	}
	for i := 0; i < len(arr); i += 2 {
		op, ok := arr[i].(string)
		if !ok {
			return false
		}
		bound, ok := asFloat64(arr[i+1])
		if !ok {
			return false
		}
		switch op {
		case "=":
			if value != bound {
				return false
			}
		case ">":
			if !(value > bound) {
				return false
			}
		case ">=":
			if !(value >= bound) {
				return false
			}
		case "<":
			if !(value < bound) {
				return false
			}
		case "<=":
			if !(value <= bound) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func anythingButMatches(arg, actual any) bool {
	switch typed := arg.(type) {
	case map[string]any:
		if len(typed) != 1 {
			return false
		}
		for nested, nestedArg := range typed {
			switch nested {
			case "prefix":
				return !stringHasAnyPrefix(actual, nestedArg)
			case "suffix":
				return !stringHasAnySuffix(actual, nestedArg)
			default:
				return false
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if jsonValueEqual(item, actual) {
				return false
			}
		}
		return true
	default:
		return !jsonValueEqual(typed, actual)
	}
}

func stringHasAnyPrefix(actual any, want any) bool {
	s, ok := actual.(string)
	if !ok {
		return false
	}
	switch typed := want.(type) {
	case string:
		return strings.HasPrefix(s, typed)
	case []any:
		for _, item := range typed {
			p, ok := item.(string)
			if ok && strings.HasPrefix(s, p) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func stringHasAnySuffix(actual any, want any) bool {
	s, ok := actual.(string)
	if !ok {
		return false
	}
	switch typed := want.(type) {
	case string:
		return strings.HasSuffix(s, typed)
	case []any:
		for _, item := range typed {
			suf, ok := item.(string)
			if ok && strings.HasSuffix(s, suf) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func asFloat64(v any) (float64, bool) {
	switch typed := v.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		f, err := typed.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(typed, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// patternValueMatches is the shared leaf matcher used by EventBridge and ESM filters
// when the field is present. Missing-field exists semantics are handled only by matchLeafMatchers.
func patternValueMatches(patternVal, actual any) bool {
	switch typed := patternVal.(type) {
	case []any:
		return matchLeafMatchers(typed, actual, true)
	default:
		return leafMatcherMatches(typed, actual, true)
	}
}

func jsonValueEqual(a, b any) bool {
	return reflect.DeepEqual(normalizeJSONValue(a), normalizeJSONValue(b))
}

func normalizeJSONValue(v any) any {
	switch typed := v.(type) {
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed)
		}
		return typed
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			out[k] = normalizeJSONValue(val)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, val := range typed {
			out[i] = normalizeJSONValue(val)
		}
		return out
	default:
		return v
	}
}
