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
`

// CloudControlResource is a lab Cloud Control resource row.
type CloudControlResource struct {
	TypeName   string
	Identifier string
	Properties string
	CreatedAt  int64
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
	switch typeName {
	case "AWS::S3::Bucket", "AWS::IAM::Role":
		return true
	default:
		return false
	}
}

// CloudControlCreateResource creates an allowlisted resource and records it.
func (s *Store) CloudControlCreateResource(accountID, typeName, desiredState string) (CloudControlResource, error) {
	typeName = strings.TrimSpace(typeName)
	desiredState = strings.TrimSpace(desiredState)
	if typeName == "" || desiredState == "" {
		return CloudControlResource{}, fmt.Errorf("%w: TypeName and DesiredState required", ErrCloudControlBadRequest)
	}
	if !cloudControlTypeAllowed(typeName) {
		return CloudControlResource{}, fmt.Errorf("%w: type %q is not supported", ErrCloudControlTypeUnsupported, typeName)
	}
	var props map[string]any
	if err := json.Unmarshal([]byte(desiredState), &props); err != nil {
		return CloudControlResource{}, fmt.Errorf("%w: DesiredState must be JSON object", ErrCloudControlBadRequest)
	}
	id, propJSON, err := s.provisionCloudControl(accountID, typeName, props)
	if err != nil {
		return CloudControlResource{}, err
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO cloudcontrol_resources (account_id, type_name, identifier, properties, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, typeName, id, propJSON, now,
	)
	if err != nil {
		return CloudControlResource{}, fmt.Errorf("insert cloudcontrol resource: %w", err)
	}
	return CloudControlResource{TypeName: typeName, Identifier: id, Properties: propJSON, CreatedAt: now}, nil
}

func (s *Store) provisionCloudControl(accountID, typeName string, props map[string]any) (identifier, propJSON string, err error) {
	switch typeName {
	case "AWS::S3::Bucket":
		name, _ := props["BucketName"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			name = "cc-bucket-" + shortID()
			props["BucketName"] = name
		}
		if _, err := s.CreateBucket(accountID, name); err != nil {
			if errors.Is(err, ErrBucketAlreadyExists) {
				return "", "", fmt.Errorf("%w: bucket %s", ErrCloudControlAlreadyExists, name)
			}
			return "", "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		raw, _ := json.Marshal(props)
		return name, string(raw), nil
	case "AWS::IAM::Role":
		name, _ := props["RoleName"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			name = "CCRole" + shortID()
			props["RoleName"] = name
		}
		trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
		if doc, ok := props["AssumeRolePolicyDocument"]; ok {
			raw, mErr := json.Marshal(doc)
			if mErr != nil {
				return "", "", fmt.Errorf("%w: AssumeRolePolicyDocument", ErrCloudControlBadRequest)
			}
			trust = string(raw)
		}
		arn, err := s.CreateRole(accountID, name, trust)
		if err != nil {
			return "", "", fmt.Errorf("%w: %v", ErrCloudControlBadRequest, err)
		}
		props["Arn"] = arn
		raw, _ := json.Marshal(props)
		return name, string(raw), nil
	default:
		return "", "", fmt.Errorf("%w: type %q", ErrCloudControlTypeUnsupported, typeName)
	}
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
		// Fall back to live store for resources created outside Cloud Control.
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
	default:
		return CloudControlResource{}, ErrCloudControlTypeUnsupported
	}
}

// CloudControlListResources lists resources of a type (tracked + live fallback).
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
		return nil, ErrCloudControlTypeUnsupported
	}
}

// CloudControlDeleteResource deletes an allowlisted resource.
func (s *Store) CloudControlDeleteResource(accountID, typeName, identifier string) error {
	typeName = strings.TrimSpace(typeName)
	identifier = strings.TrimSpace(identifier)
	if !cloudControlTypeAllowed(typeName) {
		return fmt.Errorf("%w: type %q is not supported", ErrCloudControlTypeUnsupported, typeName)
	}
	if identifier == "" {
		return fmt.Errorf("%w: Identifier required", ErrCloudControlBadRequest)
	}
	switch typeName {
	case "AWS::S3::Bucket":
		if err := s.DeleteBucket(accountID, identifier); err != nil {
			return fmt.Errorf("%w: %v", ErrCloudControlNotFound, err)
		}
	case "AWS::IAM::Role":
		if err := s.DeleteRole(accountID, identifier); err != nil {
			return fmt.Errorf("%w: %v", ErrCloudControlNotFound, err)
		}
	}
	_, _ = s.db.Exec(
		`DELETE FROM cloudcontrol_resources WHERE account_id = ? AND type_name = ? AND identifier = ?`,
		accountID, typeName, identifier,
	)
	return nil
}

// CloudControlRequestToken returns a lab request token.
func CloudControlRequestToken() string {
	return uuid.NewString()
}
