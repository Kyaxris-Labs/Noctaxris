package store

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrFunctionAlreadyExists = errors.New("ResourceConflictException")
	ErrNoSuchFunction        = errors.New("ResourceNotFoundException")
	ErrInvalidFunctionName   = errors.New("ValidationException: invalid function name")
)

const (
	// DefaultLambdaRegion is the lab region embedded in Lambda ARNs.
	DefaultLambdaRegion = "us-east-1"

	// LambdaStateActive is the default function state after create.
	LambdaStateActive = "Active"

	// LambdaRuntimePython312 is the lab-supported runtime.
	LambdaRuntimePython312 = "python3.12"
)

var functionNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// LambdaFunction is a Lambda function metadata row. Zip bytes live on disk at CodePath.
type LambdaFunction struct {
	AccountID    string
	FunctionName string
	FunctionARN  string
	RoleARN      string
	Runtime      string
	Handler      string
	Timeout      int
	Memory       int
	Env          map[string]string
	CodeSHA256   string
	CodePath     string
	State        string
	Description  string
	LastModified string
}

// CreateFunctionMeta holds CreateFunction inputs including zip bytes.
type CreateFunctionMeta struct {
	AccountID    string
	Region       string
	FunctionName string
	RoleARN      string
	Runtime      string
	Handler      string
	Timeout      int
	Memory       int
	Env          map[string]string
	Description  string
	Zip          []byte
}

// UpdateFunctionConfigurationMeta holds UpdateFunctionConfiguration fields.
type UpdateFunctionConfigurationMeta struct {
	RoleARN string
	Timeout int
	Memory  int
	Handler string
	Env     map[string]string
	Runtime string
}

// FunctionARN builds arn:aws:lambda:REGION:ACCOUNT:function:NAME.
func FunctionARN(accountID, region, name string) string {
	if region == "" {
		region = DefaultLambdaRegion
	}
	return fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", region, accountID, name)
}

// ValidateFunctionName checks lab function naming: letter start, then alnum/hyphen/underscore.
func ValidateFunctionName(name string) error {
	if name == "" || len(name) > 64 || !functionNamePattern.MatchString(name) {
		return ErrInvalidFunctionName
	}
	return nil
}

func marshalEnv(env map[string]string) (string, error) {
	if len(env) == 0 {
		return "{}", nil
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("marshal env: %w", err)
	}
	return string(raw), nil
}

func unmarshalEnv(raw string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("unmarshal env: %w", err)
	}
	return out, nil
}

func writeLambdaZip(dataRoot, accountID, name string, zipBytes []byte) (relPath, shaHex string, err error) {
	sum := sha256.Sum256(zipBytes)
	shaHex = hex.EncodeToString(sum[:])
	dirRel := filepath.Join("lambda", accountID, name)
	absDir := filepath.Join(dataRoot, dirRel)
	if err := os.MkdirAll(absDir, 0o700); err != nil {
		return "", "", fmt.Errorf("mkdir lambda code dir: %w", err)
	}
	relPath = filepath.Join(dirRel, "code.zip")
	absZip := filepath.Join(dataRoot, relPath)
	if err := os.WriteFile(absZip, zipBytes, 0o600); err != nil {
		return "", "", fmt.Errorf("write lambda zip: %w", err)
	}
	unpackDir := filepath.Join(absDir, "code")
	if err := os.RemoveAll(unpackDir); err != nil {
		return "", "", fmt.Errorf("clear lambda unpack dir: %w", err)
	}
	if err := os.MkdirAll(unpackDir, 0o700); err != nil {
		return "", "", fmt.Errorf("mkdir lambda unpack dir: %w", err)
	}
	if err := unzipBytes(zipBytes, unpackDir); err != nil {
		return "", "", fmt.Errorf("unpack lambda zip: %w", err)
	}
	return relPath, shaHex, nil
}

