package config

import (
	"net"
	"testing"
)

func TestParseTrustedProxies(t *testing.T) {
	t.Parallel()
	nets, err := ParseTrustedProxies("203.0.113.0/24, 2001:db8::1")
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 {
		t.Fatalf("len=%d", len(nets))
	}
	if !PeerInTrustedProxies("203.0.113.9", nets) {
		t.Fatal("expected 203.0.113.9 in /24")
	}
	if PeerInTrustedProxies("198.51.100.1", nets) {
		t.Fatal("198.51.100.1 must not match")
	}
	if !PeerInTrustedProxies("2001:db8::1", nets) {
		t.Fatal("expected exact IPv6")
	}
	empty, err := ParseTrustedProxies("")
	if err != nil || empty != nil {
		t.Fatalf("empty: %v %v", empty, err)
	}
	if PeerInTrustedProxies("203.0.113.9", nil) {
		t.Fatal("nil trusted must deny")
	}
	_, err = ParseTrustedProxies("not-an-ip")
	if err == nil {
		t.Fatal("expected parse error")
	}
	_ = net.IPv4zero
}
