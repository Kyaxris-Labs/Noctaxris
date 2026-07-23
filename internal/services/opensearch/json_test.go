package opensearch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDescribeDomainJSONNeverActiveOnStub(t *testing.T) {
	d := store.OpenSearchDomain{
		DomainID:      "id-1",
		DomainName:    "lab-domain",
		DomainARN:     "arn:aws:es:us-east-1:000000000001:domain/lab-domain",
		EngineVersion: "OpenSearch_2.11",
		DomainStatus:  store.OpenSearchDomainStatusCreateFailed,
		StubEndpoint:  "stub://127.0.0.1/opensearch/id-1",
		FailureReason: HintMaxMapCount,
	}
	raw, err := DescribeDomainJSON(d)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "stub://127.0.0.1/opensearch/") {
		t.Fatalf("missing stub endpoint: %s", body)
	}
	if !strings.Contains(body, `"DomainStatus":"CreateFailed"`) {
		t.Fatalf("want CreateFailed status: %s", body)
	}
	if strings.Contains(body, `"DomainStatus":"Active"`) {
		t.Fatalf("must not claim Active on stub://: %s", body)
	}
	var wrap map[string]any
	if err := json.Unmarshal(raw, &wrap); err != nil {
		t.Fatal(err)
	}
	st, _ := wrap["DomainStatus"].(map[string]any)
	if st == nil {
		t.Fatalf("DomainStatus object missing: %s", body)
	}
	if st["Created"] != false {
		t.Fatalf("Created=%v want false for CreateFailed stub", st["Created"])
	}
	if st["Processing"] != false {
		t.Fatalf("Processing=%v want false for CreateFailed stub", st["Processing"])
	}
	if st["FailureReason"] != HintMaxMapCount {
		t.Fatalf("FailureReason=%v want max_map_count hint", st["FailureReason"])
	}
}
