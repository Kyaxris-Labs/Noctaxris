package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// APIGatewayCORS is HTTP API CorsConfiguration (lab subset).
type APIGatewayCORS struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	MaxAge           int
	AllowCredentials bool
}

// HasCORS reports whether any CORS field is configured.
func (c APIGatewayCORS) HasCORS() bool {
	return len(c.AllowOrigins) > 0 ||
		len(c.AllowMethods) > 0 ||
		len(c.AllowHeaders) > 0 ||
		len(c.ExposeHeaders) > 0 ||
		c.MaxAge > 0 ||
		c.AllowCredentials
}

// NormalizeAPIGatewayCORS validates and normalizes CorsConfiguration.
// AllowCredentials with AllowOrigins containing "*" fails closed.
func NormalizeAPIGatewayCORS(c APIGatewayCORS) (APIGatewayCORS, error) {
	out := APIGatewayCORS{
		AllowOrigins:     normalizeCORSStringList(c.AllowOrigins),
		AllowMethods:     normalizeCORSMethods(c.AllowMethods),
		AllowHeaders:     normalizeCORSStringList(c.AllowHeaders),
		ExposeHeaders:    normalizeCORSStringList(c.ExposeHeaders),
		MaxAge:           c.MaxAge,
		AllowCredentials: c.AllowCredentials,
	}
	if out.MaxAge < 0 {
		return APIGatewayCORS{}, fmt.Errorf("%w: CorsConfiguration.MaxAge must be >= 0", ErrAPIGatewayBadRequest)
	}
	if out.AllowCredentials {
		for _, o := range out.AllowOrigins {
			if o == "*" {
				return APIGatewayCORS{}, fmt.Errorf("%w: CorsConfiguration AllowCredentials cannot be used with AllowOrigins *", ErrAPIGatewayBadRequest)
			}
		}
	}
	return out, nil
}

func normalizeCORSStringList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func normalizeCORSMethods(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if s != "*" {
			s = strings.ToUpper(s)
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func marshalAPIGatewayCORS(c APIGatewayCORS) (string, error) {
	if !c.HasCORS() {
		return "", nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal cors: %w", err)
	}
	return string(b), nil
}

func unmarshalAPIGatewayCORS(raw string) APIGatewayCORS {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return APIGatewayCORS{}
	}
	var c APIGatewayCORS
	_ = json.Unmarshal([]byte(raw), &c)
	return c
}

// ParseAPIGatewayCORSFromParams reads CorsConfiguration from CreateApi/UpdateApi params.
func ParseAPIGatewayCORSFromParams(params map[string]any) (APIGatewayCORS, bool, error) {
	raw, ok := params["CorsConfiguration"]
	if !ok {
		raw, ok = params["corsConfiguration"]
	}
	if !ok || raw == nil {
		return APIGatewayCORS{}, false, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return APIGatewayCORS{}, true, fmt.Errorf("%w: CorsConfiguration must be an object", ErrAPIGatewayBadRequest)
	}
	c := APIGatewayCORS{
		AllowOrigins:     anyStringSlice(m, "AllowOrigins", "allowOrigins"),
		AllowMethods:     anyStringSlice(m, "AllowMethods", "allowMethods"),
		AllowHeaders:     anyStringSlice(m, "AllowHeaders", "allowHeaders"),
		ExposeHeaders:    anyStringSlice(m, "ExposeHeaders", "exposeHeaders"),
		AllowCredentials: anyBool(m, "AllowCredentials", "allowCredentials"),
		MaxAge:           anyInt(m, "MaxAge", "maxAge"),
	}
	norm, err := NormalizeAPIGatewayCORS(c)
	if err != nil {
		return APIGatewayCORS{}, true, err
	}
	return norm, true, nil
}

func anyStringSlice(m map[string]any, keys ...string) []string {
	var raw any
	for _, k := range keys {
		if v, ok := m[k]; ok {
			raw = v
			break
		}
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

func anyBool(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case bool:
				return t
			case string:
				return strings.EqualFold(t, "true") || t == "1"
			}
		}
	}
	return false
}

func anyInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case float64:
				return int(t)
			case int:
				return t
			case int64:
				return int(t)
			case json.Number:
				n, _ := t.Int64()
				return int(n)
			}
		}
	}
	return 0
}

// MatchCORSOrigin returns the Access-Control-Allow-Origin value for a request Origin.
// Empty string means omit ACAO.
func MatchCORSOrigin(allowOrigins []string, requestOrigin string) string {
	if len(allowOrigins) == 0 {
		return ""
	}
	req := strings.TrimSpace(requestOrigin)
	for _, o := range allowOrigins {
		if o == "*" {
			if req == "" {
				return "*"
			}
			return "*"
		}
		if req != "" && strings.EqualFold(o, req) {
			return req
		}
	}
	return ""
}
