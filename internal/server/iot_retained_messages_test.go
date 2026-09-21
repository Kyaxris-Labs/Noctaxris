package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIoTListRetainedMessagesSummariesAndDeny(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	empty := mustJSONTarget(t, handler, "AWSIotService.ListRetainedMessages", "iot", map[string]any{}, now)
	if empty.Code != http.StatusOK {
		t.Fatalf("empty list status=%d body=%q", empty.Code, empty.Body.String())
	}
	var emptyBody map[string]any
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatal(err)
	}
	emptyTopics, _ := emptyBody["retainedTopics"].([]any)
	if emptyTopics == nil || len(emptyTopics) != 0 {
		t.Fatalf("empty retainedTopics=%v body=%q", emptyBody["retainedTopics"], empty.Body.String())
	}

	if err := st.PutIoTRetainedMessage(testAccountID, testRegion, "alpha/topic", []byte("hello"), 1); err != nil {
		t.Fatal(err)
	}
	if err := st.PutIoTRetainedMessage(testAccountID, testRegion, "beta/topic", []byte("world!"), 0); err != nil {
		t.Fatal(err)
	}

	listed := mustJSONTarget(t, handler, "AWSIotService.ListRetainedMessages", "iot", map[string]any{}, now)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%q", listed.Code, listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), `"payload"`) {
		t.Fatalf("list must omit payload: %s", listed.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(listed.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	topics, _ := body["retainedTopics"].([]any)
	if len(topics) != 2 {
		t.Fatalf("want 2 summaries got %s", listed.Body.String())
	}
	byTopic := map[string]map[string]any{}
	for _, raw := range topics {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("summary type %T", raw)
		}
		name, _ := item["topic"].(string)
		byTopic[name] = item
		if _, ok := item["payload"]; ok {
			t.Fatalf("payload leaked on %s", name)
		}
	}
	alpha := byTopic["alpha/topic"]
	beta := byTopic["beta/topic"]
	if alpha == nil || beta == nil {
		t.Fatalf("missing topics: %s", listed.Body.String())
	}
	if alpha["payloadSize"] != float64(5) || alpha["qos"] != float64(1) {
		t.Fatalf("alpha summary=%v", alpha)
	}
	if beta["payloadSize"] != float64(6) || beta["qos"] != float64(0) {
		t.Fatalf("beta summary=%v", beta)
	}
	if ts, ok := alpha["lastModifiedTime"].(float64); !ok || ts <= 0 {
		t.Fatalf("alpha lastModifiedTime=%v", alpha["lastModifiedTime"])
	}

	_, userARN, err := st.CreateUser(testAccountID, "iot-retain-deny")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "iot-retain-deny")
	if err != nil {
		t.Fatal(err)
	}
	deny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"iot:ListRetainedMessages","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "noretain", deny); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSIotService.ListRetainedMessages")
	signHeader(t, req, raw, akid, secret, testRegion, "iot", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("deny ListRetainedMessages status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func retainedTopicsByName(t *testing.T, rec *httptest.ResponseRecorder) map[string]map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("ListRetainedMessages status=%d body=%q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"payload"`) {
		t.Fatalf("list must omit payload: %s", rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	topics, _ := body["retainedTopics"].([]any)
	out := map[string]map[string]any{}
	for _, raw := range topics {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("summary type %T", raw)
		}
		name, _ := item["topic"].(string)
		if _, ok := item["payload"]; ok {
			t.Fatalf("payload leaked on %s", name)
		}
		out[name] = item
	}
	return out
}

func TestIoTHTTPPublishRetainThenList(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	jsonPub := mustJSONTarget(t, handler, "AWSIotDataService.Publish", "iot-data", map[string]any{
		"topic":   "json/topic",
		"payload": "hello",
		"qos":     1,
		"retain":  true,
	}, now)
	if jsonPub.Code != http.StatusOK {
		t.Fatalf("JSON Publish status=%d body=%q", jsonPub.Code, jsonPub.Body.String())
	}

	ctlPub := mustJSONTarget(t, handler, "AWSIotService.Publish", "iot-data", map[string]any{
		"topic":   "ctl/topic",
		"payload": "from-ctl",
		"qos":     0,
		"retain":  true,
	}, now)
	if ctlPub.Code != http.StatusOK {
		t.Fatalf("AWSIotService.Publish status=%d body=%q", ctlPub.Code, ctlPub.Body.String())
	}

	skip := mustJSONTarget(t, handler, "AWSIotDataService.Publish", "iot-data", map[string]any{
		"topic":   "skip/me",
		"payload": "nope",
		"retain":  false,
	}, now)
	if skip.Code != http.StatusOK {
		t.Fatalf("non-retain Publish status=%d body=%q", skip.Code, skip.Body.String())
	}

	restPub := mustIoTREST(t, handler, http.MethodPost,
		"http://127.0.0.1:4566/topics/rest/topic?qos=1&retain=true",
		"iot-data", []byte("world!"), now)
	if restPub.Code != http.StatusOK {
		t.Fatalf("REST Publish status=%d body=%q", restPub.Code, restPub.Body.String())
	}

	gwPub := mustIoTREST(t, handler, http.MethodPost,
		"http://127.0.0.1:4566/topics/gw/topic?qos=0&retain=true",
		"iotdevicegateway", []byte("via-gw"), now)
	if gwPub.Code != http.StatusOK {
		t.Fatalf("iotdevicegateway Publish status=%d body=%q", gwPub.Code, gwPub.Body.String())
	}

	listed := mustJSONTarget(t, handler, "AWSIotService.ListRetainedMessages", "iot", map[string]any{}, now)
	byTopic := retainedTopicsByName(t, listed)
	if byTopic["skip/me"] != nil {
		t.Fatalf("retain=false should not store skip/me: %s", listed.Body.String())
	}
	jsonRow := byTopic["json/topic"]
	if jsonRow == nil || jsonRow["payloadSize"] != float64(5) || jsonRow["qos"] != float64(1) {
		t.Fatalf("json/topic summary=%v body=%s", jsonRow, listed.Body.String())
	}
	if ts, ok := jsonRow["lastModifiedTime"].(float64); !ok || ts <= 0 {
		t.Fatalf("json lastModifiedTime=%v", jsonRow["lastModifiedTime"])
	}
	restRow := byTopic["rest/topic"]
	if restRow == nil || restRow["payloadSize"] != float64(6) || restRow["qos"] != float64(1) {
		t.Fatalf("rest/topic summary=%v body=%s", restRow, listed.Body.String())
	}
	if byTopic["ctl/topic"] == nil || byTopic["gw/topic"] == nil {
		t.Fatalf("missing ctl or gw rows: %s", listed.Body.String())
	}

	clear := mustJSONTarget(t, handler, "AWSIotDataService.Publish", "iot-data", map[string]any{
		"topic":   "json/topic",
		"payload": "",
		"retain":  true,
	}, now)
	if clear.Code != http.StatusOK {
		t.Fatalf("clear Publish status=%d body=%q", clear.Code, clear.Body.String())
	}
	restClear := mustIoTREST(t, handler, http.MethodPost,
		"http://127.0.0.1:4566/topics/rest/topic?retain=true",
		"iot-data", []byte{}, now)
	if restClear.Code != http.StatusOK {
		t.Fatalf("REST clear status=%d body=%q", restClear.Code, restClear.Body.String())
	}

	after := mustJSONTarget(t, handler, "AWSIotService.ListRetainedMessages", "iot", map[string]any{}, now)
	afterTopics := retainedTopicsByName(t, after)
	if afterTopics["json/topic"] != nil || afterTopics["rest/topic"] != nil {
		t.Fatalf("empty retain should clear topics: %s", after.Body.String())
	}
	if afterTopics["ctl/topic"] == nil || afterTopics["gw/topic"] == nil {
		t.Fatalf("unrelated retained rows dropped: %s", after.Body.String())
	}
}

func TestIoTHTTPPublishIAMDenyAndUnsigned(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, userARN, err := st.CreateUser(testAccountID, "iot-publish-deny")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "iot-publish-deny")
	if err != nil {
		t.Fatal(err)
	}
	deny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"iot-data:Publish","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "nopub", deny); err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(map[string]any{
		"topic":   "deny/topic",
		"payload": "secret",
		"retain":  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSIotDataService.Publish")
	signHeader(t, req, raw, akid, secret, testRegion, "iot-data", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("deny JSON Publish status=%d body=%q", rec.Code, rec.Body.String())
	}

	restBody := []byte("secret")
	restReq := mustNewRequest(t, http.MethodPost,
		"http://127.0.0.1:4566/topics/deny/rest?qos=1&retain=true", restBody)
	restReq.Header.Set("Content-Type", "application/json")
	signHeader(t, restReq, restBody, akid, secret, testRegion, "iot-data", now)
	restRec := httptest.NewRecorder()
	handler.ServeHTTP(restRec, restReq)
	if restRec.Code != http.StatusForbidden {
		t.Fatalf("deny REST Publish status=%d body=%q", restRec.Code, restRec.Body.String())
	}

	listed := mustJSONTarget(t, handler, "AWSIotService.ListRetainedMessages", "iot", map[string]any{}, now)
	byTopic := retainedTopicsByName(t, listed)
	if byTopic["deny/topic"] != nil || byTopic["deny/rest"] != nil {
		t.Fatalf("denied publish must not retain: %s", listed.Body.String())
	}

	unsigned := mustNewRequest(t, http.MethodPost,
		"http://127.0.0.1:4566/topics/unsigned/topic?retain=true", []byte("x"))
	unsignedRec := httptest.NewRecorder()
	handler.ServeHTTP(unsignedRec, unsigned)
	if unsignedRec.Code != http.StatusForbidden || !strings.Contains(unsignedRec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("unsigned Publish want 403 MissingAuthenticationToken, got %d %s", unsignedRec.Code, unsignedRec.Body.String())
	}
}
