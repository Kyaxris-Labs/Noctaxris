package server

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// cfnServerAuthorizer evaluates provision actions as the stack RoleARN when set,
// otherwise as the CreateStack / Cloud Control caller. PassRole uses the same
// effective principal (caller still PassRole-checked for CFN RoleARN at CreateStack).
type cfnServerAuthorizer struct {
	s         *Server
	effective *authn.Verified
}

func (s *Server) newCFNAuthorizer(caller *authn.Verified, stackRoleARN string) (store.CFNAuthorizer, error) {
	effective := caller
	if strings.TrimSpace(stackRoleARN) != "" {
		asRole, err := s.verifiedAsCFNStackRole(caller, stackRoleARN)
		if err != nil {
			return nil, err
		}
		effective = asRole
	}
	return &cfnServerAuthorizer{s: s, effective: effective}, nil
}

func (s *Server) verifiedAsCFNStackRole(caller *authn.Verified, roleARN string) (*authn.Verified, error) {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return nil, errors.New("RoleARN must be a valid IAM role ARN")
	}
	if accountID != caller.AccountID {
		return nil, errors.New("RoleARN must be in the same account")
	}
	if _, _, err := s.store.GetRole(accountID, roleName); err != nil {
		return nil, errors.New("Role not found")
	}
	out := *caller
	out.Principal = identity.RoleSessionPrincipal(accountID, roleName, "cloudformation", caller.AccessKeyID)
	return &out, nil
}

func (a *cfnServerAuthorizer) AuthorizeAction(action, resource string) error {
	if !a.s.authorize(a.effective, action, resource) {
		return fmt.Errorf("not authorized to perform %s", action)
	}
	return nil
}

func (a *cfnServerAuthorizer) AuthorizePassRole(roleARN, servicePrincipal, sourceARN string) error {
	switch servicePrincipal {
	case authz.ServicePrincipalLambda:
		return a.s.checkLambdaPassRole(a.effective, roleARN, sourceARN)
	case authz.ServicePrincipalEvents:
		return a.s.checkEventsPassRole(a.effective, roleARN, sourceARN)
	default:
		return fmt.Errorf("unsupported PassRole service principal %q", servicePrincipal)
	}
}

func cfnCapabilitiesFromParams(params url.Values) []string {
	var out []string
	for i := 1; ; i++ {
		v := strings.TrimSpace(params.Get(fmt.Sprintf("Capabilities.member.%d", i)))
		if v == "" {
			break
		}
		out = append(out, v)
	}
	return out
}
