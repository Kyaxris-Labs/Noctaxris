package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
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

// Nested OpenSearch HTTP port inside the DinD network (never published on the host).
const OpenSearchNestedPort = 9200

// OpenSearch domain statuses used by the lab control plane.
const (
	OpenSearchDomainStatusCreating     = "Creating"
	OpenSearchDomainStatusCreateFailed = "CreateFailed"
	OpenSearchDomainStatusActive       = "Active"
)

const (
	openSearchLabHostPrefix    = "noctaxris-opensearch-"
	openSearchLabHostPrefixAlt = "noctaxris-data-opensearch-"
)

// AWS OpenSearch DomainName: min 3, max 28, pattern [a-z][a-z0-9\-]+ (no dots).
// https://docs.aws.amazon.com/opensearch-service/latest/APIReference/API_CreateDomain.html
var openSearchDomainNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{2,27}$`)

const openSearchSchema = `
CREATE TABLE IF NOT EXISTS opensearch_domains (
  account_id TEXT NOT NULL,
  domain_id TEXT NOT NULL,
  domain_name TEXT NOT NULL,
  domain_arn TEXT NOT NULL,
  engine_version TEXT NOT NULL DEFAULT 'OpenSearch_2.11',
  domain_status TEXT NOT NULL,
  stub_endpoint TEXT NOT NULL,
  container_id TEXT NOT NULL DEFAULT '',
  failure_reason TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, domain_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_opensearch_domains_name ON opensearch_domains(account_id, domain_name);
`

// OpenSearchDomain is an OpenSearch Service domain control-plane row.
// Endpoint is stub:// until a nested OpenSearch container is Active.
type OpenSearchDomain struct {
	DomainID      string
	DomainName    string
	DomainARN     string
	EngineVersion string
	DomainStatus  string
	StubEndpoint  string
	ContainerID   string
	FailureReason string
	CreatedAt     int64
}

// EnsureOpenSearchSchema creates OpenSearch tables if missing and migrates columns.
func EnsureOpenSearchSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure opensearch schema: db is nil")
	}
	if _, err := db.Exec(openSearchSchema); err != nil {
		return fmt.Errorf("ensure opensearch schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE opensearch_domains ADD COLUMN container_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE opensearch_domains ADD COLUMN failure_reason TEXT NOT NULL DEFAULT ''`,
	}); err != nil {
		return fmt.Errorf("ensure opensearch schema: migrate: %w", err)
	}
	return nil
}

// EnsureOpenSearchSchema ensures OpenSearch tables on an open store.
func (s *Store) EnsureOpenSearchSchema() error {
	return EnsureOpenSearchSchema(s.db)
}

// ValidateOpenSearchDomainName enforces AWS CreateDomain DomainName charset
// (lowercase letters, digits, hyphen; 3-28 chars; no dots).
func ValidateOpenSearchDomainName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: DomainName is required", ErrOpenSearchBadRequest)
	}
	if !openSearchDomainNameRE.MatchString(name) {
		return fmt.Errorf("%w: DomainName must be 3-28 chars matching [a-z][a-z0-9-]* (no dots)", ErrOpenSearchBadRequest)
	}
	return nil
}

// ValidateNestedOpenSearchHost allows only DinD nested OpenSearch hostnames.
// Loopback, wildcards, raw IPs, and dotted suffixes after the lab prefix are rejected
// so callers never dial attacker-controlled DNS (e.g. noctaxris-opensearch-ssrf.attacker.com).
func ValidateNestedOpenSearchHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("nested opensearch endpoint is empty")
	}
	lower := strings.ToLower(host)
	switch lower {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "*", "host.docker.internal":
		return fmt.Errorf("refusing non-nested opensearch host %q", host)
	}
	if strings.Contains(host, "/") || strings.Contains(host, "\\") {
		return fmt.Errorf("invalid nested opensearch host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("refusing IP opensearch host %q (nested container DNS name required)", host)
	}
	if !strings.HasPrefix(lower, openSearchLabHostPrefix) && !strings.HasPrefix(lower, openSearchLabHostPrefixAlt) {
		return fmt.Errorf("nested opensearch host %q is not a data-plane endpoint", host)
	}
	suffix := strings.TrimPrefix(lower, openSearchLabHostPrefix)
	if strings.HasPrefix(lower, openSearchLabHostPrefixAlt) {
		suffix = strings.TrimPrefix(lower, openSearchLabHostPrefixAlt)
	}
	if suffix == "" {
		return fmt.Errorf("nested opensearch host %q is missing a domain suffix", host)
	}
	if strings.Contains(suffix, ".") {
		return fmt.Errorf("refusing dotted nested opensearch host %q", host)
	}
	return nil
}

