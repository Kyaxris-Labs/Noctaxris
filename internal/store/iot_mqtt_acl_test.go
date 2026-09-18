package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLabMQTTACLScopedToThingAndDynsecClientId(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"

	thingA, err := st.CreateIoTThing(account, region, "device-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	thingB, err := st.CreateIoTThing(account, region, "device-b", nil)
	if err != nil {
		t.Fatal(err)
	}
	certA, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	certB, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateIoTPolicy(account, region, "connect-pub",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:Connect","iot:Publish"],"Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "connect-pub", certA.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "connect-pub", certB.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thingA.ThingName, certA.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thingB.ThingName, certB.CertificateARN); err != nil {
		t.Fatal(err)
	}

	mat, err := st.EnsureLabMQTTBrokerMaterial()
	if err != nil {
		t.Fatal(err)
	}
	acl, err := os.ReadFile(filepath.Join(mat.SecretsDir, "mosquitto.acl"))
	if err != nil {
		t.Fatal(err)
	}
	aclBody := string(acl)
	if mosquittoACLHasPatternLine(aclBody) {
		t.Fatalf("global pattern ACL still present: %s", aclBody)
	}
	if !strings.Contains(aclBody, "user "+certA.CertificateID) || !strings.Contains(aclBody, "user "+certB.CertificateID) {
		t.Fatalf("missing per-device ACL users: %s", aclBody)
	}
	if !strings.Contains(aclBody, "$aws/things/device-a/shadow/#") || !strings.Contains(aclBody, "$aws/things/device-b/shadow/#") {
		t.Fatalf("missing thing-scoped shadow ACL: %s", aclBody)
	}
	if strings.Contains(aclBody, "$aws/things/+/shadow/#") {
		t.Fatalf("wildcard shadow ACL still present: %s", aclBody)
	}

	dynsecRaw, err := os.ReadFile(filepath.Join(mat.SecretsDir, "dynamic-security.json"))
	if err != nil {
		t.Fatal(err)
	}
	var dynsec map[string]any
	if err := json.Unmarshal(dynsecRaw, &dynsec); err != nil {
		t.Fatal(err)
	}
	clients, _ := dynsec["clients"].([]any)
	foundA, foundB := false, false
	for _, raw := range clients {
		c, _ := raw.(map[string]any)
		user, _ := c["username"].(string)
		clientID, _ := c["clientid"].(string)
		disabled, _ := c["disabled"].(bool)
		switch user {
		case certA.CertificateID:
			foundA = true
			if clientID != thingA.ThingName {
				t.Fatalf("device-a dynsec clientid=%q want %q", clientID, thingA.ThingName)
			}
			if disabled {
				t.Fatal("device-a with Connect Allow should not be disabled")
			}
		case certB.CertificateID:
			foundB = true
			if clientID != thingB.ThingName {
				t.Fatalf("device-b dynsec clientid=%q want %q", clientID, thingB.ThingName)
			}
		}
	}
	if !foundA || !foundB {
		t.Fatalf("dynsec missing device clients: %s", dynsecRaw)
	}

	id, err := st.MQTTConnectCertificateForThing(thingA.ThingName)
	if err != nil {
		t.Fatal(err)
	}
	if id != certA.CertificateID {
		t.Fatalf("connect cert=%s want %s", id, certA.CertificateID)
	}
}

func TestLabMQTTDynsecDisablesAttachedWithoutConnect(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	thing, err := st.CreateIoTThing(account, region, "no-connect-thing", nil)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateIoTPolicy(account, region, "pub-only",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:Publish","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "pub-only", cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if st.AllowMQTTConnect(cert.CertificateID, thing.ThingName) {
		t.Fatal("publish-only policy must not AllowMQTTConnect")
	}
	mat, err := st.EnsureLabMQTTBrokerMaterial()
	if err != nil {
		t.Fatal(err)
	}
	dynsecRaw, err := os.ReadFile(filepath.Join(mat.SecretsDir, "dynamic-security.json"))
	if err != nil {
		t.Fatal(err)
	}
	var dynsec map[string]any
	if err := json.Unmarshal(dynsecRaw, &dynsec); err != nil {
		t.Fatal(err)
	}
	clients, _ := dynsec["clients"].([]any)
	found := false
	for _, raw := range clients {
		c, _ := raw.(map[string]any)
		user, _ := c["username"].(string)
		if user != cert.CertificateID {
			continue
		}
		found = true
		disabled, _ := c["disabled"].(bool)
		if !disabled {
			t.Fatal("attached cert without iot:Connect must be dynsec disabled")
		}
	}
	if !found {
		t.Fatalf("attached cert missing from dynsec: %s", dynsecRaw)
	}
	if _, err := st.MQTTConnectCertificateForThing(thing.ThingName); err == nil {
		t.Fatal("MQTTConnectCertificateForThing must fail without iot:Connect")
	}
}

func TestMQTTConnectCertificateForThingAmbiguous(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	thing, err := st.CreateIoTThing(account, region, "two-connect-thing", nil)
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
	if _, err := st.CreateIoTPolicy(account, region, "connect-both",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iot:Connect","iot:Publish"],"Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "connect-both", c1.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTPolicy(account, region, "connect-both", c2.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, c1.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachIoTThingPrincipal(account, region, thing.ThingName, c2.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MQTTConnectCertificateForThing(thing.ThingName); err == nil {
		t.Fatal("two Connect-Allow certs must fail closed")
	}
}

func TestLabMQTTDynsecDisablesUnattachedCert(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	cert, err := st.CreateIoTKeysAndCertificate(account, region, true)
	if err != nil {
		t.Fatal(err)
	}
	mat, err := st.EnsureLabMQTTBrokerMaterial()
	if err != nil {
		t.Fatal(err)
	}
	dynsecRaw, err := os.ReadFile(filepath.Join(mat.SecretsDir, "dynamic-security.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dynsecRaw), `"username": "`+cert.CertificateID+`"`) {
		t.Fatalf("unattached cert missing from dynsec: %s", dynsecRaw)
	}
	if !strings.Contains(string(dynsecRaw), `"disabled": true`) {
		t.Fatalf("unattached cert should be disabled: %s", dynsecRaw)
	}
}

func mosquittoACLHasPatternLine(aclBody string) bool {
	for _, line := range strings.Split(aclBody, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "pattern ") {
			return true
		}
	}
	return false
}
