package iot_test

import (
	"encoding/json"
	"testing"

	iotsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/iot"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIoTJSON(t *testing.T) {
	thing := store.IoTThing{ThingName: "lab-thing", ThingARN: "arn:aws:iot:us-east-1:1:thing/lab-thing"}
	if _, err := iotsvc.CreateThingJSON(thing); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.DescribeThingJSON(thing); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.ListThingsJSON([]store.IoTThing{thing}); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.UpdateThingJSON(thing); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.EmptyJSON(); err != nil {
		t.Fatal(err)
	}

	cert := store.IoTCertificate{
		CertificateID: "cert-1", CertificateARN: "arn:aws:iot:us-east-1:1:cert/cert-1",
		Status: "ACTIVE", CertificatePEM: "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----",
	}
	if _, err := iotsvc.CreateKeysAndCertificateJSON(cert); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.DescribeCertificateJSON(cert); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.ListCertificatesJSON([]store.IoTCertificate{cert}); err != nil {
		t.Fatal(err)
	}

	policy := store.IoTPolicy{PolicyName: "lab-policy", PolicyDocument: `{"Version":"2012-10-17","Statement":[]}`}
	if _, err := iotsvc.CreatePolicyJSON(policy); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.GetPolicyJSON(policy); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.ListPoliciesJSON([]store.IoTPolicy{policy}); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.ListThingPrincipalsJSON([]string{cert.CertificateARN}); err != nil {
		t.Fatal(err)
	}

	rule := store.IoTTopicRule{RuleName: "lab-rule", SQL: "SELECT * FROM 'topic'", ActionsJSON: `[]`}
	if _, err := iotsvc.CreateTopicRuleJSON(rule); err != nil {
		t.Fatal(err)
	}
	raw, err := iotsvc.GetTopicRuleJSON(rule)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if _, err := iotsvc.ListTopicRulesJSON([]store.IoTTopicRule{rule}); err != nil {
		t.Fatal(err)
	}
}
