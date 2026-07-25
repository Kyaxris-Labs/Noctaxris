package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRoute53NotFound   = errors.New("NoSuchHostedZone")
	ErrRoute53BadRequest = errors.New("InvalidInput")
)

const DefaultRoute53Region = "us-east-1"

const route53Schema = `
CREATE TABLE IF NOT EXISTS route53_hosted_zones (
  account_id TEXT NOT NULL,
  zone_id TEXT NOT NULL,
  name TEXT NOT NULL,
  caller_ref TEXT NOT NULL DEFAULT '',
  private_zone INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, zone_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_r53_caller ON route53_hosted_zones(account_id, caller_ref);
CREATE TABLE IF NOT EXISTS route53_rrsets (
  account_id TEXT NOT NULL,
  zone_id TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  ttl INTEGER NOT NULL DEFAULT 300,
  records_json TEXT NOT NULL,
  alias_dns_name TEXT NOT NULL DEFAULT '',
  alias_hosted_zone_id TEXT NOT NULL DEFAULT '',
  alias_evaluate_target_health INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, zone_id, name, type)
);
`

// Route53HostedZone is a hosted zone row.
type Route53HostedZone struct {
	ID          string
	Name        string
	CallerRef   string
	PrivateZone bool
	CreatedAt   int64
}

// Route53AliasTarget is an alias to an in-account CloudFront or ELB DNS name.
type Route53AliasTarget struct {
	DNSName              string
	HostedZoneId         string
	EvaluateTargetHealth bool
}

// Route53ResourceRecordSet is an A/CNAME record set, optionally an Alias.
type Route53ResourceRecordSet struct {
	Name        string
	Type        string
	TTL         int
	Records     []string
	AliasTarget *Route53AliasTarget
}

// EnsureRoute53Schema creates Route 53 tables if missing.
func EnsureRoute53Schema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure route53 schema: db is nil")
	}
	if _, err := db.Exec(route53Schema); err != nil {
		return fmt.Errorf("ensure route53 schema: %w", err)
	}
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE route53_rrsets ADD COLUMN alias_dns_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE route53_rrsets ADD COLUMN alias_hosted_zone_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE route53_rrsets ADD COLUMN alias_evaluate_target_health INTEGER NOT NULL DEFAULT 0`,
	}); err != nil {
		return fmt.Errorf("ensure route53 schema alter: %w", err)
	}
	return nil
}

// EnsureRoute53Schema ensures Route 53 tables on an open store.
func (s *Store) EnsureRoute53Schema() error {
	return EnsureRoute53Schema(s.db)
}

// CreateRoute53HostedZone creates a hosted zone. Name should end with a dot.
func (s *Store) CreateRoute53HostedZone(accountID, name, callerRef string, privateZone bool) (Route53HostedZone, error) {
	name = normalizeDNSName(name)
	callerRef = strings.TrimSpace(callerRef)
	if name == "" {
		return Route53HostedZone{}, fmt.Errorf("%w: Name required", ErrRoute53BadRequest)
	}
	if callerRef == "" {
		callerRef = uuid.NewString()
	}
	id := "Z" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:13])
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO route53_hosted_zones (account_id, zone_id, name, caller_ref, private_zone, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, id, name, callerRef, boolToInt(privateZone), now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return Route53HostedZone{}, fmt.Errorf("%w: CallerReference already used", ErrRoute53BadRequest)
		}
		return Route53HostedZone{}, fmt.Errorf("create hosted zone: %w", err)
	}
	return Route53HostedZone{ID: id, Name: name, CallerRef: callerRef, PrivateZone: privateZone, CreatedAt: now}, nil
}

// DeleteRoute53HostedZone deletes a zone and its record sets.
func (s *Store) DeleteRoute53HostedZone(accountID, zoneID string) error {
	zoneID = normalizeZoneID(zoneID)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete hosted zone begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM route53_hosted_zones WHERE account_id = ? AND zone_id = ?`, accountID, zoneID)
	if err != nil {
		return fmt.Errorf("delete hosted zone: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrRoute53NotFound
	}
	if _, err := tx.Exec(`DELETE FROM route53_rrsets WHERE account_id = ? AND zone_id = ?`, accountID, zoneID); err != nil {
		return fmt.Errorf("delete rrsets: %w", err)
	}
	return tx.Commit()
}

