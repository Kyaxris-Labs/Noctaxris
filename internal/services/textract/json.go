package textract

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// DetectDocumentTextJSON builds a DetectDocumentText / AnalyzeDocument success body.
func DetectDocumentTextJSON(det store.TextractDetection) ([]byte, error) {
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(det.BlocksJSON), &blocks); err != nil {
		return nil, err
	}
	out := map[string]any{
		"DocumentMetadata":               map[string]any{"Pages": 1},
		"Blocks":                         blocks,
		"DetectDocumentTextModelVersion": "1.0",
	}
	if det.FeatureTypes != "" {
		out["AnalyzeDocumentModelVersion"] = "1.0"
		delete(out, "DetectDocumentTextModelVersion")
	}
	return json.Marshal(out)
}
