package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// DefaultECRRegion is the lab region embedded in ECR repository ARNs.
	DefaultECRRegion = "us-east-1"

	// LabRegistryHost is the docker login / push host for the lab registry (127.0.0.1:4566/ACCOUNT/NAME).
	LabRegistryHost = "127.0.0.1:4566"

	// DefaultAuthTokenTTL is the lab default lifetime for ECR authorization tokens (~12h).
	DefaultAuthTokenTTL = 12 * time.Hour
)

var (
	ErrRepositoryAlreadyExists    = errors.New("RepositoryAlreadyExistsException")
	ErrRepositoryNotFound         = errors.New("RepositoryNotFoundException")
	ErrImageNotFound              = errors.New("ImageNotFoundException")
	ErrInvalidAuthorizationToken  = errors.New("InvalidAuthorizationTokenException")
	ErrExpiredAuthorizationToken  = errors.New("ExpiredAuthorizationTokenException")
)

// Repository is an ECR repository metadata row.
type Repository struct {
	Name      string
	ARN       string
	URI       string
	CreatedAt string
}

// Image is ECR image metadata for a repository digest.
type Image struct {
	RepositoryName string
	ImageDigest    string
	ImageTags      []string
	ManifestPath   string
	ImagePushedAt  string
}

const ecrSchema = `
CREATE TABLE IF NOT EXISTS ecr_repositories (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  uri TEXT NOT NULL,
  policy TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS ecr_images (
  account_id TEXT NOT NULL,
  repo_name TEXT NOT NULL,
  digest TEXT NOT NULL,
  tags_json TEXT NOT NULL DEFAULT '[]',
  manifest_path TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  PRIMARY KEY (account_id, repo_name, digest)
);
CREATE INDEX IF NOT EXISTS idx_ecr_images_repo ON ecr_images(account_id, repo_name);
CREATE TABLE IF NOT EXISTS ecr_auth_tokens (
  token_hash TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  principal TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL,
  issued_at TEXT NOT NULL
);
`

// EnsureECRSchema creates ECR tables if missing.
func EnsureECRSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure ecr schema: db is nil")
	}
	if _, err := db.Exec(ecrSchema); err != nil {
		return fmt.Errorf("ensure ecr schema: %w", err)
	}
	return nil
}

// EnsureECRSchema ensures ECR tables on an open store (tests and Open wiring).
func (s *Store) EnsureECRSchema() error {
	return EnsureECRSchema(s.db)
}

// RepositoryARN builds arn:aws:ecr:REGION:ACCOUNT:repository/NAME.
func RepositoryARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultECRRegion
	}
	return fmt.Sprintf("arn:aws:ecr:%s:%s:repository/%s", region, accountID, name)
}

// RepositoryURI builds the lab docker registry path 127.0.0.1:4566/ACCOUNT/NAME.
func RepositoryURI(accountID, name string) string {
	return fmt.Sprintf("%s/%s/%s", LabRegistryHost, accountID, name)
}

func normalizeRepositoryName(name string) string {
	return strings.TrimSpace(name)
}

type repositoryRow struct {
	Name      string
	ARN       string
	URI       string
	Policy    string
	CreatedAt string
}