// ListRoute53HostedZones lists hosted zones for an account.
func (s *Store) ListRoute53HostedZones(accountID string) ([]Route53HostedZone, error) {
	rows, err := s.db.Query(
		`SELECT zone_id, name, caller_ref, private_zone, created_at FROM route53_hosted_zones
		 WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list hosted zones: %w", err)
	}
	defer rows.Close()
	var out []Route53HostedZone
	for rows.Next() {
		var z Route53HostedZone
		var priv int
		if err := rows.Scan(&z.ID, &z.Name, &z.CallerRef, &priv, &z.CreatedAt); err != nil {
			return nil, fmt.Errorf("list hosted zones scan: %w", err)
		}
		z.PrivateZone = priv == 1
		out = append(out, z)
	}
	return out, rows.Err()
}

// GetRoute53HostedZone returns a zone by ID.
func (s *Store) GetRoute53HostedZone(accountID, zoneID string) (Route53HostedZone, error) {
	zoneID = normalizeZoneID(zoneID)
	var z Route53HostedZone
	var priv int
	err := s.db.QueryRow(
		`SELECT zone_id, name, caller_ref, private_zone, created_at FROM route53_hosted_zones
		 WHERE account_id = ? AND zone_id = ?`,
		accountID, zoneID,
	).Scan(&z.ID, &z.Name, &z.CallerRef, &priv, &z.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Route53HostedZone{}, ErrRoute53NotFound
	}
	if err != nil {
		return Route53HostedZone{}, fmt.Errorf("get hosted zone: %w", err)
	}
	z.PrivateZone = priv == 1
	return z, nil
}

// ChangeRoute53ResourceRecordSets applies CREATE/UPSERT/DELETE for A and CNAME only.
// AliasTarget is allowed on Type A when DNSName matches an in-account CloudFront DomainName or ELB DNSName.
func (s *Store) ChangeRoute53ResourceRecordSets(accountID, zoneID string, changes []Route53Change) error {
	zoneID = normalizeZoneID(zoneID)
	if _, err := s.GetRoute53HostedZone(accountID, zoneID); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("change rrsets begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, ch := range changes {
		action := strings.ToUpper(strings.TrimSpace(ch.Action))
		name := normalizeDNSName(ch.Name)
		rtype := strings.ToUpper(strings.TrimSpace(ch.Type))
		if name == "" || (rtype != "A" && rtype != "CNAME") {
			return fmt.Errorf("%w: only A and CNAME supported", ErrRoute53BadRequest)
		}
		if action != "CREATE" && action != "UPSERT" && action != "DELETE" {
			return fmt.Errorf("%w: Action must be CREATE, UPSERT, or DELETE", ErrRoute53BadRequest)
		}
		if action == "DELETE" {
			if _, err := tx.Exec(
				`DELETE FROM route53_rrsets WHERE account_id = ? AND zone_id = ? AND name = ? AND type = ?`,
				accountID, zoneID, name, rtype,
			); err != nil {
				return fmt.Errorf("delete rrset: %w", err)
			}
			continue
		}

		aliasDNS := ""
		aliasHZ := ""
		aliasETH := 0
		recordsJSON := ""
		ttl := ch.TTL
		if ttl <= 0 {
			ttl = 300
		}

		if ch.AliasTarget != nil {
			if rtype != "A" {
				return fmt.Errorf("%w: AliasTarget only supported for Type A", ErrRoute53BadRequest)
			}
			if len(ch.Records) > 0 {
				return fmt.Errorf("%w: AliasTarget cannot include ResourceRecords", ErrRoute53BadRequest)
			}
			dns := strings.TrimSpace(ch.AliasTarget.DNSName)
			hz := strings.TrimSpace(ch.AliasTarget.HostedZoneId)
			if dns == "" || hz == "" {
				return fmt.Errorf("%w: AliasTarget DNSName and HostedZoneId required", ErrRoute53BadRequest)
			}
			ok, err := s.route53AliasDNSNameKnown(accountID, dns)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("%w: AliasTarget DNSName is not an in-account CloudFront or ELB DNS name", ErrRoute53BadRequest)
			}
			aliasDNS = normalizeAliasDNSName(dns)
			aliasHZ = hz
			if ch.AliasTarget.EvaluateTargetHealth {
				aliasETH = 1
			}
			ttl = 0
			recordsJSON = ""
		} else {
			records := ch.Records
			if records == nil {
				records = []string{}
			}
			recordsJSON = joinRecords(records)
		}

		if action == "CREATE" {
			var exists int
			err := tx.QueryRow(
				`SELECT 1 FROM route53_rrsets WHERE account_id = ? AND zone_id = ? AND name = ? AND type = ?`,
				accountID, zoneID, name, rtype,
			).Scan(&exists)
			if err == nil {
				return fmt.Errorf("%w: record already exists", ErrRoute53BadRequest)
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("check rrset: %w", err)
			}
		}
		_, err := tx.Exec(
			`INSERT INTO route53_rrsets
			 (account_id, zone_id, name, type, ttl, records_json, alias_dns_name, alias_hosted_zone_id, alias_evaluate_target_health)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, zone_id, name, type) DO UPDATE SET
			   ttl = excluded.ttl,
			   records_json = excluded.records_json,
			   alias_dns_name = excluded.alias_dns_name,
			   alias_hosted_zone_id = excluded.alias_hosted_zone_id,
			   alias_evaluate_target_health = excluded.alias_evaluate_target_health`,
			accountID, zoneID, name, rtype, ttl, recordsJSON, aliasDNS, aliasHZ, aliasETH,
		)
		if err != nil {
			return fmt.Errorf("upsert rrset: %w", err)
		}
	}
	return tx.Commit()
}

// ListRoute53ResourceRecordSets lists record sets for a zone.
func (s *Store) ListRoute53ResourceRecordSets(accountID, zoneID string) ([]Route53ResourceRecordSet, error) {
	zoneID = normalizeZoneID(zoneID)
	if _, err := s.GetRoute53HostedZone(accountID, zoneID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT name, type, ttl, records_json, alias_dns_name, alias_hosted_zone_id, alias_evaluate_target_health
		 FROM route53_rrsets
		 WHERE account_id = ? AND zone_id = ? ORDER BY name, type`,
		accountID, zoneID,
	)
	if err != nil {
		return nil, fmt.Errorf("list rrsets: %w", err)
	}
	defer rows.Close()
	var out []Route53ResourceRecordSet
	for rows.Next() {
		var rs Route53ResourceRecordSet
		var recordsJSON, aliasDNS, aliasHZ string
		var aliasETH int
		if err := rows.Scan(&rs.Name, &rs.Type, &rs.TTL, &recordsJSON, &aliasDNS, &aliasHZ, &aliasETH); err != nil {
			return nil, fmt.Errorf("list rrsets scan: %w", err)
		}
		if aliasDNS != "" {
			rs.AliasTarget = &Route53AliasTarget{
				DNSName:              aliasDNS,
				HostedZoneId:         aliasHZ,
				EvaluateTargetHealth: aliasETH == 1,
			}
		} else {
			rs.Records = splitRecords(recordsJSON)
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

// Route53Change is one ChangeResourceRecordSets change.
type Route53Change struct {
	Action      string
	Name        string
	Type        string
	TTL         int
	Records     []string
	AliasTarget *Route53AliasTarget
}

func (s *Store) route53AliasDNSNameKnown(accountID, dnsName string) (bool, error) {
	want := normalizeAliasDNSName(dnsName)
	if want == "" {
		return false, nil
	}
	dists, err := s.ListCloudFrontDistributions(accountID)
	if err != nil {
		return false, fmt.Errorf("list cloudfront for alias: %w", err)
	}
	for _, d := range dists {
		if normalizeAliasDNSName(d.DomainName) == want {
			return true, nil
		}
	}
	lbs, err := s.DescribeELBv2LoadBalancers(accountID, nil)
	if err != nil {
		return false, fmt.Errorf("list elbv2 for alias: %w", err)
	}
	for _, lb := range lbs {
		if normalizeAliasDNSName(lb.DNSName) == want {
			return true, nil
		}
	}
	return false, nil
}

func normalizeAliasDNSName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.TrimSuffix(name, ".")
}

func normalizeDNSName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name
}

