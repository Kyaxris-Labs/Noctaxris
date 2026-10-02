package server

import (
	"errors"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
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
	return s.checkPassRoleForService(verified, roleARN, sourceARN, servicePrincipal, label)
}

// checkPassRoleForService is the thin Server wrapper around authz.CheckPassRole
// for configure APIs that pass a role to a service principal.
func (s *Server) checkPassRoleForService(verified *authn.Verified, roleARN, sourceARN, servicePrincipal, label string) error {
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

// resolveInstanceProfileRoleARN maps an IamInstanceProfile name/ARN (or role ARN)
// to the IAM role ARN used for configure-time PassRole.
func (s *Server) resolveInstanceProfileRoleARN(accountID, profile string) (string, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return "", nil
	}
	if _, _, ok := sts.ParseRoleARN(profile); ok {
		return profile, nil
	}
	name := profile
	const ipMarker = ":instance-profile/"
	if i := strings.Index(profile, ipMarker); i >= 0 {
		name = profile[i+len(ipMarker):]
		if j := strings.LastIndex(name, "/"); j >= 0 {
			name = name[j+1:]
		}
	}
	p, err := s.store.GetInstanceProfile(accountID, name)
	if err == nil {
		if strings.TrimSpace(p.RoleARN) == "" {
			return "", errors.New("instance profile has no role")
		}
		return p.RoleARN, nil
	}
	// Lab: when no instance-profile row exists, the profile name is the role name.
	return store.RoleARN(accountID, name), nil
}

func (s *Server) checkEC2InstanceProfilePassRole(verified *authn.Verified, profile string) error {
	roleARN, err := s.resolveInstanceProfileRoleARN(verified.AccountID, profile)
	if err != nil {
		return err
	}
	if roleARN == "" {
		return nil
	}
	return s.checkServicePassRole(verified, roleARN, "", authz.ServicePrincipalEC2, "EC2", passRoleMsgs{
		InvalidARN:   "IamInstanceProfile must resolve to a valid IAM role ARN",
		WrongAccount: "IamInstanceProfile role must be in the same account",
		NotFound:     "Role not found for IamInstanceProfile",
	})
}

func (s *Server) checkASGLifecyclePassRole(verified *authn.Verified, roleARN string) error {
	return s.checkServicePassRole(verified, roleARN, "", authz.ServicePrincipalAutoScaling, "Auto Scaling", passRoleMsgs{})
}

func (s *Server) checkBackupPassRole(verified *authn.Verified, roleARN string) error {
	return s.checkServicePassRole(verified, roleARN, "", authz.ServicePrincipalBackup, "Backup", passRoleMsgs{})
}

func (s *Server) checkEKSPassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	return s.checkServicePassRole(verified, roleARN, sourceARN, authz.ServicePrincipalEKS, "EKS", passRoleMsgs{})
}

func (s *Server) checkEMRPassRole(verified *authn.Verified, roleARN string) error {
	return s.checkServicePassRole(verified, roleARN, "", authz.ServicePrincipalEMR, "EMR", passRoleMsgs{})
}

func (s *Server) checkVPCFlowPassRole(verified *authn.Verified, roleARN string) error {
	return s.checkServicePassRole(verified, roleARN, "", authz.ServicePrincipalVPCFlowLogs, "VPC Flow Logs", passRoleMsgs{})
}
