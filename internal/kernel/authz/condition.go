package authz

import (
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog/conditionkeys"
)

type conditionOutcome int

const (
	condMatch conditionOutcome = iota
	condNoMatch
	condCatalogUnknown
)

const (
	condOpStringEquals            = "StringEquals"
	condOpStringLike              = "StringLike"
	condOpStringNotLike           = "StringNotLike"
	condOpArnLike                 = "ArnLike"
	condOpArnEquals               = "ArnEquals"
	condOpArnNotEquals            = "ArnNotEquals"
	condOpArnNotLike              = "ArnNotLike"
	condOpStringNotEquals         = "StringNotEquals"
	condOpStringEqualsIfExists    = "StringEqualsIfExists"
	condOpStringLikeIfExists      = "StringLikeIfExists"
	condOpStringNotEqualsIfExists = "StringNotEqualsIfExists"
	condOpStringNotLikeIfExists   = "StringNotLikeIfExists"
	condOpNull                    = "Null"
	condOpBool                    = "Bool"
	condOpBoolIfExists            = "BoolIfExists"
	condOpIpAddress               = "IpAddress"
	condOpIpAddressIfExists       = "IpAddressIfExists"
	condOpNotIpAddress            = "NotIpAddress"
	condOpNotIpAddressIfExists    = "NotIpAddressIfExists"
	condOpNumericEquals           = "NumericEquals"
	condOpNumericNotEquals        = "NumericNotEquals"
	condOpNumericLessThan         = "NumericLessThan"
	condOpNumericLessThanEquals   = "NumericLessThanEquals"
	condOpNumericGreaterThan      = "NumericGreaterThan"
	condOpNumericGreaterThanEquals = "NumericGreaterThanEquals"
	condOpDateEquals              = "DateEquals"
	condOpDateNotEquals           = "DateNotEquals"
	condOpDateLessThan            = "DateLessThan"
	condOpDateLessThanEquals      = "DateLessThanEquals"
	condOpDateGreaterThan         = "DateGreaterThan"
	condOpDateGreaterThanEquals   = "DateGreaterThanEquals"

	setQualForAnyValue  = "ForAnyValue"
	setQualForAllValues = "ForAllValues"
)

// conditionApplies reports how a statement Condition relates to request keys.
// ADR-0005 §7: catalog-unknown keys never silent-skip. Unrecognized operators
// use the same fail-closed outcome (request Deny). Unpopulated known keys
// fail positive operators (never allow). StringNotEquals / Null / IfExists
// follow AWS missing-key operator rules.
func conditionApplies(raw json.RawMessage, keys map[string]string) conditionOutcome {
	if len(raw) == 0 || string(raw) == "null" {
		return condMatch
	}

	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err != nil {
		return condNoMatch
	}
	if len(block) == 0 {
		return condMatch
	}

	for op, payload := range block {
		baseOp, setQual, ok := parseConditionOperator(op)
		if !ok {
			return condCatalogUnknown
		}
		var kv map[string]stringOrSlice
		if err := json.Unmarshal(payload, &kv); err != nil {
			return condNoMatch
		}
		for key, expected := range kv {
			if !conditionkeys.Known(key) {
				return condCatalogUnknown
			}
			actual, present := keys[key]
			if setQual != "" {
				if !setOperatorMatches(setQual, baseOp, actual, present, expected) {
					return condNoMatch
				}
				continue
			}
			matched, known := operatorMatches(baseOp, actual, present, expected)
			if !known {
				return condCatalogUnknown
			}
			if !matched {
				return condNoMatch
			}
		}
	}
	return condMatch
}

// parseConditionOperator splits optional ForAnyValue:/ForAllValues: qualifiers.
// ok is false for unrecognized base operators.
func parseConditionOperator(op string) (baseOp, setQual string, ok bool) {
	op = strings.TrimSpace(op)
	switch {
	case strings.HasPrefix(op, setQualForAnyValue+":"):
		baseOp = strings.TrimPrefix(op, setQualForAnyValue+":")
		setQual = setQualForAnyValue
	case strings.HasPrefix(op, setQualForAllValues+":"):
		baseOp = strings.TrimPrefix(op, setQualForAllValues+":")
		setQual = setQualForAllValues
	default:
		baseOp = op
	}
	if !knownConditionOperator(baseOp) {
		return "", "", false
	}
	if setQual != "" && !setOperatorBaseAllowed(baseOp) {
		return "", "", false
	}
	return baseOp, setQual, true
}

