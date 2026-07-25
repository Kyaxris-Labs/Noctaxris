package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTransferServerExists   = errors.New("ResourceExistsException")
	ErrTransferServerNotFound = errors.New("ResourceNotFoundException")
	ErrTransferUserExists     = errors.New("ResourceExistsException")
	ErrTransferUserNotFound   = errors.New("ResourceNotFoundException")
	ErrTransferBadRequest     = errors.New("InvalidRequestException")
	ErrTransferPathEscape     = errors.New("InvalidRequestException: path escapes home directory")
	ErrTransferPathNotFound   = errors.New("ResourceNotFoundException")
)

const DefaultTransferRegion = "us-east-1"

const transferSchema = `
CREATE TABLE IF NOT EXISTS transfer_servers (
  account_id TEXT NOT NULL,
  server_id TEXT NOT NULL,
  server_arn TEXT NOT NULL,
  protocols TEXT NOT NULL,
  endpoint_type TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL,
  identity_provider_type TEXT NOT NULL DEFAULT 'SERVICE_MANAGED',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, server_id)
);
CREATE TABLE IF NOT EXISTS transfer_users (
  account_id TEXT NOT NULL,
  server_id TEXT NOT NULL,
  user_name TEXT NOT NULL,
  home_directory TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, server_id, user_name)
);
`

// TransferServer is a Transfer Family server row.
type TransferServer struct {
	ServerID             string
	ServerARN            string
	Protocols            string // comma-joined, lab uses SFTP
	EndpointType         string
	State                string
	IdentityProviderType string
	CreatedAt            int64
}

// TransferUser is a Transfer Family user row.
type TransferUser struct {
	ServerID      string
	UserName      string
	HomeDirectory string
	RoleARN       string
	CreatedAt     int64
}

// TransferDirEntry is one name in a lab home directory listing.
type TransferDirEntry struct {
	Name  string
	IsDir bool
	Size  int64
}

// EnsureTransferSchema creates Transfer Family tables if missing.
func EnsureTransferSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure transfer schema: db is nil")
	}
	if _, err := db.Exec(transferSchema); err != nil {
		return fmt.Errorf("ensure transfer schema: %w", err)
	}
	return nil
}

// EnsureTransferSchema ensures Transfer tables on an open store.
func (s *Store) EnsureTransferSchema() error {
	return EnsureTransferSchema(s.db)
}

// TransferServerARN builds arn:aws:transfer:REGION:ACCOUNT:server/ID
func TransferServerARN(region, accountID, serverID string) string {
	if region == "" {
		region = DefaultTransferRegion
	}
	return fmt.Sprintf("arn:aws:transfer:%s:%s:server/%s", region, accountID, serverID)
}

func (s *Store) transferHomeRoot(accountID, serverID, userName string) string {
	return filepath.Join(s.dataRoot, "transfer", accountID, serverID, "home", userName)
}

