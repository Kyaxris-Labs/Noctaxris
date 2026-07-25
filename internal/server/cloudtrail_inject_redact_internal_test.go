package server

import "testing"

func TestRedactCloudTrailInjectMapStripsSecretsAndCapsDepth(t *testing.T) {
	in := map[string]any{
		"secretId":     "app/db",
		"SecretString": "should-not-appear",
		"nested": map[string]any{
			"MasterUserPassword": "pw",
			"ok":                 "keep",
		},
	}
	out := redactCloudTrailInjectMap(in, cloudTrailInjectMaxDepth)
	if out["SecretString"] != "[REDACTED]" {
		t.Fatalf("SecretString=%v want [REDACTED]", out["SecretString"])
	}
	if out["secretId"] != "app/db" {
		t.Fatalf("secretId=%v", out["secretId"])
	}
	nested, _ := out["nested"].(map[string]any)
	if nested == nil {
		t.Fatal("missing nested")
	}
	if nested["MasterUserPassword"] != "[REDACTED]" {
		t.Fatalf("nested password=%v", nested["MasterUserPassword"])
	}
	if nested["ok"] != "keep" {
		t.Fatalf("nested ok=%v", nested["ok"])
	}

	deep := map[string]any{"v": "leaf"}
	cur := deep
	for i := 0; i < cloudTrailInjectMaxDepth+3; i++ {
		next := map[string]any{"child": cur}
		cur = next
	}
	capped := redactCloudTrailInjectMap(cur, cloudTrailInjectMaxDepth)
	walk := any(capped)
	for d := 0; d < cloudTrailInjectMaxDepth+5; d++ {
		if walk == "[redacted:depth]" {
			return
		}
		m, ok := walk.(map[string]any)
		if !ok {
			break
		}
		if m["_redacted"] == "depth" {
			return
		}
		if child, ok := m["child"]; ok {
			walk = child
			continue
		}
		break
	}
	t.Fatalf("expected depth redaction marker, got %#v", capped)
}
