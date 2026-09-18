package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestValidateAPIGatewayHTTPProxyURIDenyWhenUnset(t *testing.T) {
	t.Setenv(store.EnvAPIGatewayHTTPProxy, "")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com")
	err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com/ok")
	if err == nil {
		t.Fatal("expected deny when HTTP_PROXY env unset")
	}
}

func TestValidateAPIGatewayHTTPProxyURIAllowAllowlisted(t *testing.T) {
	t.Setenv(store.EnvAPIGatewayHTTPProxy, "1")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com/api,lab.example.org")
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com/api/v1"); err != nil {
		t.Fatalf("prefix allow: %v", err)
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://lab.example.org/hooks"); err != nil {
		t.Fatalf("host allow: %v", err)
	}
}

func TestValidateAPIGatewayHTTPProxyURIDenyEvilURL(t *testing.T) {
	t.Setenv(store.EnvAPIGatewayHTTPProxy, "1")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com/api")
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://evil.example/steal"); err == nil {
		t.Fatal("expected deny for non-allowlisted URL")
	}
}

func TestValidateAPIGatewayHTTPProxyURIOriginNotStringPrefix(t *testing.T) {
	t.Setenv(store.EnvAPIGatewayHTTPProxy, "1")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com")
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com/ok"); err != nil {
		t.Fatalf("same origin: %v", err)
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com/api/v1"); err != nil {
		t.Fatalf("same origin any path: %v", err)
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com.evil.com/steal"); err == nil {
		t.Fatal("expected deny host suffix")
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com@evil.com/steal"); err == nil {
		t.Fatal("expected deny userinfo host")
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com:8443/ok"); err == nil {
		t.Fatal("expected deny non-default port")
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("http://example.com/ok"); err == nil {
		t.Fatal("expected deny scheme mismatch")
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com:443/ok"); err != nil {
		t.Fatalf("default https port: %v", err)
	}
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com@evil.com/")
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://evil.com/steal"); err == nil {
		t.Fatal("expected deny allowlist userinfo entry")
	}
}

func TestValidateAPIGatewayHTTPProxyURIPathBoundary(t *testing.T) {
	t.Setenv(store.EnvAPIGatewayHTTPProxy, "1")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com/api")
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com/api"); err != nil {
		t.Fatalf("exact path: %v", err)
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com/api/v1"); err != nil {
		t.Fatalf("path child: %v", err)
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com/apievil"); err == nil {
		t.Fatal("expected deny path sibling")
	}
	if err := store.ValidateAPIGatewayHTTPProxyURI("https://example.com/other"); err == nil {
		t.Fatal("expected deny other path")
	}
}

func TestValidateAPIGatewayHTTPProxyURIMetadataNeedsExplicitAllowlist(t *testing.T) {
	t.Setenv(store.EnvAPIGatewayHTTPProxy, "1")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com/")
	if err := store.ValidateAPIGatewayHTTPProxyURI("http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Fatal("expected deny metadata when not explicitly allowlisted")
	}
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com")
	if err := store.ValidateAPIGatewayHTTPProxyURI("http://example.com@169.254.169.254/latest/meta-data/"); err == nil {
		t.Fatal("expected deny userinfo metadata when not listed")
	}
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "http://169.254.169.254/")
	if err := store.ValidateAPIGatewayHTTPProxyURI("http://169.254.169.254/latest/meta-data/"); err != nil {
		t.Fatalf("explicit metadata allowlist should pass validate: %v", err)
	}
}

func TestCreateAPIGatewayIntegrationHTTPProxyGates(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const acct = "000000000001"
	api, err := st.CreateAPIGatewayAPI(acct, "us-east-1", "proxy-lab", "HTTP")
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(store.EnvAPIGatewayHTTPProxy, "")
	if _, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "HTTP_PROXY", "https://example.com/x", "", ""); err == nil {
		t.Fatal("expected HTTP_PROXY reject when unset")
	}
	if _, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "VPC_LINK", "https://example.com/x", "", ""); err == nil {
		t.Fatal("expected VPC_LINK reject when unset")
	}

	t.Setenv(store.EnvAPIGatewayHTTPProxy, "1")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com/")
	in, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "HTTP_PROXY", "https://example.com/x", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if in.IntegrationType != store.APIGatewayIntegrationHTTPProxy {
		t.Fatalf("type=%q", in.IntegrationType)
	}

	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com")
	if _, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "HTTP_PROXY", "https://example.com.evil.com/steal", "", ""); err == nil {
		t.Fatal("expected host-suffix deny")
	}
	if _, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "HTTP_PROXY", "https://example.com@evil.com/steal", "", ""); err == nil {
		t.Fatal("expected userinfo deny")
	}
}
