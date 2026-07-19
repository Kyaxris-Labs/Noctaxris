package authz

import "strings"

// SQSRequest is the authorization input for SQS API evaluation.
type SQSRequest struct {
	Caller         RequestContext
	IdentityDocs   []string
	QueuePolicyDoc string
}

// EvaluateSQS applies lab SQS authorization:
//  1. Explicit Deny in identity (non-root) or queue policy → Deny
//  2. Else Allow if identity Evaluate Allows (root short-circuit OK) OR queue policy Allows
//  3. Else Deny
//
// Queue policy evaluation uses policyEffectHits (no root short-circuit).
func EvaluateSQS(req SQSRequest) Decision {
	var identityDeny bool
	if !req.Caller.Principal.IsRoot {
		identityDeny, _ = policyEffectHits(req.Caller, req.IdentityDocs)
	}
	var queueDeny, queueAllow bool
	if strings.TrimSpace(req.QueuePolicyDoc) != "" {
		queueDeny, queueAllow = policyEffectHits(req.Caller, []string{req.QueuePolicyDoc})
	}
	if identityDeny || queueDeny {
		return Deny
	}
	if Evaluate(req.Caller, req.IdentityDocs) == Allow || queueAllow {
		return Allow
	}
	return Deny
}
