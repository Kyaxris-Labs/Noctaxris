package catalog

// ActionSTSGetCallerIdentity is the IAM action for STS GetCallerIdentity.
const ActionSTSGetCallerIdentity = "sts:GetCallerIdentity"

// KnownAction reports whether action is implemented in this Phase.
// Phase 1 only recognizes sts:GetCallerIdentity.
func KnownAction(action string) bool {
	return action == ActionSTSGetCallerIdentity
}
