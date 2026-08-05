package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAppSyncGetListDeleteGraphqlAPI(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "api-crud",
		"authenticationType": "API_KEY",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraphqlApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)
	if apiID == "" {
		t.Fatal("missing apiId")
	}

	get := mustJSONTarget(t, handler, "AWSAppSync.GetGraphqlApi", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "api-crud") {
		t.Fatalf("GetGraphqlApi status=%d body=%q", get.Code, get.Body.String())
	}
	miss := mustJSONTarget(t, handler, "AWSAppSync.GetGraphqlApi", "appsync", map[string]any{
		"apiId": "missing",
	}, now)
	if miss.Code != http.StatusNotFound {
		t.Fatalf("GetGraphqlApi missing status=%d body=%q", miss.Code, miss.Body.String())
	}
	empty := mustJSONTarget(t, handler, "AWSAppSync.GetGraphqlApi", "appsync", map[string]any{
		"apiId": "",
	}, now)
	if empty.Code != http.StatusNotFound && empty.Code != http.StatusBadRequest {
		t.Fatalf("GetGraphqlApi empty status=%d body=%q", empty.Code, empty.Body.String())
	}

	list := mustJSONTarget(t, handler, "AWSAppSync.ListGraphqlApis", "appsync", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), apiID) {
		t.Fatalf("ListGraphqlApis status=%d body=%q", list.Code, list.Body.String())
	}

	delMiss := mustJSONTarget(t, handler, "AWSAppSync.DeleteGraphqlApi", "appsync", map[string]any{
		"apiId": "missing",
	}, now)
	if delMiss.Code != http.StatusNotFound {
		t.Fatalf("DeleteGraphqlApi missing status=%d body=%q", delMiss.Code, delMiss.Body.String())
	}
	del := mustJSONTarget(t, handler, "AWSAppSync.DeleteGraphqlApi", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteGraphqlApi status=%d body=%q", del.Code, del.Body.String())
	}
	getAfter := mustJSONTarget(t, handler, "AWSAppSync.GetGraphqlApi", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	if getAfter.Code != http.StatusNotFound {
		t.Fatalf("Get after delete status=%d body=%q", getAfter.Code, getAfter.Body.String())
	}
}
