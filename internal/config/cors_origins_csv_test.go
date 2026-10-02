package config_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func TestFunctionURLCORSOriginsCSVParsing(t *testing.T) {
	t.Setenv("NOCTAXRIS_FUNCTION_URL_CORS_ORIGINS", " https://a.example , ,https://b.example,  ")
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.FunctionURLCORSOrigins
	if len(got) != 2 || got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Fatalf("CORS origins=%v", got)
	}

	t.Setenv("NOCTAXRIS_FUNCTION_URL_CORS_ORIGINS", "   ")
	cfg, err = config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FunctionURLCORSOrigins != nil {
		t.Fatalf("blank CSV want nil, got %v", cfg.FunctionURLCORSOrigins)
	}
}

func TestListenIsLoopbackIPv6AndInvalidHost(t *testing.T) {
	if !config.ListenIsLoopback("[::1]:4566") {
		t.Fatal("::1 should be loopback")
	}
	if config.ListenIsLoopback("not a host") {
		t.Fatal("invalid host must be non-loopback")
	}
	if config.ListenIsLoopback("") {
		t.Fatal("empty must be non-loopback")
	}
}
