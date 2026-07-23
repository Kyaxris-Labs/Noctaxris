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
	ErrFunctionAlreadyExists  = errors.New("ResourceConflictException")
	ErrNoSuchFunction         = errors.New("ResourceNotFoundException")
	ErrInvalidFunctionName    = errors.New("ValidationException: invalid function name")
	ErrNoSuchVersion          = errors.New("ResourceNotFoundException: version not found")
	ErrNoSuchAlias            = errors.New("ResourceNotFoundException: alias not found")
	ErrAliasAlreadyExists     = errors.New("ResourceConflictException: alias already exists")
	ErrInvalidAliasName       = errors.New("ValidationException: invalid alias name")
	ErrInvalidFunctionVersion = errors.New("ValidationException: invalid function version")
	ErrNoSuchLayer            = errors.New("ResourceNotFoundException: layer version not found")
	ErrInvalidLayerName       = errors.New("ValidationException: invalid layer name")
	ErrTooManyLayers          = errors.New("ValidationException: too many layers")
	ErrInvalidLayerARN        = errors.New("ValidationException: invalid layer ARN")
)

const (
	// DefaultLambdaRegion is the lab region embedded in Lambda ARNs.
	DefaultLambdaRegion = "us-east-1"

	// LambdaStateActive is the default function state after create.
	LambdaStateActive = "Active"

	// LambdaPackageTypeZip is the default zip deployment package.
	LambdaPackageTypeZip = "Zip"

	// LambdaPackageTypeImage is a container image deployment package.
	LambdaPackageTypeImage = "Image"

	// MaxLambdaLayers is the lab maximum layer attachments per function.
	MaxLambdaLayers = 5
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
	PackageType  string
	ImageURI     string
	State        string
	Description  string
	LastModified            string
	Layers                  []string
	DeadLetterTargetArn     string
	DestinationOnFailureArn string
	DestinationOnSuccessArn string
	ResourcePolicy          string
}

// CreateFunctionMeta holds CreateFunction inputs including zip bytes or an image URI.
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
	PackageType             string
	Zip                     []byte
	ImageURI                string
	Layers                  []string
	DeadLetterTargetArn     string
	DestinationOnFailureArn string
	DestinationOnSuccessArn string
}

// UpdateFunctionConfigurationMeta holds UpdateFunctionConfiguration fields.
type UpdateFunctionConfigurationMeta struct {
	RoleARN string
	Timeout int
	Memory  int
	Handler string
	Env              map[string]string
	Runtime                 string
	Layers                  *[]string
	DeadLetterTargetArn     *string
	DestinationOnFailureArn *string
	DestinationOnSuccessArn *string
}

// LambdaFunctionVersion is an immutable published function version.
type LambdaFunctionVersion struct {
	LambdaFunction
	Version     int
	VersionARN  string
	PublishedAt string
}

// LambdaAlias points a name at a published version number.
type LambdaAlias struct {
	AccountID       string
	FunctionName    string
	AliasName       string
	FunctionVersion int
	AliasARN        string
	Description     string
	RevisionID      string
}

// QualifiedFunction is function metadata plus the resolved qualifier label.
type QualifiedFunction struct {
	LambdaFunction
	Version string
}

// FunctionARN builds arn:aws:lambda:REGION:ACCOUNT:function:NAME.
func FunctionARN(accountID, region, name string) string {
	if region == "" {
		region = DefaultLambdaRegion
	}
	return fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", region, accountID, name)
}

// LambdaVersionARN builds arn:aws:lambda:REGION:ACCOUNT:function:NAME:VERSION.
func LambdaVersionARN(accountID, region, name string, version int) string {
	return fmt.Sprintf("%s:%d", FunctionARN(accountID, region, name), version)
}

// LambdaAliasARN builds arn:aws:lambda:REGION:ACCOUNT:function:NAME:ALIAS.
func LambdaAliasARN(accountID, region, name, alias string) string {
	return fmt.Sprintf("%s:%s", FunctionARN(accountID, region, name), alias)
}

// LayerVersionARN builds arn:aws:lambda:REGION:ACCOUNT:layer:NAME:VERSION.
func LayerVersionARN(accountID, region, name string, version int) string {
	if region == "" {
		region = DefaultLambdaRegion
	}
	return fmt.Sprintf("arn:aws:lambda:%s:%s:layer:%s:%d", region, accountID, name, version)
}

// ParseLayerVersionARN splits a layer version ARN into account, region, name, and version.
func ParseLayerVersionARN(arn string) (accountID, region, name string, version int, ok bool) {
	arn = strings.TrimSpace(arn)
	const prefix = "arn:aws:lambda:"
	if !strings.HasPrefix(arn, prefix) {
		return "", "", "", 0, false
	}
	rest := strings.TrimPrefix(arn, prefix)
	parts := strings.Split(rest, ":")
	if len(parts) != 5 || parts[2] != "layer" {
		return "", "", "", 0, false
	}
	region = parts[0]
	accountID = parts[1]
	name = parts[3]
	if _, err := fmt.Sscanf(parts[4], "%d", &version); err != nil || version <= 0 {
		return "", "", "", 0, false
	}
	return accountID, region, name, version, true
}

// ParseFunctionAccountID extracts the account id from a Lambda function ARN.
// Bare names return ok=false.
func ParseFunctionAccountID(raw string) (accountID string, ok bool) {
	raw = strings.TrimSpace(raw)
	const prefix = "arn:aws:lambda:"
	if !strings.HasPrefix(raw, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(raw, prefix)
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) < 3 || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// ParseFunctionQualifier splits a function name or ARN into base name and qualifier.
// Qualifier is "$LATEST" when omitted.
func ParseFunctionQualifier(raw string) (name, qualifier string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "$LATEST"
	}
	if i := strings.LastIndex(raw, ":function:"); i >= 0 {
		rest := raw[i+len(":function:"):]
		if j := strings.LastIndex(rest, ":"); j >= 0 {
			return rest[:j], rest[j+1:]
		}
		return rest, "$LATEST"
	}
	if j := strings.LastIndex(raw, ":"); j >= 0 {
		return raw[:j], raw[j+1:]
	}
	return raw, "$LATEST"
}

