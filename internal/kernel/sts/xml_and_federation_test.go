package sts_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

func TestValidateLabTokenCode(t *testing.T) {
	seed := []byte("lab-mfa-seed")
	at := time.Unix(1_700_000_000, 0).UTC()
	code := sts.LabTokenCode(seed, at)
	if !sts.ValidateLabTokenCode(seed, code, at, 0) {
		t.Fatal("exact minute must validate")
	}
	if !sts.ValidateLabTokenCode(seed, code, at.Add(time.Minute), 1) {
		t.Fatal("skew=1 must accept previous minute")
	}
	if sts.ValidateLabTokenCode(seed, code, at.Add(2*time.Minute), 0) {
		t.Fatal("out of window must fail")
	}
	if sts.ValidateLabTokenCode(nil, code, at, 0) {
		t.Fatal("empty seed must fail")
	}
	if sts.ValidateLabTokenCode(seed, "", at, 0) {
		t.Fatal("empty token must fail")
	}
	if sts.ValidateLabTokenCode(seed, "000000", at, -1) {
		t.Fatal("negative skew treated as 0; wrong code must fail")
	}
}

func TestAuthorizationMessageRoundTrip(t *testing.T) {
	enc := sts.EncodeAuthorizationMessage("AccessDenied", "not allowed")
	if enc == "" {
		t.Fatal("expected non-empty encoding")
	}
	decoded, err := sts.DecodeAuthorizationMessage(enc)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(decoded), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["code"] != "AccessDenied" || obj["message"] != "not allowed" {
		t.Fatalf("got %#v", obj)
	}
}

func TestDecodeAuthorizationMessage_failClosed(t *testing.T) {
	_, err := sts.DecodeAuthorizationMessage("!!!")
	if err == nil {
		t.Fatal("expected invalid encoding error")
	}
	if sts.CodeOf(err) != "" && !strings.Contains(err.Error(), sts.CodeInvalidAuthorizationMessageException) {
		// Decode returns fmt.Errorf with the code string, not *Error
	}
	if !strings.Contains(err.Error(), sts.CodeInvalidAuthorizationMessageException) {
		t.Fatalf("want code in error, got %v", err)
	}
	badJSON := base64.StdEncoding.EncodeToString([]byte("not-json"))
	_, err = sts.DecodeAuthorizationMessage(badJSON)
	if err == nil || !strings.Contains(err.Error(), sts.CodeInvalidAuthorizationMessageException) {
		t.Fatalf("want invalid message exception, got %v", err)
	}
}

func TestDecodeAuthorizationMessageXML(t *testing.T) {
	raw, err := sts.DecodeAuthorizationMessageXML(`{"code":"AccessDenied"}`, "req-decode")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{"DecodeAuthorizationMessageResponse", "DecodedMessage", "req-decode", "AccessDenied"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestGetSessionTokenXML(t *testing.T) {
	exp := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	raw, err := sts.GetSessionTokenXML(sts.GetSessionTokenResult{
		AccessKeyID:     "ASIASESSION",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      exp,
		RequestID:       "req-st",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{"GetSessionTokenResponse", "ASIASESSION", "SessionToken", "req-st", "2026-08-01T12:00:00Z"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestGetFederationTokenXML(t *testing.T) {
	exp := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	raw, err := sts.GetFederationTokenXML(sts.GetFederationTokenResult{
		AccessKeyID:      "ASIAFED",
		SecretAccessKey:  "secret",
		SessionToken:     "token",
		Expiration:       exp,
		FederatedUserARN: "arn:aws:sts::000000000001:federated-user/bob",
		FederatedUserID:  "000000000001:bob",
		PackedPolicySize: 12,
		RequestID:        "req-ft",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{"GetFederationTokenResponse", "ASIAFED", "FederatedUser", "PackedPolicySize", "req-ft"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestGetAccessKeyInfoXML(t *testing.T) {
	raw, err := sts.GetAccessKeyInfoXML("000000000001", "req-aki")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{"GetAccessKeyInfoResponse", "000000000001", "req-aki"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestAssumeRootXML(t *testing.T) {
	exp := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	raw, err := sts.AssumeRootXML(sts.AssumeRootResult{
		AccessKeyID:     "ASIAROOT",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      exp,
		SourceIdentity:  "alice",
		RequestID:       "req-root",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{"AssumeRootResponse", "ASIAROOT", "SourceIdentity", "alice", "req-root"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestFederationFailClosed(t *testing.T) {
	err := sts.GetWebIdentityToken(false, "aud")
	if err == nil {
		t.Fatal("expected fail-closed without OIDC")
	}
	if sts.CodeOf(err) != sts.CodeNotSupported {
		t.Fatalf("CodeOf=%q want %q err=%v", sts.CodeOf(err), sts.CodeNotSupported, err)
	}
	err = sts.GetWebIdentityToken(true, "aud")
	if err == nil {
		t.Fatal("expected AccessDenied when configured but unsupported")
	}
	if sts.CodeOf(err) != sts.CodeAccessDeniedException {
		t.Fatalf("CodeOf=%q", sts.CodeOf(err))
	}
	err = sts.GetDelegatedAccessToken()
	if err == nil {
		t.Fatal("expected fail-closed delegated access")
	}
	if sts.CodeOf(err) != sts.CodeAccessDeniedException {
		t.Fatalf("CodeOf=%q", sts.CodeOf(err))
	}
	if sts.CodeOf(nil) != "" {
		t.Fatal("CodeOf(nil) must be empty")
	}
	if !strings.Contains(err.Error(), sts.CodeAccessDeniedException) {
		t.Fatalf("Error()=%q", err.Error())
	}
}

func TestParseRoleARN_boundaries(t *testing.T) {
	cases := []struct {
		arn string
		ok  bool
	}{
		{"arn:aws:iam::000000000001:role/R", true},
		{"arn:aws:iam::000000000001:user/R", false},
		{"arn:aws:sts::000000000001:assumed-role/R/s", false},
		{"arn:aws:iam::123:role/R", false},
		{"", false},
	}
	for _, tc := range cases {
		_, _, ok := sts.ParseRoleARN(tc.arn)
		if ok != tc.ok {
			t.Fatalf("ParseRoleARN(%q)=%v want %v", tc.arn, ok, tc.ok)
		}
	}
	acct, name, ok := sts.ParseRoleARN("arn:aws:iam::000000000001:role/R")
	if !ok || acct != "000000000001" || name != "R" {
		t.Fatalf("got %q %q %v", acct, name, ok)
	}
}
