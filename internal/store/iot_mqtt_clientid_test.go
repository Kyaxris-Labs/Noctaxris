package store_test

import (
	"testing"
)

func TestIoTMQTTClientIdMustEqualThingName(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	thing, err := st.CreateIoTThing(account, region, "device-one", nil)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateIoTPolicy(account, region, "connect",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:Connect","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "connect", cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, cert.CertificateARN); err != nil {
		t.Fatal(err)
	}

	if !st.AllowMQTTConnect(cert.CertificateID, thing.ThingName) {
		t.Fatal("thing name clientId should allow Connect")
	}
	if st.AllowMQTTConnect(cert.CertificateID, cert.CertificateID) {
		t.Fatal("certificateId clientId should deny Connect")
	}
	if st.AllowMQTTConnect(cert.CertificateID, "device-one.pem") {
		t.Fatal("filename-shaped clientId should deny Connect")
	}
	if st.AllowMQTTConnect(cert.CertificateID, "other-thing") {
		t.Fatal("mismatched thing clientId should deny Connect")
	}
}
