package store

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCredentialReportUserRowCoverage(t *testing.T) {
	u := User{UserName: "alice", ARN: "arn:aws:iam::1:user/alice", CreateDate: "2020-01-01T00:00:00Z"}
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	keys := []credentialReportKey{
		{
			AccessKeyID: "AKIA111",
			Status:      AccessKeyStatusActive,
			LastUsed: AccessKeyLastUsed{
				HasLastUsed:  true,
				LastUsedDate: at,
				ServiceName:  "s3",
				Region:       "us-west-2",
			},
		},
		{AccessKeyID: "AKIA222", Status: "Inactive"},
	}
	row := credentialReportUserRow(u, true, keys)
	if row[7] != "true" || row[8] != "true" {
		t.Fatalf("mfa/active key: %#v", row)
	}
	if !strings.Contains(row[10], "2026") || row[12] != "s3" {
		t.Fatalf("key1 usage: %#v", row)
	}
	if row[13] != "false" {
		t.Fatalf("inactive key2: %#v", row)
	}
	sparse := credentialReportUserRow(u, false, nil)
	if sparse[7] != "false" || sparse[8] != "false" {
		t.Fatalf("no keys: %#v", sparse)
	}
}

func TestBuildAthenaDuckSetupSQLCoverage(t *testing.T) {
	tables := []AthenaDuckTable{
		{Name: "csv_t", Location: "s3://b/prefix/", InputFormat: "TextInputFormat"},
		{Name: "pq_t", Location: "s3://b/data/file.parquet", InputFormat: "parquet", SerializationLib: "ParquetHiveSerDe"},
		{Name: "json_t", Location: "s3://b/events.json", SerializationLib: "json"},
		{Name: "", Location: "s3://skip"},
		{Name: "quoted", Location: `s3://b/o'bj/*`, InputFormat: "text"},
	}
	sql := buildAthenaDuckSetupSQL(tables)
	if !strings.Contains(sql, `read_csv_auto`) || !strings.Contains(sql, `read_parquet`) || !strings.Contains(sql, `read_json_auto`) {
		t.Fatalf("setup=%q", sql)
	}
	if strings.Contains(sql, "skip") {
		t.Fatalf("empty name should be skipped: %q", sql)
	}
	if duckReadFunction("parquet", "") != "read_parquet" {
		t.Fatal("parquet fn")
	}
	if duckReadFunction("", "json") != "read_json_auto" {
		t.Fatal("json fn")
	}
	if duckReadFunction("text", "") != "read_csv_auto" {
		t.Fatal("default fn")
	}
	rows := athenaDuckResultRows([]AthenaColumnInfo{{Name: "a", Type: "varchar"}}, [][]string{{"1"}})
	if len(rows) != 2 || rows[0][0] != "a" || rows[1][0] != "1" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestCloudFrontLoggingHelpersCoverage(t *testing.T) {
	if got := ParseCloudFrontLoggingBucket(""); got != "" {
		t.Fatalf("empty=%q", got)
	}
	if got := ParseCloudFrontLoggingBucket("s3://my-bucket/prefix/obj"); got != "my-bucket" {
		t.Fatalf("bucket=%q", got)
	}
	if got := ParseCloudFrontLoggingBucket("logs.s3.amazonaws.com.cn"); got != "logs" {
		t.Fatalf("cn=%q", got)
	}
	ts := time.Date(2026, 8, 5, 9, 30, 0, 0, time.UTC)
	key := CloudFrontAccessLogObjectKey("111122223333", "E123", "pre/", ts)
	if !strings.Contains(key, "AWSLogs/111122223333/CloudFront/2026/08/05") {
		t.Fatalf("key=%q", key)
	}
	line := FormatCloudFrontAccessLogLine(CloudFrontAccessLogInput{Status: 404})
	if !strings.Contains(line, "\tGET\t") || !strings.Contains(line, "\t404\t") {
		t.Fatalf("line=%q", line)
	}
}

func TestLightsailHelperCoverage(t *testing.T) {
	if got := lightsailParseCidrs(nil); len(got) != 1 || got[0] != "0.0.0.0/0" {
		t.Fatalf("nil cidrs=%v", got)
	}
	if got := lightsailParseCidrs([]string{}); len(got) != 1 {
		t.Fatal("empty string slice")
	}
	if got := lightsailParseCidrs([]any{"10.0.0.0/8", "  "}); len(got) != 1 || got[0] != "10.0.0.0/8" {
		t.Fatalf("any cidrs=%v", got)
	}
	n, ok := lightsailAsInt(json.Number("42"))
	if !ok || n != 42 {
		t.Fatalf("json number=%d ok=%v", n, ok)
	}
	port, err := lightsailPortFromMap(map[string]any{
		"FromPort": float64(80),
		"ToPort":   float64(80),
		"Protocol": "tcp",
		"Cidrs":    []any{"203.0.113.0/24"},
	})
	if err != nil || port.FromPort != 80 || port.Cidrs[0] != "203.0.113.0/24" {
		t.Fatalf("port=%+v err=%v", port, err)
	}
	if _, err := lightsailPortFromMap(map[string]any{"fromPort": 1}); err == nil {
		t.Fatal("expected bad port map")
	}
	if _, err := lightsailPortFromMap(map[string]any{
		"fromPort": 70000, "toPort": 80, "protocol": "tcp",
	}); err == nil {
		t.Fatal("expected port bounds error")
	}
	if _, err := lightsailPortFromMap(map[string]any{
		"fromPort": 1, "toPort": 2, "protocol": "gre",
	}); err == nil {
		t.Fatal("expected bad protocol")
	}
}

func TestEnsureSchemasNilDBCoverage(t *testing.T) {
	if err := EnsureSecurityHubSchema(nil); err == nil {
		t.Fatal("securityhub nil db")
	}
	if err := EnsureACMSchema(nil); err == nil {
		t.Fatal("acm nil db")
	}
	if err := EnsureLambdaFunctionURLSchema(nil); err == nil {
		t.Fatal("lambda url nil db")
	}
	if err := EnsureSESSchema(nil); err == nil {
		t.Fatal("ses nil db")
	}
}

func TestLabFunctionURLCoverage(t *testing.T) {
	u := LabFunctionURL("https://127.0.0.1:4566", "000000000001", "fn")
	want := "http://127.0.0.1:4566/lambda-url/000000000001/fn"
	if u != want {
		t.Fatalf("url=%q want %q", u, want)
	}
	if LabFunctionURL("", "1", "x") == "" {
		t.Fatal("default host")
	}
}
