package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestOpenSearchLabQueryRequiresESHttpBeyondDescribe(t *testing.T) {
	srv, st := newOpenSearchQueryTestServer(t)
	if _, err := st.CreateOpenSearchDomain(testAccountIDOS, store.DefaultOpenSearchRegion, "httpauthz", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetOpenSearchContainerID(
		testAccountIDOS, "httpauthz", "cid",
		store.OpenSearchDomainStatusActive,
		store.OpenSearchNestedEndpoint("httpauthz"),
		"",
	); err != nil {
		t.Fatal(err)
	}

	userName := "os-describe-only"
	if _, _, err := st.CreateUser(testAccountIDOS, userName); err != nil {
		t.Fatal(err)
	}
	akid, _, err := st.CreateUserAccessKey(testAccountIDOS, userName)
	if err != nil {
		t.Fatal(err)
	}
	descOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"es:DescribeDomain","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountIDOS, "OSDescribeOnly", descOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountIDOS, userName, polARN); err != nil {
		t.Fatal(err)
	}

	v := &authn.Verified{
		AccountID:   testAccountIDOS,
		AccessKeyID: akid,
		Region:      store.DefaultOpenSearchRegion,
		Service:     "es",
		Principal:   identity.UserPrincipal(testAccountIDOS, userName, akid),
	}
	body := []byte(`{"title":"x"}`)
	req := httptest.NewRequest(http.MethodPut, "/opensearch/httpauthz/lab/idx/_doc/1", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(rec, req, body, "req-1", "evt-1", v, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("DescribeDomain-only index status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), catalog.ActionOpenSearchESHttpPut) {
		t.Fatalf("body=%q", rec.Body.String())
	}

	httpAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["es:DescribeDomain","es:ESHttpPut"],"Resource":"*"}]}`
	httpARN, err := st.CreateManagedPolicy(testAccountIDOS, "OSESHTTP", httpAllow)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountIDOS, userName, httpARN); err != nil {
		t.Fatal(err)
	}

	prev := openSearchLabTransport
	openSearchLabTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"result":"created"}`)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { openSearchLabTransport = prev })

	reqOK := httptest.NewRequest(http.MethodPut, "/opensearch/httpauthz/lab/idx/_doc/1", strings.NewReader(string(body)))
	okRec := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(okRec, reqOK, body, "req-2", "evt-2", v, false)
	if okRec.Code != http.StatusOK {
		t.Fatalf("ESHttpPut Allow status=%d body=%q", okRec.Code, okRec.Body.String())
	}

	searchBody := []byte(`{"query":{"match_all":{}}}`)
	searchDeny := httptest.NewRequest(http.MethodPost, "/opensearch/httpauthz/lab/idx/_search", strings.NewReader(string(searchBody)))
	searchDenyRec := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(searchDenyRec, searchDeny, searchBody, "req-3", "evt-3", v, false)
	if searchDenyRec.Code != http.StatusForbidden {
		t.Fatalf("DescribeDomain+ESHttpPut without ESHttpPost search status=%d want 403 body=%q", searchDenyRec.Code, searchDenyRec.Body.String())
	}
	if !strings.Contains(searchDenyRec.Body.String(), catalog.ActionOpenSearchESHttpPost) {
		t.Fatalf("search deny body=%q", searchDenyRec.Body.String())
	}

	postAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["es:DescribeDomain","es:ESHttpPost"],"Resource":"*"}]}`
	postARN, err := st.CreateManagedPolicy(testAccountIDOS, "OSESHTTPPost", postAllow)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountIDOS, userName, postARN); err != nil {
		t.Fatal(err)
	}
	searchOK := httptest.NewRequest(http.MethodPost, "/opensearch/httpauthz/lab/idx/_search", strings.NewReader(string(searchBody)))
	searchOKRec := httptest.NewRecorder()
	srv.handleOpenSearchLabQuery(searchOKRec, searchOK, searchBody, "req-4", "evt-4", v, false)
	if searchOKRec.Code != http.StatusOK {
		t.Fatalf("ESHttpPost Allow search status=%d body=%q", searchOKRec.Code, searchOKRec.Body.String())
	}
}
