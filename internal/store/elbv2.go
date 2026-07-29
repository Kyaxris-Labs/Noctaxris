package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/google/uuid"
)

var (
	ErrELBv2NotFound       = errors.New("LoadBalancerNotFound")
	ErrELBv2TGNotFound     = errors.New("TargetGroupNotFound")
	ErrELBv2ListenerNotFound = errors.New("ListenerNotFound")
	ErrELBv2RuleNotFound   = errors.New("RuleNotFound")
	ErrELBv2PriorityInUse  = errors.New("PriorityInUse")
	ErrELBv2BadRequest     = errors.New("ValidationError")
)

const DefaultELBv2Region = "us-east-1"

const elbv2Schema = `
CREATE TABLE IF NOT EXISTS elbv2_load_balancers (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  dns_name TEXT NOT NULL,
  type TEXT NOT NULL,
  scheme TEXT NOT NULL DEFAULT 'internet-facing',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_elbv2_lb_arn ON elbv2_load_balancers(account_id, arn);
CREATE TABLE IF NOT EXISTS elbv2_target_groups (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  target_type TEXT NOT NULL,
  protocol TEXT NOT NULL DEFAULT 'HTTP',
  port INTEGER NOT NULL DEFAULT 80,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_elbv2_tg_arn ON elbv2_target_groups(account_id, arn);
CREATE TABLE IF NOT EXISTS elbv2_listeners (
  account_id TEXT NOT NULL,
  listener_arn TEXT NOT NULL,
  load_balancer_arn TEXT NOT NULL,
  port INTEGER NOT NULL,
  protocol TEXT NOT NULL,
  target_group_arn TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, listener_arn)
);
CREATE TABLE IF NOT EXISTS elbv2_targets (
  account_id TEXT NOT NULL,
  target_group_arn TEXT NOT NULL,
  target_id TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, target_group_arn, target_id)
);
CREATE TABLE IF NOT EXISTS elbv2_rules (
  account_id TEXT NOT NULL,
  rule_arn TEXT NOT NULL,
  listener_arn TEXT NOT NULL,
  priority INTEGER NOT NULL,
  path_patterns TEXT NOT NULL,
  host_headers TEXT NOT NULL DEFAULT '[]',
  http_headers TEXT NOT NULL DEFAULT '[]',
  query_strings TEXT NOT NULL DEFAULT '[]',
  source_ips TEXT NOT NULL DEFAULT '[]',
  target_group_arn TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, rule_arn)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_elbv2_rule_priority
  ON elbv2_rules(account_id, listener_arn, priority);
`

// ELBv2LoadBalancer is a lite load balancer row.
type ELBv2LoadBalancer struct {
	Name      string
	ARN       string
	DNSName   string
	Type      string
	Scheme    string
	CreatedAt int64
}

// ELBv2TargetGroup is a lite target group row.
type ELBv2TargetGroup struct {
	Name       string
	ARN        string
	TargetType string // lambda | ip | instance
	Protocol   string
	Port       int
	CreatedAt  int64
}

// ELBv2Listener is a lite listener row.
type ELBv2Listener struct {
	ListenerARN     string
	LoadBalancerARN string
	Port            int
	Protocol        string
	TargetGroupARN  string
	CreatedAt       int64
}

// ELBv2Target is a registered target (Lambda ARN or lab IP).
type ELBv2Target struct {
	ID   string
	Port int
}

// ELBv2HTTPHeaderCond is one http-header condition (name + OR'd values).
type ELBv2HTTPHeaderCond struct {
	Name   string   `json:"Name"`
	Values []string `json:"Values"`
}

// ELBv2QueryStringCond is one query-string match evaluation (key optional).
type ELBv2QueryStringCond struct {
	Key   string `json:"Key,omitempty"`
	Value string `json:"Value"`
}

// ELBv2RuleConditions holds CreateRule/ModifyRule condition fields.
type ELBv2RuleConditions struct {
	PathPatterns []string
	HostHeaders  []string
	HTTPHeaders  []ELBv2HTTPHeaderCond
	QueryStrings []ELBv2QueryStringCond
	SourceIPs    []string
}

// ELBv2RuleMatchInput is the lab request shape used for /alb/ rule matching.
type ELBv2RuleMatchInput struct {
	Path     string
	Host     string
	Headers  map[string]string // lower-case header name -> first value
	Query    map[string]string // lower-case query key -> first value
	SourceIP string
}

// ELBv2Rule is a listener rule with ALB condition fields (non-default).
type ELBv2Rule struct {
	RuleARN        string
	ListenerARN    string
	Priority       int
	PathPatterns   []string
	HostHeaders    []string
	HTTPHeaders    []ELBv2HTTPHeaderCond
	QueryStrings   []ELBv2QueryStringCond
	SourceIPs      []string
	TargetGroupARN string
	CreatedAt      int64
}

// ELBv2TargetHealthDesc is one DescribeTargetHealth row.
type ELBv2TargetHealthDesc struct {
	Target      ELBv2Target
	State       string
	Reason      string
	Description string
}

// EnsureELBv2Schema creates ELBv2 tables if missing and migrates columns.
func EnsureELBv2Schema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure elbv2 schema: db is nil")
	}
	if _, err := db.Exec(elbv2Schema); err != nil {
		return fmt.Errorf("ensure elbv2 schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE elbv2_rules ADD COLUMN host_headers TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE elbv2_rules ADD COLUMN http_headers TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE elbv2_rules ADD COLUMN query_strings TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE elbv2_rules ADD COLUMN source_ips TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE elbv2_load_balancers ADD COLUMN access_logs_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE elbv2_load_balancers ADD COLUMN access_logs_s3_bucket TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE elbv2_load_balancers ADD COLUMN access_logs_s3_prefix TEXT NOT NULL DEFAULT ''`,
	}); err != nil {
		return fmt.Errorf("ensure elbv2 schema: migrate: %w", err)
	}
	return nil
}

// EnsureELBv2Schema ensures ELBv2 tables on an open store.
func (s *Store) EnsureELBv2Schema() error {
	return EnsureELBv2Schema(s.db)
}

func elbv2LBARN(region, accountID, name, id, lbType string) string {
	if region == "" {
		region = DefaultELBv2Region
	}
	prefix := "app"
	if lbType == "network" {
		prefix = "net"
	}
	return fmt.Sprintf("arn:aws:elasticloadbalancing:%s:%s:loadbalancer/%s/%s/%s", region, accountID, prefix, name, id)
}

func elbv2TGARN(region, accountID, name, id string) string {
	if region == "" {
		region = DefaultELBv2Region
	}
	return fmt.Sprintf("arn:aws:elasticloadbalancing:%s:%s:targetgroup/%s/%s", region, accountID, name, id)
}

func normalizeELBv2LoadBalancerType(lbType string) (string, error) {
	lbType = strings.ToLower(strings.TrimSpace(lbType))
	if lbType == "" {
		return "application", nil
	}
	switch lbType {
	case "application", "network":
		return lbType, nil
	default:
		return "", fmt.Errorf("%w: Type must be application or network", ErrELBv2BadRequest)
	}
}

