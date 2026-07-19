package sts

import (
	"strings"
	"testing"
	"time"
)

func TestParseRoleARN(t *testing.T) {
	acct, name, ok := ParseRoleARN("arn:aws:iam::000000000002:role/OrganizationAccountAccessRole")
	if !ok || acct != "000000000002" || name != "OrganizationAccountAccessRole" {
		t.Fatalf("got %q %q %v", acct, name, ok)
	}
	if _, _, ok := ParseRoleARN("bad"); ok {
		t.Fatal("expected false")
	}
}

func TestAssumeRoleXML(t *testing.T) {
	exp := time.Date(2026, 7, 19, 13, 0, 0, 0, time.UTC)
	raw, err := AssumeRoleXML(AssumeRoleResult{
		AccessKeyID:     "ASIAEXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      exp,
		AssumedRoleARN:  "arn:aws:sts::000000000002:assumed-role/OrganizationAccountAccessRole/admin",
		AssumedRoleID:   "000000000002:OrganizationAccountAccessRole:admin",
		RequestID:       "req-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"AssumeRoleResponse",
		"ASIAEXAMPLE",
		"SessionToken",
		"AssumedRoleUser",
		"req-1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}
