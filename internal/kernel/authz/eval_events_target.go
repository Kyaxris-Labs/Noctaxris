package authz

import (
	"fmt"
	"strings"
)

// EventTargetResourcePolicyAllows reports whether policyDoc explicitly Allows action
// on resourceARN for Principal Service = servicePrincipal or Principal AWS = account root.
// Explicit Deny for a matching principal blocks that path. Allow when either path Allows
// and is not Denied.
func EventTargetResourcePolicyAllows(policyDoc, action, resourceARN, servicePrincipal, accountID string) bool {
	policyDoc = strings.TrimSpace(policyDoc)
	if policyDoc == "" || action == "" || resourceARN == "" || servicePrincipal == "" || accountID == "" {
		return false
	}
	doc, err := parsePolicyDocument(policyDoc)
	if err != nil {
		return false
	}
	var serviceDeny, serviceAllow, rootDeny, rootAllow bool
	for _, st := range doc.Statement {
		if resourcePrincipalStatementMatches(st, action, resourceARN, func(spec principalSpec) bool {
			return principalServiceMatches(spec, servicePrincipal)
		}) {
			switch {
			case strings.EqualFold(st.Effect, "Deny"):
				serviceDeny = true
			case strings.EqualFold(st.Effect, "Allow"):
				serviceAllow = true
			}
		}
		if resourcePrincipalStatementMatches(st, action, resourceARN, func(spec principalSpec) bool {
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

func resourcePrincipalStatementMatches(st statement, action, resourceARN string, principalCheck func(principalSpec) bool) bool {
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
	return true
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
