package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/server"
)

func TestParseTransferLabHomePath(t *testing.T) {
	sid, user, rel, ok := server.ParseTransferLabHomePath("/transfer/s-abc/home/alice/inbox/a.txt")
	if !ok || sid != "s-abc" || user != "alice" || rel != "inbox/a.txt" {
		t.Fatalf("got sid=%q user=%q rel=%q ok=%v", sid, user, rel, ok)
	}
	if !server.IsTransferLabHomePath("/transfer/s-abc/home/alice") {
		t.Fatal("expected home root path")
	}
	if server.IsTransferLabHomePath("/not-transfer/x") {
		t.Fatal("unexpected match")
	}
}

func TestTransferLabFileJSONRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustTransferJSON(t, handler, "CreateServer", map[string]any{"Protocols": []string{"SFTP"}}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateServer status=%d body=%q", create.Code, create.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["ServerId"].(string)

	desc := mustTransferJSON(t, handler, "DescribeServer", map[string]any{"ServerId": id}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeServer status=%d body=%q", desc.Code, desc.Body.String())
	}
	if !strings.Contains(desc.Body.String(), `"State":"ONLINE"`) && !strings.Contains(desc.Body.String(), `"State": "ONLINE"`) {
		t.Fatalf("want ONLINE in describe body=%q", desc.Body.String())
	}

	user := mustTransferJSON(t, handler, "CreateUser", map[string]any{
		"ServerId": id, "UserName": "alice",
	}, now)
	if user.Code != http.StatusOK {
		t.Fatalf("CreateUser status=%d body=%q", user.Code, user.Body.String())
	}

	put := mustTransferJSON(t, handler, "PutFile", map[string]any{
		"ServerId": id, "UserName": "alice", "Path": "docs/readme.txt", "Body": "hello-transfer",
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutFile status=%d body=%q", put.Code, put.Body.String())
	}

	get := mustTransferJSON(t, handler, "GetFile", map[string]any{
		"ServerId": id, "UserName": "alice", "Path": "docs/readme.txt",
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetFile status=%d body=%q", get.Code, get.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	b64, _ := got["BodyBase64"].(string)
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "hello-transfer" {
		t.Fatalf("body=%q", raw)
	}

	bad := mustTransferJSON(t, handler, "PutFile", map[string]any{
		"ServerId": id, "UserName": "alice", "Path": "../escape.txt", "Body": "nope",
	}, now)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("traversal put status=%d body=%q", bad.Code, bad.Body.String())
	}
}

func TestTransferLabHomeHTTPRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustTransferJSON(t, handler, "CreateServer", map[string]any{"Protocols": []string{"SFTP"}}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateServer status=%d body=%q", create.Code, create.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["ServerId"].(string)
	if id == "" {
		t.Fatal("missing ServerId")
	}
	user := mustTransferJSON(t, handler, "CreateUser", map[string]any{
		"ServerId": id, "UserName": "carol",
	}, now)
	if user.Code != http.StatusOK {
		t.Fatalf("CreateUser status=%d body=%q", user.Code, user.Body.String())
	}

	payload := []byte("hello-transfer-http")
	homeURL := "http://127.0.0.1:4566/transfer/" + id + "/home/carol/inbox/note.txt"
	putReq := mustNewRequest(t, http.MethodPut, homeURL, payload)
	putReq.Header.Set("Content-Type", "text/plain")
	signHeader(t, putReq, payload, testAccessKey, testSecret, testRegion, "transfer", now)
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("HTTP PutFile path status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	getReq := mustNewRequest(t, http.MethodGet, homeURL, nil)
	signHeader(t, getReq, nil, testAccessKey, testSecret, testRegion, "transfer", now)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("HTTP GetFile path status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("GetFile JSON: %v body=%q", err, getRec.Body.String())
	}
	b64, _ := got["BodyBase64"].(string)
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(payload) {
		t.Fatalf("HTTP roundtrip body=%q want %q", raw, payload)
	}
}
