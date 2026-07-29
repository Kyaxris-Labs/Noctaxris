package store

import (
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrIoTNotFound      = errors.New("ResourceNotFoundException")
	ErrIoTBadRequest    = errors.New("InvalidRequestException")
	ErrIoTConflict      = errors.New("ResourceAlreadyExistsException")
	ErrIoTVersionConflict = errors.New("VersionConflictException")
	ErrIoTDeleteConflict  = errors.New("DeleteConflictException")
)

const DefaultIoTRegion = "us-east-1"

const iotSchema = `
CREATE TABLE IF NOT EXISTS iot_things (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  thing_name TEXT NOT NULL,
  thing_arn TEXT NOT NULL,
  attributes_json TEXT NOT NULL DEFAULT '{}',
  version INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, thing_name)
);
CREATE TABLE IF NOT EXISTS iot_certificates (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  certificate_id TEXT NOT NULL,
  certificate_arn TEXT NOT NULL,
  status TEXT NOT NULL,
  cert_pem TEXT NOT NULL,
  public_key_pem TEXT NOT NULL,
  private_key_pem TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, certificate_id)
);
CREATE TABLE IF NOT EXISTS iot_policies (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  policy_name TEXT NOT NULL,
  policy_arn TEXT NOT NULL,
  policy_document TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, policy_name)
);
CREATE TABLE IF NOT EXISTS iot_policy_attachments (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  policy_name TEXT NOT NULL,
  target TEXT NOT NULL,
  PRIMARY KEY (account_id, region, policy_name, target)
);
CREATE TABLE IF NOT EXISTS iot_thing_principals (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  thing_name TEXT NOT NULL,
  principal TEXT NOT NULL,
  PRIMARY KEY (account_id, region, thing_name, principal)
);
CREATE TABLE IF NOT EXISTS iot_shadows (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  thing_name TEXT NOT NULL,
  shadow_name TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, thing_name, shadow_name)
);
CREATE TABLE IF NOT EXISTS iot_topic_rules (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  rule_name TEXT NOT NULL,
  rule_arn TEXT NOT NULL,
  sql_text TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  rule_disabled INTEGER NOT NULL DEFAULT 0,
  actions_json TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region, rule_name)
);
`

// IoTThing is a lab IoT thing.
type IoTThing struct {
	ThingName  string
	ThingARN   string
	Attributes map[string]string
	Version    int64
	CreatedAt  int64
}

// IoTCertificate is a lab IoT certificate with local PEM material.
type IoTCertificate struct {
	CertificateID  string
	CertificateARN string
	Status         string
	CertificatePEM string
	PublicKey      string
	PrivateKey     string
	CreatedAt      int64
}

// IoTPolicy is a lab IoT policy document.
type IoTPolicy struct {
	PolicyName     string
	PolicyARN      string
	PolicyDocument string
	CreatedAt      int64
}

// IoTShadow is a classic or named thing shadow document.
type IoTShadow struct {
	ThingName   string
	ShadowName  string
	PayloadJSON string
	Version     int64
	UpdatedAt   int64
}

// EnsureIoTSchema creates IoT tables if missing.
func EnsureIoTSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure iot schema: db is nil")
	}
	if _, err := db.Exec(iotSchema); err != nil {
		return fmt.Errorf("ensure iot schema: %w", err)
	}
	return nil
}

// EnsureIoTSchema ensures IoT tables on an open store.
func (s *Store) EnsureIoTSchema() error {
	return EnsureIoTSchema(s.db)
}

func iotRegion(region string) string {
	if strings.TrimSpace(region) == "" {
		return DefaultIoTRegion
	}
	return region
}

// IoTThingARN builds arn:aws:iot:REGION:ACCOUNT:thing/NAME
func IoTThingARN(region, accountID, thingName string) string {
	return fmt.Sprintf("arn:aws:iot:%s:%s:thing/%s", iotRegion(region), accountID, thingName)
}

// IoTCertificateARN builds arn:aws:iot:REGION:ACCOUNT:cert/ID
func IoTCertificateARN(region, accountID, certID string) string {
	return fmt.Sprintf("arn:aws:iot:%s:%s:cert/%s", iotRegion(region), accountID, certID)
}

