package authz

import "strings"

// ServicePrincipalLambda is the AWS Lambda service principal used in role trust.
const ServicePrincipalLambda = "lambda.amazonaws.com"

const actionPassRole = "iam:PassRole"
const actionAssumeRole = "sts:AssumeRole"

// PassRoleRequest is input for configure-time PassRole checks.
type PassRoleRequest struct {
	Caller           RequestContext // Action is set by CheckPassRole to iam:PassRole
	IdentityDocs     []string
	RoleARN          string
	TrustPolicyDoc   string
	ServicePrincipal string // e.g. ServicePrincipalLambda
}

// CheckPassRole applies configure-time PassRole dual evaluation:
//  1. Caller must Allow iam:PassRole on RoleARN (root short-circuits via Evaluate)
//  2. Trust policy must Allow sts:AssumeRole for Principal Service = ServicePrincipal
//
// Both sides must Allow; otherwise Deny.
func CheckPassRole(req PassRoleRequest) Decision {
	ctx := req.Caller
	ctx.Action = actionPassRole
	ctx.Resource = req.RoleARN

	if Evaluate(ctx, req.IdentityDocs) != Allow {
		return Deny
	}
	if !TrustAllowsService(req.TrustPolicyDoc, req.ServicePrincipal) {
		return Deny
	}
	return Allow
}

// TrustAllowsService reports whether trustDoc explicitly Allows sts:AssumeRole
// for the given service principal (Principal.Service). Explicit Deny for that
// service yields false. Principal "*" matches any service.
func TrustAllowsService(trustDoc, servicePrincipal string) bool {
	if trustDoc == "" || servicePrincipal == "" {
		return false
	}
	doc, err := parsePolicyDocument(trustDoc)
	if err != nil {
		return false
	}
	var denyHit, allowHit bool
	for _, st := range doc.Statement {
		if !trustServiceStatementMatches(st, servicePrincipal) {
			continue
		}
		switch {
		case strings.EqualFold(st.Effect, "Deny"):
			denyHit = true
		case strings.EqualFold(st.Effect, "Allow"):
			allowHit = true
		}
	}
	if denyHit {
		return false
	}
	return allowHit
}

func trustServiceStatementMatches(st statement, servicePrincipal string) bool {
	effect := strings.EqualFold(st.Effect, "Allow") || strings.EqualFold(st.Effect, "Deny")
	if !effect {
		return false
	}
	if st.Principal == nil || !principalServiceMatches(*st.Principal, servicePrincipal) {
		return false
	}
	if !actionsMatch(st.Action, actionAssumeRole) {
		return false
	}
	if len(st.Resource) > 0 && !resourcesMatch(st.Resource, "*") {
		return false
	}
	return true
}

func principalServiceMatches(spec principalSpec, servicePrincipal string) bool {
	if spec.All {
		return true
	}
	for _, s := range spec.Service {
		if s == "*" || s == servicePrincipal {
			return true
		}
	}
	return false
}
