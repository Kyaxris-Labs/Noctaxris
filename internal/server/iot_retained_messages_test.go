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
