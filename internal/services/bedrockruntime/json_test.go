package bedrockruntime_test

import (
	"encoding/json"
	"testing"

	brsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/bedrockruntime"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestInvokeModelEnvelopeJSON(t *testing.T) {
	inv := store.BedrockInvocation{ResponseBody: `{"completion":"hi"}`}
	raw, err := brsvc.InvokeModelEnvelopeJSON(inv)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["contentType"] != "application/json" {
		t.Fatalf("out=%v", out)
	}
}
