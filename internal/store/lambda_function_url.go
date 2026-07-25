package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// FunctionURLAuthNone is AuthType NONE (public lab endpoint on loopback).
	FunctionURLAuthNone = "NONE"
	// FunctionURLAuthIAM is AuthType AWS_IAM.
	FunctionURLAuthIAM = "AWS_IAM"
)

var (
	// ErrNoSuchFunctionURL is returned when a function has no URL config.
	ErrNoSuchFunctionURL = errors.New("ResourceNotFoundException")
	// ErrFunctionURLExists is returned when CreateFunctionUrlConfig collides.
	ErrFunctionURLExists = errors.New("ResourceConflictException")
	// ErrInvalidFunctionURLAuthType is returned for unknown AuthType values.
	ErrInvalidFunctionURLAuthType = errors.New("ValidationException: AuthType must be NONE or AWS_IAM")
)

// LambdaFunctionURL is a function URL configuration row.
type LambdaFunctionURL struct {
	AccountID    string
	FunctionName string
	FunctionARN  string
	AuthType     string
	FunctionURL  string
	CreationTime string
	// CorsAllowOrigins is empty for lab default (*). When set, only listed origins are echoed.
	CorsAllowOrigins []string
}

// CreateFunctionURLInput holds CreateFunctionUrlConfig fields.
type CreateFunctionURLInput struct {
	AccountID    string
	FunctionName string
	AuthType     string
	// EndpointHost is used to build the lab FunctionUrl (e.g. 127.0.0.1:4566).
	EndpointHost string
	// CorsAllowOrigins from Cors.AllowOrigins or env default.
	CorsAllowOrigins []string
}

const lambdaFunctionURLSchema = `
CREATE TABLE IF NOT EXISTS lambda_function_urls (
  account_id TEXT NOT NULL,
  function_name TEXT NOT NULL,
  function_arn TEXT NOT NULL,
  auth_type TEXT NOT NULL,
  function_url TEXT NOT NULL,
  creation_time TEXT NOT NULL,
  cors_allow_origins_json TEXT NOT NULL DEFAULT '[]',
  PRIMARY KEY (account_id, function_name)
);
`

// EnsureLambdaFunctionURLSchema creates the function URL table.
func EnsureLambdaFunctionURLSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure lambda function url schema: db is nil")
	}
	if _, err := db.Exec(lambdaFunctionURLSchema); err != nil {
		return fmt.Errorf("ensure lambda function url schema: %w", err)
	}
	if err := execMigrateStmt(db, `ALTER TABLE lambda_function_urls ADD COLUMN cors_allow_origins_json TEXT NOT NULL DEFAULT '[]'`, nil); err != nil {
		return fmt.Errorf("ensure lambda function url schema alter: %w", err)
	}
	return nil
}

// EnsureLambdaFunctionURLSchema ensures function URL tables on an open store.
func (s *Store) EnsureLambdaFunctionURLSchema() error {
	return EnsureLambdaFunctionURLSchema(s.db)
}

func normalizeFunctionURLAuthType(authType string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(authType)) {
	case FunctionURLAuthNone:
		return FunctionURLAuthNone, nil
	case FunctionURLAuthIAM:
		return FunctionURLAuthIAM, nil
	default:
		return "", ErrInvalidFunctionURLAuthType
	}
}

// LabFunctionURL builds http://HOST/lambda-url/ACCOUNT/FUNCTION.
func LabFunctionURL(endpointHost, accountID, functionName string) string {
	host := strings.TrimSpace(endpointHost)
	if host == "" {
		host = "127.0.0.1:4566"
	}
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	return fmt.Sprintf("http://%s/lambda-url/%s/%s", host, accountID, functionName)
}

