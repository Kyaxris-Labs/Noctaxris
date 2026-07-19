package authz

import "strings"

// KMSRequest is the authorization input for KMS API evaluation.
//
// When KeyPolicyDoc is empty and GrantSatisfied is false:
//   - Resource "*" (account-level ops such as CreateKey/ListKeys) uses identity-only Evaluate
//   - otherwise (key-scoped resource without a key policy) the decision is Deny
type KMSRequest struct {
	Caller         RequestContext
	IdentityDocs   []string
	KeyPolicyDoc   string
	GrantSatisfied bool // true if a matching grant allows this operation for this principal
}

// EvaluateKMS applies lab KMS authorization:
//  1. Explicit Deny in identity or key policy → Deny
//  2. If GrantSatisfied → Allow (still Deny if step 1 hit)
//  3. Else key policy must Allow (no root short-circuit on key policy)
//  4. AND identity must Allow (root short-circuit OK via Evaluate)
//  5. Else Deny
//
// Account-level ops: when KeyPolicyDoc == "" and not grant-satisfied, only identity Evaluate runs.
func EvaluateKMS(req KMSRequest) Decision {
	if req.KeyPolicyDoc == "" && !req.GrantSatisfied {
		if req.Caller.Resource == "*" {
			return Evaluate(req.Caller, req.IdentityDocs)
		}
		return Deny
	}

	identityDeny, _ := policyEffectHits(req.Caller, req.IdentityDocs)
	var keyDeny, keyAllow bool
	if req.KeyPolicyDoc != "" {
		keyDeny, keyAllow = policyEffectHits(req.Caller, []string{req.KeyPolicyDoc})
	}
	if identityDeny || keyDeny {
		return Deny
	}
	if req.GrantSatisfied {
		return Allow
	}
	if !keyAllow {
		return Deny
	}
	if Evaluate(req.Caller, req.IdentityDocs) != Allow {
		return Deny
	}
	return Allow
}

// policyEffectHits reports whether any statement in docs is an explicit Deny or Allow match.
func policyEffectHits(ctx RequestContext, docs []string) (denyHit, allowHit bool) {
	for _, raw := range docs {
		doc, err := parsePolicyDocument(raw)
		if err != nil {
			continue
		}
		for _, st := range doc.Statement {
			if !statementMatches(st, ctx) {
				continue
			}
			switch {
			case strings.EqualFold(st.Effect, "Deny"):
				denyHit = true
			case strings.EqualFold(st.Effect, "Allow"):
				allowHit = true
			}
		}
	}
	return denyHit, allowHit
}
