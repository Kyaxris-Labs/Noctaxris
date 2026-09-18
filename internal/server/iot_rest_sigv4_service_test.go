package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIoTDataPlaneRESTDoesNotStealS3PathStyle(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "mux-thing",
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateThing %d %s", rec.Code, rec.Body.String())
	}

	upd := mustIoTREST(t, handler, http.MethodPost,
		"http://127.0.0.1:4566/things/mux-thing/shadow",
		"iotdevicegateway",
		[]byte(`{"state":{"reported":{"n":7}}}`), now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateThingShadow REST %d %s", upd.Code, upd.Body.String())
	}

	shadow := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/things/mux-thing/shadow",
		"iotdevicegateway", nil, now)
	if shadow.Code != http.StatusOK || !strings.Contains(shadow.Body.String(), `"n"`) {
		t.Fatalf("GetThingShadow REST %d %s", shadow.Code, shadow.Body.String())
	}

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/things", nil, "s3", now, nil)
	obj := []byte("s3-not-shadow")
	put := mustS3(t, handler, http.MethodPut,
		"http://127.0.0.1:4566/things/mux-thing/shadow", obj, "s3", now, nil)
	if put.Code != http.StatusOK {
		t.Fatalf("S3 PutObject /things/mux-thing/shadow %d %s", put.Code, put.Body.String())
	}

	s3Get := mustS3(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/things/mux-thing/shadow", nil, "s3", now, nil)
	if s3Get.Code != http.StatusOK || s3Get.Body.String() != string(obj) {
		t.Fatalf("s3 GetObject should not be GetThingShadow: %d %s", s3Get.Code, s3Get.Body.String())
	}
	if strings.Contains(s3Get.Body.String(), "reported") {
		t.Fatalf("s3 GetObject returned shadow JSON: %s", s3Get.Body.String())
	}

	stsGet := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/things/mux-thing/shadow", "sts", nil, now)
	if stsGet.Code == http.StatusOK && strings.Contains(stsGet.Body.String(), "reported") {
		t.Fatalf("sts-scoped GET /things must not be GetThingShadow: %s", stsGet.Body.String())
	}

	named := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/api/things/shadow/ListNamedShadowsForThing/mux-thing",
		"s3", nil, now)
	if named.Code == http.StatusOK && strings.Contains(named.Body.String(), `"results"`) {
		t.Fatalf("s3-scoped named-shadow list must not run IoT: %s", named.Body.String())
	}
}

func TestIoTCredentialsPathDoesNotClaimUnsignedS3(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/role-aliases", nil, "s3", now, nil)
	obj := []byte("s3-credentials-object")
	put := mustS3(t, handler, http.MethodPut,
		"http://127.0.0.1:4566/role-aliases/device-creds/credentials", obj, "s3", now, nil)
	if put.Code != http.StatusOK {
		t.Fatalf("S3 PutObject credentials key %d %s", put.Code, put.Body.String())
	}

	s3Get := mustS3(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/role-aliases/device-creds/credentials", nil, "s3", now, nil)
	if s3Get.Code != http.StatusOK || s3Get.Body.String() != string(obj) {
		t.Fatalf("s3 GetObject on credentials key %d %s", s3Get.Code, s3Get.Body.String())
	}
	if strings.Contains(s3Get.Body.String(), "accessKeyId") {
		t.Fatalf("s3 GetObject minted credentials: %s", s3Get.Body.String())
	}

	unsigned := mustNewRequest(t, http.MethodGet,
		"http://127.0.0.1:4566/role-aliases/device-creds/credentials", nil)
	unsignedRec := httptest.NewRecorder()
	handler.ServeHTTP(unsignedRec, unsigned)
	if unsignedRec.Code == http.StatusOK || strings.Contains(unsignedRec.Body.String(), "accessKeyId") {
		t.Fatalf("unsigned credentials GET must not mint: %d %s", unsignedRec.Code, unsignedRec.Body.String())
	}
	if unsignedRec.Code != http.StatusForbidden {
		t.Fatalf("unsigned credentials GET want 403, got %d %s", unsignedRec.Code, unsignedRec.Body.String())
	}
}
