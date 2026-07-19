package validate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
)

var (
	v = validator.New()

	// IAM resource names follow AWS IAM name constraints.
	iamNamePattern = regexp.MustCompile(`^[\w+=,.@-]+$`)
)

// ErrInvalid is returned when an input fails validation.
var ErrInvalid = errors.New("invalid input")

func wrap(field string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %v", ErrInvalid, field, err)
}

// AccountID checks a 12-digit AWS account id.
func AccountID(id string) error {
	if err := v.Var(id, "required,len=12,numeric"); err != nil {
		return wrap("AccountId", err)
	}
	return nil
}

// IAMName validates an IAM user, role, or policy name.
func IAMName(name string) error {
	if err := v.Var(name, "required,min=1,max=64"); err != nil {
		return wrap("Name", err)
	}
	if !iamNamePattern.MatchString(name) {
		return fmt.Errorf("%w: Name: must match IAM name pattern", ErrInvalid)
	}
	return nil
}

// Email validates an email address for Organizations CreateAccount.
func Email(addr string) error {
	if err := v.Var(addr, "required,email"); err != nil {
		return wrap("Email", err)
	}
	return nil
}

// AccountName validates an Organizations account name.
func AccountName(name string) error {
	if err := v.Var(name, "required,min=1,max=50"); err != nil {
		return wrap("AccountName", err)
	}
	return nil
}

// OIDCIssuerURL validates an OIDC issuer URL (http or https).
func OIDCIssuerURL(raw string) error {
	if err := v.Var(raw, "required,url"); err != nil {
		return wrap("OIDCIssuerURL", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return wrap("OIDCIssuerURL", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("%w: OIDCIssuerURL: scheme must be http or https", ErrInvalid)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: OIDCIssuerURL: host required", ErrInvalid)
	}
	return nil
}

// OIDCClientID validates an OIDC audience / client id.
func OIDCClientID(id string) error {
	if err := v.Var(id, "required,min=1,max=255"); err != nil {
		return wrap("OIDCClientID", err)
	}
	return nil
}

// AccessKeyID validates a long-lived or temporary access key id shape.
func AccessKeyID(id string) error {
	if err := v.Var(id, "required,min=16,max=128,alphanum"); err != nil {
		return wrap("AccessKeyId", err)
	}
	return nil
}

// RoleARN checks a basic IAM role ARN shape.
func RoleARN(arn string) error {
	if err := v.Var(arn, "required,startswith=arn:aws:iam::"); err != nil {
		return wrap("RoleArn", err)
	}
	if !strings.Contains(arn, ":role/") {
		return fmt.Errorf("%w: RoleArn: must contain :role/", ErrInvalid)
	}
	return nil
}

// PolicyDocument ensures the document is JSON with Version and Statement.
func PolicyDocument(doc string) error {
	if strings.TrimSpace(doc) == "" {
		return fmt.Errorf("%w: PolicyDocument: required", ErrInvalid)
	}
	var parsed struct {
		Version   string          `json:"Version"`
		Statement json.RawMessage `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(doc), &parsed); err != nil {
		return fmt.Errorf("%w: PolicyDocument: invalid JSON: %v", ErrInvalid, err)
	}
	if parsed.Version == "" {
		return fmt.Errorf("%w: PolicyDocument: Version required", ErrInvalid)
	}
	if len(parsed.Statement) == 0 || string(parsed.Statement) == "null" {
		return fmt.Errorf("%w: PolicyDocument: Statement required", ErrInvalid)
	}
	return nil
}

// ReadableFilePath cleans path, rejects empty/null bytes, and ensures the file exists and is regular.
func ReadableFilePath(path string) (cleaned string, err error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("%w: path: empty or contains null byte", ErrInvalid)
	}
	cleaned = filepath.Clean(path)
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("%w: path: invalid", ErrInvalid)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: path: %v", ErrInvalid, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: path: not a regular file", ErrInvalid)
	}
	return cleaned, nil
}

// IsInvalid reports whether err wraps ErrInvalid.
func IsInvalid(err error) bool {
	return errors.Is(err, ErrInvalid)
}
