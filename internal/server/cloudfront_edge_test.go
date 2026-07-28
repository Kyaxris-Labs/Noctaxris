package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCloudFrontEdgeFetchesS3Origin(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	const bucket = "cf-edge-bucket"
	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatal(err)
	}
	payload := []byte("edge-object-bytes")
	if _, err := st.PutObject(testAccountID, bucket, "docs/hi.txt", store.PutObjectMeta{
		Data: payload, PlainSize: int64(len(payload)), ContentType: "text/plain",
	}); err != nil {
		t.Fatal(err)
	}
	d, err := st.CreateCloudFrontDistribution(testAccountID, "lab", "edge-ref-1", true, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: bucket, OriginType: "s3",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != store.CloudFrontStatusDeployed {
		t.Fatalf("status=%q", d.Status)
	}

	edge := srv.CloudFrontEdgeHandler()
	url := "http://127.0.0.1:4566/cloudfront/" + d.ID + "/docs/hi.txt"
	req := mustNewRequest(t, http.MethodGet, url, nil)
	req.Header.Del("Content-Type")
	signS3Header(t, req, nil, testAccessKey, testSecret, testRegion, "cloudfront", now)

	rec := httptest.NewRecorder()
	edge.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edge GET status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(payload) {
		t.Fatalf("body=%q want %q", rec.Body.String(), payload)
	}
}

func TestCloudFrontEdgeRequiresSigV4(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	if _, err := st.CreateBucket(testAccountID, "cf-anon-bucket"); err != nil {
		t.Fatal(err)
	}
	d, err := st.CreateCloudFrontDistribution(testAccountID, "lab", "edge-anon", true, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: "cf-anon-bucket", OriginType: "s3",
	}})
	if err != nil {
		t.Fatal(err)
	}

	edge := srv.CloudFrontEdgeHandler()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/cloudfront/"+d.ID+"/k", nil)
	rec := httptest.NewRecorder()
	edge.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous edge status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("expected MissingAuthenticationToken in %q", rec.Body.String())
	}
}

func TestCloudFrontEdgeDisabledDistributionForbidden(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := st.CreateBucket(testAccountID, "cf-disabled-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(testAccountID, "cf-disabled-bucket", "a.txt", store.PutObjectMeta{
		Data: []byte("x"), PlainSize: 1, ContentType: "text/plain",
	}); err != nil {
		t.Fatal(err)
	}
	d, err := st.CreateCloudFrontDistribution(testAccountID, "lab", "edge-disabled", false, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: "cf-disabled-bucket", OriginType: "s3",
	}})
	if err != nil {
		t.Fatal(err)
	}

	edge := srv.CloudFrontEdgeHandler()
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/cloudfront/"+d.ID+"/a.txt", nil)
	req.Header.Del("Content-Type")
	signS3Header(t, req, nil, testAccessKey, testSecret, testRegion, "cloudfront", now)
	rec := httptest.NewRecorder()
	edge.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled edge status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
}

func TestCloudFrontEdgeMissingObjectNotFound(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := st.CreateBucket(testAccountID, "cf-miss-bucket"); err != nil {
		t.Fatal(err)
	}
	d, err := st.CreateCloudFrontDistribution(testAccountID, "lab", "edge-miss", true, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: "cf-miss-bucket", OriginType: "s3",
	}})
	if err != nil {
		t.Fatal(err)
	}

	edge := srv.CloudFrontEdgeHandler()
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/cloudfront/"+d.ID+"/nope.txt", nil)
	req.Header.Del("Content-Type")
	signS3Header(t, req, nil, testAccessKey, testSecret, testRegion, "cloudfront", now)
	rec := httptest.NewRecorder()
	edge.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing object status=%d want 404 body=%q", rec.Code, rec.Body.String())
	}
}

func TestCloudFrontEdgePathPatternOriginSelection(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	const defaultBucket = "cf-path-default"
	const imagesBucket = "cf-path-images"
	for _, b := range []string{defaultBucket, imagesBucket} {
		if _, err := st.CreateBucket(testAccountID, b); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.PutObject(testAccountID, defaultBucket, "index.html", store.PutObjectMeta{
		Data: []byte("default-origin"), PlainSize: 14, ContentType: "text/html",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(testAccountID, imagesBucket, "images/logo.png", store.PutObjectMeta{
		Data: []byte("image-origin"), PlainSize: 12, ContentType: "image/png",
	}); err != nil {
		t.Fatal(err)
	}

	d, err := st.CreateCloudFrontDistributionWithBehaviors(testAccountID, "lab", "edge-path", true,
		[]store.CloudFrontOrigin{
			{ID: "default", DomainName: defaultBucket, OriginType: "s3"},
			{ID: "images", DomainName: imagesBucket, OriginType: "s3"},
		},
		[]store.CloudFrontCacheBehavior{
			{PathPattern: "/images/*", TargetOriginId: "images"},
			{PathPattern: "*", TargetOriginId: "default"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	edge := srv.CloudFrontEdgeHandler()

	imgReq := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/cloudfront/"+d.ID+"/images/logo.png", nil)
	imgReq.Header.Del("Content-Type")
	signS3Header(t, imgReq, nil, testAccessKey, testSecret, testRegion, "cloudfront", now)
	imgRec := httptest.NewRecorder()
	edge.ServeHTTP(imgRec, imgReq)
	if imgRec.Code != http.StatusOK {
		t.Fatalf("images path status=%d body=%q", imgRec.Code, imgRec.Body.String())
	}
	if imgRec.Body.String() != "image-origin" {
		t.Fatalf("images body=%q want image-origin", imgRec.Body.String())
	}

	defReq := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/cloudfront/"+d.ID+"/index.html", nil)
	defReq.Header.Del("Content-Type")
	signS3Header(t, defReq, nil, testAccessKey, testSecret, testRegion, "cloudfront", now)
	defRec := httptest.NewRecorder()
	edge.ServeHTTP(defRec, defReq)
	if defRec.Code != http.StatusOK {
		t.Fatalf("default path status=%d body=%q", defRec.Code, defRec.Body.String())
	}
	if defRec.Body.String() != "default-origin" {
		t.Fatalf("default body=%q want default-origin", defRec.Body.String())
	}
}
