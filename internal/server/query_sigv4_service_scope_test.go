package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryProtocolRejectsServiceScopeMismatch(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	// CreateUser remaps to iam:CreateUser; signing as s3 must fail closed.
	body := "Action=CreateUser&Version=2010-05-08&UserName=scope-mismatch-user"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "s3", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("mismatched Query scope status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Credential should be scoped to correct service") &&
		!strings.Contains(rec.Body.String(), "SignatureDoesNotMatch") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestQueryProtocolRejectsNonIAMShortNameServiceMismatch(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	// CreateCacheCluster stays a short name; signing as s3 must not reach ElastiCache.
	body := "Action=CreateCacheCluster&Version=2015-02-02&CacheClusterId=scope-mismatch&Engine=redis"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "s3", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("mismatched ElastiCache Query status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Credential should be scoped to correct service") &&
		!strings.Contains(rec.Body.String(), "SignatureDoesNotMatch") {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "CacheCluster") && strings.Contains(rec.Body.String(), "CreateCacheClusterResponse") {
		t.Fatalf("s3-scoped CreateCacheCluster must not run ElastiCache body=%q", rec.Body.String())
	}
}

func TestQueryProtocolAllowsMatchingServiceScope(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	body := "Action=DescribeDBClusters&Version=2014-10-31"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "neptune", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "Credential should be scoped") {
		t.Fatalf("matching neptune Query must not fail SigV4 scope body=%q", rec.Body.String())
	}
}