func knownConditionOperator(op string) bool {
	switch op {
	case condOpStringEquals, condOpStringLike, condOpStringNotLike,
		condOpArnLike, condOpArnEquals, condOpArnNotEquals, condOpArnNotLike,
		condOpStringNotEquals,
		condOpStringEqualsIfExists, condOpStringLikeIfExists,
		condOpStringNotEqualsIfExists, condOpStringNotLikeIfExists,
		condOpNull, condOpBool, condOpBoolIfExists,
		condOpIpAddress, condOpIpAddressIfExists, condOpNotIpAddress, condOpNotIpAddressIfExists,
		condOpNumericEquals, condOpNumericNotEquals,
		condOpNumericLessThan, condOpNumericLessThanEquals,
		condOpNumericGreaterThan, condOpNumericGreaterThanEquals,
		condOpDateEquals, condOpDateNotEquals,
		condOpDateLessThan, condOpDateLessThanEquals,
		condOpDateGreaterThan, condOpDateGreaterThanEquals:
		return true
	default:
		// Numeric*/Date* IfExists variants
		if strings.HasSuffix(op, "IfExists") {
			base := strings.TrimSuffix(op, "IfExists")
			switch base {
			case condOpNumericEquals, condOpNumericNotEquals,
				condOpNumericLessThan, condOpNumericLessThanEquals,
				condOpNumericGreaterThan, condOpNumericGreaterThanEquals,
				condOpDateEquals, condOpDateNotEquals,
				condOpDateLessThan, condOpDateLessThanEquals,
				condOpDateGreaterThan, condOpDateGreaterThanEquals,
				condOpArnEquals, condOpArnLike, condOpArnNotEquals, condOpArnNotLike:
				return true
			}
		}
		return false
	}
}

func setOperatorBaseAllowed(op string) bool {
	switch op {
	case condOpStringEquals, condOpStringLike, condOpStringNotEquals, condOpStringNotLike,
		condOpArnEquals, condOpArnLike, condOpArnNotEquals, condOpArnNotLike:
		return true
	default:
		return false
	}
}

// operatorMatches returns (matched, known). known=false means unrecognized operator.
func operatorMatches(op, actual string, present bool, expected stringOrSlice) (matched, known bool) {
	ifExists := false
	if strings.HasSuffix(op, "IfExists") {
		ifExists = true
		op = strings.TrimSuffix(op, "IfExists")
	}

	switch op {
	case condOpStringEquals, condOpArnEquals:
		if !present {
			return ifExists, true
		}
		return conditionValuesMatch(condOpStringEquals, actual, expected), true
	case condOpStringLike, condOpArnLike:
		if !present {
			return ifExists, true
		}
		return conditionValuesMatch(condOpStringLike, actual, expected), true
	case condOpStringNotEquals, condOpArnNotEquals:
		if !present {
			return true, true
		}
		return !conditionValuesMatch(condOpStringEquals, actual, expected), true
	case condOpStringNotLike, condOpArnNotLike:
		if !present {
			return true, true
		}
		return !conditionValuesMatch(condOpStringLike, actual, expected), true
	case condOpNull:
		for _, want := range expected {
			switch strings.ToLower(want) {
			case "true":
				if present {
					return false, true
				}
			case "false":
				if !present {
					return false, true
				}
			default:
				return false, true
			}
		}
		return true, true
	case condOpBool:
		if !present {
			return ifExists, true
		}
		return boolValuesMatch(actual, expected), true
	case condOpIpAddress:
		if !present {
			return ifExists, true
		}
		return ipAddressMatch(actual, expected), true
	case condOpNotIpAddress:
		if !present {
			return true, true
		}
		return !ipAddressMatch(actual, expected), true
	case condOpNumericEquals, condOpNumericNotEquals,
		condOpNumericLessThan, condOpNumericLessThanEquals,
		condOpNumericGreaterThan, condOpNumericGreaterThanEquals:
		if !present {
			return ifExists, true
		}
		ok, cmp := numericCompare(actual, expected, op)
		if op == condOpNumericNotEquals {
			return ok && !cmp, true
		}
		return ok && cmp, true
	case condOpDateEquals, condOpDateNotEquals,
		condOpDateLessThan, condOpDateLessThanEquals,
		condOpDateGreaterThan, condOpDateGreaterThanEquals:
		if !present {
			return ifExists, true
		}
		ok, cmp := dateCompare(actual, expected, op)
		if op == condOpDateNotEquals {
			return ok && !cmp, true
		}
		return ok && cmp, true
	default:
		return false, false
	}
}

