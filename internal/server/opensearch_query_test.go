package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestOpenSearchNestedHostAllowlist(t *testing.T) {
	if err := validateNestedOpenSearchHost("noctaxris-opensearch-lab1"); err != nil {
		t.Fatal(err)
	}
	if err := validateNestedOpenSearchHost("noctaxris-data-opensearch-lab1"); err != nil {
		t.Fatal(err)
	}
	if err := validateNestedOpenSearchHost("evil.example"); err == nil {
		t.Fatal("expected reject")
	}
	for _, host := range []string{
		"",
		"localhost",
		"127.0.0.1",
		"::1",
		"0.0.0.0",
		"8.8.8.8",
		"host.docker.internal",
		"noctaxris-data-rds-lab1",
		"noctaxris-opensearch-",
		"noctaxris-data-opensearch-",
	} {
		if err := validateNestedOpenSearchHost(host); err == nil {
			t.Fatalf("expected reject for host %q", host)
		}
	}
}

func TestOpenSearchLabSearchBodyAllowlist(t *testing.T) {
	okBodies := []string{
		`{}`,
		`{"size":10}`,
		`{"query":{"match_all":{}}}`,
		`{"query":{"match":{"title":"hello"}},"size":5}`,
	}
	for _, body := range okBodies {
		if err := validateOpenSearchLabSearchBody([]byte(body)); err != nil {
			t.Fatalf("allow %s: %v", body, err)
		}
	}
	badBodies := []string{
		`{"aggs":{}}`,
		`{"query":{"bool":{"must":[]}}}`,
		`{"query":{"term":{"a":1}}}`,
		`{"_source":true}`,
		`{"query":{"match_all":{}},"sort":[]}`,
		`not-json`,
	}
	for _, body := range badBodies {
		if err := validateOpenSearchLabSearchBody([]byte(body)); err == nil {
			t.Fatalf("expected reject for %s", body)
		}
	}
}

func TestOpenSearchLabIndexAndSearchProxy(t *testing.T) {
	srv, st := newOpenSearchQueryTestServer(t)
	if _, err := st.CreateOpenSearchDomain(testAccountIDOS, store.DefaultOpenSearchRegion, "lab1", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetOpenSearchContainerID(
		testAccountIDOS, "lab1", "cid-lab1",
		store.OpenSearchDomainStatusActive,
		store.OpenSearchNestedEndpoint("lab1"),
		"",
	); err != nil {
		t.Fatal(err)
	}

	var gotMethod, gotURL string
	var gotBody []byte
	prev := openSearchLabTransport
	openSearchLabTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod = req.Method
		gotURL = req.URL.String()
		b, _ := io.ReadAll(req.Body)
		gotBody = b
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"result":"created"}`)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { openSearchLabTransport = prev })

	putReq := httptest.NewRequest(http.MethodPut, "/opensearch/lab1/lab/books/_doc/1", strings.NewReader(`{"title":"hello"}`))
	putRec := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(putRec, putReq, []byte(`{"title":"hello"}`), "req-1", "evt-1", nil, false)
	if putRec.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%q", putRec.Code, putRec.Body.String())
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("proxied method=%q", gotMethod)
	}
	if gotURL != "http://noctaxris-opensearch-lab1:9200/books/_doc/1" {
		t.Fatalf("proxied url=%q", gotURL)
	}
	if string(gotBody) != `{"title":"hello"}` {
		t.Fatalf("proxied body=%q", gotBody)
	}
	if !strings.Contains(putRec.Body.String(), "created") {
		t.Fatalf("response body=%q", putRec.Body.String())
	}

	searchBody := []byte(`{"query":{"match":{"title":"hello"}},"size":3}`)
	openSearchLabTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod = req.Method
		gotURL = req.URL.String()
		b, _ := io.ReadAll(req.Body)
		gotBody = b
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"hits":{"total":{"value":1}}}`)),
			Request:    req,
		}, nil
	})
	searchReq := httptest.NewRequest(http.MethodPost, "/opensearch/lab1/lab/books/_search", bytes.NewReader(searchBody))
	searchRec := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(searchRec, searchReq, searchBody, "req-2", "evt-2", nil, false)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%q", searchRec.Code, searchRec.Body.String())
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("search method=%q", gotMethod)
	}
	if gotURL != "http://noctaxris-opensearch-lab1:9200/books/_search" {
		t.Fatalf("search url=%q", gotURL)
	}
	if !bytes.Equal(gotBody, searchBody) {
		t.Fatalf("search body=%q", gotBody)
	}
}

