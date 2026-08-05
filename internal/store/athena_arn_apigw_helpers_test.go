package store

import (
	"bytes"
	"compress/gzip"
	"errors"
	"net"
	"testing"
)

func TestAthenaJSONAndScalarHelpersCoverage(t *testing.T) {
	cols := []GlueColumn{{Name: "id"}, {Name: "name"}}
	rows, err := parseJSONLines([]byte(`[{"id":"1","name":"alice"}]`), cols)
	if err != nil || len(rows) != 1 || rows[0][0] != "1" {
		t.Fatalf("array rows=%v err=%v", rows, err)
	}
	ctCols := []GlueColumn{{Name: "eventName"}}
	ctRows, err := parseJSONLines([]byte(`{"Records":[{"eventName":"ConsoleLogin"}]}`), ctCols)
	if err != nil || len(ctRows) != 1 || ctRows[0][0] != "ConsoleLogin" {
		t.Fatalf("records rows=%v err=%v", ctRows, err)
	}
	ndjson, err := parseJSONLines([]byte("{\"id\":\"1\"}\n{\"id\":\"2\"}\n"), []GlueColumn{{Name: "id"}})
	if err != nil || len(ndjson) != 2 {
		t.Fatalf("ndjson=%v err=%v", ndjson, err)
	}
	if _, err := parseJSONLines([]byte("not-json"), cols); err == nil {
		t.Fatal("expected parse error")
	}
	if rows, err := parseJSONLines([]byte(""), cols); err != nil || rows != nil {
		t.Fatalf("empty=%v err=%v", rows, err)
	}

	if athenaScalarString(nil) != "" {
		t.Fatal("nil scalar")
	}
	if athenaScalarString("x") != "x" || athenaScalarString(true) != "true" {
		t.Fatal("string/bool scalar")
	}
	if athenaScalarString(1.5) == "" || athenaScalarString(map[string]int{"n": 1}) == "" {
		t.Fatal("float/map scalar")
	}

	if !sqlLikeMatch("hello-world", "hello%") || sqlLikeMatch("nope", "yes%") {
		t.Fatal("like match")
	}
	if sqlLikeMatch("x", "[bad") {
		t.Fatal("bad pattern should not match")
	}

	allCols := []GlueColumn{{Name: "a"}, {Name: "b"}}
	in := [][]string{{"1", "2"}, {"3", "4"}}
	proj := projectAthenaRows(in, allCols, []GlueColumn{{Name: "b"}})
	if len(proj) != 2 || proj[0][0] != "2" {
		t.Fatalf("project=%v", proj)
	}
	same := projectAthenaRows(in, allCols, allCols)
	if len(same) != 2 {
		t.Fatalf("same project=%v", same)
	}
}

