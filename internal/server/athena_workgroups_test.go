package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAthenaWorkGroupHandlers(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AmazonAthena.CreateWorkGroup", "athena", map[string]any{
		"Name":        "analytics",
		"Description": "lab",
		"Configuration": map[string]any{
			"EnforceWorkGroupConfiguration": true,
			"ResultConfiguration": map[string]any{
				"OutputLocation": "s3://athena-wg/results/",
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateWorkGroup status=%d body=%q", create.Code, create.Body.String())
	}

	dup := mustJSONTarget(t, handler, "AmazonAthena.CreateWorkGroup", "athena", map[string]any{
		"Name": "analytics",
	}, now)
	if dup.Code != http.StatusBadRequest || !strings.Contains(dup.Body.String(), "already exists") {
		t.Fatalf("duplicate Create status=%d body=%q", dup.Code, dup.Body.String())
	}

	empty := mustJSONTarget(t, handler, "AmazonAthena.CreateWorkGroup", "athena", map[string]any{
		"Name": "",
	}, now)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty Name status=%d body=%q", empty.Code, empty.Body.String())
	}

	get := mustJSONTarget(t, handler, "AmazonAthena.GetWorkGroup", "athena", map[string]any{
		"WorkGroup": "analytics",
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetWorkGroup status=%d body=%q", get.Code, get.Body.String())
	}
	var getBody map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &getBody); err != nil {
		t.Fatal(err)
	}
	wg, _ := getBody["WorkGroup"].(map[string]any)
	if wg["Name"] != "analytics" || wg["State"] != "ENABLED" {
		t.Fatalf("get body=%s", get.Body.String())
	}
	cfg, _ := wg["Configuration"].(map[string]any)
	rc, _ := cfg["ResultConfiguration"].(map[string]any)
	if rc["OutputLocation"] != "s3://athena-wg/results/" || cfg["EnforceWorkGroupConfiguration"] != true {
		t.Fatalf("config=%v", cfg)
	}

	primary := mustJSONTarget(t, handler, "AmazonAthena.GetWorkGroup", "athena", map[string]any{
		"WorkGroup": "primary",
	}, now)
	if primary.Code != http.StatusOK {
		t.Fatalf("Get primary status=%d body=%q", primary.Code, primary.Body.String())
	}

	list := mustJSONTarget(t, handler, "AmazonAthena.ListWorkGroups", "athena", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListWorkGroups status=%d body=%q", list.Code, list.Body.String())
	}
	var listBody map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	groups, _ := listBody["WorkGroups"].([]any)
	if len(groups) < 2 {
		t.Fatalf("list=%s", list.Body.String())
	}

	upd := mustJSONTarget(t, handler, "AmazonAthena.UpdateWorkGroup", "athena", map[string]any{
		"WorkGroup": "analytics",
		"State":     "DISABLED",
		"ConfigurationUpdates": map[string]any{
			"EnforceWorkGroupConfiguration": false,
			"ResultConfigurationUpdates": map[string]any{
				"OutputLocation": "s3://athena-wg/other/",
			},
		},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateWorkGroup status=%d body=%q", upd.Code, upd.Body.String())
	}

	startDisabled := mustJSONTarget(t, handler, "AmazonAthena.StartQueryExecution", "athena", map[string]any{
		"QueryString": "SELECT 1",
		"WorkGroup":   "analytics",
	}, now)
	if startDisabled.Code != http.StatusBadRequest || !strings.Contains(startDisabled.Body.String(), "DISABLED") {
		t.Fatalf("Start on DISABLED status=%d body=%q", startDisabled.Code, startDisabled.Body.String())
	}

	delPrimary := mustJSONTarget(t, handler, "AmazonAthena.DeleteWorkGroup", "athena", map[string]any{
		"WorkGroup": "primary",
	}, now)
	if delPrimary.Code != http.StatusBadRequest || !strings.Contains(delPrimary.Body.String(), "primary") {
		t.Fatalf("Delete primary status=%d body=%q", delPrimary.Code, delPrimary.Body.String())
	}

	del := mustJSONTarget(t, handler, "AmazonAthena.DeleteWorkGroup", "athena", map[string]any{
		"WorkGroup": "analytics",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteWorkGroup status=%d body=%q", del.Code, del.Body.String())
	}
}
