package server_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSTSAssumeRoleWithSAMLSuccess(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	meta, key := mustLabSAMLMetadata(t)
	createIdP := iamForm(t, handler, "Action=CreateSAMLProvider&Version=2010-05-08&Name=LabSAML&SAMLMetadataDocument="+url.QueryEscape(meta), now)
	if createIdP.Code != http.StatusOK {
		t.Fatalf("CreateSAMLProvider %d %s", createIdP.Code, createIdP.Body.String())
	}
	principalARN := xmlTag(t, createIdP.Body.String(), "SAMLProviderArn")

	trust := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Federated":"` + principalARN + `"},"Action":"sts:AssumeRoleWithSAML"}]}`)
	role := iamForm(t, handler, "Action=CreateRole&Version=2010-05-08&RoleName=saml-lab-role&AssumeRolePolicyDocument="+trust, now)
	if role.Code != http.StatusOK {
		t.Fatalf("CreateRole %d %s", role.Code, role.Body.String())
	}
	roleARN := "arn:aws:iam::" + testAccountID + ":role/saml-lab-role"

	assertion := mustLabSAMLAssertionB64(t, key, "https://idp.example/", now.Add(-time.Minute), now.Add(time.Hour))
	body := url.Values{
		"Action":          {"AssumeRoleWithSAML"},
		"Version":         {"2011-06-15"},
		"RoleArn":         {roleARN},
		"PrincipalArn":    {principalARN},
		"SAMLAssertion":   {assertion},
		"RoleSessionName": {"saml-session"},
	}.Encode()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("AssumeRoleWithSAML %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AccessKeyId") || !strings.Contains(rec.Body.String(), "saml-lab-role") {
		t.Fatalf("unexpected AssumeRoleWithSAML body %q", rec.Body.String())
	}

	badBody := url.Values{"Action": {"AssumeRoleWithSAML"}, "Version": {"2011-06-15"}}.Encode()
	badReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(badBody))
	signHeader(t, badReq, []byte(badBody), testAccessKey, testSecret, testRegion, "sts", now)
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code == http.StatusOK {
		t.Fatal("missing params should fail")
	}

	badAssertBody := url.Values{
		"Action": {"AssumeRoleWithSAML"}, "Version": {"2011-06-15"},
		"RoleArn": {roleARN}, "PrincipalArn": {principalARN}, "SAMLAssertion": {"not-base64!!"},
	}.Encode()
	badAssertReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(badAssertBody))
	signHeader(t, badAssertReq, []byte(badAssertBody), testAccessKey, testSecret, testRegion, "sts", now)
	badAssertRec := httptest.NewRecorder()
	handler.ServeHTTP(badAssertRec, badAssertReq)
	if badAssertRec.Code == http.StatusOK {
		t.Fatal("bad assertion should fail")
	}
}

func mustLabSAMLMetadata(t *testing.T) (metadataXML string, key *rsa.PrivateKey) {
	t.Helper()
	var err error
	key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
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

func mustLabSAMLAssertionB64(t *testing.T, key *rsa.PrivateKey, audience string, notBefore, notOnOrAfter time.Time) string {
	t.Helper()
	nb := notBefore.UTC().Format(time.RFC3339)
	na := notOnOrAfter.UTC().Format(time.RFC3339)
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
		t.Fatal(err)
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
