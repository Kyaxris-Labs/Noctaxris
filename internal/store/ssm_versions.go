package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const (
	// MaxSSMParameterLabels is the AWS-like per-version label cap (lab lite).
	MaxSSMParameterLabels = 10
)

var (
	ErrParameterVersionNotFound           = errors.New("ParameterVersionNotFound")
	ErrParameterVersionLabelLimitExceeded = errors.New("ParameterVersionLabelLimitExceeded")
)

const ssmVersionsSchema = `
CREATE TABLE IF NOT EXISTS ssm_parameter_versions (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  version INTEGER NOT NULL,
  arn TEXT NOT NULL,
  param_type TEXT NOT NULL,
  value_plain TEXT NOT NULL DEFAULT '',
  value_sealed BLOB,
  sealed INTEGER NOT NULL DEFAULT 0,
  kms_key_id TEXT NOT NULL DEFAULT '',
  labels_json TEXT NOT NULL DEFAULT '[]',
  last_modified TEXT NOT NULL,
  PRIMARY KEY (account_id, name, version)
);
`

// ParameterHistory is one historical Put of a parameter, including labels.
type ParameterHistory struct {
	Name         string
	ARN          string
	Type         string
	Value        string
	Version      int
	LastModified string
	KeyID        string
	Labels       []string
}

// ParameterSelector resolves Name path (name:version / name:label) plus optional Version/Label fields.
type ParameterSelector struct {
	Name     string
	Version  int
	Label    string
	Selector string // ":N" or ":label" when a selector was used; empty for latest
}

func ensureSSMVersionsSchema(db *sql.DB) error {
	if _, err := db.Exec(ssmVersionsSchema); err != nil {
		return fmt.Errorf("ensure ssm versions schema: %w", err)
	}
	return backfillSSMParameterVersions(db)
}

func backfillSSMParameterVersions(db *sql.DB) error {
	rows, err := db.Query(
		`SELECT account_id, name, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, version, last_modified
		 FROM ssm_parameters p
		 WHERE NOT EXISTS (
		   SELECT 1 FROM ssm_parameter_versions v
		   WHERE v.account_id = p.account_id AND v.name = p.name AND v.version = p.version
		 )`,
	)
	if err != nil {
		return fmt.Errorf("backfill ssm versions: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			accountID, name, arn, paramType, plain, kmsKeyID, modified string
			version, sealed                                            int
			sealedB                                                    []byte
		)
		if err := rows.Scan(
			&accountID, &name, &arn, &paramType, &plain, &sealedB, &sealed, &kmsKeyID, &version, &modified,
		); err != nil {
			return fmt.Errorf("backfill ssm versions: scan: %w", err)
		}
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO ssm_parameter_versions
			 (account_id, name, version, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, labels_json, last_modified)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', ?)`,
			accountID, name, version, arn, paramType, plain, sealedB, sealed, kmsKeyID, modified,
		); err != nil {
			return fmt.Errorf("backfill ssm versions: insert %s v%d: %w", name, version, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backfill ssm versions: %w", err)
	}
	return nil
}

func (s *Store) insertParameterVersion(accountID string, row parameterRow, labels []string) error {
	if labels == nil {
		labels = []string{}
	}
	raw, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("insert parameter version: labels: %w", err)
	}
	sealed := 0
	if row.Sealed {
		sealed = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO ssm_parameter_versions
		 (account_id, name, version, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, labels_json, last_modified)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, row.Name, row.Version, row.ARN, row.Type, row.ValuePlain, row.ValueSealed, sealed, row.KMSKeyID, string(raw), row.LastModified,
	)
	if err != nil {
		return fmt.Errorf("insert parameter version %s v%d: %w", row.Name, row.Version, err)
	}
	return nil
}