// IoTPolicyARN builds arn:aws:iot:REGION:ACCOUNT:policy/NAME
func IoTPolicyARN(region, accountID, policyName string) string {
	return fmt.Sprintf("arn:aws:iot:%s:%s:policy/%s", iotRegion(region), accountID, policyName)
}

// CreateIoTThing creates a thing. Identical recreate is idempotent; conflicting attrs fail.
func (s *Store) CreateIoTThing(accountID, region, thingName string, attrs map[string]string) (IoTThing, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTThing{}, err
	}
	region = iotRegion(region)
	thingName = strings.TrimSpace(thingName)
	if thingName == "" {
		return IoTThing{}, fmt.Errorf("%w: thingName required", ErrIoTBadRequest)
	}
	if attrs == nil {
		attrs = map[string]string{}
	}
	attrJSON, err := json.Marshal(attrs)
	if err != nil {
		return IoTThing{}, fmt.Errorf("%w: attributes", ErrIoTBadRequest)
	}
	existing, err := s.DescribeIoTThing(accountID, region, thingName)
	if err == nil {
		if attributesEqual(existing.Attributes, attrs) {
			return existing, nil
		}
		return IoTThing{}, ErrIoTConflict
	}
	if !errors.Is(err, ErrIoTNotFound) {
		return IoTThing{}, err
	}
	now := time.Now().UTC().UnixMilli()
	arn := IoTThingARN(region, accountID, thingName)
	_, err = s.db.Exec(
		`INSERT INTO iot_things (account_id, region, thing_name, thing_arn, attributes_json, version, created_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?)`,
		accountID, region, thingName, arn, string(attrJSON), now,
	)
	if err != nil {
		return IoTThing{}, fmt.Errorf("create thing: %w", err)
	}
	return IoTThing{ThingName: thingName, ThingARN: arn, Attributes: attrs, Version: 1, CreatedAt: now}, nil
}

// DescribeIoTThing returns a thing.
func (s *Store) DescribeIoTThing(accountID, region, thingName string) (IoTThing, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTThing{}, err
	}
	region = iotRegion(region)
	var t IoTThing
	var attrJSON string
	err := s.db.QueryRow(
		`SELECT thing_name, thing_arn, attributes_json, version, created_at
		 FROM iot_things WHERE account_id = ? AND region = ? AND thing_name = ?`,
		accountID, region, strings.TrimSpace(thingName),
	).Scan(&t.ThingName, &t.ThingARN, &attrJSON, &t.Version, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IoTThing{}, ErrIoTNotFound
	}
	if err != nil {
		return IoTThing{}, fmt.Errorf("describe thing: %w", err)
	}
	_ = json.Unmarshal([]byte(attrJSON), &t.Attributes)
	if t.Attributes == nil {
		t.Attributes = map[string]string{}
	}
	return t, nil
}

// ListIoTThings lists things for an account/region.
func (s *Store) ListIoTThings(accountID, region string) ([]IoTThing, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	region = iotRegion(region)
	rows, err := s.db.Query(
		`SELECT thing_name, thing_arn, attributes_json, version, created_at
		 FROM iot_things WHERE account_id = ? AND region = ? ORDER BY thing_name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list things: %w", err)
	}
	defer rows.Close()
	var out []IoTThing
	for rows.Next() {
		var t IoTThing
		var attrJSON string
		if err := rows.Scan(&t.ThingName, &t.ThingARN, &attrJSON, &t.Version, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("list things scan: %w", err)
		}
		_ = json.Unmarshal([]byte(attrJSON), &t.Attributes)
		if t.Attributes == nil {
			t.Attributes = map[string]string{}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateIoTThing updates attributes with optional expectedVersion.
func (s *Store) UpdateIoTThing(accountID, region, thingName string, attrs map[string]string, expectedVersion *int64) (IoTThing, error) {
	existing, err := s.DescribeIoTThing(accountID, region, thingName)
	if err != nil {
		return IoTThing{}, err
	}
	if expectedVersion != nil && existing.Version != *expectedVersion {
		return IoTThing{}, ErrIoTVersionConflict
	}
	if attrs == nil {
		attrs = map[string]string{}
	}
	attrJSON, err := json.Marshal(attrs)
	if err != nil {
		return IoTThing{}, fmt.Errorf("%w: attributes", ErrIoTBadRequest)
	}
	newVersion := existing.Version + 1
	_, err = s.db.Exec(
		`UPDATE iot_things SET attributes_json = ?, version = ? WHERE account_id = ? AND region = ? AND thing_name = ?`,
		string(attrJSON), newVersion, accountID, iotRegion(region), strings.TrimSpace(thingName),
	)
	if err != nil {
		return IoTThing{}, fmt.Errorf("update thing: %w", err)
	}
	existing.Attributes = attrs
	existing.Version = newVersion
	return existing, nil
}

// DeleteIoTThing deletes a thing.
func (s *Store) DeleteIoTThing(accountID, region, thingName string) error {
	if err := s.EnsureIoTSchema(); err != nil {
		return err
	}
	region = iotRegion(region)
	thingName = strings.TrimSpace(thingName)
	res, err := s.db.Exec(
		`DELETE FROM iot_things WHERE account_id = ? AND region = ? AND thing_name = ?`,
		accountID, region, thingName,
	)
	if err != nil {
		return fmt.Errorf("delete thing: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIoTNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM iot_thing_principals WHERE account_id = ? AND region = ? AND thing_name = ?`, accountID, region, thingName)
	_, _ = s.db.Exec(`DELETE FROM iot_shadows WHERE account_id = ? AND region = ? AND thing_name = ?`, accountID, region, thingName)
	return nil
}

