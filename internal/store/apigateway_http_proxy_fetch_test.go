package store_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestValidateAPIGatewayHTTPProxyURIBoundary(t *testing.T) {
	t.Setenv(store.EnvAPIGatewayHTTPProxy, "1")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com/")
	if err := store.ValidateAPIGatewayHTTPProxyURI(""); err == nil {
		t.Fatal("empty uri")
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("ftp://example.com/x"); err == nil {
		t.Fatal("bad scheme")
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("not-a-url"); err == nil {
		t.Fatal("relative")
	}
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "")
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com/x"); err == nil {
		t.Fatal("empty allowlist")
	}
	if !store.APIGatewayHTTPProxyEnabled() {
		t.Fatal("expected enabled")
	}
}

func TestFetchAPIGatewayHTTPProxyHappyAndNegatives(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo", r.Header.Get("X-Lab"))
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			_, _ = w.Write([]byte("post:" + string(b)))
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	t.Setenv(store.EnvAPIGatewayHTTPProxy, "1")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, srv.URL+"/")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	status, body, hdr, err := store.FetchAPIGatewayHTTPProxy(ctx, "GET", srv.URL+"/ping", nil, http.Header{
		"X-Lab":             []string{"1"},
		"Host":              []string{"evil"},
		"Connection":        []string{"keep-alive"},
		"Content-Length":    []string{"9"},
		"Transfer-Encoding": []string{"chunked"},
	})
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != 200 || string(body) != "ok" || hdr.Get("X-Echo") != "1" {
		t.Fatalf("status=%d body=%q echo=%q", status, body, hdr.Get("X-Echo"))
	}

	status, body, _, err = store.FetchAPIGatewayHTTPProxy(ctx, "POST", srv.URL+"/ping", []byte("hi"), nil)
	if err != nil || status != 200 || string(body) != "post:hi" {
		t.Fatalf("POST: status=%d body=%q err=%v", status, body, err)
	}

	t.Setenv(store.EnvAPIGatewayHTTPProxy, "")
	if _, _, _, err := store.FetchAPIGatewayHTTPProxy(ctx, "GET", srv.URL+"/ping", nil, nil); err == nil {
		t.Fatal("expected fail-closed when proxy disabled")
	}
}

func TestPinnedAPIGatewayHTTPProxyDialContextRejectsLoopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	_, err := store.PinnedAPIGatewayHTTPProxyDialContext(ctx, "tcp", "127.0.0.1:9")
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("want unsafe dial reject, got %v", err)
	}
}
