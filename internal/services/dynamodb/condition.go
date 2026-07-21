package dynamodb

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrConditionalCheckFailed is returned when a ConditionExpression evaluates to false.
var ErrConditionalCheckFailed = errors.New("ConditionalCheckFailedException")

// ErrFilterNoMatch is returned when a FilterExpression evaluates to false for an item.
var ErrFilterNoMatch = errors.New("FilterExpressionNoMatch")

// EvaluateConditionExpression evaluates a documented lab subset of DynamoDB
// ConditionExpression against an existing item (nil/empty when the item is absent).
//
// Supported:
//   - attribute_not_exists(attr) / attribute_exists(attr)
//   - top-level comparisons: attr = :v, <>, <, <=, >, >=
//   - AND / OR of the above (left-associative, AND binds tighter than OR is not modeled;
//     expressions are split on top-level OR then AND)
//
// Unsupported operators or nested paths return a ValidationException-shaped error
// (never silently ignored).
func EvaluateConditionExpression(
	item ItemMap,
	expr string,
	names map[string]any,
	values map[string]any,
) error {
	return evaluateItemExpression(item, expr, names, values, "ConditionExpression", ErrConditionalCheckFailed)
}

// EvaluateFilterExpression evaluates FilterExpression with the same lab subset as
// ConditionExpression. Non-matching items return ErrFilterNoMatch (omit from results).
// Unsupported operators fail closed with a ValidationException-shaped error.
func EvaluateFilterExpression(
	item ItemMap,
	expr string,
	names map[string]any,
	values map[string]any,
) error {
	return evaluateItemExpression(item, expr, names, values, "FilterExpression", ErrFilterNoMatch)
}

func evaluateItemExpression(
	item ItemMap,
	expr string,
	names map[string]any,
	values map[string]any,
	label string,
	noMatch error,
) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	if item == nil {
		item = ItemMap{}
	}
	ok, err := evalConditionOR(item, expr, names, values)
	if err != nil {
		return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), "ConditionExpression", label))
	}
	if !ok {
		return noMatch
	}
	return nil
}

