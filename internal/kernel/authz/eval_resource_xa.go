package authz

import "strings"

// ResourceAccessRequest is the shared input for same-account OR and
// cross-account AND resource authorization (S3, SQS, SNS, DynamoDB, etc.).
type ResourceAccessRequest struct {
	Caller             RequestContext
	IdentityDocs       []string
	ResourcePolicyDoc  string
	ResourceAccountID  string // empty or == caller.AccountID → same-account OR
}

// EvaluateResourceAccess applies dataplane resource authorization:
//
// Same account (ResourceAccountID empty or equals caller account):
//  1. Explicit Deny in identity (non-root) or resource policy → Deny
//  2. Catalog-unknown condition key → Deny
//  3. Else Allow if identity Evaluate Allows (root short-circuit OK) OR resource policy Allows
//  4. Else Deny
//
// Cross account (caller account ≠ ResourceAccountID):
//  1. Explicit Deny in identity (non-root) or resource policy → Deny
//  2. Catalog-unknown condition key → Deny
//  3. Empty resource policy → Deny
//  4. Else Allow only if identity Evaluate Allows AND resource policy Allows
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
	var resourceDeny, resourceAllow bool
	var resourceUnknown bool
	if strings.TrimSpace(req.ResourcePolicyDoc) != "" {
		resourceDeny, resourceAllow, resourceUnknown = resourcePolicyEffectHits(req.Caller, []string{req.ResourcePolicyDoc})
	}
	if identityUnknown || resourceUnknown || identityDeny || resourceDeny {
		return Deny
	}

	identityAllow := Evaluate(req.Caller, req.IdentityDocs) == Allow

	if crossAccount {
		if strings.TrimSpace(req.ResourcePolicyDoc) == "" {
			return Deny
		}
		if identityAllow && resourceAllow {
			return Allow
		}
		return Deny
	}

	if identityAllow || resourceAllow {
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
