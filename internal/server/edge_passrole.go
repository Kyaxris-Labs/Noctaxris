package server

import (
	"errors"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

// checkAPIGatewayPassRole enforces iam:PassRole plus apigateway.amazonaws.com trust
// when Gateway configure APIs supply RoleArn (integrations / credentials).
func (s *Server) checkAPIGatewayPassRole(verified *authn.Verified, roleARN string) error {
	return s.checkEdgePassRole(verified, roleARN, authz.ServicePrincipalAPIGateway, "API Gateway")
}

// checkCognitoPassRole enforces iam:PassRole plus cognito-idp.amazonaws.com trust
// when Cognito configure APIs supply RoleArn.
func (s *Server) checkCognitoPassRole(verified *authn.Verified, roleARN string) error {
	return s.checkEdgePassRole(verified, roleARN, authz.ServicePrincipalCognitoIDP, "Cognito")
}

func (s *Server) checkEdgePassRole(verified *authn.Verified, roleARN, servicePrincipal, label string) error {
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
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to " + label)
	}
	return nil
}
