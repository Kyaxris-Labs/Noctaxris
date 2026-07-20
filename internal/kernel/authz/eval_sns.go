package authz

// SNSRequest is the authorization input for SNS API evaluation.
type SNSRequest struct {
	Caller             RequestContext
	IdentityDocs       []string
	TopicPolicyDoc     string
	ResourceAccountID  string // empty or == caller.AccountID → same-account OR
}

// EvaluateSNS applies lab SNS authorization via EvaluateResourceAccess.
func EvaluateSNS(req SNSRequest) Decision {
	return EvaluateResourceAccess(ResourceAccessRequest{
		Caller:             req.Caller,
		IdentityDocs:       req.IdentityDocs,
		ResourcePolicyDoc:  req.TopicPolicyDoc,
		ResourceAccountID:  req.ResourceAccountID,
	})
}
