package authz

import "strings"

// SNSRequest is the authorization input for SNS API evaluation.
type SNSRequest struct {
	Caller          RequestContext
	IdentityDocs    []string
	TopicPolicyDoc  string
}

// EvaluateSNS applies lab SNS authorization:
//  1. Explicit Deny in identity (non-root) or topic policy → Deny
//  2. Catalog-unknown condition key → Deny
//  3. Else Allow if identity Evaluate Allows (root short-circuit OK) OR topic policy Allows
//  4. Else Deny
//
// Topic policy evaluation uses policyEffectHits (no root short-circuit).
func EvaluateSNS(req SNSRequest) Decision {
	var identityDeny bool
	var identityUnknown bool
	if !req.Caller.Principal.IsRoot {
		identityDeny, _, identityUnknown = policyEffectHits(req.Caller, req.IdentityDocs)
	}
	var topicDeny, topicAllow bool
	var topicUnknown bool
	if strings.TrimSpace(req.TopicPolicyDoc) != "" {
		topicDeny, topicAllow, topicUnknown = policyEffectHits(req.Caller, []string{req.TopicPolicyDoc})
	}
	if identityUnknown || topicUnknown || identityDeny || topicDeny {
		return Deny
	}
	if Evaluate(req.Caller, req.IdentityDocs) == Allow || topicAllow {
		return Allow
	}
	return Deny
}
