package store

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	labMQTTBrokerDirName     = "mqtt-broker"
	labMQTTCAFile            = "ca.crt"
	labMQTTServerCertFile    = "server.crt"
	labMQTTServerKeyFile     = "server.key"
	labMQTTBridgeCertFile    = "bridge.crt"
	labMQTTBridgeKeyFile     = "bridge.key"
	labMQTTMosquittoConfFile = "mosquitto.conf"
	labMQTTACLFile           = "mosquitto.acl"

	mqttBrokerContainerDir = "/etc/noctaxris/mqtt"
)

// LabMQTTBrokerMaterial is TLS and Mosquitto config on disk for the shared broker.
type LabMQTTBrokerMaterial struct {
	SecretsDir       string
	MosquittoConf    string
	BridgeCertPEM    string
	BridgeKeyPEM     string
	CACertPEM        string
	ContainerConf    string
	ContainerCA      string
	ContainerServerC string
	ContainerServerK string
	Binds            []string
}

// EnsureLabMQTTBrokerMaterial creates or loads Mosquitto TLS files and config under the lab secrets dir.
func (s *Store) EnsureLabMQTTBrokerMaterial() (LabMQTTBrokerMaterial, error) {
	if s == nil {
		return LabMQTTBrokerMaterial{}, fmt.Errorf("ensure mqtt broker material: store is nil")
	}
	ca, err := s.EnsureLabIoTCA()
	if err != nil {
		return LabMQTTBrokerMaterial{}, err
	}
	dir := filepath.Join(LabIoTSecretsDir(s.dataRoot), labMQTTBrokerDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return LabMQTTBrokerMaterial{}, fmt.Errorf("create mqtt broker dir: %w", err)
	}
	caPath := filepath.Join(dir, labMQTTCAFile)
	if err := writeFileIfMissing(caPath, []byte(ca.CertPEM), 0o600); err != nil {
		return LabMQTTBrokerMaterial{}, err
	}
	serverCertPath := filepath.Join(dir, labMQTTServerCertFile)
	serverKeyPath := filepath.Join(dir, labMQTTServerKeyFile)
	if err := ensureLabMQTTLSPair(ca, serverCertPath, serverKeyPath, "noctaxris-lab-mqtt", false, true); err != nil {
		return LabMQTTBrokerMaterial{}, err
	}
	bridgeCertPath := filepath.Join(dir, labMQTTBridgeCertFile)
	bridgeKeyPath := filepath.Join(dir, labMQTTBridgeKeyFile)
	if err := ensureLabMQTTLSPair(ca, bridgeCertPath, bridgeKeyPath, LabMQTTBridgeUsername, true, false); err != nil {
		return LabMQTTBrokerMaterial{}, err
	}
	aclPath := filepath.Join(dir, labMQTTACLFile)
	dynsecPath := filepath.Join(dir, labMQTTDynsecFile)
	aclBody, dynsecBody, err := s.labMQTTAuthFiles()
	if err != nil {
		return LabMQTTBrokerMaterial{}, err
	}
	if err := os.WriteFile(aclPath, []byte(aclBody), 0o600); err != nil {
		return LabMQTTBrokerMaterial{}, fmt.Errorf("write mosquitto acl: %w", err)
	}
	if err := os.WriteFile(dynsecPath, dynsecBody, 0o600); err != nil {
		return LabMQTTBrokerMaterial{}, fmt.Errorf("write mosquitto dynsec: %w", err)
	}
	stampPath, err := writeMQTTAuthStamp(dir, aclBody, dynsecBody)
	if err != nil {
		return LabMQTTBrokerMaterial{}, err
	}
	confPath := filepath.Join(dir, labMQTTMosquittoConfFile)
	confBody := mosquittoConfContent(mqttBrokerContainerDir)
	if err := os.WriteFile(confPath, []byte(confBody), 0o600); err != nil {
		return LabMQTTBrokerMaterial{}, fmt.Errorf("write mosquitto conf: %w", err)
	}
	bridgeCertPEM, err := os.ReadFile(bridgeCertPath)
	if err != nil {
		return LabMQTTBrokerMaterial{}, err
	}
	bridgeKeyPEM, err := os.ReadFile(bridgeKeyPath)
	if err != nil {
		return LabMQTTBrokerMaterial{}, err
	}
	binds := []string{
		filepath.ToSlash(confPath) + ":" + mqttBrokerContainerDir + "/mosquitto.conf:ro",
		filepath.ToSlash(caPath) + ":" + mqttBrokerContainerDir + "/ca.crt:ro",
		filepath.ToSlash(serverCertPath) + ":" + mqttBrokerContainerDir + "/server.crt:ro",
		filepath.ToSlash(serverKeyPath) + ":" + mqttBrokerContainerDir + "/server.key:ro",
		filepath.ToSlash(aclPath) + ":" + mqttBrokerContainerDir + "/mosquitto.acl:ro",
		filepath.ToSlash(dynsecPath) + ":" + mqttBrokerContainerDir + "/dynamic-security.json:ro",
		filepath.ToSlash(stampPath) + ":" + mqttBrokerContainerDir + "/auth.stamp:ro",
	}
	return LabMQTTBrokerMaterial{
		SecretsDir:       dir,
		MosquittoConf:    confPath,
		BridgeCertPEM:    string(bridgeCertPEM),
		BridgeKeyPEM:     string(bridgeKeyPEM),
		CACertPEM:        ca.CertPEM,
		ContainerConf:    mqttBrokerContainerDir + "/mosquitto.conf",
		ContainerCA:      mqttBrokerContainerDir + "/ca.crt",
		ContainerServerC: mqttBrokerContainerDir + "/server.crt",
		ContainerServerK: mqttBrokerContainerDir + "/server.key",
		Binds:            binds,
	}, nil
}

