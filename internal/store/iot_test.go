package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openIoTStore(t *testing.T) *store.Store {
	t.Helper()
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
	return st
}

func TestIoTThingCertPolicyShadowFlow(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	thing, err := st.CreateIoTThing(account, region, "lab-device", map[string]string{"env": "lab"})
	if err != nil {
		t.Fatalf("CreateIoTThing: %v", err)
	}
	if thing.ThingARN == "" {
		t.Fatal("missing thing ARN")
	}
	again, err := st.CreateIoTThing(account, region, "lab-device", map[string]string{"env": "lab"})
	if err != nil || again.ThingName != thing.ThingName {
		t.Fatalf("idempotent create: %+v err=%v", again, err)
	}
	if _, err := st.CreateIoTThing(account, region, "lab-device", map[string]string{"env": "prod"}); err == nil {
		t.Fatal("expected conflict on attribute change")
	}

	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatalf("CreateIoTKeysAndCertificate: %v", err)
	}
	if cert.CertificatePEM == "" || cert.PrivateKey == "" || cert.Status != "ACTIVE" {
		t.Fatalf("bad cert: %+v", cert)
	}
	if err := st.UpdateIoTCertificate(account, region, cert.CertificateID, "INACTIVE"); err != nil {
		t.Fatalf("UpdateIoTCertificate: %v", err)
	}

	pol, err := st.CreateIoTPolicy(account, region, "lab-policy", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:*","Resource":"*"}]}`)
	if err != nil {
		t.Fatalf("CreateIoTPolicy: %v", err)
	}
	if err := st.AttachIoTPolicy(account, region, pol.PolicyName, cert.CertificateARN); err != nil {
		t.Fatalf("AttachIoTPolicy: %v", err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, cert.CertificateARN); err != nil {
		t.Fatalf("AttachIoTThingPrincipal: %v", err)
	}
	principals, err := st.ListIoTThingPrincipals(account, region, thing.ThingName)
	if err != nil || len(principals) != 1 || principals[0] != cert.CertificateARN {
		t.Fatalf("ListIoTThingPrincipals: %v %v", principals, err)
	}

	sh, err := st.UpdateIoTThingShadow(account, region, thing.ThingName, "", map[string]any{
		"state": map[string]any{"desired": map[string]any{"power": "on"}},
	})
	if err != nil || sh.Version != 1 {
		t.Fatalf("UpdateIoTThingShadow: %+v err=%v", sh, err)
	}
	got, err := st.GetIoTThingShadow(account, region, thing.ThingName, "")
	if err != nil || !strings.Contains(got.PayloadJSON, "power") {
		t.Fatalf("GetIoTThingShadow: %+v err=%v", got, err)
	}
	if err := st.DeleteIoTThingShadow(account, region, thing.ThingName, ""); err != nil {
		t.Fatalf("DeleteIoTThingShadow: %v", err)
	}

	if err := st.UpdateIoTCertificate(account, region, cert.CertificateID, "ACTIVE"); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if err := st.DeleteIoTCertificate(account, region, cert.CertificateID); err == nil {
		t.Fatal("expected delete conflict for ACTIVE cert")
	}
	if err := st.UpdateIoTCertificate(account, region, cert.CertificateID, "INACTIVE"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteIoTCertificate(account, region, cert.CertificateID); err == nil {
		t.Fatal("expected delete conflict while attached")
	}

	if err := st.DetachIoTPolicy(account, region, pol.PolicyName, cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	// Detach principal by deleting thing attachments via DeleteIoTThing after cleanup.
	if err := st.DeleteIoTThing(account, region, thing.ThingName); err != nil {
		t.Fatalf("DeleteIoTThing: %v", err)
	}
	if err := st.DeleteIoTCertificate(account, region, cert.CertificateID); err != nil {
		t.Fatalf("DeleteIoTCertificate: %v", err)
	}
	if err := st.DeleteIoTPolicy(account, region, pol.PolicyName); err != nil {
		t.Fatalf("DeleteIoTPolicy: %v", err)
	}
}
