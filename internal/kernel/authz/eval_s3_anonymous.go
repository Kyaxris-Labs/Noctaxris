package authz

import (
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

// CannedACLPublicRead and CannedACLPublicReadWrite grant AllUsers READ
// (anonymous GetObject) in the lab ACL subset.
const (
	CannedACLPrivate         = "private"
	CannedACLPublicRead      = "public-read"
	CannedACLPublicReadWrite = "public-read-write"
)

// ObjectACLGrantsAnonymousRead reports whether a canned object ACL grants
// unauthenticated GetObject (AllUsers READ).
func ObjectACLGrantsAnonymousRead(cannedACL string) bool {
	switch strings.ToLower(strings.TrimSpace(cannedACL)) {
	case CannedACLPublicRead, CannedACLPublicReadWrite:
		return true
	default:
		return false
	}
}

// AnonymousS3GetRequest is the input for unauthenticated S3 GetObject evaluation.
type AnonymousS3GetRequest struct {
	Resource          string // object ARN
	ResourceAccountID string
	BucketPolicyDoc   string
	ObjectCannedACL   string
}

// EvaluateAnonymousS3GetObject authorizes unsigned GetObject against a bucket
// policy (Principal "*" / {"AWS":"*"}) and/or object canned ACL public-read.
// Explicit Deny in the bucket policy wins over ACL. Empty policy + private ACL
// denies. Caller account is the bucket owner so same-account resource evaluation
// applies without inventing a cross-account identity Allow.
func EvaluateAnonymousS3GetObject(req AnonymousS3GetRequest) Decision {
	accountID := strings.TrimSpace(req.ResourceAccountID)
	caller := RequestContext{
		Principal: identity.AnonymousPrincipal(accountID),
		Action:    "s3:GetObject",
		Resource:  req.Resource,
	}

	policy := strings.TrimSpace(req.BucketPolicyDoc)
	if policy != "" {
		denyHit, allowHit, unknown := resourcePolicyEffectHits(caller, []string{policy})
		if unknown || denyHit {
			return Deny
		}
		if allowHit {
			return Allow
		}
	}
	if ObjectACLGrantsAnonymousRead(req.ObjectCannedACL) {
		return Allow
	}
	return Deny
}
