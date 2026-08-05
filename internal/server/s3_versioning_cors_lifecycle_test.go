package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestS3VersioningNegativesCoverage(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/ver-neg-bucket", nil, "s3", now, nil)

	// Malformed XML
	badXML := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/ver-neg-bucket?versioning", []byte("<not-versioning/>"), "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if badXML.Code != http.StatusBadRequest {
		t.Fatalf("malformed versioning want 400 got %d %s", badXML.Code, badXML.Body.String())
	}

	// Illegal status
	illegal := []byte(`<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Maybe</Status></VersioningConfiguration>`)
	ill := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/ver-neg-bucket?versioning", illegal, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if ill.Code != http.StatusBadRequest || !strings.Contains(ill.Body.String(), "IllegalVersioningConfigurationException") {
		t.Fatalf("illegal status want IllegalVersioning got %d %s", ill.Code, ill.Body.String())
	}

	enabled := []byte(`<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`)
	if rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/ver-neg-bucket?versioning", enabled, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	}); rec.Code != http.StatusOK {
		t.Fatalf("enable versioning %d %s", rec.Code, rec.Body.String())
	}

	suspended := []byte(`<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Suspended</Status></VersioningConfiguration>`)
	if rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/ver-neg-bucket?versioning", suspended, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	}); rec.Code != http.StatusOK {
		t.Fatalf("suspend versioning %d %s", rec.Code, rec.Body.String())
	}

	get := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/ver-neg-bucket?versioning", nil, "s3", now, nil)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "Suspended") {
		t.Fatalf("GetBucketVersioning %d %s", get.Code, get.Body.String())
	}

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/ver-neg-bucket/obj.txt", []byte("v1"), "s3", now, map[string]string{"Content-Type": "text/plain"})
	list := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/ver-neg-bucket?versions&prefix=obj", nil, "s3", now, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("ListObjectVersions %d %s", list.Code, list.Body.String())
	}

	// Missing bucket
	missing := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/no-such-ver-bucket?versioning", nil, "s3", now, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing bucket versioning want 404 got %d", missing.Code)
	}
}

func TestS3CORSLifecycleNegativesCoverage(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/cors-lc-neg", nil, "s3", now, nil)

	malCORS := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/cors-lc-neg?cors", []byte("<bad/>"), "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if malCORS.Code != http.StatusBadRequest {
		t.Fatalf("malformed cors want 400 got %d %s", malCORS.Code, malCORS.Body.String())
	}

	// Invalid CORS: empty AllowedMethod/Origin should fail closed via store validation
	emptyRule := []byte(`<CORSConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><CORSRule></CORSRule></CORSConfiguration>`)
	invCORS := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/cors-lc-neg?cors", emptyRule, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if invCORS.Code != http.StatusBadRequest {
		t.Fatalf("invalid cors want 400 got %d %s", invCORS.Code, invCORS.Body.String())
	}

	okCORS := []byte(`<CORSConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><CORSRule><AllowedOrigin>*</AllowedOrigin><AllowedMethod>GET</AllowedMethod><AllowedHeader>*</AllowedHeader><MaxAgeSeconds>60</MaxAgeSeconds></CORSRule></CORSConfiguration>`)
	if rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/cors-lc-neg?cors", okCORS, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	}); rec.Code != http.StatusOK {
		t.Fatalf("PutBucketCors %d %s", rec.Code, rec.Body.String())
	}

	malLC := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/cors-lc-neg?lifecycle", []byte("<bad/>"), "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if malLC.Code != http.StatusBadRequest {
		t.Fatalf("malformed lifecycle want 400 got %d %s", malLC.Code, malLC.Body.String())
	}

	okLC := []byte(`<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>expire</ID><Status>Enabled</Status><Filter><Prefix>tmp/</Prefix></Filter><Expiration><Days>7</Days></Expiration></Rule></LifecycleConfiguration>`)
	if rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/cors-lc-neg?lifecycle", okLC, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	}); rec.Code != http.StatusOK {
		t.Fatalf("PutBucketLifecycle %d %s", rec.Code, rec.Body.String())
	}

	getCORS := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/cors-lc-neg?cors", nil, "s3", now, nil)
	if getCORS.Code != http.StatusOK || !strings.Contains(getCORS.Body.String(), "CORSRule") {
		t.Fatalf("GetBucketCors %d %s", getCORS.Code, getCORS.Body.String())
	}
	getLC := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/cors-lc-neg?lifecycle", nil, "s3", now, nil)
	if getLC.Code != http.StatusOK || !strings.Contains(getLC.Body.String(), "LifecycleConfiguration") {
		t.Fatalf("GetLifecycleConfiguration %d %s", getLC.Code, getLC.Body.String())
	}

	noCORSBucket := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/fresh-no-cors", nil, "s3", now, nil)
	if noCORSBucket.Code != http.StatusOK {
		t.Fatalf("create fresh-no-cors bucket %d", noCORSBucket.Code)
	}
	noCORS := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/fresh-no-cors?cors", nil, "s3", now, nil)
	if noCORS.Code != http.StatusNotFound {
		t.Fatalf("GetBucketCors missing want 404 got %d", noCORS.Code)
	}

	delCORS := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/cors-lc-neg?cors", nil, "s3", now, nil)
	if delCORS.Code != http.StatusNoContent && delCORS.Code != http.StatusOK {
		t.Fatalf("DeleteBucketCors %d", delCORS.Code)
	}
	delLC := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/cors-lc-neg?lifecycle", nil, "s3", now, nil)
	if delLC.Code != http.StatusNoContent && delLC.Code != http.StatusOK {
		t.Fatalf("DeleteLifecycleConfiguration %d", delLC.Code)
	}
}
