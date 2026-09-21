package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func mustIoTREST(t *testing.T, handler http.Handler, method, rawURL, service string, body []byte, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = []byte{}
	}
	req := mustNewRequest(t, method, rawURL, body)
	req.Header.Set("Content-Type", "application/json")
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, service, now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestIoTDescribeEndpointLabAddresses(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	jsonResp := mustJSONTarget(t, handler, "AWSIotService.DescribeEndpoint", "iot", map[string]any{
		"endpointType": "iot:Data-ATS",
	}, now)
	if jsonResp.Code != http.StatusOK || !strings.Contains(jsonResp.Body.String(), `"endpointAddress":"127.0.0.1:4566"`) {
		t.Fatalf("JSON DescribeEndpoint status=%d body=%q", jsonResp.Code, jsonResp.Body.String())
	}

	types := []string{"iot:Data", "iot:Data-ATS", "iot:Jobs", "iot:CredentialProvider"}
	for _, et := range types {
		rec := mustIoTREST(t, handler, http.MethodGet,
			"http://127.0.0.1:4566/endpoint?endpointType="+et, "iot", nil, now)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%q", et, rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out["endpointAddress"] != "127.0.0.1:4566" {
			t.Fatalf("%s address=%v", et, out["endpointAddress"])
		}
	}

	bad := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/endpoint?endpointType=iot:Unknown", "iot", nil, now)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown type status=%d body=%q", bad.Code, bad.Body.String())
	}

	s3Get := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/endpoint", nil, "s3", now, nil)
	if strings.Contains(s3Get.Body.String(), "endpointAddress") {
		t.Fatalf("S3 GET /endpoint should not be DescribeEndpoint: %s", s3Get.Body.String())
	}

	retained := mustJSONTarget(t, handler, "AWSIotService.ListRetainedMessages", "iot", map[string]any{}, now)
	if retained.Code != http.StatusOK || !strings.Contains(retained.Body.String(), `"retainedTopics":[]`) {
		t.Fatalf("ListRetainedMessages status=%d body=%q", retained.Code, retained.Body.String())
	}
}

func TestIoTDescribeEndpointIoTTLSListenPort(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.IoTTLSListen = "127.0.0.1:8443"
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	types := []string{"iot:Data", "iot:Data-ATS", "iot:Jobs", "iot:CredentialProvider"}
	for _, et := range types {
		rec := mustIoTREST(t, handler, http.MethodGet,
			"http://127.0.0.1:4566/endpoint?endpointType="+et, "iot", nil, now)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%q", et, rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out["endpointAddress"] != "127.0.0.1:8443" {
			t.Fatalf("%s address=%v want 127.0.0.1:8443", et, out["endpointAddress"])
		}
	}
}

func TestIoTDescribeEndpointDistinctHosts(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.IoTEndpointHost = "lab.example.local"
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	cases := map[string]string{
		"iot:Data":               "data.iot.lab.example.local:4566",
		"iot:Data-ATS":           "data-ats.iot.lab.example.local:4566",
		"iot:Jobs":               "jobs.iot.lab.example.local:4566",
		"iot:CredentialProvider": "credentials.iot.lab.example.local:4566",
	}
	for et, want := range cases {
		rec := mustIoTREST(t, handler, http.MethodGet,
			"http://127.0.0.1:4566/endpoint?endpointType="+et, "iot", nil, now)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"endpointAddress":"`+want+`"`) {
			t.Fatalf("%s status=%d body=%q want %s", et, rec.Code, rec.Body.String(), want)
		}
	}
}

func TestIoTDescribeEndpointDistinctHostsIoTTLSListen(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.IoTEndpointHost = "lab.example.local"
		cfg.IoTTLSListen = "127.0.0.1:8443"
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	cases := map[string]string{
		"iot:Data":               "data.iot.lab.example.local:8443",
		"iot:Data-ATS":           "data-ats.iot.lab.example.local:8443",
		"iot:Jobs":               "jobs.iot.lab.example.local:8443",
		"iot:CredentialProvider": "credentials.iot.lab.example.local:8443",
	}
	for et, want := range cases {
		rec := mustIoTREST(t, handler, http.MethodGet,
			"http://127.0.0.1:4566/endpoint?endpointType="+et, "iot", nil, now)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"endpointAddress":"`+want+`"`) {
			t.Fatalf("%s status=%d body=%q want %s", et, rec.Code, rec.Body.String(), want)
		}
	}
}
