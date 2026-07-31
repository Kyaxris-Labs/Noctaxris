package server

import (
	"errors"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

// passRoleMsgs customizes ARN validation error text per AWS API surface.
// Empty fields use the defaults below.
type passRoleMsgs struct {
	InvalidARN   string
	WrongAccount string
	NotFound     string
}

func (m passRoleMsgs) withDefaults() passRoleMsgs {
	if m.InvalidARN == "" {
		m.InvalidARN = "roleARN must be a valid IAM role ARN"
	}
	if m.WrongAccount == "" {
		m.WrongAccount = "roleARN must be in the same account"
	}
	if m.NotFound == "" {
		m.NotFound = "Role not found"
	}
	return m
}

// checkAPIGatewayPassRole enforces iam:PassRole plus apigateway.amazonaws.com trust
// when Gateway configure APIs supply CredentialsArn. sourceARN is the HTTP API ARN
// (arn:aws:apigateway:region::/apis/api-id) for trust aws:SourceArn.
// Cognito trigger PassRole is checkCognitoPassRole in cognito_handlers.go (pool ARN SourceArn).
func (s *Server) checkAPIGatewayPassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	return s.checkEdgePassRole(verified, roleARN, sourceARN, authz.ServicePrincipalAPIGateway, "API Gateway")
}

func (s *Server) checkEdgePassRole(verified *authn.Verified, roleARN, sourceARN, servicePrincipal, label string) error {
	return s.checkServicePassRole(verified, roleARN, sourceARN, servicePrincipal, label, passRoleMsgs{})
}

// checkServicePassRole is the shared PassRole path for configure APIs.
func (s *Server) checkServicePassRole(verified *authn.Verified, roleARN, sourceARN, servicePrincipal, label string, msgs passRoleMsgs) error {
	msgs = msgs.withDefaults()
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New(msgs.InvalidARN)
	}
	if accountID != verified.AccountID {
		return errors.New(msgs.WrongAccount)
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New(msgs.NotFound)
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to " + label)
	}
	decision := authz.CheckPassRole(authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal:     verified.Principal,
			Resource:      roleARN,
			Region:        verified.Region,
			ConditionKeys: s.conditionKeys(verified),
		},
		EvalInputs:       in,
		RoleARN:          roleARN,
		TrustPolicyDoc:   trust,
		ServicePrincipal: servicePrincipal,
		SourceArn:        sourceARN,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to " + label)
	}
	return nil
}
