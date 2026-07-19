package authz

import "strings"

// S3Request is the authorization input for S3 API evaluation.
type S3Request struct {
	Caller          RequestContext
	IdentityDocs    []string
	BucketPolicyDoc string
}

// EvaluateS3 applies lab S3 authorization:
//  1. Explicit Deny in identity (non-root) or bucket policy → Deny
//  2. Else Allow if identity Evaluate Allows (root short-circuit OK) OR bucket policy Allows
//  3. Else Deny
//
// Bucket policy evaluation uses policyEffectHits (no root short-circuit).
func EvaluateS3(req S3Request) Decision {
	var identityDeny bool
	if !req.Caller.Principal.IsRoot {
		identityDeny, _ = policyEffectHits(req.Caller, req.IdentityDocs)
	}
	var bucketDeny, bucketAllow bool
	if strings.TrimSpace(req.BucketPolicyDoc) != "" {
		bucketDeny, bucketAllow = policyEffectHits(req.Caller, []string{req.BucketPolicyDoc})
	}
	if identityDeny || bucketDeny {
		return Deny
	}
	if Evaluate(req.Caller, req.IdentityDocs) == Allow || bucketAllow {
		return Allow
	}
	return Deny
}
