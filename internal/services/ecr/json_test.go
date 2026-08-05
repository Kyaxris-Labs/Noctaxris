package ecr_test

import (
	"encoding/json"
	"testing"
	"time"

	ecrsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ecr"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestECRJSON(t *testing.T) {
	reg := "000000000001"
	repo := store.Repository{Name: "lab/app", ARN: "arn:aws:ecr:us-east-1:1:repository/lab/app"}
	if _, err := ecrsvc.CreateRepositoryJSON(repo, reg); err != nil {
		t.Fatal(err)
	}
	if _, err := ecrsvc.DescribeRepositoriesJSON([]store.Repository{repo}, reg); err != nil {
		t.Fatal(err)
	}
	if _, err := ecrsvc.DeleteRepositoryJSON(repo, reg); err != nil {
		t.Fatal(err)
	}
	exp := time.Unix(1_700_000_000, 0).UTC()
	if _, err := ecrsvc.GetAuthorizationTokenJSON("pass", exp); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[]}`
	if _, err := ecrsvc.GetRepositoryPolicyJSON(repo.Name, reg, policy); err != nil {
		t.Fatal(err)
	}
	if _, err := ecrsvc.SetRepositoryPolicyJSON(repo.Name, reg, policy); err != nil {
		t.Fatal(err)
	}
	if _, err := ecrsvc.DeleteRepositoryPolicyJSON(repo.Name, reg); err != nil {
		t.Fatal(err)
	}
	if _, err := ecrsvc.ListTagsForResourceJSON([]store.ResourceTag{{Key: "env", Value: "lab"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ecrsvc.EmptyOKJSON(); err != nil {
		t.Fatal(err)
	}
	img := store.Image{RepositoryName: repo.Name, ImageDigest: "sha256:abc", ImageTags: []string{"latest"}, ImagePushedAt: "2024-01-01T00:00:00Z"}
	if _, err := ecrsvc.PutImageJSON(img); err != nil {
		t.Fatal(err)
	}
	list, err := ecrsvc.ListImagesJSON([]store.Image{img})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(list, &listOut); err != nil {
		t.Fatal(err)
	}
	if _, err := ecrsvc.BatchGetImageJSON([]store.Image{img}); err != nil {
		t.Fatal(err)
	}
	if _, err := ecrsvc.BatchDeleteImageJSON([]store.Image{img}); err != nil {
		t.Fatal(err)
	}
}
