package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestNormalizeJWTAudienceAndOverlap(t *testing.T) {
	if got := normalizeJWTAudience("a"); len(got) != 1 || got[0] != "a" {
		t.Fatalf("string aud=%v", got)
	}
	if got := normalizeJWTAudience(""); got != nil {
		t.Fatalf("empty string aud=%v", got)
	}
	if got := normalizeJWTAudience([]any{"a", "", 1, "b"}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("slice aud=%v", got)
	}
	if got := normalizeJWTAudience(42); got != nil {
		t.Fatalf("other aud=%v", got)
	}
	if !audienceOverlap([]string{"x", "y"}, []string{"y"}) {
		t.Fatal("expected overlap")
	}
	if audienceOverlap([]string{"x"}, []string{"z"}) {
		t.Fatal("unexpected overlap")
	}
	if !issuerEqual("https://ex.com/", "https://ex.com") {
		t.Fatal("issuerEqual trim")
	}
	if issuerEqual("https://a.com", "https://b.com") {
		t.Fatal("issuerEqual mismatch")
	}
}

func TestVerifyAPIGatewayJWTFailureBranches(t *testing.T) {
	s := &Server{}
	now := time.Now().UTC()
	if err := s.verifyAPIGatewayJWT("", "https://iss", []string{"aud"}, now, "access"); err == nil {
		t.Fatal("empty token")
	}
	if err := s.verifyAPIGatewayJWT("t", "", []string{"aud"}, now, "access"); err == nil {
		t.Fatal("empty issuer")
	}
	if err := s.verifyAPIGatewayJWT("t", "https://iss", nil, now, "access"); err == nil {
		t.Fatal("empty audience")
	}
	if err := s.verifyAPIGatewayJWT("t", "https://iss", []string{"aud"}, now); err == nil {
		t.Fatal("empty token_use allowlist")
	}
	if err := s.verifyAPIGatewayJWT("not.a.jwt", "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_missing", []string{"aud"}, now, "access"); err == nil {
		t.Fatal("expected jwks/verify failure")
	}
}

func TestCORSHelpersSuccessAndFailure(t *testing.T) {
	if !corsMethodAllowed([]string{"GET", "POST"}, "get") {
		t.Fatal("case-insensitive allow")
	}
	if !corsMethodAllowed([]string{"*"}, "DELETE") {
		t.Fatal("wildcard allow")
	}
	if corsMethodAllowed([]string{"GET"}, "POST") {
		t.Fatal("unexpected allow")
	}

	cors := store.APIGatewayCORS{
		AllowOrigins: []string{"https://lab.example"},
		AllowMethods: []string{"GET"},
		AllowHeaders: []string{"authorization"},
		MaxAge:       60,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1/x", nil)
	req.Header.Set("Origin", "https://lab.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	if !writeHTTPAPICORSPreflight(rec, req, cors) || rec.Code != http.StatusNoContent {
		t.Fatalf("preflight ok code=%d", rec.Code)
	}

	denyOrigin := httptest.NewRecorder()
	badOrigin := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1/x", nil)
	badOrigin.Header.Set("Origin", "https://evil.example")
	badOrigin.Header.Set("Access-Control-Request-Method", "GET")
	if !writeHTTPAPICORSPreflight(denyOrigin, badOrigin, cors) || denyOrigin.Code != http.StatusForbidden {
		t.Fatalf("deny origin code=%d", denyOrigin.Code)
	}

	denyMethod := httptest.NewRecorder()
	badMethod := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1/x", nil)
	badMethod.Header.Set("Origin", "https://lab.example")
	badMethod.Header.Set("Access-Control-Request-Method", "POST")
	if !writeHTTPAPICORSPreflight(denyMethod, badMethod, cors) || denyMethod.Code != http.StatusForbidden {
		t.Fatalf("deny method code=%d", denyMethod.Code)
	}

	skip := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/x", nil)
	if writeHTTPAPICORSPreflight(skip, getReq, cors) {
		t.Fatal("non-OPTIONS should skip")
	}

	hdr := httptest.NewRecorder()
	getReq.Header.Set("Origin", "https://lab.example")
	applyHTTPAPICORSHeaders(hdr, getReq, cors)
	if hdr.Header().Get("Access-Control-Allow-Origin") != "https://lab.example" {
		t.Fatalf("ACAO=%q", hdr.Header().Get("Access-Control-Allow-Origin"))
	}
	applyHTTPAPICORSHeaders(hdr, getReq, store.APIGatewayCORS{})
}

func TestWebSocketPathHelpers(t *testing.T) {
	if !isWebSocketAPILabPath("/ws-api/a1/$default/$connect") {
		t.Fatal("expected ws path")
	}
	if isWebSocketAPILabPath("/ws-api/a1/$default/hello") {
		t.Fatal("custom route key not lab path")
	}
	if isWebSocketAPILabPath("/http-api/a1/$default/x") {
		t.Fatal("http path")
	}
	apiID, stage, rk, ok := parseWebSocketAPILabPath("/ws-api/api1/prod/$disconnect")
	if !ok || apiID != "api1" || stage != "prod" || rk != "$disconnect" {
		t.Fatalf("parse ws got %s %s %s ok=%v", apiID, stage, rk, ok)
	}
	if _, _, _, ok := parseWebSocketAPILabPath("/ws-api/a/b/c"); ok {
		t.Fatal("bad route key")
	}
	if !isAPIGatewayConnectionsPath("/execute-api/a1/$default/@connections/c1") {
		t.Fatal("expected connections path")
	}
	if isAPIGatewayConnectionsPath("/execute-api/a1/$default/connections/c1") {
		t.Fatal("missing @")
	}
	a, s, c, ok := parseAPIGatewayConnectionsPath("/execute-api/api/stg/@connections/cid")
	if !ok || a != "api" || s != "stg" || c != "cid" {
		t.Fatalf("parse connections %s %s %s ok=%v", a, s, c, ok)
	}
	if _, _, _, ok := parseAPIGatewayConnectionsPath("/execute-api/a/b/@connections"); ok {
		t.Fatal("short connections path")
	}
}