func unzipBytes(zipBytes []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return err
	}
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.Clean(f.Name)
		if name == "." || strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("unsafe zip entry %q", f.Name)
		}
		target := filepath.Join(destAbs, name)
		if !strings.HasPrefix(target, destAbs+string(os.PathSeparator)) && target != destAbs {
			return fmt.Errorf("zip entry escapes dest: %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// FunctionCodeDir returns the absolute unpacked code directory for a function.
func (s *Store) FunctionCodeDir(accountID, name string) string {
	return filepath.Join(s.dataRoot, "lambda", accountID, name, "code")
}

// FunctionCodeDirInContainer returns the DinD-visible path when dataRoot is
// mounted at the same absolute path inside the engine (Compose default).
func FunctionCodeDirInContainer(dataRoot, accountID, name string) string {
	return filepath.ToSlash(filepath.Join(dataRoot, "lambda", accountID, name, "code"))
}

func scanLambdaFunction(scan func(dest ...any) error) (LambdaFunction, error) {
	var (
		fn      LambdaFunction
		envJSON string
	)
	err := scan(
		&fn.AccountID, &fn.FunctionName, &fn.FunctionARN, &fn.RoleARN, &fn.Runtime, &fn.Handler,
		&fn.Timeout, &fn.Memory, &envJSON, &fn.CodeSHA256, &fn.CodePath, &fn.State, &fn.Description, &fn.LastModified,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LambdaFunction{}, ErrNoSuchFunction
	}
	if err != nil {
		return LambdaFunction{}, err
	}
	fn.Env, err = unmarshalEnv(envJSON)
	if err != nil {
		return LambdaFunction{}, err
	}
	return fn, nil
}

const lambdaSelectCols = `account_id, function_name, function_arn, role_arn, runtime, handler,
		 timeout, memory, env_json, code_sha256, code_path, state, description, last_modified`

// CreateFunction inserts function metadata and writes zip bytes under dataRoot/lambda/.
func (s *Store) CreateFunction(meta CreateFunctionMeta) (LambdaFunction, error) {
	if err := ValidateFunctionName(meta.FunctionName); err != nil {
		return LambdaFunction{}, err
	}
	if len(meta.Zip) == 0 {
		return LambdaFunction{}, fmt.Errorf("create function: zip bytes required")
	}
	envJSON, err := marshalEnv(meta.Env)
	if err != nil {
		return LambdaFunction{}, err
	}
	codePath, shaHex, err := writeLambdaZip(s.dataRoot, meta.AccountID, meta.FunctionName, meta.Zip)
	if err != nil {
		return LambdaFunction{}, err
	}
	arn := FunctionARN(meta.AccountID, meta.Region, meta.FunctionName)
	modified := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO lambda_functions
		 (account_id, function_name, function_arn, role_arn, runtime, handler,
		  timeout, memory, env_json, code_sha256, code_path, state, description, last_modified)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		meta.AccountID, meta.FunctionName, arn, meta.RoleARN, meta.Runtime, meta.Handler,
		meta.Timeout, meta.Memory, envJSON, shaHex, codePath, LambdaStateActive, meta.Description, modified,
	)
	if err != nil {
		_ = os.RemoveAll(filepath.Join(s.dataRoot, "lambda", meta.AccountID, meta.FunctionName))
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return LambdaFunction{}, ErrFunctionAlreadyExists
		}
		return LambdaFunction{}, fmt.Errorf("create function: %w", err)
	}
	envCopy, err := unmarshalEnv(envJSON)
	if err != nil {
		return LambdaFunction{}, err
	}
	return LambdaFunction{
		AccountID:    meta.AccountID,
		FunctionName: meta.FunctionName,
		FunctionARN:  arn,
		RoleARN:      meta.RoleARN,
		Runtime:      meta.Runtime,
		Handler:      meta.Handler,
		Timeout:      meta.Timeout,
		Memory:       meta.Memory,
		Env:          envCopy,
		CodeSHA256:   shaHex,
		CodePath:     codePath,
		State:        LambdaStateActive,
		Description:  meta.Description,
		LastModified: modified,
	}, nil
}

// GetFunction returns function metadata or ErrNoSuchFunction.
func (s *Store) GetFunction(accountID, name string) (LambdaFunction, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaFunction{}, err
	}
	row := s.db.QueryRow(
		`SELECT `+lambdaSelectCols+` FROM lambda_functions WHERE account_id = ? AND function_name = ?`,
		accountID, name,
	)
	fn, err := scanLambdaFunction(row.Scan)
	if err != nil {
		if errors.Is(err, ErrNoSuchFunction) {
			return LambdaFunction{}, err
		}
		return LambdaFunction{}, fmt.Errorf("get function: %w", err)
	}
	return fn, nil
}

// ListFunctions returns functions for an account ordered by name.
func (s *Store) ListFunctions(accountID string) ([]LambdaFunction, error) {
	rows, err := s.db.Query(
		`SELECT `+lambdaSelectCols+` FROM lambda_functions WHERE account_id = ? ORDER BY function_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list functions: %w", err)
	}
	defer rows.Close()
	out := []LambdaFunction{}
	for rows.Next() {
		fn, err := scanLambdaFunction(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("list functions: %w", err)
		}
		out = append(out, fn)
	}
	return out, rows.Err()
}

// DeleteFunction removes function metadata and on-disk zip artifacts.
func (s *Store) DeleteFunction(accountID, name string) error {
	if err := ValidateFunctionName(name); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`DELETE FROM lambda_functions WHERE account_id = ? AND function_name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete function: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchFunction
	}
	_ = os.RemoveAll(filepath.Join(s.dataRoot, "lambda", accountID, name))
	return nil
}

// UpdateFunctionCode replaces the zip artifact and code_sha256.
func (s *Store) UpdateFunctionCode(accountID, name string, zipBytes []byte) (LambdaFunction, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaFunction{}, err
	}
	if len(zipBytes) == 0 {
		return LambdaFunction{}, fmt.Errorf("update function code: zip bytes required")
	}
	if _, err := s.GetFunction(accountID, name); err != nil {
		return LambdaFunction{}, err
	}
	codePath, shaHex, err := writeLambdaZip(s.dataRoot, accountID, name, zipBytes)
	if err != nil {
		return LambdaFunction{}, err
	}
	modified := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE lambda_functions SET code_sha256 = ?, code_path = ?, last_modified = ?
		 WHERE account_id = ? AND function_name = ?`,
		shaHex, codePath, modified, accountID, name,
	)
	if err != nil {
		return LambdaFunction{}, fmt.Errorf("update function code: %w", err)
	}
	return s.GetFunction(accountID, name)
}

// UpdateFunctionConfiguration updates role, timeout, memory, handler, env, and runtime.
func (s *Store) UpdateFunctionConfiguration(accountID, name string, meta UpdateFunctionConfigurationMeta) (LambdaFunction, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaFunction{}, err
	}
	if _, err := s.GetFunction(accountID, name); err != nil {
		return LambdaFunction{}, err
	}
	envJSON, err := marshalEnv(meta.Env)
	if err != nil {
		return LambdaFunction{}, err
	}
	modified := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE lambda_functions
		 SET role_arn = ?, timeout = ?, memory = ?, handler = ?, env_json = ?, runtime = ?, last_modified = ?
		 WHERE account_id = ? AND function_name = ?`,
		meta.RoleARN, meta.Timeout, meta.Memory, meta.Handler, envJSON, meta.Runtime, modified,
		accountID, name,
	)
	if err != nil {
		return LambdaFunction{}, fmt.Errorf("update function configuration: %w", err)
	}
	return s.GetFunction(accountID, name)
}