// CreateTransferServer creates a server with SFTP protocol and sandbox home roots.
func (s *Store) CreateTransferServer(accountID, region string, protocols []string) (TransferServer, error) {
	if len(protocols) == 0 {
		protocols = []string{"SFTP"}
	}
	for _, p := range protocols {
		if !strings.EqualFold(p, "SFTP") {
			return TransferServer{}, fmt.Errorf("%w: lab supports SFTP only", ErrTransferBadRequest)
		}
	}
	id := "s-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:17]
	arn := TransferServerARN(region, accountID, id)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO transfer_servers
		 (account_id, server_id, server_arn, protocols, endpoint_type, state, identity_provider_type, created_at)
		VALUES (?, ?, ?, 'SFTP', '', 'ONLINE', 'SERVICE_MANAGED', ?)`,
		accountID, id, arn, now,
	)
	if err != nil {
		return TransferServer{}, fmt.Errorf("create transfer server: %w", err)
	}
	return TransferServer{
		ServerID: id, ServerARN: arn, Protocols: "SFTP",
		EndpointType: "", State: "ONLINE", IdentityProviderType: "SERVICE_MANAGED", CreatedAt: now,
	}, nil
}

// DescribeTransferServer returns a server by id.
func (s *Store) DescribeTransferServer(accountID, serverID string) (TransferServer, error) {
	var sv TransferServer
	err := s.db.QueryRow(
		`SELECT server_id, server_arn, protocols, endpoint_type, state, identity_provider_type, created_at
		 FROM transfer_servers WHERE account_id = ? AND server_id = ?`,
		accountID, serverID,
	).Scan(&sv.ServerID, &sv.ServerARN, &sv.Protocols, &sv.EndpointType, &sv.State, &sv.IdentityProviderType, &sv.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TransferServer{}, ErrTransferServerNotFound
	}
	if err != nil {
		return TransferServer{}, fmt.Errorf("describe transfer server: %w", err)
	}
	return sv, nil
}

// ListTransferServers lists servers for an account.
func (s *Store) ListTransferServers(accountID string) ([]TransferServer, error) {
	rows, err := s.db.Query(
		`SELECT server_id, server_arn, protocols, endpoint_type, state, identity_provider_type, created_at
		 FROM transfer_servers WHERE account_id = ? ORDER BY server_id`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list transfer servers: %w", err)
	}
	defer rows.Close()
	out := []TransferServer{}
	for rows.Next() {
		var sv TransferServer
		if err := rows.Scan(&sv.ServerID, &sv.ServerARN, &sv.Protocols, &sv.EndpointType, &sv.State, &sv.IdentityProviderType, &sv.CreatedAt); err != nil {
			return nil, fmt.Errorf("list transfer servers: scan: %w", err)
		}
		out = append(out, sv)
	}
	return out, rows.Err()
}

// DeleteTransferServer deletes a server and its users plus sandbox dirs.
func (s *Store) DeleteTransferServer(accountID, serverID string) error {
	if _, err := s.DescribeTransferServer(accountID, serverID); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete transfer server: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM transfer_users WHERE account_id = ? AND server_id = ?`, accountID, serverID); err != nil {
		return fmt.Errorf("delete transfer server: users: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM transfer_servers WHERE account_id = ? AND server_id = ?`, accountID, serverID); err != nil {
		return fmt.Errorf("delete transfer server: server: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete transfer server: commit: %w", err)
	}
	_ = os.RemoveAll(filepath.Join(s.dataRoot, "transfer", accountID, serverID))
	return nil
}

