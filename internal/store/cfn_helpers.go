package store

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

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