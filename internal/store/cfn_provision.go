package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Store) deleteCFNPhysical(accountID string, res CFNStackResource) {
	switch res.ResourceType {
	case "AWS::S3::Bucket":
		_ = s.DeleteBucket(accountID, res.PhysicalID)
	case "AWS::S3::BucketPolicy":
		bucket := strings.TrimSuffix(res.PhysicalID, "#BucketPolicy")
		_ = s.DeleteBucketPolicy(accountID, bucket)
	case "AWS::IAM::Role":
		if _, name, ok := parseIAMRoleARN(res.PhysicalID); ok {
			_ = s.DeleteRole(accountID, name)
		} else {
			_ = s.DeleteRole(accountID, res.PhysicalID)
		}
	case "AWS::IAM::User":
		_ = s.DeleteUser(accountID, res.PhysicalID)
	case "AWS::IAM::Group":
		_ = s.DeleteGroup(accountID, res.PhysicalID)
	case "AWS::IAM::ManagedPolicy", "AWS::IAM::Policy":
		_ = s.clearManagedPolicyAttachments(res.PhysicalID)
		_ = s.DeleteManagedPolicy(res.PhysicalID)
	case "AWS::SQS::Queue":
		name := res.PhysicalID
		if q, err := s.GetQueueByURL(res.PhysicalID); err == nil {
			name = q.QueueName
		}
		_ = s.DeleteQueue(accountID, name)
	case "AWS::SQS::QueuePolicy":
		queueURL := strings.TrimSuffix(res.PhysicalID, "#QueuePolicy")
		if q, err := s.GetQueueByURL(queueURL); err == nil {
			_ = s.SetQueueAttributes(accountID, q.QueueName, map[string]string{"Policy": ""})
		}
	case "AWS::DynamoDB::Table":
		_ = s.DeleteTable(accountID, res.PhysicalID)
	case "AWS::Lambda::Function":
		_ = s.DeleteFunction(accountID, res.PhysicalID)
	case "AWS::Lambda::Permission":
		if fn, sid, ok := splitCFNLambdaPermissionPhysical(res.PhysicalID); ok {
			_ = s.RemoveFunctionPermission(accountID, fn, sid)
		}
	case "AWS::KMS::Key":
		_, _ = s.ScheduleKeyDeletion(res.PhysicalID, 7)
	case "AWS::KMS::Alias":
		_ = s.DeleteAlias(accountID, res.PhysicalID)
	case "AWS::SNS::Topic":
		name := res.PhysicalID
		if t, err := s.GetTopicByARN(res.PhysicalID); err == nil {
			name = t.TopicName
		}
		_ = s.DeleteTopic(accountID, name)
	case "AWS::SNS::TopicPolicy":
		topicARN := strings.TrimSuffix(res.PhysicalID, "#TopicPolicy")
		if t, err := s.GetTopicByARN(topicARN); err == nil {
			_ = s.SetTopicAttributes(accountID, t.TopicName, map[string]string{"Policy": ""})
		}
	case "AWS::SNS::Subscription":
		_ = s.Unsubscribe(res.PhysicalID)
	case "AWS::Logs::LogGroup":
		_ = s.DeleteLogGroup(accountID, res.PhysicalID)
	case "AWS::Events::EventBus":
		_ = s.DeleteEventBus(accountID, res.PhysicalID)
	case "AWS::Events::Rule":
		if bus, rule, ok := splitCFNEventRulePhysical(res.PhysicalID); ok {
			_ = s.DeleteRule(accountID, bus, rule)
		}
	case "AWS::SSM::Parameter":
		_ = s.DeleteParameter(accountID, res.PhysicalID)
	case "AWS::SecretsManager::Secret":
		_ = s.DeleteSecret(accountID, res.PhysicalID)
	case "AWS::CloudFormation::Stack":
		_ = s.DeleteCFNStack(accountID, res.PhysicalID)
	}
}

