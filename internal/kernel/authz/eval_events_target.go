package authz

import (
	"fmt"
	"strings"
)

// DeliverySourceConditionKeys builds aws:SourceArn / aws:SourceAccount for
// service-principal delivery evaluation (confused-deputy locks).
func DeliverySourceConditionKeys(sourceARN, sourceAccount string) map[string]string {
	keys := map[string]string{}
	sourceARN = strings.TrimSpace(sourceARN)
	sourceAccount = strings.TrimSpace(sourceAccount)
	if sourceARN != "" {
		keys["aws:SourceArn"] = sourceARN
	}
	if sourceAccount != "" {
		keys["aws:SourceAccount"] = sourceAccount
	} else if sourceARN != "" {
		if acct := accountIDFromResourceARN(sourceARN); acct != "" {
			keys["aws:SourceAccount"] = acct
		}
	}
	return keys
}

func accountIDFromResourceARN(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 5 {
		return ""
	}
	acct := strings.TrimSpace(parts[4])
	if len(acct) != 12 {
		return ""
	}
	for _, r := range acct {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return acct
}

// EventTargetResourcePolicyAllows reports whether policyDoc explicitly Allows action
// on resourceARN for Principal Service = servicePrincipal or Principal AWS = account root.
// Explicit Deny for a matching principal blocks that path. Allow when either path Allows
// and is not Denied.
//
// When a statement includes Condition, conditionKeys must satisfy it (same engine as
// identity Evaluate). Unpopulated aws:SourceArn / aws:SourceAccount fail closed on
// positive operators so SourceArn locks deny mismatched or missing delivery sources.
func EventTargetResourcePolicyAllows(policyDoc, action, resourceARN, servicePrincipal, accountID string, conditionKeys map[string]string) bool {
	policyDoc = strings.TrimSpace(policyDoc)
	if policyDoc == "" || action == "" || resourceARN == "" || servicePrincipal == "" || accountID == "" {
		return false
	}
	doc, err := parsePolicyDocument(policyDoc)
	if err != nil {
		return false
	}
	if conditionKeys == nil {
		conditionKeys = map[string]string{}
	}
	for _, st := range doc.Statement {
		if conditionCatalogUnknown(st.Condition) {
			return false
		}
	}
	var serviceDeny, serviceAllow, rootDeny, rootAllow bool
	for _, st := range doc.Statement {
		if resourcePrincipalStatementMatches(st, action, resourceARN, conditionKeys, func(spec principalSpec) bool {
			return principalServiceMatches(spec, servicePrincipal)
		}) {
			switch {
			case strings.EqualFold(st.Effect, "Deny"):
				serviceDeny = true
			case strings.EqualFold(st.Effect, "Allow"):
				serviceAllow = true
			}
		}
		if resourcePrincipalStatementMatches(st, action, resourceARN, conditionKeys, func(spec principalSpec) bool {
			return principalRootMatches(spec, accountID)
		}) {
			switch {
			case strings.EqualFold(st.Effect, "Deny"):
				rootDeny = true
			case strings.EqualFold(st.Effect, "Allow"):
				rootAllow = true
			}
		}
	}
	return eventTargetPathAllowed(serviceDeny, serviceAllow) || eventTargetPathAllowed(rootDeny, rootAllow)
}

func eventTargetPathAllowed(denyHit, allowHit bool) bool {
	if denyHit {
		return false
	}
	return allowHit
}

func resourcePrincipalStatementMatches(st statement, action, resourceARN string, conditionKeys map[string]string, principalCheck func(principalSpec) bool) bool {
	effect := strings.EqualFold(st.Effect, "Allow") || strings.EqualFold(st.Effect, "Deny")
	if !effect {
		return false
	}
	if st.Principal == nil || !principalCheck(*st.Principal) {
		return false
	}
	if !actionsMatch(st.Action, action) {
		return false
	}
	if len(st.Resource) > 0 && !resourcesMatch(st.Resource, resourceARN) {
		return false
	}
	switch conditionApplies(st.Condition, conditionKeys) {
	case condMatch:
		return true
	case condNoMatch, condCatalogUnknown:
		return false
	default:
		return false
	}
}

func principalRootMatches(spec principalSpec, accountID string) bool {
	if spec.All {
		return true
	}
	rootARN := fmt.Sprintf("arn:aws:iam::%s:root", accountID)
	for _, p := range spec.AWS {
		if p == "*" || p == rootARN {
			return true
		}
		if id, ok := accountRootARN(p); ok && id == accountID {
			return true
		}
	}
	return false
}
