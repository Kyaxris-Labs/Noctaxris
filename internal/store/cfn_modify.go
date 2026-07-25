package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// applyCFNModify updates an existing physical resource in place for allowlisted types.
// Unsupported type/property sets return ErrCFNBadTemplate (fail closed).
func (s *Store) applyCFNModify(accountID, region, stackName, stackID, logicalID, resType, physicalID string, oldProps, newProps map[string]any, auth cfnProvisionAuth) error {
	_ = stackName
	_ = stackID
	if newProps == nil {
		newProps = map[string]any{}
	}
	if oldProps == nil {
		oldProps = map[string]any{}
	}
	if region == "" {
		region = DefaultCFNRegion
	}
	switch resType {
	case "AWS::SSM::Parameter":
		return s.modifyCFNSSMParameter(accountID, region, physicalID, oldProps, newProps)
	case "AWS::S3::Bucket":
		return s.modifyCFNS3Bucket(accountID, physicalID, oldProps, newProps)
	case "AWS::S3::BucketPolicy":
		return s.modifyCFNBucketPolicy(accountID, physicalID, oldProps, newProps, auth)
	case "AWS::IAM::Role":
		return s.modifyCFNIAMRole(accountID, physicalID, oldProps, newProps, auth)
	case "AWS::IAM::ManagedPolicy", "AWS::IAM::Policy":
		return s.modifyCFNManagedPolicyDocument(accountID, physicalID, oldProps, newProps, auth)
	case "AWS::SQS::Queue":
		return s.modifyCFNSQSQueue(accountID, physicalID, oldProps, newProps)
	case "AWS::SQS::QueuePolicy":
		return s.modifyCFNQueuePolicyResource(accountID, physicalID, oldProps, newProps, auth)
	case "AWS::SNS::Topic":
		return s.modifyCFNSNSTopic(accountID, physicalID, oldProps, newProps)
	case "AWS::SNS::TopicPolicy":
		return s.modifyCFNTopicPolicyResource(accountID, physicalID, oldProps, newProps, auth)
	case "AWS::Lambda::Function":
		return s.modifyCFNLambdaFunction(accountID, region, physicalID, oldProps, newProps, auth)
	case "AWS::Lambda::Permission":
		return s.modifyCFNLambdaPermission(accountID, physicalID, logicalID, oldProps, newProps, auth)
	case "AWS::Events::Rule":
		return s.modifyCFNEventRule(accountID, region, physicalID, oldProps, newProps, auth)
	case "AWS::SecretsManager::Secret":
		return s.modifyCFNSecret(accountID, physicalID, oldProps, newProps)
	case "AWS::DynamoDB::Table":
		return s.modifyCFNDynamoTable(accountID, physicalID, oldProps, newProps)
	case "AWS::KMS::Alias":
		return s.modifyCFNKMSAlias(accountID, physicalID, oldProps, newProps)
	case "AWS::KMS::Key":
		return s.modifyCFNKMSKey(physicalID, oldProps, newProps)
	case "AWS::Logs::LogGroup":
		return s.modifyCFNLogGroup(accountID, physicalID, oldProps, newProps)
	case "AWS::Events::EventBus":
		return s.modifyCFNEventBus(accountID, physicalID, oldProps, newProps)
	default:
		return fmt.Errorf("%w: Modify not supported for type %s (fail-closed); logical id %s", ErrCFNBadTemplate, resType, logicalID)
	}
}

func cfnPropJSONEqual(a, b map[string]any, key string) bool {
	ra, _ := json.Marshal(a[key])
	rb, _ := json.Marshal(b[key])
	return string(ra) == string(rb)
}

func cfnRejectImmutablePropChange(resType, logicalID string, oldProps, newProps map[string]any, keys ...string) error {
	for _, key := range keys {
		if _, ok := newProps[key]; !ok {
			continue
		}
		if !cfnPropJSONEqual(oldProps, newProps, key) {
			return fmt.Errorf("%w: unsupported Modify of immutable property %q on %s (%s); fail-closed", ErrCFNBadTemplate, key, logicalID, resType)
		}
	}
	return nil
}

