package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrOpenSearchDomainExists   = errors.New("ResourceAlreadyExistsException")
	ErrOpenSearchDomainNotFound = errors.New("ResourceNotFoundException")
	ErrOpenSearchBadRequest     = errors.New("ValidationException")
)

const DefaultOpenSearchRegion = "us-east-1"

const openSearchSchema = `
CREATE TABLE IF NOT EXISTS opensearch_domains (
  account_id TEXT NOT NULL,
  domain_id TEXT NOT NULL,
  domain_name TEXT NOT NULL,
  domain_arn TEXT NOT NULL,
  engine_version TEXT NOT NULL DEFAULT 'OpenSearch_2.11',
  domain_status TEXT NOT NULL,
  stub_endpoint TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, domain_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_opensearch_domains_name ON opensearch_domains(account_id, domain_name);
`

// OpenSearchDomain is an OpenSearch Service domain control-plane row (stub endpoint).
type OpenSearchDomain struct {
	DomainID      string
	DomainName    string
	DomainARN     string
	EngineVersion string
	DomainStatus  string
	StubEndpoint  string
	CreatedAt     int64
}

// EnsureOpenSearchSchema creates OpenSearch tables if missing.
func EnsureOpenSearchSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure opensearch schema: db is nil")
	}
	if _, err := db.Exec(openSearchSchema); err != nil {
		return fmt.Errorf("ensure opensearch schema: %w", err)
	}
	return nil
}

// EnsureOpenSearchSchema ensures OpenSearch tables on an open store.
func (s *Store) EnsureOpenSearchSchema() error {
	return EnsureOpenSearchSchema(s.db)
}

// OpenSearchDomainARN builds arn:aws:es:REGION:ACCOUNT:domain/NAME.
func OpenSearchDomainARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultOpenSearchRegion
	}
	return fmt.Sprintf("arn:aws:es:%s:%s:domain/%s", region, accountID, name)
}

// CreateOpenSearchDomain creates a control-plane domain with a loopback-only stub endpoint.
func (s *Store) CreateOpenSearchDomain(accountID, region, name, engineVersion string) (OpenSearchDomain, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return OpenSearchDomain{}, fmt.Errorf("%w: DomainName is required", ErrOpenSearchBadRequest)
	}
	if engineVersion == "" {
		engineVersion = "OpenSearch_2.11"
	}
	var existing string
	err := s.db.QueryRow(
		`SELECT domain_id FROM opensearch_domains WHERE account_id = ? AND domain_name = ?`,
		accountID, name,
	).Scan(&existing)
	if err == nil {
		return OpenSearchDomain{}, ErrOpenSearchDomainExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return OpenSearchDomain{}, fmt.Errorf("create opensearch domain: %w", err)
	}
	id := uuid.NewString()
	arn := OpenSearchDomainARN(region, accountID, name)
	stub := fmt.Sprintf("stub://127.0.0.1/opensearch/%s", id)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO opensearch_domains
		 (account_id, domain_id, domain_name, domain_arn, engine_version, domain_status, stub_endpoint, created_at)
		 VALUES (?, ?, ?, ?, ?, 'Active', ?, ?)`,
		accountID, id, name, arn, engineVersion, stub, now,
	)
	if err != nil {
		return OpenSearchDomain{}, fmt.Errorf("create opensearch domain: insert: %w", err)
	}
	return OpenSearchDomain{
		DomainID: id, DomainName: name, DomainARN: arn,
		EngineVersion: engineVersion, DomainStatus: "Active",
		StubEndpoint: stub, CreatedAt: now,
	}, nil
}

// DescribeOpenSearchDomain returns a domain by name.
func (s *Store) DescribeOpenSearchDomain(accountID, name string) (OpenSearchDomain, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return OpenSearchDomain{}, fmt.Errorf("%w: DomainName is required", ErrOpenSearchBadRequest)
	}
	var d OpenSearchDomain
	err := s.db.QueryRow(
		`SELECT domain_id, domain_name, domain_arn, engine_version, domain_status, stub_endpoint, created_at
		 FROM opensearch_domains WHERE account_id = ? AND domain_name = ?`,
		accountID, name,
	).Scan(&d.DomainID, &d.DomainName, &d.DomainARN, &d.EngineVersion, &d.DomainStatus, &d.StubEndpoint, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OpenSearchDomain{}, ErrOpenSearchDomainNotFound
	}
	if err != nil {
		return OpenSearchDomain{}, fmt.Errorf("describe opensearch domain: %w", err)
	}
	return d, nil
}

// ListOpenSearchDomainNames lists domains for an account.
func (s *Store) ListOpenSearchDomainNames(accountID string) ([]OpenSearchDomain, error) {
	rows, err := s.db.Query(
		`SELECT domain_id, domain_name, domain_arn, engine_version, domain_status, stub_endpoint, created_at
		 FROM opensearch_domains WHERE account_id = ? ORDER BY domain_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list opensearch domains: %w", err)
	}
	defer rows.Close()
	out := []OpenSearchDomain{}
	for rows.Next() {
		var d OpenSearchDomain
		if err := rows.Scan(&d.DomainID, &d.DomainName, &d.DomainARN, &d.EngineVersion, &d.DomainStatus, &d.StubEndpoint, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("list opensearch domains: scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteOpenSearchDomain deletes a domain by name.
func (s *Store) DeleteOpenSearchDomain(accountID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: DomainName is required", ErrOpenSearchBadRequest)
	}
	res, err := s.db.Exec(`DELETE FROM opensearch_domains WHERE account_id = ? AND domain_name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete opensearch domain: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrOpenSearchDomainNotFound
	}
	return nil
}
