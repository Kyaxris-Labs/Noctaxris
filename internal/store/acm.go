package store

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrACMNotFound   = errors.New("ResourceNotFoundException")
	ErrACMBadRequest = errors.New("InvalidParameterException")
)

const DefaultACMRegion = "us-east-1"

const acmSchema = `
CREATE TABLE IF NOT EXISTS acm_certificates (
  account_id TEXT NOT NULL,
  certificate_arn TEXT NOT NULL,
  domain_name TEXT NOT NULL,
  status TEXT NOT NULL,
  cert_pem TEXT NOT NULL,
  key_pem TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, certificate_arn)
);
CREATE INDEX IF NOT EXISTS idx_acm_domain ON acm_certificates(account_id, domain_name);
`

// ACMCertificate is a lab ACM certificate row (self-signed PEM).
type ACMCertificate struct {
	CertificateARN string
	DomainName     string
	Status         string
	CertPEM        string
	KeyPEM         string
	CreatedAt      int64
}

// EnsureACMSchema creates ACM tables if missing.
func EnsureACMSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure acm schema: db is nil")
	}
	if _, err := db.Exec(acmSchema); err != nil {
		return fmt.Errorf("ensure acm schema: %w", err)
	}
	return nil
}

// EnsureACMSchema ensures ACM tables on an open store.
func (s *Store) EnsureACMSchema() error {
	return EnsureACMSchema(s.db)
}

// ACMCertificateARN builds arn:aws:acm:REGION:ACCOUNT:certificate/ID
func ACMCertificateARN(region, accountID, id string) string {
	if region == "" {
		region = DefaultACMRegion
	}
	return fmt.Sprintf("arn:aws:acm:%s:%s:certificate/%s", region, accountID, id)
}

// RequestACMCertificate creates a lab self-signed certificate for DomainName.
func (s *Store) RequestACMCertificate(accountID, region, domainName string) (ACMCertificate, error) {
	domainName = strings.TrimSpace(strings.ToLower(domainName))
	if domainName == "" {
		return ACMCertificate{}, fmt.Errorf("%w: DomainName required", ErrACMBadRequest)
	}
	if region == "" {
		region = DefaultACMRegion
	}
	certPEM, keyPEM, err := generateSelfSignedCert(domainName)
	if err != nil {
		return ACMCertificate{}, fmt.Errorf("generate certificate: %w", err)
	}
	id := uuid.NewString()
	arn := ACMCertificateARN(region, accountID, id)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO acm_certificates (account_id, certificate_arn, domain_name, status, cert_pem, key_pem, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, arn, domainName, "ISSUED", certPEM, keyPEM, now,
	)
	if err != nil {
		return ACMCertificate{}, fmt.Errorf("request certificate: %w", err)
	}
	return ACMCertificate{
		CertificateARN: arn,
		DomainName:     domainName,
		Status:         "ISSUED",
		CertPEM:        certPEM,
		KeyPEM:         keyPEM,
		CreatedAt:      now,
	}, nil
}

// DescribeACMCertificate returns a certificate by ARN.
func (s *Store) DescribeACMCertificate(accountID, certificateARN string) (ACMCertificate, error) {
	certificateARN = strings.TrimSpace(certificateARN)
	var c ACMCertificate
	err := s.db.QueryRow(
		`SELECT certificate_arn, domain_name, status, cert_pem, key_pem, created_at
		 FROM acm_certificates WHERE account_id = ? AND certificate_arn = ?`,
		accountID, certificateARN,
	).Scan(&c.CertificateARN, &c.DomainName, &c.Status, &c.CertPEM, &c.KeyPEM, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ACMCertificate{}, ErrACMNotFound
	}
	if err != nil {
		return ACMCertificate{}, fmt.Errorf("describe certificate: %w", err)
	}
	return c, nil
}

// ListACMCertificates lists certificates for an account.
func (s *Store) ListACMCertificates(accountID string) ([]ACMCertificate, error) {
	rows, err := s.db.Query(
		`SELECT certificate_arn, domain_name, status, cert_pem, key_pem, created_at
		 FROM acm_certificates WHERE account_id = ? ORDER BY domain_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	defer rows.Close()
	var out []ACMCertificate
	for rows.Next() {
		var c ACMCertificate
		if err := rows.Scan(&c.CertificateARN, &c.DomainName, &c.Status, &c.CertPEM, &c.KeyPEM, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("list certificates scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteACMCertificate deletes a certificate by ARN.
func (s *Store) DeleteACMCertificate(accountID, certificateARN string) error {
	certificateARN = strings.TrimSpace(certificateARN)
	res, err := s.db.Exec(
		`DELETE FROM acm_certificates WHERE account_id = ? AND certificate_arn = ?`,
		accountID, certificateARN,
	)
	if err != nil {
		return fmt.Errorf("delete certificate: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrACMNotFound
	}
	return nil
}

func generateSelfSignedCert(domainName string) (certPEM, keyPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: domainName, Organization: []string{"Noctaxris Lab"}},
		NotBefore:    time.Now().UTC().Add(-time.Hour),
		NotAfter:     time.Now().UTC().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{domainName},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	certBuf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBuf := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return string(certBuf), string(keyBuf), nil
}
