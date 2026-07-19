package authz

import "strings"

// DynamoDBRequest is the authorization input for DynamoDB API evaluation.
type DynamoDBRequest struct {
	Caller            RequestContext
	IdentityDocs      []string
	ResourcePolicyDoc string
}

// EvaluateDynamoDB applies lab DynamoDB authorization:
//  1. Explicit Deny in identity (non-root) or resource policy → Deny
//  2. Else Allow if identity Evaluate Allows (root short-circuit OK) OR resource policy Allows
//  3. Else Deny
//
// Resource policy evaluation uses policyEffectHits (no root short-circuit).
func EvaluateDynamoDB(req DynamoDBRequest) Decision {
	var identityDeny bool
	if !req.Caller.Principal.IsRoot {
		identityDeny, _ = policyEffectHits(req.Caller, req.IdentityDocs)
	}
	var resourceDeny, resourceAllow bool
	if strings.TrimSpace(req.ResourcePolicyDoc) != "" {
		resourceDeny, resourceAllow = policyEffectHits(req.Caller, []string{req.ResourcePolicyDoc})
	}
	if identityDeny || resourceDeny {
		return Deny
	}
	if Evaluate(req.Caller, req.IdentityDocs) == Allow || resourceAllow {
		return Allow
	}
	return Deny
}
