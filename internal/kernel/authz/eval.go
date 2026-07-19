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

// Evaluate applies single-account Phase 1 identity-policy semantics:
//  1. Root principal → Allow
//  2. Explicit Deny matching action+resource (+ applicable Condition) → Deny
//  3. Else any Allow matching → Allow
//  4. Else implicit Deny
//
// Unparseable policy documents are ignored. Statements whose Condition cannot
// be evaluated (unsupported operator/key or missing ConditionKeys) do not match
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

	var denyHit, allowHit bool
	for _, raw := range identityPolicyDocs {
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
	if denyHit {
		return Deny
	}
	if allowHit {
		return Allow
	}
	return Deny
}
