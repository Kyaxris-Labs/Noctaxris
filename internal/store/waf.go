package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrWAFNotFound      = errors.New("WAFNonexistentItemException")
	ErrWAFAlreadyExists = errors.New("WAFDuplicateItemException")
	ErrWAFBadRequest    = errors.New("WAFInvalidParameterException")
)

const DefaultWAFRegion = "us-east-1"

const wafSchema = `
CREATE TABLE IF NOT EXISTS wafv2_web_acls (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  id TEXT NOT NULL,
  scope TEXT NOT NULL,
  arn TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  default_action TEXT NOT NULL,
  rules_json TEXT NOT NULL DEFAULT '[]',
  lock_token TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name, scope)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_waf_id ON wafv2_web_acls(account_id, id);
CREATE TABLE IF NOT EXISTS wafv2_associations (
  account_id TEXT NOT NULL,
  web_acl_arn TEXT NOT NULL,
  resource_arn TEXT NOT NULL,
  PRIMARY KEY (account_id, resource_arn)
);
CREATE TABLE IF NOT EXISTS wafv2_rule_groups (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  id TEXT NOT NULL,
  scope TEXT NOT NULL,
  arn TEXT NOT NULL,
  capacity INTEGER NOT NULL DEFAULT 1,
  rules_json TEXT NOT NULL DEFAULT '[]',
  lock_token TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name, scope)
);
CREATE TABLE IF NOT EXISTS wafv2_ip_sets (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  id TEXT NOT NULL,
  scope TEXT NOT NULL,
  arn TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  ip_address_version TEXT NOT NULL,
  addresses_json TEXT NOT NULL DEFAULT '[]',
  lock_token TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name, scope)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_waf_ipset_id ON wafv2_ip_sets(account_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_waf_ipset_arn ON wafv2_ip_sets(account_id, arn);
`

// WAFWebACL is a WAFv2 Web ACL row.
type WAFWebACL struct {
	Name          string
	ID            string
	Scope         string
	ARN           string
	Description   string
	DefaultAction string // Allow or Block
	RulesJSON     string
	LockToken     string
	CreatedAt     int64
}

// WAFRuleGroup is a lite rule group shape.
type WAFRuleGroup struct {
	Name      string
	ID        string
	Scope     string
	ARN       string
	Capacity  int
	RulesJSON string
	LockToken string
	CreatedAt int64
}

// WAFByteMatchStatement is a lab subset of AWS WAFv2 ByteMatchStatement.
type WAFByteMatchStatement struct {
	SearchString         string `json:"SearchString"`
	PositionalConstraint string `json:"PositionalConstraint"` // CONTAINS or EXACTLY
	FieldToMatchType     string `json:"FieldToMatchType"`     // UriPath or SingleHeader
	HeaderName           string `json:"HeaderName,omitempty"` // required for SingleHeader
}

// WAFSizeConstraintStatement is a lab subset of AWS WAFv2 SizeConstraintStatement.
type WAFSizeConstraintStatement struct {
	FieldToMatchType   string `json:"FieldToMatchType"`     // UriPath or SingleHeader
	HeaderName         string `json:"HeaderName,omitempty"` // required for SingleHeader
	ComparisonOperator string `json:"ComparisonOperator"`   // EQ NE LE LT GE GT
	Size               int64  `json:"Size"`
}

// WAFIPSet is a persisted WAFv2 IP set resource.
type WAFIPSet struct {
	Name             string
	ID               string
	Scope            string
	ARN              string
	Description      string
	IPAddressVersion string // IPV4 or IPV6
	Addresses        []string
	AddressesJSON    string
	LockToken        string
	CreatedAt        int64
}

// WAFIPSetReferenceStatement is a lab IP set match: ARN of a stored IPSet and/or inline CIDRs.
type WAFIPSetReferenceStatement struct {
	ARN       string   `json:"ARN,omitempty"`       // IPSet ARN; unknown ARNs fail closed on evaluate
	Addresses []string `json:"Addresses,omitempty"` // inline CIDR or host (host becomes /32 or /128)
}

// WAFRequestView carries invoke-path fields used for statement evaluation.
type WAFRequestView struct {
	URI      string
	Headers  map[string]string
	SourceIP string // client address for IPSet; from RemoteAddr or first XFF hop
}