// CreateIoTKeysAndCertificate creates a device cert and key pair signed by the lab IoT CA.
func (s *Store) CreateIoTKeysAndCertificate(accountID, region string, setAsActive bool) (IoTCertificate, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTCertificate{}, err
	}
	region = iotRegion(region)
	ca, err := s.EnsureLabIoTCA()
	if err != nil {
		return IoTCertificate{}, fmt.Errorf("ensure lab iot ca: %w", err)
	}
	certPEM, keyPEM, certID, err := signIoTDeviceCertificate(ca, "")
	if err != nil {
		return IoTCertificate{}, fmt.Errorf("generate iot certificate: %w", err)
	}
	status := "INACTIVE"
	if setAsActive {
		status = "ACTIVE"
	}
	// Extract public key PEM from private key for response shape.
	pubPEM := extractRSAPublicKeyPEM(keyPEM)
	now := time.Now().UTC().UnixMilli()
	arn := IoTCertificateARN(region, accountID, certID)
	_, err = s.db.Exec(
		`INSERT INTO iot_certificates (account_id, region, certificate_id, certificate_arn, status, cert_pem, public_key_pem, private_key_pem, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, certID, arn, status, certPEM, pubPEM, keyPEM, now,
	)
	if err != nil {
		return IoTCertificate{}, fmt.Errorf("create iot certificate: %w", err)
	}
	return IoTCertificate{
		CertificateID: certID, CertificateARN: arn, Status: status,
		CertificatePEM: certPEM, PublicKey: pubPEM, PrivateKey: keyPEM, CreatedAt: now,
	}, nil
}

func extractRSAPublicKeyPEM(privateKeyPEM string) string {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return ""
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return ""
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return ""
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
}

// DescribeIoTCertificate returns a certificate by id.
func (s *Store) DescribeIoTCertificate(accountID, region, certificateID string) (IoTCertificate, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTCertificate{}, err
	}
	region = iotRegion(region)
	var c IoTCertificate
	err := s.db.QueryRow(
		`SELECT certificate_id, certificate_arn, status, cert_pem, public_key_pem, private_key_pem, created_at
		 FROM iot_certificates WHERE account_id = ? AND region = ? AND certificate_id = ?`,
		accountID, region, strings.TrimSpace(certificateID),
	).Scan(&c.CertificateID, &c.CertificateARN, &c.Status, &c.CertificatePEM, &c.PublicKey, &c.PrivateKey, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IoTCertificate{}, ErrIoTNotFound
	}
	if err != nil {
		return IoTCertificate{}, fmt.Errorf("describe certificate: %w", err)
	}
	return c, nil
}

// ListIoTCertificates lists certificates.
func (s *Store) ListIoTCertificates(accountID, region string) ([]IoTCertificate, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	region = iotRegion(region)
	rows, err := s.db.Query(
		`SELECT certificate_id, certificate_arn, status, cert_pem, public_key_pem, private_key_pem, created_at
		 FROM iot_certificates WHERE account_id = ? AND region = ? ORDER BY created_at`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	defer rows.Close()
	var out []IoTCertificate
	for rows.Next() {
		var c IoTCertificate
		if err := rows.Scan(&c.CertificateID, &c.CertificateARN, &c.Status, &c.CertificatePEM, &c.PublicKey, &c.PrivateKey, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("list certificates scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateIoTCertificate updates certificate status (ACTIVE/INACTIVE/REVOKED).
func (s *Store) UpdateIoTCertificate(accountID, region, certificateID, newStatus string) error {
	newStatus = strings.ToUpper(strings.TrimSpace(newStatus))
	switch newStatus {
	case "ACTIVE", "INACTIVE", "REVOKED":
	default:
		return fmt.Errorf("%w: newStatus must be ACTIVE, INACTIVE, or REVOKED", ErrIoTBadRequest)
	}
	if _, err := s.DescribeIoTCertificate(accountID, region, certificateID); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`UPDATE iot_certificates SET status = ? WHERE account_id = ? AND region = ? AND certificate_id = ?`,
		newStatus, accountID, iotRegion(region), strings.TrimSpace(certificateID),
	)
	if err != nil {
		return fmt.Errorf("update certificate: %w", err)
	}
	return nil
}

// DeleteIoTCertificate deletes a certificate. Active or attached certs are rejected.
func (s *Store) DeleteIoTCertificate(accountID, region, certificateID string) error {
	c, err := s.DescribeIoTCertificate(accountID, region, certificateID)
	if err != nil {
		return err
	}
	if c.Status == "ACTIVE" {
		return fmt.Errorf("%w: certificate is ACTIVE", ErrIoTDeleteConflict)
	}
	region = iotRegion(region)
	var n int
	err = s.db.QueryRow(
		`SELECT COUNT(*) FROM iot_thing_principals WHERE account_id = ? AND region = ? AND principal = ?`,
		accountID, region, c.CertificateARN,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("delete certificate check attach: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w: certificate is attached to a thing", ErrIoTDeleteConflict)
	}
	_, err = s.db.Exec(
		`DELETE FROM iot_certificates WHERE account_id = ? AND region = ? AND certificate_id = ?`,
		accountID, region, c.CertificateID,
	)
	if err != nil {
		return fmt.Errorf("delete certificate: %w", err)
	}
	_, _ = s.db.Exec(
		`DELETE FROM iot_policy_attachments WHERE account_id = ? AND region = ? AND target = ?`,
		accountID, region, c.CertificateARN,
	)
	return nil
}

// CreateIoTPolicy creates a policy.
func (s *Store) CreateIoTPolicy(accountID, region, policyName, document string) (IoTPolicy, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTPolicy{}, err
	}
	region = iotRegion(region)
	policyName = strings.TrimSpace(policyName)
	if policyName == "" {
		return IoTPolicy{}, fmt.Errorf("%w: policyName required", ErrIoTBadRequest)
	}
	if strings.TrimSpace(document) == "" {
		return IoTPolicy{}, fmt.Errorf("%w: policyDocument required", ErrIoTBadRequest)
	}
	if _, err := s.GetIoTPolicy(accountID, region, policyName); err == nil {
		return IoTPolicy{}, ErrIoTConflict
	} else if !errors.Is(err, ErrIoTNotFound) {
		return IoTPolicy{}, err
	}
	now := time.Now().UTC().UnixMilli()
	arn := IoTPolicyARN(region, accountID, policyName)
	_, err := s.db.Exec(
		`INSERT INTO iot_policies (account_id, region, policy_name, policy_arn, policy_document, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, region, policyName, arn, document, now,
	)
	if err != nil {
		return IoTPolicy{}, fmt.Errorf("create policy: %w", err)
	}
	return IoTPolicy{PolicyName: policyName, PolicyARN: arn, PolicyDocument: document, CreatedAt: now}, nil
}