func TestOpenSearchLabRejectUnknownDSL(t *testing.T) {
	srv, st := newOpenSearchQueryTestServer(t)
	if _, err := st.CreateOpenSearchDomain(testAccountIDOS, store.DefaultOpenSearchRegion, "labdsl", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetOpenSearchContainerID(
		testAccountIDOS, "labdsl", "cid",
		store.OpenSearchDomainStatusActive,
		store.OpenSearchNestedEndpoint("labdsl"),
		"",
	); err != nil {
		t.Fatal(err)
	}
	dialed := false
	prev := openSearchLabTransport
	openSearchLabTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		dialed = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
	})
	t.Cleanup(func() { openSearchLabTransport = prev })

	body := []byte(`{"query":{"bool":{"must":[]}}}`)
	req := httptest.NewRequest(http.MethodPost, "/opensearch/labdsl/lab/idx/_search", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(rec, req, body, "req", "evt", nil, false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%q", rec.Code, rec.Body.String())
	}
	if dialed {
		t.Fatal("must not proxy unknown DSL")
	}
}

func TestOpenSearchLabFailClosedStubAndCreating(t *testing.T) {
	srv, st := newOpenSearchQueryTestServer(t)
	if _, err := st.CreateOpenSearchDomain(testAccountIDOS, store.DefaultOpenSearchRegion, "stubdom", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetOpenSearchContainerID(
		testAccountIDOS, "stubdom", "",
		store.OpenSearchDomainStatusCreateFailed,
		"stub://127.0.0.1/opensearch/x",
		"mmap",
	); err != nil {
		t.Fatal(err)
	}

	dialed := false
	prev := openSearchLabTransport
	openSearchLabTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		dialed = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
	})
	t.Cleanup(func() { openSearchLabTransport = prev })

	body := []byte(`{"a":1}`)
	req := httptest.NewRequest(http.MethodPut, "/opensearch/stubdom/lab/idx/_doc/1", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(rec, req, body, "req", "evt", nil, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stub status=%d want 409 body=%q", rec.Code, rec.Body.String())
	}
	if dialed {
		t.Fatal("must not dial stub domain")
	}

	if _, err := st.CreateOpenSearchDomain(testAccountIDOS, store.DefaultOpenSearchRegion, "creating", ""); err != nil {
		t.Fatal(err)
	}
	req2 := httptest.NewRequest(http.MethodPut, "/opensearch/creating/lab/idx/_doc/1", bytes.NewReader(body))
	rec2 := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(rec2, req2, body, "req", "evt", nil, false)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("creating status=%d want 409 body=%q", rec2.Code, rec2.Body.String())
	}
	if dialed {
		t.Fatal("must not dial Creating domain")
	}
}

func TestOpenSearchLabNeverDialsOperatorHost(t *testing.T) {
	srv, st := newOpenSearchQueryTestServer(t)
	if _, err := st.CreateOpenSearchDomain(testAccountIDOS, store.DefaultOpenSearchRegion, "evilend", ""); err != nil {
		t.Fatal(err)
	}
	// Bypass store Active/stub guard by writing a nested-looking Active row, then
	// force StubEndpoint to an operator host via direct SQL is not available; use
	// SetOpenSearchContainerID with a non-allowlisted host that still looks like host:port.
	if err := st.SetOpenSearchContainerID(
		testAccountIDOS, "evilend", "cid",
		store.OpenSearchDomainStatusActive,
		"evil.example:9200",
		"",
	); err != nil {
		t.Fatal(err)
	}
	dialed := false
	prev := openSearchLabTransport
	openSearchLabTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		dialed = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
	})
	t.Cleanup(func() { openSearchLabTransport = prev })

	body := []byte(`{"a":1}`)
	req := httptest.NewRequest(http.MethodPut, "/opensearch/evilend/lab/idx/_doc/1", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(rec, req, body, "req", "evt", nil, false)
	if rec.Code == http.StatusOK {
		t.Fatalf("must reject operator host, body=%q", rec.Body.String())
	}
	if dialed {
		t.Fatal("must not dial operator-supplied host")
	}
}

const testAccountIDOS = "000000000001"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newOpenSearchQueryTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if err := st.EnsureRoot(testAccountIDOS, "AKIAROOTEXAMPLE01", "secret-root-value"); err != nil {
		t.Fatal(err)
	}
	aud, err := audit.NewWriter(filepath.Join(dir, "cloudtrail"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = aud.Close()
	})
	cfg := config.Config{
		ListenAddr: "127.0.0.1:0",
		DataRoot:   dir,
		AccountID:  testAccountIDOS,
	}
	return New(cfg, st, aud), st
}