func (s *Store) getRepositoryRow(accountID, name string) (repositoryRow, error) {
	name = normalizeRepositoryName(name)
	var row repositoryRow
	err := s.db.QueryRow(
		`SELECT name, arn, uri, policy, created_at FROM ecr_repositories WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&row.Name, &row.ARN, &row.URI, &row.Policy, &row.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repositoryRow{}, ErrRepositoryNotFound
		}
		return repositoryRow{}, fmt.Errorf("get repository row %s: %w", name, err)
	}
	return row, nil
}

func repositoryFromRow(row repositoryRow) Repository {
	return Repository{
		Name:      row.Name,
		ARN:       row.ARN,
		URI:       row.URI,
		CreatedAt: row.CreatedAt,
	}
}

// CreateRepository stores a new ECR repository for the account.
func (s *Store) CreateRepository(accountID, region, name string) (Repository, error) {
	name = normalizeRepositoryName(name)
	if name == "" {
		return Repository{}, fmt.Errorf("create repository: name is required")
	}
	if region == "" {
		region = DefaultECRRegion
	}

	if _, err := s.getRepositoryRow(accountID, name); err == nil {
		return Repository{}, ErrRepositoryAlreadyExists
	} else if !errors.Is(err, ErrRepositoryNotFound) {
		return Repository{}, err
	}

	arn := RepositoryARN(region, accountID, name)
	uri := RepositoryURI(accountID, name)
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO ecr_repositories (account_id, name, arn, uri, created_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, name, arn, uri, created,
	)
	if err != nil {
		return Repository{}, fmt.Errorf("create repository %s: %w", name, err)
	}
	return Repository{Name: name, ARN: arn, URI: uri, CreatedAt: created}, nil
}

// DescribeRepositories returns repository metadata. When names is empty, all
// repositories for the account are returned. Missing names are omitted.
func (s *Store) DescribeRepositories(accountID string, names []string) ([]Repository, error) {
	if len(names) == 0 {
		rows, err := s.db.Query(
			`SELECT name, arn, uri, policy, created_at FROM ecr_repositories WHERE account_id = ? ORDER BY name`,
			accountID,
		)
		if err != nil {
			return nil, fmt.Errorf("describe repositories %s: %w", accountID, err)
		}
		defer rows.Close()
		return scanRepositories(rows, accountID)
	}

	out := make([]Repository, 0, len(names))
	for _, name := range names {
		row, err := s.getRepositoryRow(accountID, name)
		if errors.Is(err, ErrRepositoryNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, repositoryFromRow(row))
	}
	if out == nil {
		out = []Repository{}
	}
	return out, nil
}

func scanRepositories(rows *sql.Rows, accountID string) ([]Repository, error) {
	var out []Repository
	for rows.Next() {
		var row repositoryRow
		if err := rows.Scan(&row.Name, &row.ARN, &row.URI, &row.Policy, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("describe repositories %s: %w", accountID, err)
		}
		out = append(out, repositoryFromRow(row))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("describe repositories %s: %w", accountID, err)
	}
	if out == nil {
		out = []Repository{}
	}
	return out, nil
}

// DeleteRepository removes a repository and its image metadata.
func (s *Store) DeleteRepository(accountID, name string) error {
	name = normalizeRepositoryName(name)
	if _, err := s.getRepositoryRow(accountID, name); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM ecr_images WHERE account_id = ? AND repo_name = ?`, accountID, name); err != nil {
		return fmt.Errorf("delete repository images %s: %w", name, err)
	}
	res, err := s.db.Exec(`DELETE FROM ecr_repositories WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete repository %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete repository %s: %w", name, err)
	}
	if affected == 0 {
		return ErrRepositoryNotFound
	}
	return nil
}

// SetRepositoryPolicy replaces the repository policy document.
func (s *Store) SetRepositoryPolicy(accountID, name, policy string) error {
	name = normalizeRepositoryName(name)
	res, err := s.db.Exec(
		`UPDATE ecr_repositories SET policy = ? WHERE account_id = ? AND name = ?`,
		policy, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("set repository policy %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set repository policy %s: %w", name, err)
	}
	if affected == 0 {
		return ErrRepositoryNotFound
	}
	return nil
}

// GetRepositoryPolicy returns the stored policy or ErrNoSuchResourcePolicy when empty.
func (s *Store) GetRepositoryPolicy(accountID, name string) (string, error) {
	row, err := s.getRepositoryRow(accountID, name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(row.Policy) == "" {
		return "", ErrNoSuchResourcePolicy
	}
	return row.Policy, nil
}

// DeleteRepositoryPolicy clears the repository policy.
func (s *Store) DeleteRepositoryPolicy(accountID, name string) error {
	name = normalizeRepositoryName(name)
	res, err := s.db.Exec(
		`UPDATE ecr_repositories SET policy = '' WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete repository policy %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete repository policy %s: %w", name, err)
	}
	if affected == 0 {
		return ErrRepositoryNotFound
	}
	return nil
}

func marshalTags(tags []string) (string, error) {
	if len(tags) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("marshal tags: %w", err)
	}
	return string(raw), nil
}

func unmarshalTags(raw string) ([]string, error) {
	out := []string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("unmarshal tags: %w", err)
	}
	return out, nil
}

func mergeTags(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	out := make([]string, 0, len(existing)+len(incoming))
	for _, tag := range append(existing, incoming...) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

type imageRow struct {
	RepoName      string
	Digest        string
	TagsJSON      string
	ManifestPath  string
	ImagePushedAt string
}

func (s *Store) getImageRow(accountID, repoName, digest string) (imageRow, error) {
	var row imageRow
	err := s.db.QueryRow(
		`SELECT repo_name, digest, tags_json, manifest_path, created_at
		 FROM ecr_images WHERE account_id = ? AND repo_name = ? AND digest = ?`,
		accountID, repoName, digest,
	).Scan(&row.RepoName, &row.Digest, &row.TagsJSON, &row.ManifestPath, &row.ImagePushedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return imageRow{}, ErrImageNotFound
		}
		return imageRow{}, fmt.Errorf("get image row %s@%s: %w", repoName, digest, err)
	}
	return row, nil
}

