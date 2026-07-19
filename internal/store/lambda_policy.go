package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrLambdaPolicyStatementExists   = errors.New("ResourceConflictException: statement id already exists")
	ErrLambdaPolicyStatementNotFound = errors.New("ResourceNotFoundException: statement not found")
)

// EnsureLambdaPolicySchema adds the function resource policy column.
func EnsureLambdaPolicySchema(db *sql.DB) error {
	stmt := `ALTER TABLE lambda_functions ADD COLUMN resource_policy TEXT NOT NULL DEFAULT ''`
	if _, err := db.Exec(stmt); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return nil
		}
		return fmt.Errorf("ensure lambda policy schema: %w", err)
	}
	return nil
}

// AddFunctionPermission appends an Allow statement to the function resource policy.
func (s *Store) AddFunctionPermission(accountID, nameOrARN string, statementID, action, principal, sourceAccount string) (string, error) {
	name, err := resolveFunctionName(accountID, nameOrARN)
	if err != nil {
		return "", err
	}
	fn, err := s.GetFunction(accountID, name)
	if err != nil {
		return "", err
	}
	statementID = strings.TrimSpace(statementID)
	if statementID == "" {
		return "", fmt.Errorf("validation: StatementId is required")
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return "", fmt.Errorf("validation: Action is required")
	}
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return "", fmt.Errorf("validation: Principal is required")
	}
	if err := validateLambdaSameAccountPrincipal(accountID, principal); err != nil {
		return "", err
	}
	if src := strings.TrimSpace(sourceAccount); src != "" && src != accountID {
		return "", fmt.Errorf("validation: SourceAccount must match function account")
	}

	doc, err := parseLambdaPolicyDoc(fn.ResourcePolicy)
	if err != nil {
		return "", err
	}
	if lambdaPolicyHasSid(doc, statementID) {
		return "", ErrLambdaPolicyStatementExists
	}

	stmt := map[string]any{
		"Sid":       statementID,
		"Effect":    "Allow",
		"Principal": map[string]any{"AWS": principal},
		"Action":    action,
		"Resource":  fn.FunctionARN,
	}
	statements, err := lambdaPolicyStatements(doc)
	if err != nil {
		return "", err
	}
	statements = append(statements, stmt)
	doc["Statement"] = statements

	updated, err := marshalLambdaPolicyDoc(doc)
	if err != nil {
		return "", err
	}
	if err := s.setFunctionResourcePolicy(accountID, name, updated); err != nil {
		return "", err
	}
	rawStmt, err := json.Marshal(stmt)
	if err != nil {
		return "", fmt.Errorf("marshal statement: %w", err)
	}
	return string(rawStmt), nil
}

// RemoveFunctionPermission deletes a statement by Sid from the function resource policy.
func (s *Store) RemoveFunctionPermission(accountID, nameOrARN, statementID string) error {
	name, err := resolveFunctionName(accountID, nameOrARN)
	if err != nil {
		return err
	}
	fn, err := s.GetFunction(accountID, name)
	if err != nil {
		return err
	}
	statementID = strings.TrimSpace(statementID)
	if statementID == "" {
		return fmt.Errorf("validation: StatementId is required")
	}
	if strings.TrimSpace(fn.ResourcePolicy) == "" {
		return ErrLambdaPolicyStatementNotFound
	}
	doc, err := parseLambdaPolicyDoc(fn.ResourcePolicy)
	if err != nil {
		return err
	}
	statements, err := lambdaPolicyStatements(doc)
	if err != nil {
		return err
	}
	next, removed := removeLambdaPolicyStatement(statements, statementID)
	if !removed {
		return ErrLambdaPolicyStatementNotFound
	}
	if len(next) == 0 {
		return s.setFunctionResourcePolicy(accountID, name, "")
	}
	doc["Statement"] = next
	updated, err := marshalLambdaPolicyDoc(doc)
	if err != nil {
		return err
	}
	return s.setFunctionResourcePolicy(accountID, name, updated)
}

