package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/google/uuid"
)

const (
	registryV2Prefix     = "/v2/"
	maxRegistryBodyBytes = 16 << 20 // lab blob/manifest limit (matches S3 PutObject)
	registryServiceName  = "ecr"
)

type registryRoute struct {
	accountID string
	repoName  string
	remainder string
}

func isRegistryV2Path(path string) bool {
	return path == "/v2" || path == "/v2/" || strings.HasPrefix(path, registryV2Prefix)
}

func (s *Server) handleRegistryV2(w http.ResponseWriter, r *http.Request) {
	if !isRegistryV2Path(r.URL.Path) {
		http.NotFound(w, r)
		return
	}

	path := r.URL.Path
	if path == "/v2" {
		path = "/v2/"
	}

	if path == "/v2/" {
		s.handleRegistryV2Root(w, r)
		return
	}

	accountID, token, ok := s.authenticateRegistry(r)
	if !ok {
		s.writeRegistryUnauthorized(w)
		return
	}

	route, err := parseRegistryRoute(path)
	if err != nil {
		s.writeRegistryError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository name not known to registry")
		return
	}
	if route.accountID != accountID {
		s.writeRegistryUnauthorized(w)
		return
	}
	if err := s.storeEnsureRepository(accountID, route.repoName); err != nil {
		if errors.Is(err, store.ErrRepositoryNotFound) {
			s.writeRegistryError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository name not known to registry")
			return
		}
		s.writeRegistryError(w, http.StatusInternalServerError, "UNKNOWN", "internal error")
		return
	}

	switch {
	case r.Method == http.MethodGet && route.remainder == "tags/list":
		s.handleRegistryListTags(w, r, accountID, route.repoName)
	case strings.HasPrefix(route.remainder, "blobs/uploads/"):
		s.handleRegistryBlobUpload(w, r, accountID, route.repoName, strings.TrimPrefix(route.remainder, "blobs/uploads/"))
	case strings.HasPrefix(route.remainder, "blobs/"):
		s.handleRegistryBlob(w, r, accountID, route.repoName, strings.TrimPrefix(route.remainder, "blobs/"))
	case strings.HasPrefix(route.remainder, "manifests/"):
		s.handleRegistryManifest(w, r, accountID, route.repoName, strings.TrimPrefix(route.remainder, "manifests/"), token)
	default:
		s.writeRegistryError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository name not known to registry")
	}
}

func (s *Server) handleRegistryV2Root(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, _, ok := s.authenticateRegistry(r); !ok {
		s.writeRegistryUnauthorized(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte("{}"))
	}
}

func (s *Server) authenticateRegistry(r *http.Request) (accountID, token string, ok bool) {
	token = extractRegistryToken(r)
	if token == "" {
		return "", "", false
	}
	accountID, _, err := s.store.ValidateAuthorizationToken(token)
	if err != nil {
		return "", "", false
	}
	return accountID, token, true
}

func extractRegistryToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(auth), "basic ") {
		raw, err := decodeBasicAuth(strings.TrimSpace(auth[6:]))
		if err != nil {
			return ""
		}
		user, pass, found := strings.Cut(raw, ":")
		if !found || user != "AWS" || pass == "" {
			return ""
		}
		return pass
	}
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func decodeBasicAuth(encoded string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func parseRegistryRoute(path string) (registryRoute, error) {
	if !strings.HasPrefix(path, registryV2Prefix) {
		return registryRoute{}, fmt.Errorf("not a registry path")
	}
	rest := strings.TrimPrefix(path, registryV2Prefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return registryRoute{}, fmt.Errorf("invalid repository path")
	}
	out := registryRoute{
		accountID: parts[0],
		repoName:  parts[1],
	}
	if len(parts) == 3 {
		out.remainder = parts[2]
	}
	return out, nil
}

func (s *Server) storeEnsureRepository(accountID, repoName string) error {
	repos, err := s.store.DescribeRepositories(accountID, []string{repoName})
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return store.ErrRepositoryNotFound
	}
	return nil
}