func TestAthenaDecompressObjectCoverage(t *testing.T) {
	plain := []byte("plain-text")
	out, err := athenaDecompressObject("file.txt", plain)
	if err != nil || string(out) != "plain-text" {
		t.Fatalf("plain=%q err=%v", out, err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte("gz-body"))
	_ = zw.Close()
	out, err = athenaDecompressObject("log.json.gz", gz.Bytes())
	if err != nil || string(out) != "gz-body" {
		t.Fatalf("gzip=%q err=%v", out, err)
	}
	if _, err := athenaDecompressObject("x.gz", []byte("not-gzip")); err == nil {
		t.Fatal("expected gzip error")
	}
}

func TestIAMARNHelperCoverage(t *testing.T) {
	if UserARN("000000000001", "/team/", "alice") == "" {
		t.Fatal("user arn")
	}
	if PolicyARN("000000000001", "", "ReadOnly") == "" {
		t.Fatal("policy arn")
	}
	if RoleARNWithPath("000000000001", "/service/", "lambda") == "" {
		t.Fatal("role path arn")
	}
	if GroupARN("000000000001", "", "admins") == "" {
		t.Fatal("group arn")
	}
	if InstanceProfileARN("000000000001", "/path/", "ec2") == "" {
		t.Fatal("instance profile arn")
	}
	acct, ok := arnAccountID("arn:aws:s3:::bucket")
	if ok || acct != "" {
		t.Fatalf("s3 arn account=%q ok=%v", acct, ok)
	}
	acct, ok = arnAccountID("arn:aws:iam::000000000001:user/alice")
	if !ok || acct != "000000000001" {
		t.Fatalf("iam arn account=%q ok=%v", acct, ok)
	}
	if _, ok := arnAccountID("arn:aws:iam::bad:user/x"); ok {
		t.Fatal("bad account id")
	}
}

func TestAPIGatewayProxyAndIdentityHelpersCoverage(t *testing.T) {
	if !apigwHTTPProxyEntryAllowsUnsafe("http://127.0.0.1:8080/path") {
		t.Fatal("loopback url should be unsafe")
	}
	if !apigwHTTPProxyEntryAllowsUnsafe("169.254.169.254") {
		t.Fatal("link local should be unsafe")
	}
	if apigwHTTPProxyEntryAllowsUnsafe("https://example.com") {
		t.Fatal("public host should not be unsafe entry")
	}
	if apigwHTTPProxyEntryAllowsUnsafe("not-a-url://") {
		t.Fatal("bad url")
	}
	if err := rejectAPIGatewayHTTPProxyUnsafeIP(net.ParseIP("10.0.0.1")); err == nil {
		t.Fatal("private ip reject")
	}
	if err := rejectAPIGatewayHTTPProxyUnsafeIP(net.ParseIP("8.8.8.8")); err != nil {
		t.Fatalf("public ip: %v", err)
	}
	if !isValidAPIGatewayIdentitySource("$request.header.Authorization") {
		t.Fatal("header source")
	}
	if !isValidAPIGatewayIdentitySource("$request.querystring.token") {
		t.Fatal("qs source")
	}
	if !isValidAPIGatewayIdentitySource("$context.routeKey") {
		t.Fatal("route key source")
	}
	if isValidAPIGatewayIdentitySource("$request.header.") {
		t.Fatal("empty header suffix")
	}
}

func TestNormalizeCorsAllowOriginsCoverage(t *testing.T) {
	if normalizeCorsAllowOrigins(nil) != nil {
		t.Fatal("nil origins")
	}
	out := normalizeCorsAllowOrigins([]string{" https://a.com ", "https://a.com", "", "https://b.com"})
	if len(out) != 2 || out[0] != "https://a.com" || out[1] != "https://b.com" {
		t.Fatalf("origins=%v", out)
	}
}

func TestJsonObjectRowCoverage(t *testing.T) {
	row := jsonObjectRow(map[string]any{"Name": "Alice", "count": float64(2)}, []GlueColumn{{Name: "name"}, {Name: "count"}})
	if row[0] != "Alice" || row[1] != "2" {
		t.Fatalf("row=%v", row)
	}
	if expanded := expandCloudTrailRecordsObject(map[string]any{"Records": []any{"bad"}}, nil); expanded != nil {
		t.Fatalf("bad records=%v", expanded)
	}
}

func TestAthenaBadJSONArrayCoverage(t *testing.T) {
	_, err := parseJSONLines([]byte(`[not-json]`), []GlueColumn{{Name: "x"}})
	if err == nil || !errors.Is(err, ErrAthenaBadRequest) {
		t.Fatalf("err=%v", err)
	}
}

func TestIsGlueJSONCoverage(t *testing.T) {
	if !isGlueJSON(GlueTable{SerDeInfo: GlueSerDeInfo{SerializationLibrary: "org.openx.data.jsonserde.JsonSerDe"}}) {
		t.Fatal("json serde")
	}
	if isGlueJSON(GlueTable{InputFormat: "text"}) {
		t.Fatal("non-json")
	}
}
