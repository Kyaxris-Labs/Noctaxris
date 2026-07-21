package federation

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestValidateWebIdentityClaimsExpAndNBF(t *testing.T) {
	const issuer = "https://oidc.example/issuer"
	const client = "lab-client"
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	if _, err := validateWebIdentityClaims(map[string]any{
		"iss": issuer,
		"sub": "user-1",
		"aud": client,
	}, issuer, client, now); err == nil {
		t.Fatal("expected reject for missing exp")
	}

	if _, err := validateWebIdentityClaims(map[string]any{
		"iss": issuer,
		"sub": "user-1",
		"aud": client,
		"exp": now.Add(-time.Hour).Unix(),
	}, issuer, client, now); err == nil {
		t.Fatal("expected reject for expired exp")
	}

	if _, err := validateWebIdentityClaims(map[string]any{
		"iss": issuer,
		"sub": "user-1",
		"aud": client,
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(time.Hour).Unix(),
	}, issuer, client, now); err == nil {
		t.Fatal("expected reject for future nbf")
	}

	got, err := validateWebIdentityClaims(map[string]any{
		"iss": issuer,
		"sub": "user-1",
		"aud": client,
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(-time.Minute).Unix(),
	}, issuer, client, now)
	if err != nil {
		t.Fatalf("validateWebIdentityClaims: %v", err)
	}
	if got.Subject != "user-1" || got.Issuer != issuer {
		t.Fatalf("got %+v", got)
	}
}

func TestValidateWebIdentityClaimsGoldenVectors(t *testing.T) {
	const issuer = "https://oidc.example/issuer"
	const client = "lab-client"
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		claims  map[string]any
		wantErr string
	}{
		{
			name: "iss mismatch",
			claims: map[string]any{
				"iss": "https://other.example/",
				"sub": "user-1",
				"aud": client,
				"exp": now.Add(time.Hour).Unix(),
			},
			wantErr: "iss mismatch",
		},
		{
			name: "aud mismatch",
			claims: map[string]any{
				"iss": issuer,
				"sub": "user-1",
				"aud": "other-client",
				"exp": now.Add(time.Hour).Unix(),
			},
			wantErr: "aud mismatch",
		},
		{
			name: "aud array accepts client",
			claims: map[string]any{
				"iss": issuer,
				"sub": "user-1",
				"aud": []any{"other", client},
				"exp": now.Add(time.Hour).Unix(),
			},
		},
		{
			name: "issuer trailing slash normalized",
			claims: map[string]any{
				"iss": issuer + "/",
				"sub": "user-1",
				"aud": client,
				"exp": float64(now.Add(time.Hour).Unix()),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateWebIdentityClaims(tc.claims, issuer, client, now)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateWebIdentityClaims: %v", err)
			}
			if got.Subject != "user-1" {
				t.Fatalf("subject=%q", got.Subject)
			}
		})
	}
}

func TestVerifySAMLAssertionFailClosedGolden(t *testing.T) {
	meta, _ := mustSAMLIdP(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		metadata   string
		assertion  string
		wantSubstr string
	}{
		{
			name:       "empty metadata",
			metadata:   "",
			assertion:  base64.StdEncoding.EncodeToString([]byte("<Assertion/>")),
			wantSubstr: "IdP not configured",
		},
		{
			name:       "metadata without certificates",
			metadata:   `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example/"><IDPSSODescriptor></IDPSSODescriptor></EntityDescriptor>`,
			assertion:  base64.StdEncoding.EncodeToString([]byte("<Assertion/>")),
			wantSubstr: "no usable certificates",
		},
		{
			name:       "assertion not base64",
			metadata:   meta,
			assertion:  "%%%not-base64%%%",
			wantSubstr: "not valid base64",
		},
		{
			name:       "assertion missing signature",
			metadata:   meta,
			assertion:  base64.StdEncoding.EncodeToString([]byte(`<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"/>`)),
			wantSubstr: "signature missing",
		},
		{
			name:     "missing DigestValue fail closed",
			metadata: meta,
			assertion: base64.StdEncoding.EncodeToString([]byte(
				`<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">` +
					`<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#">` +
					`<ds:SignedInfo></ds:SignedInfo>` +
					`<ds:SignatureValue>AA==</ds:SignatureValue>` +
					`</ds:Signature></saml:Assertion>`,
			)),
			wantSubstr: "DigestValue missing",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifySAMLAssertionAt(tc.metadata, tc.assertion, now)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSubstr)
			}
			if _, ok := err.(*Error); !ok {
				t.Fatalf("expected federation.Error, got %T", err)
			}
		})
	}
}

