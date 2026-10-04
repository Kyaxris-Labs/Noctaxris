package store_test

import (
	"crypto/x509"
	"encoding/pem"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestACMOrganizationRespectsStripProduct(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	st.SetStripProduct(false)
	cert, err := st.RequestACMCertificate("000000000001", "us-east-1", "example.test")
	if err != nil {
		t.Fatalf("RequestACMCertificate: %v", err)
	}
	org := parseCertOrganization(t, cert.CertPEM)
	if org != "Noctaxris Lab" {
		t.Fatalf("default Organization=%q", org)
	}

	dir2 := t.TempDir()
	key2, err := store.LoadOrCreateMasterKey(filepath.Join(dir2, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st2, err := store.Open(dir2, key2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	st2.SetStripProduct(true)
	cert2, err := st2.RequestACMCertificate("000000000001", "us-east-1", "strip.test")
	if err != nil {
		t.Fatalf("RequestACMCertificate strip: %v", err)
	}
	org2 := parseCertOrganization(t, cert2.CertPEM)
	if org2 != "Lab" {
		t.Fatalf("strip Organization=%q want Lab", org2)
	}
}

func parseCertOrganization(t *testing.T, certPEM string) string {
	t.Helper()
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		t.Fatal("pem decode failed")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if len(cert.Subject.Organization) == 0 {
		t.Fatal("missing Organization")
	}
	return cert.Subject.Organization[0]
}
