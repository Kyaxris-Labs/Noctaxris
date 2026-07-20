package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustTaggingJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "ResourceGroupsTaggingAPI_20170126."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "tagging", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestTaggingRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	arn := "arn:aws:sqs:us-east-1:000000000001:tag-lab-q"

	tag := mustTaggingJSON(t, handler, "TagResources", map[string]any{
		"ResourceARNList": []string{arn},
		"Tags":            map[string]string{"env": "lab"},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResources status=%d body=%q", tag.Code, tag.Body.String())
	}

	get := mustTaggingJSON(t, handler, "GetResources", map[string]any{
		"TagFilters": []map[string]any{
			{"Key": "env", "Values": []string{"lab"}},
		},
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetResources status=%d body=%q", get.Code, get.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	list, _ := getOut["ResourceTagMappingList"].([]any)
	if len(list) != 1 {
		t.Fatalf("list=%v body=%q", list, get.Body.String())
	}

	untag := mustTaggingJSON(t, handler, "UntagResources", map[string]any{
		"ResourceARNList": []string{arn},
		"TagKeys":         []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResources status=%d body=%q", untag.Code, untag.Body.String())
	}

	get2 := mustTaggingJSON(t, handler, "GetResources", map[string]any{}, now)
	if get2.Code != http.StatusOK {
		t.Fatalf("GetResources2 status=%d body=%q", get2.Code, get2.Body.String())
	}
	var getOut2 map[string]any
	if err := json.Unmarshal(get2.Body.Bytes(), &getOut2); err != nil {
		t.Fatal(err)
	}
	list2, _ := getOut2["ResourceTagMappingList"].([]any)
	if len(list2) != 0 {
		t.Fatalf("list2=%v", list2)
	}
}
