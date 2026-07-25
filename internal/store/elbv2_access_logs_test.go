package store_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestELBv2AccessLogObjectKeyHiveLayout(t *testing.T) {
	ts := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	key := store.ELBv2AccessLogObjectKey("111122223333", "us-east-1", "lab-alb", "prefix/", ts)
	want := "prefix/AWSLogs/111122223333/elasticloadbalancing/us-east-1/2026/07/25/111122223333_elasticloadbalancing_us-east-1_lab-alb.log"
	if key != want {
		t.Fatalf("key=%q want %q", key, want)
	}
}

func TestELBv2AppendAccessLogToS3(t *testing.T) {
	st := openS3Store(t)
	account := "111122223333"
	if _, err := st.CreateBucket(account, "alb-logs"); err != nil {
		t.Fatal(err)
	}
	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "log-alb", "internet-facing")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetELBv2LoadBalancerAttributes(account, lb.ARN, map[string]string{
		store.ELBv2AttrAccessLogsS3Enabled: "true",
		store.ELBv2AttrAccessLogsS3Bucket:  "alb-logs",
		store.ELBv2AttrAccessLogsS3Prefix:  "ingress/",
	}); err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	in := store.ELBv2AccessLogInput{
		Region:           "us-east-1",
		ClientIP:         "203.0.113.10",
		ClientPort:       12345,
		RequestMethod:    "GET",
		RequestPath:      "/api/x",
		UserAgent:        "curl/8.0",
		TargetGroupARN:   "arn:aws:elasticloadbalancing:us-east-1:111122223333:targetgroup/tg/abc",
		ElbStatusCode:    200,
		TargetStatusCode: 200,
		ReceivedBytes:    5,
		SentBytes:        12,
		RequestTime:      ts,
	}
	if err := st.AppendELBv2AccessLog(account, lb, in); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendELBv2AccessLog(account, lb, in); err != nil {
		t.Fatal(err)
	}
	key := store.ELBv2AccessLogObjectKey(account, "us-east-1", lb.Name, "ingress/", ts)
	_, data, err := st.GetObject(account, "alb-logs", key)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines=%d want 2 body=%q", len(lines), string(data))
	}
	if !strings.Contains(lines[0], `"GET /api/x HTTP/1.1"`) {
		t.Fatalf("line missing request: %q", lines[0])
	}
	if !strings.Contains(lines[0], " 200 ") {
		t.Fatalf("line missing status: %q", lines[0])
	}
}

func TestELBv2AccessLogsRejectMissingBucket(t *testing.T) {
	st := openS3Store(t)
	account := "111122223333"
	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "no-bucket-alb", "internet-facing")
	if err != nil {
		t.Fatal(err)
	}
	err = st.SetELBv2LoadBalancerAttributes(account, lb.ARN, map[string]string{
		store.ELBv2AttrAccessLogsS3Enabled: "true",
		store.ELBv2AttrAccessLogsS3Bucket:  "missing-bucket",
	})
	if err == nil {
		t.Fatal("expected error enabling access logs without bucket")
	}
}

func TestCloudFrontAppendAccessLogToS3(t *testing.T) {
	st := openS3Store(t)
	account := "111122223333"
	if _, err := st.CreateBucket(account, "cf-logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "origin"); err != nil {
		t.Fatal(err)
	}
	d, err := st.CreateCloudFrontDistribution(account, "lab", "cf-log-ref", true, []store.CloudFrontOrigin{{
		ID: "o1", DomainName: "origin", OriginType: "s3",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCloudFrontDistributionLogging(account, d.ID, store.CloudFrontLoggingConfig{
		Enabled: true,
		Bucket:  "cf-logs.s3.amazonaws.com",
		Prefix:  "cf/",
	}); err != nil {
		t.Fatal(err)
	}
	d, err = st.GetCloudFrontDistribution(account, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	if err := st.AppendCloudFrontAccessLog(account, d, store.CloudFrontAccessLogInput{
		ClientIP:    "198.51.100.2",
		Method:      "GET",
		Host:        "d123.cloudfront.noctaxris.local",
		URIStem:     "/docs/hi.txt",
		Status:      200,
		UserAgent:   "aws-cli/2.0",
		BytesSent:   4,
		TimeTaken:   0.012,
		RequestTime: ts,
		RequestID:   "req-1",
	}); err != nil {
		t.Fatal(err)
	}
	key := store.CloudFrontAccessLogObjectKey(account, d.ID, "cf/", ts)
	_, data, err := st.GetObject(account, "cf-logs", key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "GET") || !strings.Contains(string(data), "\t200\t") {
		t.Fatalf("unexpected cf log line: %q", string(data))
	}
}
