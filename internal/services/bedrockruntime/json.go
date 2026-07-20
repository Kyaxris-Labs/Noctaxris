package bedrockruntime

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// InvokeModelEnvelopeJSON wraps a recorded invocation for protocol tests that
// expect a body/contentType envelope (SDK-style). REST path handlers write
// ResponseBody bytes directly.
func InvokeModelEnvelopeJSON(inv store.BedrockInvocation) ([]byte, error) {
	return json.Marshal(map[string]any{
		"body":        inv.ResponseBody,
		"contentType": "application/json",
	})
}