// ValidateAliasName checks lab alias naming (not $LATEST, not all digits).
func ValidateAliasName(name string) error {
	if name == "" || name == "$LATEST" {
		return ErrInvalidAliasName
	}
	if functionNamePattern.MatchString(name) && !isNumericVersion(name) {
		return nil
	}
	return ErrInvalidAliasName
}

func isNumericVersion(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ValidateLayerName checks lab layer naming (same rules as function names).
func ValidateLayerName(name string) error {
	if name == "" || len(name) > 64 || !functionNamePattern.MatchString(name) {
		return ErrInvalidLayerName
	}
	return nil
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

func marshalLayers(layers []string) (string, error) {
	if len(layers) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(layers)
	if err != nil {
		return "", fmt.Errorf("marshal layers: %w", err)
	}
	return string(raw), nil
}

func unmarshalLayers(raw string) ([]string, error) {
	out := []string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("unmarshal layers: %w", err)
	}
	return out, nil
}

func hashImageURI(imageURI string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(imageURI)))
	return hex.EncodeToString(sum[:])
}

func normalizePackageType(packageType string) string {
	switch strings.TrimSpace(packageType) {
	case LambdaPackageTypeImage:
		return LambdaPackageTypeImage
	default:
		return LambdaPackageTypeZip
	}
}

func writeLambdaZip(dataRoot, accountID, name string, zipBytes []byte) (relPath, shaHex string, err error) {
	sum := sha256.Sum256(zipBytes)
	shaHex = hex.EncodeToString(sum[:])
	dirRel := filepath.Join("lambda", accountID, name)
	absDir := filepath.Join(dataRoot, dirRel)
	if err := ensureLabSharedDir(absDir); err != nil {
		return "", "", fmt.Errorf("mkdir lambda code dir: %w", err)
	}
	if err := ensureLabSharedAncestors(dataRoot, absDir); err != nil {
		return "", "", err
	}
	relPath = filepath.Join(dirRel, "code.zip")
	absZip := filepath.Join(dataRoot, relPath)
	// Zip archive stays private; only the unpacked tree is bind-mounted into DinD.
	if err := os.WriteFile(absZip, zipBytes, 0o600); err != nil {
		return "", "", fmt.Errorf("write lambda zip: %w", err)
	}
	unpackDir := filepath.Join(absDir, "code")
	if err := os.RemoveAll(unpackDir); err != nil {
		return "", "", fmt.Errorf("clear lambda unpack dir: %w", err)
	}
	if err := ensureLabSharedDir(unpackDir); err != nil {
		return "", "", fmt.Errorf("mkdir lambda unpack dir: %w", err)
	}
	if err := unzipBytes(zipBytes, unpackDir); err != nil {
		return "", "", fmt.Errorf("unpack lambda zip: %w", err)
	}
	if err := chmodLabSharedTree(unpackDir); err != nil {
		return "", "", err
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
			if err := ensureLabSharedDir(target); err != nil {
				return err
			}
			continue
		}
		if err := ensureLabSharedDir(filepath.Dir(target)); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, LabSharedFileMode)
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
		if err := os.Chmod(target, LabSharedFileMode); err != nil {
			return err
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
		fn         LambdaFunction
		envJSON    string
		layersJSON string
	)
	err := scan(
		&fn.AccountID, &fn.FunctionName, &fn.FunctionARN, &fn.RoleARN, &fn.Runtime, &fn.Handler,
		&fn.Timeout, &fn.Memory, &envJSON, &fn.CodeSHA256, &fn.CodePath, &fn.PackageType, &fn.ImageURI,
		&fn.State, &fn.Description, &fn.LastModified,
		&layersJSON, &fn.DeadLetterTargetArn, &fn.DestinationOnFailureArn, &fn.DestinationOnSuccessArn, &fn.ResourcePolicy,
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
	fn.Layers, err = unmarshalLayers(layersJSON)
	if err != nil {
		return LambdaFunction{}, err
	}
	if fn.PackageType == "" {
		fn.PackageType = LambdaPackageTypeZip
	}
	return fn, nil
}

const lambdaSelectCols = `account_id, function_name, function_arn, role_arn, runtime, handler,
		 timeout, memory, env_json, code_sha256, code_path, package_type, image_uri, state, description, last_modified, layers_json,
		 dead_letter_target_arn, destination_on_failure_arn, COALESCE(destination_on_success_arn, ''), resource_policy`

// EnsureLambdaImageSchema adds package_type and image_uri columns for container images.
func EnsureLambdaImageSchema(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE lambda_functions ADD COLUMN package_type TEXT NOT NULL DEFAULT 'Zip'`,
		`ALTER TABLE lambda_functions ADD COLUMN image_uri TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lambda_versions ADD COLUMN package_type TEXT NOT NULL DEFAULT 'Zip'`,
		`ALTER TABLE lambda_versions ADD COLUMN image_uri TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				continue
			}
			return fmt.Errorf("ensure lambda image schema: %w", err)
		}
	}
	return nil
}

