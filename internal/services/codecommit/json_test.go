package codecommit_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	codecommitsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/codecommit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCodeCommitJSON(t *testing.T) {
	repo := store.CodeCommitRepository{
		AccountID: "000000000001", ARN: "arn:repo", CloneURLHTTP: "https://git", CloneURLSSH: "ssh://git",
		CreationDate: 1, LastModifiedDate: 2, Description: "d", RepositoryID: "rid", RepositoryName: "lab",
		DefaultBranch: "main",
	}
	raw, err := codecommitsvc.CreateRepositoryJSON(repo)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	raw, _ = codecommitsvc.GetRepositoryJSON(repo)
	_ = json.Unmarshal(raw, &out)
	meta, _ := out["repositoryMetadata"].(map[string]any)
	if meta["repositoryName"] != "lab" {
		t.Fatalf("get=%v", out)
	}

	raw, _ = codecommitsvc.ListRepositoriesJSON([]store.CodeCommitRepository{repo})
	_ = json.Unmarshal(raw, &out)

	raw, _ = codecommitsvc.DeleteRepositoryJSON("rid")
	_ = json.Unmarshal(raw, &out)
	raw, _ = codecommitsvc.DeleteRepositoryJSON("")

	put := store.CodeCommitPutFileResult{BlobID: "b", CommitID: "c", TreeID: "t"}
	raw, _ = codecommitsvc.PutFileJSON(put)

	f := store.CodeCommitFile{
		BlobID: "b", CommitID: "c", FileContent: []byte("hi"), FileMode: "NORMAL",
		FilePath: "README.md", FileSize: 2,
	}
	raw, _ = codecommitsvc.GetFileJSON(f)
	_ = json.Unmarshal(raw, &out)
	if out["fileContent"] != base64.StdEncoding.EncodeToString(f.FileContent) {
		t.Fatalf("file=%v", out)
	}

	folder := store.CodeCommitFolder{
		CommitID: "c", FolderPath: "/", TreeID: "t",
		Files: []store.CodeCommitFileEntry{{
			AbsolutePath: "/README.md", RelativePath: "README.md", BlobID: "b", FileMode: "NORMAL",
		}},
		SubFolders: []store.CodeCommitFolderEntry{{
			AbsolutePath: "/src", RelativePath: "src", TreeID: "t2",
		}},
	}
	raw, _ = codecommitsvc.GetFolderJSON(folder)
	_ = json.Unmarshal(raw, &out)
	if len(out["files"].([]any)) != 1 {
		t.Fatalf("folder=%v", out)
	}
}
