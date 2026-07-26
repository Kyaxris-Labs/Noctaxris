package store

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultCodeCommitRegion is the lab region embedded in CodeCommit ARNs.
	DefaultCodeCommitRegion = "us-east-1"

	codeCommitMaxFileBytes = 6 << 20
)

var (
	ErrCodeCommitRepoExists     = errors.New("RepositoryNameExistsException")
	ErrCodeCommitRepoNotFound   = errors.New("RepositoryDoesNotExistException")
	ErrCodeCommitInvalidInput   = errors.New("InvalidRepositoryNameException")
	ErrCodeCommitBadRequest     = errors.New("InvalidInputException")
	ErrCodeCommitBranchMissing  = errors.New("BranchDoesNotExistException")
	ErrCodeCommitPathEscape     = errors.New("InvalidPathException")
	ErrCodeCommitFileNotFound   = errors.New("FileDoesNotExistException")
	ErrCodeCommitFolderNotFound = errors.New("FolderDoesNotExistException")
	ErrCodeCommitFileContent    = errors.New("FileContentRequiredException")
	ErrCodeCommitFileTooLarge   = errors.New("FileContentSizeLimitExceededException")
	ErrCodeCommitParentOutdated = errors.New("ParentCommitIdOutdatedException")
)

var codeCommitRepoNameRe = regexp.MustCompile(`^[\w.-]+$`)

const codecommitSchema = `
CREATE TABLE IF NOT EXISTS codecommit_repos (
  account_id TEXT NOT NULL,
  repository_name TEXT NOT NULL,
  repository_id TEXT NOT NULL,
  repository_arn TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  default_branch TEXT NOT NULL DEFAULT 'main',
  head_commit_id TEXT NOT NULL DEFAULT '',
  created_at REAL NOT NULL,
  last_modified_at REAL NOT NULL,
  PRIMARY KEY (account_id, repository_name)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codecommit_repos_id ON codecommit_repos(account_id, repository_id);
`

// CodeCommitRepository is a CreateRepository / GetRepository row.
type CodeCommitRepository struct {
	RepositoryName        string
	RepositoryID          string
	ARN                   string
	AccountID             string
	Description           string
	DefaultBranch         string
	HeadCommitID          string
	CloneURLHTTP          string
	CloneURLSSH           string
	CreationDate          float64
	LastModifiedDate      float64
}

// CodeCommitPutFileResult is returned by PutFile.
type CodeCommitPutFileResult struct {
	BlobID   string
	CommitID string
	TreeID   string
}

// CodeCommitFileEntry is one file listed by GetFolder.
type CodeCommitFileEntry struct {
	AbsolutePath string
	RelativePath string
	BlobID       string
	FileMode     string
}

// CodeCommitFolderEntry is one subfolder listed by GetFolder.
type CodeCommitFolderEntry struct {
	AbsolutePath string
	RelativePath string
	TreeID       string
}

// CodeCommitFolder is a GetFolder result.
type CodeCommitFolder struct {
	CommitID   string
	FolderPath string
	TreeID     string
	Files      []CodeCommitFileEntry
	SubFolders []CodeCommitFolderEntry
}

// CodeCommitFile is a GetFile result.
type CodeCommitFile struct {
	BlobID      string
	CommitID    string
	FileContent []byte
	FileMode    string
	FilePath    string
	FileSize    int64
}

// CodeCommitFileInput is one path/content pair for batch put.
type CodeCommitFileInput struct {
	FilePath    string
	FileContent []byte
}

// EnsureCodeCommitSchema creates CodeCommit tables if missing.
func EnsureCodeCommitSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure codecommit schema: db is nil")
	}
	if _, err := db.Exec(codecommitSchema); err != nil {
		return fmt.Errorf("ensure codecommit schema: %w", err)
	}
	return nil
}

// EnsureCodeCommitSchema ensures CodeCommit tables on an open store.
func (s *Store) EnsureCodeCommitSchema() error {
	return EnsureCodeCommitSchema(s.db)
}

func (s *Store) ensureCodeCommit() error {
	return s.EnsureCodeCommitSchema()
}

// CodeCommitRepositoryARN builds arn:aws:codecommit:REGION:ACCOUNT:NAME
func CodeCommitRepositoryARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultCodeCommitRegion
	}
	return fmt.Sprintf("arn:aws:codecommit:%s:%s:%s", region, accountID, name)
}

