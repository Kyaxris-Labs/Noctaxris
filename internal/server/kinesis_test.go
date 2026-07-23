package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustKinesisJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Kinesis_20131202."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "kinesis", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestKinesisRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustKinesisJSON(t, handler, "CreateStream", map[string]any{
		"StreamName": "lab-k",
		"ShardCount": 1,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStream status=%d body=%q", create.Code, create.Body.String())
	}

	put := mustKinesisJSON(t, handler, "PutRecord", map[string]any{
		"StreamName":   "lab-k",
		"PartitionKey": "pk",
		"Data":         base64.StdEncoding.EncodeToString([]byte("payload")),
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutRecord status=%d body=%q", put.Code, put.Body.String())
	}

	desc := mustKinesisJSON(t, handler, "DescribeStream", map[string]any{
		"StreamName": "lab-k",
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeStream status=%d body=%q", desc.Code, desc.Body.String())
	}

	itRec := mustKinesisJSON(t, handler, "GetShardIterator", map[string]any{
		"StreamName":        "lab-k",
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
	if iterator == "" {
		t.Fatalf("missing iterator: %q", itRec.Body.String())
	}

	get := mustKinesisJSON(t, handler, "GetRecords", map[string]any{
		"ShardIterator": iterator,
		"Limit":         10,
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetRecords status=%d body=%q", get.Code, get.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	recs, _ := getOut["Records"].([]any)
	if len(recs) != 1 {
		t.Fatalf("records=%v", getOut["Records"])
	}

	list := mustKinesisJSON(t, handler, "ListStreams", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListStreams status=%d body=%q", list.Code, list.Body.String())
	}

	del := mustKinesisJSON(t, handler, "DeleteStream", map[string]any{
		"StreamName": "lab-k",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteStream status=%d body=%q", del.Code, del.Body.String())
	}
}
