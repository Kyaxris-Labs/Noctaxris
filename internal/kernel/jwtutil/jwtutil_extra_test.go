package jwtutil_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
)

func TestParseJWKSBoundaries(t *testing.T) {
	if _, err := jwtutil.ParseJWKS(nil); err == nil {
		t.Fatal("empty JWKS must fail")
	}
	if _, err := jwtutil.ParseJWKS([]byte(`{`)); err == nil {
		t.Fatal("malformed JWKS must fail")
	}
	if _, err := jwtutil.ParseJWKS([]byte(`{"keys":[]}`)); err == nil {
		t.Fatal("empty keys must fail")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwtutil.MarshalJWKS(&key.PublicKey, "kid-1")
	if err != nil {
		t.Fatal(err)
	}
	set, err := jwtutil.ParseJWKS(raw)
	if err != nil || len(set.Keys) != 1 || set.Keys[0].KeyID != "kid-1" {
		t.Fatalf("parse ok got err=%v set=%v", err, set)
	}
}

func TestSignRS256RejectsNilKeyAndEmptyKid(t *testing.T) {
	if _, err := jwtutil.SignRS256([]byte(`{}`), nil, "kid"); err == nil {
		t.Fatal("nil key must fail")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jwtutil.SignRS256([]byte(`{}`), key, ""); err == nil {
		t.Fatal("empty kid must fail")
	}
}

func TestSignAndVerifyHS256RoundTripAndRejects(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	payload, _ := json.Marshal(map[string]any{"sub": "hs-user", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := jwtutil.SignHS256(payload, nil); err == nil {
		t.Fatal("empty hmac key must fail")
	}
	token, err := jwtutil.SignHS256(payload, key)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwtutil.VerifyCompactHS256(token, key)
	if err != nil {
		t.Fatal(err)
	}
	if jwtutil.ClaimString(claims, "sub") != "hs-user" {
		t.Fatalf("sub=%v", claims["sub"])
	}
	if _, err := jwtutil.VerifyCompactHS256("", key); err == nil {
		t.Fatal("empty token must fail")
	}
	if _, err := jwtutil.VerifyCompactHS256(token, nil); err == nil {
		t.Fatal("empty verify key must fail")
	}
	if _, err := jwtutil.VerifyCompactHS256(token, []byte("wrong-key-wrong-key-wrong-key!!")); err == nil {
		t.Fatal("wrong hmac must fail")
	}
	if _, err := jwtutil.VerifyCompactRS256("", []byte(`{"keys":[]}`)); err == nil {
		t.Fatal("empty RS256 token must fail")
	}
}

func TestPeekUnverifiedClaimsAndClaimHelpers(t *testing.T) {
	if _, err := jwtutil.PeekUnverifiedClaims("not-a-jwt"); err == nil {
		t.Fatal("malformed must fail")
	}
	if _, err := jwtutil.PeekUnverifiedClaims("a.!!!.c"); err == nil {
		t.Fatal("bad payload b64 must fail")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{
		"sub": "peek",
		"n":   float64(42),
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(-time.Minute).Unix(),
	})
	token, err := jwtutil.SignRS256(payload, key, "kid")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwtutil.PeekUnverifiedClaims(token)
	if err != nil {
		t.Fatal(err)
	}
	if jwtutil.ClaimString(claims, "sub") != "peek" {
		t.Fatalf("sub=%v", claims["sub"])
	}
	if jwtutil.ClaimString(claims, "n") != "42" {
		t.Fatalf("float claim=%q", jwtutil.ClaimString(claims, "n"))
	}
	if jwtutil.ClaimString(nil, "sub") != "" || jwtutil.ClaimString(claims, "missing") != "" {
		t.Fatal("missing claim must be empty")
	}
	if jwtutil.ClaimString(map[string]any{"x": true}, "x") == "" {
		t.Fatal("bool claim should stringify")
	}
	if jwtutil.ClaimExpired(claims, now) {
		t.Fatal("fresh token must not be expired")
	}
	if !jwtutil.ClaimExpired(map[string]any{"exp": now.Add(-time.Hour).Unix()}, now) {
		t.Fatal("past exp must be expired")
	}
	if !jwtutil.ClaimExpired(map[string]any{}, now) {
		t.Fatal("missing exp must fail closed")
	}
	if !jwtutil.ClaimExpired(map[string]any{"exp": "bad"}, now) {
		t.Fatal("non-numeric exp must fail closed")
	}
	if jwtutil.ClaimNotYetValid(nil, now) {
		t.Fatal("nil claims nbf optional")
	}
	if !jwtutil.ClaimNotYetValid(map[string]any{"nbf": json.Number("x")}, now) {
		t.Fatal("bad json.Number nbf must fail closed")
	}
	if jwtutil.ClaimNotYetValid(map[string]any{"nbf": int64(now.Add(-time.Minute).Unix())}, now) {
		t.Fatal("int64 past nbf valid")
	}
	if jwtutil.ClaimNotYetValid(map[string]any{"nbf": int(now.Add(-time.Minute).Unix())}, now) {
		t.Fatal("int past nbf valid")
	}
	parts := strings.Split(token, ".")
	badClaims := parts[0] + "." + "e30" + "." + parts[2] // "{}"
	if _, err := jwtutil.PeekUnverifiedClaims(badClaims); err != nil {
		t.Fatalf("empty object claims should decode: %v", err)
	}
}

func TestVerifyCompactRS256RejectsBadClaimsJSON(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwtutil.SignRS256([]byte(`not-json`), key, "kid")
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := jwtutil.MarshalJWKS(&key.PublicKey, "kid")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jwtutil.VerifyCompactRS256(token, jwks); err == nil {
		t.Fatal("non-object payload must fail claims parse")
	}
}
