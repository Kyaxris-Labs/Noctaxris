package authz

import "strings"

// CrossAccountRequest is the input for AssumeRole dual evaluation
// (caller identity policy + role trust policy), per AWS cross-account logic.
type CrossAccountRequest struct {
	Caller             RequestContext // Action should be sts:AssumeRole; Resource = role ARN
	CallerIdentityDocs []string
	TrustPolicyDoc     string
}

// EvaluateCrossAccount applies AssumeRole dual evaluation:
//  1. Explicit Deny on caller identity or trust → Deny
//  2. Caller identity must Allow sts:AssumeRole on the role ARN
//     (account root short-circuits to Allow via Evaluate)
//  3. Trust policy must explicitly Allow the caller principal
//     (resource-based: no implicit allow; root still needs trust Allow)
//  4. Else Deny
//
// Both sides must Allow. Trust documents use Principal (AWS string or list);
// Resource may be omitted or "*".
func EvaluateCrossAccount(req CrossAccountRequest) Decision {
	if Evaluate(req.Caller, req.CallerIdentityDocs) == Deny {
		return Deny
	}

	trustDeny, trustAllow := evaluateTrust(req.Caller, req.TrustPolicyDoc)
	if trustDeny {
		return Deny
	}
	if trustAllow {
		return Allow
	}
	return Deny
}

func evaluateTrust(ctx RequestContext, trustDoc string) (denyHit, allowHit bool) {
	if trustDoc == "" {
		return false, false
	}
	doc, err := parsePolicyDocument(trustDoc)
	if err != nil {
		return false, false
	}
	for _, st := range doc.Statement {
		if !trustStatementMatches(st, ctx) {
			continue
		}
		switch {
		case strings.EqualFold(st.Effect, "Deny"):
			denyHit = true
		case strings.EqualFold(st.Effect, "Allow"):
			allowHit = true
		}
	}
	return denyHit, allowHit
}
