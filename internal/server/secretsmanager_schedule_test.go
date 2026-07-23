package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestRotateSecretRulesDeferImmediate(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "rules-defer",
		"SecretString": "keep-me",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", create.Code, create.Body.String())
	}

	rot := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{
		"SecretId":          "rules-defer",
		"RotateImmediately": false,
		"RotationRules": map[string]any{
			"AutomaticallyAfterDays": 7,
		},
	}, now)
	if rot.Code != http.StatusOK {
		t.Fatalf("RotateSecret status=%d body=%q", rot.Code, rot.Body.String())
	}

	get := mustSecretsJSON(t, handler, "GetSecretValue", map[string]any{"SecretId": "rules-defer"}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetSecretValue status=%d", get.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["SecretString"] != "keep-me" {
		t.Fatalf("deferred rotate changed value: %v", got["SecretString"])
	}

	desc := mustSecretsJSON(t, handler, "DescribeSecret", map[string]any{"SecretId": "rules-defer"}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeSecret status=%d body=%q", desc.Code, desc.Body.String())
	}
	var meta map[string]any
	if err := json.Unmarshal(desc.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["RotationEnabled"] != true {
		t.Fatalf("RotationEnabled=%v", meta["RotationEnabled"])
	}
	rules, _ := meta["RotationRules"].(map[string]any)
	if rules["AutomaticallyAfterDays"].(float64) != 7 {
		t.Fatalf("rules=%v", rules)
	}

	n, err := st.ProcessDueSecretRotations(now.Add(8*24*time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("due rotations=%d", n)
	}
	get2 := mustSecretsJSON(t, handler, "GetSecretValue", map[string]any{"SecretId": "rules-defer"}, now)
	var got2 map[string]any
	_ = json.Unmarshal(get2.Body.Bytes(), &got2)
	if got2["SecretString"] == "keep-me" {
		t.Fatal("expected scheduled rotate to change secret")
	}
}
