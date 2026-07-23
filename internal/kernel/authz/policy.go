package authz

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

// policyDocument is an IAM identity or resource (trust) policy JSON document.
type policyDocument struct {
	Version   string      `json:"Version"`
	Statement []statement `json:"Statement"`
}

type statement struct {
	Effect    string          `json:"Effect"`
	Action    stringOrSlice   `json:"Action"`
	Resource  stringOrSlice   `json:"Resource"`
	Principal *principalSpec  `json:"Principal"`
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

// principalSpec is the IAM Principal element (trust / resource policies).
// Supports Principal "*", {"AWS":"..."}, {"AWS":["...",...]},
// {"Service":"..."}, {"Service":["...",...]}, {"Federated":"..."},
// and {"Federated":["...",...]}.
type principalSpec struct {
	All       bool
	AWS       stringOrSlice
	Service   stringOrSlice
	Federated stringOrSlice
}

func (p *principalSpec) UnmarshalJSON(data []byte) error {
	var star string
	if err := json.Unmarshal(data, &star); err == nil {
		if star != "*" {
			return fmt.Errorf("unsupported Principal string %q", star)
		}
		p.All = true
		return nil
	}
	var obj struct {
		AWS       json.RawMessage `json:"AWS"`
		Service   json.RawMessage `json:"Service"`
		Federated json.RawMessage `json:"Federated"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	if len(obj.AWS) > 0 && string(obj.AWS) != "null" {
		var aws stringOrSlice
		if err := json.Unmarshal(obj.AWS, &aws); err != nil {
			return err
		}
		p.AWS = aws
	}
	if len(obj.Service) > 0 && string(obj.Service) != "null" {
		var svc stringOrSlice
		if err := json.Unmarshal(obj.Service, &svc); err != nil {
			return err
		}
		p.Service = svc
	}
	if len(obj.Federated) > 0 && string(obj.Federated) != "null" {
		var fed stringOrSlice
		if err := json.Unmarshal(obj.Federated, &fed); err != nil {
			return err
		}
		p.Federated = fed
	}
	return nil
}

func parsePolicyDocument(raw string) (policyDocument, error) {
	var doc policyDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return policyDocument{}, err
	}
	return doc, nil
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

func statementMatches(st statement, ctx RequestContext) (matches bool, catalogUnknown bool) {
	keys := ctx.ConditionKeys
	if keys == nil {
		keys = map[string]string{}
	}
	// Catalog-unknown is checked before action/resource so typo keys never silent-skip.
	if conditionCatalogUnknown(st.Condition) {
		return false, true
	}
	effect := strings.EqualFold(st.Effect, "Allow") || strings.EqualFold(st.Effect, "Deny")
	if !effect {
		return false, false
	}
	// Identity statements omit Principal. Resource-based docs use resourceStatementMatches.
	if st.Principal != nil && !principalMatches(*st.Principal, ctx.Principal) {
		return false, false
	}
	if !actionsMatch(st.Action, ctx.Action) {
		return false, false
	}
	if !resourcesMatch(st.Resource, ctx.Resource) {
		return false, false
	}
	switch conditionApplies(st.Condition, keys) {
	case condMatch:
		return true, false
	case condCatalogUnknown:
		return false, true
	default:
		return false, false
	}
}

// ValidateResourcePolicyDocument requires every statement to name a Principal.
// Identity policies must not use this helper (Principal remains optional there).
func ValidateResourcePolicyDocument(raw string) error {
	doc, err := parsePolicyDocument(raw)
	if err != nil {
		return fmt.Errorf("malformed policy document: %w", err)
	}
	if len(doc.Statement) == 0 {
		return fmt.Errorf("policy document must include at least one Statement")
	}
	for i, st := range doc.Statement {
		if st.Principal == nil {
			return fmt.Errorf("statement %d: Principal is required on resource-based policies", i)
		}
	}
	return nil
}

// resourceStatementMatches evaluates a resource-based policy statement
// (KMS key policy, S3/Secrets/DynamoDB/ECR). Missing Principal is match-none.
func resourceStatementMatches(st statement, ctx RequestContext) (matches bool, catalogUnknown bool) {
	keys := ctx.ConditionKeys
	if keys == nil {
		keys = map[string]string{}
	}
	if conditionCatalogUnknown(st.Condition) {
		return false, true
	}
	effect := strings.EqualFold(st.Effect, "Allow") || strings.EqualFold(st.Effect, "Deny")
	if !effect {
		return false, false
	}
	if st.Principal == nil || !principalMatches(*st.Principal, ctx.Principal) {
		return false, false
	}
	if !actionsMatch(st.Action, ctx.Action) {
		return false, false
	}
	if !resourcesMatch(st.Resource, ctx.Resource) {
		return false, false
	}
	switch conditionApplies(st.Condition, keys) {
	case condMatch:
		return true, false
	case condCatalogUnknown:
		return false, true
	default:
		return false, false
	}
}

// trustStatementMatches evaluates a role trust (resource-based) statement.
// Resource may be omitted or "*"; Principal must match the caller.
// There is no implicit allow: a missing Principal never matches.
func trustStatementMatches(st statement, ctx RequestContext) (matches bool, catalogUnknown bool) {
	keys := ctx.ConditionKeys
	if keys == nil {
		keys = map[string]string{}
	}
	if conditionCatalogUnknown(st.Condition) {
		return false, true
	}
	effect := strings.EqualFold(st.Effect, "Allow") || strings.EqualFold(st.Effect, "Deny")
	if !effect {
		return false, false
	}
	if st.Principal == nil || !principalMatches(*st.Principal, ctx.Principal) {
		return false, false
	}
	if !actionsMatch(st.Action, ctx.Action) {
		return false, false
	}
	if len(st.Resource) > 0 && !resourcesMatch(st.Resource, ctx.Resource) {
		return false, false
	}
	switch conditionApplies(st.Condition, keys) {
	case condMatch:
		return true, false
	case condCatalogUnknown:
		return false, true
	default:
		return false, false
	}
}

func principalMatches(spec principalSpec, caller identity.Principal) bool {
	if spec.All {
		return true
	}
	for _, p := range spec.Federated {
		if p == "*" {
			return true
		}
		if caller.FederatedProviderARN != "" && p == caller.FederatedProviderARN {
			return true
		}
	}
	// Federation assume paths must not over-allow via account-root AWS principals.
	if caller.FederatedProviderARN != "" || (caller.Kind == identity.KindFederated && len(spec.Federated) > 0) {
		return false
	}
	callerARN := caller.ARN()
	for _, p := range spec.AWS {
		if p == "*" {
			return true
		}
		if p == caller.AccountID {
			return true
		}
		if p == callerARN && callerARN != "" {
			return true
		}
		if accountID, ok := accountRootARN(p); ok && accountID == caller.AccountID {
			return true
		}
	}
	return false
}

// accountRootARN reports whether s is arn:aws:iam::ACCOUNT:root and returns ACCOUNT.
func accountRootARN(s string) (accountID string, ok bool) {
	const prefix = "arn:aws:iam::"
	const suffix = ":root"
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(s, prefix), suffix)
	if id == "" || strings.Contains(id, ":") {
		return "", false
	}
	return id, true
}