// PinnedNestedOpenSearchDialContext resolves addr, re-validates the nested hostname
// at dial time, and connects to a resolved IP (allows private Docker DNS addresses).
// Unlike SNS/JWKS pinned dialers, it does not require public IPs.
func PinnedNestedOpenSearchDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("opensearch: dial addr: %w", err)
	}
	if err := ValidateNestedOpenSearchHost(host); err != nil {
		return nil, err
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		if ip := net.ParseIP(host); ip != nil {
			return nil, fmt.Errorf("opensearch: refusing IP dial host %q", host)
		}
		return nil, fmt.Errorf("opensearch: resolve dial host: %w", err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("opensearch: dial host resolved to no addresses")
	}
	var dialer net.Dialer
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("opensearch: no dial target")
}

// NestedOpenSearchHTTPClient returns an HTTP client that dials only allowlisted
// nested OpenSearch hosts (pinned dial) and refuses off-network redirects.
func NestedOpenSearchHTTPClient(timeout time.Duration, checkRedirect func(*http.Request, []*http.Request) error) *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	var transport *http.Transport
	if ok {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}
	transport.DialContext = PinnedNestedOpenSearchDialContext
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
}

// OpenSearchDomainARN builds arn:aws:es:REGION:ACCOUNT:domain/NAME.
func OpenSearchDomainARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultOpenSearchRegion
	}
	return fmt.Sprintf("arn:aws:es:%s:%s:domain/%s", region, accountID, name)
}

// OpenSearchNestedEndpoint returns the nested-network host:port string (no host publish).
func OpenSearchNestedEndpoint(domainName string) string {
	name := strings.ToLower(strings.TrimSpace(domainName))
	return fmt.Sprintf("noctaxris-opensearch-%s:%d", name, OpenSearchNestedPort)
}