// GetIoTPolicy returns a policy.
func (s *Store) GetIoTPolicy(accountID, region, policyName string) (IoTPolicy, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTPolicy{}, err
	}
	region = iotRegion(region)
	var p IoTPolicy
	err := s.db.QueryRow(
		`SELECT policy_name, policy_arn, policy_document, created_at
		 FROM iot_policies WHERE account_id = ? AND region = ? AND policy_name = ?`,
		accountID, region, strings.TrimSpace(policyName),
	).Scan(&p.PolicyName, &p.PolicyARN, &p.PolicyDocument, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IoTPolicy{}, ErrIoTNotFound
	}
	if err != nil {
		return IoTPolicy{}, fmt.Errorf("get policy: %w", err)
	}
	return p, nil
}

// ListIoTPolicies lists policies.
func (s *Store) ListIoTPolicies(accountID, region string) ([]IoTPolicy, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	region = iotRegion(region)
	rows, err := s.db.Query(
		`SELECT policy_name, policy_arn, policy_document, created_at
		 FROM iot_policies WHERE account_id = ? AND region = ? ORDER BY policy_name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	defer rows.Close()
	var out []IoTPolicy
	for rows.Next() {
		var p IoTPolicy
		if err := rows.Scan(&p.PolicyName, &p.PolicyARN, &p.PolicyDocument, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list policies scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteIoTPolicy deletes a policy when unattached.
func (s *Store) DeleteIoTPolicy(accountID, region, policyName string) error {
	if _, err := s.GetIoTPolicy(accountID, region, policyName); err != nil {
		return err
	}
	region = iotRegion(region)
	policyName = strings.TrimSpace(policyName)
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM iot_policy_attachments WHERE account_id = ? AND region = ? AND policy_name = ?`,
		accountID, region, policyName,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("delete policy check: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w: policy is attached", ErrIoTDeleteConflict)
	}
	_, err = s.db.Exec(
		`DELETE FROM iot_policies WHERE account_id = ? AND region = ? AND policy_name = ?`,
		accountID, region, policyName,
	)
	if err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	return nil
}

