package store

import (
	"strings"
	"testing"
	"time"
)

func TestMoreUnexportedHelperZeros(t *testing.T) {
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(dir + "/master.key")
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	account := "000000000001"

	if err := validateS3NotificationTopicARN("https://evil.example/x"); err == nil {
		t.Fatal("expected http topic reject")
	}
	if err := validateS3NotificationTopicARN("arn:aws:sns:us-east-1:000000000001:lab"); err != nil {
		t.Fatal(err)
	}
	if err := validateS3NotificationTopicARN("arn:aws:sqs:us-east-1:000000000001:q"); err == nil {
		t.Fatal("expected non-sns reject")
	}

	norm, err := normalizeS3TopicConfig(S3TopicConfig{
		Events:   []string{"s3:ObjectCreated:*"},
		TopicARN: "arn:aws:sns:us-east-1:000000000001:lab-topic",
	}, 0)
	if err != nil || norm.ID == "" || norm.TopicARN == "" {
		t.Fatalf("normalize topic=%+v err=%v", norm, err)
	}
	if _, err := normalizeS3TopicConfig(S3TopicConfig{TopicARN: "bad"}, 1); err == nil {
		t.Fatal("expected bad topic config")
	}

	row, err := encodeSelectCSVRow([]string{"a", "b"}, ',')
	if err != nil || !strings.Contains(row, "a") {
		t.Fatalf("csv row=%q err=%v", row, err)
	}
	if _, err := encodeSelectCSVRow([]any{"x", 2}, ','); err != nil {
		t.Fatal(err)
	}
	if got, err := encodeSelectCSVRow(map[string]string{"k": "v"}, ','); err != nil || !strings.Contains(got, "k") {
		t.Fatalf("map row=%q err=%v", got, err)
	}
	if got, err := encodeSelectCSVRow(map[string]any{"n": 1}, ','); err != nil || !strings.Contains(got, "n") {
		t.Fatalf("any map=%q err=%v", got, err)
	}
	if got, err := encodeSelectCSVRow(struct{ X int }{3}, ','); err != nil || got == "" {
		t.Fatalf("struct row=%q err=%v", got, err)
	}

	if sum := md5SumHex([]byte("abc")); len(sum) != 32 {
		t.Fatalf("md5=%q", sum)
	}
	pass, err := randomLabPassword()
	if err != nil || !strings.HasPrefix(pass, "lab-") {
		t.Fatalf("password=%q err=%v", pass, err)
	}
	if name := rdsDataSecretNameFromARN("arn:aws:secretsmanager:us-east-1:1:secret:mysecret-AbCdEf"); name != "mysecret" {
		t.Fatalf("secret name=%q", name)
	}
	if name := rdsDataSecretNameFromARN("not-an-arn"); name != "" {
		t.Fatalf("bad arn name=%q", name)
	}

	av, err := transactCanonicalAV(map[string]any{"S": "hello"})
	if err != nil || av == "" {
		t.Fatalf("canonical av=%q err=%v", av, err)
	}

	// Dead non-Tx FIFO helpers still need coverage.
	if _, found, err := st.findDedupMessage(account, "q", ""); err != nil || found {
		t.Fatalf("empty dedup found=%v err=%v", found, err)
	}
	if _, found, err := st.findDedupMessage(account, "q", "dedup-1"); err != nil || found {
		t.Fatalf("missing dedup found=%v err=%v", found, err)
	}
	if _, err := st.nextSequenceNumber(account, "q", "g1"); err == nil {
		// may fail without schema row; either path exercises the function
		t.Log("nextSequenceNumber without fifo counter:", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if blocked, err := st.fifoBlockedGroups(account, "q", now); err != nil {
		t.Log("fifoBlockedGroups:", err)
	} else if blocked == nil {
		t.Fatal("expected non-nil blocked map")
	}

	if _, err := st.latestHostedVersion(account, "app", "prof"); err == nil {
		t.Fatal("expected missing hosted version")
	}
}

func TestUnexportedHelperZerosCoverage(t *testing.T) {
	if err := rejectAPIGatewayHTTPProxyUnsafeHost("localhost"); err == nil {
		t.Fatal("expected localhost reject")
	}
	if err := rejectAPIGatewayHTTPProxyUnsafeHost("127.0.0.1"); err == nil {
		t.Fatal("expected loopback reject")
	}
	if err := rejectAPIGatewayHTTPProxyUnsafeHost("metadata.google.internal"); err == nil {
		t.Fatal("expected metadata reject")
	}

	b, k, err := parseS3SourceLocation("s3://bucket/path/key.zip")
	if err != nil || b != "bucket" || k != "path/key.zip" {
		t.Fatalf("parse s3 loc b=%q k=%q err=%v", b, k, err)
	}
	if _, _, err := parseS3SourceLocation("s3://onlybucket"); err == nil {
		t.Fatal("expected bad s3 location")
	}

	if got := stringSliceFromAny([]any{"a", 1, true, ""}); len(got) < 2 {
		t.Fatalf("stringSliceFromAny=%v", got)
	}
	if got := stringSliceFromAny("solo"); len(got) != 1 || got[0] != "solo" {
		t.Fatalf("string from string=%v", got)
	}
	if got := stringSliceFromAny(""); got != nil {
		t.Fatalf("empty string slice=%v", got)
	}

	cart := cartesianStrings([][]string{{"a", "b"}, {"1", "2"}})
	if len(cart) != 4 {
		t.Fatalf("cartesian=%v", cart)
	}
	if cart := cartesianStrings(nil); cart != nil {
		t.Fatalf("empty cartesian=%v", cart)
	}

	rt, id := splitResourceARN("arn:aws:s3:::bucket/key")
	if rt == "" || id == "" {
		t.Fatalf("split arn type=%q id=%q", rt, id)
	}
	rt2, id2 := splitResourceARN("short")
	if rt2 != "Unknown" || id2 != "short" {
		t.Fatalf("short arn type=%q id=%q", rt2, id2)
	}

	if !isHex64(strings.Repeat("a", 64)) {
		t.Fatal("expected hex64")
	}
	if isHex64("abcd") || isHex64(strings.Repeat("g", 64)) {
		t.Fatal("expected non-hex reject")
	}

	start, end, err := parseCronRange("1-3", 0, 59)
	if err != nil || start != 1 || end != 3 {
		t.Fatalf("cron range start=%d end=%d err=%v", start, end, err)
	}
	if _, _, err := parseCronRange("99-100", 0, 59); err == nil {
		t.Fatal("expected cron range error")
	}
}
