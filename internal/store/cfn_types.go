package store

import (
	"fmt"
	"strings"
)

func supportedCFNType(t string) bool {
	switch t {
	case "AWS::S3::Bucket",
		"AWS::S3::BucketPolicy",
		"AWS::IAM::Role",
		"AWS::IAM::User",
		"AWS::IAM::Group",
		"AWS::IAM::ManagedPolicy",
		"AWS::IAM::Policy",
		"AWS::SQS::Queue",
		"AWS::SQS::QueuePolicy",
		"AWS::DynamoDB::Table",
		"AWS::Lambda::Function",
		"AWS::Lambda::Permission",
		"AWS::KMS::Key",
		"AWS::KMS::Alias",
		"AWS::SNS::Topic",
		"AWS::SNS::TopicPolicy",
		"AWS::SNS::Subscription",
		"AWS::Logs::LogGroup",
		"AWS::Events::EventBus",
		"AWS::Events::Rule",
		"AWS::SSM::Parameter",
		"AWS::SecretsManager::Secret",
		"AWS::CloudFormation::Stack":
		return true
	default:
		return false
	}
}

func cloudControlSupportedCFNType(t string) bool {
	switch t {
	case "AWS::CloudFormation::Stack",
		"AWS::SQS::QueuePolicy",
		"AWS::SNS::TopicPolicy",
		"AWS::SNS::Subscription",
		"AWS::S3::BucketPolicy",
		"AWS::Lambda::Permission",
		"AWS::IAM::Policy":
		return false
	default:
		return supportedCFNType(t)
	}
}

func cfnAllowedProps(resType string) map[string]struct{} {
	list := map[string][]string{
		"AWS::S3::Bucket":             {"BucketName", "BucketEncryption", "Tags", "NotificationConfiguration"},
		"AWS::S3::BucketPolicy":       {"Bucket", "PolicyDocument"},
		"AWS::IAM::Role":              {"RoleName", "AssumeRolePolicyDocument", "Policies", "ManagedPolicyArns", "MaxSessionDuration", "Description", "Path", "Tags"},
		"AWS::IAM::User":              {"UserName", "Path", "ManagedPolicyArns", "Groups", "Policies", "Tags"},
		"AWS::IAM::Group":             {"GroupName", "Path", "ManagedPolicyArns", "Policies", "Tags"},
		"AWS::IAM::ManagedPolicy":     {"ManagedPolicyName", "Path", "Description", "PolicyDocument", "Roles", "Users", "Groups"},
		"AWS::IAM::Policy":            {"PolicyName", "PolicyDocument", "Roles", "Users", "Groups"},
		"AWS::SQS::Queue":             {"QueueName", "DelaySeconds", "VisibilityTimeout", "MessageRetentionPeriod", "ReceiveMessageWaitTimeSeconds", "FifoQueue", "ContentBasedDeduplication", "KmsMasterKeyId", "Tags"},
		"AWS::SQS::QueuePolicy":       {"Queues", "PolicyDocument"},
		"AWS::DynamoDB::Table":        {"TableName", "BillingMode", "AttributeDefinitions", "KeySchema", "SSESpecification", "Tags"},
		"AWS::Lambda::Function":       {"FunctionName", "Role", "Runtime", "Handler", "Code", "Timeout", "MemorySize", "Description", "Environment", "Tags"},
		"AWS::Lambda::Permission":     {"FunctionName", "Action", "Principal", "SourceArn", "SourceAccount", "StatementId", "FunctionUrlAuthType", "InvokedViaFunctionUrl"},
		"AWS::KMS::Key":               {"Description", "KeyPolicy", "EnableKeyRotation", "PendingWindowInDays", "Tags"},
		"AWS::KMS::Alias":             {"AliasName", "TargetKeyId"},
		"AWS::SNS::Topic":             {"TopicName", "DisplayName", "KmsMasterKeyId", "Tags"},
		"AWS::SNS::TopicPolicy":       {"Topics", "PolicyDocument"},
		"AWS::SNS::Subscription":      {"TopicArn", "Protocol", "Endpoint", "FilterPolicy", "FilterPolicyScope", "RawMessageDelivery", "DeliveryPolicy", "RedrivePolicy"},
		"AWS::Logs::LogGroup":         {"LogGroupName", "RetentionInDays", "Tags"},
		"AWS::Events::EventBus":       {"Name", "Policy", "Tags"},
		// ScheduleExpression is deferred (use Scheduler); listing it would silently no-op.
		"AWS::Events::Rule":           {"Name", "EventBusName", "EventPattern", "State", "Description", "Targets"},
		"AWS::SSM::Parameter":         {"Name", "Type", "Value", "Description", "KeyId", "Tier", "Tags"},
		"AWS::SecretsManager::Secret": {"Name", "Description", "SecretString", "KmsKeyId", "Tags"},
		"AWS::CloudFormation::Stack":  {"TemplateURL", "Parameters", "TimeoutInMinutes", "Tags"},
	}
	allowed, ok := list[resType]
	if !ok {
		return nil
	}
	out := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		out[k] = struct{}{}
	}
	return out
}

// validateCFNProperties fails closed on unknown property keys for supported types.
func validateCFNProperties(logicalID, resType string, props map[string]any) error {
	if props == nil {
		return nil
	}
	allowed := cfnAllowedProps(resType)
	if allowed == nil {
		return fmt.Errorf("%w: unsupported resource type %q for %s", ErrCFNBadTemplate, resType, logicalID)
	}
	for key := range props {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("%w: unsupported property %q on %s (%s)", ErrCFNBadTemplate, key, logicalID, resType)
		}
	}
	return nil
}