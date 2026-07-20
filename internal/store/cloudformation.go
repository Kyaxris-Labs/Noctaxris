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
	ErrCFNStackExists   = errors.New("AlreadyExistsException")
	ErrCFNStackNotFound = errors.New("ValidationError")
	ErrCFNBadTemplate   = errors.New("ValidationError")
)

const (
	DefaultCFNRegion = "us-east-1"
)

const cfnSchema = `
CREATE TABLE IF NOT EXISTS cfn_stacks (
  account_id TEXT NOT NULL,
  stack_name TEXT NOT NULL,
  stack_id TEXT NOT NULL,
  status TEXT NOT NULL,
  template_body TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  status_reason TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, stack_name)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cfn_stack_id ON cfn_stacks(stack_id);
CREATE TABLE IF NOT EXISTS cfn_stack_resources (
  stack_id TEXT NOT NULL,
  logical_id TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  physical_id TEXT NOT NULL,
  status TEXT NOT NULL,
  PRIMARY KEY (stack_id, logical_id)
);
`

// CFNStack is a CloudFormation stack row.
type CFNStack struct {
	StackName     string
	StackID       string
	Status        string
	TemplateBody  string
	RoleARN       string
	StatusReason  string
	CreationTime  int64
	LastUpdated   int64
	Resources     []CFNStackResource
}

// CFNStackResource is one provisioned template resource.
type CFNStackResource struct {
	LogicalID    string
	ResourceType string
	PhysicalID   string
	Status       string
}

type cfnTemplate struct {
	Resources map[string]cfnResource `json:"Resources"`
}

type cfnResource struct {
	Type       string         `json:"Type"`
	Properties map[string]any `json:"Properties"`
}

// EnsureCFNSchema creates CloudFormation tables if missing.
func EnsureCFNSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cfn schema: db is nil")
	}
	if _, err := db.Exec(cfnSchema); err != nil {
		return fmt.Errorf("ensure cfn schema: %w", err)
	}
	return nil
}

// EnsureCFNSchema ensures CloudFormation tables on an open store.
func (s *Store) EnsureCFNSchema() error {
	return EnsureCFNSchema(s.db)
}

// CFNStackARN builds arn:aws:cloudformation:REGION:ACCOUNT:stack/NAME/UUID
func CFNStackARN(region, accountID, name, id string) string {
	if region == "" {
		region = DefaultCFNRegion
	}
	return fmt.Sprintf("arn:aws:cloudformation:%s:%s:stack/%s/%s", region, accountID, name, id)
}

