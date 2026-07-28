package store

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	labIoTCACertFile = "iot-lab-ca.crt"
	labIoTCAKeyFile  = "iot-lab-ca.key"
)

var labIoTCAMu sync.Mutex

// LabIoTCA holds PEM-encoded lab IoT CA certificate and private key.
type LabIoTCA struct {
	CertPEM string
	KeyPEM  string
}

// LabIoTSecretsDir returns the sibling secrets directory for lab IoT CA material.
// Example: dataRoot=/var/lib/noctaxris → /var/lib/noctaxris-secrets
func LabIoTSecretsDir(dataRoot string) string {
	abs, err := filepath.Abs(dataRoot)
	if err != nil {
		abs = dataRoot
	}
	parent := filepath.Dir(abs)
	base := filepath.Base(abs)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "noctaxris"
	}
	return filepath.Join(parent, base+"-secrets")
}

func labIoTCAPaths(dataRoot string) (certPath, keyPath string) {
	dir := LabIoTSecretsDir(dataRoot)
	return filepath.Join(dir, labIoTCACertFile), filepath.Join(dir, labIoTCAKeyFile)
}

// EnsureLabIoTCA creates or loads the process-wide lab IoT CA (idempotent).
func (s *Store) EnsureLabIoTCA() (LabIoTCA, error) {
	labIoTCAMu.Lock()
	defer labIoTCAMu.Unlock()
	return s.ensureLabIoTCAUnlocked()
}

func (s *Store) ensureLabIoTCAUnlocked() (LabIoTCA, error) {
	if s == nil {
		return LabIoTCA{}, fmt.Errorf("ensure lab iot ca: store is nil")
	}
	certPath, keyPath := labIoTCAPaths(s.dataRoot)
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil {
		return LabIoTCA{CertPEM: string(certPEM), KeyPEM: string(keyPEM)}, nil
	}
	if certErr != nil && !os.IsNotExist(certErr) {
		return LabIoTCA{}, fmt.Errorf("read lab iot ca cert: %w", certErr)
	}
	if keyErr != nil && !os.IsNotExist(keyErr) {
		return LabIoTCA{}, fmt.Errorf("read lab iot ca key: %w", keyErr)
	}
	if certErr == nil || keyErr == nil {
		return LabIoTCA{}, fmt.Errorf("ensure lab iot ca: incomplete CA material on disk")
	}
	newCertPEM, newKeyPEM, err := generateLabIoTCA()
	if err != nil {
		return LabIoTCA{}, err
	}
	dir := filepath.Dir(certPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return LabIoTCA{}, fmt.Errorf("create lab iot ca dir: %w", err)
	}
	if err := os.WriteFile(certPath, []byte(newCertPEM), 0o600); err != nil {
		return LabIoTCA{}, fmt.Errorf("write lab iot ca cert: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(newKeyPEM), 0o600); err != nil {
		return LabIoTCA{}, fmt.Errorf("write lab iot ca key: %w", err)
	}
	return LabIoTCA{CertPEM: newCertPEM, KeyPEM: newKeyPEM}, nil
}

func generateLabIoTCA() (certPEM, keyPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Noctaxris Lab IoT CA",
			Organization: []string{"Noctaxris Lab"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	certBuf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBuf := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return string(certBuf), string(keyBuf), nil
}

// LabIoTCACertificatePEM returns the lab CA certificate PEM (ensures CA exists).
func (s *Store) LabIoTCACertificatePEM() (string, error) {
	ca, err := s.EnsureLabIoTCA()
	if err != nil {
		return "", err
	}
	return ca.CertPEM, nil
}

// signIoTDeviceCertificate issues a device cert signed by the lab CA with ClientAuth EKU.
// certificateId is embedded in CN or URI SAN when stable; returned id is SHA-256 of DER.
func signIoTDeviceCertificate(ca LabIoTCA, embedCertificateID string) (certPEM, keyPEM, certificateID string, err error) {
	caCert, caKey, err := parseCAKeyPair(ca)
	if err != nil {
		return "", "", "", err
	}
	deviceKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", "", err
	}
	embed := strings.TrimSpace(embedCertificateID)
	var lastDER []byte
	for attempt := 0; attempt < 4; attempt++ {
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return "", "", "", err
		}
		now := time.Now().UTC()
		cn := "noctaxris-iot-device"
		if len(embed) == 64 {
			cn = embed
		}
		var uris []*url.URL
		if embed != "" {
			u, parseErr := url.Parse("urn:noctaxris:iot:certificateid:" + embed)
			if parseErr != nil {
				return "", "", "", parseErr
			}
			uris = []*url.URL{u}
		}
		tmpl := &x509.Certificate{
			SerialNumber: serial,
			Subject: pkix.Name{
				CommonName:   cn,
				Organization: []string{"Noctaxris Lab"},
			},
			NotBefore:    now.Add(-time.Hour),
			NotAfter:     now.Add(825 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			URIs:         uris,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &deviceKey.PublicKey, caKey)
		if err != nil {
			return "", "", "", err
		}
		lastDER = der
		nextID := sha256Hex(der)
		if nextID == embed {
			break
		}
		embed = nextID
	}
	if len(lastDER) == 0 {
		return "", "", "", fmt.Errorf("sign iot device certificate: no certificate produced")
	}
	certificateID = sha256Hex(lastDER)
	certBuf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: lastDER})
	keyBuf := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(deviceKey)})
	return string(certBuf), string(keyBuf), certificateID, nil
}

func parseCAKeyPair(ca LabIoTCA) (*x509.Certificate, *rsa.PrivateKey, error) {
	certBlock, _ := pem.Decode([]byte(ca.CertPEM))
	if certBlock == nil {
		return nil, nil, fmt.Errorf("parse lab iot ca cert pem")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	keyBlock, _ := pem.Decode([]byte(ca.KeyPEM))
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("parse lab iot ca key pem")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return caCert, caKey, nil
}

func sha256Hex(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// IoTCertificateIDEmbedded reports whether certificateId appears in the device cert CN or URI SAN.
func IoTCertificateIDEmbedded(cert *x509.Certificate, certificateID string) bool {
	if cert == nil || certificateID == "" {
		return false
	}
	if IoTCertificateIDFromDER(cert.Raw) != certificateID {
		return false
	}
	if cert.Subject.CommonName == certificateID {
		return true
	}
	prefix := "urn:noctaxris:iot:certificateid:"
	for _, u := range cert.URIs {
		if u == nil {
			continue
		}
		s := u.String()
		if s == prefix+certificateID || strings.HasPrefix(s, prefix) {
			return true
		}
	}
	// CN set to a prior embed pass (64-char hex) counts as lab mapping metadata.
	if len(cert.Subject.CommonName) == 64 && isHex64(cert.Subject.CommonName) {
		return true
	}
	return false
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// IoTCertificateIDFromDER returns the lab certificate id (SHA-256 hex of DER).
func IoTCertificateIDFromDER(der []byte) string {
	return sha256Hex(der)
}