func writeFileIfMissing(path string, data []byte, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, data, mode)
}

func mosquittoConfContent(mountDir string) string {
	mountDir = strings.TrimSuffix(mountDir, "/")
	return fmt.Sprintf(`listener 1883 0.0.0.0
allow_anonymous false
require_certificate true
use_identity_as_username true
cafile %s/ca.crt
certfile %s/server.crt
keyfile %s/server.key
acl_file %s/mosquitto.acl
plugin %s
plugin_opt_config_file %s/dynamic-security.json
`, mountDir, mountDir, mountDir, mountDir, labMQTTDynsecPluginPath, mountDir)
}

func ensureLabMQTTLSPair(ca LabIoTCA, certPath, keyPath, cn string, clientAuth, serverAuth bool) error {
	if _, err := os.Stat(certPath); err == nil {
		if _, err2 := os.Stat(keyPath); err2 == nil {
			return nil
		}
	}
	certPEM, keyPEM, err := signLabMQTTLeaf(ca, cn, clientAuth, serverAuth)
	if err != nil {
		return err
	}
	if err := os.WriteFile(certPath, []byte(certPEM), 0o600); err != nil {
		return err
	}
	return os.WriteFile(keyPath, []byte(keyPEM), 0o600)
}

func signLabMQTTLeaf(ca LabIoTCA, cn string, clientAuth, serverAuth bool) (certPEM, keyPEM string, err error) {
	caCert, caKey, err := parseCAKeyPair(ca)
	if err != nil {
		return "", "", err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	var ekus []x509.ExtKeyUsage
	if clientAuth {
		ekus = append(ekus, x509.ExtKeyUsageClientAuth)
	}
	if serverAuth {
		ekus = append(ekus, x509.ExtKeyUsageServerAuth)
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   strings.TrimSpace(cn),
			Organization: []string{"Noctaxris Lab"},
		},
		NotBefore:   now.Add(-time.Hour),
		NotAfter:    now.Add(825 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: ekus,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}
	certBuf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBuf := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return string(certBuf), string(keyBuf), nil
}
