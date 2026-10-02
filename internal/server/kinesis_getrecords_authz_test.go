package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKinesisGetRecordsAuthorizesStreamARN(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	stream := "getrec-stream"
	create := mustKinesisJSON(t, handler, "CreateStream", map[string]any{
		"StreamName": stream,
		"ShardCount": 1,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStream status=%d body=%q", create.Code, create.Body.String())
	}
	put := mustKinesisJSON(t, handler, "PutRecord", map[string]any{
		"StreamName":   stream,
		"PartitionKey": "pk",
		"Data":         base64.StdEncoding.EncodeToString([]byte("payload")),
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutRecord status=%d body=%q", put.Code, put.Body.String())
	}
	itRec := mustKinesisJSON(t, handler, "GetShardIterator", map[string]any{
		"StreamName":        stream,
		"ShardId":           store.LabKinesisShardID(0),
		"ShardIteratorType": "TRIM_HORIZON",
	}, now)
	if itRec.Code != http.StatusOK {
		t.Fatalf("GetShardIterator status=%d body=%q", itRec.Code, itRec.Body.String())
	}
	var itOut map[string]any
	if err := json.Unmarshal(itRec.Body.Bytes(), &itOut); err != nil {
		t.Fatal(err)
	}
	iterator, _ := itOut["ShardIterator"].(string)

	userName := "kinesis-getrec-limited"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	streamARN := store.KinesisStreamARN(testRegion, testAccountID, stream)
	allowOther := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kinesis:GetRecords","Resource":"arn:aws:kinesis:` + testRegion + `:` + testAccountID + `:stream/other-stream"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "KinesisGetRecOther", allowOther)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}

	raw, _ := json.Marshal(map[string]any{"ShardIterator": iterator, "Limit": 10})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Kinesis_20131202.GetRecords")
	signHeader(t, req, raw, akid, secret, testRegion, "kinesis", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("GetRecords wrong-stream Allow status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}

	allowStream := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kinesis:GetRecords","Resource":"` + streamARN + `"}]}`
	okARN, err := st.CreateManagedPolicy(testAccountID, "KinesisGetRecOK", allowStream)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, okARN); err != nil {
		t.Fatal(err)
	}
	reqOK := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	reqOK.Header.Set("Content-Type", "application/x-amz-json-1.1")
	reqOK.Header.Set("X-Amz-Target", "Kinesis_20131202.GetRecords")
	signHeader(t, reqOK, raw, akid, secret, testRegion, "kinesis", now)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, reqOK)
	if okRec.Code != http.StatusOK {
		t.Fatalf("GetRecords stream ARN Allow status=%d body=%q", okRec.Code, okRec.Body.String())
	}
}

func TestKinesisGetRecordsRejectsForeignAccountIterator(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	stream := "acct-bound-stream"
	if _, err := st.CreateKinesisStream(testAccountID, testRegion, stream, 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutKinesisRecord(testAccountID, stream, "pk", []byte("x")); err != nil {
		t.Fatal(err)
	}
	it, err := st.GetKinesisShardIterator(testAccountID, stream, store.LabKinesisShardID(0), "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}

	otherAccount := "000000000099"
	if err := st.EnsureRoot(otherAccount, "AKIOTHERACCOUNT099", "other-secret-value"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(otherAccount, "foreign"); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(otherAccount, "foreign")
	if err != nil {
		t.Fatal(err)
	}
	allowAll := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kinesis:*","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(otherAccount, "KinesisAll", allowAll)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(otherAccount, "foreign", polARN); err != nil {
		t.Fatal(err)
	}

	raw, _ := json.Marshal(map[string]any{"ShardIterator": it})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Kinesis_20131202.GetRecords")
	signHeader(t, req, raw, akid, secret, testRegion, "kinesis", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign account GetRecords status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
}