// CreateFunctionURLConfig creates a function URL for $LATEST.
func (s *Store) CreateFunctionURLConfig(in CreateFunctionURLInput) (LambdaFunctionURL, error) {
	base, _ := ParseFunctionQualifier(strings.TrimSpace(in.FunctionName))
	if err := ValidateFunctionName(base); err != nil {
		return LambdaFunctionURL{}, err
	}
	fn, err := s.GetFunction(in.AccountID, base)
	if err != nil {
		return LambdaFunctionURL{}, err
	}
	authType, err := normalizeFunctionURLAuthType(in.AuthType)
	if err != nil {
		return LambdaFunctionURL{}, err
	}
	if _, err := s.GetFunctionURLConfig(in.AccountID, base); err == nil {
		return LambdaFunctionURL{}, ErrFunctionURLExists
	} else if !errors.Is(err, ErrNoSuchFunctionURL) {
		return LambdaFunctionURL{}, err
	}
	url := LabFunctionURL(in.EndpointHost, in.AccountID, base)
	created := nowRFC3339()
	origins := normalizeCorsAllowOrigins(in.CorsAllowOrigins)
	originsJSON, err := json.Marshal(origins)
	if err != nil {
		return LambdaFunctionURL{}, fmt.Errorf("marshal cors origins: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO lambda_function_urls
		 (account_id, function_name, function_arn, auth_type, function_url, creation_time, cors_allow_origins_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		in.AccountID, base, fn.FunctionARN, authType, url, created, string(originsJSON),
	)
	if err != nil {
		return LambdaFunctionURL{}, fmt.Errorf("create function url config: %w", err)
	}
	return LambdaFunctionURL{
		AccountID:        in.AccountID,
		FunctionName:     base,
		FunctionARN:      fn.FunctionARN,
		AuthType:         authType,
		FunctionURL:      url,
		CreationTime:     created,
		CorsAllowOrigins: origins,
	}, nil
}

func normalizeCorsAllowOrigins(origins []string) []string {
	if len(origins) == 0 {
		return nil
	}
	out := make([]string, 0, len(origins))
	seen := map[string]struct{}{}
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if _, ok := seen[o]; ok {
			continue
		}
		seen[o] = struct{}{}
		out = append(out, o)
	}
	return out
}

func scanFunctionURL(row interface {
	Scan(dest ...any) error
}) (LambdaFunctionURL, error) {
	var u LambdaFunctionURL
	var corsJSON string
	err := row.Scan(&u.AccountID, &u.FunctionName, &u.FunctionARN, &u.AuthType, &u.FunctionURL, &u.CreationTime, &corsJSON)
	if err != nil {
		return LambdaFunctionURL{}, err
	}
	_ = json.Unmarshal([]byte(corsJSON), &u.CorsAllowOrigins)
	return u, nil
}

// GetFunctionURLConfig returns the URL config for a function.
func (s *Store) GetFunctionURLConfig(accountID, functionName string) (LambdaFunctionURL, error) {
	base, _ := ParseFunctionQualifier(strings.TrimSpace(functionName))
	row := s.db.QueryRow(
		`SELECT account_id, function_name, function_arn, auth_type, function_url, creation_time,
		        COALESCE(cors_allow_origins_json, '[]')
		 FROM lambda_function_urls WHERE account_id = ? AND function_name = ?`,
		accountID, base,
	)
	u, err := scanFunctionURL(row)
	if errors.Is(err, sql.ErrNoRows) {
		return LambdaFunctionURL{}, ErrNoSuchFunctionURL
	}
	if err != nil {
		return LambdaFunctionURL{}, fmt.Errorf("get function url config: %w", err)
	}
	return u, nil
}

// ListFunctionURLConfigs returns URL configs for an account, optionally filtered by function.
func (s *Store) ListFunctionURLConfigs(accountID, functionName string) ([]LambdaFunctionURL, error) {
	functionName = strings.TrimSpace(functionName)
	var (
		rows *sql.Rows
		err  error
	)
	if functionName == "" {
		rows, err = s.db.Query(
			`SELECT account_id, function_name, function_arn, auth_type, function_url, creation_time,
			        COALESCE(cors_allow_origins_json, '[]')
			 FROM lambda_function_urls WHERE account_id = ? ORDER BY function_name`,
			accountID,
		)
	} else {
		base, _ := ParseFunctionQualifier(functionName)
		rows, err = s.db.Query(
			`SELECT account_id, function_name, function_arn, auth_type, function_url, creation_time,
			        COALESCE(cors_allow_origins_json, '[]')
			 FROM lambda_function_urls WHERE account_id = ? AND function_name = ?`,
			accountID, base,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list function url configs: %w", err)
	}
	defer rows.Close()
	out := []LambdaFunctionURL{}
	for rows.Next() {
		u, err := scanFunctionURL(rows)
		if err != nil {
			return nil, fmt.Errorf("list function url configs: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteFunctionURLConfig removes a function URL config.
func (s *Store) DeleteFunctionURLConfig(accountID, functionName string) error {
	base, _ := ParseFunctionQualifier(strings.TrimSpace(functionName))
	res, err := s.db.Exec(
		`DELETE FROM lambda_function_urls WHERE account_id = ? AND function_name = ?`,
		accountID, base,
	)
	if err != nil {
		return fmt.Errorf("delete function url config: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoSuchFunctionURL
	}
	return nil
}
