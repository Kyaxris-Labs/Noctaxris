package authz

import (
	"encoding/json"
	"strings"
)

// policyDocument is an IAM identity policy JSON document.
type policyDocument struct {
	Version   string      `json:"Version"`
	Statement []statement `json:"Statement"`
}

type statement struct {
	Effect    string          `json:"Effect"`
	Action    stringOrSlice   `json:"Action"`
	Resource  stringOrSlice   `json:"Resource"`
	Condition json.RawMessage `json:"Condition"`
}

// stringOrSlice unmarshals a JSON string or array of strings.
type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = []string{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

func parsePolicyDocument(raw string) (policyDocument, error) {
	var doc policyDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return policyDocument{}, err
	}
	return doc, nil
}

const (
	condOpStringEquals = "StringEquals"
	condOpStringLike   = "StringLike"
)

var supportedConditionKeys = map[string]struct{}{
	"aws:PrincipalAccount": {},
	"aws:RequestedRegion":  {},
}

// conditionApplies reports whether the statement Condition matches ctx.
// Phase 1: only StringEquals / StringLike for aws:PrincipalAccount and
// aws:RequestedRegion. Unsupported operator/key or missing ConditionKeys
// value means the statement does not apply (neither Allow nor Deny).
func conditionApplies(raw json.RawMessage, keys map[string]string) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}

	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err != nil {
		return false
	}
	if len(block) == 0 {
		return true
	}

	for op, payload := range block {
		switch op {
		case condOpStringEquals, condOpStringLike:
		default:
			return false
		}

		var kv map[string]stringOrSlice
		if err := json.Unmarshal(payload, &kv); err != nil {
			return false
		}
		for key, expected := range kv {
			if _, ok := supportedConditionKeys[key]; !ok {
				return false
			}
			actual, ok := keys[key]
			if !ok {
				return false
			}
			if !conditionValuesMatch(op, actual, expected) {
				return false
			}
		}
	}
	return true
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

// stringLikeMatch implements IAM StringLike wildcards (* any sequence, ? one char).
func stringLikeMatch(pattern, value string) bool {
	return globMatch(pattern, value, true)
}

// matchPattern supports exact match and * wildcards, including prefix forms like sts:*.
func matchPattern(pattern, value string) bool {
	if pattern == "*" || pattern == value {
		return true
	}
	return globMatch(pattern, value, false)
}

func globMatch(pattern, value string, questionWildcard bool) bool {
	pi, vi := 0, 0
	starP, starV := -1, -1
	for vi < len(value) || pi < len(pattern) {
		if pi < len(pattern) {
			switch c := pattern[pi]; {
			case c == '*':
				starP = pi
				starV = vi
				pi++
				continue
			case questionWildcard && c == '?':
				if vi < len(value) {
					pi++
					vi++
					continue
				}
			case vi < len(value) && c == value[vi]:
				pi++
				vi++
				continue
			}
		}
		if starP >= 0 {
			starV++
			pi = starP + 1
			vi = starV
			continue
		}
		return false
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern) && vi == len(value)
}

func actionsMatch(patterns stringOrSlice, action string) bool {
	for _, p := range patterns {
		if matchPattern(p, action) {
			return true
		}
	}
	return false
}

func resourcesMatch(patterns stringOrSlice, resource string) bool {
	for _, p := range patterns {
		if matchPattern(p, resource) {
			return true
		}
	}
	return false
}

func statementMatches(st statement, ctx RequestContext) bool {
	effect := strings.EqualFold(st.Effect, "Allow") || strings.EqualFold(st.Effect, "Deny")
	if !effect {
		return false
	}
	if !actionsMatch(st.Action, ctx.Action) {
		return false
	}
	if !resourcesMatch(st.Resource, ctx.Resource) {
		return false
	}
	keys := ctx.ConditionKeys
	if keys == nil {
		keys = map[string]string{}
	}
	return conditionApplies(st.Condition, keys)
}