func validateCodeCommitRepoName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 || !codeCommitRepoNameRe.MatchString(name) {
		return ErrCodeCommitInvalidInput
	}
	if strings.HasSuffix(strings.ToLower(name), ".git") {
		return ErrCodeCommitInvalidInput
	}
	return nil
}

func (s *Store) codeCommitRepoRoot(accountID, repoName string) string {
	return filepath.Join(s.dataRoot, "codecommit", accountID, repoName)
}

func (s *Store) codeCommitTreeRoot(accountID, repoName string) string {
	return filepath.Join(s.codeCommitRepoRoot(accountID, repoName), "tree")
}

func codeCommitCloneHTTP(region, name string) string {
	if region == "" {
		region = DefaultCodeCommitRegion
	}
	return fmt.Sprintf("https://git-codecommit.%s.amazonaws.com/v1/repos/%s", region, name)
}

func codeCommitCloneSSH(region, name string) string {
	if region == "" {
		region = DefaultCodeCommitRegion
	}
	return fmt.Sprintf("ssh://git-codecommit.%s.amazonaws.com/v1/repos/%s", region, name)
}

func codeCommitBlobID(content []byte) string {
	sum := sha1.Sum(content)
	return hex.EncodeToString(sum[:])
}

func codeCommitCommitID(repoID string, paths []string, blobIDs []string) string {
	h := sha1.New()
	_, _ = io.WriteString(h, repoID)
	_, _ = io.WriteString(h, "\n")
	for i := range paths {
		_, _ = io.WriteString(h, paths[i])
		_, _ = io.WriteString(h, "\t")
		_, _ = io.WriteString(h, blobIDs[i])
		_, _ = io.WriteString(h, "\n")
	}
	_, _ = fmt.Fprintf(h, "%d\n", time.Now().UnixNano())
	return hex.EncodeToString(h.Sum(nil))
}

func codeCommitTreeID(paths []string, blobIDs []string) string {
	h := sha1.New()
	for i := range paths {
		_, _ = io.WriteString(h, paths[i])
		_, _ = io.WriteString(h, "\t")
		_, _ = io.WriteString(h, blobIDs[i])
		_, _ = io.WriteString(h, "\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func codeCommitResolvePath(treeRoot, relativePath string) (string, string, error) {
	treeRoot = filepath.Clean(treeRoot)
	rel := strings.TrimSpace(relativePath)
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	rel = filepath.FromSlash(rel)
	if rel == "" || rel == "." {
		return treeRoot, "", nil
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", ErrCodeCommitPathEscape
	}
	abs := filepath.Clean(filepath.Join(treeRoot, rel))
	r, err := filepath.Rel(treeRoot, abs)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", "", ErrCodeCommitPathEscape
	}
	return abs, filepath.ToSlash(r), nil
}

func scanCodeCommitRepo(row interface {
	Scan(dest ...any) error
}, accountID, region string) (CodeCommitRepository, error) {
	var r CodeCommitRepository
	var created, modified float64
	err := row.Scan(
		&r.RepositoryName, &r.RepositoryID, &r.ARN, &r.Description,
		&r.DefaultBranch, &r.HeadCommitID, &created, &modified,
	)
	if err != nil {
		return CodeCommitRepository{}, err
	}
	r.AccountID = accountID
	r.CreationDate = created
	r.LastModifiedDate = modified
	r.CloneURLHTTP = codeCommitCloneHTTP(region, r.RepositoryName)
	r.CloneURLSSH = codeCommitCloneSSH(region, r.RepositoryName)
	return r, nil
}

// CreateCodeCommitRepository creates an empty lab repository under data root.
func (s *Store) CreateCodeCommitRepository(accountID, region, name, description string) (CodeCommitRepository, error) {
	if err := s.ensureCodeCommit(); err != nil {
		return CodeCommitRepository{}, err
	}
	if err := validateCodeCommitRepoName(name); err != nil {
		return CodeCommitRepository{}, err
	}
	if region == "" {
		region = DefaultCodeCommitRegion
	}
	id := uuid.NewString()
	arn := CodeCommitRepositoryARN(region, accountID, name)
	now := float64(time.Now().UTC().UnixNano()) / 1e9
	_, err := s.db.Exec(
		`INSERT INTO codecommit_repos
		 (account_id, repository_name, repository_id, repository_arn, description, default_branch, head_commit_id, created_at, last_modified_at)
		 VALUES (?, ?, ?, ?, ?, 'main', '', ?, ?)`,
		accountID, name, id, arn, description, now, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return CodeCommitRepository{}, ErrCodeCommitRepoExists
		}
		return CodeCommitRepository{}, fmt.Errorf("create codecommit repository: %w", err)
	}
	tree := s.codeCommitTreeRoot(accountID, name)
	if err := os.MkdirAll(tree, 0o750); err != nil {
		return CodeCommitRepository{}, fmt.Errorf("create codecommit tree: %w", err)
	}
	return CodeCommitRepository{
		RepositoryName:   name,
		RepositoryID:     id,
		ARN:              arn,
		AccountID:        accountID,
		Description:      description,
		DefaultBranch:    "main",
		CloneURLHTTP:     codeCommitCloneHTTP(region, name),
		CloneURLSSH:      codeCommitCloneSSH(region, name),
		CreationDate:     now,
		LastModifiedDate: now,
	}, nil
}

