package server

import (
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// tryAnonymousS3Object handles unsigned path-style GetObject/HeadObject when
// NOCTAXRIS_ALLOW_ANONYMOUS_S3=1. It does not skip SigV4 for other S3 APIs or
// for buckets/objects without a public policy Allow or public-read ACL.
// Returns true when the request was consumed (success or AccessDenied/NoSuch*).
func (s *Server) tryAnonymousS3Object(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	readOnly bool,
) bool {
	if !s.cfg.AnonymousS3Allowed() {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		return false
	}
	if strings.EqualFold(r.URL.Query().Get("X-Amz-Algorithm"), "AWS4-HMAC-SHA256") {
		return false
	}
	if !isS3PathStyleRequest(r, body, resolveAction(r, body)) {
		return false
	}
	bucket, key, ok := parseS3Path(r.URL.Path)
	if !ok || bucket == "" || key == "" {
		return false
	}
	if !isAnonymousS3ObjectQuery(r) {
		return false
	}

	ref, err := s.s3ResolveBucket(bucket)
	if err != nil {
		verified := s.anonymousS3Verified(s.cfg.AccountID, r)
		if err == store.ErrNoSuchBucket {
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchBucket",
				"The specified bucket does not exist", eventNameForAnonymousS3(r))
			return true
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", eventNameForAnonymousS3(r))
		return true
	}

	verified := s.anonymousS3Verified(ref.accountID, r)
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))

	objectACL := ""
	meta, headErr := s.store.HeadObject(ref.accountID, bucket, key)
	if headErr == nil {
		objectACL = meta.CannedACL
	}

	if authz.EvaluateAnonymousS3GetObject(authz.AnonymousS3GetRequest{
		Resource:          resource,
		ResourceAccountID: ref.accountID,
		BucketPolicyDoc:   ref.policy,
		ObjectCannedACL:   objectACL,
	}) != authz.Allow {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", eventNameForAnonymousS3(r))
		return true
	}

	if r.Method == http.MethodHead {
		s.s3HeadObject(w, r, requestID, eventID, verified, readOnly, bucket, key)
		return true
	}
	s.s3GetObject(w, r, requestID, eventID, verified, readOnly, bucket, key)
	return true
}

func (s *Server) anonymousS3Verified(accountID string, r *http.Request) *authn.Verified {
	return &authn.Verified{
		Principal: identity.AnonymousPrincipal(accountID),
		AccountID: accountID,
		Region:    "us-east-1",
		Service:   "s3",
		SourceIP:  clientIP(r),
	}
}

func (s *Server) authorizeAnonymousS3Get(
	verified *authn.Verified,
	resource, bucketPolicy, resourceAccountID, objectACL string,
) bool {
	if verified == nil || verified.Principal.Kind != identity.KindAnonymous {
		return false
	}
	return authz.EvaluateAnonymousS3GetObject(authz.AnonymousS3GetRequest{
		Resource:          resource,
		ResourceAccountID: resourceAccountID,
		BucketPolicyDoc:   bucketPolicy,
		ObjectCannedACL:   objectACL,
	}) == authz.Allow
}

func eventNameForAnonymousS3(r *http.Request) string {
	if r.Method == http.MethodHead {
		return "HeadObject"
	}
	return "GetObject"
}

// isAnonymousS3ObjectQuery allows only GetObject/HeadObject query shapes
// (optional versionId). Subresource queries are not anonymous-eligible.
func isAnonymousS3ObjectQuery(r *http.Request) bool {
	q := r.URL.Query()
	for key := range q {
		switch strings.ToLower(key) {
		case "versionid":
			continue
		default:
			return false
		}
	}
	return true
}