// CreateELBv2LoadBalancer creates an application or network load balancer lite.
func (s *Store) CreateELBv2LoadBalancer(accountID, region, name, scheme, lbType string) (ELBv2LoadBalancer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ELBv2LoadBalancer{}, fmt.Errorf("%w: Name required", ErrELBv2BadRequest)
	}
	if scheme == "" {
		scheme = "internet-facing"
	}
	normalizedType, err := normalizeELBv2LoadBalancerType(lbType)
	if err != nil {
		return ELBv2LoadBalancer{}, err
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	arn := elbv2LBARN(region, accountID, name, id, normalizedType)
	dns := fmt.Sprintf("%s-%s.%s.elb.lab.local", name, id[:8], regionOrELB(region))
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO elbv2_load_balancers (account_id, name, arn, dns_name, type, scheme, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, arn, dns, normalizedType, scheme, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return ELBv2LoadBalancer{}, fmt.Errorf("%w: load balancer name already exists", ErrELBv2BadRequest)
		}
		return ELBv2LoadBalancer{}, fmt.Errorf("create load balancer: %w", err)
	}
	return ELBv2LoadBalancer{Name: name, ARN: arn, DNSName: dns, Type: normalizedType, Scheme: scheme, CreatedAt: now}, nil
}

func regionOrELB(region string) string {
	if region == "" {
		return DefaultELBv2Region
	}
	return region
}

// DescribeELBv2LoadBalancers lists load balancers, optionally filtered by ARN.
func (s *Store) DescribeELBv2LoadBalancers(accountID string, arns []string) ([]ELBv2LoadBalancer, error) {
	rows, err := s.db.Query(
		`SELECT name, arn, dns_name, type, scheme, created_at FROM elbv2_load_balancers
		 WHERE account_id = ? ORDER BY name`, accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("describe load balancers: %w", err)
	}
	defer rows.Close()
	want := map[string]struct{}{}
	for _, a := range arns {
		a = strings.TrimSpace(a)
		if a != "" {
			want[a] = struct{}{}
		}
	}
	var out []ELBv2LoadBalancer
	for rows.Next() {
		var lb ELBv2LoadBalancer
		if err := rows.Scan(&lb.Name, &lb.ARN, &lb.DNSName, &lb.Type, &lb.Scheme, &lb.CreatedAt); err != nil {
			return nil, fmt.Errorf("describe load balancers scan: %w", err)
		}
		if len(want) > 0 {
			if _, ok := want[lb.ARN]; !ok {
				continue
			}
		}
		out = append(out, lb)
	}
	return out, rows.Err()
}

