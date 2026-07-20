package server

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// verifyAPIGatewayJWT validates a Bearer JWT for HTTP API JWT authorizers and AppSync Cognito.
// Uses shared jwtutil (go-jose v4, RS256 only). Checks iss, aud or client_id, exp,
// and prefers token_use=access (also accepts id when audience matches).
func (s *Server) verifyAPIGatewayJWT(token, issuer string, audience []string, now time.Time) error {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if token == "" || issuer == "" || len(audience) == 0 {
		return fmt.Errorf("jwt inputs incomplete")
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
	tokenUse := strings.ToLower(strings.TrimSpace(jwtutil.ClaimString(claims, "token_use")))
	if tokenUse != "" && tokenUse != "access" && tokenUse != "id" {
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

// loadJWKSForIssuer prefers in-process Cognito store JWKS for lab issuers (httptest-safe),
// otherwise fetches <issuer>/.well-known/jwks.json over HTTP.
func (s *Server) loadJWKSForIssuer(issuer string) ([]byte, error) {
	if _, poolID, ok := parseLabCognitoIssuer(issuer); ok {
		jwks, err := s.store.CognitoJWKSJSON(poolID)
		if err != nil {
			return nil, fmt.Errorf("cognito jwks: %w", err)
		}
		return jwks, nil
	}
	jwksURL := issuer + "/.well-known/jwks.json"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("jwks unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("jwks read: %w", err)
	}
	return body, nil
}

func parseLabCognitoIssuer(issuer string) (region, poolID string, ok bool) {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	const prefix = store.CognitoLabIssuerHost + "/cognito-idp/"
	if !strings.HasPrefix(issuer, prefix) {
		// Also accept host-less path form used in some lab docs.
		pathPrefix := "/cognito-idp/"
		if strings.HasPrefix(issuer, pathPrefix) {
			rest := strings.TrimPrefix(issuer, pathPrefix)
			parts := strings.Split(rest, "/")
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				return parts[0], parts[1], true
			}
		}
		return "", "", false
	}
	rest := strings.TrimPrefix(issuer, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
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
