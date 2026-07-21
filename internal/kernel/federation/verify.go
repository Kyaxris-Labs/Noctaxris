package federation

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwksfetch"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
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
// Lab subset (fail-closed): RSA verify of SignedInfo, Reference DigestValue over the
// Assertion with Signature removed, Conditions time window, and AudienceRestriction
// matching the metadata entityID. Exclusive C14N is not implemented; assertions must
// use this lab digest shape.
func VerifySAMLAssertion(metadataXML, samlAssertionB64 string) error {
	return VerifySAMLAssertionAt(metadataXML, samlAssertionB64, time.Now().UTC())
}

// VerifySAMLAssertionAt is VerifySAMLAssertion with an injectable clock for tests.
func VerifySAMLAssertionAt(metadataXML, samlAssertionB64 string, now time.Time) error {
	if strings.TrimSpace(metadataXML) == "" {
		return newError(CodeIdPNotConfigured, "IdP not configured")
	}
	entityID, certs, err := extractMetadata(metadataXML)
	if err != nil || len(certs) == 0 {
		return newError(CodeInvalidIdentityToken, "SAML IdP metadata has no usable certificates")
	}
	if strings.TrimSpace(entityID) == "" {
		return newError(CodeInvalidIdentityToken, "SAML IdP metadata missing entityID")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(samlAssertionB64))
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(samlAssertionB64))
		if err != nil {
			return newError(CodeInvalidIdentityToken, "SAML assertion is not valid base64")
		}
	}
	signedInfo, signatureValue, digestValue, err := extractSAMLSignature(raw)
	if err != nil {
		return newError(CodeInvalidIdentityToken, "SAML assertion signature missing or malformed")
	}
	if strings.TrimSpace(digestValue) == "" {
		return newError(CodeInvalidIdentityToken, "SAML assertion Reference DigestValue missing")
	}
	sig, err := base64.StdEncoding.DecodeString(signatureValue)
	if err != nil {
		return newError(CodeInvalidIdentityToken, "SAML signature value is invalid")
	}
	sum := sha256.Sum256(signedInfo)
	sigOK := false
	for _, cert := range certs {
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			continue
		}
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err == nil {
			sigOK = true
			break
		}
	}
	if !sigOK {
		return newError(CodeInvalidIdentityToken, "SAML assertion signature verification failed")
	}
	if err := verifySAMLReferenceDigest(raw, digestValue); err != nil {
		return err
	}
	if err := validateSAMLConditions(raw, entityID, now); err != nil {
		return err
	}
	return nil
}