// DeleteELBv2LoadBalancer deletes a load balancer by ARN.
func (s *Store) DeleteELBv2LoadBalancer(accountID, arn string) error {
	arn = strings.TrimSpace(arn)
	res, err := s.db.Exec(`DELETE FROM elbv2_load_balancers WHERE account_id = ? AND arn = ?`, accountID, arn)
	if err != nil {
		return fmt.Errorf("delete load balancer: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrELBv2NotFound
	}
	ls, _ := s.DescribeELBv2Listeners(accountID, arn)
	for _, l := range ls {
		_, _ = s.db.Exec(`DELETE FROM elbv2_rules WHERE account_id = ? AND listener_arn = ?`, accountID, l.ListenerARN)
	}
	_, _ = s.db.Exec(`DELETE FROM elbv2_listeners WHERE account_id = ? AND load_balancer_arn = ?`, accountID, arn)
	return nil
}

// CreateELBv2TargetGroup creates a target group. TargetType must be lambda, ip, or instance.
func (s *Store) CreateELBv2TargetGroup(accountID, region, name, targetType, protocol string, port int) (ELBv2TargetGroup, error) {
	name = strings.TrimSpace(name)
	targetType = strings.ToLower(strings.TrimSpace(targetType))
	if name == "" {
		return ELBv2TargetGroup{}, fmt.Errorf("%w: Name required", ErrELBv2BadRequest)
	}
	switch targetType {
	case "lambda", "ip", "instance":
	default:
		return ELBv2TargetGroup{}, fmt.Errorf("%w: TargetType must be lambda, ip, or instance", ErrELBv2BadRequest)
	}
	protocol = strings.ToUpper(strings.TrimSpace(protocol))
	if protocol == "" {
		if targetType == "lambda" {
			protocol = "HTTP"
		} else {
			protocol = "TCP"
		}
	}
	switch targetType {
	case "lambda":
		if protocol != "HTTP" && protocol != "HTTPS" {
			return ELBv2TargetGroup{}, fmt.Errorf("%w: lambda target groups support HTTP or HTTPS only", ErrELBv2BadRequest)
		}
	case "ip", "instance":
		switch protocol {
		case "TCP", "TLS", "HTTP", "HTTPS":
		default:
			return ELBv2TargetGroup{}, fmt.Errorf("%w: ip/instance target groups support TCP, TLS, HTTP, or HTTPS", ErrELBv2BadRequest)
		}
	}
	if port <= 0 {
		if protocol == "HTTPS" || protocol == "TLS" {
			port = 443
		} else {
			port = 80
		}
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	arn := elbv2TGARN(region, accountID, name, id)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO elbv2_target_groups (account_id, name, arn, target_type, protocol, port, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, arn, targetType, protocol, port, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return ELBv2TargetGroup{}, fmt.Errorf("%w: target group name already exists", ErrELBv2BadRequest)
		}
		return ELBv2TargetGroup{}, fmt.Errorf("create target group: %w", err)
	}
	return ELBv2TargetGroup{Name: name, ARN: arn, TargetType: targetType, Protocol: protocol, Port: port, CreatedAt: now}, nil
}

// DescribeELBv2TargetGroups lists target groups.
func (s *Store) DescribeELBv2TargetGroups(accountID string, arns []string) ([]ELBv2TargetGroup, error) {
	rows, err := s.db.Query(
		`SELECT name, arn, target_type, protocol, port, created_at FROM elbv2_target_groups
		 WHERE account_id = ? ORDER BY name`, accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("describe target groups: %w", err)
	}
	defer rows.Close()
	want := map[string]struct{}{}
	for _, a := range arns {
		a = strings.TrimSpace(a)
		if a != "" {
			want[a] = struct{}{}
		}
	}
	var out []ELBv2TargetGroup
	for rows.Next() {
		var tg ELBv2TargetGroup
		if err := rows.Scan(&tg.Name, &tg.ARN, &tg.TargetType, &tg.Protocol, &tg.Port, &tg.CreatedAt); err != nil {
			return nil, fmt.Errorf("describe target groups scan: %w", err)
		}
		if len(want) > 0 {
			if _, ok := want[tg.ARN]; !ok {
				continue
			}
		}
		out = append(out, tg)
	}
	return out, rows.Err()
}

// DeleteELBv2TargetGroup deletes a target group by ARN.
func (s *Store) DeleteELBv2TargetGroup(accountID, arn string) error {
	arn = strings.TrimSpace(arn)
	res, err := s.db.Exec(`DELETE FROM elbv2_target_groups WHERE account_id = ? AND arn = ?`, accountID, arn)
	if err != nil {
		return fmt.Errorf("delete target group: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrELBv2TGNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM elbv2_targets WHERE account_id = ? AND target_group_arn = ?`, accountID, arn)
	return nil
}

// CreateELBv2Listener creates a listener forwarding to a target group.
// Application LBs accept HTTP/HTTPS; network LBs accept TCP/TLS (HTTP rejected).
func (s *Store) CreateELBv2Listener(accountID, region, loadBalancerARN, targetGroupARN, protocol string, port int) (ELBv2Listener, error) {
	loadBalancerARN = strings.TrimSpace(loadBalancerARN)
	targetGroupARN = strings.TrimSpace(targetGroupARN)
	if loadBalancerARN == "" || targetGroupARN == "" {
		return ELBv2Listener{}, fmt.Errorf("%w: LoadBalancerArn and TargetGroupArn required", ErrELBv2BadRequest)
	}
	lbs, err := s.DescribeELBv2LoadBalancers(accountID, []string{loadBalancerARN})
	if err != nil {
		return ELBv2Listener{}, err
	}
	if len(lbs) == 0 {
		return ELBv2Listener{}, ErrELBv2NotFound
	}
	lb := lbs[0]
	tgs, err := s.DescribeELBv2TargetGroups(accountID, []string{targetGroupARN})
	if err != nil {
		return ELBv2Listener{}, err
	}
	if len(tgs) == 0 {
		return ELBv2Listener{}, ErrELBv2TGNotFound
	}
	tg := tgs[0]
	protocol = strings.ToUpper(strings.TrimSpace(protocol))
	switch lb.Type {
	case "network":
		if protocol == "" {
			protocol = "TCP"
		}
		if protocol != "TCP" && protocol != "TLS" {
			return ELBv2Listener{}, fmt.Errorf("%w: network load balancers support TCP or TLS listeners only", ErrELBv2BadRequest)
		}
		if tg.TargetType != "ip" && tg.TargetType != "instance" {
			return ELBv2Listener{}, fmt.Errorf("%w: network listeners require ip or instance target groups", ErrELBv2BadRequest)
		}
		if tg.Protocol != "TCP" && tg.Protocol != "TLS" {
			return ELBv2Listener{}, fmt.Errorf("%w: network listeners require TCP or TLS target groups", ErrELBv2BadRequest)
		}
		if port <= 0 {
			if protocol == "TLS" {
				port = 443
			} else {
				port = 80
			}
		}
	default:
		if protocol == "" {
			protocol = "HTTP"
		}
		if protocol != "HTTP" && protocol != "HTTPS" {
			return ELBv2Listener{}, fmt.Errorf("%w: application load balancers support HTTP or HTTPS listeners only", ErrELBv2BadRequest)
		}
		if port <= 0 {
			if protocol == "HTTPS" {
				port = 443
			} else {
				port = 80
			}
		}
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	if region == "" {
		region = DefaultELBv2Region
	}
	arnPrefix := "app"
	if lb.Type == "network" {
		arnPrefix = "net"
	}
	listenerARN := fmt.Sprintf("arn:aws:elasticloadbalancing:%s:%s:listener/%s/%s/%s",
		region, accountID, arnPrefix, lb.Name, id)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO elbv2_listeners (account_id, listener_arn, load_balancer_arn, port, protocol, target_group_arn, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, listenerARN, loadBalancerARN, port, protocol, targetGroupARN, now,
	)
	if err != nil {
		return ELBv2Listener{}, fmt.Errorf("create listener: %w", err)
	}
	return ELBv2Listener{
		ListenerARN: listenerARN, LoadBalancerARN: loadBalancerARN,
		Port: port, Protocol: protocol, TargetGroupARN: targetGroupARN, CreatedAt: now,
	}, nil
}

// DescribeELBv2Listeners lists listeners for a load balancer.
func (s *Store) DescribeELBv2Listeners(accountID, loadBalancerARN string) ([]ELBv2Listener, error) {
	rows, err := s.db.Query(
		`SELECT listener_arn, load_balancer_arn, port, protocol, target_group_arn, created_at
		 FROM elbv2_listeners WHERE account_id = ? AND load_balancer_arn = ? ORDER BY port`,
		accountID, strings.TrimSpace(loadBalancerARN),
	)
	if err != nil {
		return nil, fmt.Errorf("describe listeners: %w", err)
	}
	defer rows.Close()
	var out []ELBv2Listener
	for rows.Next() {
		var l ELBv2Listener
		if err := rows.Scan(&l.ListenerARN, &l.LoadBalancerARN, &l.Port, &l.Protocol, &l.TargetGroupARN, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("describe listeners scan: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DeleteELBv2Listener deletes a listener by ARN.
func (s *Store) DeleteELBv2Listener(accountID, listenerARN string) error {
	listenerARN = strings.TrimSpace(listenerARN)
	res, err := s.db.Exec(`DELETE FROM elbv2_listeners WHERE account_id = ? AND listener_arn = ?`,
		accountID, listenerARN)
	if err != nil {
		return fmt.Errorf("delete listener: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrELBv2NotFound
	}
	_, _ = s.db.Exec(`DELETE FROM elbv2_rules WHERE account_id = ? AND listener_arn = ?`, accountID, listenerARN)
	return nil
}

// GetELBv2LoadBalancerByName returns a load balancer by account and name.
func (s *Store) GetELBv2LoadBalancerByName(accountID, name string) (ELBv2LoadBalancer, error) {
	name = strings.TrimSpace(name)
	var lb ELBv2LoadBalancer
	err := s.db.QueryRow(
		`SELECT name, arn, dns_name, type, scheme, created_at FROM elbv2_load_balancers
		 WHERE account_id = ? AND name = ?`, accountID, name,
	).Scan(&lb.Name, &lb.ARN, &lb.DNSName, &lb.Type, &lb.Scheme, &lb.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ELBv2LoadBalancer{}, ErrELBv2NotFound
	}
	if err != nil {
		return ELBv2LoadBalancer{}, fmt.Errorf("get load balancer by name: %w", err)
	}
	return lb, nil
}

// GetELBv2ListenerByPort returns the listener on a load balancer for a port.
func (s *Store) GetELBv2ListenerByPort(accountID, loadBalancerARN string, port int) (ELBv2Listener, error) {
	var l ELBv2Listener
	err := s.db.QueryRow(
		`SELECT listener_arn, load_balancer_arn, port, protocol, target_group_arn, created_at
		 FROM elbv2_listeners WHERE account_id = ? AND load_balancer_arn = ? AND port = ?`,
		accountID, strings.TrimSpace(loadBalancerARN), port,
	).Scan(&l.ListenerARN, &l.LoadBalancerARN, &l.Port, &l.Protocol, &l.TargetGroupARN, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ELBv2Listener{}, ErrELBv2NotFound
	}
	if err != nil {
		return ELBv2Listener{}, fmt.Errorf("get listener by port: %w", err)
	}
	return l, nil
}

// ELBv2TargetGroupHasListener reports whether any listener or rule forwards to the target group.
func (s *Store) ELBv2TargetGroupHasListener(accountID, targetGroupARN string) (bool, error) {
	tgARN := strings.TrimSpace(targetGroupARN)
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM elbv2_listeners WHERE account_id = ? AND target_group_arn = ?`,
		accountID, tgARN,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("target group has listener: %w", err)
	}
	if n > 0 {
		return true, nil
	}
	err = s.db.QueryRow(
		`SELECT COUNT(1) FROM elbv2_rules WHERE account_id = ? AND target_group_arn = ?`,
		accountID, tgARN,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("target group has rule: %w", err)
	}
	return n > 0, nil
}

// DescribeELBv2TargetHealth returns health reflecting lab-listener usefulness.
// Lambda targets are healthy when a listener forwards to the group, the function
// still resolves, and elasticloadbalancing.amazonaws.com is Allowed. Otherwise
// unused (no listener) or unhealthy (stale registration). IP and instance targets
// are healthy when a listener or rule forwards (control-plane registration only;
// no L4 dataplane).
func (s *Store) DescribeELBv2TargetHealth(accountID, targetGroupARN string) ([]ELBv2TargetHealthDesc, error) {
	targetGroupARN = strings.TrimSpace(targetGroupARN)
	tgs, err := s.DescribeELBv2TargetGroups(accountID, []string{targetGroupARN})
	if err != nil {
		return nil, err
	}
	if len(tgs) == 0 {
		return nil, ErrELBv2TGNotFound
	}
	tg := tgs[0]
	targets, err := s.ListELBv2Targets(accountID, targetGroupARN)
	if err != nil {
		return nil, err
	}
	inUse, err := s.ELBv2TargetGroupHasListener(accountID, targetGroupARN)
	if err != nil {
		return nil, err
	}
	out := make([]ELBv2TargetHealthDesc, 0, len(targets))
	for _, t := range targets {
		desc := ELBv2TargetHealthDesc{Target: t}
		switch tg.TargetType {
		case "lambda":
			if !inUse {
				desc.State = "unused"
				desc.Reason = "Target.NotInUse"
				desc.Description = "No listener forwards to this target group."
				out = append(out, desc)
				continue
			}
			fnAccount, fnName, ok := ParseLambdaARNFromSFNResource(t.ID)
			if !ok || fnName == "" {
				desc.State = "unhealthy"
				desc.Reason = "Target.InvalidState"
				desc.Description = "Lambda target Id is not a function ARN."
				out = append(out, desc)
				continue
			}
			fn, err := s.GetFunction(fnAccount, fnName)
			if errors.Is(err, ErrNoSuchFunction) {
				desc.State = "unhealthy"
				desc.Reason = "Target.FailedHealthChecks"
				desc.Description = "Lambda function not found."
				out = append(out, desc)
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("describe target health: resolve function: %w", err)
			}
			if !s.deliveryTargetResourcePolicyAllows(
				fnAccount, fn.FunctionARN, actionLambdaInvokeFunction, authz.ServicePrincipalELB, targetGroupARN,
			) {
				desc.State = "unhealthy"
				desc.Reason = "Target.FailedHealthChecks"
				desc.Description = "Lambda resource policy does not Allow elasticloadbalancing.amazonaws.com."
				out = append(out, desc)
				continue
			}
			desc.State = "healthy"
			desc.Description = "Lab listener can invoke this Lambda target."
			out = append(out, desc)
		case "ip", "instance":
			if !inUse {
				desc.State = "unused"
				desc.Reason = "Target.NotInUse"
				desc.Description = "No listener forwards to this target group."
				out = append(out, desc)
				continue
			}
			desc.State = "healthy"
			desc.Description = "Lab control-plane registration only; no L4 dataplane."
			out = append(out, desc)
		default:
			desc.State = "unused"
			desc.Reason = "Target.NotInUse"
			desc.Description = "Unsupported target type."
			out = append(out, desc)
		}
	}
	return out, nil
}

// RegisterELBv2Targets registers Lambda ARN, lab IP, or lab instance-id targets.
// Lambda targets must resolve to an existing function and Allow
// elasticloadbalancing.amazonaws.com on the function resource policy
// (SourceArn may be the target group ARN). DescribeTargetHealth reports
// healthy when a lab listener forwards to the group and permission still Allows.
// Instance Ids are lab-opaque i-* labels; /nlb/ dataplane resolves private IP from EC2 when present.
func (s *Store) RegisterELBv2Targets(accountID, targetGroupARN string, targets []ELBv2Target) error {
	targetGroupARN = strings.TrimSpace(targetGroupARN)
	tgs, err := s.DescribeELBv2TargetGroups(accountID, []string{targetGroupARN})
	if err != nil {
		return err
	}
	if len(tgs) == 0 {
		return ErrELBv2TGNotFound
	}
	tg := tgs[0]
	for _, t := range targets {
		id := strings.TrimSpace(t.ID)
		if id == "" {
			return fmt.Errorf("%w: Target Id required", ErrELBv2BadRequest)
		}
		switch tg.TargetType {
		case "lambda":
			if !strings.HasPrefix(id, "arn:aws:lambda:") {
				return fmt.Errorf("%w: lambda target Id must be a Lambda function ARN", ErrELBv2BadRequest)
			}
			fnAccount, fnName, ok := ParseLambdaARNFromSFNResource(id)
			if !ok || fnName == "" {
				return fmt.Errorf("%w: lambda target Id must be a Lambda function ARN", ErrELBv2BadRequest)
			}
			fn, err := s.GetFunction(fnAccount, fnName)
			if errors.Is(err, ErrNoSuchFunction) {
				return fmt.Errorf("%w: Lambda function not found for target", ErrELBv2BadRequest)
			}
			if err != nil {
				return fmt.Errorf("register targets: resolve function: %w", err)
			}
			if !s.deliveryTargetResourcePolicyAllows(
				fnAccount, fn.FunctionARN, actionLambdaInvokeFunction, authz.ServicePrincipalELB, targetGroupARN,
			) {
				return fmt.Errorf("%w: Lambda resource policy must Allow elasticloadbalancing.amazonaws.com to invoke the function", ErrELBv2BadRequest)
			}
		case "ip":
			ip := id
			if host, _, err := net.SplitHostPort(id); err == nil {
				ip = host
			}
			parsed := net.ParseIP(ip)
			if parsed == nil {
				return fmt.Errorf("%w: ip target Id must be a lab IP address", ErrELBv2BadRequest)
			}
			if elbv2LabForwardIPDenied(parsed) {
				return fmt.Errorf("%w: ip target must not be unspecified or link-local", ErrELBv2BadRequest)
			}
		case "instance":
			if !strings.HasPrefix(id, "i-") || len(id) < 3 {
				return fmt.Errorf("%w: instance target Id must be a lab instance id (i-...)", ErrELBv2BadRequest)
			}
		default:
			return fmt.Errorf("%w: unsupported target type", ErrELBv2BadRequest)
		}
		_, err := s.db.Exec(
			`INSERT INTO elbv2_targets (account_id, target_group_arn, target_id, port) VALUES (?, ?, ?, ?)
			 ON CONFLICT(account_id, target_group_arn, target_id) DO UPDATE SET port = excluded.port`,
			accountID, targetGroupARN, id, t.Port,
		)
		if err != nil {
			return fmt.Errorf("register targets: %w", err)
		}
	}
	return nil
}

// ListELBv2Targets lists registered targets for a target group.
func (s *Store) ListELBv2Targets(accountID, targetGroupARN string) ([]ELBv2Target, error) {
	rows, err := s.db.Query(
		`SELECT target_id, port FROM elbv2_targets WHERE account_id = ? AND target_group_arn = ? ORDER BY target_id`,
		accountID, strings.TrimSpace(targetGroupARN),
	)
	if err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}
	defer rows.Close()
	var out []ELBv2Target
	for rows.Next() {
		var t ELBv2Target
		if err := rows.Scan(&t.ID, &t.Port); err != nil {
			return nil, fmt.Errorf("list targets scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MatchELBv2PathPattern reports whether routePath matches a lite path-pattern.
// Exact `/foo` requires equality. Prefix `/foo*` (single trailing `*` only) uses HasPrefix.
func MatchELBv2PathPattern(routePath, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	routePath = strings.TrimSpace(routePath)
	if pattern == "" {
		return false
	}
	if strings.Count(pattern, "*") > 1 {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(routePath, strings.TrimSuffix(pattern, "*"))
	}
	if strings.Contains(pattern, "*") {
		return false
	}
	return routePath == pattern
}

// MatchELBv2HostHeader reports whether requestHost matches a lite host-header value.
// Matching is case-insensitive. Exact `foo.example.com` requires equality after stripping
// an optional `:port`. Prefix `*.example.com` is not supported; only a single trailing `*`
// (e.g. `api.*`) uses HasPrefix on the host (without port).
func MatchELBv2HostHeader(requestHost, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	requestHost = strings.TrimSpace(requestHost)
	if pattern == "" {
		return false
	}
	host := requestHost
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	pattern = strings.ToLower(pattern)
	if strings.Count(pattern, "*") > 1 {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(host, strings.TrimSuffix(pattern, "*"))
	}
	if strings.Contains(pattern, "*") {
		return false
	}
	return host == pattern
}

func cleanELBv2StringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func validateELBv2PathPatterns(patterns []string) error {
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			return fmt.Errorf("%w: path-pattern value must be non-empty", ErrELBv2BadRequest)
		}
		if strings.Count(p, "*") > 1 || (strings.Contains(p, "*") && !strings.HasSuffix(p, "*")) {
			return fmt.Errorf("%w: path-pattern supports exact /foo or prefix /foo* (single trailing * only)", ErrELBv2BadRequest)
		}
	}
	return nil
}

func validateELBv2HostHeaders(hosts []string) error {
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			return fmt.Errorf("%w: host-header value must be non-empty", ErrELBv2BadRequest)
		}
		if strings.Count(h, "*") > 1 || (strings.Contains(h, "*") && !strings.HasSuffix(h, "*")) {
			return fmt.Errorf("%w: host-header supports exact hostname or prefix host* (single trailing * only)", ErrELBv2BadRequest)
		}
	}
	return nil
}

func validateELBv2LiteWildcards(field string, values []string) error {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			return fmt.Errorf("%w: %s value must be non-empty", ErrELBv2BadRequest, field)
		}
		stars := strings.Count(v, "*")
		switch {
		case stars == 0:
			continue
		case stars == 1 && (strings.HasPrefix(v, "*") || strings.HasSuffix(v, "*")):
			continue
		case stars == 2 && strings.HasPrefix(v, "*") && strings.HasSuffix(v, "*") && len(v) >= 2:
			continue
		default:
			return fmt.Errorf("%w: %s supports exact, prefix x*, suffix *x, or *contains* wildcards only", ErrELBv2BadRequest, field)
		}
	}
	return nil
}

func validateELBv2HTTPHeaders(conds []ELBv2HTTPHeaderCond) error {
	for _, c := range conds {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return fmt.Errorf("%w: http-header HttpHeaderName required", ErrELBv2BadRequest)
		}
		if strings.Contains(name, "*") {
			return fmt.Errorf("%w: http-header name does not support wildcards", ErrELBv2BadRequest)
		}
		cleaned := cleanELBv2StringList(c.Values)
		if len(cleaned) == 0 {
			return fmt.Errorf("%w: http-header Values required", ErrELBv2BadRequest)
		}
		if err := validateELBv2LiteWildcards("http-header", cleaned); err != nil {
			return err
		}
	}
	return nil
}

func validateELBv2QueryStrings(conds []ELBv2QueryStringCond) error {
	for _, c := range conds {
		val := strings.TrimSpace(c.Value)
		if val == "" {
			return fmt.Errorf("%w: query-string Value required", ErrELBv2BadRequest)
		}
		if err := validateELBv2LiteWildcards("query-string", []string{val}); err != nil {
			return err
		}
	}
	return nil
}

func validateELBv2SourceIPs(cidrs []string) error {
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			return fmt.Errorf("%w: source-ip value must be non-empty", ErrELBv2BadRequest)
		}
		if c == "255.255.255.255/32" {
			return fmt.Errorf("%w: source-ip does not support 255.255.255.255/32", ErrELBv2BadRequest)
		}
		if _, _, err := net.ParseCIDR(c); err != nil {
			return fmt.Errorf("%w: source-ip Values must be CIDR blocks", ErrELBv2BadRequest)
		}
	}
	return nil
}

func cleanELBv2RuleConditions(cond ELBv2RuleConditions) (ELBv2RuleConditions, error) {
	out := ELBv2RuleConditions{
		PathPatterns: cleanELBv2StringList(cond.PathPatterns),
		HostHeaders:  cleanELBv2StringList(cond.HostHeaders),
		SourceIPs:    cleanELBv2StringList(cond.SourceIPs),
	}
	for _, h := range cond.HTTPHeaders {
		name := strings.TrimSpace(h.Name)
		vals := cleanELBv2StringList(h.Values)
		if name == "" && len(vals) == 0 {
			continue
		}
		out.HTTPHeaders = append(out.HTTPHeaders, ELBv2HTTPHeaderCond{Name: name, Values: vals})
	}
	for _, q := range cond.QueryStrings {
		key := strings.TrimSpace(q.Key)
		val := strings.TrimSpace(q.Value)
		if key == "" && val == "" {
			continue
		}
		out.QueryStrings = append(out.QueryStrings, ELBv2QueryStringCond{Key: key, Value: val})
	}
	if len(out.PathPatterns) == 0 && len(out.HostHeaders) == 0 &&
		len(out.HTTPHeaders) == 0 && len(out.QueryStrings) == 0 && len(out.SourceIPs) == 0 {
		return ELBv2RuleConditions{}, fmt.Errorf("%w: Conditions require at least one of path-pattern, host-header, http-header, query-string, source-ip", ErrELBv2BadRequest)
	}
	if err := validateELBv2PathPatterns(out.PathPatterns); err != nil {
		return ELBv2RuleConditions{}, err
	}
	if err := validateELBv2HostHeaders(out.HostHeaders); err != nil {
		return ELBv2RuleConditions{}, err
	}
	if err := validateELBv2HTTPHeaders(out.HTTPHeaders); err != nil {
		return ELBv2RuleConditions{}, err
	}
	if err := validateELBv2QueryStrings(out.QueryStrings); err != nil {
		return ELBv2RuleConditions{}, err
	}
	if err := validateELBv2SourceIPs(out.SourceIPs); err != nil {
		return ELBv2RuleConditions{}, err
	}
	return out, nil
}

// MatchELBv2LiteWildcard reports whether value matches a lite wildcard pattern.
// Supports exact, prefix `x*`, suffix `*x`, and `*contains*` (two stars at ends only).
func MatchELBv2LiteWildcard(value, pattern string, caseInsensitive bool) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" {
		return false
	}
	if caseInsensitive {
		pattern = strings.ToLower(pattern)
		value = strings.ToLower(value)
	}
	stars := strings.Count(pattern, "*")
	switch {
	case stars == 0:
		return value == pattern
	case stars == 1 && strings.HasSuffix(pattern, "*"):
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	case stars == 1 && strings.HasPrefix(pattern, "*"):
		return strings.HasSuffix(value, strings.TrimPrefix(pattern, "*"))
	case stars == 2 && strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*"):
		inner := strings.TrimSuffix(strings.TrimPrefix(pattern, "*"), "*")
		return strings.Contains(value, inner)
	default:
		return false
	}
}

// MatchELBv2HTTPHeader reports whether request headers satisfy one http-header condition.
func MatchELBv2HTTPHeader(headers map[string]string, cond ELBv2HTTPHeaderCond) bool {
	name := strings.ToLower(strings.TrimSpace(cond.Name))
	if name == "" || len(cond.Values) == 0 {
		return false
	}
	got := ""
	if headers != nil {
		got = headers[name]
	}
	for _, pat := range cond.Values {
		if MatchELBv2LiteWildcard(got, pat, true) {
			return true
		}
	}
	return false
}

// MatchELBv2QueryString reports whether query params satisfy one query-string evaluation.
func MatchELBv2QueryString(query map[string]string, cond ELBv2QueryStringCond) bool {
	valPat := strings.TrimSpace(cond.Value)
	if valPat == "" {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(cond.Key))
	if key != "" {
		got := ""
		if query != nil {
			got = query[key]
		}
		return MatchELBv2LiteWildcard(got, valPat, true)
	}
	for _, got := range query {
		if MatchELBv2LiteWildcard(got, valPat, true) {
			return true
		}
	}
	return false
}

// MatchELBv2SourceIP reports whether sourceIP is in any of the CIDR Values.
func MatchELBv2SourceIP(sourceIP string, cidrs []string) bool {
	ip := net.ParseIP(strings.TrimSpace(sourceIP))
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(c))
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func ruleHasAnyCondition(rule ELBv2Rule) bool {
	return len(rule.PathPatterns) > 0 || len(rule.HostHeaders) > 0 ||
		len(rule.HTTPHeaders) > 0 || len(rule.QueryStrings) > 0 || len(rule.SourceIPs) > 0
}

// MatchELBv2Rule reports whether a rule matches the lab request.
// Values within path-pattern, host-header, and source-ip are OR'd.
// Each http-header condition is AND'd with others; Values within one are OR'd.
// Query-string evaluations on a rule are OR'd (one QueryStringConfig Values list).
// Distinct field types are AND'd. Absent field types are ignored.
func MatchELBv2Rule(rule ELBv2Rule, in ELBv2RuleMatchInput) bool {
	if !ruleHasAnyCondition(rule) {
		return false
	}
	if len(rule.PathPatterns) > 0 {
		pathOK := false
		for _, pat := range rule.PathPatterns {
			if MatchELBv2PathPattern(in.Path, pat) {
				pathOK = true
				break
			}
		}
		if !pathOK {
			return false
		}
	}
	if len(rule.HostHeaders) > 0 {
		hostOK := false
		for _, pat := range rule.HostHeaders {
			if MatchELBv2HostHeader(in.Host, pat) {
				hostOK = true
				break
			}
		}
		if !hostOK {
			return false
		}
	}
	for _, hc := range rule.HTTPHeaders {
		if !MatchELBv2HTTPHeader(in.Headers, hc) {
			return false
		}
	}
	if len(rule.QueryStrings) > 0 {
		qOK := false
		for _, qc := range rule.QueryStrings {
			if MatchELBv2QueryString(in.Query, qc) {
				qOK = true
				break
			}
		}
		if !qOK {
			return false
		}
	}
	if len(rule.SourceIPs) > 0 && !MatchELBv2SourceIP(in.SourceIP, rule.SourceIPs) {
		return false
	}
	return true
}

// MatchELBv2RulePathHost is a compatibility wrapper for path+host-only matching.
func MatchELBv2RulePathHost(rule ELBv2Rule, routePath, requestHost string) bool {
	return MatchELBv2Rule(rule, ELBv2RuleMatchInput{Path: routePath, Host: requestHost})
}

func getELBv2ListenerByARN(s *Store, accountID, listenerARN string) (ELBv2Listener, error) {
	var l ELBv2Listener
	err := s.db.QueryRow(
		`SELECT listener_arn, load_balancer_arn, port, protocol, target_group_arn, created_at
		 FROM elbv2_listeners WHERE account_id = ? AND listener_arn = ?`,
		accountID, strings.TrimSpace(listenerARN),
	).Scan(&l.ListenerARN, &l.LoadBalancerARN, &l.Port, &l.Protocol, &l.TargetGroupARN, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ELBv2Listener{}, ErrELBv2ListenerNotFound
	}
	if err != nil {
		return ELBv2Listener{}, fmt.Errorf("get listener by arn: %w", err)
	}
	return l, nil
}

// GetELBv2ListenerByARN returns a listener by ARN.
func (s *Store) GetELBv2ListenerByARN(accountID, listenerARN string) (ELBv2Listener, error) {
	return getELBv2ListenerByARN(s, accountID, listenerARN)
}

// CreateELBv2Rule creates a forward rule with ALB conditions (path/host/http-header/query/source-ip).
func (s *Store) CreateELBv2Rule(accountID, region, listenerARN, targetGroupARN string, priority int, cond ELBv2RuleConditions) (ELBv2Rule, error) {
	listenerARN = strings.TrimSpace(listenerARN)
	targetGroupARN = strings.TrimSpace(targetGroupARN)
	if listenerARN == "" {
		return ELBv2Rule{}, fmt.Errorf("%w: ListenerArn required", ErrELBv2BadRequest)
	}
	if targetGroupARN == "" {
		return ELBv2Rule{}, fmt.Errorf("%w: Actions forward TargetGroupArn required", ErrELBv2BadRequest)
	}
	if priority < 1 || priority > 50000 {
		return ELBv2Rule{}, fmt.Errorf("%w: Priority must be between 1 and 50000", ErrELBv2BadRequest)
	}
	cleaned, err := cleanELBv2RuleConditions(cond)
	if err != nil {
		return ELBv2Rule{}, err
	}
	listener, err := getELBv2ListenerByARN(s, accountID, listenerARN)
	if err != nil {
		return ELBv2Rule{}, err
	}
	lbs, err := s.DescribeELBv2LoadBalancers(accountID, []string{listener.LoadBalancerARN})
	if err != nil {
		return ELBv2Rule{}, err
	}
	if len(lbs) == 0 {
		return ELBv2Rule{}, ErrELBv2NotFound
	}
	if lbs[0].Type == "network" {
		return ELBv2Rule{}, fmt.Errorf("%w: CreateRule is not supported on network load balancers", ErrELBv2BadRequest)
	}
	tgs, err := s.DescribeELBv2TargetGroups(accountID, []string{targetGroupARN})
	if err != nil {
		return ELBv2Rule{}, err
	}
	if len(tgs) == 0 {
		return ELBv2Rule{}, ErrELBv2TGNotFound
	}
	var existing int
	if err := s.db.QueryRow(
		`SELECT COUNT(1) FROM elbv2_rules WHERE account_id = ? AND listener_arn = ? AND priority = ?`,
		accountID, listenerARN, priority,
	).Scan(&existing); err != nil {
		return ELBv2Rule{}, fmt.Errorf("create rule: check priority: %w", err)
	}
	if existing > 0 {
		return ELBv2Rule{}, ErrELBv2PriorityInUse
	}
	if region == "" {
		region = DefaultELBv2Region
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	ruleARN := strings.Replace(listenerARN, ":listener/", ":listener-rule/", 1) + "/" + id
	if !strings.Contains(ruleARN, "listener-rule") {
		ruleARN = fmt.Sprintf("arn:aws:elasticloadbalancing:%s:%s:listener-rule/%s", region, accountID, id)
	}
	pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, err := encodeELBv2RuleConditions(cleaned)
	if err != nil {
		return ELBv2Rule{}, err
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO elbv2_rules (account_id, rule_arn, listener_arn, priority, path_patterns, host_headers, http_headers, query_strings, source_ips, target_group_arn, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, ruleARN, listenerARN, priority, pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, targetGroupARN, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return ELBv2Rule{}, ErrELBv2PriorityInUse
		}
		return ELBv2Rule{}, fmt.Errorf("create rule: %w", err)
	}
	return ELBv2Rule{
		RuleARN: ruleARN, ListenerARN: listenerARN, Priority: priority,
		PathPatterns: cleaned.PathPatterns, HostHeaders: cleaned.HostHeaders,
		HTTPHeaders: cleaned.HTTPHeaders, QueryStrings: cleaned.QueryStrings, SourceIPs: cleaned.SourceIPs,
		TargetGroupARN: targetGroupARN, CreatedAt: now,
	}, nil
}

func encodeELBv2RuleConditions(cond ELBv2RuleConditions) (pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON string, err error) {
	b, err := json.Marshal(cond.PathPatterns)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("encode path patterns: %w", err)
	}
	pathsJSON = string(b)
	b, err = json.Marshal(cond.HostHeaders)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("encode host headers: %w", err)
	}
	hostsJSON = string(b)
	if cond.HTTPHeaders == nil {
		cond.HTTPHeaders = []ELBv2HTTPHeaderCond{}
	}
	b, err = json.Marshal(cond.HTTPHeaders)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("encode http headers: %w", err)
	}
	httpJSON = string(b)
	if cond.QueryStrings == nil {
		cond.QueryStrings = []ELBv2QueryStringCond{}
	}
	b, err = json.Marshal(cond.QueryStrings)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("encode query strings: %w", err)
	}
	queryJSON = string(b)
	b, err = json.Marshal(cond.SourceIPs)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("encode source ips: %w", err)
	}
	ipsJSON = string(b)
	return pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, nil
}

func scanELBv2Rule(ruleARN, listenerARN, pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, tgARN string, priority int, createdAt int64) ELBv2Rule {
	r := ELBv2Rule{
		RuleARN: ruleARN, ListenerARN: listenerARN, Priority: priority,
		TargetGroupARN: tgARN, CreatedAt: createdAt,
	}
	_ = json.Unmarshal([]byte(pathsJSON), &r.PathPatterns)
	if r.PathPatterns == nil {
		r.PathPatterns = []string{}
	}
	_ = json.Unmarshal([]byte(hostsJSON), &r.HostHeaders)
	if r.HostHeaders == nil {
		r.HostHeaders = []string{}
	}
	_ = json.Unmarshal([]byte(httpJSON), &r.HTTPHeaders)
	if r.HTTPHeaders == nil {
		r.HTTPHeaders = []ELBv2HTTPHeaderCond{}
	}
	_ = json.Unmarshal([]byte(queryJSON), &r.QueryStrings)
	if r.QueryStrings == nil {
		r.QueryStrings = []ELBv2QueryStringCond{}
	}
	_ = json.Unmarshal([]byte(ipsJSON), &r.SourceIPs)
	if r.SourceIPs == nil {
		r.SourceIPs = []string{}
	}
	return r
}

// DescribeELBv2Rules lists non-default rules for a listener, optionally filtered by RuleArns.
func (s *Store) DescribeELBv2Rules(accountID, listenerARN string, ruleARNs []string) ([]ELBv2Rule, error) {
	listenerARN = strings.TrimSpace(listenerARN)
	want := map[string]struct{}{}
	for _, a := range ruleARNs {
		a = strings.TrimSpace(a)
		if a != "" {
			want[a] = struct{}{}
		}
	}
	var (
		rows *sql.Rows
		err  error
	)
	switch {
	case listenerARN != "":
		if _, err := getELBv2ListenerByARN(s, accountID, listenerARN); err != nil {
			return nil, err
		}
		rows, err = s.db.Query(
			`SELECT rule_arn, listener_arn, priority, path_patterns, host_headers,
			        COALESCE(http_headers, '[]'), COALESCE(query_strings, '[]'), COALESCE(source_ips, '[]'),
			        target_group_arn, created_at
			 FROM elbv2_rules WHERE account_id = ? AND listener_arn = ? ORDER BY priority ASC`,
			accountID, listenerARN,
		)
	case len(want) > 0:
		rows, err = s.db.Query(
			`SELECT rule_arn, listener_arn, priority, path_patterns, host_headers,
			        COALESCE(http_headers, '[]'), COALESCE(query_strings, '[]'), COALESCE(source_ips, '[]'),
			        target_group_arn, created_at
			 FROM elbv2_rules WHERE account_id = ? ORDER BY priority ASC`,
			accountID,
		)
	default:
		return nil, fmt.Errorf("%w: ListenerArn or RuleArns required", ErrELBv2BadRequest)
	}
	if err != nil {
		return nil, fmt.Errorf("describe rules: %w", err)
	}
	defer rows.Close()
	var out []ELBv2Rule
	for rows.Next() {
		var (
			ruleARN, listener, pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, tgARN string
			priority                                                                     int
			createdAt                                                                    int64
		)
		if err := rows.Scan(&ruleARN, &listener, &priority, &pathsJSON, &hostsJSON, &httpJSON, &queryJSON, &ipsJSON, &tgARN, &createdAt); err != nil {
			return nil, fmt.Errorf("describe rules scan: %w", err)
		}
		r := scanELBv2Rule(ruleARN, listener, pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, tgARN, priority, createdAt)
		if len(want) > 0 {
			if _, ok := want[r.RuleARN]; !ok {
				continue
			}
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(want) > 0 && listenerARN == "" && len(out) == 0 {
		return nil, ErrELBv2RuleNotFound
	}
	return out, nil
}

// GetELBv2RuleByARN returns a rule by ARN.
func (s *Store) GetELBv2RuleByARN(accountID, ruleARN string) (ELBv2Rule, error) {
	var (
		listener, pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, tgARN string
		priority                                                            int
		createdAt                                                           int64
		arn                                                                 string
	)
	err := s.db.QueryRow(
		`SELECT rule_arn, listener_arn, priority, path_patterns, host_headers,
		        COALESCE(http_headers, '[]'), COALESCE(query_strings, '[]'), COALESCE(source_ips, '[]'),
		        target_group_arn, created_at
		 FROM elbv2_rules WHERE account_id = ? AND rule_arn = ?`,
		accountID, strings.TrimSpace(ruleARN),
	).Scan(&arn, &listener, &priority, &pathsJSON, &hostsJSON, &httpJSON, &queryJSON, &ipsJSON, &tgARN, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ELBv2Rule{}, ErrELBv2RuleNotFound
	}
	if err != nil {
		return ELBv2Rule{}, fmt.Errorf("get rule by arn: %w", err)
	}
	return scanELBv2Rule(arn, listener, pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, tgARN, priority, createdAt), nil
}

// ModifyELBv2Rule updates conditions and/or forward TargetGroupArn on an existing rule.
// Pass nil for conditions or targetGroupARN to leave that property unchanged.
func (s *Store) ModifyELBv2Rule(accountID, ruleARN string, conditions *ELBv2RuleConditions, targetGroupARN *string) (ELBv2Rule, error) {
	ruleARN = strings.TrimSpace(ruleARN)
	if ruleARN == "" {
		return ELBv2Rule{}, fmt.Errorf("%w: RuleArn required", ErrELBv2BadRequest)
	}
	if conditions == nil && targetGroupARN == nil {
		return ELBv2Rule{}, fmt.Errorf("%w: Conditions or Actions required", ErrELBv2BadRequest)
	}
	rule, err := s.GetELBv2RuleByARN(accountID, ruleARN)
	if err != nil {
		return ELBv2Rule{}, err
	}
	cond := ELBv2RuleConditions{
		PathPatterns: rule.PathPatterns, HostHeaders: rule.HostHeaders,
		HTTPHeaders: rule.HTTPHeaders, QueryStrings: rule.QueryStrings, SourceIPs: rule.SourceIPs,
	}
	if conditions != nil {
		cleaned, cleanErr := cleanELBv2RuleConditions(*conditions)
		if cleanErr != nil {
			return ELBv2Rule{}, cleanErr
		}
		cond = cleaned
	}
	tgARN := rule.TargetGroupARN
	if targetGroupARN != nil {
		tgARN = strings.TrimSpace(*targetGroupARN)
		if tgARN == "" {
			return ELBv2Rule{}, fmt.Errorf("%w: Actions forward TargetGroupArn required", ErrELBv2BadRequest)
		}
		tgs, tgErr := s.DescribeELBv2TargetGroups(accountID, []string{tgARN})
		if tgErr != nil {
			return ELBv2Rule{}, tgErr
		}
		if len(tgs) == 0 {
			return ELBv2Rule{}, ErrELBv2TGNotFound
		}
	}
	pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, err := encodeELBv2RuleConditions(cond)
	if err != nil {
		return ELBv2Rule{}, err
	}
	_, err = s.db.Exec(
		`UPDATE elbv2_rules SET path_patterns = ?, host_headers = ?, http_headers = ?, query_strings = ?, source_ips = ?, target_group_arn = ?
		 WHERE account_id = ? AND rule_arn = ?`,
		pathsJSON, hostsJSON, httpJSON, queryJSON, ipsJSON, tgARN, accountID, ruleARN,
	)
	if err != nil {
		return ELBv2Rule{}, fmt.Errorf("modify rule: %w", err)
	}
	rule.PathPatterns = cond.PathPatterns
	rule.HostHeaders = cond.HostHeaders
	rule.HTTPHeaders = cond.HTTPHeaders
	rule.QueryStrings = cond.QueryStrings
	rule.SourceIPs = cond.SourceIPs
	rule.TargetGroupARN = tgARN
	return rule, nil
}

// ModifyELBv2Listener updates default forward TargetGroupArn and/or port/protocol where modeled.
// Pass nil for a field to leave it unchanged.
func (s *Store) ModifyELBv2Listener(accountID, listenerARN string, port *int, protocol *string, targetGroupARN *string) (ELBv2Listener, error) {
	listenerARN = strings.TrimSpace(listenerARN)
	if listenerARN == "" {
		return ELBv2Listener{}, fmt.Errorf("%w: ListenerArn required", ErrELBv2BadRequest)
	}
	if port == nil && protocol == nil && targetGroupARN == nil {
		return ELBv2Listener{}, fmt.Errorf("%w: Port, Protocol, or DefaultActions required", ErrELBv2BadRequest)
	}
	listener, err := getELBv2ListenerByARN(s, accountID, listenerARN)
	if err != nil {
		return ELBv2Listener{}, err
	}
	lbs, err := s.DescribeELBv2LoadBalancers(accountID, []string{listener.LoadBalancerARN})
	if err != nil {
		return ELBv2Listener{}, err
	}
	if len(lbs) == 0 {
		return ELBv2Listener{}, ErrELBv2NotFound
	}
	lb := lbs[0]

	newPort := listener.Port
	if port != nil {
		newPort = *port
		if newPort < 1 || newPort > 65535 {
			return ELBv2Listener{}, fmt.Errorf("%w: Port must be between 1 and 65535", ErrELBv2BadRequest)
		}
	}
	newProtocol := listener.Protocol
	if protocol != nil {
		newProtocol = strings.ToUpper(strings.TrimSpace(*protocol))
	}
	newTG := listener.TargetGroupARN
	if targetGroupARN != nil {
		newTG = strings.TrimSpace(*targetGroupARN)
		if newTG == "" {
			return ELBv2Listener{}, fmt.Errorf("%w: DefaultActions forward TargetGroupArn required", ErrELBv2BadRequest)
		}
	}

	tgs, err := s.DescribeELBv2TargetGroups(accountID, []string{newTG})
	if err != nil {
		return ELBv2Listener{}, err
	}
	if len(tgs) == 0 {
		return ELBv2Listener{}, ErrELBv2TGNotFound
	}
	tg := tgs[0]

	switch lb.Type {
	case "network":
		if newProtocol != "TCP" && newProtocol != "TLS" {
			return ELBv2Listener{}, fmt.Errorf("%w: network load balancers support TCP or TLS listeners only", ErrELBv2BadRequest)
		}
		if tg.TargetType != "ip" && tg.TargetType != "instance" {
			return ELBv2Listener{}, fmt.Errorf("%w: network listeners require ip or instance target groups", ErrELBv2BadRequest)
		}
		if tg.Protocol != "TCP" && tg.Protocol != "TLS" {
			return ELBv2Listener{}, fmt.Errorf("%w: network listeners require TCP or TLS target groups", ErrELBv2BadRequest)
		}
	default:
		if newProtocol != "HTTP" && newProtocol != "HTTPS" {
			return ELBv2Listener{}, fmt.Errorf("%w: application load balancers support HTTP or HTTPS listeners only", ErrELBv2BadRequest)
		}
	}

	if newPort != listener.Port {
		var conflict int
		if err := s.db.QueryRow(
			`SELECT COUNT(1) FROM elbv2_listeners WHERE account_id = ? AND load_balancer_arn = ? AND port = ? AND listener_arn != ?`,
			accountID, listener.LoadBalancerARN, newPort, listenerARN,
		).Scan(&conflict); err != nil {
			return ELBv2Listener{}, fmt.Errorf("modify listener: check port: %w", err)
		}
		if conflict > 0 {
			return ELBv2Listener{}, fmt.Errorf("%w: a listener already exists on the specified port", ErrELBv2BadRequest)
		}
	}

	_, err = s.db.Exec(
		`UPDATE elbv2_listeners SET port = ?, protocol = ?, target_group_arn = ? WHERE account_id = ? AND listener_arn = ?`,
		newPort, newProtocol, newTG, accountID, listenerARN,
	)
	if err != nil {
		return ELBv2Listener{}, fmt.Errorf("modify listener: %w", err)
	}
	listener.Port = newPort
	listener.Protocol = newProtocol
	listener.TargetGroupARN = newTG
	return listener, nil
}

// DeleteELBv2Rule deletes a non-default rule by ARN.
func (s *Store) DeleteELBv2Rule(accountID, ruleARN string) error {
	res, err := s.db.Exec(`DELETE FROM elbv2_rules WHERE account_id = ? AND rule_arn = ?`,
		accountID, strings.TrimSpace(ruleARN))
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrELBv2RuleNotFound
	}
	return nil
}

// ResolveELBv2ListenerTargetGroup picks a target group for the lab request:
// first matching rule by ascending priority, else the listener default TargetGroupArn.
func (s *Store) ResolveELBv2ListenerTargetGroup(accountID string, listener ELBv2Listener, in ELBv2RuleMatchInput) (string, error) {
	rules, err := s.DescribeELBv2Rules(accountID, listener.ListenerARN, nil)
	if err != nil {
		return "", err
	}
	for _, rule := range rules {
		if MatchELBv2Rule(rule, in) {
			return rule.TargetGroupARN, nil
		}
	}
	return listener.TargetGroupARN, nil
}
