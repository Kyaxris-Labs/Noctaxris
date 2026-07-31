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

type cfnResourceMeta struct {
	Allowed   []string
	Patchable []string // Cloud Control UpdateResource patch keys; empty => no CC patch surface
	Immutable []string // properties that cannot change on CFN stack Modify
}

var cfnResourceMetas = map[string]cfnResourceMeta{
	"AWS::S3::Bucket": {
		Allowed:   []string{"BucketName", "BucketEncryption", "Tags", "NotificationConfiguration"},
		Patchable: []string{"BucketEncryption", "NotificationConfiguration"},
		Immutable: []string{"BucketName"},
	},
	"AWS::S3::BucketPolicy": {
		Allowed:   []string{"Bucket", "PolicyDocument"},
		Immutable: []string{"Bucket"},
	},
	"AWS::IAM::Role": {
		Allowed:   []string{"RoleName", "AssumeRolePolicyDocument", "Policies", "ManagedPolicyArns", "MaxSessionDuration", "Description", "Path", "Tags"},
		Patchable: []string{"AssumeRolePolicyDocument", "Policies", "ManagedPolicyArns"},
		Immutable: []string{"RoleName", "Path"},
	},
	"AWS::IAM::User": {
		Allowed:   []string{"UserName", "Path", "ManagedPolicyArns", "Groups", "Policies", "Tags"},
		Patchable: []string{"Policies", "ManagedPolicyArns", "Groups"},
	},
	"AWS::IAM::Group": {
		Allowed:   []string{"GroupName", "Path", "ManagedPolicyArns", "Policies", "Tags"},
		Patchable: []string{"Policies", "ManagedPolicyArns"},
	},
	"AWS::IAM::ManagedPolicy": {
		Allowed:   []string{"ManagedPolicyName", "Path", "Description", "PolicyDocument", "Roles", "Users", "Groups"},
		Patchable: []string{"PolicyDocument", "Roles", "Users", "Groups", "Description"},
		Immutable: []string{"ManagedPolicyName", "PolicyName", "Path"},
	},
	"AWS::IAM::Policy": {
		Allowed: []string{"PolicyName", "PolicyDocument", "Roles", "Users", "Groups"},
	},
	"AWS::SQS::Queue": {
		Allowed:   []string{"QueueName", "DelaySeconds", "VisibilityTimeout", "MessageRetentionPeriod", "ReceiveMessageWaitTimeSeconds", "FifoQueue", "ContentBasedDeduplication", "KmsMasterKeyId", "Tags"},
		Patchable: []string{"VisibilityTimeout", "MessageRetentionPeriod", "DelaySeconds", "ReceiveMessageWaitTimeSeconds"},
		Immutable: []string{"QueueName", "FifoQueue"},
	},
	"AWS::SQS::QueuePolicy": {
		Allowed: []string{"Queues", "PolicyDocument"},
	},
	"AWS::DynamoDB::Table": {
		Allowed:   []string{"TableName", "BillingMode", "AttributeDefinitions", "KeySchema", "SSESpecification", "Tags"},
		Patchable: []string{"SSESpecification"},
		Immutable: []string{"TableName", "KeySchema", "AttributeDefinitions", "BillingMode"},
	},
	"AWS::Lambda::Function": {
		Allowed:   []string{"FunctionName", "Role", "Runtime", "Handler", "Code", "Timeout", "MemorySize", "Description", "Environment", "Tags"},
		Patchable: []string{"Timeout", "MemorySize", "Environment", "Handler", "Runtime"},
		Immutable: []string{"FunctionName"},
	},
	"AWS::Lambda::Permission": {
		Allowed: []string{"FunctionName", "Action", "Principal", "SourceArn", "SourceAccount", "StatementId", "FunctionUrlAuthType", "InvokedViaFunctionUrl"},
	},
	"AWS::KMS::Key": {
		Allowed:   []string{"Description", "KeyPolicy", "EnableKeyRotation", "PendingWindowInDays", "Tags"},
		Patchable: []string{"Description", "KeyPolicy", "EnableKeyRotation"},
	},
	"AWS::KMS::Alias": {
		Allowed:   []string{"AliasName", "TargetKeyId"},
		Patchable: []string{"TargetKeyId"},
		Immutable: []string{"AliasName"},
	},
	"AWS::SNS::Topic": {
		Allowed:   []string{"TopicName", "DisplayName", "KmsMasterKeyId", "Tags"},
		Patchable: []string{"DisplayName", "KmsMasterKeyId"},
		Immutable: []string{"TopicName"},
	},
	"AWS::SNS::TopicPolicy": {
		Allowed: []string{"Topics", "PolicyDocument"},
	},
	"AWS::SNS::Subscription": {
		Allowed: []string{"TopicArn", "Protocol", "Endpoint", "FilterPolicy", "FilterPolicyScope", "RawMessageDelivery", "DeliveryPolicy", "RedrivePolicy"},
	},
	"AWS::Logs::LogGroup": {
		Allowed:   []string{"LogGroupName", "RetentionInDays", "Tags"},
		Patchable: []string{"RetentionInDays"},
		Immutable: []string{"LogGroupName"},
	},
	"AWS::Events::EventBus": {
		Allowed:   []string{"Name", "Policy", "Tags"},
		Patchable: []string{"Policy", "Tags"},
		Immutable: []string{"Name"},
	},
	// ScheduleExpression is deferred (use Scheduler); listing it would silently no-op.
	"AWS::Events::Rule": {
		Allowed:   []string{"Name", "EventBusName", "EventPattern", "State", "Description", "Targets"},
		Patchable: []string{"EventPattern", "State", "Description", "EventBusName"},
		Immutable: []string{"Name", "EventBusName"},
	},
	"AWS::SSM::Parameter": {
		Allowed:   []string{"Name", "Type", "Value", "Description", "KeyId", "Tier", "Tags"},
		Patchable: []string{"Value", "Type", "KeyId"},
		Immutable: []string{"Name"},
	},
	"AWS::SecretsManager::Secret": {
		Allowed:   []string{"Name", "Description", "SecretString", "KmsKeyId", "Tags"},
		Patchable: []string{"Description", "KmsKeyId"},
		Immutable: []string{"Name"},
	},
	"AWS::CloudFormation::Stack": {
		Allowed: []string{"TemplateURL", "Parameters", "TimeoutInMinutes", "Tags"},
	},
}

func cfnAllowedProps(resType string) map[string]struct{} {
	meta, ok := cfnResourceMetas[resType]
	if !ok {
		return nil
	}
	out := make(map[string]struct{}, len(meta.Allowed))
	for _, k := range meta.Allowed {
		out[k] = struct{}{}
	}
	return out
}

func cfnRejectCloudControlPatchKeys(resType string, patch map[string]any) error {
	meta := cfnResourceMetas[resType]
	return cloudControlRejectUnknownPatchKeys(patch, meta.Patchable...)
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
