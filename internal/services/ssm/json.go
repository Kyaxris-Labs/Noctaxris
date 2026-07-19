package ssm

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// PutParameterJSON builds a PutParameter response.
func PutParameterJSON(version int) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Version": version,
		"Tier":    "Standard",
	})
}

// GetParameterJSON builds a GetParameter response.
func GetParameterJSON(p store.Parameter, includeValue bool) ([]byte, error) {
	entry, err := parameterJSON(p, includeValue)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"Parameter": entry})
}

// GetParametersJSON builds a GetParameters response.
func GetParametersJSON(params []store.Parameter, invalid []string, withDecryption bool) ([]byte, error) {
	entries := make([]map[string]any, 0, len(params))
	for _, p := range params {
		includeValue := p.Type == store.ParamTypeString || withDecryption
		entry, err := parameterJSON(p, includeValue)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if invalid == nil {
		invalid = []string{}
	}
	return json.Marshal(map[string]any{
		"Parameters":        entries,
		"InvalidParameters": invalid,
	})
}

// DescribeParametersJSON builds a DescribeParameters response.
func DescribeParametersJSON(params []store.Parameter) ([]byte, error) {
	entries := make([]map[string]any, 0, len(params))
	for _, p := range params {
		entry, err := describeParameterJSON(p)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]any{"Parameters": entries})
}

// EmptyOKJSON is the empty success body used by DeleteParameter.
func EmptyOKJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

func parameterJSON(p store.Parameter, includeValue bool) (map[string]any, error) {
	ts, err := lastModifiedUnix(p.LastModified)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"ARN":              p.ARN,
		"Name":             p.Name,
		"Type":             p.Type,
		"Version":          p.Version,
		"LastModifiedDate": ts,
		"DataType":         "text",
	}
	if includeValue && p.Value != "" {
		out["Value"] = p.Value
	}
	if p.KeyID != "" && includeValue {
		out["KeyId"] = p.KeyID
	}
	return out, nil
}

func describeParameterJSON(p store.Parameter) (map[string]any, error) {
	ts, err := lastModifiedUnix(p.LastModified)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"ARN":              p.ARN,
		"Name":             p.Name,
		"Type":             p.Type,
		"Version":          p.Version,
		"LastModifiedDate": ts,
		"DataType":         "text",
	}
	if p.KeyID != "" {
		out["KeyId"] = p.KeyID
	}
	return out, nil
}

func lastModifiedUnix(raw string) (float64, error) {
	if raw == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0, err
	}
	return float64(t.Unix()), nil
}
