package federation

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	jose "gopkg.in/go-jose/go-jose.v2"
)

// Error codes matching STS federation failures.
const (
	CodeInvalidIdentityToken = "InvalidIdentityToken"
	CodeAccessDenied         = "AccessDenied"
	CodeIdPNotConfigured     = "AccessDenied"
)

// Error is a distinguishable federation failure.
type Error struct {
	code    string
	message string
}

func (e *Error) Error() string {
	if e.message == "" {
		return e.code
	}
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

// Code returns the AWS-style error code.
func (e *Error) Code() string { return e.code }

func newError(code, message string) *Error {
	return &Error{code: code, message: message}
}

// VerifySAMLAssertion validates a base64-encoded SAML assertion against IdP metadata XML.
// Fail-closed: missing certs, bad signature, or parse errors deny.
func VerifySAMLAssertion(metadataXML, samlAssertionB64 string) error {
	if strings.TrimSpace(metadataXML) == "" {
		return newError(CodeIdPNotConfigured, "IdP not configured")
	}
	certs, err := extractCertsFromMetadata(metadataXML)
	if err != nil || len(certs) == 0 {
		return newError(CodeInvalidIdentityToken, "SAML IdP metadata has no usable certificates")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(samlAssertionB64))
	if err != nil {
		// try URL-safe / raw
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(samlAssertionB64))
		if err != nil {
			return newError(CodeInvalidIdentityToken, "SAML assertion is not valid base64")
		}
	}
	signedInfo, signatureValue, err := extractSAMLSignature(raw)
	if err != nil {
		return newError(CodeInvalidIdentityToken, "SAML assertion signature missing or malformed")
	}
	sig, err := base64.StdEncoding.DecodeString(signatureValue)
	if err != nil {
		return newError(CodeInvalidIdentityToken, "SAML signature value is invalid")
	}
	sum := sha256.Sum256(signedInfo)
	for _, cert := range certs {
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			continue
		}
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err == nil {
			return nil
		}
	}
	return newError(CodeInvalidIdentityToken, "SAML assertion signature verification failed")
}

type entityDescriptor struct {
	IDPSSODescriptor *struct {
		KeyDescriptors []struct {
			KeyInfo struct {
				X509Data struct {
					X509Certificate string `xml:"X509Certificate"`
				} `xml:"X509Data"`
			} `xml:"KeyInfo"`
		} `xml:"KeyDescriptor"`
	} `xml:"IDPSSODescriptor"`
}

