package ecr

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const labProxyEndpoint = "http://127.0.0.1:4566"

// CreateRepositoryJSON builds a CreateRepository response.
func CreateRepositoryJSON(repo store.Repository, registryID string) ([]byte, error) {
	entry, err := repositoryJSON(repo, registryID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"repository": entry})
}

// DescribeRepositoriesJSON builds a DescribeRepositories response.
func DescribeRepositoriesJSON(repos []store.Repository, registryID string) ([]byte, error) {
	entries := make([]map[string]any, 0, len(repos))
	for _, repo := range repos {
		entry, err := repositoryJSON(repo, registryID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]any{"repositories": entries})
}

// DeleteRepositoryJSON builds a DeleteRepository response.
func DeleteRepositoryJSON(repo store.Repository, registryID string) ([]byte, error) {
	entry, err := repositoryJSON(repo, registryID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"repository": entry})
}

// GetAuthorizationTokenJSON builds a GetAuthorizationToken response.
func GetAuthorizationTokenJSON(password string, expiresAt time.Time) ([]byte, error) {
	token := base64.StdEncoding.EncodeToString([]byte("AWS:" + password))
	ts, err := timestampUnix(expiresAt)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"authorizationData": []map[string]any{{
			"authorizationToken": token,
			"expiresAt":          ts,
			"proxyEndpoint":      labProxyEndpoint,
		}},
	})
}

// GetRepositoryPolicyJSON builds a GetRepositoryPolicy response.
func GetRepositoryPolicyJSON(name, registryID, policy string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"repositoryName": name,
		"registryId":     registryID,
		"policyText":     policy,
	})
}

// SetRepositoryPolicyJSON builds a SetRepositoryPolicy response.
func SetRepositoryPolicyJSON(name, registryID, policy string) ([]byte, error) {
	return GetRepositoryPolicyJSON(name, registryID, policy)
}

// DeleteRepositoryPolicyJSON builds a DeleteRepositoryPolicy response.
func DeleteRepositoryPolicyJSON(name, registryID string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"repositoryName": name,
		"registryId":     registryID,
	})
}

// PutImageJSON builds a PutImage response.
func PutImageJSON(img store.Image) ([]byte, error) {
	entry, err := imageDetailJSON(img)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"image": entry})
}

// ListImagesJSON builds a ListImages response.
func ListImagesJSON(images []store.Image) ([]byte, error) {
	entries := make([]map[string]any, 0, len(images))
	for _, img := range images {
		entries = append(entries, imageIdentifierJSON(img))
	}
	return json.Marshal(map[string]any{"imageIds": entries})
}

// BatchGetImageJSON builds a BatchGetImage response.
func BatchGetImageJSON(images []store.Image) ([]byte, error) {
	entries := make([]map[string]any, 0, len(images))
	for _, img := range images {
		entry, err := imageDetailJSON(img)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]any{
		"images":   entries,
		"failures": []any{},
	})
}

// BatchDeleteImageJSON builds a BatchDeleteImage response.
func BatchDeleteImageJSON(images []store.Image) ([]byte, error) {
	entries := make([]map[string]any, 0, len(images))
	for _, img := range images {
		entries = append(entries, imageIdentifierJSON(img))
	}
	return json.Marshal(map[string]any{"imageIds": entries})
}

func repositoryJSON(repo store.Repository, registryID string) (map[string]any, error) {
	created, err := dateUnix(repo.CreatedAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"repositoryArn":    repo.ARN,
		"registryId":     registryID,
		"repositoryName": repo.Name,
		"repositoryUri":  repo.URI,
		"createdAt":      created,
	}, nil
}

func imageIdentifierJSON(img store.Image) map[string]any {
	out := map[string]any{
		"imageDigest": img.ImageDigest,
	}
	if len(img.ImageTags) > 0 {
		out["imageTag"] = img.ImageTags[0]
	}
	return out
}

func imageDetailJSON(img store.Image) (map[string]any, error) {
	pushed, err := dateUnix(img.ImagePushedAt)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"repositoryName": img.RepositoryName,
		"imageId":        imageIdentifierJSON(img),
		"imageManifest":  "",
		"registryId":     "",
		"imagePushedAt":  pushed,
	}
	if img.ManifestPath != "" {
		out["imageManifest"] = img.ManifestPath
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

func timestampUnix(t time.Time) (float64, error) {
	if t.IsZero() {
		return 0, nil
	}
	return float64(t.Unix()), nil
}