func (s *Store) provisionCFNResource(accountID, region, stackName, parentStackID, logicalID, resType string, props map[string]any, auth cfnProvisionAuth) (physicalID string, attrs map[string]string, err error) {
	if props == nil {
		props = map[string]any{}
	}
	if region == "" {
		region = DefaultCFNRegion
	}
	switch resType {
	case "AWS::S3::Bucket":
		if err := s.cfnAuthorizeAction(auth, "s3:CreateBucket", "*"); err != nil {
			return "", nil, err
		}
		name := cfnStringProp(props, "BucketName")
		if name == "" {
			name = strings.ToLower(strings.ReplaceAll(logicalID, " ", "-")) + "-" + shortID()
		}
		b, err := s.CreateBucket(accountID, name)
		if err != nil {
			return "", nil, fmt.Errorf("%w: S3 bucket %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		if err := s.applyCFNS3Encryption(accountID, b.Name, props); err != nil {
			_ = s.DeleteBucket(accountID, b.Name)
			return "", nil, err
		}
		if err := s.applyCFNS3NotificationConfiguration(accountID, b.Name, props); err != nil {
			_ = s.DeleteBucket(accountID, b.Name)
			return "", nil, err
		}
		arn := fmt.Sprintf("arn:aws:s3:::%s", b.Name)
		return b.Name, map[string]string{
			"Ref": b.Name, "Arn": arn, "DomainName": b.Name + ".s3.amazonaws.com",
		}, nil
	case "AWS::S3::BucketPolicy":
		return s.provisionCFNBucketPolicy(accountID, logicalID, props, auth)
	case "AWS::IAM::Role":
		if err := s.cfnAuthorizeAction(auth, "iam:CreateRole", "*"); err != nil {
			return "", nil, err
		}
		name := cfnStringProp(props, "RoleName")
		if name == "" {
			name = logicalID + shortID()
		}
		trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
		if doc, ok := props["AssumeRolePolicyDocument"]; ok {
			raw, err := json.Marshal(doc)
			if err != nil {
				return "", nil, fmt.Errorf("%w: Role trust for %s", ErrCFNBadTemplate, logicalID)
			}
			trust = string(raw)
		}
		arn, err := s.CreateRole(accountID, name, trust)
		if err != nil {
			return "", nil, fmt.Errorf("%w: IAM role %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		if err := s.applyCFNRolePolicies(accountID, arn, name, props, auth); err != nil {
			_ = s.DeleteRole(accountID, name)
			return "", nil, err
		}
		return arn, map[string]string{"Ref": name, "Arn": arn}, nil
	case "AWS::IAM::User":
		return s.provisionCFNIAMUser(accountID, logicalID, props, auth)
	case "AWS::IAM::Group":
		return s.provisionCFNIAMGroup(accountID, logicalID, props, auth)
	case "AWS::IAM::ManagedPolicy":
		return s.provisionCFNManagedPolicy(accountID, logicalID, props, auth)
	case "AWS::IAM::Policy":
		return s.provisionCFNIAMPolicy(accountID, logicalID, props, auth)
	case "AWS::SQS::Queue":
		if err := s.cfnAuthorizeAction(auth, "sqs:CreateQueue", "*"); err != nil {
			return "", nil, err
		}
		name := cfnStringProp(props, "QueueName")
		if name == "" {
			name = strings.ToLower(strings.ReplaceAll(logicalID, " ", "-")) + "-" + shortID()
		}
		q, err := s.CreateQueue(accountID, region, "127.0.0.1:4566", name, cfnSQSQueueAttributes(props))
		if err != nil {
			return "", nil, fmt.Errorf("%w: SQS queue %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		return q.QueueURL, map[string]string{
			"Ref": q.QueueURL, "Arn": q.QueueARN, "QueueName": q.QueueName, "QueueUrl": q.QueueURL,
		}, nil
	case "AWS::SQS::QueuePolicy":
		return s.applyCFNQueuePolicy(accountID, props, auth)
	case "AWS::DynamoDB::Table":
		if err := s.cfnAuthorizeAction(auth, "dynamodb:CreateTable", "*"); err != nil {
			return "", nil, err
		}
		name := cfnStringProp(props, "TableName")
		if name == "" {
			name = strings.ToLower(strings.ReplaceAll(logicalID, " ", "-")) + "-" + shortID()
		}
		hashName, hashType, rangeName, rangeType, keyErr := cfnDynamoKeys(props)
		if keyErr != nil {
			return "", nil, fmt.Errorf("%w: DynamoDB table %s: %v", ErrCFNBadTemplate, logicalID, keyErr)
		}
		table, err := s.CreateTable(accountID, region, name, hashName, hashType, rangeName, rangeType, "", "", nil)
		if err != nil {
			return "", nil, fmt.Errorf("%w: DynamoDB table %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		if err := s.applyCFNDynamoSSE(accountID, table.TableName, props); err != nil {
			_ = s.DeleteTable(accountID, table.TableName)
			return "", nil, err
		}
		return table.TableName, map[string]string{
			"Ref": table.TableName, "Arn": table.TableARN,
		}, nil
	case "AWS::Lambda::Function":
		fnName := cfnStringProp(props, "FunctionName")
		if fnName == "" {
			fnName = logicalID + shortID()
		}
		role := cfnStringProp(props, "Role")
		if role == "" {
			return "", nil, fmt.Errorf("%w: Lambda Role required for %s", ErrCFNBadTemplate, logicalID)
		}
		if !strings.HasPrefix(role, "arn:aws:iam::") {
			role = fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, role)
		}
		fnARN := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", region, accountID, fnName)
		if err := s.cfnAuthorizeAction(auth, "lambda:CreateFunction", fnARN); err != nil {
			return "", nil, err
		}
		if err := s.cfnAuthorizePassRole(auth, role, cfnSvcPrincipalLambda, fnARN); err != nil {
			return "", nil, err
		}
		runtime := cfnStringProp(props, "Runtime")
		if runtime == "" {
			runtime = LambdaRuntimePython312
		}
		handler := cfnStringProp(props, "Handler")
		if handler == "" {
			handler = "index.handler"
		}
		zipBytes, zipErr := cfnLambdaZip(props, handler)
		if zipErr != nil {
			return "", nil, fmt.Errorf("%w: Lambda code %s: %v", ErrCFNBadTemplate, logicalID, zipErr)
		}
		timeout := cfnIntProp(props, "Timeout", 3)
		memory := cfnIntProp(props, "MemorySize", 128)
		fn, err := s.CreateFunction(CreateFunctionMeta{
			AccountID: accountID, Region: region, FunctionName: fnName, RoleARN: role,
			Runtime: runtime, Handler: handler, Timeout: timeout, Memory: memory, Zip: zipBytes,
		})
		if err != nil {
			return "", nil, fmt.Errorf("%w: Lambda function %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		return fn.FunctionName, map[string]string{
			"Ref": fn.FunctionARN, "Arn": fn.FunctionARN,
		}, nil
	case "AWS::Lambda::Permission":
		return s.provisionCFNLambdaPermission(accountID, logicalID, props, auth)
	case "AWS::KMS::Key":
		if err := s.cfnAuthorizeAction(auth, "kms:CreateKey", "*"); err != nil {
			return "", nil, err
		}
		creator := fmt.Sprintf("arn:aws:iam::%s:root", accountID)
		policy := ""
		if doc, ok := props["KeyPolicy"]; ok && doc != nil {
			raw, mErr := json.Marshal(doc)
			if mErr != nil {
				return "", nil, fmt.Errorf("%w: KeyPolicy for %s", ErrCFNBadTemplate, logicalID)
			}
			policy = string(raw)
		}
		key, err := s.CreateKey(accountID, creator, policy)
		if err != nil {
			return "", nil, fmt.Errorf("%w: KMS key %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		if strings.EqualFold(cfnStringProp(props, "EnableKeyRotation"), "true") {
			_ = s.SetKeyRotationEnabled(key.KeyID, true)
		}
		return key.KeyID, map[string]string{"Ref": key.KeyID, "Arn": key.ARN}, nil
	case "AWS::KMS::Alias":
		return s.provisionCFNKMSAlias(accountID, logicalID, props, auth)
	case "AWS::SNS::Topic":
		if err := s.cfnAuthorizeAction(auth, "sns:CreateTopic", "*"); err != nil {
			return "", nil, err
		}
		name := cfnStringProp(props, "TopicName")
		if name == "" {
			name = strings.ToLower(strings.ReplaceAll(logicalID, " ", "-")) + "-" + shortID()
		}
		attrsIn := map[string]string{}
		if d := cfnStringProp(props, "DisplayName"); d != "" {
			attrsIn["DisplayName"] = d
		}
		if k := cfnStringProp(props, "KmsMasterKeyId"); k != "" {
			attrsIn["KmsMasterKeyId"] = k
		}
		topic, err := s.CreateTopic(accountID, region, name, attrsIn)
		if err != nil {
			return "", nil, fmt.Errorf("%w: SNS topic %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		return topic.TopicARN, map[string]string{"Ref": topic.TopicARN, "TopicName": topic.TopicName}, nil
	case "AWS::SNS::TopicPolicy":
		return s.applyCFNTopicPolicy(accountID, props, auth)
	case "AWS::SNS::Subscription":
		return s.provisionCFNSNSSubscription(accountID, logicalID, props, auth)
	case "AWS::Logs::LogGroup":
		return s.provisionCFNLogGroup(accountID, region, logicalID, props, auth)
	case "AWS::Events::EventBus":
		if err := s.cfnAuthorizeAction(auth, "events:CreateEventBus", "*"); err != nil {
			return "", nil, err
		}
		name := cfnStringProp(props, "Name")
		if name == "" {
			name = logicalID
		}
		bus, err := s.CreateEventBus(accountID, region, name)
		if err != nil {
			return "", nil, fmt.Errorf("%w: EventBus %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		if raw, ok := props["Policy"]; ok && raw != nil {
			policyJSON, pErr := cfnPolicyDocumentJSON(raw)
			if pErr != nil {
				_ = s.DeleteEventBus(accountID, name)
				return "", nil, fmt.Errorf("%w: EventBus Policy for %s: %v", ErrCFNBadTemplate, logicalID, pErr)
			}
			if err := s.PutEventBusPolicy(accountID, name, policyJSON); err != nil {
				_ = s.DeleteEventBus(accountID, name)
				return "", nil, fmt.Errorf("%w: EventBus %s Policy: %v", ErrCFNBadTemplate, logicalID, err)
			}
		}
		return bus.Name, map[string]string{"Ref": bus.Name, "Arn": bus.ARN, "Name": bus.Name}, nil
	case "AWS::Events::Rule":
		return s.provisionCFNEventRule(accountID, region, logicalID, props, auth)
	case "AWS::SSM::Parameter":
		if err := s.cfnAuthorizeAction(auth, "ssm:PutParameter", "*"); err != nil {
			return "", nil, err
		}
		name := cfnStringProp(props, "Name")
		if name == "" {
			name = "/" + logicalID
		}
		ptype := cfnStringProp(props, "Type")
		if ptype == "" {
			ptype = ParamTypeString
		}
		value := cfnStringProp(props, "Value")
		if value == "" {
			return "", nil, fmt.Errorf("%w: SSM Parameter Value required for %s", ErrCFNBadTemplate, logicalID)
		}
		keyID := cfnStringProp(props, "KeyId")
		p, err := s.PutParameter(accountID, region, name, ptype, value, keyID, true)
		if err != nil {
			return "", nil, fmt.Errorf("%w: SSM Parameter %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		return p.Name, map[string]string{"Ref": p.Name, "Type": p.Type}, nil
	case "AWS::SecretsManager::Secret":
		if err := s.cfnAuthorizeAction(auth, "secretsmanager:CreateSecret", "*"); err != nil {
			return "", nil, err
		}
		name := cfnStringProp(props, "Name")
		if name == "" {
			name = logicalID + "-" + shortID()
		}
		secretString := cfnStringProp(props, "SecretString")
		kmsKey := cfnStringProp(props, "KmsKeyId")
		desc := cfnStringProp(props, "Description")
		sec, err := s.CreateSecret(accountID, region, name, secretString, nil, kmsKey, desc, "")
		if err != nil {
			return "", nil, fmt.Errorf("%w: Secret %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		return sec.Name, map[string]string{"Ref": sec.ARN, "Arn": sec.ARN, "Name": sec.Name}, nil
	case "AWS::CloudFormation::Stack":
		_ = stackName
		return s.provisionCFNNestedStack(accountID, region, parentStackID, logicalID, props, auth)
	default:
		return "", nil, fmt.Errorf("%w: unsupported resource type %q", ErrCFNBadTemplate, resType)
	}
}