func imageFromRow(row imageRow) (Image, error) {
	tags, err := unmarshalTags(row.TagsJSON)
	if err != nil {
		return Image{}, err
	}
	return Image{
		RepositoryName: row.RepoName,
		ImageDigest:    row.Digest,
		ImageTags:      tags,
		ManifestPath:   row.ManifestPath,
		ImagePushedAt:  row.ImagePushedAt,
	}, nil
}

func (s *Store) ensureRepositoryExists(accountID, repoName string) error {
	_, err := s.getRepositoryRow(accountID, repoName)
	return err
}

// PutImage stores or updates image metadata for a repository digest.
func (s *Store) PutImage(accountID, repoName, digest string, tags []string, manifestPath string) (Image, error) {
	repoName = normalizeRepositoryName(repoName)
	digest = strings.TrimSpace(digest)
	if repoName == "" || digest == "" {
		return Image{}, fmt.Errorf("put image: repository and digest are required")
	}
	if err := s.ensureRepositoryExists(accountID, repoName); err != nil {
		return Image{}, err
	}

	existing, lookupErr := s.getImageRow(accountID, repoName, digest)
	exists := lookupErr == nil
	if !exists && !errors.Is(lookupErr, ErrImageNotFound) {
		return Image{}, lookupErr
	}

	merged := mergeTags(nil, tags)
	pushedAt := nowRFC3339()
	if exists {
		prevTags, err := unmarshalTags(existing.TagsJSON)
		if err != nil {
			return Image{}, err
		}
		merged = mergeTags(prevTags, tags)
		if manifestPath == "" {
			manifestPath = existing.ManifestPath
		}
		pushedAt = existing.ImagePushedAt
		tagsJSON, err := marshalTags(merged)
		if err != nil {
			return Image{}, err
		}
		_, err = s.db.Exec(
			`UPDATE ecr_images SET tags_json = ?, manifest_path = ? WHERE account_id = ? AND repo_name = ? AND digest = ?`,
			tagsJSON, manifestPath, accountID, repoName, digest,
		)
		if err != nil {
			return Image{}, fmt.Errorf("put image %s@%s: %w", repoName, digest, err)
		}
	} else {
		tagsJSON, err := marshalTags(merged)
		if err != nil {
			return Image{}, err
		}
		_, err = s.db.Exec(
			`INSERT INTO ecr_images (account_id, repo_name, digest, tags_json, manifest_path, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			accountID, repoName, digest, tagsJSON, manifestPath, pushedAt,
		)
		if err != nil {
			return Image{}, fmt.Errorf("put image %s@%s: %w", repoName, digest, err)
		}
	}

	return Image{
		RepositoryName: repoName,
		ImageDigest:    digest,
		ImageTags:      merged,
		ManifestPath:   manifestPath,
		ImagePushedAt:  pushedAt,
	}, nil
}

// ListImages returns image metadata for a repository.
func (s *Store) ListImages(accountID, repoName string) ([]Image, error) {
	repoName = normalizeRepositoryName(repoName)
	if err := s.ensureRepositoryExists(accountID, repoName); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT repo_name, digest, tags_json, manifest_path, created_at
		 FROM ecr_images WHERE account_id = ? AND repo_name = ? ORDER BY created_at`,
		accountID, repoName,
	)
	if err != nil {
		return nil, fmt.Errorf("list images %s: %w", repoName, err)
	}
	defer rows.Close()
	return scanImages(rows, repoName)
}

func scanImages(rows *sql.Rows, repoName string) ([]Image, error) {
	var out []Image
	for rows.Next() {
		var row imageRow
		if err := rows.Scan(&row.RepoName, &row.Digest, &row.TagsJSON, &row.ManifestPath, &row.ImagePushedAt); err != nil {
			return nil, fmt.Errorf("list images %s: %w", repoName, err)
		}
		img, err := imageFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, img)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list images %s: %w", repoName, err)
	}
	if out == nil {
		out = []Image{}
	}
	return out, nil
}