// WAFRule is an allow/block rule with optional label or statement.
type WAFRule struct {
	Name                    string                      `json:"Name"`
	Priority                int                         `json:"Priority"`
	Action                  string                      `json:"Action"` // Allow or Block
	Label                   string                      `json:"Label,omitempty"`
	ByteMatchStatement      *WAFByteMatchStatement      `json:"ByteMatchStatement,omitempty"`
	SizeConstraintStatement *WAFSizeConstraintStatement `json:"SizeConstraintStatement,omitempty"`
	IPSetReferenceStatement *WAFIPSetReferenceStatement `json:"IPSetReferenceStatement,omitempty"`
}

// EnsureWAFSchema creates WAFv2 tables if missing.
func EnsureWAFSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure waf schema: db is nil")
	}
	if _, err := db.Exec(wafSchema); err != nil {
		return fmt.Errorf("ensure waf schema: %w", err)
	}
	return nil
}

// EnsureWAFSchema ensures WAFv2 tables on an open store.
func (s *Store) EnsureWAFSchema() error {
	return EnsureWAFSchema(s.db)
}

// WAFWebACLARN builds arn:aws:wafv2:REGION:ACCOUNT:SCOPE/webacl/NAME/ID
func WAFWebACLARN(region, accountID, scope, name, id string) string {
	if region == "" {
		region = DefaultWAFRegion
	}
	scope = strings.ToLower(scope)
	return fmt.Sprintf("arn:aws:wafv2:%s:%s:%s/webacl/%s/%s", region, accountID, scope, name, id)
}

// WAFIPSetARN builds arn:aws:wafv2:REGION:ACCOUNT:SCOPE/ipset/NAME/ID
func WAFIPSetARN(region, accountID, scope, name, id string) string {
	if region == "" {
		region = DefaultWAFRegion
	}
	scope = strings.ToLower(scope)
	return fmt.Sprintf("arn:aws:wafv2:%s:%s:%s/ipset/%s/%s", region, accountID, scope, name, id)
}

// CreateWAFWebACL creates a Web ACL.
func (s *Store) CreateWAFWebACL(accountID, region, name, scope, description, defaultAction string, rules []WAFRule) (WAFWebACL, error) {
	name = strings.TrimSpace(name)
	scope = strings.ToUpper(strings.TrimSpace(scope))
	if name == "" || (scope != "REGIONAL" && scope != "CLOUDFRONT") {
		return WAFWebACL{}, fmt.Errorf("%w: Name and Scope (REGIONAL|CLOUDFRONT) required", ErrWAFBadRequest)
	}
	if defaultAction == "" {
		defaultAction = "Allow"
	}
	if !strings.EqualFold(defaultAction, "Allow") && !strings.EqualFold(defaultAction, "Block") {
		return WAFWebACL{}, fmt.Errorf("%w: DefaultAction must be Allow or Block", ErrWAFBadRequest)
	}
	if rules == nil {
		rules = []WAFRule{}
	}
	rulesJSON, _ := json.Marshal(rules)
	id := uuid.NewString()
	lock := uuid.NewString()
	arn := WAFWebACLARN(region, accountID, scope, name, id)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO wafv2_web_acls (account_id, name, id, scope, arn, description, default_action, rules_json, lock_token, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, id, scope, arn, description, defaultAction, string(rulesJSON), lock, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return WAFWebACL{}, ErrWAFAlreadyExists
		}
		return WAFWebACL{}, fmt.Errorf("create web acl: %w", err)
	}
	return WAFWebACL{
		Name: name, ID: id, Scope: scope, ARN: arn, Description: description,
		DefaultAction: defaultAction, RulesJSON: string(rulesJSON), LockToken: lock, CreatedAt: now,
	}, nil
}

// UpdateWAFWebACL updates rules and default action.
func (s *Store) UpdateWAFWebACL(accountID, name, scope, lockToken, defaultAction string, rules []WAFRule) (WAFWebACL, error) {
	acl, err := s.GetWAFWebACL(accountID, name, scope, "")
	if err != nil {
		return WAFWebACL{}, err
	}
	if lockToken != "" && lockToken != acl.LockToken {
		return WAFWebACL{}, fmt.Errorf("%w: lock token mismatch", ErrWAFBadRequest)
	}
	if defaultAction == "" {
		defaultAction = acl.DefaultAction
	}
	if rules == nil {
		rules = []WAFRule{}
		_ = json.Unmarshal([]byte(acl.RulesJSON), &rules)
	}
	rulesJSON, _ := json.Marshal(rules)
	newLock := uuid.NewString()
	_, err = s.db.Exec(
		`UPDATE wafv2_web_acls SET default_action = ?, rules_json = ?, lock_token = ? WHERE account_id = ? AND name = ? AND scope = ?`,
		defaultAction, string(rulesJSON), newLock, accountID, name, strings.ToUpper(scope),
	)
	if err != nil {
		return WAFWebACL{}, fmt.Errorf("update web acl: %w", err)
	}
	acl.DefaultAction = defaultAction
	acl.RulesJSON = string(rulesJSON)
	acl.LockToken = newLock
	return acl, nil
}

