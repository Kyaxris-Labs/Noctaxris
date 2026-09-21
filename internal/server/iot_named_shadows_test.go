package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIoTListNamedShadowsExcludesClassic(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "named-shadow-thing",
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateThing %d %s", rec.Code, rec.Body.String())
	}

	classic := mustJSONTarget(t, handler, "AWSIotDataService.UpdateThingShadow", "iot-data", map[string]any{
		"thingName": "named-shadow-thing",
		"payload":   map[string]any{"state": map[string]any{"reported": map[string]any{"classic": true}}},
	}, now)
	if classic.Code != http.StatusOK {
		t.Fatalf("classic UpdateThingShadow %d %s", classic.Code, classic.Body.String())
	}

	emptyList := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/api/things/shadow/ListNamedShadowsForThing/named-shadow-thing",
		"iotdevicegateway", nil, now)
	if emptyList.Code != http.StatusOK {
		t.Fatalf("classic-only list status=%d body=%q", emptyList.Code, emptyList.Body.String())
	}
	var listed map[string]any
	if err := json.Unmarshal(emptyList.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	results, _ := listed["results"].([]any)
	if len(results) != 0 {
		t.Fatalf("classic shadow leaked into named list: %s", emptyList.Body.String())
	}

	named := mustJSONTarget(t, handler, "AWSIotDataService.UpdateThingShadow", "iot-data", map[string]any{
		"thingName":  "named-shadow-thing",
		"shadowName": "delta",
		"payload":    map[string]any{"state": map[string]any{"reported": map[string]any{"n": 1}}},
	}, now)
	if named.Code != http.StatusOK {
		t.Fatalf("named UpdateThingShadow %d %s", named.Code, named.Body.String())
	}

	got := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/api/things/shadow/ListNamedShadowsForThing/named-shadow-thing",
		"iotdevicegateway", nil, now)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"delta"`) {
		t.Fatalf("named list status=%d body=%q", got.Code, got.Body.String())
	}
	if strings.Contains(got.Body.String(), `""`) && strings.Count(got.Body.String(), `"results"`) == 1 {
		var again map[string]any
		_ = json.Unmarshal(got.Body.Bytes(), &again)
		for _, name := range again["results"].([]any) {
			if name == "" {
				t.Fatalf("empty classic name in results: %s", got.Body.String())
			}
		}
	}

	unknown := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/api/things/shadow/ListNamedShadowsForThing/missing-thing",
		"iotdevicegateway", nil, now)
	if unknown.Code != http.StatusOK || !strings.Contains(unknown.Body.String(), `"results":[]`) {
		t.Fatalf("unknown thing list status=%d body=%q", unknown.Code, unknown.Body.String())
	}

	shadowGet := mustIoTREST(t, handler, http.MethodGet,
		"http://127.0.0.1:4566/things/named-shadow-thing/shadow?name=delta",
		"iotdevicegateway", nil, now)
	if shadowGet.Code != http.StatusOK || !strings.Contains(shadowGet.Body.String(), `"n"`) {
		t.Fatalf("named REST get status=%d body=%q", shadowGet.Code, shadowGet.Body.String())
	}
}

func TestIoTListNamedShadowsUnsignedHTTPForbidden(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if rec := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "unsigned-shadow-thing",
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateThing %d %s", rec.Code, rec.Body.String())
	}
	named := mustJSONTarget(t, handler, "AWSIotDataService.UpdateThingShadow", "iot-data", map[string]any{
		"thingName":  "unsigned-shadow-thing",
		"shadowName": "delta",
		"payload":    map[string]any{"state": map[string]any{"reported": map[string]any{"n": 1}}},
	}, now)
	if named.Code != http.StatusOK {
		t.Fatalf("named UpdateThingShadow %d %s", named.Code, named.Body.String())
	}

	req := mustNewRequest(t, http.MethodGet,
		"http://127.0.0.1:4566/api/things/shadow/ListNamedShadowsForThing/unsigned-shadow-thing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unsigned named-shadow list status=%d body=%q", rec.Code, rec.Body.String())
	}
}
