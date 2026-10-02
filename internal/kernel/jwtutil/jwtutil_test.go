package jwtutil_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
	jose "github.com/go-jose/go-jose/v4"
)

func TestSignAndVerifyRS256(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "lab-kid-1"
	payload, _ := json.Marshal(map[string]any{
		"iss": "http://127.0.0.1:4566/cognito-idp/us-east-1/pool",
		"sub": "user-1",
		"aud": "client-1",
		"exp": time.Now().UTC().Add(time.Hour).Unix(),
	})
	token, err := jwtutil.SignRS256(payload, key, kid)
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := jwtutil.MarshalJWKS(&key.PublicKey, kid)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwtutil.VerifyCompactRS256(token, jwks)
	if err != nil {
		t.Fatal(err)
	}
	if jwtutil.ClaimString(claims, "sub") != "user-1" {
		t.Fatalf("sub = %v", claims["sub"])
	}
	if jwtutil.ClaimExpired(claims, time.Now()) {
		t.Fatal("token should not be expired")
	}
}

func TestClaimNotYetValid(t *testing.T) {
	now := time.Now().UTC()
	if jwtutil.ClaimNotYetValid(map[string]any{"exp": now.Add(time.Hour).Unix()}, now) {
		t.Fatal("missing nbf should be valid")
	}
	if !jwtutil.ClaimNotYetValid(map[string]any{
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(time.Hour).Unix(),
	}, now) {
		t.Fatal("future nbf should be not yet valid")
	}
	if jwtutil.ClaimNotYetValid(map[string]any{
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(-time.Minute).Unix(),
	}, now) {
		t.Fatal("past nbf should be valid")
	}
	if !jwtutil.ClaimNotYetValid(map[string]any{"nbf": "bad"}, now) {
		t.Fatal("non-numeric nbf should fail closed")
	}
}

func TestVerifyRejectsHS256(t *testing.T) {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte("0123456789abcdef0123456789abcdef")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := signer.Sign([]byte(`{"sub":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	token, err := obj.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := jwtutil.MarshalJWKS(&key.PublicKey, "k")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jwtutil.VerifyCompactRS256(token, jwks); err == nil {
		t.Fatal("expected HS256 reject")
	}
}

func TestSignAndVerifyHS256(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	payload, _ := json.Marshal(map[string]any{"sub": "lab-user", "exp": time.Now().UTC().Add(time.Hour).Unix()})
	token, err := jwtutil.SignHS256(payload, key)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwtutil.VerifyCompactHS256(token, key)
	if err != nil {
		t.Fatal(err)
	}
	if jwtutil.ClaimString(claims, "sub") != "lab-user" {
		t.Fatalf("sub=%v", claims["sub"])
	}
	if _, err := jwtutil.VerifyCompactHS256(token, []byte("wrong-key-0123456789abcdef012345")); err == nil {
		t.Fatal("wrong key must fail")
	}
	if _, err := jwtutil.SignHS256(payload, nil); err == nil {
		t.Fatal("empty key must fail")
	}
	if _, err := jwtutil.VerifyCompactHS256("", key); err == nil {
		t.Fatal("empty token must fail")
	}
}

func TestPeekUnverifiedClaims(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	payload, _ := json.Marshal(map[string]any{"iss": "http://lab/issuer", "aud": "client-1"})
	token, err := jwtutil.SignHS256(payload, key)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwtutil.PeekUnverifiedClaims(token)
	if err != nil {
		t.Fatal(err)
	}
	if jwtutil.ClaimString(claims, "iss") != "http://lab/issuer" {
		t.Fatalf("iss=%v", claims["iss"])
	}
	if _, err := jwtutil.PeekUnverifiedClaims("not-a-jwt"); err == nil {
		t.Fatal("malformed must fail")
	}
}

func TestVerifyRejectsBadSignature(t *testing.T) {
	keyA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwtutil.SignRS256([]byte(`{"sub":"x","exp":9999999999}`), keyA, "kid")
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := jwtutil.MarshalJWKS(&keyB.PublicKey, "kid")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jwtutil.VerifyCompactRS256(token, jwks); err == nil {
		t.Fatal("expected signature failure")
	}
}