func setOperatorMatches(setQual, baseOp, actual string, present bool, expected stringOrSlice) bool {
	// Lab keys are single-valued; treat a present key as a one-element set.
	// AWS: ForAllValues with missing key is vacuously true; ForAnyValue is false.
	if !present {
		return setQual == setQualForAllValues
	}
	requestVals := []string{actual}
	switch setQual {
	case setQualForAnyValue:
		for _, rv := range requestVals {
			matched, known := operatorMatches(baseOp, rv, true, expected)
			if known && matched {
				return true
			}
		}
		return false
	case setQualForAllValues:
		for _, rv := range requestVals {
			matched, known := operatorMatches(baseOp, rv, true, expected)
			if !known || !matched {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func boolValuesMatch(actual string, expected stringOrSlice) bool {
	a := strings.ToLower(strings.TrimSpace(actual))
	if a != "true" && a != "false" {
		return false
	}
	for _, want := range expected {
		if a == strings.ToLower(strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func ipAddressMatch(actual string, expected stringOrSlice) bool {
	ip := net.ParseIP(strings.TrimSpace(actual))
	if ip == nil {
		return false
	}
	for _, want := range expected {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		if strings.Contains(want, "/") {
			_, network, err := net.ParseCIDR(want)
			if err != nil {
				continue
			}
			if network.Contains(ip) {
				return true
			}
			continue
		}
		if other := net.ParseIP(want); other != nil && ip.Equal(other) {
			return true
		}
	}
	return false
}

func numericCompare(actual string, expected stringOrSlice, op string) (ok, matches bool) {
	av, err := strconv.ParseFloat(strings.TrimSpace(actual), 64)
	if err != nil {
		return false, false
	}
	for _, want := range expected {
		wv, err := strconv.ParseFloat(strings.TrimSpace(want), 64)
		if err != nil {
			continue
		}
		ok = true
		switch op {
		case condOpNumericEquals:
			if av == wv {
				return true, true
			}
		case condOpNumericNotEquals:
			if av == wv {
				return true, true
			}
		case condOpNumericLessThan:
			if av < wv {
				return true, true
			}
		case condOpNumericLessThanEquals:
			if av <= wv {
				return true, true
			}
		case condOpNumericGreaterThan:
			if av > wv {
				return true, true
			}
		case condOpNumericGreaterThanEquals:
			if av >= wv {
				return true, true
			}
		}
	}
	if op == condOpNumericNotEquals {
		return ok, false
	}
	return ok, false
}

func dateCompare(actual string, expected stringOrSlice, op string) (ok, matches bool) {
	at, aok := parseConditionTime(actual)
	if !aok {
		return false, false
	}
	for _, want := range expected {
		wt, wok := parseConditionTime(want)
		if !wok {
			continue
		}
		ok = true
		switch op {
		case condOpDateEquals:
			if at.Equal(wt) {
				return true, true
			}
		case condOpDateNotEquals:
			if at.Equal(wt) {
				return true, true
			}
		case condOpDateLessThan:
			if at.Before(wt) {
				return true, true
			}
		case condOpDateLessThanEquals:
			if !at.After(wt) {
				return true, true
			}
		case condOpDateGreaterThan:
			if at.After(wt) {
				return true, true
			}
		case condOpDateGreaterThanEquals:
			if !at.Before(wt) {
				return true, true
			}
		}
	}
	if op == condOpDateNotEquals {
		return ok, false
	}
	return ok, false
}

func parseConditionTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(unix, 0).UTC(), true
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// conditionCatalogUnknown reports whether any key or operator in the Condition
// block is unrecognized (ADR-0005 §7.1; unknown operators fail closed).
func conditionCatalogUnknown(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err != nil {
		return false
	}
	for op, payload := range block {
		if _, _, ok := parseConditionOperator(op); !ok {
			return true
		}
		var kv map[string]stringOrSlice
		if err := json.Unmarshal(payload, &kv); err != nil {
			continue
		}
		for key := range kv {
			if !conditionkeys.Known(key) {
				return true
			}
		}
	}
	return false
}

func conditionValuesMatch(op, actual string, expected stringOrSlice) bool {
	for _, want := range expected {
		switch op {
		case condOpStringEquals:
			if actual == want {
				return true
			}
		case condOpStringLike:
			if stringLikeMatch(want, actual) {
				return true
			}
		}
	}
	return false
}