// GetCodeCommitRepository returns repository metadata.
func (s *Store) GetCodeCommitRepository(accountID, region, name string) (CodeCommitRepository, error) {
	if err := s.ensureCodeCommit(); err != nil {
		return CodeCommitRepository{}, err
	}
	if err := validateCodeCommitRepoName(name); err != nil {
		return CodeCommitRepository{}, err
	}
	row := s.db.QueryRow(
		`SELECT repository_name, repository_id, repository_arn, description, default_branch, head_commit_id, created_at, last_modified_at
		 FROM codecommit_repos WHERE account_id = ? AND repository_name = ?`,
		accountID, name,
	)
	r, err := scanCodeCommitRepo(row, accountID, region)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeCommitRepository{}, ErrCodeCommitRepoNotFound
	}
	if err != nil {
		return CodeCommitRepository{}, fmt.Errorf("get codecommit repository: %w", err)
	}
	return r, nil
}

// ListCodeCommitRepositories lists repositories for an account.
func (s *Store) ListCodeCommitRepositories(accountID string) ([]CodeCommitRepository, error) {
	if err := s.ensureCodeCommit(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT repository_name, repository_id, repository_arn, description, default_branch, head_commit_id, created_at, last_modified_at
		 FROM codecommit_repos WHERE account_id = ? ORDER BY repository_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list codecommit repositories: %w", err)
	}
	defer rows.Close()
	out := make([]CodeCommitRepository, 0)
	for rows.Next() {
		r, err := scanCodeCommitRepo(rows, accountID, DefaultCodeCommitRegion)
		if err != nil {
			return nil, fmt.Errorf("list codecommit repositories: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list codecommit repositories: %w", err)
	}
	return out, nil
}

// DeleteCodeCommitRepository deletes a repository and its on-disk tree.
func (s *Store) DeleteCodeCommitRepository(accountID, name string) (string, error) {
	if err := s.ensureCodeCommit(); err != nil {
		return "", err
	}
	if err := validateCodeCommitRepoName(name); err != nil {
		return "", err
	}
	var id string
	err := s.db.QueryRow(
		`SELECT repository_id FROM codecommit_repos WHERE account_id = ? AND repository_name = ?`,
		accountID, name,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("delete codecommit repository: %w", err)
	}
	if _, err := s.db.Exec(
		`DELETE FROM codecommit_repos WHERE account_id = ? AND repository_name = ?`,
		accountID, name,
	); err != nil {
		return "", fmt.Errorf("delete codecommit repository: %w", err)
	}
	_ = os.RemoveAll(s.codeCommitRepoRoot(accountID, name))
	return id, nil
}

// PutCodeCommitFile writes one file into the lab working tree and advances head.
func (s *Store) PutCodeCommitFile(accountID, region, repoName, branchName, filePath string, content []byte, parentCommitID string) (CodeCommitPutFileResult, error) {
	return s.BatchPutCodeCommitFiles(accountID, region, repoName, branchName, []CodeCommitFileInput{{
		FilePath: filePath, FileContent: content,
	}}, parentCommitID)
}

// BatchPutCodeCommitFiles writes multiple files in one lab commit.
func (s *Store) BatchPutCodeCommitFiles(
	accountID, region, repoName, branchName string,
	files []CodeCommitFileInput,
	parentCommitID string,
) (CodeCommitPutFileResult, error) {
	if err := s.ensureCodeCommit(); err != nil {
		return CodeCommitPutFileResult{}, err
	}
	if len(files) == 0 {
		return CodeCommitPutFileResult{}, fmt.Errorf("%w: at least one file required", ErrCodeCommitBadRequest)
	}
	repo, err := s.GetCodeCommitRepository(accountID, region, repoName)
	if err != nil {
		return CodeCommitPutFileResult{}, err
	}
	if branchName == "" {
		branchName = repo.DefaultBranch
	}
	if branchName != repo.DefaultBranch {
		// Lab stores a single working tree (default branch only).
		return CodeCommitPutFileResult{}, fmt.Errorf("%w: lab supports default branch %q only", ErrCodeCommitBranchMissing, repo.DefaultBranch)
	}
	if repo.HeadCommitID != "" && parentCommitID != "" && parentCommitID != repo.HeadCommitID {
		return CodeCommitPutFileResult{}, ErrCodeCommitParentOutdated
	}

	treeRoot := s.codeCommitTreeRoot(accountID, repoName)
	if err := os.MkdirAll(treeRoot, 0o750); err != nil {
		return CodeCommitPutFileResult{}, fmt.Errorf("codecommit put files: mkdir: %w", err)
	}

	paths := make([]string, 0, len(files))
	blobIDs := make([]string, 0, len(files))
	var lastBlob string
	for _, f := range files {
		if len(f.FileContent) == 0 {
			return CodeCommitPutFileResult{}, ErrCodeCommitFileContent
		}
		if len(f.FileContent) > codeCommitMaxFileBytes {
			return CodeCommitPutFileResult{}, ErrCodeCommitFileTooLarge
		}
		abs, rel, err := codeCommitResolvePath(treeRoot, f.FilePath)
		if err != nil {
			return CodeCommitPutFileResult{}, err
		}
		if rel == "" {
			return CodeCommitPutFileResult{}, fmt.Errorf("%w: path must name a file", ErrCodeCommitPathEscape)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
			return CodeCommitPutFileResult{}, fmt.Errorf("codecommit put files: mkdir: %w", err)
		}
		if err := os.WriteFile(abs, f.FileContent, 0o640); err != nil {
			return CodeCommitPutFileResult{}, fmt.Errorf("codecommit put files: write: %w", err)
		}
		blob := codeCommitBlobID(f.FileContent)
		paths = append(paths, rel)
		blobIDs = append(blobIDs, blob)
		lastBlob = blob
	}

	commitID := codeCommitCommitID(repo.RepositoryID, paths, blobIDs)
	treeID := codeCommitTreeID(paths, blobIDs)
	now := float64(time.Now().UTC().UnixNano()) / 1e9
	if _, err := s.db.Exec(
		`UPDATE codecommit_repos SET head_commit_id = ?, last_modified_at = ? WHERE account_id = ? AND repository_name = ?`,
		commitID, now, accountID, repoName,
	); err != nil {
		return CodeCommitPutFileResult{}, fmt.Errorf("codecommit put files: update head: %w", err)
	}
	return CodeCommitPutFileResult{BlobID: lastBlob, CommitID: commitID, TreeID: treeID}, nil
}

// GetCodeCommitFile reads a file from the lab working tree.
func (s *Store) GetCodeCommitFile(accountID, region, repoName, filePath string) (CodeCommitFile, error) {
	if err := s.ensureCodeCommit(); err != nil {
		return CodeCommitFile{}, err
	}
	repo, err := s.GetCodeCommitRepository(accountID, region, repoName)
	if err != nil {
		return CodeCommitFile{}, err
	}
	treeRoot := s.codeCommitTreeRoot(accountID, repoName)
	abs, rel, err := codeCommitResolvePath(treeRoot, filePath)
	if err != nil {
		return CodeCommitFile{}, err
	}
	if rel == "" {
		return CodeCommitFile{}, fmt.Errorf("%w: path must name a file", ErrCodeCommitPathEscape)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return CodeCommitFile{}, ErrCodeCommitFileNotFound
		}
		return CodeCommitFile{}, fmt.Errorf("codecommit get file: %w", err)
	}
	if info.IsDir() {
		return CodeCommitFile{}, fmt.Errorf("%w: path is a directory", ErrCodeCommitPathEscape)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return CodeCommitFile{}, fmt.Errorf("codecommit get file: %w", err)
	}
	return CodeCommitFile{
		BlobID:      codeCommitBlobID(data),
		CommitID:    repo.HeadCommitID,
		FileContent: data,
		FileMode:    "NORMAL",
		FilePath:    rel,
		FileSize:    int64(len(data)),
	}, nil
}

