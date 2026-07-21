package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustElastiCacheQuery(t *testing.T, handler http.Handler, body string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "elasticache", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestElastiCacheHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustElastiCacheQuery(t, handler, strings.Join([]string{
		"Action=CreateCacheCluster",
		"Version=2015-02-02",
		"CacheClusterId=lab-cache-1",
		"Engine=redis",
		"CacheNodeType=cache.t3.micro",
		"NumCacheNodes=1",
	}, "&"), now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateCacheCluster status=%d body=%q", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "lab-cache-1") || !strings.Contains(create.Body.String(), "cache.noctaxris.internal") {
		t.Fatalf("create body=%q", create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "creating") {
		t.Fatalf("without DinD expect creating, body=%q", create.Body.String())
	}
	if strings.Contains(create.Body.String(), "available") {
		t.Fatalf("must not claim available without nested engine: %q", create.Body.String())
	}

	desc := mustElastiCacheQuery(t, handler,
		"Action=DescribeCacheClusters&Version=2015-02-02&CacheClusterId=lab-cache-1", now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "6379") {
		t.Fatalf("DescribeCacheClusters status=%d body=%q", desc.Code, desc.Body.String())
	}
	if !strings.Contains(desc.Body.String(), "creating") {
		t.Fatalf("describe without DinD expect creating, body=%q", desc.Body.String())
	}

	del := mustElastiCacheQuery(t, handler,
		"Action=DeleteCacheCluster&Version=2015-02-02&CacheClusterId=lab-cache-1", now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteCacheCluster status=%d body=%q", del.Code, del.Body.String())
	}
}

func mustDocDBQuery(t *testing.T, handler http.Handler, body string, now time.Time, service string) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, service, now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestDocDBHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createBody := strings.Join([]string{
		"Action=CreateDBCluster",
		"Version=2014-10-31",
		"DBClusterIdentifier=lab-docdb-1",
		"Engine=docdb",
		"MasterUsername=labadmin",
		"MasterUserPassword=" + url.QueryEscape("lab-password-1"),
	}, "&")
	// DocumentDB CLI typically signs as rds.
	create := mustDocDBQuery(t, handler, createBody, now, "rds")
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDBCluster status=%d body=%q", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "lab-docdb-1") || !strings.Contains(create.Body.String(), "docdb.noctaxris.internal") {
		t.Fatalf("create body=%q", create.Body.String())
	}

	desc := mustDocDBQuery(t, handler,
		"Action=DescribeDBClusters&Version=2014-10-31&DBClusterIdentifier=lab-docdb-1", now, "rds")
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "27017") {
		t.Fatalf("DescribeDBClusters status=%d body=%q", desc.Code, desc.Body.String())
	}

	rej := mustDocDBQuery(t, handler, strings.Join([]string{
		"Action=CreateDBCluster",
		"Version=2014-10-31",
		"DBClusterIdentifier=neo-1",
		"Engine=neptune",
	}, "&"), now, "rds")
	if rej.Code == http.StatusOK {
		t.Fatalf("neptune should be rejected: %q", rej.Body.String())
	}

	del := mustDocDBQuery(t, handler,
		"Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=lab-docdb-1", now, "rds")
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteDBCluster status=%d body=%q", del.Code, del.Body.String())
	}
}
