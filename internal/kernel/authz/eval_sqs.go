package authz

// SQSRequest is the authorization input for SQS API evaluation.
type SQSRequest struct {
	Caller             RequestContext
	IdentityDocs       []string
	QueuePolicyDoc     string
	ResourceAccountID  string // empty or == caller.AccountID → same-account OR
}

// EvaluateSQS applies lab SQS authorization via EvaluateResourceAccess.
func EvaluateSQS(req SQSRequest) Decision {
	return EvaluateResourceAccess(ResourceAccessRequest{
		Caller:             req.Caller,
		IdentityDocs:       req.IdentityDocs,
		ResourcePolicyDoc:  req.QueuePolicyDoc,
		ResourceAccountID:  req.ResourceAccountID,
	})
}
