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
//  2. Catalog-unknown condition key → Deny
//  3. If GrantSatisfied → key policy must still Allow (grant is not a key-policy bypass)
//  4. Else key policy must Allow (no root short-circuit on key policy)
//  5. AND identity must Allow (root short-circuit OK via Evaluate)
//  6. Else Deny
//
// Account-level ops: when KeyPolicyDoc == "" and not grant-satisfied, only identity Evaluate runs.
func EvaluateKMS(req KMSRequest) Decision {
	if req.KeyPolicyDoc == "" && !req.GrantSatisfied {
		if req.Caller.Resource == "*" {
			return Evaluate(req.Caller, req.IdentityDocs)
		}
		return Deny
	}

	identityDeny, _, identityUnknown := policyEffectHits(req.Caller, req.IdentityDocs)
	var keyDeny, keyAllow bool
	var keyUnknown bool
	if req.KeyPolicyDoc != "" {
		keyDeny, keyAllow, keyUnknown = resourcePolicyEffectHits(req.Caller, []string{req.KeyPolicyDoc})
	}
	if identityUnknown || keyUnknown || identityDeny || keyDeny {
		return Deny
	}
	if req.GrantSatisfied {
		if !keyAllow {
			return Deny
		}
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
// catalogUnknown is true if any Condition references a key absent from the catalog.
// Uses identity statement matching (Principal optional).
func policyEffectHits(ctx RequestContext, docs []string) (denyHit, allowHit bool, catalogUnknown bool) {
	return policyEffectHitsWith(ctx, docs, statementMatches)
}

// resourcePolicyEffectHits is policyEffectHits for resource-based documents
// (KMS key policy, S3/Secrets/DynamoDB/ECR resource policies). Missing Principal is match-none.
func resourcePolicyEffectHits(ctx RequestContext, docs []string) (denyHit, allowHit bool, catalogUnknown bool) {
	return policyEffectHitsWith(ctx, docs, resourceStatementMatches)
}

func policyEffectHitsWith(
	ctx RequestContext,
	docs []string,
	match func(statement, RequestContext) (bool, bool),
) (denyHit, allowHit bool, catalogUnknown bool) {
	for _, raw := range docs {
		doc, err := parsePolicyDocument(raw)
		if err != nil {
			continue
		}
		for _, st := range doc.Statement {
			matches, unknown := match(st, ctx)
			if unknown {
				return false, false, true
			}
			if !matches {
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
	return denyHit, allowHit, false
}
