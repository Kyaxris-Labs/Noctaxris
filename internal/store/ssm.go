package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	// DefaultSSMRegion is the lab region embedded in SSM parameter ARNs.
	DefaultSSMRegion = "us-east-1"

	ParamTypeString       = "String"
	ParamTypeStringList   = "StringList"
	ParamTypeSecureString = "SecureString"

	// AliasAWSSSM is the lab convenience alias for SSM SecureString defaults.
	AliasAWSSSM = "alias/aws/ssm"
)

var (
	ErrParameterAlreadyExists = errors.New("ParameterAlreadyExists")
	ErrParameterNotFound      = errors.New("ParameterNotFound")
)

// Parameter is an SSM parameter metadata row. Value is plaintext when returned
// from Get* with decryption enabled.
type Parameter struct {
	Name         string
	ARN          string
	Type         string
	Value        string
	Version      int
	LastModified string
	KeyID        string
}

const ssmSchema = `
CREATE TABLE IF NOT EXISTS ssm_parameters (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  param_type TEXT NOT NULL,
  value_plain TEXT NOT NULL DEFAULT '',
  value_sealed BLOB,
  sealed INTEGER NOT NULL DEFAULT 0,
  kms_key_id TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL DEFAULT 1,
  last_modified TEXT NOT NULL,
  PRIMARY KEY (account_id, name)
);
`

// EnsureSSMSchema creates the SSM parameters table if missing.
func EnsureSSMSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure ssm schema: db is nil")
	}
	if _, err := db.Exec(ssmSchema); err != nil {
		return fmt.Errorf("ensure ssm schema: %w", err)
	}
	return nil
}

// EnsureSSMSchema ensures SSM tables on an open store (tests and Open wiring).
func (s *Store) EnsureSSMSchema() error {
	return EnsureSSMSchema(s.db)
}

// ParameterARN builds arn:aws:ssm:REGION:ACCOUNT:parameter/NAME (name normalized with leading slash).
func ParameterARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultSSMRegion
	}
	n := normalizeParameterName(name)
	return fmt.Sprintf("arn:aws:ssm:%s:%s:parameter%s", region, accountID, n)
}

// SSMEncryptionContext is the AWS Parameter Store KMS EncryptionContext map
// (PARAMETER_ARN) used for SecureString seal/unseal and EvaluateKMS conditions.
func SSMEncryptionContext(parameterARN string) map[string]string {
	return map[string]string{
		"PARAMETER_ARN": parameterARN,
	}
}

func normalizeParameterName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	return name
}

// validateStringListValue enforces AWS StringList Value: comma-separated members, no empties.
func validateStringListValue(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("ValidationException: StringList Value is required")
	}
	parts := strings.Split(value, ",")
	for _, p := range parts {
		if p == "" {
			return fmt.Errorf("ValidationException: StringList Value members must not be empty")
		}
	}
	return nil
}

// ParameterValueIncluded reports whether Get* responses include Value without WithDecryption.
func ParameterValueIncluded(paramType string, withDecryption bool) bool {
	switch paramType {
	case ParamTypeString, ParamTypeStringList:
		return true
	case ParamTypeSecureString:
		return withDecryption
	default:
		return withDecryption
	}
}

// EnsureSSMAlias seeds alias/aws/ssm for the account, creating a lab CMK when missing.
// Idempotent: returns the target key id when the alias already exists.
func (s *Store) EnsureSSMAlias(accountID string) (string, error) {
	if strings.TrimSpace(accountID) == "" {
		return "", fmt.Errorf("ensure ssm alias: account id required")
	}

	keyID, err := s.lookupAliasTarget(accountID, AliasAWSSSM)
	if err == nil {
		return keyID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("ensure ssm alias: lookup: %w", err)
	}

	k, err := s.CreateKey(accountID, fmt.Sprintf("arn:aws:iam::%s:root", accountID), "")
	if err != nil {
		return "", fmt.Errorf("ensure ssm alias: create key: %w", err)
	}
	if err := s.CreateAlias(accountID, AliasAWSSSM, k.KeyID); err != nil {
		return "", fmt.Errorf("ensure ssm alias: create alias: %w", err)
	}
	return k.KeyID, nil
}

// ResolveSSMKeyID resolves KeyId for SecureString or defaults to alias/aws/ssm.
func (s *Store) ResolveSSMKeyID(accountID, keyIDOrAlias string) (string, error) {
	id := strings.TrimSpace(keyIDOrAlias)
	if id == "" || id == AliasAWSSSM {
		return s.EnsureSSMAlias(accountID)
	}
	if strings.HasPrefix(id, "alias/aws/ssm") {
		if _, err := s.EnsureSSMAlias(accountID); err != nil {
			return "", err
		}
	}
	return s.ResolveKeyID(accountID, id)
}

func (s *Store) resolveSSMKeyID(accountID, keyIDOrAlias string) (string, error) {
	return s.ResolveSSMKeyID(accountID, keyIDOrAlias)
}

