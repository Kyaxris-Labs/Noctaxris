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

// Route53ResourceRecordSet is an A or CNAME record set.
type Route53ResourceRecordSet struct {
	Name    string
	Type    string
	TTL     int
	Records []string
}

// EnsureRoute53Schema creates Route 53 tables if missing.
func EnsureRoute53Schema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure route53 schema: db is nil")
	}
	if _, err := db.Exec(route53Schema); err != nil {
		return fmt.Errorf("ensure route53 schema: %w", err)
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
		ttl := ch.TTL
		if ttl <= 0 {
			ttl = 300
		}
		records := ch.Records
		if records == nil {
			records = []string{}
		}
		recordsJSON := joinRecords(records)
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
			`INSERT INTO route53_rrsets (account_id, zone_id, name, type, ttl, records_json)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, zone_id, name, type) DO UPDATE SET
			   ttl = excluded.ttl, records_json = excluded.records_json`,
			accountID, zoneID, name, rtype, ttl, recordsJSON,
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
		`SELECT name, type, ttl, records_json FROM route53_rrsets
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
		var recordsJSON string
		if err := rows.Scan(&rs.Name, &rs.Type, &rs.TTL, &recordsJSON); err != nil {
			return nil, fmt.Errorf("list rrsets scan: %w", err)
		}
		rs.Records = splitRecords(recordsJSON)
		out = append(out, rs)
	}
	return out, rows.Err()
}

// Route53Change is one ChangeResourceRecordSets change.
type Route53Change struct {
	Action  string
	Name    string
	Type    string
	TTL     int
	Records []string
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

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
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
