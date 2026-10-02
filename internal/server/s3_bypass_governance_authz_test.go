package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3DeleteObjectBypassGovernanceRequiresAction(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	userName := "s3-bypass-limited"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowDeleteOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:CreateBucket","s3:PutObject","s3:DeleteObject","s3:GetObject","s3:ListBucket"],"Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "S3DeleteNoBypass", allowDeleteOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}

	bucket := "bypass-gov-bucket"
	key := "locked.txt"
	if _, err := st.CreateBucketWithOptions(testAccountID, bucket, store.CreateBucketOptions{ObjectLockEnabled: true}); err != nil {
		t.Fatal(err)
	}
	retain := now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := st.PutObject(testAccountID, bucket, key, store.PutObjectMeta{
		Data:                  []byte("locked-object"),
		ContentType:           "text/plain",
		PlainSize:             int64(len("locked-object")),
		ObjectLockMode:        "GOVERNANCE",
		ObjectLockRetainUntil: retain,
	}); err != nil {
		t.Fatal(err)
	}

	delNoBypass := mustNewRequest(t, http.MethodDelete, "http://127.0.0.1:4566/"+bucket+"/"+key, nil)
	delNoBypass.Header.Del("Content-Type")
	signS3Header(t, delNoBypass, nil, akid, secret, testRegion, "s3", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, delNoBypass)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("Delete without bypass status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}

	delBypass := mustNewRequest(t, http.MethodDelete, "http://127.0.0.1:4566/"+bucket+"/"+key, nil)
	delBypass.Header.Del("Content-Type")
	delBypass.Header.Set("x-amz-bypass-governance-retention", "true")
	signS3Header(t, delBypass, nil, akid, secret, testRegion, "s3", now)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, delBypass)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("Delete bypass without action status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}

	allowBypass := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:DeleteObject","s3:BypassGovernanceRetention"],"Resource":"*"}]}`
	bypassARN, err := st.CreateManagedPolicy(testAccountID, "S3BypassGov", allowBypass)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, bypassARN); err != nil {
		t.Fatal(err)
	}

	delBypassOK := mustNewRequest(t, http.MethodDelete, "http://127.0.0.1:4566/"+bucket+"/"+key, nil)
	delBypassOK.Header.Del("Content-Type")
	delBypassOK.Header.Set("x-amz-bypass-governance-retention", "true")
	signS3Header(t, delBypassOK, nil, akid, secret, testRegion, "s3", now)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, delBypassOK)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("Delete bypass with action status=%d want 204 body=%q", rec.Code, rec.Body.String())
	}
}

func TestS3BypassGovernanceHeaderFalseDoesNotRequireAction(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	bucket := "no-bypass-hdr"
	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatal(err)
	}
	body := []byte("plain")
	if _, err := st.PutObject(testAccountID, bucket, "plain.txt", store.PutObjectMeta{
		Data: body, PlainSize: int64(len(body)), ContentType: "text/plain",
	}); err != nil {
		t.Fatal(err)
	}

	del := mustNewRequest(t, http.MethodDelete, "http://127.0.0.1:4566/"+bucket+"/plain.txt", nil)
	del.Header.Del("Content-Type")
	del.Header.Set("x-amz-bypass-governance-retention", "false")
	signS3Header(t, del, nil, testAccessKey, testSecret, testRegion, "s3", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, del)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("Delete status=%d want 204 body=%q", rec.Code, rec.Body.String())
	}
}