func extractCertsFromMetadata(metadataXML string) ([]*x509.Certificate, error) {
	var ed entityDescriptor
	if err := xml.Unmarshal([]byte(metadataXML), &ed); err != nil {
		return nil, err
	}
	if ed.IDPSSODescriptor == nil {
		return nil, fmt.Errorf("no IDPSSODescriptor")
	}
	var certs []*x509.Certificate
	for _, kd := range ed.IDPSSODescriptor.KeyDescriptors {
		b64 := strings.TrimSpace(kd.KeyInfo.X509Data.X509Certificate)
		if b64 == "" {
			continue
		}
		der, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			continue
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			// try PEM-wrapped
			block, _ := pem.Decode([]byte("-----BEGIN CERTIFICATE-----\n" + b64 + "\n-----END CERTIFICATE-----"))
			if block == nil {
				continue
			}
			cert, err = x509.ParseCertificate(block.Bytes)
			if err != nil {
				continue
			}
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

func extractSAMLSignature(assertion []byte) (signedInfo []byte, signatureValue string, err error) {
	// Minimal XML Signature extraction: locate SignedInfo and SignatureValue.
	const siStart = "<ds:SignedInfo"
	const siStartAlt = "<SignedInfo"
	const siEnd = "</ds:SignedInfo>"
	const siEndAlt = "</SignedInfo>"
	const svStart = "<ds:SignatureValue"
	const svStartAlt = "<SignatureValue"

	raw := string(assertion)
	start, endTag := findTag(raw, siStart, siEnd)
	if start < 0 {
		start, endTag = findTag(raw, siStartAlt, siEndAlt)
	}
	if start < 0 {
		return nil, "", fmt.Errorf("SignedInfo not found")
	}
	end := strings.Index(raw[start:], endTag)
	if end < 0 {
		return nil, "", fmt.Errorf("SignedInfo end not found")
	}
	signedInfo = []byte(raw[start : start+end+len(endTag)])

	svIdx := strings.Index(raw, svStart)
	tagLen := len(svStart)
	if svIdx < 0 {
		svIdx = strings.Index(raw, svStartAlt)
		tagLen = len(svStartAlt)
	}
	if svIdx < 0 {
		return nil, "", fmt.Errorf("SignatureValue not found")
	}
	rest := raw[svIdx+tagLen:]
	gt := strings.Index(rest, ">")
	if gt < 0 {
		return nil, "", fmt.Errorf("SignatureValue open tag")
	}
	rest = rest[gt+1:]
	closeIdx := strings.Index(rest, "</")
	if closeIdx < 0 {
		return nil, "", fmt.Errorf("SignatureValue close")
	}
	return signedInfo, strings.TrimSpace(rest[:closeIdx]), nil
}

func findTag(raw, startTag, endTag string) (start int, end string) {
	start = strings.Index(raw, startTag)
	return start, endTag
}

// OIDCClaims are verified JWT claims used for web identity.
type OIDCClaims struct {
	Issuer   string
	Subject  string
	Audience []string
}

// VerifyWebIdentityJWT validates a JWT against JWKS fetched from issuer.
// Requires matching iss and aud (clientID). Fail-closed on any error.
func VerifyWebIdentityJWT(token, issuerURL, clientID string, httpClient *http.Client) (OIDCClaims, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	issuerURL = strings.TrimRight(issuerURL, "/")
	if issuerURL == "" || clientID == "" || token == "" {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "OIDC token validation inputs incomplete")
	}

	jwksURL := issuerURL + "/.well-known/jwks.json"
	resp, err := httpClient.Get(jwksURL)
	if err != nil {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWKS unreachable; assertion rejected")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWKS unreachable; assertion rejected")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWKS read failed")
	}

	var jwks jose.JSONWebKeySet
	if err := json.Unmarshal(body, &jwks); err != nil {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWKS parse failed")
	}

	jws, err := jose.ParseSigned(token)
	if err != nil {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT parse failed")
	}
	var payload []byte
	var lastErr error
	for _, key := range jwks.Keys {
		payload, lastErr = jws.Verify(&key)
		if lastErr == nil {
			break
		}
	}
	if payload == nil {
		// also try RSA from n/e style if needed — Verify above covers JSONWebKey
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT signature verification failed")
	}

	var claims struct {
		Iss string `json:"iss"`
		Sub string `json:"sub"`
		Aud any    `json:"aud"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT claims parse failed")
	}
	if !issuerMatches(claims.Iss, issuerURL) {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT iss mismatch")
	}
	auds := normalizeAud(claims.Aud)
	if !containsString(auds, clientID) {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT aud mismatch")
	}
	return OIDCClaims{Issuer: claims.Iss, Subject: claims.Sub, Audience: auds}, nil
}

func issuerMatches(iss, configured string) bool {
	a := strings.TrimRight(strings.TrimSpace(iss), "/")
	b := strings.TrimRight(strings.TrimSpace(configured), "/")
	return strings.EqualFold(a, b)
}

func normalizeAud(aud any) []string {
	switch v := aud.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// ParseJWTUnverified extracts iss/aud without verifying signature (for IdP lookup only).
func ParseJWTUnverified(token string) (iss string, aud []string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", nil, fmt.Errorf("malformed JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, err
	}
	var claims struct {
		Iss string `json:"iss"`
		Aud any    `json:"aud"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", nil, err
	}
	return claims.Iss, normalizeAud(claims.Aud), nil
}
