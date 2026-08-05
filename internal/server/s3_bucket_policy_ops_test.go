package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestS3BucketPolicyEncryptionOpsAndHeadMissing(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	bucket := "policy-ops-bucket"
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket, nil, "s3", now, nil)

	headOK := mustS3(t, handler, http.MethodHead, "http://127.0.0.1:4566/"+bucket, nil, "s3", now, nil)
	if headOK.Code != http.StatusOK {
		t.Fatalf("HeadBucket status=%d body=%q", headOK.Code, headOK.Body.String())
	}

	headMiss := mustS3(t, handler, http.MethodHead, "http://127.0.0.1:4566/no-such-policy-ops-bucket", nil, "s3", now, nil)
	if headMiss.Code != http.StatusNotFound || !strings.Contains(headMiss.Body.String(), "NoSuchBucket") {
		t.Fatalf("HeadBucket missing status=%d body=%q", headMiss.Code, headMiss.Body.String())
	}

	policy := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":"*"}]}`)
	putPol := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"?policy", policy, "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	getPol := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+bucket+"?policy", nil, "s3", now, nil)
	if getPol.Code != http.StatusOK || !strings.Contains(getPol.Body.String(), "GetObject") {
		t.Fatalf("GetBucketPolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}
	delPol := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/"+bucket+"?policy", nil, "s3", now, nil)
	if delPol.Code != http.StatusOK && delPol.Code != http.StatusNoContent {
		t.Fatalf("DeleteBucketPolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}
	getPolGone := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+bucket+"?policy", nil, "s3", now, nil)
	if getPolGone.Code == http.StatusOK {
		t.Fatalf("GetBucketPolicy after delete should fail: %q", getPolGone.Body.String())
	}

	encBody := []byte(`<ServerSideEncryptionConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>AES256</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`)
	putEnc := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"?encryption", encBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putEnc.Code != http.StatusOK {
		t.Fatalf("PutBucketEncryption status=%d body=%q", putEnc.Code, putEnc.Body.String())
	}
	getEnc := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+bucket+"?encryption", nil, "s3", now, nil)
	if getEnc.Code != http.StatusOK || !strings.Contains(getEnc.Body.String(), "AES256") {
		t.Fatalf("GetBucketEncryption status=%d body=%q", getEnc.Code, getEnc.Body.String())
	}
	delEnc := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/"+bucket+"?encryption", nil, "s3", now, nil)
	if delEnc.Code != http.StatusOK && delEnc.Code != http.StatusNoContent {
		t.Fatalf("DeleteBucketEncryption status=%d body=%q", delEnc.Code, delEnc.Body.String())
	}
	getEncGone := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+bucket+"?encryption", nil, "s3", now, nil)
	if getEncGone.Code == http.StatusOK {
		t.Fatalf("GetBucketEncryption after delete should fail: %q", getEncGone.Body.String())
	}

	putPolMiss := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/no-such-policy-ops-bucket?policy", policy, "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if putPolMiss.Code != http.StatusNotFound || !strings.Contains(putPolMiss.Body.String(), "NoSuchBucket") {
		t.Fatalf("PutBucketPolicy missing bucket status=%d body=%q", putPolMiss.Code, putPolMiss.Body.String())
	}
	putEncMiss := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/no-such-policy-ops-bucket?encryption", encBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putEncMiss.Code != http.StatusNotFound || !strings.Contains(putEncMiss.Body.String(), "NoSuchBucket") {
		t.Fatalf("PutBucketEncryption missing bucket status=%d body=%q", putEncMiss.Code, putEncMiss.Body.String())
	}

	badPolicy := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"?policy", []byte("not-json"), "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if badPolicy.Code != http.StatusBadRequest {
		t.Fatalf("PutBucketPolicy malformed status=%d body=%q", badPolicy.Code, badPolicy.Body.String())
	}
}
