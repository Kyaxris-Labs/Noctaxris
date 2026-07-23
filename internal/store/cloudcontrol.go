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
func (s *Store) CloudControlCreateResource(accountID, typeName, desiredState string) (CloudControlResource, string, error) {
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
	physicalID, attrs, err := s.provisionCFNResource(accountID, DefaultCFNRegion, "cloudcontrol", "", "CloudControl", typeName, props)
	if err != nil {
		if errors.Is(err, ErrBucketAlreadyExists) || strings.Contains(err.Error(), "already exists") {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlAlreadyExists, err)
		}
		return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
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
	typeName = strings.TrimSpace(typeName)
	identifier = strings.TrimSpace(identifier)
	if !cloudControlTypeAllowed(typeName) {
		return "", fmt.Errorf("%w: type %q is not supported", ErrCloudControlTypeUnsupported, typeName)
	}
	if identifier == "" {
		return "", fmt.Errorf("%w: Identifier required", ErrCloudControlBadRequest)
	}
	s.deleteCFNPhysical(accountID, CFNStackResource{ResourceType: typeName, PhysicalID: identifier, LogicalID: identifier})
	_, _ = s.db.Exec(
		`DELETE FROM cloudcontrol_resources WHERE account_id = ? AND type_name = ? AND identifier = ?`,
		accountID, typeName, identifier,
	)
	token := CloudControlRequestToken()
	s.recordCloudControlRequest(accountID, token, typeName, identifier, "DELETE", "SUCCESS", "")
	return token, nil
}

// CloudControlUpdateResource updates a mutable property subset for allowlisted types.
func (s *Store) CloudControlUpdateResource(accountID, typeName, identifier, patchDocument string) (CloudControlResource, string, error) {
	typeName = strings.TrimSpace(typeName)
	identifier = strings.TrimSpace(identifier)
	patchDocument = strings.TrimSpace(patchDocument)
	if !cloudControlTypeAllowed(typeName) {
		return CloudControlResource{}, "", fmt.Errorf("%w: type %q is not supported", ErrCloudControlTypeUnsupported, typeName)
	}
	var patch map[string]any
	if err := json.Unmarshal([]byte(patchDocument), &patch); err != nil {
		return CloudControlResource{}, "", fmt.Errorf("%w: PatchDocument must be JSON object", ErrCloudControlBadRequest)
	}
	switch typeName {
	case "AWS::SSM::Parameter":
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
		token := CloudControlRequestToken()
		res := CloudControlResource{TypeName: typeName, Identifier: p.Name, Properties: string(props)}
		s.recordCloudControlRequest(accountID, token, typeName, p.Name, "UPDATE", "SUCCESS", string(props))
		_, _ = s.db.Exec(
			`INSERT INTO cloudcontrol_resources (account_id, type_name, identifier, properties, created_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, type_name, identifier) DO UPDATE SET properties = excluded.properties`,
			accountID, typeName, p.Name, string(props), time.Now().UTC().UnixMilli(),
		)
		return res, token, nil
	case "AWS::S3::Bucket":
		if err := s.applyCFNS3Encryption(accountID, identifier, patch); err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props, _ := json.Marshal(patch)
		if len(patch) == 0 {
			props, _ = json.Marshal(map[string]any{"BucketName": identifier})
		}
		token := CloudControlRequestToken()
		res := CloudControlResource{TypeName: typeName, Identifier: identifier, Properties: string(props)}
		s.recordCloudControlRequest(accountID, token, typeName, identifier, "UPDATE", "SUCCESS", string(props))
		return res, token, nil
	case "AWS::Events::Rule":
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
		r, err := s.PutRule(accountID, DefaultCFNRegion, bus, rule, pattern, cfnStringProp(patch, "Description"), state)
		if err != nil {
			return CloudControlResource{}, "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		phys := bus + "|" + rule
		props, _ := json.Marshal(map[string]any{"Name": rule, "EventBusName": bus, "Arn": r.ARN})
		token := CloudControlRequestToken()
		res := CloudControlResource{TypeName: typeName, Identifier: phys, Properties: string(props)}
		s.recordCloudControlRequest(accountID, token, typeName, phys, "UPDATE", "SUCCESS", string(props))
		return res, token, nil
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