package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMapS3StoreErrorBranches(t *testing.T) {
	srv, _ := accessKeyPlumbServer(t)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/bucket/key", nil)
	verified := &authn.Verified{AccountID: "000000000001", AccessKeyID: "AKIAROOTEXAMPLE01", Region: "us-east-1"}

	rec := httptest.NewRecorder()
	if !srv.mapS3StoreError(rec, req, "rid", "eid", verified, false, store.ErrNoSuchBucket, "GetObject") {
		t.Fatal("NoSuchBucket should map")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("NoSuchBucket code %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	if !srv.mapS3StoreError(rec, req, "rid", "eid", verified, false, store.ErrInvalidObjectKey, "PutObject") {
		t.Fatal("InvalidObjectKey should map")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("InvalidObjectKey code %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	if !srv.mapS3StoreError(rec, req, "rid", "eid", verified, false, store.ErrInvalidBucketName, "CreateBucket") {
		t.Fatal("InvalidBucketName should map")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("InvalidBucketName code %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	if !srv.mapS3StoreError(rec, req, "rid", "eid", verified, false, errors.New("boom"), "PutObject") {
		t.Fatal("generic should map")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("generic code %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	if srv.mapS3StoreError(rec, req, "rid", "eid", verified, false, nil, "PutObject") {
		t.Fatal("nil err should not map")
	}
}

func TestWriteSQSEncryptErrorBranches(t *testing.T) {
	srv, _ := accessKeyPlumbServer(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/", nil)
	verified := &authn.Verified{AccountID: "000000000001", Region: "us-east-1"}

	rec := httptest.NewRecorder()
	srv.writeSQSEncryptError(rec, req, "rid", "eid", verified, false, errSQSAccessDenied)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("access denied status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.writeSQSEncryptError(rec, req, "rid", "eid", verified, false, errors.New("kms boom"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("kms encrypt status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestWriteCURAndCWErrorHelpers(t *testing.T) {
	s, _ := accessKeyPlumbServer(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/", nil)
	verified := &authn.Verified{
		AccountID:   "000000000001",
		AccessKeyID: "AKIAROOTEXAMPLE01",
		Region:      "us-east-1",
		Principal:   identity.RootPrincipal("000000000001", "AKIAROOTEXAMPLE01"),
	}

	rec := httptest.NewRecorder()
	s.writeCURError(rec, req, nil, "rid", 400, "ValidationException", "bad", false, "eid", verified)
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("cur %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeCWError(rec, req, nil, "rid", 400, "ValidationException", "bad", false, "eid", verified)
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("cw %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeEC2Error(rec, req, "rid", 400, "InvalidParameterValue", "bad", false, "eid", verified)
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("ec2 %d", rec.Code)
	}
}
