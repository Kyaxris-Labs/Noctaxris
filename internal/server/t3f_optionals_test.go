package server_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustRoute53InjectQueryLogs(t *testing.T, handler http.Handler, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "NoctaxrisRoute53.InjectQueryLogs")
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "route53", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRoute53InjectQueryLogsDisabled(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustRoute53InjectQueryLogs(t, handler, map[string]any{
		"LogGroupName": "/aws/route53/querylogs",
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q want 403", rec.Code, rec.Body.String())
	}
}

func TestRoute53InjectQueryLogsPutLogEvents(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.Route53QueryLogInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	group := "/aws/route53/lab-query"
	stream := store.Route53QueryLabLogStreamName(now)

	line := `{"version":"1.100000","query_name":"test.example.com.","rcode":"NOERROR"}`
	rec := mustRoute53InjectQueryLogs(t, handler, map[string]any{
		"LogGroupName":  group,
		"LogStreamName": stream,
		"Lines":         []any{line},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("inject status=%d body=%q", rec.Code, rec.Body.String())
	}

	events, err := st.GetLogEvents(testAccountID, group, stream, 0, 0, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Message != line {
		t.Fatalf("log events=%+v want line %q", events, line)
	}
}

func TestFirehoseVpcFlowDestinationWritesV2Line(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	bucket := "fh-vpcflow-lab"

	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"s3:PutObject","Resource":"*"}]}`
	if err := st.PutBucketPolicy(testAccountID, bucket, policy); err != nil {
		t.Fatal(err)
	}

	create := mustJSONTarget(t, handler, "Firehose_20150804.CreateDeliveryStream", "firehose", map[string]any{
		"DeliveryStreamName": "vpc-flow-stream",
		"VpcFlowLogsDestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::" + bucket,
			"Prefix":    "flows/",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("create stream status=%d body=%q", create.Code, create.Body.String())
	}

	v2 := store.FormatVPCFlowLogV2Line(store.VPCFlowRecord{
		Version: 2, AccountID: testAccountID, InterfaceID: "eni-abc",
		SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 1234, DstPort: 443,
		Protocol: 6, Packets: 1, Bytes: 60, Start: now.Unix(), End: now.Unix() + 1,
		Action: "ACCEPT", LogStatus: "OK",
	})
	data := base64.StdEncoding.EncodeToString([]byte(v2))
	put := mustJSONTarget(t, handler, "Firehose_20150804.PutRecord", "firehose", map[string]any{
		"DeliveryStreamName": "vpc-flow-stream",
		"Record":             map[string]any{"Data": data},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("put record status=%d body=%q", put.Code, put.Body.String())
	}

	list, err := st.ListObjectsV2(testAccountID, bucket, "flows/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Contents) == 0 {
		t.Fatal("expected S3 object from VPC flow Firehose delivery")
	}
	_, body, err := st.GetObject(testAccountID, bucket, list.Contents[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(v2)) {
		t.Fatalf("object body=%q want v2 line %q", body, v2)
	}
}

func TestControlTowerHonestStub(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	list := mustJSONTarget(t, handler, "ControlTower.ListLandingZones", "controltower", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%q", list.Code, list.Body.String())
	}
	var listParsed map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &listParsed); err != nil {
		t.Fatal(err)
	}
	zones, _ := listParsed["landingZones"].([]any)
	if len(zones) != 0 {
		t.Fatalf("landingZones=%v want empty", zones)
	}

	get := mustJSONTarget(t, handler, "ControlTower.GetLandingZone", "controltower", map[string]any{
		"landingZoneIdentifier": "arn:aws:controltower:us-east-1:000000000001:landingzone/lz-deadbeef",
	}, now)
	if get.Code != http.StatusBadRequest {
		t.Fatalf("get status=%d body=%q want 400", get.Code, get.Body.String())
	}
	if !strings.Contains(get.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("body=%q want not found", get.Body.String())
	}
}