func (s *Server) handleRegistryListTags(w http.ResponseWriter, r *http.Request, accountID, repoName string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	images, err := s.store.ListImages(accountID, repoName)
	if err != nil {
		s.writeRegistryError(w, http.StatusInternalServerError, "UNKNOWN", "internal error")
		return
	}
	tagSet := make(map[string]struct{})
	for _, img := range images {
		for _, tag := range img.ImageTags {
			tagSet[tag] = struct{}{}
		}
	}
	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sortStrings(tags)
	payload, _ := json.Marshal(map[string]any{
		"name": repoName,
		"tags": tags,
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) handleRegistryBlob(w http.ResponseWriter, r *http.Request, accountID, repoName, digest string) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		s.writeRegistryError(w, http.StatusBadRequest, "BLOB_UPLOAD_INVALID", "digest required")
		return
	}
	switch r.Method {
	case http.MethodHead, http.MethodGet:
		s.serveRegistryBlob(w, r, digest)
	case http.MethodPut:
		if err := s.writeRegistryBlobFromRequest(r, digest); err != nil {
			s.writeRegistryError(w, http.StatusBadRequest, "BLOB_UPLOAD_INVALID", "blob upload failed")
			return
		}
		loc := fmt.Sprintf("/v2/%s/%s/blobs/%s", accountID, repoName, digest)
		w.Header().Set("Location", loc)
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusCreated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRegistryBlobUpload(w http.ResponseWriter, r *http.Request, accountID, repoName, uploadID string) {
	switch r.Method {
	case http.MethodPost:
		if uploadID != "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		uploadID = uuid.NewString()
		loc := fmt.Sprintf("/v2/%s/%s/blobs/uploads/%s", accountID, repoName, uploadID)
		w.Header().Set("Location", loc)
		w.Header().Set("Range", "0-0")
		w.WriteHeader(http.StatusAccepted)
	case http.MethodPut:
		digest := strings.TrimSpace(r.URL.Query().Get("digest"))
		if digest == "" {
			s.writeRegistryError(w, http.StatusBadRequest, "BLOB_UPLOAD_INVALID", "digest required")
			return
		}
		if err := s.writeRegistryBlobFromRequest(r, digest); err != nil {
			s.writeRegistryError(w, http.StatusBadRequest, "BLOB_UPLOAD_INVALID", "blob upload failed")
			return
		}
		loc := fmt.Sprintf("/v2/%s/%s/blobs/%s", accountID, repoName, digest)
		w.Header().Set("Location", loc)
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusCreated)
	case http.MethodPatch:
		s.writeRegistryError(w, http.StatusNotImplemented, "UNSUPPORTED", "chunked upload not implemented")
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) serveRegistryBlob(w http.ResponseWriter, r *http.Request, digest string) {
	path, err := registryBlobPath(s.cfg.DataRoot, digest)
	if err != nil {
		s.writeRegistryError(w, http.StatusBadRequest, "DIGEST_INVALID", "invalid digest")
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeRegistryError(w, http.StatusNotFound, "BLOB_UNKNOWN", "blob unknown to registry")
			return
		}
		s.writeRegistryError(w, http.StatusInternalServerError, "UNKNOWN", "internal error")
		return
	}
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		s.writeRegistryError(w, http.StatusInternalServerError, "UNKNOWN", "internal error")
		return
	}
	defer f.Close()
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

func (s *Server) writeRegistryBlobFromRequest(r *http.Request, digest string) error {
	path, err := registryBlobPath(s.cfg.DataRoot, digest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".upload"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	written, err := io.Copy(f, io.TeeReader(io.LimitReader(r.Body, maxRegistryBodyBytes+1), hasher))
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if written > maxRegistryBodyBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("blob too large")
	}
	gotDigest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if gotDigest != digest {
		_ = os.Remove(tmp)
		return fmt.Errorf("digest mismatch")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Server) handleRegistryManifest(w http.ResponseWriter, r *http.Request, accountID, repoName, reference, authToken string) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		s.writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "manifest reference required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		s.putRegistryManifest(w, r, accountID, repoName, reference, authToken)
	case http.MethodGet, http.MethodHead:
		s.getRegistryManifest(w, r, accountID, repoName, reference)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) putRegistryManifest(w http.ResponseWriter, r *http.Request, accountID, repoName, reference, authToken string) {
	body, err := readBody(r, maxRegistryBodyBytes)
	if err != nil {
		s.writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "unable to read manifest")
		return
	}
	if len(body) == 0 {
		s.writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "empty manifest")
		return
	}
	sum := sha256.Sum256(body)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	relPath, err := registryManifestRelPath(accountID, repoName, digest)
	if err != nil {
		s.writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "invalid manifest digest")
		return
	}
	absPath := filepath.Join(s.cfg.DataRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o700); err != nil {
		s.writeRegistryError(w, http.StatusInternalServerError, "UNKNOWN", "internal error")
		return
	}
	if err := os.WriteFile(absPath, body, 0o600); err != nil {
		s.writeRegistryError(w, http.StatusInternalServerError, "UNKNOWN", "internal error")
		return
	}

	var tags []string
	if !strings.HasPrefix(reference, "sha256:") {
		tags = []string{reference}
	}
	if _, err := s.store.PutImage(accountID, repoName, digest, tags, relPath); err != nil {
		s.writeRegistryError(w, http.StatusInternalServerError, "UNKNOWN", "internal error")
		return
	}

	s.registrySyncToEngine(r.Context(), accountID, repoName, reference, authToken)

	loc := fmt.Sprintf("/v2/%s/%s/manifests/%s", accountID, repoName, digest)
	w.Header().Set("Location", loc)
	w.Header().Set("Docker-Content-Digest", digest)
	ct := strings.TrimSpace(r.Header.Get("Content-Type"))
	if ct == "" {
		ct = "application/vnd.docker.distribution.manifest.v2+json"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) getRegistryManifest(w http.ResponseWriter, r *http.Request, accountID, repoName, reference string) {
	var relPath string
	var digest string
	if strings.HasPrefix(reference, "sha256:") {
		digest = reference
		var err error
		relPath, err = registryManifestRelPath(accountID, repoName, digest)
		if err != nil {
			s.writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "invalid digest")
			return
		}
	} else {
		images, err := s.store.BatchGetImage(accountID, repoName, nil, []string{reference})
		if err != nil || len(images) == 0 {
			s.writeRegistryError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest unknown")
			return
		}
		digest = images[0].ImageDigest
		relPath = images[0].ManifestPath
	}
	absPath := filepath.Join(s.cfg.DataRoot, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeRegistryError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest unknown")
			return
		}
		s.writeRegistryError(w, http.StatusInternalServerError, "UNKNOWN", "internal error")
		return
	}
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) registrySyncToEngine(ctx context.Context, accountID, repoName, reference, authToken string) {
	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		return
	}
	if strings.HasPrefix(reference, "sha256:") {
		return
	}
	cli, err := s.computeClient()
	if err != nil {
		log.Printf("registry: DinD image sync skipped for %s/%s:%s: compute client: %v", accountID, repoName, reference, err)
		return
	}
	if cli == nil {
		return
	}
	ref := fmt.Sprintf("%s/%s/%s:%s", registryDinDPullHost(s.cfg.ListenAddr), accountID, repoName, reference)
	if err := cli.PullLabRegistryImage(ctx, ref, "AWS", authToken); err != nil {
		log.Printf("registry: DinD image sync failed for %s: %v", ref, err)
	}
}

