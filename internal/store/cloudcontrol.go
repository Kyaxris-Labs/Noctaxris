package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCloudControlTypeUnsupported = errors.New("UnsupportedTypeException")
	ErrCloudControlNotFound        = errors.New("ResourceNotFoundException")
	ErrCloudControlBadRequest      = errors.New("InvalidRequestException")
	ErrCloudControlAlreadyExists   = errors.New("AlreadyExistsException")
)

const cloudControlSchema = `
CREATE TABLE IF NOT EXISTS cloudcontrol_resources (
  account_id TEXT NOT NULL,
  type_name TEXT NOT NULL,
  identifier TEXT NOT NULL,
  properties TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, type_name, identifier)
);
CREATE INDEX IF NOT EXISTS idx_cc_type ON cloudcontrol_resources(account_id, type_name);
CREATE TABLE IF NOT EXISTS cloudcontrol_requests (
  request_token TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  type_name TEXT NOT NULL,
  identifier TEXT NOT NULL,
  operation TEXT NOT NULL,
  status TEXT NOT NULL,
  properties TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
`

// CloudControlResource is a lab Cloud Control resource row.
type CloudControlResource struct {
	TypeName   string
	Identifier string
	Properties string
	CreatedAt  int64
}

// CloudControlRequestStatus is a sync ProgressEvent lookup row.
type CloudControlRequestStatus struct {
	RequestToken string
	TypeName     string
	Identifier   string
	Operation    string
	Status       string
	Properties   string
	CreatedAt    int64
}

// EnsureCloudControlSchema creates Cloud Control tables if missing.
func EnsureCloudControlSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cloudcontrol schema: db is nil")
	}
	if _, err := db.Exec(cloudControlSchema); err != nil {
		return fmt.Errorf("ensure cloudcontrol schema: %w", err)
	}
	return nil
}

// EnsureCloudControlSchema ensures Cloud Control tables on an open store.
func (s *Store) EnsureCloudControlSchema() error {
	return EnsureCloudControlSchema(s.db)
}

func cloudControlTypeAllowed(typeName string) bool {
	return cloudControlSupportedCFNType(typeName)
}

