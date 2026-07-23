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
		VALUES (?, ?, ?, 'SFTP', '', 'OFFLINE', 'SERVICE_MANAGED', ?)`,
		accountID, id, arn, now,
	)
	if err != nil {
		return TransferServer{}, fmt.Errorf("create transfer server: %w", err)
	}
	return TransferServer{
		ServerID: id, ServerARN: arn, Protocols: "SFTP",
		EndpointType: "", State: "OFFLINE", IdentityProviderType: "SERVICE_MANAGED", CreatedAt: now,
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