// CreateFunction inserts function metadata and writes zip bytes under dataRoot/lambda/.
func (s *Store) CreateFunction(meta CreateFunctionMeta) (LambdaFunction, error) {
	if err := ValidateFunctionName(meta.FunctionName); err != nil {
		return LambdaFunction{}, err
	}
	packageType := normalizePackageType(meta.PackageType)
	if packageType == LambdaPackageTypeZip {
		if err := ValidateLambdaRuntime(meta.Runtime); err != nil {
			return LambdaFunction{}, err
		}
	}
	envJSON, err := marshalEnv(meta.Env)
	if err != nil {
		return LambdaFunction{}, err
	}
	layers, err := validateFunctionLayers(s, meta.AccountID, meta.Layers)
	if err != nil {
		return LambdaFunction{}, err
	}
	layersJSON, err := marshalLayers(layers)
	if err != nil {
		return LambdaFunction{}, err
	}
	var codePath, shaHex, imageURI string
	switch packageType {
	case LambdaPackageTypeImage:
		imageURI = strings.TrimSpace(meta.ImageURI)
		if imageURI == "" {
			return LambdaFunction{}, fmt.Errorf("create function: image URI required")
		}
		shaHex = hashImageURI(imageURI)
	default:
		if len(meta.Zip) == 0 {
			return LambdaFunction{}, fmt.Errorf("create function: zip bytes required")
		}
		codePath, shaHex, err = writeLambdaZip(s.dataRoot, meta.AccountID, meta.FunctionName, meta.Zip)
		if err != nil {
			return LambdaFunction{}, err
		}
	}
	arn := FunctionARN(meta.AccountID, meta.Region, meta.FunctionName)
	modified := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO lambda_functions
		 (account_id, function_name, function_arn, role_arn, runtime, handler,
		  timeout, memory, env_json, code_sha256, code_path, package_type, image_uri, state, description, last_modified, layers_json,
		  dead_letter_target_arn, destination_on_failure_arn, destination_on_success_arn)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		meta.AccountID, meta.FunctionName, arn, meta.RoleARN, meta.Runtime, meta.Handler,
		meta.Timeout, meta.Memory, envJSON, shaHex, codePath, packageType, imageURI, LambdaStateActive, meta.Description, modified, layersJSON,
		strings.TrimSpace(meta.DeadLetterTargetArn), strings.TrimSpace(meta.DestinationOnFailureArn),
		strings.TrimSpace(meta.DestinationOnSuccessArn),
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
		PackageType:  packageType,
		ImageURI:     imageURI,
		State:        LambdaStateActive,
		Description:  meta.Description,
		LastModified:            modified,
		Layers:                  append([]string(nil), layers...),
		DeadLetterTargetArn:     strings.TrimSpace(meta.DeadLetterTargetArn),
		DestinationOnFailureArn: strings.TrimSpace(meta.DestinationOnFailureArn),
		DestinationOnSuccessArn: strings.TrimSpace(meta.DestinationOnSuccessArn),
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
	_, _ = s.db.Exec(`DELETE FROM lambda_versions WHERE account_id = ? AND function_name = ?`, accountID, name)
	_, _ = s.db.Exec(`DELETE FROM lambda_aliases WHERE account_id = ? AND function_name = ?`, accountID, name)
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
	current, err := s.GetFunction(accountID, name)
	if err != nil {
		return LambdaFunction{}, err
	}
	if current.PackageType == LambdaPackageTypeImage {
		return LambdaFunction{}, fmt.Errorf("update function code: function uses Image package type")
	}
	codePath, shaHex, err := writeLambdaZip(s.dataRoot, accountID, name, zipBytes)
	if err != nil {
		return LambdaFunction{}, err
	}
	modified := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE lambda_functions SET code_sha256 = ?, code_path = ?, image_uri = '', last_modified = ?
		 WHERE account_id = ? AND function_name = ?`,
		shaHex, codePath, modified, accountID, name,
	)
	if err != nil {
		return LambdaFunction{}, fmt.Errorf("update function code: %w", err)
	}
	return s.GetFunction(accountID, name)
}

// UpdateFunctionImageCode replaces the container image URI for an Image function.
func (s *Store) UpdateFunctionImageCode(accountID, name, imageURI string) (LambdaFunction, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaFunction{}, err
	}
	imageURI = strings.TrimSpace(imageURI)
	if imageURI == "" {
		return LambdaFunction{}, fmt.Errorf("update function code: image URI required")
	}
	current, err := s.GetFunction(accountID, name)
	if err != nil {
		return LambdaFunction{}, err
	}
	if current.PackageType != LambdaPackageTypeImage {
		return LambdaFunction{}, fmt.Errorf("update function code: function uses Zip package type")
	}
	shaHex := hashImageURI(imageURI)
	modified := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE lambda_functions SET code_sha256 = ?, code_path = '', image_uri = ?, last_modified = ?
		 WHERE account_id = ? AND function_name = ?`,
		shaHex, imageURI, modified, accountID, name,
	)
	if err != nil {
		return LambdaFunction{}, fmt.Errorf("update function image code: %w", err)
	}
	return s.GetFunction(accountID, name)
}

// UpdateFunctionConfiguration updates role, timeout, memory, handler, env, and runtime.
func (s *Store) UpdateFunctionConfiguration(accountID, name string, meta UpdateFunctionConfigurationMeta) (LambdaFunction, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaFunction{}, err
	}
	current, err := s.GetFunction(accountID, name)
	if err != nil {
		return LambdaFunction{}, err
	}
	if current.PackageType == LambdaPackageTypeZip {
		if err := ValidateLambdaRuntime(meta.Runtime); err != nil {
			return LambdaFunction{}, err
		}
	}
	envJSON, err := marshalEnv(meta.Env)
	if err != nil {
		return LambdaFunction{}, err
	}
	layers := current.Layers
	if meta.Layers != nil {
		layers, err = validateFunctionLayers(s, accountID, *meta.Layers)
		if err != nil {
			return LambdaFunction{}, err
		}
	}
	layersJSON, err := marshalLayers(layers)
	if err != nil {
		return LambdaFunction{}, err
	}
	deadLetter := current.DeadLetterTargetArn
	if meta.DeadLetterTargetArn != nil {
		deadLetter = strings.TrimSpace(*meta.DeadLetterTargetArn)
	}
	onFailure := current.DestinationOnFailureArn
	if meta.DestinationOnFailureArn != nil {
		onFailure = strings.TrimSpace(*meta.DestinationOnFailureArn)
	}
	onSuccess := current.DestinationOnSuccessArn
	if meta.DestinationOnSuccessArn != nil {
		onSuccess = strings.TrimSpace(*meta.DestinationOnSuccessArn)
	}
	modified := nowRFC3339()
	_, err = s.db.Exec(
		`UPDATE lambda_functions
		 SET role_arn = ?, timeout = ?, memory = ?, handler = ?, env_json = ?, runtime = ?, last_modified = ?, layers_json = ?,
		     dead_letter_target_arn = ?, destination_on_failure_arn = ?, destination_on_success_arn = ?
		 WHERE account_id = ? AND function_name = ?`,
		meta.RoleARN, meta.Timeout, meta.Memory, meta.Handler, envJSON, meta.Runtime, modified, layersJSON,
		deadLetter, onFailure, onSuccess, accountID, name,
	)
	if err != nil {
		return LambdaFunction{}, fmt.Errorf("update function configuration: %w", err)
	}
	return s.GetFunction(accountID, name)
}

