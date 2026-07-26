package codecommit

import (
	"encoding/base64"
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateRepositoryJSON builds a CreateRepository response.
func CreateRepositoryJSON(r store.CodeCommitRepository) ([]byte, error) {
	return json.Marshal(map[string]any{
		"repositoryMetadata": repositoryMetadata(r),
	})
}

// GetRepositoryJSON builds a GetRepository response.
func GetRepositoryJSON(r store.CodeCommitRepository) ([]byte, error) {
	return json.Marshal(map[string]any{
		"repositoryMetadata": repositoryMetadata(r),
	})
}

// ListRepositoriesJSON builds a ListRepositories response.
func ListRepositoriesJSON(repos []store.CodeCommitRepository) ([]byte, error) {
	items := make([]map[string]any, 0, len(repos))
	for _, r := range repos {
		items = append(items, map[string]any{
			"repositoryId":   r.RepositoryID,
			"repositoryName": r.RepositoryName,
		})
	}
	return json.Marshal(map[string]any{"repositories": items})
}

// DeleteRepositoryJSON builds a DeleteRepository response.
func DeleteRepositoryJSON(repositoryID string) ([]byte, error) {
	out := map[string]any{}
	if repositoryID != "" {
		out["repositoryId"] = repositoryID
	}
	return json.Marshal(out)
}

// PutFileJSON builds a PutFile response.
func PutFileJSON(r store.CodeCommitPutFileResult) ([]byte, error) {
	return json.Marshal(map[string]any{
		"blobId":   r.BlobID,
		"commitId": r.CommitID,
		"treeId":   r.TreeID,
	})
}

// GetFileJSON builds a GetFile response.
func GetFileJSON(f store.CodeCommitFile) ([]byte, error) {
	return json.Marshal(map[string]any{
		"blobId":      f.BlobID,
		"commitId":    f.CommitID,
		"fileContent": base64.StdEncoding.EncodeToString(f.FileContent),
		"fileMode":    f.FileMode,
		"filePath":    f.FilePath,
		"fileSize":    f.FileSize,
	})
}

// GetFolderJSON builds a GetFolder response.
func GetFolderJSON(f store.CodeCommitFolder) ([]byte, error) {
	files := make([]map[string]any, 0, len(f.Files))
	for _, e := range f.Files {
		files = append(files, map[string]any{
			"absolutePath": e.AbsolutePath,
			"relativePath": e.RelativePath,
			"blobId":       e.BlobID,
			"fileMode":     e.FileMode,
		})
	}
	subFolders := make([]map[string]any, 0, len(f.SubFolders))
	for _, e := range f.SubFolders {
		subFolders = append(subFolders, map[string]any{
			"absolutePath": e.AbsolutePath,
			"relativePath": e.RelativePath,
			"treeId":       e.TreeID,
		})
	}
	return json.Marshal(map[string]any{
		"commitId":      f.CommitID,
		"folderPath":    f.FolderPath,
		"treeId":        f.TreeID,
		"files":         files,
		"subFolders":    subFolders,
		"subModules":    []any{},
		"symbolicLinks": []any{},
	})
}

func repositoryMetadata(r store.CodeCommitRepository) map[string]any {
	m := map[string]any{
		"accountId":            r.AccountID,
		"Arn":                  r.ARN,
		"cloneUrlHttp":         r.CloneURLHTTP,
		"cloneUrlSsh":          r.CloneURLSSH,
		"creationDate":         r.CreationDate,
		"lastModifiedDate":     r.LastModifiedDate,
		"repositoryDescription": r.Description,
		"repositoryId":         r.RepositoryID,
		"repositoryName":       r.RepositoryName,
		"defaultBranch":        r.DefaultBranch,
	}
	return m
}
