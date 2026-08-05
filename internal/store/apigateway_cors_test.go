package store_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestNormalizeAPIGatewayCORSHappyAndFailClosed(t *testing.T) {
	norm, err := store.NormalizeAPIGatewayCORS(store.APIGatewayCORS{
		AllowOrigins:  []string{" https://a.example ", "https://a.example", "HTTPS://B.EXAMPLE"},
		AllowMethods:  []string{"get", "GET", "post", "*"},
		AllowHeaders:  []string{"X-A", "x-a", ""},
		ExposeHeaders: []string{"X-Exp"},
		MaxAge:        600,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(norm.AllowOrigins) != 2 {
		t.Fatalf("origins=%v", norm.AllowOrigins)
	}
	if len(norm.AllowMethods) != 3 || norm.AllowMethods[0] != "GET" {
		t.Fatalf("methods=%v", norm.AllowMethods)
	}
	if !norm.HasCORS() {
		t.Fatal("expected HasCORS")
	}

	if _, err := store.NormalizeAPIGatewayCORS(store.APIGatewayCORS{MaxAge: -1}); !errors.Is(err, store.ErrAPIGatewayBadRequest) {
		t.Fatalf("maxAge: %v", err)
	}
	if _, err := store.NormalizeAPIGatewayCORS(store.APIGatewayCORS{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	}); !errors.Is(err, store.ErrAPIGatewayBadRequest) {
		t.Fatalf("credentials+*: %v", err)
	}
	empty, err := store.NormalizeAPIGatewayCORS(store.APIGatewayCORS{})
	if err != nil || empty.HasCORS() {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
}

func TestParseAPIGatewayCORSFromParams(t *testing.T) {
	_, present, err := store.ParseAPIGatewayCORSFromParams(map[string]any{})
	if err != nil || present {
		t.Fatalf("absent: present=%v err=%v", present, err)
	}
	_, present, err = store.ParseAPIGatewayCORSFromParams(map[string]any{"CorsConfiguration": "bad"})
	if !present || !errors.Is(err, store.ErrAPIGatewayBadRequest) {
		t.Fatalf("non-object: present=%v err=%v", present, err)
	}

	cors, present, err := store.ParseAPIGatewayCORSFromParams(map[string]any{
		"corsConfiguration": map[string]any{
			"allowOrigins":     []any{"https://lab.example"},
			"allowMethods":     []string{"get", "OPTIONS"},
			"allowHeaders":     []any{"Authorization"},
			"exposeHeaders":    []any{"X-Req"},
			"allowCredentials": "true",
			"maxAge":           float64(120),
		},
	})
	if err != nil || !present {
		t.Fatalf("parse: present=%v err=%v", present, err)
	}
	if len(cors.AllowOrigins) != 1 || cors.MaxAge != 120 || !cors.AllowCredentials {
		t.Fatalf("cors=%+v", cors)
	}

	_, present, err = store.ParseAPIGatewayCORSFromParams(map[string]any{
		"CorsConfiguration": map[string]any{
			"AllowOrigins":     []any{"*"},
			"AllowCredentials": true,
			"MaxAge":           json.Number("30"),
		},
	})
	if !present || !errors.Is(err, store.ErrAPIGatewayBadRequest) {
		t.Fatalf("fail-closed *: present=%v err=%v", present, err)
	}
}

func TestMatchCORSOrigin(t *testing.T) {
	if got := store.MatchCORSOrigin(nil, "https://a"); got != "" {
		t.Fatalf("nil allow=%q", got)
	}
	if got := store.MatchCORSOrigin([]string{"*"}, ""); got != "*" {
		t.Fatalf("star empty req=%q", got)
	}
	if got := store.MatchCORSOrigin([]string{"*"}, "https://x"); got != "*" {
		t.Fatalf("star=%q", got)
	}
	if got := store.MatchCORSOrigin([]string{"https://Lab.Example"}, "https://lab.example"); got != "https://lab.example" {
		t.Fatalf("case match=%q", got)
	}
	if got := store.MatchCORSOrigin([]string{"https://a.example"}, "https://b.example"); got != "" {
		t.Fatalf("mismatch=%q", got)
	}
	if got := store.MatchCORSOrigin([]string{"https://a.example"}, ""); got != "" {
		t.Fatalf("empty req=%q", got)
	}
}
