package authz

import "strings"

// orgFilterAllows reports whether every org filter document (SCP or RCP)
// explicitly Allows the request. Empty docs are handled by the caller
// (skip the filter). SCPs/RCPs never grant on their own.
func orgFilterAllows(ctx RequestContext, docs []string) bool {
	for _, raw := range docs {
		if evaluatePolicies(ctx, []string{raw}) != Allow {
			return false
		}
	}
	return true
}

// denyScanFull returns true if any applicable policy type has an explicit
// Deny match or a catalog-unknown condition key. Management accounts skip
// SCP docs (SCPs do not apply).
func denyScanFull(ctx RequestContext, in EvalInputs) bool {
	docs := append([]string{}, in.IdentityDocs...)
	docs = append(docs, in.SessionDocs...)
	docs = append(docs, in.RCPDocs...)
	if !in.IsManagementAccount {
		docs = append(docs, in.SCPDocs...)
	}
	if !ctx.Principal.IsRoot && strings.TrimSpace(in.BoundaryDoc) != "" {
		docs = append(docs, in.BoundaryDoc)
	}
	for _, raw := range docs {
		deny, _, unknown := policyEffectHits(ctx, []string{raw})
		if unknown || deny {
			return true
		}
	}
	return false
}