func (s *Store) modifyCFNSSMParameter(accountID, region, physicalID string, oldProps, newProps map[string]any) error {
	if err := cfnRejectImmutablePropChange("AWS::SSM::Parameter", physicalID, oldProps, newProps, "Name"); err != nil {
		return err
	}
	name := cfnStringProp(newProps, "Name")
	if name == "" {
		name = physicalID
	}
	if name != physicalID {
		return fmt.Errorf("%w: SSM Parameter Name cannot change on Modify (fail-closed)", ErrCFNBadTemplate)
	}
	value := cfnStringProp(newProps, "Value")
	if value == "" {
		return fmt.Errorf("%w: SSM Parameter Value required on Modify", ErrCFNBadTemplate)
	}
	ptype := cfnStringProp(newProps, "Type")
	if ptype == "" {
		ptype = ParamTypeString
	}
	_, err := s.PutParameter(accountID, region, name, ptype, value, cfnStringProp(newProps, "KeyId"), true)
	if err != nil {
		return fmt.Errorf("%w: SSM Parameter Modify: %v", ErrCFNBadTemplate, err)
	}
	return nil
}

func (s *Store) modifyCFNS3Bucket(accountID, physicalID string, oldProps, newProps map[string]any) error {
	if err := cfnRejectImmutablePropChange("AWS::S3::Bucket", physicalID, oldProps, newProps, "BucketName"); err != nil {
		return err
	}
	name := cfnStringProp(newProps, "BucketName")
	if name == "" {
		name = physicalID
	}
	if name != physicalID {
		return fmt.Errorf("%w: S3 BucketName cannot change on Modify (fail-closed)", ErrCFNBadTemplate)
	}
	if err := s.applyCFNS3Encryption(accountID, physicalID, newProps); err != nil {
		return err
	}
	if err := s.applyCFNS3NotificationConfiguration(accountID, physicalID, newProps); err != nil {
		return err
	}
	return nil
}

func (s *Store) modifyCFNBucketPolicy(accountID, physicalID string, oldProps, newProps map[string]any, auth cfnProvisionAuth) error {
	if err := cfnRejectImmutablePropChange("AWS::S3::BucketPolicy", physicalID, oldProps, newProps, "Bucket"); err != nil {
		return err
	}
	_, _, err := s.provisionCFNBucketPolicy(accountID, "BucketPolicy", newProps, auth)
	return err
}

func (s *Store) modifyCFNIAMRole(accountID, physicalID string, oldProps, newProps map[string]any, auth cfnProvisionAuth) error {
	if err := cfnRejectImmutablePropChange("AWS::IAM::Role", physicalID, oldProps, newProps, "RoleName", "Path"); err != nil {
		return err
	}
	roleName := cfnStringProp(newProps, "RoleName")
	roleARN := physicalID
	if _, name, ok := parseIAMRoleARN(physicalID); ok {
		roleName = name
		roleARN = physicalID
	} else if roleName != "" {
		roleARN = fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)
	} else {
		roleName = physicalID
		roleARN = fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)
	}
	if doc, ok := newProps["AssumeRolePolicyDocument"]; ok && doc != nil {
		if err := s.cfnAuthorizeAction(auth, "iam:UpdateAssumeRolePolicy", "*"); err != nil {
			return err
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("%w: Role trust on Modify", ErrCFNBadTemplate)
		}
		if err := s.UpdateAssumeRolePolicy(accountID, roleName, string(raw)); err != nil {
			return fmt.Errorf("%w: UpdateAssumeRolePolicy: %v", ErrCFNBadTemplate, err)
		}
	}
	return s.replaceCFNRolePolicies(accountID, roleARN, roleName, newProps, auth)
}

