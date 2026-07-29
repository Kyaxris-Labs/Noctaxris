package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestS3BucketCorsPutGetDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/cors-api-bucket", nil, "s3", now, nil)

	getMissing := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/cors-api-bucket?cors", nil, "s3", now, nil)
	if getMissing.Code != http.StatusNotFound || !strings.Contains(getMissing.Body.String(), "NoSuchCORSConfiguration") {
		t.Fatalf("GetBucketCors missing status=%d body=%q", getMissing.Code, getMissing.Body.String())
	}

	body := []byte(`<CORSConfiguration><CORSRule><AllowedOrigin>http://www.example.com</AllowedOrigin><AllowedMethod>GET</AllowedMethod><AllowedMethod>PUT</AllowedMethod><AllowedHeader>*</AllowedHeader><MaxAgeSeconds>3000</MaxAgeSeconds></CORSRule></CORSConfiguration>`)
	putRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/cors-api-bucket?cors", body, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutBucketCors status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	getRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/cors-api-bucket?cors", nil, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetBucketCors status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	got := getRec.Body.String()
	if !strings.Contains(got, "<AllowedOrigin>http://www.example.com</AllowedOrigin>") ||
		!strings.Contains(got, "<AllowedMethod>GET</AllowedMethod>") ||
		!strings.Contains(got, "<MaxAgeSeconds>3000</MaxAgeSeconds>") {
		t.Fatalf("unexpected cors xml=%q", got)
	}

	delRec := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/cors-api-bucket?cors", nil, "s3", now, nil)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DeleteBucketCors status=%d body=%q", delRec.Code, delRec.Body.String())
	}
}

func TestS3LifecycleConfigurationPutGetDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/lc-api-bucket", nil, "s3", now, nil)

	getMissing := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/lc-api-bucket?lifecycle", nil, "s3", now, nil)
	if getMissing.Code != http.StatusNotFound || !strings.Contains(getMissing.Body.String(), "NoSuchLifecycleConfiguration") {
		t.Fatalf("GetLifecycle missing status=%d body=%q", getMissing.Code, getMissing.Body.String())
	}

	body := []byte(`<LifecycleConfiguration><Rule><ID>expire-tmp</ID><Status>Enabled</Status><Filter><Prefix>tmp/</Prefix></Filter><Expiration><Days>7</Days></Expiration></Rule></LifecycleConfiguration>`)
	putRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/lc-api-bucket?lifecycle", body, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutLifecycle status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	getRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/lc-api-bucket?lifecycle", nil, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetLifecycle status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	got := getRec.Body.String()
	if !strings.Contains(got, "<ID>expire-tmp</ID>") ||
		!strings.Contains(got, "<Prefix>tmp/</Prefix>") ||
		!strings.Contains(got, "<Days>7</Days>") {
		t.Fatalf("unexpected lifecycle xml=%q", got)
	}

	delRec := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/lc-api-bucket?lifecycle", nil, "s3", now, nil)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DeleteLifecycle status=%d body=%q", delRec.Code, delRec.Body.String())
	}
}

func TestS3SelectObjectContentCSVAndFailClosed(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/select-bucket", nil, "s3", now, nil)
	csvPayload := []byte("name,age\nalice,30\nbob,40\n")
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/select-bucket/people.csv", csvPayload, "s3", now, map[string]string{
		"Content-Type": "text/csv",
	})

	selectBody := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<SelectObjectContentRequest>
  <Expression>SELECT * FROM s3object LIMIT 1</Expression>
  <ExpressionType>SQL</ExpressionType>
  <InputSerialization><CSV><FileHeaderInfo>USE</FileHeaderInfo></CSV></InputSerialization>
  <OutputSerialization><JSON></JSON></OutputSerialization>
</SelectObjectContentRequest>`)
	selectRec := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/select-bucket/people.csv?select&select-type=2", selectBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if selectRec.Code != http.StatusOK {
		t.Fatalf("SelectObjectContent status=%d body=%q", selectRec.Code, selectRec.Body.String())
	}
	var resp struct {
		Records []json.RawMessage `json:"Records"`
		End     bool              `json:"End"`
	}
	if err := json.Unmarshal(selectRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%q", err, selectRec.Body.String())
	}
	if !resp.End || len(resp.Records) != 1 {
		t.Fatalf("resp=%+v", resp)
	}
	var row map[string]string
	if err := json.Unmarshal(resp.Records[0], &row); err != nil {
		t.Fatal(err)
	}
	if row["name"] != "alice" || row["age"] != "30" {
		t.Fatalf("row=%v", row)
	}

	badBody := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<SelectObjectContentRequest>
  <Expression>SELECT name FROM s3object WHERE age &gt; 10</Expression>
  <ExpressionType>SQL</ExpressionType>
  <InputSerialization><CSV><FileHeaderInfo>USE</FileHeaderInfo></CSV></InputSerialization>
  <OutputSerialization><JSON></JSON></OutputSerialization>
</SelectObjectContentRequest>`)
	badRec := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/select-bucket/people.csv?select&select-type=2", badBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if badRec.Code != http.StatusBadRequest || !strings.Contains(badRec.Body.String(), "InvalidRequest") {
		t.Fatalf("unsupported SQL status=%d body=%q", badRec.Code, badRec.Body.String())
	}
}