// GetCodeCommitFolder lists one directory in the lab working tree.
func (s *Store) GetCodeCommitFolder(accountID, region, repoName, folderPath string) (CodeCommitFolder, error) {
	if err := s.ensureCodeCommit(); err != nil {
		return CodeCommitFolder{}, err
	}
	repo, err := s.GetCodeCommitRepository(accountID, region, repoName)
	if err != nil {
		return CodeCommitFolder{}, err
	}
	treeRoot := s.codeCommitTreeRoot(accountID, repoName)
	abs, rel, err := codeCommitResolvePath(treeRoot, folderPath)
	if err != nil {
		return CodeCommitFolder{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return CodeCommitFolder{}, ErrCodeCommitFolderNotFound
		}
		return CodeCommitFolder{}, fmt.Errorf("codecommit get folder: %w", err)
	}
	if !info.IsDir() {
		return CodeCommitFolder{}, fmt.Errorf("%w: path is not a directory", ErrCodeCommitPathEscape)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return CodeCommitFolder{}, fmt.Errorf("codecommit get folder: %w", err)
	}
	out := CodeCommitFolder{
		CommitID:   repo.HeadCommitID,
		FolderPath: rel,
		Files:      make([]CodeCommitFileEntry, 0),
		SubFolders: make([]CodeCommitFolderEntry, 0),
	}
	var paths, blobIDs []string
	for _, e := range entries {
		name := e.Name()
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		if e.IsDir() {
			out.SubFolders = append(out.SubFolders, CodeCommitFolderEntry{
				AbsolutePath: childRel,
				RelativePath: name,
				TreeID:       codeCommitBlobID([]byte(childRel)),
			})
			continue
		}
		data, err := os.ReadFile(filepath.Join(abs, name))
		if err != nil {
			return CodeCommitFolder{}, fmt.Errorf("codecommit get folder: read: %w", err)
		}
		blob := codeCommitBlobID(data)
		out.Files = append(out.Files, CodeCommitFileEntry{
			AbsolutePath: childRel,
			RelativePath: name,
			BlobID:       blob,
			FileMode:     "NORMAL",
		})
		paths = append(paths, childRel)
		blobIDs = append(blobIDs, blob)
	}
	out.TreeID = codeCommitTreeID(paths, blobIDs)
	return out, nil
}

// ExportCodeCommitRepoTree returns the absolute path to the lab working tree for cloning into builds.
func (s *Store) ExportCodeCommitRepoTree(accountID, repoName string) (string, error) {
	if err := s.ensureCodeCommit(); err != nil {
		return "", err
	}
	if _, err := s.GetCodeCommitRepository(accountID, DefaultCodeCommitRegion, repoName); err != nil {
		return "", err
	}
	tree := s.codeCommitTreeRoot(accountID, repoName)
	if err := os.MkdirAll(tree, 0o750); err != nil {
		return "", fmt.Errorf("export codecommit tree: %w", err)
	}
	return tree, nil
}

// MaterializeCodeCommitRepo copies the lab working tree into destDir for nested build clone paths.
func (s *Store) MaterializeCodeCommitRepo(accountID, repoName, destDir string) error {
	src, err := s.ExportCodeCommitRepoTree(accountID, repoName)
	if err != nil {
		return err
	}
	destDir = filepath.Clean(destDir)
	if destDir == "" || destDir == "." {
		return fmt.Errorf("%w: destDir required", ErrCodeCommitBadRequest)
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("materialize codecommit repo: mkdir: %w", err)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
