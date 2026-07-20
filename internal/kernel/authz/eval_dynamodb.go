package authz

// DynamoDBRequest is the authorization input for DynamoDB API evaluation.
type DynamoDBRequest struct {
	Caller             RequestContext
	IdentityDocs       []string
	ResourcePolicyDoc  string
	ResourceAccountID  string // empty or == caller.AccountID → same-account OR
}

// EvaluateDynamoDB applies lab DynamoDB authorization via EvaluateResourceAccess.
func EvaluateDynamoDB(req DynamoDBRequest) Decision {
	return EvaluateResourceAccess(ResourceAccessRequest{
		Caller:             req.Caller,
		IdentityDocs:       req.IdentityDocs,
		ResourcePolicyDoc:  req.ResourcePolicyDoc,
		ResourceAccountID:  req.ResourceAccountID,
	})
}
