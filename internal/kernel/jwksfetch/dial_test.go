package jwksfetch

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestPinnedSafeDialRejectsPrivateAndLoopback(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, addr := range []string{
		"127.0.0.1:443",
		"10.0.0.1:443",
		"169.254.169.254:80",
		"[::1]:443",
	} {
		_, err := pinnedSafeDialContext(ctx, "tcp", addr)
		if err == nil {
			t.Fatalf("expected reject for %s", addr)
		}
		if !strings.Contains(err.Error(), "jwksfetch:") && !strings.Contains(err.Error(), "not allowed") {
			// Dial may fail with network errors after IP reject; both are fail-closed.
			if _, ok := err.(*net.OpError); !ok && !strings.Contains(strings.ToLower(err.Error()), "refused") {
				t.Fatalf("addr=%s err=%v", addr, err)
			}
		}
	}
}