// GetWAFWebACL returns a Web ACL by name+scope or id.
func (s *Store) GetWAFWebACL(accountID, name, scope, id string) (WAFWebACL, error) {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	var a WAFWebACL
	var err error
	if id != "" {
		err = s.db.QueryRow(
			`SELECT name, id, scope, arn, description, default_action, rules_json, lock_token, created_at
			 FROM wafv2_web_acls WHERE account_id = ? AND id = ?`,
			accountID, id,
		).Scan(&a.Name, &a.ID, &a.Scope, &a.ARN, &a.Description, &a.DefaultAction, &a.RulesJSON, &a.LockToken, &a.CreatedAt)
	} else {
		err = s.db.QueryRow(
			`SELECT name, id, scope, arn, description, default_action, rules_json, lock_token, created_at
			 FROM wafv2_web_acls WHERE account_id = ? AND name = ? AND scope = ?`,
			accountID, name, scope,
		).Scan(&a.Name, &a.ID, &a.Scope, &a.ARN, &a.Description, &a.DefaultAction, &a.RulesJSON, &a.LockToken, &a.CreatedAt)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return WAFWebACL{}, ErrWAFNotFound
	}
	if err != nil {
		return WAFWebACL{}, fmt.Errorf("get web acl: %w", err)
	}
	return a, nil
}

