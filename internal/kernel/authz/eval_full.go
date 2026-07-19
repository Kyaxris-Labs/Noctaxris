package authz

import "strings"

// EvalInputs is the full authorization input for EvaluateFull
// (identity, optional boundary, SCP/RCP, and session policies).
// (ADR-0005 §8: groups via IdentityDocs, boundaries, SCPs/RCPs, session).
type EvalInputs struct {
	IdentityDocs        []string // user + group identity policies
	BoundaryDoc         string   // empty = no permissions boundary
	SessionDocs         []string
	SCPDocs             []string
	RCPDocs             []string
	IsManagementAccount bool
}

// EvaluateFull applies the single-account enforcement spine (ADR-0005 §8):
//  1. Explicit Deny / catalog-unknown across identity, boundary, session, SCP, RCP
//  2. RCPs (if any docs): each doc must Allow; never grants alone
//  3. SCPs (if any docs and not management account): each doc must Allow
//  4. Resource-based policies: not in EvalInputs (use EvaluateS3/KMS/etc.)
//  5. Identity-based policies (root short-circuit; else need Allow)
//  6. Permissions boundary ∩ identity (empty BoundaryDoc skips; root skips)
//  7. Session policies (if present): must Allow
//
// SCPs/RCPs never grant. Boundary alone grants nothing.
func EvaluateFull(ctx RequestContext, in EvalInputs) Decision {
	if denyScanFull(ctx, in) {
		return Deny
	}

	if len(in.RCPDocs) > 0 && !orgFilterAllows(ctx, in.RCPDocs) {
		return Deny
	}
	if !in.IsManagementAccount && len(in.SCPDocs) > 0 && !orgFilterAllows(ctx, in.SCPDocs) {
		return Deny
	}

	if Evaluate(ctx, in.IdentityDocs) != Allow {
		return Deny
	}

	if !ctx.Principal.IsRoot && strings.TrimSpace(in.BoundaryDoc) != "" {
		if evaluatePolicies(ctx, []string{in.BoundaryDoc}) != Allow {
			return Deny
		}
	}

	if len(in.SessionDocs) > 0 {
		if evaluatePolicies(ctx, in.SessionDocs) != Allow {
			return Deny
		}
	}
	return Allow
}