// GetFunctionPolicy returns the stored function policy document.
func (s *Store) GetFunctionPolicy(accountID, nameOrARN string) (string, error) {
	name, err := resolveFunctionName(accountID, nameOrARN)
	if err != nil {
		return "", err
	}
	fn, err := s.GetFunction(accountID, name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(fn.ResourcePolicy) == "" {
		return "", ErrNoSuchResourcePolicy
	}
	return fn.ResourcePolicy, nil
}

func (s *Store) setFunctionResourcePolicy(accountID, name, policy string) error {
	res, err := s.db.Exec(
		`UPDATE lambda_functions SET resource_policy = ? WHERE account_id = ? AND function_name = ?`,
		policy, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("set function resource policy %s: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set function resource policy %s: %w", name, err)
	}
	if n == 0 {
		return ErrNoSuchFunction
	}
	return nil
}

func resolveFunctionName(accountID, nameOrARN string) (string, error) {
	raw := strings.TrimSpace(nameOrARN)
	if raw == "" {
		return "", fmt.Errorf("validation: FunctionName is required")
	}
	name, _ := ParseFunctionQualifier(raw)
	if strings.Contains(raw, ":function:") {
		parts := strings.Split(raw, ":function:")
		if len(parts) == 2 {
			tail := parts[1]
			if idx := strings.Index(tail, ":"); idx >= 0 {
				tail = tail[:idx]
			}
			name = tail
		}
	}
	if err := ValidateFunctionName(name); err != nil {
		return "", err
	}
	return name, nil
}

func validateLambdaSameAccountPrincipal(accountID, principal string) error {
	if principal == "*" {
		return fmt.Errorf("validation: wildcard principal is not supported in the lab")
	}
	const prefix = "arn:aws:iam::"
	if !strings.HasPrefix(principal, prefix) {
		return fmt.Errorf("validation: principal must be a same-account IAM ARN")
	}
	rest := strings.TrimPrefix(principal, prefix)
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return fmt.Errorf("validation: principal must be a same-account IAM ARN")
	}
	if rest[:colon] != accountID {
		return fmt.Errorf("validation: principal must be in the same account")
	}
	return nil
}

func parseLambdaPolicyDoc(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{
			"Version":   "2012-10-17",
			"Id":        "default",
			"Statement": []any{},
		}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("parse function policy: %w", err)
	}
	if doc["Version"] == nil {
		doc["Version"] = "2012-10-17"
	}
	if doc["Id"] == nil {
		doc["Id"] = "default"
	}
	return doc, nil
}

func lambdaPolicyStatements(doc map[string]any) ([]map[string]any, error) {
	raw, ok := doc["Statement"]
	if !ok || raw == nil {
		return []map[string]any{}, nil
	}
	switch typed := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("parse function policy: invalid statement")
			}
			out = append(out, m)
		}
		return out, nil
	case map[string]any:
		return []map[string]any{typed}, nil
	default:
		return nil, fmt.Errorf("parse function policy: invalid statement list")
	}
}

func lambdaPolicyHasSid(doc map[string]any, sid string) bool {
	statements, err := lambdaPolicyStatements(doc)
	if err != nil {
		return false
	}
	for _, stmt := range statements {
		if s, _ := stmt["Sid"].(string); s == sid {
			return true
		}
	}
	return false
}

func removeLambdaPolicyStatement(statements []map[string]any, sid string) ([]map[string]any, bool) {
	out := make([]map[string]any, 0, len(statements))
	removed := false
	for _, stmt := range statements {
		if s, _ := stmt["Sid"].(string); s == sid {
			removed = true
			continue
		}
		out = append(out, stmt)
	}
	return out, removed
}

func marshalLambdaPolicyDoc(doc map[string]any) (string, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal function policy: %w", err)
	}
	return string(raw), nil
}
