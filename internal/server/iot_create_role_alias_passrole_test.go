package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const iotCredentialsTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"credentials.iot.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
const iotCredentialsTrustBad = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}`

func TestIoTCreateRoleAliasPassRoleAllow(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "iot-ra-ok", iotCredentialsTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/iot-ra-ok"

	rec := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias": "lab-alias-ok",
		"roleArn":   roleARN,
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "lab-alias-ok") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestIoTCreateRoleAliasPassRoleDenyWrongTrust(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "iot-ra-bad", iotCredentialsTrustBad, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/iot-ra-bad"

	rec := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias": "lab-alias-deny",
		"roleArn":   roleARN,
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not authorized to pass role to IoT") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestIoTCreateRoleAliasPassRoleRejectsEmptyRoleAlias(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "iot-ra-empty", iotCredentialsTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/iot-ra-empty"

	rec := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias": "",
		"roleArn":   roleARN,
	}, now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%q", rec.Code, rec.Body.String())
	}
}

func TestIoTCreateRoleAliasPassRoleRejectsInvalidRoleARN(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias": "lab-alias-badarn",
		"roleArn":   "not-an-arn",
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
}