// ListWAFWebACLs lists Web ACLs for a scope.
func (s *Store) ListWAFWebACLs(accountID, scope string) ([]WAFWebACL, error) {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	rows, err := s.db.Query(
		`SELECT name, id, scope, arn, description, default_action, rules_json, lock_token, created_at
		 FROM wafv2_web_acls WHERE account_id = ? AND scope = ? ORDER BY name`,
		accountID, scope,
	)
	if err != nil {
		return nil, fmt.Errorf("list web acls: %w", err)
	}
	defer rows.Close()
	var out []WAFWebACL
	for rows.Next() {
		var a WAFWebACL
		if err := rows.Scan(&a.Name, &a.ID, &a.Scope, &a.ARN, &a.Description, &a.DefaultAction, &a.RulesJSON, &a.LockToken, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("list web acls scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateWAFRuleGroup creates a lite rule group.
func (s *Store) CreateWAFRuleGroup(accountID, region, name, scope string, capacity int, rules []WAFRule) (WAFRuleGroup, error) {
	name = strings.TrimSpace(name)
	scope = strings.ToUpper(strings.TrimSpace(scope))
	if name == "" || (scope != "REGIONAL" && scope != "CLOUDFRONT") {
		return WAFRuleGroup{}, fmt.Errorf("%w: Name and Scope required", ErrWAFBadRequest)
	}
	if capacity <= 0 {
		capacity = 1
	}
	if rules == nil {
		rules = []WAFRule{}
	}
	rulesJSON, _ := json.Marshal(rules)
	id := uuid.NewString()
	lock := uuid.NewString()
	arn := fmt.Sprintf("arn:aws:wafv2:%s:%s:%s/rulegroup/%s/%s", wafRegionOrDefault(region), accountID, strings.ToLower(scope), name, id)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO wafv2_rule_groups (account_id, name, id, scope, arn, capacity, rules_json, lock_token, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, id, scope, arn, capacity, string(rulesJSON), lock, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return WAFRuleGroup{}, ErrWAFAlreadyExists
		}
		return WAFRuleGroup{}, fmt.Errorf("create rule group: %w", err)
	}
	return WAFRuleGroup{
		Name: name, ID: id, Scope: scope, ARN: arn, Capacity: capacity,
		RulesJSON: string(rulesJSON), LockToken: lock, CreatedAt: now,
	}, nil
}

func wafRegionOrDefault(region string) string {
	if region == "" {
		return DefaultWAFRegion
	}
	return region
}

func normalizeWAFIPSetAddresses(addresses []string) []string {
	if addresses == nil {
		return []string{}
	}
	out := make([]string, 0, len(addresses))
	for _, a := range addresses {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

func decodeWAFIPSetAddresses(addressesJSON string) []string {
	var addrs []string
	if addressesJSON == "" {
		return []string{}
	}
	_ = json.Unmarshal([]byte(addressesJSON), &addrs)
	return normalizeWAFIPSetAddresses(addrs)
}

func scanWAFIPSet(row interface {
	Scan(dest ...any) error
}) (WAFIPSet, error) {
	var ip WAFIPSet
	err := row.Scan(
		&ip.Name, &ip.ID, &ip.Scope, &ip.ARN, &ip.Description,
		&ip.IPAddressVersion, &ip.AddressesJSON, &ip.LockToken, &ip.CreatedAt,
	)
	if err != nil {
		return WAFIPSet{}, err
	}
	ip.Addresses = decodeWAFIPSetAddresses(ip.AddressesJSON)
	return ip, nil
}

// CreateWAFIPSet creates a persisted IP set.
func (s *Store) CreateWAFIPSet(accountID, region, name, scope, description, ipAddressVersion string, addresses []string) (WAFIPSet, error) {
	name = strings.TrimSpace(name)
	scope = strings.ToUpper(strings.TrimSpace(scope))
	ipAddressVersion = strings.ToUpper(strings.TrimSpace(ipAddressVersion))
	if name == "" || (scope != "REGIONAL" && scope != "CLOUDFRONT") {
		return WAFIPSet{}, fmt.Errorf("%w: Name and Scope (REGIONAL|CLOUDFRONT) required", ErrWAFBadRequest)
	}
	if ipAddressVersion != "IPV4" && ipAddressVersion != "IPV6" {
		return WAFIPSet{}, fmt.Errorf("%w: IPAddressVersion must be IPV4 or IPV6", ErrWAFBadRequest)
	}
	addresses = normalizeWAFIPSetAddresses(addresses)
	for _, addr := range addresses {
		if _, err := wafParseCIDR(addr); err != nil {
			return WAFIPSet{}, fmt.Errorf("%w: invalid address %q", ErrWAFBadRequest, addr)
		}
	}
	addrJSON, err := json.Marshal(addresses)
	if err != nil {
		return WAFIPSet{}, fmt.Errorf("create ip set: marshal addresses: %w", err)
	}
	id := uuid.NewString()
	lock := uuid.NewString()
	arn := WAFIPSetARN(region, accountID, scope, name, id)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO wafv2_ip_sets (account_id, name, id, scope, arn, description, ip_address_version, addresses_json, lock_token, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, id, scope, arn, description, ipAddressVersion, string(addrJSON), lock, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return WAFIPSet{}, ErrWAFAlreadyExists
		}
		return WAFIPSet{}, fmt.Errorf("create ip set: %w", err)
	}
	return WAFIPSet{
		Name: name, ID: id, Scope: scope, ARN: arn, Description: description,
		IPAddressVersion: ipAddressVersion, Addresses: addresses, AddressesJSON: string(addrJSON),
		LockToken: lock, CreatedAt: now,
	}, nil
}

// GetWAFIPSet returns an IP set by name+scope or id.
func (s *Store) GetWAFIPSet(accountID, name, scope, id string) (WAFIPSet, error) {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	var (
		ip  WAFIPSet
		err error
	)
	if id != "" {
		ip, err = scanWAFIPSet(s.db.QueryRow(
			`SELECT name, id, scope, arn, description, ip_address_version, addresses_json, lock_token, created_at
			 FROM wafv2_ip_sets WHERE account_id = ? AND id = ?`,
			accountID, id,
		))
	} else {
		ip, err = scanWAFIPSet(s.db.QueryRow(
			`SELECT name, id, scope, arn, description, ip_address_version, addresses_json, lock_token, created_at
			 FROM wafv2_ip_sets WHERE account_id = ? AND name = ? AND scope = ?`,
			accountID, name, scope,
		))
	}
	if errors.Is(err, sql.ErrNoRows) {
		return WAFIPSet{}, ErrWAFNotFound
	}
	if err != nil {
		return WAFIPSet{}, fmt.Errorf("get ip set: %w", err)
	}
	return ip, nil
}

// GetWAFIPSetByARN returns an IP set by ARN within the account.
func (s *Store) GetWAFIPSetByARN(accountID, arn string) (WAFIPSet, error) {
	arn = strings.TrimSpace(arn)
	if arn == "" {
		return WAFIPSet{}, ErrWAFNotFound
	}
	ip, err := scanWAFIPSet(s.db.QueryRow(
		`SELECT name, id, scope, arn, description, ip_address_version, addresses_json, lock_token, created_at
		 FROM wafv2_ip_sets WHERE account_id = ? AND arn = ?`,
		accountID, arn,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return WAFIPSet{}, ErrWAFNotFound
	}
	if err != nil {
		return WAFIPSet{}, fmt.Errorf("get ip set by arn: %w", err)
	}
	return ip, nil
}

// UpdateWAFIPSet replaces Addresses when LockToken matches.
func (s *Store) UpdateWAFIPSet(accountID, name, scope, id, lockToken string, addresses []string) (WAFIPSet, error) {
	ip, err := s.GetWAFIPSet(accountID, name, scope, id)
	if err != nil {
		return WAFIPSet{}, err
	}
	if lockToken != "" && lockToken != ip.LockToken {
		return WAFIPSet{}, fmt.Errorf("%w: lock token mismatch", ErrWAFBadRequest)
	}
	addresses = normalizeWAFIPSetAddresses(addresses)
	for _, addr := range addresses {
		if _, err := wafParseCIDR(addr); err != nil {
			return WAFIPSet{}, fmt.Errorf("%w: invalid address %q", ErrWAFBadRequest, addr)
		}
	}
	addrJSON, err := json.Marshal(addresses)
	if err != nil {
		return WAFIPSet{}, fmt.Errorf("update ip set: marshal addresses: %w", err)
	}
	newLock := uuid.NewString()
	_, err = s.db.Exec(
		`UPDATE wafv2_ip_sets SET addresses_json = ?, lock_token = ? WHERE account_id = ? AND id = ?`,
		string(addrJSON), newLock, accountID, ip.ID,
	)
	if err != nil {
		return WAFIPSet{}, fmt.Errorf("update ip set: %w", err)
	}
	ip.Addresses = addresses
	ip.AddressesJSON = string(addrJSON)
	ip.LockToken = newLock
	return ip, nil
}

// DeleteWAFIPSet deletes an IP set when LockToken matches.
func (s *Store) DeleteWAFIPSet(accountID, name, scope, id, lockToken string) error {
	ip, err := s.GetWAFIPSet(accountID, name, scope, id)
	if err != nil {
		return err
	}
	if lockToken != "" && lockToken != ip.LockToken {
		return fmt.Errorf("%w: lock token mismatch", ErrWAFBadRequest)
	}
	_, err = s.db.Exec(
		`DELETE FROM wafv2_ip_sets WHERE account_id = ? AND id = ?`,
		accountID, ip.ID,
	)
	if err != nil {
		return fmt.Errorf("delete ip set: %w", err)
	}
	return nil
}

// ListWAFIPSets lists IP sets for a scope.
func (s *Store) ListWAFIPSets(accountID, scope string) ([]WAFIPSet, error) {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	rows, err := s.db.Query(
		`SELECT name, id, scope, arn, description, ip_address_version, addresses_json, lock_token, created_at
		 FROM wafv2_ip_sets WHERE account_id = ? AND scope = ? ORDER BY name`,
		accountID, scope,
	)
	if err != nil {
		return nil, fmt.Errorf("list ip sets: %w", err)
	}
	defer rows.Close()
	var out []WAFIPSet
	for rows.Next() {
		ip, err := scanWAFIPSet(rows)
		if err != nil {
			return nil, fmt.Errorf("list ip sets scan: %w", err)
		}
		out = append(out, ip)
	}
	return out, rows.Err()
}

// IsWAFAssociableResourceARN reports whether ResourceArn is a lab-accepted
// association target. Unknown shapes fail closed.
//
// Accepted (AWS WAFv2 AssociateWebACL shapes plus lab HTTP API) when an invoke
// enforce path exists:
//   - arn:aws:apigateway:REGION::/apis/APIID[/stages/STAGE] (HTTP API lab)
//   - arn:aws:execute-api:REGION:ACCOUNT:APIID[/STAGE[/route]]
//   - arn:aws:appsync:REGION:ACCOUNT:apis/APIID
//   - arn:aws:lambda:REGION:ACCOUNT:function:NAME (lab Function URL associate)
//   - arn:aws:elasticloadbalancing:REGION:ACCOUNT:loadbalancer/app/NAME/ID (ELB lab listener)
//
// Rejected (no enforce path yet): apigateway restapis, cognito-idp user pools.
func IsWAFAssociableResourceARN(resourceARN string) bool {
	resourceARN = strings.TrimSpace(resourceARN)
	if resourceARN == "" || !strings.HasPrefix(resourceARN, "arn:aws:") {
		return false
	}
	parts := strings.SplitN(resourceARN, ":", 6)
	if len(parts) < 6 {
		return false
	}
	service := parts[2]
	resource := parts[5]
	switch service {
	case "apigateway":
		// parts[5] is like "/apis/xxx" (leading slash in resource).
		// restapis are rejected until a REST enforce path exists.
		r := resource
		if strings.HasPrefix(r, "/") {
			r = r[1:]
		}
		if strings.HasPrefix(r, "apis/") {
			rest := strings.TrimPrefix(r, "apis/")
			if rest == "" || strings.HasPrefix(rest, "/") {
				return false
			}
			return true
		}
		return false
	case "execute-api":
		segs := strings.Split(resource, "/")
		return len(segs) >= 1 && segs[0] != ""
	case "appsync":
		return strings.HasPrefix(resource, "apis/") && len(strings.TrimPrefix(resource, "apis/")) > 0
	case "lambda":
		// Lab Function URL association: arn:aws:lambda:REGION:ACCOUNT:function:NAME
		return strings.HasPrefix(resource, "function:") && len(strings.TrimPrefix(resource, "function:")) > 0
	case "elasticloadbalancing":
		// Application LB ARN only (NLB net/ rejected). Lab listener enforces on LB ARN.
		if !strings.HasPrefix(resource, "loadbalancer/app/") {
			return false
		}
		rest := strings.TrimPrefix(resource, "loadbalancer/app/")
		segs := strings.Split(rest, "/")
		return len(segs) == 2 && segs[0] != "" && segs[1] != ""
	default:
		return false
	}
}

// AssociateWAFWebACL associates a Web ACL ARN with a lab resource ARN string.
// ResourceArn must match IsWAFAssociableResourceARN (fail closed on unknown).
// WebACLArn must exist in-account (fail closed on phantom ARNs).
func (s *Store) AssociateWAFWebACL(accountID, webACLARN, resourceARN string) error {
	webACLARN = strings.TrimSpace(webACLARN)
	resourceARN = strings.TrimSpace(resourceARN)
	if webACLARN == "" || resourceARN == "" {
		return fmt.Errorf("%w: WebACLArn and ResourceArn required", ErrWAFBadRequest)
	}
	if !IsWAFAssociableResourceARN(resourceARN) {
		return fmt.Errorf("%w: ResourceArn is not a supported association target", ErrWAFBadRequest)
	}
	var exists int
	err := s.db.QueryRow(
		`SELECT 1 FROM wafv2_web_acls WHERE account_id = ? AND arn = ?`,
		accountID, webACLARN,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrWAFNotFound
	}
	if err != nil {
		return fmt.Errorf("associate web acl: resolve acl: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO wafv2_associations (account_id, web_acl_arn, resource_arn) VALUES (?, ?, ?)
		 ON CONFLICT(account_id, resource_arn) DO UPDATE SET web_acl_arn = excluded.web_acl_arn`,
		accountID, webACLARN, resourceARN,
	)
	if err != nil {
		return fmt.Errorf("associate web acl: %w", err)
	}
	return nil
}

// LookupWAFAssociation returns the Web ACL ARN associated with resourceARN, if any.
func (s *Store) LookupWAFAssociation(accountID, resourceARN string) (webACLARN string, ok bool, err error) {
	resourceARN = strings.TrimSpace(resourceARN)
	if accountID == "" || resourceARN == "" {
		return "", false, nil
	}
	err = s.db.QueryRow(
		`SELECT web_acl_arn FROM wafv2_associations WHERE account_id = ? AND resource_arn = ?`,
		accountID, resourceARN,
	).Scan(&webACLARN)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("lookup waf association: %w", err)
	}
	return webACLARN, true, nil
}

// EvaluateAssociatedWAF checks candidate resource ARNs for an association and evaluates
// the Web ACL (empty request label uses DefaultAction). Returns associated=false when
// no association matches.
func (s *Store) EvaluateAssociatedWAF(accountID string, candidateARNs []string, requestLabel string) (action string, associated bool, err error) {
	return s.evaluateAssociatedWAF(accountID, candidateARNs, requestLabel, nil)
}

// EvaluateAssociatedWAFWithView is EvaluateAssociatedWAF with optional ByteMatch fields.
func (s *Store) EvaluateAssociatedWAFWithView(accountID string, candidateARNs []string, requestLabel string, view *WAFRequestView) (action string, associated bool, err error) {
	return s.evaluateAssociatedWAF(accountID, candidateARNs, requestLabel, view)
}

func (s *Store) evaluateAssociatedWAF(accountID string, candidateARNs []string, requestLabel string, view *WAFRequestView) (action string, associated bool, err error) {
	seen := map[string]struct{}{}
	for _, arn := range candidateARNs {
		arn = strings.TrimSpace(arn)
		if arn == "" {
			continue
		}
		if _, dup := seen[arn]; dup {
			continue
		}
		seen[arn] = struct{}{}
		webACLARN, ok, lookupErr := s.LookupWAFAssociation(accountID, arn)
		if lookupErr != nil {
			return "", false, lookupErr
		}
		if !ok {
			continue
		}
		action, evalErr := s.EvaluateWAFRequestWithView(accountID, webACLARN, requestLabel, view)
		if evalErr != nil {
			return "", true, evalErr
		}
		return action, true, nil
	}
	return "", false, nil
}

// EvaluateWAFRequest applies Web ACL rules to a labeled request. Returns Allow or Block.
func (s *Store) EvaluateWAFRequest(accountID, webACLARN, requestLabel string) (string, error) {
	return s.EvaluateWAFRequestWithView(accountID, webACLARN, requestLabel, nil)
}

// EvaluateWAFRequestWithView evaluates rules in priority order (first match wins), else DefaultAction.
func (s *Store) EvaluateWAFRequestWithView(accountID, webACLARN, requestLabel string, view *WAFRequestView) (string, error) {
	var rulesJSON, defaultAction string
	err := s.db.QueryRow(
		`SELECT rules_json, default_action FROM wafv2_web_acls WHERE account_id = ? AND arn = ?`,
		accountID, webACLARN,
	).Scan(&rulesJSON, &defaultAction)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrWAFNotFound
	}
	if err != nil {
		return "", fmt.Errorf("evaluate waf: %w", err)
	}
	var rules []WAFRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return "", fmt.Errorf("evaluate waf: parse rules: %w", err)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		return rules[i].Name < rules[j].Name
	})
	for _, rule := range rules {
		matched, matchErr := s.wafRuleMatches(accountID, rule, requestLabel, view)
		if matchErr != nil {
			return "", matchErr
		}
		if !matched {
			continue
		}
		if strings.EqualFold(rule.Action, "Block") {
			return "Block", nil
		}
		return "Allow", nil
	}
	if strings.EqualFold(defaultAction, "Block") {
		return "Block", nil
	}
	return "Allow", nil
}

func (s *Store) wafRuleMatches(accountID string, rule WAFRule, requestLabel string, view *WAFRequestView) (bool, error) {
	if rule.ByteMatchStatement != nil {
		return evaluateWAFByteMatch(rule.ByteMatchStatement, view)
	}
	if rule.SizeConstraintStatement != nil {
		return evaluateWAFSizeConstraint(rule.SizeConstraintStatement, view)
	}
	if rule.IPSetReferenceStatement != nil {
		return s.evaluateWAFIPSet(accountID, rule.IPSetReferenceStatement, view)
	}
	if rule.Label != "" {
		return rule.Label == requestLabel, nil
	}
	return false, nil
}

func evaluateWAFByteMatch(bm *WAFByteMatchStatement, view *WAFRequestView) (bool, error) {
	if bm == nil {
		return false, fmt.Errorf("evaluate waf bytematch: statement is nil")
	}
	if view == nil {
		return false, nil
	}
	search := strings.TrimSpace(bm.SearchString)
	if search == "" {
		return false, fmt.Errorf("evaluate waf bytematch: SearchString required")
	}
	if decoded, err := base64.StdEncoding.DecodeString(search); err == nil && len(decoded) > 0 {
		search = string(decoded)
	}
	constraint := strings.ToUpper(strings.TrimSpace(bm.PositionalConstraint))
	if constraint != "CONTAINS" && constraint != "EXACTLY" {
		return false, fmt.Errorf("evaluate waf bytematch: unsupported PositionalConstraint %q", bm.PositionalConstraint)
	}
	haystack, err := wafFieldToMatchValue(bm.FieldToMatchType, bm.HeaderName, view)
	if err != nil {
		return false, fmt.Errorf("evaluate waf bytematch: %w", err)
	}
	switch constraint {
	case "CONTAINS":
		return strings.Contains(haystack, search), nil
	case "EXACTLY":
		return haystack == search, nil
	default:
		return false, nil
	}
}

func evaluateWAFSizeConstraint(sc *WAFSizeConstraintStatement, view *WAFRequestView) (bool, error) {
	if sc == nil {
		return false, fmt.Errorf("evaluate waf sizeconstraint: statement is nil")
	}
	if view == nil {
		return false, nil
	}
	op := strings.ToUpper(strings.TrimSpace(sc.ComparisonOperator))
	switch op {
	case "EQ", "NE", "LE", "LT", "GE", "GT":
	default:
		return false, fmt.Errorf("evaluate waf sizeconstraint: unsupported ComparisonOperator %q", sc.ComparisonOperator)
	}
	if sc.Size < 0 {
		return false, fmt.Errorf("evaluate waf sizeconstraint: Size must be >= 0")
	}
	value, err := wafFieldToMatchValue(sc.FieldToMatchType, sc.HeaderName, view)
	if err != nil {
		return false, fmt.Errorf("evaluate waf sizeconstraint: %w", err)
	}
	n := int64(len(value))
	switch op {
	case "EQ":
		return n == sc.Size, nil
	case "NE":
		return n != sc.Size, nil
	case "LE":
		return n <= sc.Size, nil
	case "LT":
		return n < sc.Size, nil
	case "GE":
		return n >= sc.Size, nil
	case "GT":
		return n > sc.Size, nil
	default:
		return false, nil
	}
}

func (s *Store) evaluateWAFIPSet(accountID string, ipset *WAFIPSetReferenceStatement, view *WAFRequestView) (bool, error) {
	if ipset == nil {
		return false, fmt.Errorf("evaluate waf ipset: statement is nil")
	}
	if view == nil {
		return false, nil
	}
	src := strings.TrimSpace(view.SourceIP)
	if src == "" {
		return false, nil
	}
	ip := net.ParseIP(src)
	if ip == nil {
		return false, nil
	}
	addresses, err := s.resolveWAFIPSetAddresses(accountID, ipset)
	if err != nil {
		return false, err
	}
	if len(addresses) == 0 {
		return false, nil
	}
	for _, addr := range addresses {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		network, err := wafParseCIDR(addr)
		if err != nil {
			return false, fmt.Errorf("evaluate waf ipset: %w", err)
		}
		if network.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
}

// resolveWAFIPSetAddresses returns CIDRs from ARN (fail closed if unknown) and/or inline Addresses.
func (s *Store) resolveWAFIPSetAddresses(accountID string, ipset *WAFIPSetReferenceStatement) ([]string, error) {
	var addresses []string
	arn := strings.TrimSpace(ipset.ARN)
	if arn != "" {
		resolved, err := s.GetWAFIPSetByARN(accountID, arn)
		if err != nil {
			if errors.Is(err, ErrWAFNotFound) {
				return nil, fmt.Errorf("evaluate waf ipset: unknown IPSet ARN %q", arn)
			}
			return nil, fmt.Errorf("evaluate waf ipset: resolve ARN: %w", err)
		}
		addresses = append(addresses, resolved.Addresses...)
	}
	addresses = append(addresses, normalizeWAFIPSetAddresses(ipset.Addresses)...)
	if arn == "" && len(addresses) == 0 {
		return nil, fmt.Errorf("evaluate waf ipset: ARN or Addresses required")
	}
	return addresses, nil
}

func wafParseCIDR(addr string) (*net.IPNet, error) {
	if strings.Contains(addr, "/") {
		_, network, err := net.ParseCIDR(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", addr, err)
		}
		return network, nil
	}
	ip := net.ParseIP(addr)
	if ip == nil {
		return nil, fmt.Errorf("invalid address %q", addr)
	}
	if ip.To4() != nil {
		_, network, err := net.ParseCIDR(addr + "/32")
		if err != nil {
			return nil, fmt.Errorf("invalid IPv4 host %q: %w", addr, err)
		}
		return network, nil
	}
	_, network, err := net.ParseCIDR(addr + "/128")
	if err != nil {
		return nil, fmt.Errorf("invalid IPv6 host %q: %w", addr, err)
	}
	return network, nil
}

func wafFieldToMatchValue(fieldType, headerName string, view *WAFRequestView) (string, error) {
	switch strings.TrimSpace(fieldType) {
	case "UriPath":
		return view.URI, nil
	case "SingleHeader":
		name := strings.TrimSpace(headerName)
		if name == "" {
			return "", fmt.Errorf("HeaderName required for SingleHeader")
		}
		return wafHeaderValue(view.Headers, name), nil
	default:
		return "", fmt.Errorf("unsupported FieldToMatch %q", fieldType)
	}
}

func wafHeaderValue(headers map[string]string, name string) string {
	if headers == nil {
		return ""
	}
	if v, ok := headers[name]; ok {
		return v
	}
	lower := strings.ToLower(name)
	for k, v := range headers {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return ""
}