// CreateOpenSearchDomain creates a control-plane domain in Creating until nested promote/fail.
// Never inserts Active. Without a healthy nested container the wire marks CreateFailed.
func (s *Store) CreateOpenSearchDomain(accountID, region, name, engineVersion string) (OpenSearchDomain, error) {
	name = strings.TrimSpace(name)
	if err := ValidateOpenSearchDomainName(name); err != nil {
		return OpenSearchDomain{}, err
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
	// Planned nested endpoint while Creating; stub:// only after CreateFailed.
	endpoint := OpenSearchNestedEndpoint(name)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO opensearch_domains
		 (account_id, domain_id, domain_name, domain_arn, engine_version, domain_status, stub_endpoint, container_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '', ?)`,
		accountID, id, name, arn, engineVersion, OpenSearchDomainStatusCreating, endpoint, now,
	)
	if err != nil {
		return OpenSearchDomain{}, fmt.Errorf("create opensearch domain: insert: %w", err)
	}
	return OpenSearchDomain{
		DomainID: id, DomainName: name, DomainARN: arn,
		EngineVersion: engineVersion, DomainStatus: OpenSearchDomainStatusCreating,
		StubEndpoint: endpoint, CreatedAt: now,
	}, nil
}

// SetOpenSearchContainerID records a nested engine container after dataplane start.
// endpoint may be empty to leave the existing endpoint unchanged.
// Active requires a non-empty containerID and must not use stub://.
// failureReason is stored on CreateFailed (operator hint); cleared on Active.
func (s *Store) SetOpenSearchContainerID(accountID, domainName, containerID, status, endpoint, failureReason string) error {
	domainName = strings.TrimSpace(domainName)
	if status == "" {
		status = OpenSearchDomainStatusActive
	}
	if status == OpenSearchDomainStatusActive {
		if strings.TrimSpace(containerID) == "" {
			return fmt.Errorf("%w: Active requires container_id", ErrOpenSearchBadRequest)
		}
		if strings.HasPrefix(strings.TrimSpace(endpoint), "stub://") {
			return fmt.Errorf("%w: Active must not use stub:// endpoint", ErrOpenSearchBadRequest)
		}
		failureReason = ""
	}
	if status == OpenSearchDomainStatusCreateFailed && strings.TrimSpace(endpoint) == "" {
		// Fail-closed: restore loopback stub so callers never treat nested host as live.
		var domainID string
		_ = s.db.QueryRow(
			`SELECT domain_id FROM opensearch_domains WHERE account_id = ? AND domain_name = ?`,
			accountID, domainName,
		).Scan(&domainID)
		if domainID != "" {
			endpoint = fmt.Sprintf("stub://127.0.0.1/opensearch/%s", domainID)
		}
	}
	var res sql.Result
	var err error
	if strings.TrimSpace(endpoint) == "" {
		res, err = s.db.Exec(
			`UPDATE opensearch_domains SET container_id = ?, domain_status = ?, failure_reason = ? WHERE account_id = ? AND domain_name = ?`,
			containerID, status, failureReason, accountID, domainName,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE opensearch_domains SET container_id = ?, domain_status = ?, stub_endpoint = ?, failure_reason = ? WHERE account_id = ? AND domain_name = ?`,
			containerID, status, endpoint, failureReason, accountID, domainName,
		)
	}
	if err != nil {
		return fmt.Errorf("set opensearch container: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrOpenSearchDomainNotFound
	}
	return nil
}

// DescribeOpenSearchDomain returns a domain by name.
func (s *Store) DescribeOpenSearchDomain(accountID, name string) (OpenSearchDomain, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return OpenSearchDomain{}, fmt.Errorf("%w: DomainName is required", ErrOpenSearchBadRequest)
	}
	var d OpenSearchDomain
	err := s.db.QueryRow(
		`SELECT domain_id, domain_name, domain_arn, engine_version, domain_status, stub_endpoint, container_id, failure_reason, created_at
		 FROM opensearch_domains WHERE account_id = ? AND domain_name = ?`,
		accountID, name,
	).Scan(&d.DomainID, &d.DomainName, &d.DomainARN, &d.EngineVersion, &d.DomainStatus, &d.StubEndpoint, &d.ContainerID, &d.FailureReason, &d.CreatedAt)
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
		`SELECT domain_id, domain_name, domain_arn, engine_version, domain_status, stub_endpoint, container_id, failure_reason, created_at
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
		if err := rows.Scan(&d.DomainID, &d.DomainName, &d.DomainARN, &d.EngineVersion, &d.DomainStatus, &d.StubEndpoint, &d.ContainerID, &d.FailureReason, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("list opensearch domains: scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteOpenSearchDomain deletes a domain by name. Returns container_id if set (for dataplane stop).
func (s *Store) DeleteOpenSearchDomain(accountID, name string) (containerID string, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: DomainName is required", ErrOpenSearchBadRequest)
	}
	err = s.db.QueryRow(
		`SELECT container_id FROM opensearch_domains WHERE account_id = ? AND domain_name = ?`,
		accountID, name,
	).Scan(&containerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrOpenSearchDomainNotFound
	}
	if err != nil {
		return "", fmt.Errorf("delete opensearch domain: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM opensearch_domains WHERE account_id = ? AND domain_name = ?`, accountID, name)
	if err != nil {
		return "", fmt.Errorf("delete opensearch domain: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", ErrOpenSearchDomainNotFound
	}
	return containerID, nil
}
