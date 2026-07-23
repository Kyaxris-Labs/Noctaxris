package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func TestAnonymousS3GetObjectDefaultDenyWithoutEnv(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-off", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-off/pub.txt", []byte("secret"), "s3", now, nil)
	policy := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::anon-off/*"}]}`)
	pol := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-off?policy", policy, "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if pol.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy status=%d body=%q", pol.Code, pol.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/anon-off/pub.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unsigned Get without env status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("expected MissingAuthenticationToken, got %q", rec.Body.String())
	}
}

func TestAnonymousS3GetObjectEnvPlusPolicyAllow(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.AllowAnonymousS3 = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-pol", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-pol/hi.txt", []byte("hello-anon"), "s3", now, nil)
	policy := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::anon-pol/*"}]}`)
	if rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-pol?policy", policy, "s3", now, map[string]string{
		"Content-Type": "application/json",
	}); rec.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy status=%d body=%q", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/anon-pol/hi.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous GetObject status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "hello-anon" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestAnonymousS3GetObjectEnvWithoutPolicyStillDeny(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.AllowAnonymousS3 = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-priv", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-priv/hi.txt", []byte("nope"), "s3", now, nil)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/anon-priv/hi.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("expected AccessDenied, got %q", rec.Body.String())
	}
}

func TestAnonymousS3GetObjectPublicReadACL(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.AllowAnonymousS3 = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-acl", nil, "s3", now, nil)
	put := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-acl/open.txt", []byte("via-acl"), "s3", now, map[string]string{
		"x-amz-acl": "public-read",
	})
	if put.Code != http.StatusOK {
		t.Fatalf("PutObject public-read status=%d body=%q", put.Code, put.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/anon-acl/open.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous ACL GetObject status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "via-acl" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestAnonymousS3GateDoesNotOpenListOrPut(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.AllowAnonymousS3 = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-gate", nil, "s3", now, nil)
	policy := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:*","Resource":["arn:aws:s3:::anon-gate","arn:aws:s3:::anon-gate/*"]}]}`)
	if rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-gate?policy", policy, "s3", now, map[string]string{
		"Content-Type": "application/json",
	}); rec.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy status=%d body=%q", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/anon-gate", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusForbidden || !strings.Contains(listRec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("unsigned ListObjects must stay SigV4-required, status=%d body=%q", listRec.Code, listRec.Body.String())
	}

	putReq := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:4566/anon-gate/x.txt", strings.NewReader("x"))
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusForbidden || !strings.Contains(putRec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("unsigned PutObject must stay SigV4-required, status=%d body=%q", putRec.Code, putRec.Body.String())
	}
}

func TestAnonymousS3PolicyDenyBeatsPublicReadACL(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.AllowAnonymousS3 = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-deny", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-deny/x.txt", []byte("blocked"), "s3", now, map[string]string{
		"x-amz-acl": "public-read",
	})
	policy := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":"*","Action":"s3:GetObject","Resource":"*"}]}`)
	if rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/anon-deny?policy", policy, "s3", now, map[string]string{
		"Content-Type": "application/json",
	}); rec.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy status=%d body=%q", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/anon-deny/x.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("Deny policy must beat ACL, status=%d body=%q", rec.Code, rec.Body.String())
	}
}