// PutFunctionEventInvokeConfig sets DestinationConfig OnFailure/OnSuccess for $LATEST.
func (s *Store) PutFunctionEventInvokeConfig(accountID, name, onFailure, onSuccess string) (LambdaFunction, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaFunction{}, err
	}
	if _, err := s.GetFunction(accountID, name); err != nil {
		return LambdaFunction{}, err
	}
	modified := nowRFC3339()
	_, err := s.db.Exec(
		`UPDATE lambda_functions
		 SET destination_on_failure_arn = ?, destination_on_success_arn = ?, last_modified = ?
		 WHERE account_id = ? AND function_name = ?`,
		strings.TrimSpace(onFailure), strings.TrimSpace(onSuccess), modified, accountID, name,
	)
	if err != nil {
		return LambdaFunction{}, fmt.Errorf("put event invoke config: %w", err)
	}
	return s.GetFunction(accountID, name)
}

// DeleteFunctionEventInvokeConfig clears DestinationConfig destinations for $LATEST.
func (s *Store) DeleteFunctionEventInvokeConfig(accountID, name string) error {
	if err := ValidateFunctionName(name); err != nil {
		return err
	}
	if _, err := s.GetFunction(accountID, name); err != nil {
		return err
	}
	modified := nowRFC3339()
	_, err := s.db.Exec(
		`UPDATE lambda_functions
		 SET destination_on_failure_arn = '', destination_on_success_arn = '', last_modified = ?
		 WHERE account_id = ? AND function_name = ?`,
		modified, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete event invoke config: %w", err)
	}
	return nil
}

// EnsureLambdaVersionSchema creates lambda_versions and lambda_aliases tables.
func EnsureLambdaVersionSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS lambda_versions (
		  account_id TEXT NOT NULL,
		  function_name TEXT NOT NULL,
		  version INTEGER NOT NULL,
		  function_version_arn TEXT NOT NULL,
		  role_arn TEXT NOT NULL,
		  runtime TEXT NOT NULL,
		  handler TEXT NOT NULL,
		  timeout INTEGER NOT NULL,
		  memory INTEGER NOT NULL,
		  env_json TEXT NOT NULL DEFAULT '{}',
		  code_sha256 TEXT NOT NULL,
		  code_path TEXT NOT NULL,
		  state TEXT NOT NULL DEFAULT 'Active',
		  description TEXT NOT NULL DEFAULT '',
		  last_modified TEXT NOT NULL,
		  published_at TEXT NOT NULL,
		  PRIMARY KEY (account_id, function_name, version)
		)`,
		`CREATE TABLE IF NOT EXISTS lambda_aliases (
		  account_id TEXT NOT NULL,
		  function_name TEXT NOT NULL,
		  alias_name TEXT NOT NULL,
		  function_version INTEGER NOT NULL,
		  alias_arn TEXT NOT NULL,
		  description TEXT NOT NULL DEFAULT '',
		  revision_id TEXT NOT NULL,
		  PRIMARY KEY (account_id, function_name, alias_name)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("ensure lambda version schema: %w", err)
		}
	}
	return nil
}

func copyLambdaVersionArtifacts(dataRoot, accountID, name string, version int, srcRelPath string) (relPath, shaHex string, err error) {
	srcZip := filepath.Join(dataRoot, srcRelPath)
	zipBytes, err := os.ReadFile(srcZip)
	if err != nil {
		return "", "", fmt.Errorf("read source zip: %w", err)
	}
	sum := sha256.Sum256(zipBytes)
	shaHex = hex.EncodeToString(sum[:])
	dirRel := filepath.Join("lambda", accountID, name, "versions", fmt.Sprintf("%d", version))
	absDir := filepath.Join(dataRoot, dirRel)
	if err := ensureLabSharedDir(absDir); err != nil {
		return "", "", fmt.Errorf("mkdir version dir: %w", err)
	}
	if err := ensureLabSharedAncestors(dataRoot, absDir); err != nil {
		return "", "", err
	}
	relPath = filepath.Join(dirRel, "code.zip")
	absZip := filepath.Join(dataRoot, relPath)
	if err := os.WriteFile(absZip, zipBytes, 0o600); err != nil {
		return "", "", fmt.Errorf("write version zip: %w", err)
	}
	unpackDir := filepath.Join(absDir, "code")
	if err := os.RemoveAll(unpackDir); err != nil {
		return "", "", fmt.Errorf("clear version unpack dir: %w", err)
	}
	if err := ensureLabSharedDir(unpackDir); err != nil {
		return "", "", fmt.Errorf("mkdir version unpack dir: %w", err)
	}
	if err := unzipBytes(zipBytes, unpackDir); err != nil {
		return "", "", fmt.Errorf("unpack version zip: %w", err)
	}
	if err := chmodLabSharedTree(unpackDir); err != nil {
		return "", "", err
	}
	return relPath, shaHex, nil
}

func scanLambdaVersion(scan func(dest ...any) error) (LambdaFunctionVersion, error) {
	var (
		v          LambdaFunctionVersion
		envJSON    string
		layersJSON string
	)
	err := scan(
		&v.AccountID, &v.FunctionName, &v.Version, &v.VersionARN, &v.RoleARN, &v.Runtime, &v.Handler,
		&v.Timeout, &v.Memory, &envJSON, &v.CodeSHA256, &v.CodePath, &v.PackageType, &v.ImageURI,
		&v.State, &v.Description,
		&v.LastModified, &v.PublishedAt, &layersJSON, &v.DeadLetterTargetArn, &v.DestinationOnFailureArn,
		&v.DestinationOnSuccessArn,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LambdaFunctionVersion{}, ErrNoSuchVersion
	}
	if err != nil {
		return LambdaFunctionVersion{}, err
	}
	v.Env, err = unmarshalEnv(envJSON)
	if err != nil {
		return LambdaFunctionVersion{}, err
	}
	v.Layers, err = unmarshalLayers(layersJSON)
	if err != nil {
		return LambdaFunctionVersion{}, err
	}
	if v.PackageType == "" {
		v.PackageType = LambdaPackageTypeZip
	}
	v.FunctionARN = FunctionARN(v.AccountID, DefaultLambdaRegion, v.FunctionName)
	return v, nil
}

