package authz

import (
	"encoding/json"
	"strings"

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
	condOpArnLike                 = "ArnLike"
	condOpArnEquals               = "ArnEquals"
	condOpStringNotEquals         = "StringNotEquals"
	condOpStringEqualsIfExists    = "StringEqualsIfExists"
	condOpStringLikeIfExists      = "StringLikeIfExists"
	condOpStringNotEqualsIfExists = "StringNotEqualsIfExists"
	condOpNull                    = "Null"
)

// conditionApplies reports how a statement Condition relates to request keys.
// ADR-0005 §7: catalog-unknown keys never silent-skip. Unpopulated known keys
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
		var kv map[string]stringOrSlice
		if err := json.Unmarshal(payload, &kv); err != nil {
			return condNoMatch
		}
		for key, expected := range kv {
			if !conditionkeys.Known(key) {
				return condCatalogUnknown
			}
			actual, present := keys[key]
			if !operatorMatches(op, actual, present, expected) {
				return condNoMatch
			}
		}
	}
	return condMatch
}

func operatorMatches(op, actual string, present bool, expected stringOrSlice) bool {
	switch op {
	case condOpStringEquals:
		if !present {
			return false
		}
		return conditionValuesMatch(condOpStringEquals, actual, expected)
	case condOpStringLike, condOpArnLike:
		if !present {
			return false
		}
		return conditionValuesMatch(condOpStringLike, actual, expected)
	case condOpArnEquals:
		if !present {
			return false
		}
		return conditionValuesMatch(condOpStringEquals, actual, expected)
	case condOpStringNotEquals:
		if !present {
			return true
		}
		return !conditionValuesMatch(condOpStringEquals, actual, expected)
	case condOpStringEqualsIfExists:
		if !present {
			return true
		}
		return conditionValuesMatch(condOpStringEquals, actual, expected)
	case condOpStringLikeIfExists:
		if !present {
			return true
		}
		return conditionValuesMatch(condOpStringLike, actual, expected)
	case condOpStringNotEqualsIfExists:
		if !present {
			return true
		}
		return !conditionValuesMatch(condOpStringEquals, actual, expected)
	case condOpNull:
		// Null: "true" means key must be absent. "false" means key must be present.
		for _, want := range expected {
			switch strings.ToLower(want) {
			case "true":
				if present {
					return false
				}
			case "false":
				if !present {
					return false
				}
			default:
				return false
			}
		}
		return true
	default:
		return false
	}
}

// conditionCatalogUnknown reports whether any key in the Condition block is
// absent from the generated catalog (ADR-0005 §7.1).
func conditionCatalogUnknown(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err != nil {
		return false
	}
	for _, payload := range block {
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