func (s *Store) replaceCFNPrincipalPolicies(
	accountID, principalARN, entityName string,
	props map[string]any,
	auth cfnProvisionAuth,
	putAction, deleteAction, attachAction, detachAction string,
	attachManaged func(accountID, name, policyARN string) error,
	detachManaged func(accountID, name, policyARN string) error,
) error {
	wantInline := map[string]string{}
	if raw, ok := props["Policies"].([]any); ok {
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name := cfnStringProp(m, "PolicyName")
			if name == "" {
				return fmt.Errorf("%w: Policies.PolicyName required", ErrCFNBadTemplate)
			}
			docRaw, ok := m["PolicyDocument"]
			if !ok {
				return fmt.Errorf("%w: Policies.PolicyDocument required", ErrCFNBadTemplate)
			}
			docBytes, err := json.Marshal(docRaw)
			if err != nil {
				return fmt.Errorf("%w: Policies.PolicyDocument", ErrCFNBadTemplate)
			}
			wantInline[name] = string(docBytes)
		}
	}
	if _, has := props["Policies"]; has {
		existing, err := s.ListInlinePolicies(principalARN)
		if err != nil {
			return fmt.Errorf("%w: list inline policies: %v", ErrCFNBadTemplate, err)
		}
		for _, p := range existing {
			if _, ok := wantInline[p.PolicyName]; !ok {
				if err := s.cfnAuthorizeAction(auth, deleteAction, principalARN); err != nil {
					return err
				}
				if err := s.DeleteInlinePolicy(principalARN, p.PolicyName); err != nil {
					return fmt.Errorf("%w: delete inline policy: %v", ErrCFNBadTemplate, err)
				}
			}
		}
		for name, doc := range wantInline {
			if err := s.cfnAuthorizeAction(auth, putAction, principalARN); err != nil {
				return err
			}
			if err := s.PutInlinePolicy(principalARN, name, doc); err != nil {
				return fmt.Errorf("%w: put inline policy: %v", ErrCFNBadTemplate, err)
			}
		}
	}
	if _, has := props["ManagedPolicyArns"]; has {
		want := map[string]struct{}{}
		for _, arn := range cfnStringListProp(props, "ManagedPolicyArns") {
			want[arn] = struct{}{}
		}
		attached, err := s.ListAttachedPolicyRefs(principalARN)
		if err != nil {
			return fmt.Errorf("%w: list attached policies: %v", ErrCFNBadTemplate, err)
		}
		for _, ref := range attached {
			if _, ok := want[ref.PolicyARN]; !ok {
				if err := s.cfnAuthorizeAction(auth, detachAction, principalARN); err != nil {
					return err
				}
				if err := detachManaged(accountID, entityName, ref.PolicyARN); err != nil {
					return fmt.Errorf("%w: DetachPolicy: %v", ErrCFNBadTemplate, err)
				}
			}
		}
		for arn := range want {
			if err := s.cfnAuthorizeAction(auth, attachAction, principalARN); err != nil {
				return err
			}
			if err := attachManaged(accountID, entityName, arn); err != nil {
				return fmt.Errorf("%w: AttachPolicy: %v", ErrCFNBadTemplate, err)
			}
		}
	}
	return nil
}

func (s *Store) replaceCFNUserGroups(accountID, userName string, props map[string]any, auth cfnProvisionAuth) error {
	want := map[string]struct{}{}
	for _, g := range cfnStringListProp(props, "Groups") {
		name := cfnIAMEntityName(g)
		if name != "" {
			want[name] = struct{}{}
		}
	}
	current, err := s.ListGroupsForUser(accountID, userName)
	if err != nil {
		return fmt.Errorf("%w: ListGroupsForUser: %v", ErrCFNBadTemplate, err)
	}
	for _, g := range current {
		if _, ok := want[g.GroupName]; !ok {
			if err := s.cfnAuthorizeAction(auth, "iam:RemoveUserFromGroup", "*"); err != nil {
				return err
			}
			if err := s.RemoveUserFromGroup(accountID, g.GroupName, userName); err != nil {
				return fmt.Errorf("%w: RemoveUserFromGroup: %v", ErrCFNBadTemplate, err)
			}
		}
	}
	for name := range want {
		if err := s.cfnAuthorizeAction(auth, "iam:AddUserToGroup", "*"); err != nil {
			return err
		}
		if err := s.AddUserToGroup(accountID, name, userName); err != nil {
			return fmt.Errorf("%w: AddUserToGroup: %v", ErrCFNBadTemplate, err)
		}
	}
	return nil
}

