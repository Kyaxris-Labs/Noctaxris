package store

import (
	"errors"
	"fmt"
	"strings"
)

// ErrCFNAccessDenied is returned when provision authz denies an underlying action.
var ErrCFNAccessDenied = errors.New("AccessDenied")

// ErrCFNInsufficientCapabilities is returned when IAM resources lack CAPABILITY_IAM / CAPABILITY_NAMED_IAM.
var ErrCFNInsufficientCapabilities = errors.New("InsufficientCapabilitiesException")

// CFNAuthorizer evaluates underlying service actions and PassRole during CFN / Cloud Control provision.
// When nil on a provision call, action/PassRole checks are skipped (store unit path); capabilities still apply.
type CFNAuthorizer interface {
	AuthorizeAction(action, resource string) error
	AuthorizePassRole(roleARN, servicePrincipal, sourceARN string) error
}

// cfnProvisionAuth carries request-scoped authz and capabilities through provision.
type cfnProvisionAuth struct {
	Authorizer   CFNAuthorizer
	Capabilities []string
}

const (
	cfnCapIAM      = "CAPABILITY_IAM"
	cfnCapNamedIAM = "CAPABILITY_NAMED_IAM"

	cfnSvcPrincipalLambda = "lambda.amazonaws.com"
	cfnSvcPrincipalEvents = "events.amazonaws.com"
)

func cfnHasIAMCapability(caps []string) bool {
	for _, c := range caps {
		switch strings.TrimSpace(c) {
		case cfnCapIAM, cfnCapNamedIAM:
			return true
		}
	}
	return false
}

func cfnHasNamedIAMCapability(caps []string) bool {
	for _, c := range caps {
		if strings.TrimSpace(c) == cfnCapNamedIAM {
			return true
		}
	}
	return false
}

func cfnIsIAMResourceType(resType string) bool {
	switch resType {
	case "AWS::IAM::Role", "AWS::IAM::User", "AWS::IAM::Group",
		"AWS::IAM::ManagedPolicy", "AWS::IAM::Policy":
		return true
	default:
		return false
	}
}

func cfnResourceHasCustomIAMName(resType string, props map[string]any) bool {
	switch resType {
	case "AWS::IAM::Role":
		return cfnStringProp(props, "RoleName") != ""
	case "AWS::IAM::User":
		return cfnStringProp(props, "UserName") != ""
	case "AWS::IAM::Group":
		return cfnStringProp(props, "GroupName") != ""
	case "AWS::IAM::ManagedPolicy":
		return cfnStringProp(props, "ManagedPolicyName") != ""
	case "AWS::IAM::Policy":
		return cfnStringProp(props, "PolicyName") != ""
	default:
		return false
	}
}

func (s *Store) requireCFNIAMCapabilities(tpl cfnTemplate, caps []string) error {
	needIAM := false
	needNamed := false
	for _, res := range tpl.Resources {
		if !cfnIsIAMResourceType(res.Type) {
			continue
		}
		needIAM = true
		if cfnResourceHasCustomIAMName(res.Type, res.Properties) {
			needNamed = true
		}
	}
	if !needIAM {
		return nil
	}
	if needNamed && !cfnHasNamedIAMCapability(caps) {
		return fmt.Errorf("%w: Requires capabilities : [%s]", ErrCFNInsufficientCapabilities, cfnCapNamedIAM)
	}
	if !cfnHasIAMCapability(caps) {
		return fmt.Errorf("%w: Requires capabilities : [%s]", ErrCFNInsufficientCapabilities, cfnCapIAM)
	}
	return nil
}

func (s *Store) cfnAuthorizeAction(auth cfnProvisionAuth, action, resource string) error {
	if auth.Authorizer == nil {
		return nil
	}
	if resource == "" {
		resource = "*"
	}
	if err := auth.Authorizer.AuthorizeAction(action, resource); err != nil {
		return fmt.Errorf("%w: %v", ErrCFNAccessDenied, err)
	}
	return nil
}

func (s *Store) cfnAuthorizePassRole(auth cfnProvisionAuth, roleARN, servicePrincipal, sourceARN string) error {
	if auth.Authorizer == nil {
		return nil
	}
	roleARN = strings.TrimSpace(roleARN)
	if roleARN == "" {
		return nil
	}
	if err := auth.Authorizer.AuthorizePassRole(roleARN, servicePrincipal, sourceARN); err != nil {
		return fmt.Errorf("%w: %v", ErrCFNAccessDenied, err)
	}
	return nil
}
