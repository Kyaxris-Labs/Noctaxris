package store_test

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEnsureLabIoTCAIdempotent(t *testing.T) {
	st := openIoTStore(t)
	ca1, err := st.EnsureLabIoTCA()
	if err != nil {
		t.Fatalf("EnsureLabIoTCA: %v", err)
	}
	if ca1.CertPEM == "" || ca1.KeyPEM == "" {
		t.Fatal("missing CA material")
	}
	ca2, err := st.EnsureLabIoTCA()
	if err != nil {
		t.Fatalf("EnsureLabIoTCA second: %v", err)
	}
	if ca1.CertPEM != ca2.CertPEM {
		t.Fatal("CA cert changed on second ensure")
	}
	dir := store.LabIoTSecretsDir(st.DataRoot())
	if _, err := filepath.Abs(dir); err != nil {
		t.Fatal(err)
	}
}

func TestCreateIoTKeysAndCertificateLabCASigned(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"

	ca, err := st.EnsureLabIoTCA()
	if err != nil {
		t.Fatalf("EnsureLabIoTCA: %v", err)
	}
	caBlock, _ := pem.Decode([]byte(ca.CertPEM))
	if caBlock == nil {
		t.Fatal("bad CA PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}

	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatalf("CreateIoTKeysAndCertificate: %v", err)
	}
	devBlock, _ := pem.Decode([]byte(cert.CertificatePEM))
	if devBlock == nil {
		t.Fatal("bad device PEM")
	}
	devCert, err := x509.ParseCertificate(devBlock.Bytes)
	if err != nil {
		t.Fatalf("parse device cert: %v", err)
	}
	if err := devCert.CheckSignatureFrom(caCert); err != nil {
		t.Fatalf("device cert not signed by lab CA: %v", err)
	}
	hasClient := false
	hasServer := false
	for _, eku := range devCert.ExtKeyUsage {
		switch eku {
		case x509.ExtKeyUsageClientAuth:
			hasClient = true
		case x509.ExtKeyUsageServerAuth:
			hasServer = true
		}
	}
	if !hasClient || hasServer {
		t.Fatalf("EKU want ClientAuth only, client=%v server=%v", hasClient, hasServer)
	}
	sum := sha256.Sum256(devBlock.Bytes)
	wantID := hex.EncodeToString(sum[:])
	if cert.CertificateID != wantID {
		t.Fatalf("certificateId = %q want sha256 hex %q", cert.CertificateID, wantID)
	}
	if !store.IoTCertificateIDEmbedded(devCert, cert.CertificateID) {
		t.Fatalf("certificateId not embedded in CN/SAN: CN=%q URIs=%v", devCert.Subject.CommonName, devCert.URIs)
	}
}

func TestEvaluateIoTDevicePolicyAllowPublish(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:Publish","Resource":"arn:aws:iot:us-east-1:000000000001:topic/lab/*"}]}`
	if _, err := st.CreateIoTPolicy(account, region, "pub-only", doc); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "pub-only", cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	topic := "arn:aws:iot:us-east-1:000000000001:topic/lab/sensors"
	if !st.EvaluateIoTDevicePolicy(account, region, cert.CertificateID, "iot:Publish", topic) {
		t.Fatal("expected Allow for Publish on matching topic")
	}
	if st.EvaluateIoTDevicePolicy(account, region, cert.CertificateID, "iot:Subscribe", topic) {
		t.Fatal("expected deny without Subscribe Allow")
	}
}

func TestEvaluateIoTDevicePolicyDenyOverrides(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:*","Resource":"*"}]}`
	denyDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"iot:Publish","Resource":"arn:aws:iot:us-east-1:000000000001:topic/secret/*"}]}`
	if _, err := st.CreateIoTPolicy(account, region, "allow-all", allowDoc); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateIoTPolicy(account, region, "deny-secret", denyDoc); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "allow-all", cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "deny-secret", cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	okTopic := "arn:aws:iot:us-east-1:000000000001:topic/public/data"
	secretTopic := "arn:aws:iot:us-east-1:000000000001:topic/secret/creds"
	if !st.EvaluateIoTDevicePolicy(account, region, cert.CertificateID, "iot:Publish", okTopic) {
		t.Fatal("expected Allow on public topic")
	}
	if st.EvaluateIoTDevicePolicy(account, region, cert.CertificateID, "iot:Publish", secretTopic) {
		t.Fatal("expected Deny on secret topic")
	}
}

func TestEvaluateIoTDevicePolicyNoPoliciesDeny(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	if st.EvaluateIoTDevicePolicy(account, region, cert.CertificateID, "iot:Connect", "*") {
		t.Fatal("expected deny with no attached policies")
	}
	// clientId alone must not grant access (evaluator ignores clientId; cert without policy still denies).
	if st.EvaluateIoTDevicePolicy(account, region, cert.CertificateID, "iot:Publish", "arn:aws:iot:us-east-1:000000000001:topic/x") {
		t.Fatal("expected deny")
	}
}

func TestResolveIoTMQTTDeviceMultiCertUniqueAllow(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	thing, err := st.CreateIoTThing(account, region, "multi-cert-thing", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowCert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	denyCert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, allowCert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, denyCert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:Publish","Resource":"*"}]}`
	if _, err := st.CreateIoTPolicy(account, region, "allow-pub", doc); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "allow-pub", allowCert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	topicARN := "arn:aws:iot:us-east-1:000000000001:topic/$aws/things/multi-cert-thing/shadow/update"
	dev, err := st.ResolveIoTMQTTDeviceForThingShadow(account, region, thing.ThingName, "", "iot:Publish", topicARN)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if dev.CertificateID != allowCert.CertificateID {
		t.Fatalf("cert=%s want allow cert %s", dev.CertificateID, allowCert.CertificateID)
	}
}

func TestResolveIoTMQTTDeviceMultiCertAmbiguousAllow(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	thing, err := st.CreateIoTThing(account, region, "ambig-thing", nil)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, c1.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, c2.CertificateARN); err != nil {
		t.Fatal(err)
	}
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:Publish","Resource":"*"}]}`
	if _, err := st.CreateIoTPolicy(account, region, "allow-both", doc); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "allow-both", c1.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "allow-both", c2.CertificateARN); err != nil {
		t.Fatal(err)
	}
	topicARN := "arn:aws:iot:us-east-1:000000000001:topic/$aws/things/ambig-thing/shadow/update"
	_, err = st.ResolveIoTMQTTDeviceForThingShadow(account, region, thing.ThingName, "", "iot:Publish", topicARN)
	if err == nil {
		t.Fatal("expected ambiguous multi-Allow error")
	}
	// Explicit certificateId disambiguates.
	dev, err := st.ResolveIoTMQTTDeviceForThingShadow(account, region, thing.ThingName, c2.CertificateID, "iot:Publish", topicARN)
	if err != nil {
		t.Fatal(err)
	}
	if dev.CertificateID != c2.CertificateID {
		t.Fatalf("got %s want %s", dev.CertificateID, c2.CertificateID)
	}
}