// BatchGetImage returns images matching digests and/or tags. Missing entries are omitted.
func (s *Store) BatchGetImage(accountID, repoName string, digests, tags []string) ([]Image, error) {
	repoName = normalizeRepositoryName(repoName)
	if err := s.ensureRepositoryExists(accountID, repoName); err != nil {
		return nil, err
	}

	images, err := s.ListImages(accountID, repoName)
	if err != nil {
		return nil, err
	}
	if len(digests) == 0 && len(tags) == 0 {
		return images, nil
	}

	digestSet := make(map[string]struct{}, len(digests))
	for _, d := range digests {
		d = strings.TrimSpace(d)
		if d != "" {
			digestSet[d] = struct{}{}
		}
	}
	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			tagSet[t] = struct{}{}
		}
	}

	out := make([]Image, 0)
	seen := make(map[string]struct{})
	for _, img := range images {
		if len(digestSet) > 0 {
			if _, ok := digestSet[img.ImageDigest]; ok {
				if _, dup := seen[img.ImageDigest]; !dup {
					out = append(out, img)
					seen[img.ImageDigest] = struct{}{}
				}
				continue
			}
		}
		if len(tagSet) > 0 {
			for _, tag := range img.ImageTags {
				if _, ok := tagSet[tag]; ok {
					if _, dup := seen[img.ImageDigest]; !dup {
						out = append(out, img)
						seen[img.ImageDigest] = struct{}{}
					}
					break
				}
			}
		}
	}
	if out == nil {
		out = []Image{}
	}
	return out, nil
}

// BatchDeleteImage removes images by digest and/or tag within a repository.
func (s *Store) BatchDeleteImage(accountID, repoName string, digests, tags []string) error {
	repoName = normalizeRepositoryName(repoName)
	if err := s.ensureRepositoryExists(accountID, repoName); err != nil {
		return err
	}

	images, err := s.ListImages(accountID, repoName)
	if err != nil {
		return err
	}

	digestSet := make(map[string]struct{}, len(digests))
	for _, d := range digests {
		d = strings.TrimSpace(d)
		if d != "" {
			digestSet[d] = struct{}{}
		}
	}
	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			tagSet[t] = struct{}{}
		}
	}
	if len(digestSet) == 0 && len(tagSet) == 0 {
		return nil
	}

	for _, img := range images {
		deleteDigest := false
		if len(digestSet) > 0 {
			if _, ok := digestSet[img.ImageDigest]; ok {
				deleteDigest = true
			}
		}
		if !deleteDigest && len(tagSet) > 0 {
			for _, tag := range img.ImageTags {
				if _, ok := tagSet[tag]; ok {
					deleteDigest = true
					break
				}
			}
		}
		if !deleteDigest {
			continue
		}
		if _, err := s.db.Exec(
			`DELETE FROM ecr_images WHERE account_id = ? AND repo_name = ? AND digest = ?`,
			accountID, repoName, img.ImageDigest,
		); err != nil {
			return fmt.Errorf("batch delete image %s@%s: %w", repoName, img.ImageDigest, err)
		}
	}
	return nil
}

func hashAuthToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newAuthToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// IssueAuthorizationToken stores a hashed lab ECR auth token and returns the plaintext once.
// ttl zero uses DefaultAuthTokenTTL.
func (s *Store) IssueAuthorizationToken(accountID, principal string, ttl time.Duration) (string, time.Time, error) {
	if strings.TrimSpace(accountID) == "" {
		return "", time.Time{}, fmt.Errorf("issue authorization token: account id required")
	}
	if ttl <= 0 {
		ttl = DefaultAuthTokenTTL
	}
	token, err := newAuthToken()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("issue authorization token: %w", err)
	}
	issued := time.Now().UTC()
	expires := issued.Add(ttl)
	_, err = s.db.Exec(
		`INSERT INTO ecr_auth_tokens (token_hash, account_id, principal, expires_at, issued_at) VALUES (?, ?, ?, ?, ?)`,
		hashAuthToken(token), accountID, strings.TrimSpace(principal), expires.Format(time.RFC3339), issued.Format(time.RFC3339),
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("issue authorization token: %w", err)
	}
	return token, expires, nil
}

// ValidateAuthorizationToken checks a previously issued token hash and expiry.
func (s *Store) ValidateAuthorizationToken(token string) (accountID, principal string, err error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", ErrInvalidAuthorizationToken
	}
	var expiresAt string
	err = s.db.QueryRow(
		`SELECT account_id, principal, expires_at FROM ecr_auth_tokens WHERE token_hash = ?`,
		hashAuthToken(token),
	).Scan(&accountID, &principal, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrInvalidAuthorizationToken
		}
		return "", "", fmt.Errorf("validate authorization token: %w", err)
	}
	exp, parseErr := time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil {
		return "", "", fmt.Errorf("validate authorization token: parse expiry: %w", parseErr)
	}
	if time.Now().UTC().After(exp) {
		return "", "", ErrExpiredAuthorizationToken
	}
	return accountID, principal, nil
}
