package federation

import (
	"crypto/rand"
	"crypto/rsa"
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
	metadataWithCert := mustSAMLMetadataXML(t)

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
			metadata:   `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata"><IDPSSODescriptor></IDPSSODescriptor></EntityDescriptor>`,
			assertion:  base64.StdEncoding.EncodeToString([]byte("<Assertion/>")),
			wantSubstr: "no usable certificates",
		},
		{
			name:       "assertion not base64",
			metadata:   metadataWithCert,
			assertion:  "%%%not-base64%%%",
			wantSubstr: "not valid base64",
		},
		{
			name:       "assertion missing signature",
			metadata:   metadataWithCert,
			assertion:  base64.StdEncoding.EncodeToString([]byte(`<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"/>`)),
			wantSubstr: "signature missing",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifySAMLAssertion(tc.metadata, tc.assertion)
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

func mustSAMLMetadataXML(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
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
	return `<?xml version="1.0"?>
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
}