func (s *Store) replaceCFNRolePolicies(accountID, roleARN, roleName string, props map[string]any, auth cfnProvisionAuth) error {
	return s.replaceCFNPrincipalPolicies(
		accountID, roleARN, roleName, props, auth,
		"iam:PutRolePolicy", "iam:DeleteRolePolicy", "iam:AttachRolePolicy", "iam:DetachRolePolicy",
		s.AttachRolePolicy, s.DetachRolePolicy,
	)
}

func (s *Store) modifyCFNManagedPolicyDocument(accountID, physicalID string, oldProps, newProps map[string]any, auth cfnProvisionAuth) error {
	if err := s.cfnAuthorizeAction(auth, "iam:CreatePolicyVersion", "*"); err != nil {
		return err
	}
	_ = accountID
	if err := cfnRejectImmutablePropChange("AWS::IAM::ManagedPolicy", physicalID, oldProps, newProps, "ManagedPolicyName", "PolicyName", "Path"); err != nil {
		return err
	}
	docRaw, ok := newProps["PolicyDocument"]
	if !ok || docRaw == nil {
		return fmt.Errorf("%w: PolicyDocument required on Modify", ErrCFNBadTemplate)
	}
	docBytes, err := json.Marshal(docRaw)
	if err != nil {
		return fmt.Errorf("%w: PolicyDocument on Modify", ErrCFNBadTemplate)
	}
	if err := s.replaceManagedPolicyDocument(physicalID, string(docBytes)); err != nil {
		return fmt.Errorf("%w: %v", ErrCFNBadTemplate, err)
	}
	_ = s.clearManagedPolicyAttachments(physicalID)
	return s.attachCFNManagedPolicyTargets(accountID, physicalID, newProps, auth)
}

