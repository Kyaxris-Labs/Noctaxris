package secretsmanager

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateSecretJSON builds a CreateSecret response.
func CreateSecretJSON(sec store.Secret) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ARN":       sec.ARN,
		"Name":      sec.Name,
		"VersionId": sec.VersionID,
	})
}

// GetSecretValueJSON builds a GetSecretValue response.
func GetSecretValueJSON(sec store.Secret) ([]byte, error) {
	out := map[string]any{
		"ARN":       sec.ARN,
		"Name":      sec.Name,
		"VersionId": sec.VersionID,
	}
	if sec.SecretString != "" {
		out["SecretString"] = sec.SecretString
	}
	if len(sec.SecretBinary) > 0 {
		out["SecretBinary"] = base64.StdEncoding.EncodeToString(sec.SecretBinary)
	}
	return json.Marshal(out)
}

// PutSecretValueJSON builds a PutSecretValue response.
func PutSecretValueJSON(sec store.Secret) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ARN":       sec.ARN,
		"Name":      sec.Name,
		"VersionId": sec.VersionID,
	})
}

// DeleteSecretJSON builds a DeleteSecret response.
func DeleteSecretJSON(sec store.Secret, deletionDate string) ([]byte, error) {
	ts, err := dateUnix(deletionDate)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"ARN":          sec.ARN,
		"Name":         sec.Name,
		"DeletionDate": ts,
	})
}

// DescribeSecretJSON builds a DescribeSecret response.
func DescribeSecretJSON(sec store.Secret) ([]byte, error) {
	created, err := dateUnix(sec.CreatedDate)
	if err != nil {
		return nil, err
	}
	changed, err := dateUnix(sec.LastChangedDate)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"ARN":              sec.ARN,
		"Name":             sec.Name,
		"VersionIdsToStages": map[string]any{
			sec.VersionID: []string{"AWSCURRENT"},
		},
		"CreatedDate":      created,
		"LastChangedDate":  changed,
		"LastAccessedDate": changed,
	}
	if sec.Description != "" {
		out["Description"] = sec.Description
	}
	if sec.KmsKeyID != "" {
		out["KmsKeyId"] = sec.KmsKeyID
	}
	return json.Marshal(out)
}

// ListSecretsJSON builds a ListSecrets response.
func ListSecretsJSON(secrets []store.Secret) ([]byte, error) {
	entries := make([]map[string]any, 0, len(secrets))
	for _, sec := range secrets {
		entry, err := listSecretJSON(sec)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]any{"SecretList": entries})
}

// PutResourcePolicyJSON builds a PutResourcePolicy response.
func PutResourcePolicyJSON(sec store.Secret) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ARN":  sec.ARN,
		"Name": sec.Name,
	})
}

// GetResourcePolicyJSON builds a GetResourcePolicy response.
func GetResourcePolicyJSON(sec store.Secret, policy string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ARN":            sec.ARN,
		"Name":           sec.Name,
		"ResourcePolicy": policy,
	})
}

// DeleteResourcePolicyJSON builds a DeleteResourcePolicy response.
func DeleteResourcePolicyJSON(sec store.Secret) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ARN":  sec.ARN,
		"Name": sec.Name,
	})
}

func listSecretJSON(sec store.Secret) (map[string]any, error) {
	created, err := dateUnix(sec.CreatedDate)
	if err != nil {
		return nil, err
	}
	changed, err := dateUnix(sec.LastChangedDate)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"ARN":             sec.ARN,
		"Name":            sec.Name,
		"CreatedDate":     created,
		"LastChangedDate": changed,
		"LastAccessedDate": changed,
	}
	if sec.Description != "" {
		out["Description"] = sec.Description
	}
	if sec.KmsKeyID != "" {
		out["KmsKeyId"] = sec.KmsKeyID
	}
	return out, nil
}

func dateUnix(raw string) (float64, error) {
	if raw == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0, err
	}
	return float64(t.Unix()), nil
}