// PutParameter stores a String, StringList, or SecureString parameter. keyID is
// optional for SecureString and defaults to alias/aws/ssm. overwrite replaces an existing name.
// StringList Value is a comma-separated list (AWS Parameter Store shape).
func (s *Store) PutParameter(
	accountID, region, name, paramType, value, keyID string, overwrite bool,
) (Parameter, error) {
	name = normalizeParameterName(name)
	if name == "" {
		return Parameter{}, fmt.Errorf("put parameter: name is required")
	}
	switch paramType {
	case ParamTypeString, ParamTypeStringList, ParamTypeSecureString:
	default:
		return Parameter{}, fmt.Errorf("ValidationException: unsupported type %q", paramType)
	}
	if paramType == ParamTypeStringList {
		if err := validateStringListValue(value); err != nil {
			return Parameter{}, err
		}
	}
	if region == "" {
		region = DefaultSSMRegion
	}

	existing, lookupErr := s.getParameterRow(accountID, name)
	exists := lookupErr == nil
	if exists {
		if !overwrite {
			return Parameter{}, ErrParameterAlreadyExists
		}
	} else if !errors.Is(lookupErr, ErrParameterNotFound) {
		return Parameter{}, lookupErr
	}

	modified := nowRFC3339()
	arn := ParameterARN(region, accountID, name)
	version := 1
	if exists {
		version = existing.Version + 1
	}

	var (
		plain      string
		sealed     []byte
		sealedFlag int
		kmsKeyID   string
	)

	switch paramType {
	case ParamTypeString, ParamTypeStringList:
		plain = value
	case ParamTypeSecureString:
		resolvedKeyID, err := s.resolveSSMKeyID(accountID, keyID)
		if err != nil {
			return Parameter{}, fmt.Errorf("put parameter: resolve key: %w", err)
		}
		k, err := s.GetKey(resolvedKeyID)
		if err != nil {
			return Parameter{}, fmt.Errorf("put parameter: key: %w", err)
		}
		if !KeyUsableForCrypto(k.KeyState) {
			return Parameter{}, fmt.Errorf("put parameter: %w", ErrInvalidKeyState)
		}
		cmk, err := s.UnsealKeyMaterial(resolvedKeyID)
		if err != nil {
			return Parameter{}, fmt.Errorf("put parameter: unseal key: %w", err)
		}
		encCtx := SSMEncryptionContext(arn)
		sealed, err = EncryptUnderCMK(cmk, resolvedKeyID, []byte(value), encCtx)
		if err != nil {
			return Parameter{}, fmt.Errorf("put parameter: encrypt: %w", err)
		}
		sealedFlag = 1
		kmsKeyID = resolvedKeyID
	}

	var execErr error
	if exists {
		_, execErr = s.db.Exec(
			`UPDATE ssm_parameters
			 SET arn = ?, param_type = ?, value_plain = ?, value_sealed = ?, sealed = ?,
			     kms_key_id = ?, version = ?, last_modified = ?
			 WHERE account_id = ? AND name = ?`,
			arn, paramType, plain, sealed, sealedFlag, kmsKeyID, version, modified, accountID, name,
		)
	} else {
		_, execErr = s.db.Exec(
			`INSERT INTO ssm_parameters
			 (account_id, name, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, version, last_modified)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			accountID, name, arn, paramType, plain, sealed, sealedFlag, kmsKeyID, version, modified,
		)
	}
	if execErr != nil {
		return Parameter{}, fmt.Errorf("put parameter %s: %w", name, execErr)
	}

	return Parameter{
		Name:         name,
		ARN:          arn,
		Type:         paramType,
		Value:        value,
		Version:      version,
		LastModified: modified,
		KeyID:        kmsKeyID,
	}, nil
}

type parameterRow struct {
	Name         string
	ARN          string
	Type         string
	ValuePlain   string
	ValueSealed  []byte
	Sealed       bool
	KMSKeyID     string
	Version      int
	LastModified string
}

func (s *Store) getParameterRow(accountID, name string) (parameterRow, error) {
	name = normalizeParameterName(name)
	var (
		row      parameterRow
		sealed   int
		sealedB  []byte
	)
	err := s.db.QueryRow(
		`SELECT name, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, version, last_modified
		 FROM ssm_parameters WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&row.Name, &row.ARN, &row.Type, &row.ValuePlain, &sealedB, &sealed, &row.KMSKeyID, &row.Version, &row.LastModified)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return parameterRow{}, ErrParameterNotFound
		}
		return parameterRow{}, fmt.Errorf("get parameter row %s: %w", name, err)
	}
	row.Sealed = sealed == 1
	row.ValueSealed = sealedB
	return row, nil
}