func TestVerifySAMLAssertionLabHappyAndAudienceTime(t *testing.T) {
	meta, key := mustSAMLIdP(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	entityID := "https://idp.example/"

	good := mustLabSAMLAssertion(t, key, entityID, now.Add(-time.Minute), now.Add(time.Hour))
	if err := VerifySAMLAssertionAt(meta, good, now); err != nil {
		t.Fatalf("expected Allow path: %v", err)
	}

	expired := mustLabSAMLAssertion(t, key, entityID, now.Add(-2*time.Hour), now.Add(-time.Hour))
	if err := VerifySAMLAssertionAt(meta, expired, now); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired, got %v", err)
	}

	badAud := mustLabSAMLAssertion(t, key, "https://other.example/", now.Add(-time.Minute), now.Add(time.Hour))
	if err := VerifySAMLAssertionAt(meta, badAud, now); err == nil || !strings.Contains(err.Error(), "Audience mismatch") {
		t.Fatalf("expected audience mismatch, got %v", err)
	}
}

func TestParseJWTUnverifiedGolden(t *testing.T) {
	// header {"alg":"none"} payload {"iss":"https://issuer.example","aud":"client-a"}
	token := "eyJhbGciOiJub25lIn0.eyJpc3MiOiJodHRwczovL2lzc3Vlci5leGFtcGxlIiwiYXVkIjoiY2xpZW50LWEifQ."
	iss, aud, err := ParseJWTUnverified(token)
	if err != nil {
		t.Fatalf("ParseJWTUnverified: %v", err)
	}
	if iss != "https://issuer.example" {
		t.Fatalf("iss=%q", iss)
	}
	if len(aud) != 1 || aud[0] != "client-a" {
		t.Fatalf("aud=%v", aud)
	}

	if _, _, err := ParseJWTUnverified("not-a-jwt"); err == nil {
		t.Fatal("expected malformed JWT error")
	}
}

func mustSAMLIdP(t *testing.T) (metadataXML string, key *rsa.PrivateKey) {
	t.Helper()
	var err error
	key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "noctaxris-lab-idp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	certB64 := base64.StdEncoding.EncodeToString(der)
	metadataXML = `<?xml version="1.0"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example/">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <KeyDescriptor use="signing">
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data>
          <X509Certificate>` + certB64 + `</X509Certificate>
        </X509Data>
      </KeyInfo>
    </KeyDescriptor>
  </IDPSSODescriptor>
</EntityDescriptor>`
	return metadataXML, key
}

func mustLabSAMLAssertion(t *testing.T, key *rsa.PrivateKey, audience string, notBefore, notOnOrAfter time.Time) string {
	t.Helper()
	nb := notBefore.UTC().Format(time.RFC3339)
	na := notOnOrAfter.UTC().Format(time.RFC3339)
	// Digest covers Assertion with Signature removed (lab subset; not exclusive C14N).
	stripped := `<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" ID="_assert1">` +
		`<saml:Conditions NotBefore="` + nb + `" NotOnOrAfter="` + na + `">` +
		`<saml:AudienceRestriction><saml:Audience>` + audience + `</saml:Audience></saml:AudienceRestriction>` +
		`</saml:Conditions>` +
		`</saml:Assertion>`
	digest := sha256.Sum256([]byte(stripped))
	digestB64 := base64.StdEncoding.EncodeToString(digest[:])
	signedInfo := `<ds:SignedInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#">` +
		`<ds:Reference><ds:DigestValue>` + digestB64 + `</ds:DigestValue></ds:Reference>` +
		`</ds:SignedInfo>`
	sum := sha256.Sum256([]byte(signedInfo))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15: %v", err)
	}
	assertion := `<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" ID="_assert1">` +
		`<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#">` +
		signedInfo +
		`<ds:SignatureValue>` + base64.StdEncoding.EncodeToString(sig) + `</ds:SignatureValue>` +
		`</ds:Signature>` +
		`<saml:Conditions NotBefore="` + nb + `" NotOnOrAfter="` + na + `">` +
		`<saml:AudienceRestriction><saml:Audience>` + audience + `</saml:Audience></saml:AudienceRestriction>` +
		`</saml:Conditions>` +
		`</saml:Assertion>`
	return base64.StdEncoding.EncodeToString([]byte(assertion))
}