func registryBlobPath(dataRoot, digest string) (string, error) {
	algo, hexPart, err := parseRegistryDigest(digest)
	if err != nil {
		return "", err
	}
	root := filepath.Join(dataRoot, "ecr", "blobs")
	path := filepath.Join(root, algo, hexPart)
	if err := ensurePathWithinRoot(root, path); err != nil {
		return "", err
	}
	return path, nil
}

func registryManifestRelPath(accountID, repoName, digest string) (string, error) {
	algo, hexPart, err := parseRegistryDigest(digest)
	if err != nil {
		return "", err
	}
	return filepath.Join("ecr", "manifests", accountID, repoName, algo, hexPart+".json"), nil
}

var allowedRegistryDigestAlgorithms = map[string]int{
	"sha256": 64,
}

func parseRegistryDigest(digest string) (algo, hexPart string, err error) {
	digest = strings.TrimSpace(digest)
	parts := strings.SplitN(digest, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid digest")
	}
	algo = parts[0]
	hexPart = parts[1]
	if strings.Contains(algo, "..") || strings.Contains(hexPart, "..") {
		return "", "", fmt.Errorf("invalid digest")
	}
	if strings.ContainsAny(algo, `/\`) || strings.ContainsAny(hexPart, `/\`) {
		return "", "", fmt.Errorf("invalid digest")
	}
	wantLen, ok := allowedRegistryDigestAlgorithms[algo]
	if !ok || len(hexPart) != wantLen {
		return "", "", fmt.Errorf("invalid digest")
	}
	for i := 0; i < len(hexPart); i++ {
		c := hexPart[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", "", fmt.Errorf("invalid digest")
		}
	}
	return algo, hexPart, nil
}

func ensurePathWithinRoot(root, path string) error {
	rootClean := filepath.Clean(root)
	pathClean := filepath.Clean(path)
	rel, err := filepath.Rel(rootClean, pathClean)
	if err != nil {
		return fmt.Errorf("invalid digest")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid digest")
	}
	return nil
}

func registryWWWAuthenticateHeader() string {
	return fmt.Sprintf(`Bearer realm="http://%s/v2/",service="%s"`, store.LabRegistryHost, registryServiceName)
}

func registryDinDPullHost(listenAddr string) string {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		if host, fallbackPort, splitErr := net.SplitHostPort(store.LabRegistryHost); splitErr == nil && host != "" && fallbackPort != "" {
			port = fallbackPort
		} else {
			port = "4566"
		}
	}
	return "host.docker.internal:" + port
}

func (s *Server) writeRegistryUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", registryWWWAuthenticateHeader())
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.WriteHeader(http.StatusUnauthorized)
}

func (s *Server) writeRegistryError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]any{
		"errors": []map[string]string{
			{"code": code, "message": message},
		},
	})
	_, _ = w.Write(payload)
}

func sortStrings(ss []string) {
	for i := 0; i < len(ss); i++ {
		for j := i + 1; j < len(ss); j++ {
			if ss[j] < ss[i] {
				ss[i], ss[j] = ss[j], ss[i]
			}
		}
	}
}