func normalizeZoneID(zoneID string) string {
	zoneID = strings.TrimSpace(zoneID)
	zoneID = strings.TrimPrefix(zoneID, "/hostedzone/")
	return zoneID
}

func joinRecords(records []string) string {
	return strings.Join(records, "\n")
}

func splitRecords(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// Route53QueryLabLogStreamName is the CloudWatch Logs stream for injected query logs.
func Route53QueryLabLogStreamName(at time.Time) string {
	return "route53-query/" + at.UTC().Format("2006/01/02")
}

// InjectRoute53QueryLogs delivers resolver query log lines to a CloudWatch Logs group.
func (s *Store) InjectRoute53QueryLogs(accountID, region, logGroup, logStream string, lines []string, at time.Time) (int, error) {
	logGroup = strings.TrimSpace(logGroup)
	if logGroup == "" {
		return 0, fmt.Errorf("%w: LogGroupName required", ErrRoute53BadRequest)
	}
	if region == "" {
		region = DefaultRoute53Region
	}
	if logStream == "" {
		logStream = Route53QueryLabLogStreamName(at)
	}
	if len(lines) == 0 {
		lines = []string{defaultRoute53QueryLogLine(accountID, region, at)}
	}
	if _, err := s.EnsureLogGroup(accountID, region, logGroup); err != nil {
		return 0, fmt.Errorf("route53 query inject ensure group: %w", err)
	}
	if _, err := s.getLogStream(accountID, logGroup, logStream); errors.Is(err, ErrLogStreamNotFound) {
		if _, err := s.CreateLogStream(accountID, region, logGroup, logStream); err != nil &&
			!errors.Is(err, ErrLogStreamAlreadyExists) {
			return 0, fmt.Errorf("route53 query inject create stream: %w", err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("route53 query inject get stream: %w", err)
	}
	st, err := s.getLogStream(accountID, logGroup, logStream)
	if err != nil {
		return 0, fmt.Errorf("route53 query inject get stream: %w", err)
	}
	events := make([]LogEvent, 0, len(lines))
	baseMillis := at.UnixMilli()
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		events = append(events, LogEvent{
			Message:   line,
			Timestamp: baseMillis + int64(i),
		})
	}
	if len(events) == 0 {
		return 0, fmt.Errorf("%w: no query log lines to deliver", ErrRoute53BadRequest)
	}
	if _, _, err := s.PutLogEvents(accountID, logGroup, logStream, st.UploadSequenceToken, events); err != nil {
		return 0, fmt.Errorf("route53 query inject put log events: %w", err)
	}
	return len(events), nil
}

func defaultRoute53QueryLogLine(accountID, region string, at time.Time) string {
	payload := map[string]any{
		"version":          "1.100000",
		"account_id":       accountID,
		"region":           region,
		"query_timestamp":  at.UTC().Format(time.RFC3339),
		"query_name":       "lab.example.com.",
		"query_type":       "A",
		"answer_type":      "A",
		"rcode":            "NOERROR",
		"srcaddr":          "10.0.1.5",
		"srcport":          "51514",
		"transport":        "UDP",
		"srcids":           map[string]any{"instance": "i-lab00000000000001"},
		"firewall_rule_action": "ALLOW",
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}
