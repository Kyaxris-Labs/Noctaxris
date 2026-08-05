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

func TestOpenSearchDomainJSONPaths(t *testing.T) {
	active := store.OpenSearchDomain{
		DomainID: "id-2", DomainName: "live", DomainARN: "arn:aws:es:us-east-1:1:domain/live",
		EngineVersion: "OpenSearch_2.11", DomainStatus: store.OpenSearchDomainStatusActive,
		ContainerID: "cid-1", StubEndpoint: "https://live.noctaxris.local",
	}
	for _, fn := range []func(store.OpenSearchDomain) ([]byte, error){
		CreateDomainJSON, DescribeDomainJSON,
	} {
		raw, err := fn(active)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"DomainStatus":"Active"`) {
			t.Fatalf("want Active with container: %s", raw)
		}
	}

	processing := active
	processing.DomainStatus = "Processing"
	raw, err := DescribeDomainJSON(processing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"Processing":true`) {
		t.Fatalf("want Processing true: %s", raw)
	}

	listRaw, err := ListDomainNamesJSON([]store.OpenSearchDomain{{DomainName: "a"}, {DomainName: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	var list map[string]any
	if err := json.Unmarshal(listRaw, &list); err != nil {
		t.Fatal(err)
	}
	names, _ := list["DomainNames"].([]any)
	if len(names) != 2 {
		t.Fatalf("DomainNames=%v", list)
	}

	delRaw, err := DeleteDomainJSON(active)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(delRaw), `"Deleted":true`) {
		t.Fatalf("delete=%s", delRaw)
	}
}
