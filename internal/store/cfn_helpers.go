package store

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func (s *Store) applyCFNS3NotificationConfiguration(accountID, bucket string, props map[string]any) error {
	raw, ok := props["NotificationConfiguration"]
	if !ok || raw == nil {
		return nil
	}
	nc, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: NotificationConfiguration must be an object", ErrCFNBadTemplate)
	}
	cfg := S3NotificationConfig{}
	if ebRaw, ok := nc["EventBridgeConfiguration"]; ok && ebRaw != nil {
		switch v := ebRaw.(type) {
		case map[string]any:
			enabled := true
			if rawEnabled, has := v["EventBridgeEnabled"]; has {
				switch e := rawEnabled.(type) {
				case bool:
					enabled = e
				case string:
					enabled = strings.EqualFold(strings.TrimSpace(e), "true")
				default:
					enabled = true
				}
			}
			cfg.EventBridgeEnabled = enabled
		default:
			cfg.EventBridgeEnabled = true
		}
	}
	if arr, ok := nc["LambdaConfigurations"].([]any); ok {
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: LambdaConfigurations[%d] must be an object", ErrCFNBadTemplate, i)
			}
			fn := cfnStringProp(m, "Function")
			ev := cfnStringProp(m, "Event")
			if fn == "" || ev == "" {
				return fmt.Errorf("%w: LambdaConfigurations require Event and Function", ErrCFNBadTemplate)
			}
			prefix, suffix := cfnS3NotificationFilter(m)
			cfg.LambdaConfigs = append(cfg.LambdaConfigs, S3LambdaFunctionConfig{
				ID:           fmt.Sprintf("cfn-lambda-%d", i+1),
				Events:       []string{ev},
				FilterPrefix: prefix,
				FilterSuffix: suffix,
				FunctionARN:  fn,
			})
		}
	}
	if arr, ok := nc["QueueConfigurations"].([]any); ok {
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: QueueConfigurations[%d] must be an object", ErrCFNBadTemplate, i)
			}
			q := cfnStringProp(m, "Queue")
			ev := cfnStringProp(m, "Event")
			if q == "" || ev == "" {
				return fmt.Errorf("%w: QueueConfigurations require Event and Queue", ErrCFNBadTemplate)
			}
			prefix, suffix := cfnS3NotificationFilter(m)
			cfg.QueueConfigs = append(cfg.QueueConfigs, S3QueueConfig{
				ID:           fmt.Sprintf("cfn-queue-%d", i+1),
				Events:       []string{ev},
				FilterPrefix: prefix,
				FilterSuffix: suffix,
				QueueARN:     q,
			})
		}
	}
	if arr, ok := nc["TopicConfigurations"].([]any); ok {
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: TopicConfigurations[%d] must be an object", ErrCFNBadTemplate, i)
			}
			topic := cfnStringProp(m, "Topic")
			ev := cfnStringProp(m, "Event")
			if topic == "" || ev == "" {
				return fmt.Errorf("%w: TopicConfigurations require Event and Topic", ErrCFNBadTemplate)
			}
			prefix, suffix := cfnS3NotificationFilter(m)
			cfg.TopicConfigs = append(cfg.TopicConfigs, S3TopicConfig{
				ID:           fmt.Sprintf("cfn-topic-%d", i+1),
				Events:       []string{ev},
				FilterPrefix: prefix,
				FilterSuffix: suffix,
				TopicARN:     topic,
			})
		}
	}
	if isEmptyS3NotificationConfig(cfg) {
		return nil
	}
	if err := s.PutBucketNotificationConfiguration(accountID, bucket, cfg); err != nil {
		return fmt.Errorf("%w: NotificationConfiguration: %v", ErrCFNBadTemplate, err)
	}
	return nil
}

func cfnS3NotificationFilter(m map[string]any) (prefix, suffix string) {
	filter, _ := m["Filter"].(map[string]any)
	if filter == nil {
		return "", ""
	}
	s3key, _ := filter["S3Key"].(map[string]any)
	if s3key == nil {
		return "", ""
	}
	rules, _ := s3key["Rules"].([]any)
	for _, item := range rules {
		rule, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(cfnStringProp(rule, "Name")))
		value := cfnStringProp(rule, "Value")
		switch name {
		case "prefix":
			prefix = value
		case "suffix":
			suffix = value
		}
	}
	return prefix, suffix
}