func (s *Store) replaceManagedPolicyDocument(policyARN, document string) error {
	res, err := s.db.Exec(`UPDATE managed_policies SET document = ? WHERE policy_arn = ?`, document, policyARN)
	if err != nil {
		return fmt.Errorf("update managed policy document: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("managed policy %s not found", policyARN)
	}
	_, err = s.db.Exec(`UPDATE policies SET document = ? WHERE policy_id = ?`, document, policyARN)
	if err != nil {
		return fmt.Errorf("sync managed policy document: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE managed_policy_versions SET document = ? WHERE policy_arn = ? AND is_default = 1`,
		document, policyARN,
	)
	if err != nil {
		return fmt.Errorf("update managed policy default version: %w", err)
	}
	return nil
}

func (s *Store) modifyCFNSQSQueue(accountID, physicalID string, oldProps, newProps map[string]any) error {
	if err := cfnRejectImmutablePropChange("AWS::SQS::Queue", physicalID, oldProps, newProps, "QueueName", "FifoQueue"); err != nil {
		return err
	}
	q, err := s.GetQueueByURL(physicalID)
	if err != nil {
		q, err = s.GetQueue(accountID, physicalID)
		if err != nil {
			return fmt.Errorf("%w: SQS queue Modify lookup: %v", ErrCFNBadTemplate, err)
		}
	}
	attrs := cfnSQSQueueAttributes(newProps)
	if len(attrs) == 0 {
		return nil
	}
	delete(attrs, "FifoQueue")
	if err := s.SetQueueAttributes(accountID, q.QueueName, attrs); err != nil {
		return fmt.Errorf("%w: SetQueueAttributes: %v", ErrCFNBadTemplate, err)
	}
	return nil
}

func (s *Store) modifyCFNQueuePolicyResource(accountID, physicalID string, oldProps, newProps map[string]any, auth cfnProvisionAuth) error {
	_ = oldProps
	_ = physicalID
	_, _, err := s.applyCFNQueuePolicy(accountID, newProps, auth)
	return err
}

func (s *Store) modifyCFNSNSTopic(accountID, physicalID string, oldProps, newProps map[string]any) error {
	if err := cfnRejectImmutablePropChange("AWS::SNS::Topic", physicalID, oldProps, newProps, "TopicName"); err != nil {
		return err
	}
	topic, err := s.GetTopicByARN(physicalID)
	if err != nil {
		topic, err = s.GetTopic(accountID, physicalID)
		if err != nil {
			return fmt.Errorf("%w: SNS topic Modify lookup: %v", ErrCFNBadTemplate, err)
		}
	}
	attrs := map[string]string{}
	if d := cfnStringProp(newProps, "DisplayName"); d != "" || newProps["DisplayName"] != nil {
		attrs["DisplayName"] = cfnStringProp(newProps, "DisplayName")
	}
	if k := cfnStringProp(newProps, "KmsMasterKeyId"); k != "" || newProps["KmsMasterKeyId"] != nil {
		attrs["KmsMasterKeyId"] = cfnStringProp(newProps, "KmsMasterKeyId")
	}
	if len(attrs) == 0 {
		return nil
	}
	if err := s.SetTopicAttributes(accountID, topic.TopicName, attrs); err != nil {
		return fmt.Errorf("%w: SetTopicAttributes: %v", ErrCFNBadTemplate, err)
	}
	return nil
}

func (s *Store) modifyCFNTopicPolicyResource(accountID, physicalID string, oldProps, newProps map[string]any, auth cfnProvisionAuth) error {
	_ = oldProps
	_ = physicalID
	_, _, err := s.applyCFNTopicPolicy(accountID, newProps, auth)
	return err
}

func cfnLambdaEnvVars(props map[string]any) (map[string]string, bool) {
	raw, ok := props["Environment"]
	if !ok || raw == nil {
		return nil, false
	}
	envMap, ok := raw.(map[string]any)
	if !ok {
		return nil, true
	}
	varsRaw, ok := envMap["Variables"].(map[string]any)
	if !ok {
		return map[string]string{}, true
	}
	out := make(map[string]string, len(varsRaw))
	for k, v := range varsRaw {
		out[k] = strings.TrimSpace(fmt.Sprint(v))
	}
	return out, true
}

func (s *Store) modifyCFNLambdaFunction(accountID, region, physicalID string, oldProps, newProps map[string]any, auth cfnProvisionAuth) error {
	if err := cfnRejectImmutablePropChange("AWS::Lambda::Function", physicalID, oldProps, newProps, "FunctionName"); err != nil {
		return err
	}
	fn, err := s.GetFunction(accountID, physicalID)
	if err != nil {
		return fmt.Errorf("%w: Lambda function Modify lookup: %v", ErrCFNBadTemplate, err)
	}
	meta := UpdateFunctionConfigurationMeta{
		RoleARN: fn.RoleARN,
		Timeout: fn.Timeout,
		Memory:  fn.Memory,
		Handler: fn.Handler,
		Env:     fn.Env,
		Runtime: fn.Runtime,
	}
	if role := cfnStringProp(newProps, "Role"); role != "" {
		if !strings.HasPrefix(role, "arn:aws:iam::") {
			role = fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, role)
		}
		meta.RoleARN = role
	}
	if err := s.cfnAuthorizeAction(auth, "lambda:UpdateFunctionConfiguration", fn.FunctionARN); err != nil {
		return err
	}
	if meta.RoleARN != "" && meta.RoleARN != fn.RoleARN {
		if err := s.cfnAuthorizePassRole(auth, meta.RoleARN, cfnSvcPrincipalLambda, fn.FunctionARN); err != nil {
			return err
		}
	}
	if _, ok := newProps["Timeout"]; ok {
		meta.Timeout = cfnIntProp(newProps, "Timeout", fn.Timeout)
	}
	if _, ok := newProps["MemorySize"]; ok {
		meta.Memory = cfnIntProp(newProps, "MemorySize", fn.Memory)
	}
	if h := cfnStringProp(newProps, "Handler"); h != "" {
		meta.Handler = h
	}
	if r := cfnStringProp(newProps, "Runtime"); r != "" {
		meta.Runtime = r
	}
	if env, ok := cfnLambdaEnvVars(newProps); ok {
		if env == nil {
			return fmt.Errorf("%w: Lambda Environment must be an object", ErrCFNBadTemplate)
		}
		meta.Env = env
	}
	if _, err := s.UpdateFunctionConfiguration(accountID, physicalID, meta); err != nil {
		return fmt.Errorf("%w: UpdateFunctionConfiguration: %v", ErrCFNBadTemplate, err)
	}
	if !cfnPropJSONEqual(oldProps, newProps, "Code") {
		zipBytes, zipErr := cfnLambdaZip(newProps, meta.Handler)
		if zipErr != nil {
			return fmt.Errorf("%w: Lambda Code Modify: %v", ErrCFNBadTemplate, zipErr)
		}
		if _, err := s.UpdateFunctionCode(accountID, physicalID, zipBytes); err != nil {
			return fmt.Errorf("%w: UpdateFunctionCode: %v", ErrCFNBadTemplate, err)
		}
	}
	_ = region
	return nil
}

func (s *Store) modifyCFNLambdaPermission(accountID, physicalID, logicalID string, oldProps, newProps map[string]any, auth cfnProvisionAuth) error {
	_ = oldProps
	if fn, sid, ok := splitCFNLambdaPermissionPhysical(physicalID); ok {
		_ = s.RemoveFunctionPermission(accountID, fn, sid)
	}
	_, _, err := s.provisionCFNLambdaPermission(accountID, logicalID, newProps, auth)
	return err
}

func (s *Store) modifyCFNEventRule(accountID, region, physicalID string, oldProps, newProps map[string]any, auth cfnProvisionAuth) error {
	if err := cfnRejectImmutablePropChange("AWS::Events::Rule", physicalID, oldProps, newProps, "Name", "EventBusName"); err != nil {
		return err
	}
	bus, rule, ok := splitCFNEventRulePhysical(physicalID)
	if !ok {
		rule = cfnStringProp(newProps, "Name")
		bus = cfnStringProp(newProps, "EventBusName")
	}
	props := map[string]any{}
	for k, v := range newProps {
		props[k] = v
	}
	if cfnStringProp(props, "Name") == "" && rule != "" {
		props["Name"] = rule
	}
	if cfnStringProp(props, "EventBusName") == "" && bus != "" {
		props["EventBusName"] = bus
	}
	_, _, err := s.provisionCFNEventRule(accountID, region, rule, props, auth)
	return err
}

func (s *Store) modifyCFNSecret(accountID, physicalID string, oldProps, newProps map[string]any) error {
	if err := cfnRejectImmutablePropChange("AWS::SecretsManager::Secret", physicalID, oldProps, newProps, "Name"); err != nil {
		return err
	}
	if _, ok := newProps["SecretString"]; ok && !cfnPropJSONEqual(oldProps, newProps, "SecretString") {
		return fmt.Errorf("%w: SecretString changes on Modify are not supported (use Secrets Manager APIs); fail-closed", ErrCFNBadTemplate)
	}
	name := physicalID
	if n := cfnStringProp(newProps, "Name"); n != "" {
		name = n
	}
	desc := cfnStringProp(newProps, "Description")
	if _, ok := newProps["Description"]; !ok {
		if sec, err := s.DescribeSecret(accountID, name); err == nil {
			desc = sec.Description
		}
	}
	kms := cfnStringProp(newProps, "KmsKeyId")
	if _, ok := newProps["KmsKeyId"]; !ok {
		if sec, err := s.DescribeSecret(accountID, name); err == nil {
			kms = sec.KmsKeyID
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE secretsmanager_secrets SET description = ?, kms_key_id = ?, last_changed_date = ?
		 WHERE account_id = ? AND name = ?`,
		desc, kms, now, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("%w: Secret metadata Modify: %v", ErrCFNBadTemplate, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: Secret %s not found", ErrCFNBadTemplate, name)
	}
	return nil
}

func (s *Store) modifyCFNDynamoTable(accountID, physicalID string, oldProps, newProps map[string]any) error {
	if err := cfnRejectImmutablePropChange("AWS::DynamoDB::Table", physicalID, oldProps, newProps,
		"TableName", "KeySchema", "AttributeDefinitions", "BillingMode"); err != nil {
		return err
	}
	name := cfnStringProp(newProps, "TableName")
	if name == "" {
		name = physicalID
	}
	if name != physicalID {
		return fmt.Errorf("%w: DynamoDB TableName cannot change on Modify (fail-closed)", ErrCFNBadTemplate)
	}
	return s.applyCFNDynamoSSE(accountID, physicalID, newProps)
}

func (s *Store) modifyCFNKMSAlias(accountID, physicalID string, oldProps, newProps map[string]any) error {
	if err := cfnRejectImmutablePropChange("AWS::KMS::Alias", physicalID, oldProps, newProps, "AliasName"); err != nil {
		return err
	}
	aliasName := cfnStringProp(newProps, "AliasName")
	if aliasName == "" {
		aliasName = physicalID
	}
	target := cfnStringProp(newProps, "TargetKeyId")
	if target == "" {
		return fmt.Errorf("%w: Alias TargetKeyId required on Modify", ErrCFNBadTemplate)
	}
	if err := s.UpdateAlias(accountID, aliasName, target); err != nil {
		return fmt.Errorf("%w: UpdateAlias: %v", ErrCFNBadTemplate, err)
	}
	return nil
}

func (s *Store) modifyCFNKMSKey(physicalID string, oldProps, newProps map[string]any) error {
	_ = oldProps
	if doc, ok := newProps["KeyPolicy"]; ok && doc != nil {
		raw, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("%w: KeyPolicy on Modify", ErrCFNBadTemplate)
		}
		if err := s.PutKeyPolicy(physicalID, string(raw)); err != nil {
			return fmt.Errorf("%w: PutKeyPolicy: %v", ErrCFNBadTemplate, err)
		}
	}
	if _, ok := newProps["EnableKeyRotation"]; ok {
		enabled := strings.EqualFold(cfnStringProp(newProps, "EnableKeyRotation"), "true")
		if b, ok := newProps["EnableKeyRotation"].(bool); ok {
			enabled = b
		}
		if err := s.SetKeyRotationEnabled(physicalID, enabled); err != nil {
			return fmt.Errorf("%w: SetKeyRotationEnabled: %v", ErrCFNBadTemplate, err)
		}
	}
	// Description is accepted and ignored (lab Key row has no description column).
	return nil
}

func (s *Store) modifyCFNLogGroup(accountID, physicalID string, oldProps, newProps map[string]any) error {
	if err := cfnRejectImmutablePropChange("AWS::Logs::LogGroup", physicalID, oldProps, newProps, "LogGroupName"); err != nil {
		return err
	}
	name := cfnStringProp(newProps, "LogGroupName")
	if name != "" && name != physicalID {
		return fmt.Errorf("%w: LogGroupName cannot change on Modify (fail-closed)", ErrCFNBadTemplate)
	}
	if _, ok := newProps["RetentionInDays"]; ok {
		days := cfnIntProp(newProps, "RetentionInDays", 0)
		if days <= 0 {
			return s.DeleteRetentionPolicy(accountID, physicalID)
		}
		return s.PutRetentionPolicy(accountID, physicalID, days)
	}
	return nil
}

func (s *Store) modifyCFNEventBus(accountID, physicalID string, oldProps, newProps map[string]any) error {
	if err := cfnRejectImmutablePropChange("AWS::Events::EventBus", physicalID, oldProps, newProps, "Name"); err != nil {
		return err
	}
	for key := range newProps {
		if key == "Name" || key == "Tags" || key == "Policy" {
			continue
		}
		if !cfnPropJSONEqual(oldProps, newProps, key) {
			return fmt.Errorf("%w: EventBus Modify property %q is not supported (fail-closed)", ErrCFNBadTemplate, key)
		}
	}
	if raw, ok := newProps["Policy"]; ok {
		if raw == nil {
			return s.PutEventBusPolicy(accountID, physicalID, "")
		}
		policyJSON, err := cfnPolicyDocumentJSON(raw)
		if err != nil {
			return fmt.Errorf("%w: EventBus Policy on Modify: %v", ErrCFNBadTemplate, err)
		}
		return s.PutEventBusPolicy(accountID, physicalID, policyJSON)
	}
	return nil
}
