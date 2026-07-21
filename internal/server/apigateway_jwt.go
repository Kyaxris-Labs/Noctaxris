package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwksfetch"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
)

// verifyAPIGatewayJWT validates a Bearer JWT for HTTP API JWT authorizers and AppSync Cognito.
// Uses shared jwtutil (go-jose v4, RS256 only). Checks iss, aud or client_id, exp,
// and requires token_use to be one of allowedTokenUses (empty/missing is rejected).
func (s *Server) verifyAPIGatewayJWT(token, issuer string, audience []string, now time.Time, allowedTokenUses ...string) error {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if token == "" || issuer == "" || len(audience) == 0 {
		return fmt.Errorf("jwt inputs incomplete")
	}
	if len(allowedTokenUses) == 0 {
		return fmt.Errorf("token_use allowlist required")
	}
	jwksJSON, err := s.loadJWKSForIssuer(issuer)
	if err != nil {
		return err
	}
	claims, err := jwtutil.VerifyCompactRS256(token, jwksJSON)
	if err != nil {
		return err
	}
	if !issuerEqual(jwtutil.ClaimString(claims, "iss"), issuer) {
		return fmt.Errorf("iss mismatch")
	}
	if jwtutil.ClaimExpired(claims, now) {
		return fmt.Errorf("token expired")
	}
	if jwtutil.ClaimNotYetValid(claims, now) {
		return fmt.Errorf("token not yet valid")
	}
	tokenUse := strings.ToLower(strings.TrimSpace(jwtutil.ClaimString(claims, "token_use")))
	if tokenUse == "" {
		return fmt.Errorf("missing token_use")
	}
	allowed := false
	for _, want := range allowedTokenUses {
		if tokenUse == strings.ToLower(strings.TrimSpace(want)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("unexpected token_use")
	}
	auds := normalizeJWTAudience(claims["aud"])
	if cid := jwtutil.ClaimString(claims, "client_id"); cid != "" {
		auds = append(auds, cid)
	}
	if !audienceOverlap(auds, audience) {
		return fmt.Errorf("audience mismatch")
	}
	return nil
}

// loadJWKSForIssuer prefers in-process Cognito store JWKS for lab issuers (httptest-safe).
// Non-lab issuers require NOCTAXRIS_ALLOW_REMOTE_JWKS with a public host allowlist (fail-closed SSRF).
func (s *Server) loadJWKSForIssuer(issuer string) ([]byte, error) {
	if _, poolID, ok := jwksfetch.ParseLabCognitoIssuer(issuer); ok {
		jwks, err := s.store.CognitoJWKSJSON(poolID)
		if err != nil {
			return nil, fmt.Errorf("cognito jwks: %w", err)
		}
		return jwks, nil
	}
	return jwksfetch.FetchRemoteJWKS(issuer, nil)
}

func issuerEqual(iss, configured string) bool {
	a := strings.TrimRight(strings.TrimSpace(iss), "/")
	b := strings.TrimRight(strings.TrimSpace(configured), "/")
	return strings.EqualFold(a, b)
}

func normalizeJWTAudience(aud any) []string {
	switch v := aud.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func audienceOverlap(got, want []string) bool {
	for _, w := range want {
		for _, g := range got {
			if g == w {
				return true
			}
		}
	}
	return false
}