func evalConditionOR(item ItemMap, expr string, names, values map[string]any) (bool, error) {
	parts := splitKeyword(expr, "OR")
	if len(parts) == 1 {
		return evalConditionAND(item, parts[0], names, values)
	}
	for _, p := range parts {
		ok, err := evalConditionAND(item, p, names, values)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func evalConditionAND(item ItemMap, expr string, names, values map[string]any) (bool, error) {
	parts := splitKeyword(expr, "AND")
	for _, p := range parts {
		ok, err := evalConditionTerm(item, strings.TrimSpace(p), names, values)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalConditionTerm(item ItemMap, term string, names, values map[string]any) (bool, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return false, fmt.Errorf("ConditionExpression is invalid")
	}
	upper := strings.ToUpper(term)

	if strings.HasPrefix(upper, "ATTRIBUTE_NOT_EXISTS(") && strings.HasSuffix(term, ")") {
		attr, err := resolveName(strings.TrimSpace(term[len("attribute_not_exists("):len(term)-1]), names)
		if err != nil {
			return false, err
		}
		if strings.Contains(attr, ".") || strings.Contains(attr, "[") {
			return false, fmt.Errorf("ConditionExpression nested paths are not supported")
		}
		_, exists := item[attr]
		return !exists, nil
	}
	if strings.HasPrefix(upper, "ATTRIBUTE_EXISTS(") && strings.HasSuffix(term, ")") {
		attr, err := resolveName(strings.TrimSpace(term[len("attribute_exists("):len(term)-1]), names)
		if err != nil {
			return false, err
		}
		if strings.Contains(attr, ".") || strings.Contains(attr, "[") {
			return false, fmt.Errorf("ConditionExpression nested paths are not supported")
		}
		_, exists := item[attr]
		return exists, nil
	}

	ops := []string{"<>", "<=", ">=", "=", "<", ">"}
	for _, op := range ops {
		left, right, ok := splitOnceOp(term, op)
		if !ok {
			continue
		}
		left = strings.TrimSpace(left)
		if strings.Contains(left, "(") || strings.Contains(left, ")") {
			return false, fmt.Errorf("ConditionExpression is not supported: %s", term)
		}
		attr, err := resolveName(left, names)
		if err != nil {
			return false, err
		}
		if strings.Contains(attr, ".") || strings.Contains(attr, "[") {
			return false, fmt.Errorf("ConditionExpression nested paths are not supported")
		}
		right = strings.TrimSpace(right)
		if !strings.HasPrefix(right, ":") {
			return false, fmt.Errorf("ConditionExpression comparisons require ExpressionAttributeValues placeholders")
		}
		want, ok := values[right].(map[string]any)
		if !ok {
			return false, fmt.Errorf("ExpressionAttributeValues missing %s", right)
		}
		got, exists := item[attr]
		if !exists {
			return false, nil
		}
		cmp, err := compareAV(got, want)
		if err != nil {
			return false, err
		}
		switch op {
		case "=":
			return cmp == 0, nil
		case "<>":
			return cmp != 0, nil
		case "<":
			return cmp < 0, nil
		case "<=":
			return cmp <= 0, nil
		case ">":
			return cmp > 0, nil
		case ">=":
			return cmp >= 0, nil
		}
	}

	return false, fmt.Errorf("ConditionExpression is not supported: %s", term)
}

func splitOnceOp(term, op string) (left, right string, ok bool) {
	idx := strings.Index(term, op)
	if idx < 0 {
		return "", "", false
	}
	// Avoid matching '=' inside '<>', '<=' , '>=' when looking for '=' alone.
	if op == "=" || op == "<" || op == ">" {
		if idx > 0 {
			prev := term[idx-1]
			if prev == '<' || prev == '>' || prev == '!' {
				return "", "", false
			}
		}
		if op == "<" || op == ">" {
			if idx+1 < len(term) && (term[idx+1] == '=' || term[idx+1] == '>') {
				return "", "", false
			}
		}
	}
	return term[:idx], term[idx+len(op):], true
}

func splitKeyword(expr, kw string) []string {
	upper := strings.ToUpper(expr)
	var parts []string
	start := 0
	depth := 0
	i := 0
	for i < len(expr) {
		ch := expr[i]
		if ch == '(' {
			depth++
			i++
			continue
		}
		if ch == ')' {
			if depth > 0 {
				depth--
			}
			i++
			continue
		}
		if depth == 0 {
			if i+len(kw) <= len(upper) && upper[i:i+len(kw)] == kw {
				beforeOK := i == 0 || isSpace(upper[i-1])
				after := i + len(kw)
				afterOK := after >= len(upper) || isSpace(upper[after])
				if beforeOK && afterOK {
					parts = append(parts, strings.TrimSpace(expr[start:i]))
					start = after
					i = after
					continue
				}
			}
		}
		i++
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	return parts
}

func compareAV(a, b map[string]any) (int, error) {
	if as, ok := a["S"].(string); ok {
		bs, ok := b["S"].(string)
		if !ok {
			return 0, fmt.Errorf("ConditionExpression type mismatch")
		}
		return strings.Compare(as, bs), nil
	}
	if an, ok := numericAV(a); ok {
		bn, ok := numericAV(b)
		if !ok {
			return 0, fmt.Errorf("ConditionExpression type mismatch")
		}
		switch {
		case an < bn:
			return -1, nil
		case an > bn:
			return 1, nil
		default:
			return 0, nil
		}
	}
	if ab, ok := a["BOOL"].(bool); ok {
		bb, ok := b["BOOL"].(bool)
		if !ok {
			return 0, fmt.Errorf("ConditionExpression type mismatch")
		}
		switch {
		case ab == bb:
			return 0, nil
		case !ab && bb:
			return -1, nil
		default:
			return 1, nil
		}
	}
	return 0, fmt.Errorf("ConditionExpression attribute type is not supported")
}

func numericAV(av map[string]any) (float64, bool) {
	raw, ok := av["N"]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case string:
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}