func (s *Store) applyCFNS3Encryption(accountID, bucket string, props map[string]any) error {
	raw, ok := props["BucketEncryption"]
	if !ok || raw == nil {
		return nil
	}
	encMap, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: BucketEncryption must be an object", ErrCFNBadTemplate)
	}
	cfg, _ := encMap["ServerSideEncryptionConfiguration"].([]any)
	if len(cfg) == 0 {
		return nil
	}
	rule, _ := cfg[0].(map[string]any)
	if rule == nil {
		return nil
	}
	def, _ := rule["ServerSideEncryptionByDefault"].(map[string]any)
	if def == nil {
		return nil
	}
	algo := cfnStringProp(def, "SSEAlgorithm")
	if algo == "" {
		algo = "AES256"
	}
	kms := cfnStringProp(def, "KMSMasterKeyID")
	if kms == "" {
		kms = cfnStringProp(def, "KMSMasterKeyId")
	}
	return s.PutBucketEncryption(accountID, bucket, BucketEncryption{Algorithm: algo, KMSKeyID: kms})
}

func (s *Store) applyCFNRolePolicies(accountID, roleARN, roleName string, props map[string]any) error {
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
			if err := s.PutInlinePolicy(roleARN, name, string(docBytes)); err != nil {
				return fmt.Errorf("%w: put inline policy: %v", ErrCFNBadTemplate, err)
			}
		}
	}
	if raw, ok := props["ManagedPolicyArns"].([]any); ok {
		for _, item := range raw {
			arn := strings.TrimSpace(fmt.Sprint(item))
			if arn == "" {
				continue
			}
			if err := s.AttachRolePolicy(accountID, roleName, arn); err != nil {
				return fmt.Errorf("%w: AttachRolePolicy: %v", ErrCFNBadTemplate, err)
			}
		}
	}
	return nil
}

func (s *Store) applyCFNQueuePolicy(accountID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	queues, ok := props["Queues"].([]any)
	if !ok || len(queues) == 0 {
		return "", nil, fmt.Errorf("%w: QueuePolicy Queues required", ErrCFNBadTemplate)
	}
	queueRef := strings.TrimSpace(fmt.Sprint(queues[0]))
	if queueRef == "" {
		return "", nil, fmt.Errorf("%w: QueuePolicy Queues entry empty", ErrCFNBadTemplate)
	}
	q, err := s.GetQueueByURL(queueRef)
	if err != nil {
		q, err = s.GetQueue(accountID, queueRef)
		if err != nil {
			return "", nil, fmt.Errorf("%w: QueuePolicy queue %s: %v", ErrCFNBadTemplate, queueRef, err)
		}
	}
	docRaw, ok := props["PolicyDocument"]
	if !ok {
		return "", nil, fmt.Errorf("%w: QueuePolicy PolicyDocument required", ErrCFNBadTemplate)
	}
	docBytes, err := json.Marshal(docRaw)
	if err != nil {
		return "", nil, fmt.Errorf("%w: QueuePolicy PolicyDocument", ErrCFNBadTemplate)
	}
	if err := s.SetQueueAttributes(accountID, q.QueueName, map[string]string{"Policy": string(docBytes)}); err != nil {
		return "", nil, fmt.Errorf("%w: set queue policy: %v", ErrCFNBadTemplate, err)
	}
	phys := q.QueueURL + "#QueuePolicy"
	return phys, map[string]string{"Ref": phys, "QueueUrl": q.QueueURL}, nil
}

func (s *Store) applyCFNDynamoSSE(accountID, tableName string, props map[string]any) error {
	raw, ok := props["SSESpecification"]
	if !ok || raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: SSESpecification must be an object", ErrCFNBadTemplate)
	}
	enabled := false
	switch v := m["SSEEnabled"].(type) {
	case bool:
		enabled = v
	case string:
		enabled = strings.EqualFold(v, "true")
	}
	if !enabled {
		return nil
	}
	kms := cfnStringProp(m, "KMSMasterKeyId")
	if kms == "" {
		kms = cfnStringProp(m, "KMSMasterKeyID")
	}
	sseType := "KMS"
	if kms == "" {
		sseType = "AES256"
	}
	return s.UpdateTableSSE(accountID, tableName, sseType, kms)
}