// AttachIoTPolicy attaches a policy to a target (certificate ARN).
func (s *Store) AttachIoTPolicy(accountID, region, policyName, target string) error {
	if _, err := s.GetIoTPolicy(accountID, region, policyName); err != nil {
		return err
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("%w: target required", ErrIoTBadRequest)
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO iot_policy_attachments (account_id, region, policy_name, target) VALUES (?, ?, ?, ?)`,
		accountID, iotRegion(region), strings.TrimSpace(policyName), target,
	)
	if err != nil {
		return fmt.Errorf("attach policy: %w", err)
	}
	return nil
}

// DetachIoTPolicy detaches a policy from a target.
func (s *Store) DetachIoTPolicy(accountID, region, policyName, target string) error {
	_, err := s.db.Exec(
		`DELETE FROM iot_policy_attachments WHERE account_id = ? AND region = ? AND policy_name = ? AND target = ?`,
		accountID, iotRegion(region), strings.TrimSpace(policyName), strings.TrimSpace(target),
	)
	if err != nil {
		return fmt.Errorf("detach policy: %w", err)
	}
	return nil
}

// AttachIoTThingPrincipal attaches a principal (cert ARN) to a thing.
func (s *Store) AttachIoTThingPrincipal(accountID, region, thingName, principal string) error {
	if _, err := s.DescribeIoTThing(accountID, region, thingName); err != nil {
		return err
	}
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return fmt.Errorf("%w: principal required", ErrIoTBadRequest)
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO iot_thing_principals (account_id, region, thing_name, principal) VALUES (?, ?, ?, ?)`,
		accountID, iotRegion(region), strings.TrimSpace(thingName), principal,
	)
	if err != nil {
		return fmt.Errorf("attach thing principal: %w", err)
	}
	return nil
}