// CreateCFNStack validates a JSON template, provisions supported resources, and stores the stack.
func (s *Store) CreateCFNStack(accountID, region, stackName, templateBody, roleARN string) (CFNStack, error) {
	stackName = strings.TrimSpace(stackName)
	templateBody = strings.TrimSpace(templateBody)
	if stackName == "" {
		return CFNStack{}, fmt.Errorf("%w: StackName is required", ErrCFNBadTemplate)
	}
	if templateBody == "" {
		return CFNStack{}, fmt.Errorf("%w: TemplateBody is required", ErrCFNBadTemplate)
	}
	var tpl cfnTemplate
	if err := json.Unmarshal([]byte(templateBody), &tpl); err != nil {
		return CFNStack{}, fmt.Errorf("%w: TemplateBody must be JSON", ErrCFNBadTemplate)
	}
	if len(tpl.Resources) == 0 {
		return CFNStack{}, fmt.Errorf("%w: Resources required", ErrCFNBadTemplate)
	}
	for logicalID, res := range tpl.Resources {
		switch res.Type {
		case "AWS::S3::Bucket", "AWS::IAM::Role":
		default:
			return CFNStack{}, fmt.Errorf("%w: unsupported resource type %q for %s", ErrCFNBadTemplate, res.Type, logicalID)
		}
	}

	var exists string
	err := s.db.QueryRow(
		`SELECT stack_name FROM cfn_stacks WHERE account_id = ? AND stack_name = ?`,
		accountID, stackName,
	).Scan(&exists)
	if err == nil {
		return CFNStack{}, ErrCFNStackExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CFNStack{}, fmt.Errorf("create stack lookup: %w", err)
	}

	uid := uuid.NewString()
	stackID := CFNStackARN(region, accountID, stackName, uid)
	now := time.Now().UTC().UnixMilli()
	resources := make([]CFNStackResource, 0, len(tpl.Resources))

	tx, err := s.db.Begin()
	if err != nil {
		return CFNStack{}, fmt.Errorf("create stack begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for logicalID, res := range tpl.Resources {
		physicalID, provErr := s.provisionCFNResource(accountID, logicalID, res)
		if provErr != nil {
			return CFNStack{}, provErr
		}
		resources = append(resources, CFNStackResource{
			LogicalID: logicalID, ResourceType: res.Type, PhysicalID: physicalID, Status: "CREATE_COMPLETE",
		})
		_, err = tx.Exec(
			`INSERT INTO cfn_stack_resources (stack_id, logical_id, resource_type, physical_id, status) VALUES (?, ?, ?, ?, ?)`,
			stackID, logicalID, res.Type, physicalID, "CREATE_COMPLETE",
		)
		if err != nil {
			return CFNStack{}, fmt.Errorf("insert stack resource: %w", err)
		}
	}

	_, err = tx.Exec(
		`INSERT INTO cfn_stacks (account_id, stack_name, stack_id, status, template_body, role_arn, status_reason, created_at, updated_at)
		 VALUES (?, ?, ?, 'CREATE_COMPLETE', ?, ?, '', ?, ?)`,
		accountID, stackName, stackID, templateBody, roleARN, now, now,
	)
	if err != nil {
		return CFNStack{}, fmt.Errorf("insert stack: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CFNStack{}, fmt.Errorf("create stack commit: %w", err)
	}
	return CFNStack{
		StackName: stackName, StackID: stackID, Status: "CREATE_COMPLETE",
		TemplateBody: templateBody, RoleARN: roleARN, CreationTime: now, LastUpdated: now, Resources: resources,
	}, nil
}

func (s *Store) provisionCFNResource(accountID, logicalID string, res cfnResource) (string, error) {
	props := res.Properties
	if props == nil {
		props = map[string]any{}
	}
	switch res.Type {
	case "AWS::S3::Bucket":
		name, _ := props["BucketName"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			name = strings.ToLower(strings.ReplaceAll(logicalID, " ", "-")) + "-" + shortID()
		}
		b, err := s.CreateBucket(accountID, name)
		if err != nil {
			return "", fmt.Errorf("%w: S3 bucket %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		return b.Name, nil
	case "AWS::IAM::Role":
		name, _ := props["RoleName"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			name = logicalID + shortID()
		}
		trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
		if doc, ok := props["AssumeRolePolicyDocument"]; ok {
			raw, err := json.Marshal(doc)
			if err != nil {
				return "", fmt.Errorf("%w: Role trust for %s", ErrCFNBadTemplate, logicalID)
			}
			trust = string(raw)
		}
		arn, err := s.CreateRole(accountID, name, trust)
		if err != nil {
			return "", fmt.Errorf("%w: IAM role %s: %v", ErrCFNBadTemplate, logicalID, err)
		}
		return arn, nil
	default:
		return "", fmt.Errorf("%w: unsupported resource type %q", ErrCFNBadTemplate, res.Type)
	}
}

// DescribeCFNStacks returns stacks for the account, optionally filtered by name or stack id.
func (s *Store) DescribeCFNStacks(accountID, nameOrID string) ([]CFNStack, error) {
	nameOrID = strings.TrimSpace(nameOrID)
	var rows *sql.Rows
	var err error
	if nameOrID == "" {
		rows, err = s.db.Query(
			`SELECT stack_name, stack_id, status, template_body, role_arn, status_reason, created_at, updated_at
			 FROM cfn_stacks WHERE account_id = ? ORDER BY created_at`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT stack_name, stack_id, status, template_body, role_arn, status_reason, created_at, updated_at
			 FROM cfn_stacks WHERE account_id = ? AND (stack_name = ? OR stack_id = ?)`,
			accountID, nameOrID, nameOrID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("describe stacks: %w", err)
	}
	defer rows.Close()
	var out []CFNStack
	for rows.Next() {
		var st CFNStack
		if err := rows.Scan(&st.StackName, &st.StackID, &st.Status, &st.TemplateBody, &st.RoleARN, &st.StatusReason, &st.CreationTime, &st.LastUpdated); err != nil {
			return nil, fmt.Errorf("describe stacks scan: %w", err)
		}
		res, err := s.cfnResources(st.StackID)
		if err != nil {
			return nil, err
		}
		st.Resources = res
		out = append(out, st)
	}
	if nameOrID != "" && len(out) == 0 {
		return nil, ErrCFNStackNotFound
	}
	return out, rows.Err()
}

// ListCFNStacks returns stack summaries.
func (s *Store) ListCFNStacks(accountID string) ([]CFNStack, error) {
	rows, err := s.db.Query(
		`SELECT stack_name, stack_id, status, template_body, role_arn, status_reason, created_at, updated_at
		 FROM cfn_stacks WHERE account_id = ? ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list stacks: %w", err)
	}
	defer rows.Close()
	var out []CFNStack
	for rows.Next() {
		var st CFNStack
		if err := rows.Scan(&st.StackName, &st.StackID, &st.Status, &st.TemplateBody, &st.RoleARN, &st.StatusReason, &st.CreationTime, &st.LastUpdated); err != nil {
			return nil, fmt.Errorf("list stacks scan: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DeleteCFNStack deletes a stack and its tracked resources (best-effort delete of S3/IAM).
func (s *Store) DeleteCFNStack(accountID, nameOrID string) error {
	stacks, err := s.DescribeCFNStacks(accountID, nameOrID)
	if err != nil {
		return err
	}
	if len(stacks) == 0 {
		return ErrCFNStackNotFound
	}
	st := stacks[0]
	for _, res := range st.Resources {
		switch res.ResourceType {
		case "AWS::S3::Bucket":
			_ = s.DeleteBucket(accountID, res.PhysicalID)
		case "AWS::IAM::Role":
			if _, name, ok := parseIAMRoleARN(res.PhysicalID); ok {
				_ = s.DeleteRole(accountID, name)
			}
		}
	}
	_, err = s.db.Exec(`DELETE FROM cfn_stack_resources WHERE stack_id = ?`, st.StackID)
	if err != nil {
		return fmt.Errorf("delete stack resources: %w", err)
	}
	_, err = s.db.Exec(`DELETE FROM cfn_stacks WHERE stack_id = ?`, st.StackID)
	if err != nil {
		return fmt.Errorf("delete stack: %w", err)
	}
	return nil
}

func (s *Store) cfnResources(stackID string) ([]CFNStackResource, error) {
	rows, err := s.db.Query(
		`SELECT logical_id, resource_type, physical_id, status FROM cfn_stack_resources WHERE stack_id = ?`,
		stackID,
	)
	if err != nil {
		return nil, fmt.Errorf("cfn resources: %w", err)
	}
	defer rows.Close()
	var out []CFNStackResource
	for rows.Next() {
		var r CFNStackResource
		if err := rows.Scan(&r.LogicalID, &r.ResourceType, &r.PhysicalID, &r.Status); err != nil {
			return nil, fmt.Errorf("cfn resources scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func parseIAMRoleARN(arn string) (accountID, roleName string, ok bool) {
	const prefix = "arn:aws:iam::"
	if !strings.HasPrefix(arn, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(arn, prefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[1], "role/") {
		return "", "", false
	}
	return parts[0], strings.TrimPrefix(parts[1], "role/"), true
}
