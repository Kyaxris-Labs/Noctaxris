package store

import (
	"net"
	"testing"
)

func TestLabForwardIPDenied(t *testing.T) {
	cases := []struct {
		ip     string
		denied bool
	}{
		{"", true},
		{"0.0.0.0", true},
		{"::", true},
		{"169.254.169.254", true},
		{"127.0.0.1", false},
		{"10.0.0.5", false},
		{"192.168.1.1", false},
	}
	for _, tc := range cases {
		var ip net.IP
		if tc.ip != "" {
			ip = net.ParseIP(tc.ip)
		}
		got := LabForwardIPDenied(ip)
		if got != tc.denied {
			t.Errorf("LabForwardIPDenied(%q) = %v want %v", tc.ip, got, tc.denied)
		}
	}
}