func splitCFNEventRulePhysical(physicalID string) (busName, ruleName string, ok bool) {
	parts := strings.SplitN(physicalID, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func splitCFNLambdaPermissionPhysical(physicalID string) (functionName, statementID string, ok bool) {
	parts := strings.SplitN(physicalID, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (s *Store) provisionCFNEventRule(accountID, region, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	name := cfnStringProp(props, "Name")
	if name == "" {
		name = logicalID
	}
	busName := cfnStringProp(props, "EventBusName")
	if busName == "" {
		busName = "default"
	}
	pattern := ""
	if p, ok := props["EventPattern"]; ok && p != nil {
		switch t := p.(type) {
		case string:
			pattern = t
		default:
			raw, mErr := json.Marshal(t)
			if mErr != nil {
				return "", nil, fmt.Errorf("%w: EventPattern for %s", ErrCFNBadTemplate, logicalID)
			}
			pattern = string(raw)
		}
	}
	state := cfnStringProp(props, "State")
	if state == "" {
		state = "ENABLED"
	}
	desc := cfnStringProp(props, "Description")
	rule, err := s.PutRule(accountID, region, busName, name, pattern, desc, state)
	if err != nil {
		return "", nil, fmt.Errorf("%w: Events rule %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	if raw, ok := props["Targets"].([]any); ok && len(raw) > 0 {
		targets := make([]EventTargetInput, 0, len(raw))
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			targets = append(targets, EventTargetInput{
				ID:      cfnStringProp(m, "Id"),
				ARN:     cfnStringProp(m, "Arn"),
				RoleARN: cfnStringProp(m, "RoleArn"),
				Input:   cfnStringProp(m, "Input"),
			})
		}
		if err := s.PutTargets(accountID, busName, name, targets); err != nil {
			return "", nil, fmt.Errorf("%w: Events targets %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
	}
	phys := busName + "|" + name
	return phys, map[string]string{"Ref": rule.ARN, "Arn": rule.ARN}, nil
}

func parseLabCFNTemplateURL(raw string) (bucket, key string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("TemplateURL is required")
	}
	if strings.HasPrefix(strings.ToLower(raw), "s3://") {
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", "", fmt.Errorf("parse s3 TemplateURL: %w", perr)
		}
		bucket = u.Host
		key = strings.TrimPrefix(u.Path, "/")
		if bucket == "" || key == "" {
			return "", "", fmt.Errorf("s3 TemplateURL must be s3://bucket/key")
		}
		return bucket, key, nil
	}
	u, perr := url.Parse(raw)
	if perr != nil {
		return "", "", fmt.Errorf("parse TemplateURL: %w", perr)
	}
	host := strings.ToLower(u.Hostname())
	if host != "127.0.0.1" && host != "localhost" {
		return "", "", fmt.Errorf("TemplateURL host %q is not allowlisted (lab S3 loopback or s3:// only)", u.Hostname())
	}
	if u.Port() != "" && u.Port() != "4566" {
		return "", "", fmt.Errorf("TemplateURL port %q is not allowlisted", u.Port())
	}
	path := strings.TrimPrefix(u.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("path-style TemplateURL must be http://127.0.0.1:4566/bucket/key")
	}
	return parts[0], parts[1], nil
}

func (s *Store) resolveLabCFNTemplateURL(accountID, templateURL string) (string, error) {
	bucket, key, err := parseLabCFNTemplateURL(templateURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCFNBadTemplate, err)
	}
	_, data, err := s.GetObject(accountID, bucket, key)
	if err != nil {
		return "", fmt.Errorf("%w: TemplateURL object s3://%s/%s: %v", ErrCFNBadTemplate, bucket, key, err)
	}
	body := strings.TrimSpace(string(data))
	if body == "" {
		return "", fmt.Errorf("%w: TemplateURL object is empty", ErrCFNBadTemplate)
	}
	return body, nil
}

func (s *Store) provisionCFNNestedStack(accountID, region, parentStackID, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	templateURL := cfnStringProp(props, "TemplateURL")
	body, err := s.resolveLabCFNTemplateURL(accountID, templateURL)
	if err != nil {
		return "", nil, err
	}
	childName := strings.ToLower(strings.ReplaceAll(logicalID, " ", "-")) + "-" + shortID()
	st, err := s.createCFNStackWithParent(accountID, region, childName, body, "", parentStackID)
	if err != nil {
		return "", nil, fmt.Errorf("%w: nested stack %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	return st.StackID, map[string]string{
		"Ref": st.StackID, "Arn": st.StackID,
	}, nil
}

func cfnStringListProp(props map[string]any, key string) []string {
	raw, ok := props[key]
	if !ok || raw == nil {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s := strings.TrimSpace(fmt.Sprint(item))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func cfnIAMEntityName(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if _, name, ok := parseIAMRoleARN(ref); ok {
		return name
	}
	const prefix = "arn:aws:iam::"
	if strings.HasPrefix(ref, prefix) {
		rest := strings.TrimPrefix(ref, prefix)
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) == 2 {
			for _, kind := range []string{"user/", "group/", "role/"} {
				if strings.HasPrefix(parts[1], kind) {
					name := strings.TrimPrefix(parts[1], kind)
					if i := strings.LastIndex(name, "/"); i >= 0 {
						return name[i+1:]
					}
					return name
				}
			}
		}
	}
	return ref
}

func (s *Store) attachCFNManagedPolicyTargets(accountID, policyARN string, props map[string]any) error {
	for _, roleRef := range cfnStringListProp(props, "Roles") {
		name := cfnIAMEntityName(roleRef)
		if name == "" {
			continue
		}
		if err := s.AttachRolePolicy(accountID, name, policyARN); err != nil {
			return fmt.Errorf("%w: AttachRolePolicy %s: %v", ErrCFNBadTemplate, name, err)
		}
	}
	for _, userRef := range cfnStringListProp(props, "Users") {
		name := cfnIAMEntityName(userRef)
		if name == "" {
			continue
		}
		if err := s.AttachUserPolicy(accountID, name, policyARN); err != nil {
			return fmt.Errorf("%w: AttachUserPolicy %s: %v", ErrCFNBadTemplate, name, err)
		}
	}
	for _, groupRef := range cfnStringListProp(props, "Groups") {
		name := cfnIAMEntityName(groupRef)
		if name == "" {
			continue
		}
		if err := s.AttachGroupPolicy(accountID, name, policyARN); err != nil {
			return fmt.Errorf("%w: AttachGroupPolicy %s: %v", ErrCFNBadTemplate, name, err)
		}
	}
	return nil
}

func (s *Store) clearManagedPolicyAttachments(policyARN string) error {
	_, err := s.db.Exec(`DELETE FROM policy_attachments WHERE policy_id = ?`, policyARN)
	if err != nil {
		return fmt.Errorf("clear managed policy attachments %s: %w", policyARN, err)
	}
	return nil
}

func (s *Store) provisionCFNManagedPolicy(accountID, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	name := cfnStringProp(props, "ManagedPolicyName")
	if name == "" {
		name = logicalID + shortID()
	}
	path := cfnStringProp(props, "Path")
	if path != "" && path != "/" {
		return "", nil, fmt.Errorf("%w: ManagedPolicy Path other than / is not supported for %s", ErrCFNBadTemplate, logicalID)
	}
	docRaw, ok := props["PolicyDocument"]
	if !ok || docRaw == nil {
		return "", nil, fmt.Errorf("%w: ManagedPolicy PolicyDocument required for %s", ErrCFNBadTemplate, logicalID)
	}
	docBytes, err := json.Marshal(docRaw)
	if err != nil {
		return "", nil, fmt.Errorf("%w: ManagedPolicy PolicyDocument for %s", ErrCFNBadTemplate, logicalID)
	}
	arn, err := s.CreateManagedPolicy(accountID, name, string(docBytes))
	if err != nil {
		return "", nil, fmt.Errorf("%w: ManagedPolicy %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	if err := s.attachCFNManagedPolicyTargets(accountID, arn, props); err != nil {
		_ = s.clearManagedPolicyAttachments(arn)
		_ = s.DeleteManagedPolicy(arn)
		return "", nil, err
	}
	return arn, map[string]string{"Ref": arn, "Arn": arn}, nil
}

func (s *Store) provisionCFNIAMPolicy(accountID, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	name := cfnStringProp(props, "PolicyName")
	if name == "" {
		return "", nil, fmt.Errorf("%w: Policy PolicyName required for %s", ErrCFNBadTemplate, logicalID)
	}
	roles := cfnStringListProp(props, "Roles")
	users := cfnStringListProp(props, "Users")
	groups := cfnStringListProp(props, "Groups")
	if len(roles) == 0 && len(users) == 0 && len(groups) == 0 {
		return "", nil, fmt.Errorf("%w: Policy %s requires Roles, Users, or Groups", ErrCFNBadTemplate, logicalID)
	}
	docRaw, ok := props["PolicyDocument"]
	if !ok || docRaw == nil {
		return "", nil, fmt.Errorf("%w: Policy PolicyDocument required for %s", ErrCFNBadTemplate, logicalID)
	}
	docBytes, err := json.Marshal(docRaw)
	if err != nil {
		return "", nil, fmt.Errorf("%w: Policy PolicyDocument for %s", ErrCFNBadTemplate, logicalID)
	}
	// Lab maps AWS::IAM::Policy onto a customer-managed policy so Attach* APIs apply.
	arn, err := s.CreateManagedPolicy(accountID, name, string(docBytes))
	if err != nil {
		return "", nil, fmt.Errorf("%w: Policy %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	if err := s.attachCFNManagedPolicyTargets(accountID, arn, props); err != nil {
		_ = s.clearManagedPolicyAttachments(arn)
		_ = s.DeleteManagedPolicy(arn)
		return "", nil, err
	}
	return arn, map[string]string{"Ref": name, "Arn": arn, "Id": arn}, nil
}

func (s *Store) provisionCFNBucketPolicy(accountID, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	bucket := cfnStringProp(props, "Bucket")
	if bucket == "" {
		return "", nil, fmt.Errorf("%w: BucketPolicy Bucket required for %s", ErrCFNBadTemplate, logicalID)
	}
	docRaw, ok := props["PolicyDocument"]
	if !ok || docRaw == nil {
		return "", nil, fmt.Errorf("%w: BucketPolicy PolicyDocument required for %s", ErrCFNBadTemplate, logicalID)
	}
	docBytes, err := json.Marshal(docRaw)
	if err != nil {
		return "", nil, fmt.Errorf("%w: BucketPolicy PolicyDocument for %s", ErrCFNBadTemplate, logicalID)
	}
	if err := s.PutBucketPolicy(accountID, bucket, string(docBytes)); err != nil {
		return "", nil, fmt.Errorf("%w: BucketPolicy %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	phys := bucket + "#BucketPolicy"
	return phys, map[string]string{"Ref": phys, "Bucket": bucket}, nil
}

func (s *Store) provisionCFNLambdaPermission(accountID, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	fn := cfnStringProp(props, "FunctionName")
	if fn == "" {
		return "", nil, fmt.Errorf("%w: Lambda Permission FunctionName required for %s", ErrCFNBadTemplate, logicalID)
	}
	action := cfnStringProp(props, "Action")
	if action == "" {
		return "", nil, fmt.Errorf("%w: Lambda Permission Action required for %s", ErrCFNBadTemplate, logicalID)
	}
	principal := cfnStringProp(props, "Principal")
	if principal == "" {
		return "", nil, fmt.Errorf("%w: Lambda Permission Principal required for %s", ErrCFNBadTemplate, logicalID)
	}
	if principal == "*" {
		return "", nil, fmt.Errorf("%w: Lambda Permission wildcard Principal is not supported for %s", ErrCFNBadTemplate, logicalID)
	}
	sid := cfnStringProp(props, "StatementId")
	if sid == "" {
		sid = logicalID
	}
	sourceARN := cfnStringProp(props, "SourceArn")
	sourceAccount := cfnStringProp(props, "SourceAccount")
	if _, err := s.AddFunctionPermission(accountID, fn, sid, action, principal, sourceAccount, sourceARN); err != nil {
		return "", nil, fmt.Errorf("%w: Lambda Permission %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	fnName, err := resolveFunctionName(accountID, fn)
	if err != nil {
		_ = s.RemoveFunctionPermission(accountID, fn, sid)
		return "", nil, fmt.Errorf("%w: Lambda Permission %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	phys := fnName + "|" + sid
	return phys, map[string]string{"Ref": phys}, nil
}

func cfnSQSQueueAttributes(props map[string]any) map[string]string {
	attrs := map[string]string{}
	for _, key := range []string{
		"DelaySeconds", "VisibilityTimeout", "MessageRetentionPeriod", "ReceiveMessageWaitTimeSeconds",
	} {
		if _, ok := props[key]; !ok {
			continue
		}
		if s := cfnStringProp(props, key); s != "" {
			attrs[key] = s
			continue
		}
		attrs[key] = fmt.Sprintf("%d", cfnIntProp(props, key, 0))
	}
	if k := cfnStringProp(props, "KmsMasterKeyId"); k != "" {
		attrs["KmsMasterKeyId"] = k
	}
	if _, ok := props["FifoQueue"]; ok {
		attrs["FifoQueue"] = cfnPropTruthyString(props, "FifoQueue")
	}
	if _, ok := props["ContentBasedDeduplication"]; ok {
		attrs["ContentBasedDeduplication"] = cfnPropTruthyString(props, "ContentBasedDeduplication")
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func cfnPropTruthyString(props map[string]any, key string) string {
	raw, ok := props[key]
	if !ok || raw == nil {
		return "false"
	}
	switch v := raw.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	case string:
		if strings.EqualFold(strings.TrimSpace(v), "true") {
			return "true"
		}
		return "false"
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if strings.EqualFold(s, "true") || s == "1" {
			return "true"
		}
		return "false"
	}
}

func (s *Store) applyCFNTopicPolicy(accountID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	topics, ok := props["Topics"].([]any)
	if !ok || len(topics) == 0 {
		return "", nil, fmt.Errorf("%w: TopicPolicy Topics required", ErrCFNBadTemplate)
	}
	docRaw, ok := props["PolicyDocument"]
	if !ok {
		return "", nil, fmt.Errorf("%w: TopicPolicy PolicyDocument required", ErrCFNBadTemplate)
	}
	docBytes, err := json.Marshal(docRaw)
	if err != nil {
		return "", nil, fmt.Errorf("%w: TopicPolicy PolicyDocument", ErrCFNBadTemplate)
	}
	policy := string(docBytes)
	var firstARN string
	for i, item := range topics {
		topicRef := strings.TrimSpace(fmt.Sprint(item))
		if topicRef == "" {
			return "", nil, fmt.Errorf("%w: TopicPolicy Topics[%d] empty", ErrCFNBadTemplate, i)
		}
		topic, err := s.GetTopicByARN(topicRef)
		if err != nil {
			topic, err = s.GetTopic(accountID, topicRef)
			if err != nil {
				return "", nil, fmt.Errorf("%w: TopicPolicy topic %s: %v", ErrCFNBadTemplate, topicRef, err)
			}
		}
		if err := s.SetTopicAttributes(accountID, topic.TopicName, map[string]string{"Policy": policy}); err != nil {
			return "", nil, fmt.Errorf("%w: set topic policy: %v", ErrCFNBadTemplate, err)
		}
		if firstARN == "" {
			firstARN = topic.TopicARN
		}
	}
	phys := firstARN + "#TopicPolicy"
	return phys, map[string]string{"Ref": phys, "TopicArn": firstARN}, nil
}

func (s *Store) provisionCFNSNSSubscription(accountID, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	topicARN := cfnStringProp(props, "TopicArn")
	if topicARN == "" {
		return "", nil, fmt.Errorf("%w: Subscription TopicArn required for %s", ErrCFNBadTemplate, logicalID)
	}
	protocol := cfnStringProp(props, "Protocol")
	if protocol == "" {
		return "", nil, fmt.Errorf("%w: Subscription Protocol required for %s", ErrCFNBadTemplate, logicalID)
	}
	endpoint := cfnStringProp(props, "Endpoint")
	if endpoint == "" {
		return "", nil, fmt.Errorf("%w: Subscription Endpoint required for %s", ErrCFNBadTemplate, logicalID)
	}
	sub, err := s.Subscribe(accountID, topicARN, protocol, endpoint)
	if err != nil {
		return "", nil, fmt.Errorf("%w: SNS Subscription %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	return sub.SubscriptionARN, map[string]string{
		"Ref": sub.SubscriptionARN, "Arn": sub.SubscriptionARN,
	}, nil
}

func (s *Store) provisionCFNLogGroup(accountID, region, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	name := cfnStringProp(props, "LogGroupName")
	if name == "" {
		name = "/cfn/" + strings.ToLower(strings.ReplaceAll(logicalID, " ", "-")) + "-" + shortID()
	}
	g, err := s.CreateLogGroup(accountID, region, name)
	if err != nil {
		return "", nil, fmt.Errorf("%w: LogGroup %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	return g.LogGroupName, map[string]string{"Ref": g.LogGroupName, "Arn": g.Arn}, nil
}

func (s *Store) provisionCFNKMSAlias(accountID, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	aliasName := cfnStringProp(props, "AliasName")
	if aliasName == "" {
		return "", nil, fmt.Errorf("%w: Alias AliasName required for %s", ErrCFNBadTemplate, logicalID)
	}
	target := cfnStringProp(props, "TargetKeyId")
	if target == "" {
		return "", nil, fmt.Errorf("%w: Alias TargetKeyId required for %s", ErrCFNBadTemplate, logicalID)
	}
	keyID, err := s.ResolveKeyID(accountID, target)
	if err != nil {
		return "", nil, fmt.Errorf("%w: Alias %s TargetKeyId: %v", ErrCFNBadTemplate, logicalID, err)
	}
	if err := s.CreateAlias(accountID, aliasName, keyID); err != nil {
		return "", nil, fmt.Errorf("%w: Alias %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	normalized := normalizeAliasName(aliasName)
	return normalized, map[string]string{"Ref": normalized}, nil
}

func (s *Store) applyCFNPrincipalPolicies(accountID, principalARN, entityName string, props map[string]any, attachManaged func(accountID, name, policyARN string) error) error {
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
			if err := s.PutInlinePolicy(principalARN, name, string(docBytes)); err != nil {
				return fmt.Errorf("%w: put inline policy: %v", ErrCFNBadTemplate, err)
			}
		}
	}
	if raw, ok := props["ManagedPolicyArns"].([]any); ok {
		for _, item := range raw {
			arn := strings.TrimSpace(fmt.Sprint(item))
			if arn == "" {
				continue
			}
			if err := attachManaged(accountID, entityName, arn); err != nil {
				return fmt.Errorf("%w: AttachPolicy: %v", ErrCFNBadTemplate, err)
			}
		}
	}
	return nil
}

func (s *Store) provisionCFNIAMUser(accountID, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	name := cfnStringProp(props, "UserName")
	if name == "" {
		name = logicalID + shortID()
	}
	path := cfnStringProp(props, "Path")
	if path != "" && path != "/" {
		return "", nil, fmt.Errorf("%w: IAM User Path other than / is not supported for %s", ErrCFNBadTemplate, logicalID)
	}
	_, arn, err := s.CreateUser(accountID, name)
	if err != nil {
		return "", nil, fmt.Errorf("%w: IAM User %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	if err := s.applyCFNPrincipalPolicies(accountID, arn, name, props, s.AttachUserPolicy); err != nil {
		_ = s.DeleteUser(accountID, name)
		return "", nil, err
	}
	for _, groupRef := range cfnStringListProp(props, "Groups") {
		gName := cfnIAMEntityName(groupRef)
		if gName == "" {
			continue
		}
		if err := s.AddUserToGroup(accountID, gName, name); err != nil {
			_ = s.DeleteUser(accountID, name)
			return "", nil, fmt.Errorf("%w: AddUserToGroup %s: %v", ErrCFNBadTemplate, gName, err)
		}
	}
	return name, map[string]string{"Ref": name, "Arn": arn}, nil
}

func (s *Store) provisionCFNIAMGroup(accountID, logicalID string, props map[string]any) (physicalID string, attrs map[string]string, err error) {
	name := cfnStringProp(props, "GroupName")
	if name == "" {
		name = logicalID + shortID()
	}
	path := cfnStringProp(props, "Path")
	if path != "" && path != "/" {
		return "", nil, fmt.Errorf("%w: IAM Group Path other than / is not supported for %s", ErrCFNBadTemplate, logicalID)
	}
	_, arn, err := s.CreateGroup(accountID, name)
	if err != nil {
		return "", nil, fmt.Errorf("%w: IAM Group %s: %v", ErrCFNBadTemplate, logicalID, err)
	}
	if err := s.applyCFNPrincipalPolicies(accountID, arn, name, props, s.AttachGroupPolicy); err != nil {
		_ = s.DeleteGroup(accountID, name)
		return "", nil, err
	}
	return name, map[string]string{"Ref": name, "Arn": arn}, nil
}