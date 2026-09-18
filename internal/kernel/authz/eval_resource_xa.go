package authz

import "strings"

// ResourceAccessRequest is the shared input for same-account OR and
// cross-account AND resource authorization (S3, SQS, SNS, DynamoDB, etc.).
type ResourceAccessRequest struct {
	Caller            RequestContext
	IdentityDocs      []string
	ResourcePolicyDoc string
	ResourceAccountID string // empty or == caller.AccountID → same-account OR
}

// EvaluateResourceAccess applies dataplane resource authorization:
//
// Same account (ResourceAccountID empty or equals caller account):
//  1. Explicit Deny in identity (non-root) or resource policy → Deny
//     (resource Deny uses account-delegation matching so Principal :root
//     still Denies IAM users in the account)
//  2. Catalog-unknown condition key → Deny
//  3. Else Allow if identity Evaluate Allows (root short-circuit OK) OR the
//     resource policy Allows the caller as a direct principal (* , user/role/session ARN).
//     Account :root / 12-digit account id is not a direct grant on this OR path.
//  4. Else Deny
//
// Cross account (caller account ≠ ResourceAccountID):
//  1. Explicit Deny in identity (non-root) or resource policy → Deny
//  2. Catalog-unknown condition key → Deny
//  3. Empty resource policy → Deny
//  4. Else Allow only if identity Evaluate Allows AND the resource policy
//     Allows with account-delegation matching (:root means that account)
//  5. Else Deny
//
// Resource policy evaluation uses policyEffectHits (no root short-circuit).
func EvaluateResourceAccess(req ResourceAccessRequest) Decision {
	crossAccount := isCrossAccountResource(req.Caller.Principal.AccountID, req.ResourceAccountID)

	var identityDeny bool
	var identityUnknown bool
	if !req.Caller.Principal.IsRoot {
		identityDeny, _, identityUnknown = policyEffectHits(req.Caller, req.IdentityDocs)
	}
	var resourceDeny, resourceAllowDirect, resourceAllowDelegated bool
	var resourceUnknown bool
	if strings.TrimSpace(req.ResourcePolicyDoc) != "" {
		docs := []string{req.ResourcePolicyDoc}
		var unknownDirect, unknownDelegated bool
		_, resourceAllowDirect, unknownDirect = resourcePolicyEffectHits(req.Caller, docs)
		resourceDeny, resourceAllowDelegated, unknownDelegated = delegatedResourcePolicyEffectHits(req.Caller, docs)
		resourceUnknown = unknownDirect || unknownDelegated
	}
	if identityUnknown || resourceUnknown || identityDeny || resourceDeny {
		return Deny
	}

	identityAllow := Evaluate(req.Caller, req.IdentityDocs) == Allow

	if crossAccount {
		if strings.TrimSpace(req.ResourcePolicyDoc) == "" {
			return Deny
		}
		if identityAllow && resourceAllowDelegated {
			return Allow
		}
		return Deny
	}

	if identityAllow || resourceAllowDirect {
		return Allow
	}
	return Deny
}

func isCrossAccountResource(callerAccountID, resourceAccountID string) bool {
	ra := strings.TrimSpace(resourceAccountID)
	if ra == "" {
		return false
	}
	return ra != strings.TrimSpace(callerAccountID)
}