// CreateTransferUser creates a user and sandbox home directory under data root.
func (s *Store) CreateTransferUser(accountID, serverID, userName, homeDirectory, roleARN string) (TransferUser, error) {
	userName = strings.TrimSpace(userName)
	if userName == "" {
		return TransferUser{}, fmt.Errorf("%w: UserName is required", ErrTransferBadRequest)
	}
	if _, err := s.DescribeTransferServer(accountID, serverID); err != nil {
		return TransferUser{}, err
	}
	if homeDirectory == "" {
		homeDirectory = "/" + userName
	}
	var existing string
	err := s.db.QueryRow(
		`SELECT user_name FROM transfer_users WHERE account_id = ? AND server_id = ? AND user_name = ?`,
		accountID, serverID, userName,
	).Scan(&existing)
	if err == nil {
		return TransferUser{}, ErrTransferUserExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TransferUser{}, fmt.Errorf("create transfer user: %w", err)
	}
	home := s.transferHomeRoot(accountID, serverID, userName)
	if err := os.MkdirAll(home, 0o750); err != nil {
		return TransferUser{}, fmt.Errorf("create transfer user: mkdir: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO transfer_users (account_id, server_id, user_name, home_directory, role_arn, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, serverID, userName, homeDirectory, roleARN, now,
	)
	if err != nil {
		return TransferUser{}, fmt.Errorf("create transfer user: insert: %w", err)
	}
	return TransferUser{
		ServerID: serverID, UserName: userName, HomeDirectory: homeDirectory, RoleARN: roleARN, CreatedAt: now,
	}, nil
}

// DeleteTransferUser deletes a user and their sandbox home.
func (s *Store) DeleteTransferUser(accountID, serverID, userName string) error {
	res, err := s.db.Exec(
		`DELETE FROM transfer_users WHERE account_id = ? AND server_id = ? AND user_name = ?`,
		accountID, serverID, userName,
	)
	if err != nil {
		return fmt.Errorf("delete transfer user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTransferUserNotFound
	}
	_ = os.RemoveAll(s.transferHomeRoot(accountID, serverID, userName))
	return nil
}

// TransferUserHomePath returns the absolute sandbox path for a user.
func (s *Store) TransferUserHomePath(accountID, serverID, userName string) (string, error) {
	var home string
	err := s.db.QueryRow(
		`SELECT home_directory FROM transfer_users WHERE account_id = ? AND server_id = ? AND user_name = ?`,
		accountID, serverID, userName,
	).Scan(&home)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTransferUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("transfer user home: %w", err)
	}
	return s.transferHomeRoot(accountID, serverID, userName), nil
}

func (s *Store) transferRequireOnlineServer(accountID, serverID string) error {
	sv, err := s.DescribeTransferServer(accountID, serverID)
	if err != nil {
		return err
	}
	if sv.State != "ONLINE" {
		return fmt.Errorf("%w: server is not ONLINE", ErrTransferBadRequest)
	}
	return nil
}

// TransferResolveUserFile maps a relative path under the user sandbox home to an absolute path.
func (s *Store) TransferResolveUserFile(accountID, serverID, userName, relativePath string) (string, error) {
	if err := s.transferRequireOnlineServer(accountID, serverID); err != nil {
		return "", err
	}
	home, err := s.TransferUserHomePath(accountID, serverID, userName)
	if err != nil {
		return "", err
	}
	return transferPathUnderHome(home, relativePath)
}

func transferPathUnderHome(homeRoot, relativePath string) (string, error) {
	homeRoot = filepath.Clean(homeRoot)
	rel := strings.TrimSpace(relativePath)
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	rel = filepath.FromSlash(rel)
	if rel == "" || rel == "." {
		return homeRoot, nil
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrTransferPathEscape
	}
	abs := filepath.Clean(filepath.Join(homeRoot, rel))
	if err := ensurePathWithinTransferHome(homeRoot, abs); err != nil {
		return "", err
	}
	return abs, nil
}

func ensurePathWithinTransferHome(homeRoot, absPath string) error {
	homeRoot = filepath.Clean(homeRoot)
	absPath = filepath.Clean(absPath)
	rel, err := filepath.Rel(homeRoot, absPath)
	if err != nil {
		return ErrTransferPathEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrTransferPathEscape
	}
	return nil
}

// TransferPutFile writes content at a path relative to the user home (lab API, not SFTP).
func (s *Store) TransferPutFile(accountID, serverID, userName, relativePath string, content []byte) error {
	abs, err := s.TransferResolveUserFile(accountID, serverID, userName, relativePath)
	if err != nil {
		return err
	}
	home, err := s.TransferUserHomePath(accountID, serverID, userName)
	if err != nil {
		return err
	}
	if abs == filepath.Clean(home) {
		return fmt.Errorf("%w: path must name a file", ErrTransferBadRequest)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return fmt.Errorf("transfer put file: mkdir: %w", err)
	}
	if err := os.WriteFile(abs, content, 0o640); err != nil {
		return fmt.Errorf("transfer put file: %w", err)
	}
	return nil
}

// TransferGetFile reads a file relative to the user home.
func (s *Store) TransferGetFile(accountID, serverID, userName, relativePath string) ([]byte, error) {
	abs, err := s.TransferResolveUserFile(accountID, serverID, userName, relativePath)
	if err != nil {
		return nil, err
	}
	home, err := s.TransferUserHomePath(accountID, serverID, userName)
	if err != nil {
		return nil, err
	}
	if abs == filepath.Clean(home) {
		return nil, fmt.Errorf("%w: path is a directory", ErrTransferBadRequest)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrTransferPathNotFound
		}
		return nil, fmt.Errorf("transfer get file: %w", err)
	}
	return data, nil
}

// TransferListDirectory lists entries under a relative directory path (empty or "/" is home root).
func (s *Store) TransferListDirectory(accountID, serverID, userName, relativePath string) ([]TransferDirEntry, error) {
	abs, err := s.TransferResolveUserFile(accountID, serverID, userName, relativePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrTransferPathNotFound
		}
		return nil, fmt.Errorf("transfer list directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: path is not a directory", ErrTransferBadRequest)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("transfer list directory: %w", err)
	}
	out := make([]TransferDirEntry, 0, len(entries))
	for _, e := range entries {
		ent := TransferDirEntry{Name: e.Name(), IsDir: e.IsDir()}
		if !e.IsDir() {
			if fi, err := e.Info(); err == nil {
				ent.Size = fi.Size()
			}
		}
		out = append(out, ent)
	}
	return out, nil
}
