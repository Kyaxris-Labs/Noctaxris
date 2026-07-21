package store

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostSNSHTTPRejectsRedirects(t *testing.T) {
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	}))
	t.Cleanup(redir.Close)

	host := strings.TrimPrefix(redir.URL, "http://")
	// httptest binds loopback; allowlist + host safety must reject before Do.
	t.Setenv(EnvSNSHTTPAllowlist, redir.URL+"/hook")
	err = st.postSNSHTTP(redir.URL+"/hook", "arn:sub", "arn:topic", "Notification", []byte(`{}`))
	if err == nil {
		t.Fatal("expected loopback allowlist delivery to fail closed")
	}
	_ = host
}
