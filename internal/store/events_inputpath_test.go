package store

import (
	"strings"
	"testing"
)

func TestApplyEventBridgeInputPath(t *testing.T) {
	event := `{"version":"0","detail-type":"demo","source":"noctaxris.lab","detail":{"ok":true,"n":1}}`
	got, err := applyEventBridgeInputPath(event, "$.detail")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"ok":true`) || !strings.Contains(got, `"n":1`) {
		t.Fatalf("got=%q", got)
	}
	got, err = applyEventBridgeInputPath(event, "$.detail-type")
	if err != nil || got != `"demo"` {
		t.Fatalf("detail-type got=%q err=%v", got, err)
	}
	if _, err := applyEventBridgeInputPath(event, "$.missing"); err == nil {
		t.Fatal("expected missing key error")
	}
	if _, err := applyEventBridgeInputPath(event, "detail"); err == nil {
		t.Fatal("expected $ prefix required")
	}
}
