package config_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func TestListenIsLoopback(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:4566", true},
		{"127.0.0.1:0", true},
		{"localhost:4566", true},
		{":4566", true},
		{"[::1]:4566", true},
		{"0.0.0.0:4566", false},
		{"192.168.1.10:4566", false},
	}
	for _, tc := range cases {
		if got := config.ListenIsLoopback(tc.addr); got != tc.want {
			t.Fatalf("ListenIsLoopback(%q)=%v want %v", tc.addr, got, tc.want)
		}
	}
}

func TestValidateListenSecurityNonLoopbackRequiresTLSOrOptIn(t *testing.T) {
	cfg := config.Config{ListenAddr: "0.0.0.0:4566"}
	if err := config.ValidateListenSecurity(cfg); err == nil {
		t.Fatal("expected error for non-loopback without TLS")
	}
	t.Setenv(config.EnvAllowNonLoopbackListen, "1")
	if err := config.ValidateListenSecurity(cfg); err != nil {
		t.Fatalf("opt-in should allow: %v", err)
	}
}

func TestOpenDataPlaneAllowed(t *testing.T) {
	loop := config.Config{ListenAddr: "127.0.0.1:4566"}
	if !loop.OpenDataPlaneAllowed() {
		t.Fatal("loopback should allow open data plane")
	}
	non := config.Config{ListenAddr: "0.0.0.0:4566"}
	if non.OpenDataPlaneAllowed() {
		t.Fatal("non-loopback should deny open data plane by default")
	}
	t.Setenv(config.EnvAllowOpenDataPlane, "1")
	if !non.OpenDataPlaneAllowed() {
		t.Fatal("opt-in should allow open data plane")
	}
}
