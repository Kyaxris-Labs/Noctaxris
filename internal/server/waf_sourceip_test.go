package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWAFSourceIPFromHTTP(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "http://example/path", nil)
	req.RemoteAddr = "203.0.113.9:54321"
	if got := wafSourceIPFromHTTP(req, nil); got != "203.0.113.9" {
		t.Fatalf("RemoteAddr: got %q want 203.0.113.9", got)
	}

	req.Header.Set("X-Forwarded-For", "192.0.2.44, 198.51.100.1")
	if got := wafSourceIPFromHTTP(req, nil); got != "203.0.113.9" {
		t.Fatalf("XFF ignored without trusted proxies: got %q want peer", got)
	}

	_, n, err := net.ParseCIDR("203.0.113.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if got := wafSourceIPFromHTTP(req, []*net.IPNet{n}); got != "192.0.2.44" {
		t.Fatalf("XFF first hop with trusted peer: got %q want 192.0.2.44", got)
	}

	req.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := wafSourceIPFromHTTP(req, []*net.IPNet{n}); got != "203.0.113.9" {
		t.Fatalf("invalid XFF fallback: got %q want 203.0.113.9", got)
	}
}

func TestWAFRequestViewFromEvaluateParamsSourceIP(t *testing.T) {
	t.Parallel()
	view := wafRequestViewFromEvaluateParams(map[string]any{
		"URI":      "/",
		"SourceIP": "198.51.100.10",
	})
	if view == nil || view.SourceIP != "198.51.100.10" {
		t.Fatalf("explicit SourceIP: %+v", view)
	}
	view = wafRequestViewFromEvaluateParams(map[string]any{
		"URI": "/",
		"Headers": map[string]any{
			"X-Forwarded-For": "192.0.2.1, 10.0.0.1",
		},
	})
	if view == nil || view.SourceIP != "192.0.2.1" {
		t.Fatalf("XFF derived SourceIP: %+v", view)
	}
}
