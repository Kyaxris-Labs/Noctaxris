package authz

import (
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

// Decision is the result of IAM evaluation for a request.
type Decision int

const (
	Deny Decision = iota
	Allow
)

// RequestContext is the authorization input for a single API call.
type RequestContext struct {
	Principal     identity.Principal
	Action        string
	Resource      string
	Region        string
	ConditionKeys map[string]string // e.g. aws:PrincipalAccount, aws:RequestedRegion
}

// Evaluate applies single-account identity-policy semantics:
//  1. Root principal → Allow
//  2. Catalog-unknown condition key in any statement → Deny (ADR-0005 §7.1)
//  3. Explicit Deny matching action+resource (+ applicable Condition) → Deny
//  4. Else any Allow matching → Allow
//  5. Else implicit Deny
//
// Unparseable policy documents are ignored. Statements whose Condition does
// not match (unpopulated known keys on positive operators, etc.) do not match
// for either Allow or Deny.
//
// Note: the HTTP handler for sts:GetCallerIdentity does not call Evaluate.
// AWS documents that GetCallerIdentity requires no IAM permissions after
// successful authentication. These unit tests still use that action string as
// a stand-in when exercising the identity-policy engine.
func Evaluate(ctx RequestContext, identityPolicyDocs []string) Decision {
	if ctx.Principal.IsRoot {
		return Allow
	}
	return evaluatePolicies(ctx, identityPolicyDocs)
}

// EvaluateWithSession applies identity Evaluate then intersects with optional
// sessionPolicyDocs. If sessionPolicyDocs is empty or nil, the result is the
// same as Evaluate.
//
// When session policies are present, the result is Allow only if both
// Evaluate(identityDocs) and the session-policy set Allow the request.
// Explicit Deny in either set yields Deny.
//
// Root with no session docs remains Allow (via Evaluate). Root with session
// docs still requires the session set to Allow; session evaluation does not
// take the root short-circuit (root rarely uses federation session policies).
func EvaluateWithSession(ctx RequestContext, identityDocs, sessionPolicyDocs []string) Decision {
	if len(sessionPolicyDocs) == 0 {
		return Evaluate(ctx, identityDocs)
	}
	if Evaluate(ctx, identityDocs) != Allow {
		return Deny
	}
	return evaluatePolicies(ctx, sessionPolicyDocs)
}

// evaluatePolicies applies Deny-overrides-Allow identity-policy matching
// without the root short-circuit.
func evaluatePolicies(ctx RequestContext, policyDocs []string) Decision {
	var denyHit, allowHit bool
	for _, raw := range policyDocs {
		doc, err := parsePolicyDocument(raw)
		if err != nil {
			continue
		}
		for _, st := range doc.Statement {
			matches, catalogUnknown := statementMatches(st, ctx)
			if catalogUnknown {
				return Deny
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
	if denyHit {
		return Deny
	}
	if allowHit {
		return Allow
	}
	return Deny
}