// ListIoTThingPrincipals lists principals for a thing.
func (s *Store) ListIoTThingPrincipals(accountID, region, thingName string) ([]string, error) {
	if _, err := s.DescribeIoTThing(accountID, region, thingName); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT principal FROM iot_thing_principals WHERE account_id = ? AND region = ? AND thing_name = ? ORDER BY principal`,
		accountID, iotRegion(region), strings.TrimSpace(thingName),
	)
	if err != nil {
		return nil, fmt.Errorf("list thing principals: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("list thing principals scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateIoTThingShadow merges state into a shadow document (classic when shadowName empty).
func (s *Store) UpdateIoTThingShadow(accountID, region, thingName, shadowName string, payload map[string]any) (IoTShadow, error) {
	if _, err := s.DescribeIoTThing(accountID, region, thingName); err != nil {
		return IoTShadow{}, err
	}
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTShadow{}, err
	}
	region = iotRegion(region)
	thingName = strings.TrimSpace(thingName)
	shadowName = strings.TrimSpace(shadowName)
	existing, err := s.GetIoTThingShadow(accountID, region, thingName, shadowName)
	version := int64(1)
	merged := map[string]any{}
	if err == nil {
		_ = json.Unmarshal([]byte(existing.PayloadJSON), &merged)
		version = existing.Version + 1
	} else if !errors.Is(err, ErrIoTNotFound) {
		return IoTShadow{}, err
	}
	if state, ok := payload["state"].(map[string]any); ok {
		mergedState, _ := merged["state"].(map[string]any)
		if mergedState == nil {
			mergedState = map[string]any{}
		}
		for k, v := range state {
			if v == nil {
				delete(mergedState, k)
				continue
			}
			if child, ok := v.(map[string]any); ok {
				dst, _ := mergedState[k].(map[string]any)
				if dst == nil {
					dst = map[string]any{}
				}
				mergeShadowMaps(dst, child)
				mergedState[k] = dst
			} else {
				mergedState[k] = v
			}
		}
		merged["state"] = mergedState
	} else {
		for k, v := range payload {
			merged[k] = v
		}
	}
	merged["version"] = version
	merged["timestamp"] = time.Now().UTC().Unix()
	raw, err := json.Marshal(merged)
	if err != nil {
		return IoTShadow{}, fmt.Errorf("%w: shadow payload", ErrIoTBadRequest)
	}
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO iot_shadows (account_id, region, thing_name, shadow_name, payload_json, version, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, region, thing_name, shadow_name) DO UPDATE SET
		   payload_json = excluded.payload_json, version = excluded.version, updated_at = excluded.updated_at`,
		accountID, region, thingName, shadowName, string(raw), version, now,
	)
	if err != nil {
		return IoTShadow{}, fmt.Errorf("update shadow: %w", err)
	}
	return IoTShadow{ThingName: thingName, ShadowName: shadowName, PayloadJSON: string(raw), Version: version, UpdatedAt: now}, nil
}

// GetIoTThingShadow returns a shadow document.
func (s *Store) GetIoTThingShadow(accountID, region, thingName, shadowName string) (IoTShadow, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTShadow{}, err
	}
	region = iotRegion(region)
	var sh IoTShadow
	err := s.db.QueryRow(
		`SELECT thing_name, shadow_name, payload_json, version, updated_at
		 FROM iot_shadows WHERE account_id = ? AND region = ? AND thing_name = ? AND shadow_name = ?`,
		accountID, region, strings.TrimSpace(thingName), strings.TrimSpace(shadowName),
	).Scan(&sh.ThingName, &sh.ShadowName, &sh.PayloadJSON, &sh.Version, &sh.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IoTShadow{}, ErrIoTNotFound
	}
	if err != nil {
		return IoTShadow{}, fmt.Errorf("get shadow: %w", err)
	}
	return sh, nil
}

// DeleteIoTThingShadow deletes a shadow.
func (s *Store) DeleteIoTThingShadow(accountID, region, thingName, shadowName string) error {
	if err := s.EnsureIoTSchema(); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`DELETE FROM iot_shadows WHERE account_id = ? AND region = ? AND thing_name = ? AND shadow_name = ?`,
		accountID, iotRegion(region), strings.TrimSpace(thingName), strings.TrimSpace(shadowName),
	)
	if err != nil {
		return fmt.Errorf("delete shadow: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIoTNotFound
	}
	return nil
}

func attributesEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func mergeShadowMaps(dst, src map[string]any) {
	for k, v := range src {
		if v == nil {
			delete(dst, k)
			continue
		}
		if child, ok := v.(map[string]any); ok {
			inner, _ := dst[k].(map[string]any)
			if inner == nil {
				inner = map[string]any{}
			}
			mergeShadowMaps(inner, child)
			dst[k] = inner
			continue
		}
		dst[k] = v
	}
}
