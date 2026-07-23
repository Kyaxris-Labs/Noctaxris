package server

import (
	"errors"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

// checkAPIGatewayPassRole enforces iam:PassRole plus apigateway.amazonaws.com trust
// when Gateway configure APIs supply CredentialsArn. sourceARN is the HTTP API ARN
// (arn:aws:apigateway:region::/apis/api-id) for trust aws:SourceArn.
// Cognito trigger PassRole is checkCognitoPassRole in cognito_handlers.go (pool ARN SourceArn).
func (s *Server) checkAPIGatewayPassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	return s.checkEdgePassRole(verified, roleARN, sourceARN, authz.ServicePrincipalAPIGateway, "API Gateway")
}

func (s *Server) checkEdgePassRole(verified *authn.Verified, roleARN, sourceARN, servicePrincipal, label string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("roleARN must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("roleARN must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("Role not found")
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
