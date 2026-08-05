package textract_test

import (
	"bytes"
	"encoding/json"
	"testing"

	textractsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/textract"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDetectDocumentTextJSON(t *testing.T) {
	det := store.TextractDetection{
		BlocksJSON: `[{"BlockType":"LINE","Text":"hello"}]`,
	}
	raw, err := textractsvc.DetectDocumentTextJSON(det)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["DetectDocumentTextModelVersion"] != "1.0" {
		t.Fatalf("detect=%v", out)
	}

	det.FeatureTypes = "TABLES"
	raw, err = textractsvc.DetectDocumentTextJSON(det)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("AnalyzeDocumentModelVersion")) || bytes.Contains(raw, []byte("DetectDocumentTextModelVersion")) {
		t.Fatalf("analyze raw=%s", raw)
	}

	_, err = textractsvc.DetectDocumentTextJSON(store.TextractDetection{BlocksJSON: "not-json"})
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}