func (s *Store) parameterFromRow(row parameterRow, withDecryption bool) (Parameter, error) {
	out := Parameter{
		Name:         row.Name,
		ARN:          row.ARN,
		Type:         row.Type,
		Version:      row.Version,
		LastModified: row.LastModified,
		KeyID:        row.KMSKeyID,
	}
	if !row.Sealed {
		out.Value = row.ValuePlain
		return out, nil
	}
	if !withDecryption {
		return out, nil
	}
	if row.KMSKeyID == "" {
		return Parameter{}, fmt.Errorf("parameter %s: sealed without kms key id", row.Name)
	}
	k, err := s.GetKey(row.KMSKeyID)
	if err != nil {
		return Parameter{}, fmt.Errorf("parameter %s: key: %w", row.Name, err)
	}
	if !KeyUsableForCrypto(k.KeyState) {
		return Parameter{}, fmt.Errorf("parameter %s: %w", row.Name, ErrInvalidKeyState)
	}
	encCtx := SSMEncryptionContext(row.ARN)
	plain, err := s.DecryptBlobWithKeyContext(row.KMSKeyID, row.ValueSealed, encCtx)
	if err != nil {
		return Parameter{}, fmt.Errorf("parameter %s: decrypt: %w", row.Name, err)
	}
	out.Value = string(plain)
	return out, nil
}

// GetParameter returns a parameter by name. withDecryption decrypts SecureString values.
func (s *Store) GetParameter(accountID, name string, withDecryption bool) (Parameter, error) {
	row, err := s.getParameterRow(accountID, name)
	if err != nil {
		return Parameter{}, err
	}
	return s.parameterFromRow(row, withDecryption)
}

// GetParameters returns parameters for the given names. Missing names are skipped.
func (s *Store) GetParameters(accountID string, names []string, withDecryption bool) ([]Parameter, error) {
	out := make([]Parameter, 0, len(names))
	for _, name := range names {
		row, err := s.getParameterRow(accountID, name)
		if errors.Is(err, ErrParameterNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		p, err := s.parameterFromRow(row, withDecryption)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []Parameter{}
	}
	return out, nil
}

// GetParametersByPath returns parameters under a hierarchy path.
// When recursive is false, only immediate children (one extra path segment) are returned.
func (s *Store) GetParametersByPath(accountID, path string, recursive, withDecryption bool) ([]Parameter, error) {
	path = normalizeParameterName(path)
	if path == "" || path == "/" {
		return nil, fmt.Errorf("ValidationException: Path is required")
	}
	prefix := path
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	rows, err := s.db.Query(
		`SELECT name, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, version, last_modified
		 FROM ssm_parameters WHERE account_id = ? AND name LIKE ? ORDER BY name`,
		accountID, prefix+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("get parameters by path %s: %w", path, err)
	}
	defer rows.Close()

	var out []Parameter
	for rows.Next() {
		var (
			row     parameterRow
			sealed  int
			sealedB []byte
		)
		if err := rows.Scan(&row.Name, &row.ARN, &row.Type, &row.ValuePlain, &sealedB, &sealed, &row.KMSKeyID, &row.Version, &row.LastModified); err != nil {
			return nil, fmt.Errorf("get parameters by path %s: %w", path, err)
		}
		row.Sealed = sealed == 1
		row.ValueSealed = sealedB
		rest := strings.TrimPrefix(row.Name, prefix)
		if rest == "" || rest == row.Name {
			continue
		}
		if !recursive && strings.Contains(rest, "/") {
			continue
		}
		p, err := s.parameterFromRow(row, withDecryption)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get parameters by path %s: %w", path, err)
	}
	if out == nil {
		out = []Parameter{}
	}
	return out, nil
}

// DeleteParameter removes a parameter by name.
func (s *Store) DeleteParameter(accountID, name string) error {
	name = normalizeParameterName(name)
	res, err := s.db.Exec(`DELETE FROM ssm_parameters WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete parameter %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete parameter %s: %w", name, err)
	}
	if affected == 0 {
		return ErrParameterNotFound
	}
	return nil
}

// DescribeParameters lists parameter metadata for an account. namePrefix filters by
// normalized name prefix when non-empty. Values are omitted.
func (s *Store) DescribeParameters(accountID, namePrefix string) ([]Parameter, error) {
	prefix := normalizeParameterName(namePrefix)
	query := `SELECT name, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, version, last_modified
	          FROM ssm_parameters WHERE account_id = ?`
	args := []any{accountID}
	if prefix != "" {
		query += ` AND name LIKE ?`
		args = append(args, prefix+"%")
	}
	query += ` ORDER BY name`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("describe parameters %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []Parameter
	for rows.Next() {
		var (
			row     parameterRow
			sealed  int
			sealedB []byte
		)
		if err := rows.Scan(&row.Name, &row.ARN, &row.Type, &row.ValuePlain, &sealedB, &sealed, &row.KMSKeyID, &row.Version, &row.LastModified); err != nil {
			return nil, fmt.Errorf("describe parameters %s: %w", accountID, err)
		}
		out = append(out, Parameter{
			Name:         row.Name,
			ARN:          row.ARN,
			Type:         row.Type,
			Version:      row.Version,
			LastModified: row.LastModified,
			KeyID:        row.KMSKeyID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("describe parameters %s: %w", accountID, err)
	}
	if out == nil {
		out = []Parameter{}
	}
	return out, nil
}