func (s *Store) recordCloudControlRequest(accountID, token, typeName, identifier, operation, status, props string) {
	now := time.Now().UTC().UnixMilli()
	_, _ = s.db.Exec(
		`INSERT OR REPLACE INTO cloudcontrol_requests (request_token, account_id, type_name, identifier, operation, status, properties, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		token, accountID, typeName, identifier, operation, status, props, now,
	)
}

// CloudControlCreateResource creates an allowlisted resource and records it.

func cloudControlMapProvisionErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrCFNAccessDenied) {
		return err
	}
	msg := err.Error()
	if errors.Is(err, ErrBucketAlreadyExists) ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "BucketAlreadyExists") {
		return fmt.Errorf("%w: %v", ErrCloudControlAlreadyExists, err)
	}
	return fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
}

func (s *Store) CloudControlCreateResource(accountID, typeName, desiredState string) (CloudControlResource, string, error) {
	return s.CloudControlCreateResourceAuthorized(accountID, typeName, desiredState, nil)
}

func (s *Store) CloudControlCreateResourceAuthorized(accountID, typeName, desiredState string, authz CFNAuthorizer) (CloudControlResource, string, error) {
	auth := cfnProvisionAuth{Authorizer: authz}
	typeName = strings.TrimSpace(typeName)
	desiredState = strings.TrimSpace(desiredState)
	if typeName == "" || desiredState == "" {
		return CloudControlResource{}, "", fmt.Errorf("%w: TypeName and DesiredState required", ErrCloudControlBadRequest)
	}
	if !cloudControlTypeAllowed(typeName) {
		return CloudControlResource{}, "", fmt.Errorf("%w: type %q is not supported", ErrCloudControlTypeUnsupported, typeName)
	}
	var props map[string]any
	if err := json.Unmarshal([]byte(desiredState), &props); err != nil {
		return CloudControlResource{}, "", fmt.Errorf("%w: DesiredState must be JSON object", ErrCloudControlBadRequest)
	}
	if err := validateCFNProperties("CloudControl", typeName, props); err != nil {
		return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
	}
	if cfnIsIAMResourceType(typeName) {
		// Cloud Control has no Capabilities parameter; require underlying IAM actions via authorizer.
		auth.Capabilities = []string{cfnCapNamedIAM, cfnCapIAM}
	}
	physicalID, attrs, err := s.provisionCFNResource(accountID, DefaultCFNRegion, "cloudcontrol", "", "CloudControl", typeName, props, auth)
	if err != nil {
		return CloudControlResource{}, "", cloudControlMapProvisionErr(err)
	}
	identifier := physicalID
	switch typeName {
	case "AWS::IAM::Role":
		identifier = attrs["Ref"]
	case "AWS::SNS::Topic":
		identifier = physicalID
	case "AWS::Events::Rule":
		identifier = physicalID
	}
	props["Identifier"] = identifier
	for k, v := range attrs {
		if _, exists := props[k]; !exists {
			props[k] = v
		}
	}
	propJSON, _ := json.Marshal(props)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO cloudcontrol_resources (account_id, type_name, identifier, properties, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, typeName, identifier, string(propJSON), now,
	)
	if err != nil {
		return CloudControlResource{}, "", fmt.Errorf("insert cloudcontrol resource: %w", err)
	}
	token := CloudControlRequestToken()
	res := CloudControlResource{TypeName: typeName, Identifier: identifier, Properties: string(propJSON), CreatedAt: now}
	s.recordCloudControlRequest(accountID, token, typeName, identifier, "CREATE", "SUCCESS", string(propJSON))
	return res, token, nil
}

// CloudControlGetResource returns one resource by type and identifier.
func (s *Store) CloudControlGetResource(accountID, typeName, identifier string) (CloudControlResource, error) {
	typeName = strings.TrimSpace(typeName)
	identifier = strings.TrimSpace(identifier)
	if !cloudControlTypeAllowed(typeName) {
		return CloudControlResource{}, fmt.Errorf("%w: type %q is not supported", ErrCloudControlTypeUnsupported, typeName)
	}
	var r CloudControlResource
	err := s.db.QueryRow(
		`SELECT type_name, identifier, properties, created_at FROM cloudcontrol_resources
		 WHERE account_id = ? AND type_name = ? AND identifier = ?`,
		accountID, typeName, identifier,
	).Scan(&r.TypeName, &r.Identifier, &r.Properties, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s.cloudControlLiveGet(accountID, typeName, identifier)
	}
	if err != nil {
		return CloudControlResource{}, fmt.Errorf("get cloudcontrol resource: %w", err)
	}
	return r, nil
}

func (s *Store) cloudControlLiveGet(accountID, typeName, identifier string) (CloudControlResource, error) {
	switch typeName {
	case "AWS::S3::Bucket":
		b, err := s.GetBucket(accountID, identifier)
		if err != nil {
			return CloudControlResource{}, ErrCloudControlNotFound
		}
		props, _ := json.Marshal(map[string]any{"BucketName": b.Name})
		return CloudControlResource{TypeName: typeName, Identifier: b.Name, Properties: string(props)}, nil
	case "AWS::IAM::Role":
		role, err := s.GetRoleRecord(accountID, identifier)
		if err != nil {
			return CloudControlResource{}, ErrCloudControlNotFound
		}
		props, _ := json.Marshal(map[string]any{"RoleName": role.RoleName, "Arn": role.RoleARN})
		return CloudControlResource{TypeName: typeName, Identifier: role.RoleName, Properties: string(props)}, nil
	case "AWS::SSM::Parameter":
		p, err := s.GetParameter(accountID, identifier, true)
		if err != nil {
			return CloudControlResource{}, ErrCloudControlNotFound
		}
		props, _ := json.Marshal(map[string]any{"Name": p.Name, "Type": p.Type, "Value": p.Value})
		return CloudControlResource{TypeName: typeName, Identifier: p.Name, Properties: string(props)}, nil
	default:
		return CloudControlResource{}, ErrCloudControlNotFound
	}
}

// CloudControlListResources lists resources of a type (tracked + live fallback for S3/IAM).
func (s *Store) CloudControlListResources(accountID, typeName string) ([]CloudControlResource, error) {
	typeName = strings.TrimSpace(typeName)
	if !cloudControlTypeAllowed(typeName) {
		return nil, fmt.Errorf("%w: type %q is not supported", ErrCloudControlTypeUnsupported, typeName)
	}
	rows, err := s.db.Query(
		`SELECT type_name, identifier, properties, created_at FROM cloudcontrol_resources
		 WHERE account_id = ? AND type_name = ? ORDER BY created_at`,
		accountID, typeName,
	)
	if err != nil {
		return nil, fmt.Errorf("list cloudcontrol resources: %w", err)
	}
	defer rows.Close()
	seen := map[string]struct{}{}
	var out []CloudControlResource
	for rows.Next() {
		var r CloudControlResource
		if err := rows.Scan(&r.TypeName, &r.Identifier, &r.Properties, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("list cloudcontrol scan: %w", err)
		}
		seen[r.Identifier] = struct{}{}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	live, err := s.cloudControlLiveList(accountID, typeName)
	if err != nil {
		return nil, err
	}
	for _, r := range live {
		if _, ok := seen[r.Identifier]; ok {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Store) cloudControlLiveList(accountID, typeName string) ([]CloudControlResource, error) {
	switch typeName {
	case "AWS::S3::Bucket":
		buckets, err := s.ListBuckets(accountID)
		if err != nil {
			return nil, err
		}
		out := make([]CloudControlResource, 0, len(buckets))
		for _, b := range buckets {
			props, _ := json.Marshal(map[string]any{"BucketName": b.Name})
			out = append(out, CloudControlResource{TypeName: typeName, Identifier: b.Name, Properties: string(props)})
		}
		return out, nil
	case "AWS::IAM::Role":
		roles, err := s.ListRoles(accountID)
		if err != nil {
			return nil, err
		}
		out := make([]CloudControlResource, 0, len(roles))
		for _, role := range roles {
			props, _ := json.Marshal(map[string]any{"RoleName": role.RoleName, "Arn": role.RoleARN})
			out = append(out, CloudControlResource{TypeName: typeName, Identifier: role.RoleName, Properties: string(props)})
		}
		return out, nil
	default:
		return nil, nil
	}
}

// CloudControlDeleteResource deletes an allowlisted resource.
func (s *Store) CloudControlDeleteResource(accountID, typeName, identifier string) (string, error) {
	return s.CloudControlDeleteResourceAuthorized(accountID, typeName, identifier, nil)
}

// CloudControlDeleteResourceAuthorized deletes an allowlisted resource after per-type
// underlying-action (and PassRole where create does) checks.
func (s *Store) CloudControlDeleteResourceAuthorized(accountID, typeName, identifier string, authz CFNAuthorizer) (string, error) {
	auth := cfnProvisionAuth{Authorizer: authz}
	typeName = strings.TrimSpace(typeName)
	identifier = strings.TrimSpace(identifier)
	if !cloudControlTypeAllowed(typeName) {
		return "", fmt.Errorf("%w: type %q is not supported", ErrCloudControlTypeUnsupported, typeName)
	}
	if identifier == "" {
		return "", fmt.Errorf("%w: Identifier required", ErrCloudControlBadRequest)
	}
	res := CFNStackResource{ResourceType: typeName, PhysicalID: identifier, LogicalID: identifier}
	if typeName == "AWS::IAM::Role" {
		if !strings.HasPrefix(identifier, "arn:aws:iam::") {
			res.PhysicalID = fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, identifier)
		}
	}
	if err := s.cfnAuthorizeDeletePhysical(auth, accountID, DefaultCFNRegion, res); err != nil {
		return "", err
	}
	s.deleteCFNPhysical(accountID, res)
	_, _ = s.db.Exec(
		`DELETE FROM cloudcontrol_resources WHERE account_id = ? AND type_name = ? AND identifier = ?`,
		accountID, typeName, identifier,
	)
	token := CloudControlRequestToken()
	s.recordCloudControlRequest(accountID, token, typeName, identifier, "DELETE", "SUCCESS", "")
	return token, nil
}

func cloudControlRejectUnknownPatchKeys(patch map[string]any, allowed ...string) error {
	allow := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allow[k] = struct{}{}
	}
	for k := range patch {
		if _, ok := allow[k]; !ok {
			return fmt.Errorf("%w: unsupported patch property %q", ErrCloudControlBadRequest, k)
		}
	}
	return nil
}

func (s *Store) cloudControlPersistUpdate(accountID, typeName, identifier string, propsJSON []byte) (CloudControlResource, string, error) {
	token := CloudControlRequestToken()
	res := CloudControlResource{TypeName: typeName, Identifier: identifier, Properties: string(propsJSON)}
	s.recordCloudControlRequest(accountID, token, typeName, identifier, "UPDATE", "SUCCESS", string(propsJSON))
	_, _ = s.db.Exec(
		`INSERT INTO cloudcontrol_resources (account_id, type_name, identifier, properties, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, type_name, identifier) DO UPDATE SET properties = excluded.properties`,
		accountID, typeName, identifier, string(propsJSON), time.Now().UTC().UnixMilli(),
	)
	return res, token, nil
}

// CloudControlUpdateResource updates a mutable property subset for allowlisted types.
// PatchDocument is a lab JSON object of property keys (not RFC6902), matching CreateResource DesiredState shape.
func (s *Store) CloudControlUpdateResource(accountID, typeName, identifier, patchDocument string) (CloudControlResource, string, error) {
	return s.CloudControlUpdateResourceAuthorized(accountID, typeName, identifier, patchDocument, nil)
}

func (s *Store) CloudControlUpdateResourceAuthorized(accountID, typeName, identifier, patchDocument string, authz CFNAuthorizer) (CloudControlResource, string, error) {
	auth := cfnProvisionAuth{Authorizer: authz}
	if cfnIsIAMResourceType(typeName) {
		auth.Capabilities = []string{cfnCapNamedIAM, cfnCapIAM}
	}
	typeName = strings.TrimSpace(typeName)
	identifier = strings.TrimSpace(identifier)
	patchDocument = strings.TrimSpace(patchDocument)
	if !cloudControlTypeAllowed(typeName) {
		return CloudControlResource{}, "", fmt.Errorf("%w: type %q is not supported", ErrCloudControlTypeUnsupported, typeName)
	}
	if identifier == "" {
		return CloudControlResource{}, "", fmt.Errorf("%w: Identifier required", ErrCloudControlBadRequest)
	}
	var patch map[string]any
	if err := json.Unmarshal([]byte(patchDocument), &patch); err != nil {
		return CloudControlResource{}, "", fmt.Errorf("%w: PatchDocument must be JSON object", ErrCloudControlBadRequest)
	}
	switch typeName {
	case "AWS::SSM::Parameter":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		value := cfnStringProp(patch, "Value")
		ptype := cfnStringProp(patch, "Type")
		if ptype == "" {
			ptype = ParamTypeString
		}
		if value == "" {
			return CloudControlResource{}, "", fmt.Errorf("%w: Value required", ErrCloudControlBadRequest)
		}
		p, err := s.PutParameter(accountID, DefaultCFNRegion, identifier, ptype, value, cfnStringProp(patch, "KeyId"), true)
		if err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(map[string]any{"Name": p.Name, "Type": p.Type, "Value": p.Value})
		return s.cloudControlPersistUpdate(accountID, typeName, p.Name, props)
	case "AWS::S3::Bucket":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		if err := s.applyCFNS3Encryption(accountID, identifier, patch); err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		if err := s.applyCFNS3NotificationConfiguration(accountID, identifier, patch); err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(patch)
		if len(patch) == 0 {
			props, _ = json.Marshal(map[string]any{"BucketName": identifier})
		}
		return s.cloudControlPersistUpdate(accountID, typeName, identifier, props)
	case "AWS::Events::Rule":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		bus, rule, ok := splitCFNEventRulePhysical(identifier)
		if !ok {
			bus = cfnStringProp(patch, "EventBusName")
			if bus == "" {
				bus = "default"
			}
			rule = identifier
		}
		pattern := ""
		if p, ok := patch["EventPattern"]; ok && p != nil {
			switch t := p.(type) {
			case string:
				pattern = t
			default:
				raw, _ := json.Marshal(t)
				pattern = string(raw)
			}
		}
		state := cfnStringProp(patch, "State")
		if state == "" {
			state = "ENABLED"
		}
		if err := s.cfnAuthorizeAction(auth, "events:PutRule", "*"); err != nil {
			return CloudControlResource{}, "", cloudControlMapProvisionErr(err)
		}
		r, err := s.PutRule(accountID, DefaultCFNRegion, bus, rule, pattern, cfnStringProp(patch, "Description"), state)
		if err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		phys := bus + "|" + rule
		props, _ := json.Marshal(map[string]any{"Name": rule, "EventBusName": bus, "Arn": r.ARN})
		return s.cloudControlPersistUpdate(accountID, typeName, phys, props)
	case "AWS::IAM::Role":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		role, err := s.GetRoleRecord(accountID, identifier)
		if err != nil {
			return CloudControlResource{}, "", ErrCloudControlNotFound
		}
		if err := s.modifyCFNIAMRole(accountID, role.RoleARN, map[string]any{"RoleName": role.RoleName}, patch, auth); err != nil {
			return CloudControlResource{}, "", cloudControlMapProvisionErr(err)
		}
		props, _ := json.Marshal(map[string]any{"RoleName": role.RoleName, "Arn": role.RoleARN})
		return s.cloudControlPersistUpdate(accountID, typeName, role.RoleName, props)
	case "AWS::SQS::Queue":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		q, err := s.GetQueueByURL(identifier)
		if err != nil {
			q, err = s.GetQueue(accountID, identifier)
			if err != nil {
				return CloudControlResource{}, "", ErrCloudControlNotFound
			}
		}
		attrs := cfnSQSQueueAttributes(patch)
		if len(attrs) == 0 {
			return CloudControlResource{}, "", fmt.Errorf("%w: no mutable queue attributes in patch", ErrCloudControlBadRequest)
		}
		if err := s.SetQueueAttributes(accountID, q.QueueName, attrs); err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(map[string]any{"QueueName": q.QueueName, "QueueUrl": q.QueueURL})
		return s.cloudControlPersistUpdate(accountID, typeName, q.QueueURL, props)
	case "AWS::SNS::Topic":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		topic, err := s.GetTopicByARN(identifier)
		if err != nil {
			topic, err = s.GetTopic(accountID, identifier)
			if err != nil {
				return CloudControlResource{}, "", ErrCloudControlNotFound
			}
		}
		if err := s.modifyCFNSNSTopic(accountID, topic.TopicARN, map[string]any{}, patch); err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(map[string]any{"TopicName": topic.TopicName, "TopicArn": topic.TopicARN})
		return s.cloudControlPersistUpdate(accountID, typeName, topic.TopicARN, props)
	case "AWS::SecretsManager::Secret":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		if err := s.modifyCFNSecret(accountID, identifier, map[string]any{}, patch); err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(map[string]any{
			"Name": identifier, "Description": cfnStringProp(patch, "Description"), "KmsKeyId": cfnStringProp(patch, "KmsKeyId"),
		})
		return s.cloudControlPersistUpdate(accountID, typeName, identifier, props)
	case "AWS::DynamoDB::Table":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		if _, ok := patch["SSESpecification"]; !ok {
			return CloudControlResource{}, "", fmt.Errorf("%w: SSESpecification required", ErrCloudControlBadRequest)
		}
		if err := s.applyCFNDynamoSSE(accountID, identifier, patch); err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(map[string]any{"TableName": identifier, "SSESpecification": patch["SSESpecification"]})
		return s.cloudControlPersistUpdate(accountID, typeName, identifier, props)
	case "AWS::Lambda::Function":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		if _, err := s.GetFunction(accountID, identifier); err != nil {
			return CloudControlResource{}, "", ErrCloudControlNotFound
		}
		if err := s.modifyCFNLambdaFunction(accountID, DefaultCFNRegion, identifier, map[string]any{}, patch, auth); err != nil {
			return CloudControlResource{}, "", cloudControlMapProvisionErr(err)
		}
		updated, err := s.GetFunction(accountID, identifier)
		if err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(map[string]any{
			"FunctionName": updated.FunctionName, "Timeout": updated.Timeout, "MemorySize": updated.Memory,
		})
		return s.cloudControlPersistUpdate(accountID, typeName, identifier, props)
	case "AWS::KMS::Key":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		if err := s.modifyCFNKMSKey(identifier, map[string]any{}, patch); err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(map[string]any{"KeyId": identifier})
		return s.cloudControlPersistUpdate(accountID, typeName, identifier, props)
	case "AWS::Events::EventBus":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		if _, err := s.GetEventBus(accountID, identifier); err != nil {
			return CloudControlResource{}, "", ErrCloudControlNotFound
		}
		if raw, ok := patch["Policy"]; ok {
			policyJSON := ""
			if raw != nil {
				var pErr error
				policyJSON, pErr = cfnPolicyDocumentJSON(raw)
				if pErr != nil {
					return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, pErr)
				}
			}
			if err := s.PutEventBusPolicy(accountID, identifier, policyJSON); err != nil {
				return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
			}
		}
		bus, err := s.DescribeEventBus(accountID, identifier)
		if err != nil {
			return CloudControlResource{}, "", ErrCloudControlNotFound
		}
		out := map[string]any{"Name": bus.Name, "Arn": bus.ARN}
		if strings.TrimSpace(bus.Policy) != "" {
			var pol any
			if json.Unmarshal([]byte(bus.Policy), &pol) == nil {
				out["Policy"] = pol
			} else {
				out["Policy"] = bus.Policy
			}
		}
		props, _ := json.Marshal(out)
		return s.cloudControlPersistUpdate(accountID, typeName, identifier, props)
	case "AWS::KMS::Alias":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		target := cfnStringProp(patch, "TargetKeyId")
		if target == "" {
			return CloudControlResource{}, "", fmt.Errorf("%w: TargetKeyId required", ErrCloudControlBadRequest)
		}
		if err := s.UpdateAlias(accountID, identifier, target); err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(map[string]any{"AliasName": identifier, "TargetKeyId": target})
		return s.cloudControlPersistUpdate(accountID, typeName, identifier, props)
	case "AWS::Logs::LogGroup":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		if _, err := s.getLogGroup(accountID, identifier); err != nil {
			return CloudControlResource{}, "", ErrCloudControlNotFound
		}
		out := map[string]any{"LogGroupName": identifier}
		if _, ok := patch["RetentionInDays"]; ok {
			days := cfnIntProp(patch, "RetentionInDays", 0)
			if days <= 0 {
				if err := s.DeleteRetentionPolicy(accountID, identifier); err != nil {
					return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
				}
			} else if err := s.PutRetentionPolicy(accountID, identifier, days); err != nil {
				return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
			} else {
				out["RetentionInDays"] = days
			}
		}
		props, _ := json.Marshal(out)
		return s.cloudControlPersistUpdate(accountID, typeName, identifier, props)
	case "AWS::IAM::User":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		u, err := s.GetUser(accountID, identifier)
		if err != nil {
			return CloudControlResource{}, "", ErrCloudControlNotFound
		}
		if err := s.replaceCFNPrincipalPolicies(
			accountID, u.ARN, u.UserName, patch, auth,
			"iam:PutUserPolicy", "iam:DeleteUserPolicy", "iam:AttachUserPolicy", "iam:DetachUserPolicy",
			s.AttachUserPolicy, s.DetachUserPolicy,
		); err != nil {
			return CloudControlResource{}, "", cloudControlMapProvisionErr(err)
		}
		if _, has := patch["Groups"]; has {
			if err := s.replaceCFNUserGroups(accountID, u.UserName, patch, auth); err != nil {
				return CloudControlResource{}, "", cloudControlMapProvisionErr(err)
			}
		}
		props, _ := json.Marshal(map[string]any{"UserName": u.UserName, "Arn": u.ARN})
		return s.cloudControlPersistUpdate(accountID, typeName, u.UserName, props)
	case "AWS::IAM::Group":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		g, err := s.GetGroup(accountID, identifier)
		if err != nil {
			return CloudControlResource{}, "", ErrCloudControlNotFound
		}
		if err := s.replaceCFNPrincipalPolicies(
			accountID, g.ARN, g.GroupName, patch, auth,
			"iam:PutGroupPolicy", "iam:DeleteGroupPolicy", "iam:AttachGroupPolicy", "iam:DetachGroupPolicy",
			s.AttachGroupPolicy, s.DetachGroupPolicy,
		); err != nil {
			return CloudControlResource{}, "", cloudControlMapProvisionErr(err)
		}
		props, _ := json.Marshal(map[string]any{"GroupName": g.GroupName, "Arn": g.ARN})
		return s.cloudControlPersistUpdate(accountID, typeName, g.GroupName, props)
	case "AWS::IAM::ManagedPolicy":
		if err := cfnRejectCloudControlPatchKeys(typeName, patch); err != nil {
			return CloudControlResource{}, "", err
		}
		if _, err := s.GetManagedPolicy(identifier); err != nil {
			return CloudControlResource{}, "", ErrCloudControlNotFound
		}
		if err := s.modifyCFNManagedPolicyDocument(accountID, identifier, map[string]any{}, patch, auth); err != nil {
			return CloudControlResource{}, "", cloudControlMapProvisionErr(err)
		}
		props, _ := json.Marshal(map[string]any{"Arn": identifier, "PolicyDocument": patch["PolicyDocument"]})
		return s.cloudControlPersistUpdate(accountID, typeName, identifier, props)
	default:
		return CloudControlResource{}, "", fmt.Errorf("%w: UpdateResource not supported for %s", ErrCloudControlBadRequest, typeName)
	}
}

// CloudControlGetResourceRequestStatus returns a recorded sync request token status.
func (s *Store) CloudControlGetResourceRequestStatus(accountID, requestToken string) (CloudControlRequestStatus, error) {
	requestToken = strings.TrimSpace(requestToken)
	var st CloudControlRequestStatus
	err := s.db.QueryRow(
		`SELECT request_token, type_name, identifier, operation, status, properties, created_at
		 FROM cloudcontrol_requests WHERE account_id = ? AND request_token = ?`,
		accountID, requestToken,
	).Scan(&st.RequestToken, &st.TypeName, &st.Identifier, &st.Operation, &st.Status, &st.Properties, &st.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudControlRequestStatus{}, ErrCloudControlNotFound
	}
	if err != nil {
		return CloudControlRequestStatus{}, fmt.Errorf("get resource request status: %w", err)
	}
	return st, nil
}

// CloudControlRequestToken returns a lab request token.
func CloudControlRequestToken() string {
	return uuid.NewString()
}