const lambdaVersionSelectCols = `account_id, function_name, version, function_version_arn, role_arn, runtime, handler,
		 timeout, memory, env_json, code_sha256, code_path, package_type, image_uri, state, description, last_modified, published_at, layers_json,
		 dead_letter_target_arn, destination_on_failure_arn, COALESCE(destination_on_success_arn, '')`

// PublishVersion freezes the current $LATEST code and configuration into the next version number.
func (s *Store) PublishVersion(accountID, name string) (LambdaFunctionVersion, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaFunctionVersion{}, err
	}
	latest, err := s.GetFunction(accountID, name)
	if err != nil {
		return LambdaFunctionVersion{}, err
	}
	var next int
	row := s.db.QueryRow(
		`SELECT COALESCE(MAX(version), 0) + 1 FROM lambda_versions WHERE account_id = ? AND function_name = ?`,
		accountID, name,
	)
	if err := row.Scan(&next); err != nil {
		return LambdaFunctionVersion{}, fmt.Errorf("publish version: %w", err)
	}
	var codePath, shaHex, imageURI string
	packageType := latest.PackageType
	if packageType == "" {
		packageType = LambdaPackageTypeZip
	}
	if packageType == LambdaPackageTypeImage {
		codePath = ""
		shaHex = latest.CodeSHA256
		imageURI = latest.ImageURI
	} else {
		var copyErr error
		codePath, shaHex, copyErr = copyLambdaVersionArtifacts(s.dataRoot, accountID, name, next, latest.CodePath)
		if copyErr != nil {
			return LambdaFunctionVersion{}, copyErr
		}
	}
	envJSON, err := marshalEnv(latest.Env)
	if err != nil {
		return LambdaFunctionVersion{}, err
	}
	layersJSON, err := marshalLayers(latest.Layers)
	if err != nil {
		return LambdaFunctionVersion{}, err
	}
	published := nowRFC3339()
	versionARN := LambdaVersionARN(accountID, DefaultLambdaRegion, name, next)
	_, err = s.db.Exec(
		`INSERT INTO lambda_versions
		 (account_id, function_name, version, function_version_arn, role_arn, runtime, handler,
		  timeout, memory, env_json, code_sha256, code_path, package_type, image_uri, state, description, last_modified, published_at, layers_json,
		  dead_letter_target_arn, destination_on_failure_arn, destination_on_success_arn)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, next, versionARN, latest.RoleARN, latest.Runtime, latest.Handler,
		latest.Timeout, latest.Memory, envJSON, shaHex, codePath, packageType, imageURI, latest.State, latest.Description,
		latest.LastModified, published, layersJSON, latest.DeadLetterTargetArn, latest.DestinationOnFailureArn,
		latest.DestinationOnSuccessArn,
	)
	if err != nil {
		_ = os.RemoveAll(filepath.Join(s.dataRoot, "lambda", accountID, name, "versions", fmt.Sprintf("%d", next)))
		return LambdaFunctionVersion{}, fmt.Errorf("publish version: %w", err)
	}
	return LambdaFunctionVersion{
		LambdaFunction: LambdaFunction{
			AccountID:    accountID,
			FunctionName: name,
			FunctionARN:  latest.FunctionARN,
			RoleARN:      latest.RoleARN,
			Runtime:      latest.Runtime,
			Handler:      latest.Handler,
			Timeout:      latest.Timeout,
			Memory:       latest.Memory,
			Env:          latest.Env,
			CodeSHA256:   shaHex,
			CodePath:     codePath,
			PackageType:  packageType,
			ImageURI:     imageURI,
			State:        latest.State,
			Description:  latest.Description,
			LastModified:            latest.LastModified,
			Layers:                  append([]string(nil), latest.Layers...),
			DeadLetterTargetArn:     latest.DeadLetterTargetArn,
			DestinationOnFailureArn: latest.DestinationOnFailureArn,
			DestinationOnSuccessArn: latest.DestinationOnSuccessArn,
		},
		Version:     next,
		VersionARN:  versionARN,
		PublishedAt: published,
	}, nil
}

// ListVersionsByFunction returns published versions ordered by version number.
func (s *Store) ListVersionsByFunction(accountID, name string) ([]LambdaFunctionVersion, error) {
	if err := ValidateFunctionName(name); err != nil {
		return nil, err
	}
	if _, err := s.GetFunction(accountID, name); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT `+lambdaVersionSelectCols+` FROM lambda_versions
		 WHERE account_id = ? AND function_name = ? ORDER BY version`,
		accountID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()
	out := []LambdaFunctionVersion{}
	for rows.Next() {
		v, err := scanLambdaVersion(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("list versions: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) getVersion(accountID, name string, version int) (LambdaFunctionVersion, error) {
	row := s.db.QueryRow(
		`SELECT `+lambdaVersionSelectCols+` FROM lambda_versions
		 WHERE account_id = ? AND function_name = ? AND version = ?`,
		accountID, name, version,
	)
	v, err := scanLambdaVersion(row.Scan)
	if err != nil {
		if errors.Is(err, ErrNoSuchVersion) {
			return LambdaFunctionVersion{}, err
		}
		return LambdaFunctionVersion{}, fmt.Errorf("get version: %w", err)
	}
	return v, nil
}

func (v LambdaFunctionVersion) toLambdaFunction() LambdaFunction {
	return v.LambdaFunction
}

// CreateLambdaAlias creates an alias pointing at a published version number.
func (s *Store) CreateLambdaAlias(accountID, name, aliasName string, functionVersion int, description string) (LambdaAlias, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaAlias{}, err
	}
	if err := ValidateAliasName(aliasName); err != nil {
		return LambdaAlias{}, err
	}
	if functionVersion <= 0 {
		return LambdaAlias{}, ErrInvalidFunctionVersion
	}
	if _, err := s.GetFunction(accountID, name); err != nil {
		return LambdaAlias{}, err
	}
	if _, err := s.getVersion(accountID, name, functionVersion); err != nil {
		return LambdaAlias{}, err
	}
	revisionID, err := newRevisionID()
	if err != nil {
		return LambdaAlias{}, err
	}
	aliasARN := LambdaAliasARN(accountID, DefaultLambdaRegion, name, aliasName)
	_, err = s.db.Exec(
		`INSERT INTO lambda_aliases
		 (account_id, function_name, alias_name, function_version, alias_arn, description, revision_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, aliasName, functionVersion, aliasARN, description, revisionID,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return LambdaAlias{}, ErrAliasAlreadyExists
		}
		return LambdaAlias{}, fmt.Errorf("create alias: %w", err)
	}
	return LambdaAlias{
		AccountID:       accountID,
		FunctionName:    name,
		AliasName:       aliasName,
		FunctionVersion: functionVersion,
		AliasARN:        aliasARN,
		Description:     description,
		RevisionID:      revisionID,
	}, nil
}

func scanLambdaAlias(scan func(dest ...any) error) (LambdaAlias, error) {
	var a LambdaAlias
	err := scan(
		&a.AccountID, &a.FunctionName, &a.AliasName, &a.FunctionVersion,
		&a.AliasARN, &a.Description, &a.RevisionID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LambdaAlias{}, ErrNoSuchAlias
	}
	return a, err
}

const lambdaAliasSelectCols = `account_id, function_name, alias_name, function_version, alias_arn, description, revision_id`

// GetLambdaAlias returns an alias or ErrNoSuchAlias.
func (s *Store) GetLambdaAlias(accountID, name, aliasName string) (LambdaAlias, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaAlias{}, err
	}
	if err := ValidateAliasName(aliasName); err != nil {
		return LambdaAlias{}, err
	}
	row := s.db.QueryRow(
		`SELECT `+lambdaAliasSelectCols+` FROM lambda_aliases
		 WHERE account_id = ? AND function_name = ? AND alias_name = ?`,
		accountID, name, aliasName,
	)
	a, err := scanLambdaAlias(row.Scan)
	if err != nil {
		if errors.Is(err, ErrNoSuchAlias) {
			return LambdaAlias{}, err
		}
		return LambdaAlias{}, fmt.Errorf("get alias: %w", err)
	}
	return a, nil
}

// ListLambdaAliases returns aliases for a function ordered by name.
func (s *Store) ListLambdaAliases(accountID, name string) ([]LambdaAlias, error) {
	if err := ValidateFunctionName(name); err != nil {
		return nil, err
	}
	if _, err := s.GetFunction(accountID, name); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT `+lambdaAliasSelectCols+` FROM lambda_aliases
		 WHERE account_id = ? AND function_name = ? ORDER BY alias_name`,
		accountID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("list aliases: %w", err)
	}
	defer rows.Close()
	out := []LambdaAlias{}
	for rows.Next() {
		a, err := scanLambdaAlias(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("list aliases: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateLambdaAlias changes the target version and/or description for an alias.
func (s *Store) UpdateLambdaAlias(accountID, name, aliasName string, functionVersion int, description string) (LambdaAlias, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaAlias{}, err
	}
	if err := ValidateAliasName(aliasName); err != nil {
		return LambdaAlias{}, err
	}
	if functionVersion <= 0 {
		return LambdaAlias{}, ErrInvalidFunctionVersion
	}
	if _, err := s.getVersion(accountID, name, functionVersion); err != nil {
		return LambdaAlias{}, err
	}
	existing, err := s.GetLambdaAlias(accountID, name, aliasName)
	if err != nil {
		return LambdaAlias{}, err
	}
	revisionID, err := newRevisionID()
	if err != nil {
		return LambdaAlias{}, err
	}
	desc := description
	_, err = s.db.Exec(
		`UPDATE lambda_aliases SET function_version = ?, description = ?, revision_id = ?
		 WHERE account_id = ? AND function_name = ? AND alias_name = ?`,
		functionVersion, desc, revisionID, accountID, name, aliasName,
	)
	if err != nil {
		return LambdaAlias{}, fmt.Errorf("update alias: %w", err)
	}
	return LambdaAlias{
		AccountID:       accountID,
		FunctionName:    name,
		AliasName:       aliasName,
		FunctionVersion: functionVersion,
		AliasARN:        existing.AliasARN,
		Description:     desc,
		RevisionID:      revisionID,
	}, nil
}

// DeleteLambdaAlias removes an alias.
func (s *Store) DeleteLambdaAlias(accountID, name, aliasName string) error {
	if err := ValidateFunctionName(name); err != nil {
		return err
	}
	if err := ValidateAliasName(aliasName); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`DELETE FROM lambda_aliases WHERE account_id = ? AND function_name = ? AND alias_name = ?`,
		accountID, name, aliasName,
	)
	if err != nil {
		return fmt.Errorf("delete alias: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchAlias
	}
	return nil
}

// ResolveFunction returns invoke-ready function metadata for a qualifier.
// executedVersion is "$LATEST", a numeric version string, or the resolved version for an alias.
func (s *Store) ResolveFunction(accountID, name, qualifier string) (LambdaFunction, string, error) {
	if err := ValidateFunctionName(name); err != nil {
		return LambdaFunction{}, "", err
	}
	qualifier = strings.TrimSpace(qualifier)
	if qualifier == "" || qualifier == "$LATEST" {
		fn, err := s.GetFunction(accountID, name)
		if err != nil {
			return LambdaFunction{}, "", err
		}
		return fn, "$LATEST", nil
	}
	if isNumericVersion(qualifier) {
		var version int
		if _, err := fmt.Sscanf(qualifier, "%d", &version); err != nil || version <= 0 {
			return LambdaFunction{}, "", ErrNoSuchVersion
		}
		v, err := s.getVersion(accountID, name, version)
		if err != nil {
			return LambdaFunction{}, "", err
		}
		return v.toLambdaFunction(), qualifier, nil
	}
	if err := ValidateAliasName(qualifier); err != nil {
		return LambdaFunction{}, "", err
	}
	alias, err := s.GetLambdaAlias(accountID, name, qualifier)
	if err != nil {
		return LambdaFunction{}, "", err
	}
	v, err := s.getVersion(accountID, name, alias.FunctionVersion)
	if err != nil {
		return LambdaFunction{}, "", err
	}
	return v.toLambdaFunction(), fmt.Sprintf("%d", alias.FunctionVersion), nil
}

// GetFunctionByQualifier returns function metadata labeled with the qualifier string.
func (s *Store) GetFunctionByQualifier(accountID, name, qualifier string) (QualifiedFunction, error) {
	fn, executed, err := s.ResolveFunction(accountID, name, qualifier)
	if err != nil {
		return QualifiedFunction{}, err
	}
	versionLabel := executed
	if qualifier != "" && qualifier != "$LATEST" && !isNumericVersion(qualifier) {
		versionLabel = qualifier
	}
	return QualifiedFunction{LambdaFunction: fn, Version: versionLabel}, nil
}

func newRevisionID() (string, error) {
	sum := sha256.Sum256([]byte(nowRFC3339() + fmt.Sprintf("%d", os.Getpid())))
	return hex.EncodeToString(sum[:16]), nil
}

// VersionCodeDir returns the absolute unpacked code directory for a published version.
func (s *Store) VersionCodeDir(accountID, name string, version int) string {
	return filepath.Join(s.dataRoot, "lambda", accountID, name, "versions", fmt.Sprintf("%d", version), "code")
}

// VersionCodeDirInContainer returns the DinD-visible path for a published version.
func VersionCodeDirInContainer(dataRoot, accountID, name string, version int) string {
	return filepath.ToSlash(filepath.Join(dataRoot, "lambda", accountID, name, "versions", fmt.Sprintf("%d", version), "code"))
}

// LambdaLayer is a published layer version metadata row.
type LambdaLayer struct {
	AccountID   string
	LayerName   string
	Version     int
	LayerARN    string
	CodeSHA256  string
	CodePath    string
	Description string
	PublishedAt string
}

// PublishLayerVersionMeta holds PublishLayerVersion inputs including zip bytes.
type PublishLayerVersionMeta struct {
	AccountID   string
	Region      string
	LayerName   string
	Description string
	Zip         []byte
}

func writeLambdaLayerZip(dataRoot, accountID, name string, version int, zipBytes []byte) (relPath, shaHex string, err error) {
	sum := sha256.Sum256(zipBytes)
	shaHex = hex.EncodeToString(sum[:])
	dirRel := filepath.Join("lambda", accountID, "layers", name, "versions", fmt.Sprintf("%d", version))
	absDir := filepath.Join(dataRoot, dirRel)
	if err := ensureLabSharedDir(absDir); err != nil {
		return "", "", fmt.Errorf("mkdir layer dir: %w", err)
	}
	if err := ensureLabSharedAncestors(dataRoot, absDir); err != nil {
		return "", "", err
	}
	relPath = filepath.Join(dirRel, "layer.zip")
	absZip := filepath.Join(dataRoot, relPath)
	if err := os.WriteFile(absZip, zipBytes, 0o600); err != nil {
		return "", "", fmt.Errorf("write layer zip: %w", err)
	}
	unpackDir := filepath.Join(absDir, "code")
	if err := os.RemoveAll(unpackDir); err != nil {
		return "", "", fmt.Errorf("clear layer unpack dir: %w", err)
	}
	if err := ensureLabSharedDir(unpackDir); err != nil {
		return "", "", fmt.Errorf("mkdir layer unpack dir: %w", err)
	}
	if err := unzipBytes(zipBytes, unpackDir); err != nil {
		return "", "", fmt.Errorf("unpack layer zip: %w", err)
	}
	if err := chmodLabSharedTree(unpackDir); err != nil {
		return "", "", err
	}
	return relPath, shaHex, nil
}

func validateFunctionLayers(s *Store, accountID string, layerARNs []string) ([]string, error) {
	if len(layerARNs) > MaxLambdaLayers {
		return nil, ErrTooManyLayers
	}
	out := make([]string, 0, len(layerARNs))
	for _, arn := range layerARNs {
		arn = strings.TrimSpace(arn)
		if arn == "" {
			continue
		}
		layerAccount, _, name, version, ok := ParseLayerVersionARN(arn)
		if !ok {
			return nil, ErrInvalidLayerARN
		}
		if layerAccount != accountID {
			return nil, ErrInvalidLayerARN
		}
		if _, err := s.GetLayerVersion(accountID, name, version); err != nil {
			return nil, err
		}
		out = append(out, LayerVersionARN(accountID, DefaultLambdaRegion, name, version))
	}
	return out, nil
}

// EnsureLambdaLayerSchema creates lambda_layers and adds layers_json columns.
func EnsureLambdaLayerSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS lambda_layers (
		  account_id TEXT NOT NULL,
		  layer_name TEXT NOT NULL,
		  version INTEGER NOT NULL,
		  layer_version_arn TEXT NOT NULL,
		  code_sha256 TEXT NOT NULL,
		  code_path TEXT NOT NULL,
		  description TEXT NOT NULL DEFAULT '',
		  published_at TEXT NOT NULL,
		  PRIMARY KEY (account_id, layer_name, version)
		)`,
		`ALTER TABLE lambda_functions ADD COLUMN layers_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE lambda_versions ADD COLUMN layers_json TEXT NOT NULL DEFAULT '[]'`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				continue
			}
			return fmt.Errorf("ensure lambda layer schema: %w", err)
		}
	}
	return nil
}

func scanLambdaLayer(scan func(dest ...any) error) (LambdaLayer, error) {
	var layer LambdaLayer
	err := scan(
		&layer.AccountID, &layer.LayerName, &layer.Version, &layer.LayerARN,
		&layer.CodeSHA256, &layer.CodePath, &layer.Description, &layer.PublishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LambdaLayer{}, ErrNoSuchLayer
	}
	return layer, err
}

const lambdaLayerSelectCols = `account_id, layer_name, version, layer_version_arn, code_sha256, code_path, description, published_at`

// PublishLayerVersion stores a new immutable layer version.
func (s *Store) PublishLayerVersion(meta PublishLayerVersionMeta) (LambdaLayer, error) {
	if err := ValidateLayerName(meta.LayerName); err != nil {
		return LambdaLayer{}, err
	}
	if len(meta.Zip) == 0 {
		return LambdaLayer{}, fmt.Errorf("publish layer: zip bytes required")
	}
	var next int
	row := s.db.QueryRow(
		`SELECT COALESCE(MAX(version), 0) + 1 FROM lambda_layers WHERE account_id = ? AND layer_name = ?`,
		meta.AccountID, meta.LayerName,
	)
	if err := row.Scan(&next); err != nil {
		return LambdaLayer{}, fmt.Errorf("publish layer: %w", err)
	}
	codePath, shaHex, err := writeLambdaLayerZip(s.dataRoot, meta.AccountID, meta.LayerName, next, meta.Zip)
	if err != nil {
		return LambdaLayer{}, err
	}
	region := meta.Region
	if region == "" {
		region = DefaultLambdaRegion
	}
	layerARN := LayerVersionARN(meta.AccountID, region, meta.LayerName, next)
	published := nowRFC3339()
	_, err = s.db.Exec(
		`INSERT INTO lambda_layers
		 (account_id, layer_name, version, layer_version_arn, code_sha256, code_path, description, published_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		meta.AccountID, meta.LayerName, next, layerARN, shaHex, codePath, meta.Description, published,
	)
	if err != nil {
		_ = os.RemoveAll(filepath.Join(s.dataRoot, "lambda", meta.AccountID, "layers", meta.LayerName, "versions", fmt.Sprintf("%d", next)))
		return LambdaLayer{}, fmt.Errorf("publish layer: %w", err)
	}
	return LambdaLayer{
		AccountID:   meta.AccountID,
		LayerName:   meta.LayerName,
		Version:     next,
		LayerARN:    layerARN,
		CodeSHA256:  shaHex,
		CodePath:    codePath,
		Description: meta.Description,
		PublishedAt: published,
	}, nil
}

// GetLayerVersion returns a layer version or ErrNoSuchLayer.
func (s *Store) GetLayerVersion(accountID, name string, version int) (LambdaLayer, error) {
	if err := ValidateLayerName(name); err != nil {
		return LambdaLayer{}, err
	}
	if version <= 0 {
		return LambdaLayer{}, ErrNoSuchLayer
	}
	row := s.db.QueryRow(
		`SELECT `+lambdaLayerSelectCols+` FROM lambda_layers
		 WHERE account_id = ? AND layer_name = ? AND version = ?`,
		accountID, name, version,
	)
	layer, err := scanLambdaLayer(row.Scan)
	if err != nil {
		if errors.Is(err, ErrNoSuchLayer) {
			return LambdaLayer{}, err
		}
		return LambdaLayer{}, fmt.Errorf("get layer version: %w", err)
	}
	return layer, nil
}

// ListLayerVersions returns layer versions ordered by version number.
func (s *Store) ListLayerVersions(accountID, name string) ([]LambdaLayer, error) {
	if err := ValidateLayerName(name); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT `+lambdaLayerSelectCols+` FROM lambda_layers
		 WHERE account_id = ? AND layer_name = ? ORDER BY version`,
		accountID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("list layer versions: %w", err)
	}
	defer rows.Close()
	out := []LambdaLayer{}
	for rows.Next() {
		layer, err := scanLambdaLayer(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("list layer versions: %w", err)
		}
		out = append(out, layer)
	}
	return out, rows.Err()
}

// DeleteLayerVersion removes a layer version and on-disk artifacts.
func (s *Store) DeleteLayerVersion(accountID, name string, version int) error {
	if err := ValidateLayerName(name); err != nil {
		return err
	}
	if version <= 0 {
		return ErrNoSuchLayer
	}
	res, err := s.db.Exec(
		`DELETE FROM lambda_layers WHERE account_id = ? AND layer_name = ? AND version = ?`,
		accountID, name, version,
	)
	if err != nil {
		return fmt.Errorf("delete layer version: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchLayer
	}
	_ = os.RemoveAll(filepath.Join(s.dataRoot, "lambda", accountID, "layers", name, "versions", fmt.Sprintf("%d", version)))
	return nil
}

// LayerCodeDir returns the absolute unpacked layer directory.
func (s *Store) LayerCodeDir(accountID, name string, version int) string {
	return filepath.Join(s.dataRoot, "lambda", accountID, "layers", name, "versions", fmt.Sprintf("%d", version), "code")
}

// LayerCodeDirInContainer returns the DinD-visible path for a layer version.
func LayerCodeDirInContainer(dataRoot, accountID, name string, version int) string {
	return filepath.ToSlash(filepath.Join(dataRoot, "lambda", accountID, "layers", name, "versions", fmt.Sprintf("%d", version), "code"))
}

// ResolveLayerCodeDirs returns DinD-visible unpacked layer paths for function layer ARNs.
func (s *Store) ResolveLayerCodeDirs(dataRoot, accountID string, layerARNs []string) ([]string, error) {
	out := make([]string, 0, len(layerARNs))
	for _, arn := range layerARNs {
		layerAccount, _, name, version, ok := ParseLayerVersionARN(arn)
		if !ok || layerAccount != accountID {
			return nil, ErrInvalidLayerARN
		}
		if _, err := s.GetLayerVersion(accountID, name, version); err != nil {
			return nil, err
		}
		out = append(out, LayerCodeDirInContainer(dataRoot, accountID, name, version))
	}
	return out, nil
}
