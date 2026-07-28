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
		includeValue := store.ParameterValueIncluded(p.Type, withDecryption)
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

// GetParametersByPathJSON builds a GetParametersByPath response.
func GetParametersByPathJSON(params []store.Parameter, withDecryption bool) ([]byte, error) {
	entries := make([]map[string]any, 0, len(params))
	for _, p := range params {
		includeValue := store.ParameterValueIncluded(p.Type, withDecryption)
		entry, err := parameterJSON(p, includeValue)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]any{"Parameters": entries})
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

// ListTagsForResourceJSON builds a ListTagsForResource response (TagList Key/Value).
func ListTagsForResourceJSON(tags []store.ResourceTag) ([]byte, error) {
	entries := make([]map[string]string, 0, len(tags))
	for _, t := range tags {
		entries = append(entries, map[string]string{"Key": t.Key, "Value": t.Value})
	}
	return json.Marshal(map[string]any{"TagList": entries})
}

// SendCommandJSON builds a SendCommand response.
func SendCommandJSON(cmd store.SSMCommand) ([]byte, error) {
	node, err := commandJSON(cmd)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"Command": node})
}

// GetCommandInvocationJSON builds a GetCommandInvocation response (flat detail).
func GetCommandInvocationJSON(inv store.SSMCommandInvocation) ([]byte, error) {
	return json.Marshal(invocationDetailJSON(inv))
}

// ListCommandInvocationsJSON builds a ListCommandInvocations response.
func ListCommandInvocationsJSON(invocations []store.SSMCommandInvocation) ([]byte, error) {
	entries := make([]map[string]any, 0, len(invocations))
	for _, inv := range invocations {
		entries = append(entries, invocationSummaryJSON(inv))
	}
	return json.Marshal(map[string]any{"CommandInvocations": entries})
}

func commandJSON(cmd store.SSMCommand) (map[string]any, error) {
	requested, err := lastModifiedUnix(cmd.RequestedAt)
	if err != nil {
		return nil, err
	}
	expires, err := lastModifiedUnix(cmd.ExpiresAt)
	if err != nil {
		return nil, err
	}
	params := map[string]any{}
	for k, v := range cmd.Parameters {
		params[k] = v
	}
	ids := cmd.InstanceIDs
	if ids == nil {
		ids = []string{}
	}
	out := map[string]any{
		"CommandId":         cmd.CommandID,
		"DocumentName":      cmd.DocumentName,
		"DocumentVersion":   cmd.DocumentVersion,
		"Comment":           cmd.Comment,
		"Parameters":        params,
		"InstanceIds":       ids,
		"RequestedDateTime": requested,
		"Status":            cmd.Status,
		"StatusDetails":     cmd.StatusDetails,
		"TimeoutSeconds":    cmd.TimeoutSeconds,
		"TargetCount":       cmd.TargetCount,
		"CompletedCount":    cmd.CompletedCount,
		"ErrorCount":        cmd.ErrorCount,
	}
	if cmd.ExpiresAt != "" {
		out["ExpiresAfter"] = expires
	}
	return out, nil
}

func invocationSummaryJSON(inv store.SSMCommandInvocation) map[string]any {
	requested, _ := lastModifiedUnix(inv.RequestedAt)
	return map[string]any{
		"CommandId":         inv.CommandID,
		"InstanceId":        inv.InstanceID,
		"Comment":           inv.Comment,
		"DocumentName":      inv.DocumentName,
		"DocumentVersion":   inv.DocumentVersion,
		"RequestedDateTime": requested,
		"Status":            inv.Status,
		"StatusDetails":     inv.StatusDetails,
	}
}

func invocationDetailJSON(inv store.SSMCommandInvocation) map[string]any {
	out := invocationSummaryJSON(inv)
	out["StandardOutputContent"] = inv.Stdout
	out["StandardErrorContent"] = inv.Stderr
	out["ResponseCode"] = inv.ResponseCode
	if inv.ExecutionStart != "" {
		out["ExecutionStartDateTime"] = inv.ExecutionStart
	}
	if inv.ExecutionEnd != "" {
		out["ExecutionEndDateTime"] = inv.ExecutionEnd
	}
	return out
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