type entityDescriptor struct {
	EntityID         string `xml:"entityID,attr"`
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

func extractMetadata(metadataXML string) (entityID string, certs []*x509.Certificate, err error) {
	var ed entityDescriptor
	if err := xml.Unmarshal([]byte(metadataXML), &ed); err != nil {
		return "", nil, err
	}
	certs, err = extractCertsFromMetadata(metadataXML)
	if err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(ed.EntityID), certs, nil
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

func extractSAMLSignature(assertion []byte) (signedInfo []byte, signatureValue, digestValue string, err error) {
	const siStart = "<ds:SignedInfo"
	const siStartAlt = "<SignedInfo"
	const siEnd = "</ds:SignedInfo>"
	const siEndAlt = "</SignedInfo>"
	const svStart = "<ds:SignatureValue"
	const svStartAlt = "<SignatureValue"
	const dvStart = "<ds:DigestValue"
	const dvStartAlt = "<DigestValue"

	raw := string(assertion)
	start, endTag := findTag(raw, siStart, siEnd)
	if start < 0 {
		start, endTag = findTag(raw, siStartAlt, siEndAlt)
	}
	if start < 0 {
		return nil, "", "", fmt.Errorf("SignedInfo not found")
	}
	end := strings.Index(raw[start:], endTag)
	if end < 0 {
		return nil, "", "", fmt.Errorf("SignedInfo end not found")
	}
	signedInfo = []byte(raw[start : start+end+len(endTag)])

	svIdx := strings.Index(raw, svStart)
	tagLen := len(svStart)
	if svIdx < 0 {
		svIdx = strings.Index(raw, svStartAlt)
		tagLen = len(svStartAlt)
	}
	if svIdx < 0 {
		return nil, "", "", fmt.Errorf("SignatureValue not found")
	}
	rest := raw[svIdx+tagLen:]
	gt := strings.Index(rest, ">")
	if gt < 0 {
		return nil, "", "", fmt.Errorf("SignatureValue open tag")
	}
	rest = rest[gt+1:]
	closeIdx := strings.Index(rest, "</")
	if closeIdx < 0 {
		return nil, "", "", fmt.Errorf("SignatureValue close")
	}
	signatureValue = strings.TrimSpace(rest[:closeIdx])

	digestValue = extractXMLText(raw, dvStart, "</ds:DigestValue>")
	if digestValue == "" {
		digestValue = extractXMLText(raw, dvStartAlt, "</DigestValue>")
	}
	return signedInfo, signatureValue, digestValue, nil
}

func extractXMLText(raw, startTag, endTag string) string {
	idx := strings.Index(raw, startTag)
	if idx < 0 {
		return ""
	}
	rest := raw[idx+len(startTag):]
	gt := strings.Index(rest, ">")
	if gt < 0 {
		return ""
	}
	rest = rest[gt+1:]
	closeIdx := strings.Index(rest, endTag)
	if closeIdx < 0 {
		// tolerate unprefixed close
		alt := strings.TrimPrefix(endTag, "</ds:")
		alt = "</" + alt
		closeIdx = strings.Index(rest, alt)
		if closeIdx < 0 {
			return ""
		}
	}
	return strings.TrimSpace(rest[:closeIdx])
}

func findTag(raw, startTag, endTag string) (start int, end string) {
	start = strings.Index(raw, startTag)
	return start, endTag
}

func verifySAMLReferenceDigest(assertion []byte, digestValueB64 string) error {
	want, err := base64.StdEncoding.DecodeString(strings.TrimSpace(digestValueB64))
	if err != nil || len(want) == 0 {
		return newError(CodeInvalidIdentityToken, "SAML assertion DigestValue is invalid")
	}
	stripped := stripSAMLSignature(string(assertion))
	sum := sha256.Sum256([]byte(stripped))
	if subtle.ConstantTimeCompare(sum[:], want) != 1 {
		return newError(CodeInvalidIdentityToken, "SAML assertion Reference digest mismatch")
	}
	return nil
}

func stripSAMLSignature(raw string) string {
	for _, pair := range [][2]string{
		{"<ds:Signature", "</ds:Signature>"},
		{"<Signature", "</Signature>"},
	} {
		start := strings.Index(raw, pair[0])
		if start < 0 {
			continue
		}
		end := strings.Index(raw[start:], pair[1])
		if end < 0 {
			continue
		}
		end = start + end + len(pair[1])
		return raw[:start] + raw[end:]
	}
	return raw
}

func validateSAMLConditions(assertion []byte, entityID string, now time.Time) error {
	raw := string(assertion)
	notBefore := extractXMLAttrOrElement(raw, "NotBefore")
	notOnOrAfter := extractXMLAttrOrElement(raw, "NotOnOrAfter")
	if notOnOrAfter == "" {
		return newError(CodeInvalidIdentityToken, "SAML assertion Conditions NotOnOrAfter missing")
	}
	after, err := time.Parse(time.RFC3339, notOnOrAfter)
	if err != nil {
		after, err = time.Parse(time.RFC3339Nano, notOnOrAfter)
		if err != nil {
			return newError(CodeInvalidIdentityToken, "SAML assertion NotOnOrAfter invalid")
		}
	}
	if !now.Before(after) {
		return newError(CodeInvalidIdentityToken, "SAML assertion expired")
	}
	if notBefore != "" {
		before, err := time.Parse(time.RFC3339, notBefore)
		if err != nil {
			before, err = time.Parse(time.RFC3339Nano, notBefore)
			if err != nil {
				return newError(CodeInvalidIdentityToken, "SAML assertion NotBefore invalid")
			}
		}
		if now.Before(before) {
			return newError(CodeInvalidIdentityToken, "SAML assertion not yet valid")
		}
	}
	audiences := extractAllXMLText(raw, "<saml:Audience>", "</saml:Audience>")
	if len(audiences) == 0 {
		audiences = extractAllXMLText(raw, "<Audience>", "</Audience>")
	}
	if len(audiences) == 0 {
		return newError(CodeInvalidIdentityToken, "SAML assertion Audience missing")
	}
	if !containsString(audiences, entityID) {
		return newError(CodeInvalidIdentityToken, "SAML assertion Audience mismatch")
	}
	return nil
}

func extractXMLAttrOrElement(raw, name string) string {
	// Attribute form: NotOnOrAfter="..."
	attr := name + `="`
	if idx := strings.Index(raw, attr); idx >= 0 {
		rest := raw[idx+len(attr):]
		end := strings.Index(rest, `"`)
		if end > 0 {
			return rest[:end]
		}
	}
	// Element form: <NotOnOrAfter>...</NotOnOrAfter>
	return extractXMLText(raw, "<"+name, "</"+name+">")
}

func extractAllXMLText(raw, startTag, endTag string) []string {
	var out []string
	for {
		idx := strings.Index(raw, startTag)
		if idx < 0 {
			break
		}
		rest := raw[idx+len(startTag):]
		// startTag may already include '>'; otherwise skip attributes to '>'.
		if !strings.HasSuffix(startTag, ">") {
			gt := strings.Index(rest, ">")
			if gt < 0 {
				break
			}
			rest = rest[gt+1:]
		}
		closeIdx := strings.Index(rest, endTag)
		if closeIdx < 0 {
			break
		}
		out = append(out, strings.TrimSpace(rest[:closeIdx]))
		raw = rest[closeIdx+len(endTag):]
	}
	return out
}

// OIDCClaims are verified JWT claims used for web identity.
type OIDCClaims struct {
	Issuer   string
	Subject  string
	Audience []string
}

// VerifyWebIdentityJWT validates a JWT against JWKS fetched from issuer.
// Requires matching iss and aud (clientID). Fail-closed on any error.
// Remote JWKS requires NOCTAXRIS_ALLOW_REMOTE_JWKS with a public host allowlist (SSRF fail-closed).
func VerifyWebIdentityJWT(token, issuerURL, clientID string, httpClient *http.Client) (OIDCClaims, error) {
	issuerURL = strings.TrimRight(issuerURL, "/")
	if issuerURL == "" || clientID == "" || token == "" {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "OIDC token validation inputs incomplete")
	}
	if jwksfetch.IsLabCognitoIssuer(issuerURL) {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "lab Cognito issuer requires in-process JWKS path")
	}
	body, err := jwksfetch.FetchRemoteJWKS(issuerURL, httpClient)
	if err != nil {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWKS unreachable; assertion rejected")
	}

	claimsMap, err := jwtutil.VerifyCompactRS256(token, body)
	if err != nil {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT signature verification failed")
	}
	return validateWebIdentityClaims(claimsMap, issuerURL, clientID, time.Now().UTC())
}

func validateWebIdentityClaims(claimsMap map[string]any, issuerURL, clientID string, now time.Time) (OIDCClaims, error) {
	iss := jwtutil.ClaimString(claimsMap, "iss")
	sub := jwtutil.ClaimString(claimsMap, "sub")
	if !issuerMatches(iss, issuerURL) {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT iss mismatch")
	}
	auds := normalizeAud(claimsMap["aud"])
	if !containsString(auds, clientID) {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT aud mismatch")
	}
	if jwtutil.ClaimExpired(claimsMap, now) {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT exp missing or expired")
	}
	if jwtutil.ClaimNotYetValid(claimsMap, now) {
		return OIDCClaims{}, newError(CodeInvalidIdentityToken, "JWT nbf not yet valid")
	}
	return OIDCClaims{Issuer: iss, Subject: sub, Audience: auds}, nil
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
