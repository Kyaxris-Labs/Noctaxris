// Package jwtutil provides shared RS256 JWT/JWS helpers for Noctaxris.
//
// Allowed signature algorithms are restricted to RS256 (Cognito and lab OIDC).
// Callers must not pass other algorithms to ParseSigned.
package jwtutil

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// AllowedRS256 is the only signature algorithm accepted for lab JWT verify.
var AllowedRS256 = []jose.SignatureAlgorithm{jose.RS256}

// ParseJWKS unmarshals a JWKS JSON document.
func ParseJWKS(jwksJSON []byte) (*jose.JSONWebKeySet, error) {
	if len(jwksJSON) == 0 {
		return nil, fmt.Errorf("jwks: empty")
	}
	var jwks jose.JSONWebKeySet
	if err := json.Unmarshal(jwksJSON, &jwks); err != nil {
		return nil, fmt.Errorf("jwks: parse: %w", err)
	}
	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("jwks: no keys")
	}
	return &jwks, nil
}

// VerifyCompactRS256 parses a compact JWS with RS256 only, verifies against JWKS JSON,
// and returns the payload as a generic claims map. Fail-closed on any error.
func VerifyCompactRS256(token string, jwksJSON []byte) (map[string]any, error) {
	if token == "" {
		return nil, fmt.Errorf("jwt: empty token")
	}
	jwks, err := ParseJWKS(jwksJSON)
	if err != nil {
		return nil, err
	}
	jws, err := jose.ParseSigned(token, AllowedRS256)
	if err != nil {
		return nil, fmt.Errorf("jwt: parse: %w", err)
	}
	payload, err := jws.Verify(jwks)
	if err != nil {
		return nil, fmt.Errorf("jwt: verify: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("jwt: claims: %w", err)
	}
	return claims, nil
}

// SignRS256 signs payload as a compact JWS (RS256) with kid in the protected header.
func SignRS256(payload []byte, key *rsa.PrivateKey, kid string) (string, error) {
	if key == nil {
		return "", fmt.Errorf("jwt: nil signing key")
	}
	if kid == "" {
		return "", fmt.Errorf("jwt: kid required")
	}
	jwk := jose.JSONWebKey{
		Key:       key,
		KeyID:     kid,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}
	opts := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", kid)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: &jwk}, opts)
	if err != nil {
		return "", fmt.Errorf("jwt: signer: %w", err)
	}
	obj, err := signer.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("jwt: sign: %w", err)
	}
	compact, err := obj.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("jwt: serialize: %w", err)
	}
	return compact, nil
}

// PublicJWK builds a public JWKS key entry for an RSA signing key.
func PublicJWK(pub *rsa.PublicKey, kid string) jose.JSONWebKey {
	return jose.JSONWebKey{
		Key:       pub,
		KeyID:     kid,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}
}

// MarshalJWKS encodes a single public key as a JWKS document.
func MarshalJWKS(pub *rsa.PublicKey, kid string) ([]byte, error) {
	set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{PublicJWK(pub, kid)}}
	return json.Marshal(set)
}

// ClaimString returns a string claim or empty.
func ClaimString(claims map[string]any, key string) string {
	if claims == nil {
		return ""
	}
	v, ok := claims[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%.0f", t)
	default:
		return fmt.Sprint(t)
	}
}

// ClaimExpired reports whether exp (unix seconds) is in the past relative to now.
// Missing or non-numeric exp is treated as expired (fail closed).
func ClaimExpired(claims map[string]any, now time.Time) bool {
	v, ok := claims["exp"]
	if !ok {
		return true
	}
	var exp int64
	switch t := v.(type) {
	case float64:
		exp = int64(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return true
		}
		exp = n
	case int64:
		exp = t
	case int:
		exp = int64(t)
	default:
		return true
	}
	return now.UTC().Unix() >= exp
}