func (s *Store) deleteParameterVersions(accountID, name string) error {
	name = normalizeParameterName(name)
	_, err := s.db.Exec(`DELETE FROM ssm_parameter_versions WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete parameter versions %s: %w", name, err)
	}
	return nil
}

// ParseParameterSelector splits Name (optionally name:version or name:label) and optional Version/Label params.
// Name-path selector wins over Version/Label fields when both are present.
func ParseParameterSelector(name string, versionParam int, labelParam string) ParameterSelector {
	name = strings.TrimSpace(name)
	labelParam = strings.TrimSpace(labelParam)
	out := ParameterSelector{Name: name}

	if !strings.HasPrefix(strings.ToLower(name), "arn:") {
		if base, sel, ok := splitParameterSelector(name); ok {
			out.Name = base
			if ver, err := strconv.Atoi(sel); err == nil && sel != "" && isAllDigits(sel) {
				out.Version = ver
				out.Selector = ":" + sel
			} else if sel != "" {
				out.Label = sel
				out.Selector = ":" + sel
			}
			return out
		}
	}

	out.Name = normalizeParameterName(name)
	if versionParam > 0 {
		out.Version = versionParam
		out.Selector = ":" + strconv.Itoa(versionParam)
		return out
	}
	if labelParam != "" {
		out.Label = labelParam
		out.Selector = ":" + labelParam
	}
	return out
}

func splitParameterSelector(name string) (base, selector string, ok bool) {
	idx := strings.LastIndex(name, ":")
	if idx <= 0 || idx+1 >= len(name) {
		return "", "", false
	}
	base = normalizeParameterName(name[:idx])
	selector = name[idx+1:]
	if base == "" || selector == "" {
		return "", "", false
	}
	// Avoid treating host:port-style noise; require version digits or label chars.
	if isAllDigits(selector) || isLikelyParameterLabel(selector) {
		return base, selector, true
	}
	return "", "", false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isLikelyParameterLabel(s string) bool {
	if s == "" || len(s) > 100 {
		return false
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// ValidSSMParameterLabel reports whether a label meets AWS Parameter Store rules (lab lite).
func ValidSSMParameterLabel(label string) bool {
	if label == "" || len(label) > 100 {
		return false
	}
	lower := strings.ToLower(label)
	if strings.HasPrefix(lower, "aws") || strings.HasPrefix(lower, "ssm") {
		return false
	}
	if unicode.IsDigit(rune(label[0])) {
		return false
	}
	for _, r := range label {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func decodeLabelsJSON(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	var labels []string
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		return nil, err
	}
	if labels == nil {
		labels = []string{}
	}
	return labels, nil
}

func (s *Store) getParameterVersionRow(accountID, name string, version int) (parameterRow, []string, error) {
	name = normalizeParameterName(name)
	var (
		row     parameterRow
		sealed  int
		sealedB []byte
		labels  string
	)
	err := s.db.QueryRow(
		`SELECT name, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, version, last_modified, labels_json
		 FROM ssm_parameter_versions WHERE account_id = ? AND name = ? AND version = ?`,
		accountID, name, version,
	).Scan(
		&row.Name, &row.ARN, &row.Type, &row.ValuePlain, &sealedB, &sealed, &row.KMSKeyID, &row.Version, &row.LastModified, &labels,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return parameterRow{}, nil, ErrParameterVersionNotFound
		}
		return parameterRow{}, nil, fmt.Errorf("get parameter version %s v%d: %w", name, version, err)
	}
	row.Sealed = sealed == 1
	row.ValueSealed = sealedB
	decoded, err := decodeLabelsJSON(labels)
	if err != nil {
		return parameterRow{}, nil, fmt.Errorf("get parameter version %s v%d: labels: %w", name, version, err)
	}
	return row, decoded, nil
}

func (s *Store) findParameterVersionByLabel(accountID, name, label string) (parameterRow, []string, error) {
	name = normalizeParameterName(name)
	rows, err := s.db.Query(
		`SELECT name, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, version, last_modified, labels_json
		 FROM ssm_parameter_versions WHERE account_id = ? AND name = ? ORDER BY version`,
		accountID, name,
	)
	if err != nil {
		return parameterRow{}, nil, fmt.Errorf("find parameter label %s: %w", name, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			row     parameterRow
			sealed  int
			sealedB []byte
			raw     string
		)
		if err := rows.Scan(
			&row.Name, &row.ARN, &row.Type, &row.ValuePlain, &sealedB, &sealed, &row.KMSKeyID, &row.Version, &row.LastModified, &raw,
		); err != nil {
			return parameterRow{}, nil, fmt.Errorf("find parameter label %s: %w", name, err)
		}
		labels, err := decodeLabelsJSON(raw)
		if err != nil {
			return parameterRow{}, nil, fmt.Errorf("find parameter label %s: %w", name, err)
		}
		for _, l := range labels {
			if l == label {
				row.Sealed = sealed == 1
				row.ValueSealed = sealedB
				return row, labels, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return parameterRow{}, nil, fmt.Errorf("find parameter label %s: %w", name, err)
	}
	return parameterRow{}, nil, ErrParameterVersionNotFound
}

// GetParameterResolved returns the current or selected parameter version.
func (s *Store) GetParameterResolved(accountID string, sel ParameterSelector, withDecryption bool) (Parameter, error) {
	sel.Name = normalizeParameterName(sel.Name)
	if sel.Name == "" {
		return Parameter{}, fmt.Errorf("get parameter: name is required")
	}

	if sel.Version == 0 && sel.Label == "" {
		p, err := s.GetParameter(accountID, sel.Name, withDecryption)
		if err != nil {
			return Parameter{}, err
		}
		return p, nil
	}

	if _, err := s.getParameterRow(accountID, sel.Name); err != nil {
		return Parameter{}, err
	}

	var (
		row    parameterRow
		labels []string
		err    error
	)
	switch {
	case sel.Version > 0:
		row, labels, err = s.getParameterVersionRow(accountID, sel.Name, sel.Version)
	default:
		row, labels, err = s.findParameterVersionByLabel(accountID, sel.Name, sel.Label)
	}
	if err != nil {
		return Parameter{}, err
	}
	p, err := s.parameterFromRow(row, withDecryption)
	if err != nil {
		return Parameter{}, err
	}
	p.Labels = labels
	p.Selector = sel.Selector
	return p, nil
}

// GetParametersResolved resolves each name with optional :version / :label selectors.
// Missing parameters are skipped (caller builds InvalidParameters). Version/label misses are also skipped.
func (s *Store) GetParametersResolved(accountID string, names []string, withDecryption bool) ([]Parameter, error) {
	out := make([]Parameter, 0, len(names))
	for _, raw := range names {
		sel := ParseParameterSelector(raw, 0, "")
		p, err := s.GetParameterResolved(accountID, sel, withDecryption)
		if errors.Is(err, ErrParameterNotFound) || errors.Is(err, ErrParameterVersionNotFound) {
			continue
		}
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

// LabelParameterVersionResult is the LabelParameterVersion response payload.
type LabelParameterVersionResult struct {
	ParameterVersion int
	InvalidLabels    []string
}

// LabelParameterVersion attaches labels to a parameter version (AWS-like lite move/overwrite).
// ParameterVersion 0 means latest. Invalid labels are returned without failing the call.
func (s *Store) LabelParameterVersion(
	accountID, name string, parameterVersion int, labels []string,
) (LabelParameterVersionResult, error) {
	name = normalizeParameterName(name)
	if name == "" {
		return LabelParameterVersionResult{}, fmt.Errorf("label parameter version: name is required")
	}
	if len(labels) == 0 {
		return LabelParameterVersionResult{}, fmt.Errorf("ValidationException: Labels is required")
	}

	current, err := s.getParameterRow(accountID, name)
	if err != nil {
		return LabelParameterVersionResult{}, err
	}
	if parameterVersion <= 0 {
		parameterVersion = current.Version
	}

	target, existingLabels, err := s.getParameterVersionRow(accountID, name, parameterVersion)
	if err != nil {
		return LabelParameterVersionResult{}, err
	}
	_ = target

	var (
		invalid []string
		toApply []string
	)
	seenApply := map[string]struct{}{}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if !ValidSSMParameterLabel(label) {
			invalid = append(invalid, label)
			continue
		}
		if _, ok := seenApply[label]; ok {
			continue
		}
		seenApply[label] = struct{}{}
		toApply = append(toApply, label)
	}

	// Count labels that would be newly attached to the target (already present do not count).
	existingSet := map[string]struct{}{}
	for _, l := range existingLabels {
		existingSet[l] = struct{}{}
	}
	newCount := 0
	for _, label := range toApply {
		if _, ok := existingSet[label]; !ok {
			newCount++
		}
	}
	if len(existingLabels)+newCount > MaxSSMParameterLabels {
		return LabelParameterVersionResult{}, ErrParameterVersionLabelLimitExceeded
	}

	// Move each label off other versions, then attach to target.
	for _, label := range toApply {
		if err := s.clearParameterLabel(accountID, name, label); err != nil {
			return LabelParameterVersionResult{}, err
		}
	}

	merged := make([]string, 0, len(existingLabels)+len(toApply))
	mergedSet := map[string]struct{}{}
	for _, l := range existingLabels {
		if _, ok := mergedSet[l]; ok {
			continue
		}
		mergedSet[l] = struct{}{}
		merged = append(merged, l)
	}
	for _, label := range toApply {
		if _, ok := mergedSet[label]; ok {
			continue
		}
		mergedSet[label] = struct{}{}
		merged = append(merged, label)
	}
	if err := s.setParameterVersionLabels(accountID, name, parameterVersion, merged); err != nil {
		return LabelParameterVersionResult{}, err
	}

	if invalid == nil {
		invalid = []string{}
	}
	return LabelParameterVersionResult{
		ParameterVersion: parameterVersion,
		InvalidLabels:    invalid,
	}, nil
}

func (s *Store) clearParameterLabel(accountID, name, label string) error {
	rows, err := s.db.Query(
		`SELECT version, labels_json FROM ssm_parameter_versions WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("clear parameter label: %w", err)
	}
	defer rows.Close()

	type hit struct {
		version int
		labels  []string
	}
	var updates []hit
	for rows.Next() {
		var (
			version int
			raw     string
		)
		if err := rows.Scan(&version, &raw); err != nil {
			return fmt.Errorf("clear parameter label: %w", err)
		}
		labels, err := decodeLabelsJSON(raw)
		if err != nil {
			return fmt.Errorf("clear parameter label: %w", err)
		}
		filtered := make([]string, 0, len(labels))
		changed := false
		for _, l := range labels {
			if l == label {
				changed = true
				continue
			}
			filtered = append(filtered, l)
		}
		if changed {
			updates = append(updates, hit{version: version, labels: filtered})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("clear parameter label: %w", err)
	}
	for _, u := range updates {
		if err := s.setParameterVersionLabels(accountID, name, u.version, u.labels); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) setParameterVersionLabels(accountID, name string, version int, labels []string) error {
	if labels == nil {
		labels = []string{}
	}
	raw, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("set parameter labels: %w", err)
	}
	res, err := s.db.Exec(
		`UPDATE ssm_parameter_versions SET labels_json = ? WHERE account_id = ? AND name = ? AND version = ?`,
		string(raw), accountID, name, version,
	)
	if err != nil {
		return fmt.Errorf("set parameter labels %s v%d: %w", name, version, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set parameter labels %s v%d: %w", name, version, err)
	}
	if n == 0 {
		return ErrParameterVersionNotFound
	}
	return nil
}

// GetParameterHistory returns all stored versions (oldest first) with labels.
func (s *Store) GetParameterHistory(accountID, name string, withDecryption bool) ([]ParameterHistory, error) {
	name = normalizeParameterName(name)
	if name == "" {
		return nil, fmt.Errorf("get parameter history: name is required")
	}
	if _, err := s.getParameterRow(accountID, name); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT name, arn, param_type, value_plain, value_sealed, sealed, kms_key_id, version, last_modified, labels_json
		 FROM ssm_parameter_versions WHERE account_id = ? AND name = ? ORDER BY version`,
		accountID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("get parameter history %s: %w", name, err)
	}
	defer rows.Close()

	var out []ParameterHistory
	for rows.Next() {
		var (
			row     parameterRow
			sealed  int
			sealedB []byte
			raw     string
		)
		if err := rows.Scan(
			&row.Name, &row.ARN, &row.Type, &row.ValuePlain, &sealedB, &sealed, &row.KMSKeyID, &row.Version, &row.LastModified, &raw,
		); err != nil {
			return nil, fmt.Errorf("get parameter history %s: %w", name, err)
		}
		row.Sealed = sealed == 1
		row.ValueSealed = sealedB
		labels, err := decodeLabelsJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("get parameter history %s: labels: %w", name, err)
		}
		p, err := s.parameterFromRow(row, withDecryption)
		if err != nil {
			return nil, err
		}
		out = append(out, ParameterHistory{
			Name:         p.Name,
			ARN:          p.ARN,
			Type:         p.Type,
			Value:        p.Value,
			Version:      p.Version,
			LastModified: p.LastModified,
			KeyID:        p.KeyID,
			Labels:       labels,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get parameter history %s: %w", name, err)
	}
	if out == nil {
		out = []ParameterHistory{}
	}
	return out, nil
}
