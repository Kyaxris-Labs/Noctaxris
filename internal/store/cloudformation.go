package store

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"path"
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
  parent_stack_id TEXT NOT NULL DEFAULT '',
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
CREATE TABLE IF NOT EXISTS cfn_change_sets (
  change_set_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  stack_id TEXT NOT NULL,
  stack_name TEXT NOT NULL,
  change_set_name TEXT NOT NULL,
  status TEXT NOT NULL,
  status_reason TEXT NOT NULL DEFAULT '',
  template_body TEXT NOT NULL,
  changes_json TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cfn_cs_stack ON cfn_change_sets(account_id, stack_name, change_set_name);
CREATE TABLE IF NOT EXISTS cfn_stack_drift (
  detection_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  stack_id TEXT NOT NULL,
  stack_drift_status TEXT NOT NULL,
  detection_status TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS cfn_resource_drift (
  stack_id TEXT NOT NULL,
  logical_id TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  physical_id TEXT NOT NULL,
  drift_status TEXT NOT NULL,
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
	ParentStackID string
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
	DependsOn  any            `json:"DependsOn"`
}

// EnsureCFNSchema creates CloudFormation tables if missing.
func EnsureCFNSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cfn schema: db is nil")
	}
	if _, err := db.Exec(cfnSchema); err != nil {
		return fmt.Errorf("ensure cfn schema: %w", err)
	}
	if err := execMigrateStmt(db, `ALTER TABLE cfn_stacks ADD COLUMN parent_stack_id TEXT NOT NULL DEFAULT ''`, nil); err != nil {
		return fmt.Errorf("ensure cfn schema: parent_stack_id: %w", err)
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

// CreateCFNStack validates a JSON/YAML template, provisions supported resources, and stores the stack.
func (s *Store) CreateCFNStack(accountID, region, stackName, templateBody, roleARN string, capabilities ...string) (CFNStack, error) {
	return s.CreateCFNStackAuthorized(accountID, region, stackName, templateBody, roleARN, capabilities, nil)
}

func (s *Store) CreateCFNStackAuthorized(accountID, region, stackName, templateBody, roleARN string, capabilities []string, authz CFNAuthorizer) (CFNStack, error) {
	return s.createCFNStackWithParent(accountID, region, stackName, templateBody, roleARN, "", cfnProvisionAuth{Authorizer: authz, Capabilities: capabilities})
}

func (s *Store) createCFNStackWithParent(accountID, region, stackName, templateBody, roleARN, parentStackID string, auth cfnProvisionAuth) (CFNStack, error) {
	stackName = strings.TrimSpace(stackName)
	templateBody = strings.TrimSpace(templateBody)
	if stackName == "" {
		return CFNStack{}, fmt.Errorf("%w: StackName is required", ErrCFNBadTemplate)
	}
	tpl, err := parseCFNTemplate(templateBody)
	if err != nil {
		return CFNStack{}, err
	}
	for logicalID, res := range tpl.Resources {
		if !supportedCFNType(res.Type) {
			return CFNStack{}, fmt.Errorf("%w: unsupported resource type %q for %s", ErrCFNBadTemplate, res.Type, logicalID)
		}
		if err := validateCFNProperties(logicalID, res.Type, res.Properties); err != nil {
			return CFNStack{}, err
		}
	}
	if err := s.requireCFNIAMCapabilities(tpl, auth.Capabilities); err != nil {
		return CFNStack{}, err
	}

	var exists string
	err = s.db.QueryRow(
		`SELECT stack_name FROM cfn_stacks WHERE account_id = ? AND stack_name = ?`,
		accountID, stackName,
	).Scan(&exists)
	if err == nil {
		return CFNStack{}, ErrCFNStackExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CFNStack{}, fmt.Errorf("create stack lookup: %w", err)
	}

	order, err := cfnDependencyOrder(tpl.Resources)
	if err != nil {
		return CFNStack{}, err
	}

	uid := uuid.NewString()
	stackID := CFNStackARN(region, accountID, stackName, uid)
	now := time.Now().UTC().UnixMilli()
	resources := make([]CFNStackResource, 0, len(tpl.Resources))
	eval := newCFNEvalCtx(accountID, region, stackName)

	for _, logicalID := range order {
		res := tpl.Resources[logicalID]
		props, resolveErr := eval.resolveProps(res.Properties)
		if resolveErr != nil {
			s.bestEffortRollbackCFNResources(accountID, resources)
			return CFNStack{}, resolveErr
		}
		physicalID, attrs, provErr := s.provisionCFNResource(accountID, region, stackName, stackID, logicalID, res.Type, props, auth)
		if provErr != nil {
			s.bestEffortRollbackCFNResources(accountID, resources)
			return CFNStack{}, provErr
		}
		eval.setResource(logicalID, attrs["Ref"], attrs)
		resources = append(resources, CFNStackResource{
			LogicalID: logicalID, ResourceType: res.Type, PhysicalID: physicalID, Status: "CREATE_COMPLETE",
		})
	}

	tx, err := s.db.Begin()
	if err != nil {
		s.bestEffortRollbackCFNResources(accountID, resources)
		return CFNStack{}, fmt.Errorf("create stack begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, res := range resources {
		_, err = tx.Exec(
			`INSERT INTO cfn_stack_resources (stack_id, logical_id, resource_type, physical_id, status) VALUES (?, ?, ?, ?, ?)`,
			stackID, res.LogicalID, res.ResourceType, res.PhysicalID, res.Status,
		)
		if err != nil {
			s.bestEffortRollbackCFNResources(accountID, resources)
			return CFNStack{}, fmt.Errorf("insert stack resource: %w", err)
		}
	}

	_, err = tx.Exec(
		`INSERT INTO cfn_stacks (account_id, stack_name, stack_id, status, template_body, role_arn, status_reason, parent_stack_id, created_at, updated_at)
		 VALUES (?, ?, ?, 'CREATE_COMPLETE', ?, ?, '', ?, ?, ?)`,
		accountID, stackName, stackID, templateBody, roleARN, parentStackID, now, now,
	)
	if err != nil {
		s.bestEffortRollbackCFNResources(accountID, resources)
		return CFNStack{}, fmt.Errorf("insert stack: %w", err)
	}
	if err := tx.Commit(); err != nil {
		s.bestEffortRollbackCFNResources(accountID, resources)
		return CFNStack{}, fmt.Errorf("create stack commit: %w", err)
	}
	return CFNStack{
		StackName: stackName, StackID: stackID, Status: "CREATE_COMPLETE",
		TemplateBody: templateBody, RoleARN: roleARN, ParentStackID: parentStackID,
		CreationTime: now, LastUpdated: now, Resources: resources,
	}, nil
}

func (s *Store) bestEffortRollbackCFNResources(accountID string, resources []CFNStackResource) {
	for i := len(resources) - 1; i >= 0; i-- {
		s.deleteCFNPhysical(accountID, resources[i])
	}
}

func cfnDynamoKeys(props map[string]any) (hashName, hashType, rangeName, rangeType string, err error) {
	attrTypes := map[string]string{}
	if raw, ok := props["AttributeDefinitions"].([]any); ok {
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			n := cfnStringProp(m, "AttributeName")
			t := cfnStringProp(m, "AttributeType")
			if n != "" && t != "" {
				attrTypes[n] = t
			}
		}
	}
	keys, ok := props["KeySchema"].([]any)
	if !ok || len(keys) == 0 {
		return "", "", "", "", fmt.Errorf("KeySchema required")
	}
	for _, item := range keys {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		n := cfnStringProp(m, "AttributeName")
		kt := cfnStringProp(m, "KeyType")
		at := attrTypes[n]
		if at == "" {
			at = "S"
		}
		switch strings.ToUpper(kt) {
		case "HASH":
			hashName, hashType = n, at
		case "RANGE":
			rangeName, rangeType = n, at
		}
	}
	if hashName == "" {
		return "", "", "", "", fmt.Errorf("HASH key required")
	}
	return hashName, hashType, rangeName, rangeType, nil
}

func cfnLambdaZip(props map[string]any, handler string) ([]byte, error) {
	code, _ := props["Code"].(map[string]any)
	if code == nil {
		return nil, fmt.Errorf("Code required")
	}
	zipFile := cfnStringProp(code, "ZipFile")
	if zipFile == "" {
		return nil, fmt.Errorf("Code.ZipFile required (lab subset)")
	}
	base := "index"
	if h := strings.TrimSpace(handler); h != "" {
		if i := strings.Index(h, "."); i > 0 {
			base = h[:i]
		}
	}
	ext := ".py"
	rt := strings.ToLower(cfnStringProp(props, "Runtime"))
	if strings.HasPrefix(rt, "nodejs") {
		ext = ".js"
	}
	return cfnMakeZip(base+ext, zipFile)
}

func cfnMakeZip(name, body string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(path.Base(name))
	if err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte(body)); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DescribeCFNStacks returns stacks for the account, optionally filtered by name or stack id.
func (s *Store) DescribeCFNStacks(accountID, nameOrID string) ([]CFNStack, error) {
	nameOrID = strings.TrimSpace(nameOrID)
	var rows *sql.Rows
	var err error
	if nameOrID == "" {
		rows, err = s.db.Query(
			`SELECT stack_name, stack_id, status, template_body, role_arn, status_reason, COALESCE(parent_stack_id, ''), created_at, updated_at
			 FROM cfn_stacks WHERE account_id = ? ORDER BY created_at`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT stack_name, stack_id, status, template_body, role_arn, status_reason, COALESCE(parent_stack_id, ''), created_at, updated_at
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
		if err := rows.Scan(&st.StackName, &st.StackID, &st.Status, &st.TemplateBody, &st.RoleARN, &st.StatusReason, &st.ParentStackID, &st.CreationTime, &st.LastUpdated); err != nil {
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

// ListCFNStacks returns stack summaries (includes nested stacks).
func (s *Store) ListCFNStacks(accountID string) ([]CFNStack, error) {
	rows, err := s.db.Query(
		`SELECT stack_name, stack_id, status, template_body, role_arn, status_reason, COALESCE(parent_stack_id, ''), created_at, updated_at
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
		if err := rows.Scan(&st.StackName, &st.StackID, &st.Status, &st.TemplateBody, &st.RoleARN, &st.StatusReason, &st.ParentStackID, &st.CreationTime, &st.LastUpdated); err != nil {
			return nil, fmt.Errorf("list stacks scan: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DeleteCFNStack deletes a stack, nested children first, then tracked resources.
func (s *Store) DeleteCFNStack(accountID, nameOrID string) error {
	stacks, err := s.DescribeCFNStacks(accountID, nameOrID)
	if err != nil {
		return err
	}
	if len(stacks) == 0 {
		return ErrCFNStackNotFound
	}
	st := stacks[0]
	childRows, err := s.db.Query(
		`SELECT stack_name FROM cfn_stacks WHERE account_id = ? AND parent_stack_id = ?`,
		accountID, st.StackID,
	)
	if err != nil {
		return fmt.Errorf("list child stacks: %w", err)
	}
	var children []string
	for childRows.Next() {
		var name string
		if err := childRows.Scan(&name); err != nil {
			childRows.Close()
			return err
		}
		children = append(children, name)
	}
	childRows.Close()
	for _, name := range children {
		if err := s.DeleteCFNStack(accountID, name); err != nil && !errors.Is(err, ErrCFNStackNotFound) {
			return err
		}
	}
	for i := len(st.Resources) - 1; i >= 0; i-- {
		s.deleteCFNPhysical(accountID, st.Resources[i])
	}
	_, err = s.db.Exec(`DELETE FROM cfn_stack_resources WHERE stack_id = ?`, st.StackID)
	if err != nil {
		return fmt.Errorf("delete stack resources: %w", err)
	}
	_, _ = s.db.Exec(`DELETE FROM cfn_change_sets WHERE stack_id = ?`, st.StackID)
	_, _ = s.db.Exec(`DELETE FROM cfn_resource_drift WHERE stack_id = ?`, st.StackID)
	_, _ = s.db.Exec(`DELETE FROM cfn_stack_drift WHERE stack_id = ?`, st.StackID)
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
