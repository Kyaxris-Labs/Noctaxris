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

// cfnAuthorizeDeletePhysical evaluates the underlying delete (and PassRole where create does)
// for a tracked CFN / Cloud Control physical resource. Fail closed when authz is set.
func (s *Store) cfnAuthorizeDeletePhysical(auth cfnProvisionAuth, accountID, region string, res CFNStackResource) error {
	if auth.Authorizer == nil {
		return nil
	}
	if region == "" {
		region = DefaultCFNRegion
	}
	switch res.ResourceType {
	case "AWS::S3::Bucket":
		return s.cfnAuthorizeAction(auth, "s3:DeleteBucket", "*")
	case "AWS::S3::BucketPolicy":
		return s.cfnAuthorizeAction(auth, "s3:DeleteBucketPolicy", "*")
	case "AWS::IAM::Role":
		return s.cfnAuthorizeAction(auth, "iam:DeleteRole", "*")
	case "AWS::IAM::User":
		return s.cfnAuthorizeAction(auth, "iam:DeleteUser", "*")
	case "AWS::IAM::Group":
		return s.cfnAuthorizeAction(auth, "iam:DeleteGroup", "*")
	case "AWS::IAM::ManagedPolicy", "AWS::IAM::Policy":
		return s.cfnAuthorizeAction(auth, "iam:DeletePolicy", "*")
	case "AWS::SQS::Queue":
		return s.cfnAuthorizeAction(auth, "sqs:DeleteQueue", "*")
	case "AWS::SQS::QueuePolicy":
		return s.cfnAuthorizeAction(auth, "sqs:SetQueueAttributes", "*")
	case "AWS::DynamoDB::Table":
		return s.cfnAuthorizeAction(auth, "dynamodb:DeleteTable", "*")
	case "AWS::Lambda::Function":
		fnName := res.PhysicalID
		fnARN := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", region, accountID, fnName)
		if err := s.cfnAuthorizeAction(auth, "lambda:DeleteFunction", fnARN); err != nil {
			return err
		}
		if fn, err := s.GetFunction(accountID, fnName); err == nil && strings.TrimSpace(fn.RoleARN) != "" {
			return s.cfnAuthorizePassRole(auth, fn.RoleARN, cfnSvcPrincipalLambda, fnARN)
		}
		return nil
	case "AWS::Lambda::Permission":
		return s.cfnAuthorizeAction(auth, "lambda:RemovePermission", "*")
	case "AWS::KMS::Key":
		return s.cfnAuthorizeAction(auth, "kms:ScheduleKeyDeletion", "*")
	case "AWS::KMS::Alias":
		return s.cfnAuthorizeAction(auth, "kms:DeleteAlias", "*")
	case "AWS::SNS::Topic":
		return s.cfnAuthorizeAction(auth, "sns:DeleteTopic", "*")
	case "AWS::SNS::TopicPolicy":
		return s.cfnAuthorizeAction(auth, "sns:SetTopicAttributes", "*")
	case "AWS::SNS::Subscription":
		return s.cfnAuthorizeAction(auth, "sns:Unsubscribe", "*")
	case "AWS::Logs::LogGroup":
		return s.cfnAuthorizeAction(auth, "logs:DeleteLogGroup", "*")
	case "AWS::Events::EventBus":
		return s.cfnAuthorizeAction(auth, "events:DeleteEventBus", "*")
	case "AWS::Events::Rule":
		return s.cfnAuthorizeAction(auth, "events:DeleteRule", "*")
	case "AWS::SSM::Parameter":
		return s.cfnAuthorizeAction(auth, "ssm:DeleteParameter", "*")
	case "AWS::SecretsManager::Secret":
		return s.cfnAuthorizeAction(auth, "secretsmanager:DeleteSecret", "*")
	case "AWS::CloudFormation::Stack":
		return s.cfnAuthorizeAction(auth, "cloudformation:DeleteStack", "*")
	default:
		return fmt.Errorf("%w: unsupported delete type %q", ErrCFNAccessDenied, res.ResourceType)
	}
}
