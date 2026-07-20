package authz

// S3Request is the authorization input for S3 API evaluation.
type S3Request struct {
	Caller             RequestContext
	IdentityDocs       []string
	BucketPolicyDoc    string
	ResourceAccountID  string // empty or == caller.AccountID → same-account OR
}

// EvaluateS3 applies lab S3 authorization via EvaluateResourceAccess.
func EvaluateS3(req S3Request) Decision {
	return EvaluateResourceAccess(ResourceAccessRequest{
		Caller:             req.Caller,
		IdentityDocs:       req.IdentityDocs,
		ResourcePolicyDoc:  req.BucketPolicyDoc,
		ResourceAccountID:  req.ResourceAccountID,
	})
}
