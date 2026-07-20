package bcmexports

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func exportMap(e store.BCMExport) map[string]any {
	return map[string]any{
		"Name":        e.ExportName,
		"ExportArn":   e.ExportARN,
		"Description": e.Description,
		"DataQuery": map[string]any{
			"QueryStatement": "SELECT * FROM COST_AND_USAGE_REPORT",
			"TableConfigurations": map[string]any{
				"COST_AND_USAGE_REPORT": map[string]string{"TIME_GRANULARITY": "DAILY"},
			},
		},
		"DestinationConfigurations": map[string]any{
			"S3Destination": map[string]any{
				"S3Bucket": "local-lab",
				"S3Prefix": e.FilePath,
				"S3OutputConfigurations": map[string]any{
					"OutputType": "CUSTOM",
					"Format":     e.Format,
					"Compression": "NONE",
					"Overwrite":  "CREATE_NEW_REPORT",
				},
			},
		},
	}
}

// CreateExportJSON builds CreateExport response.
func CreateExportJSON(e store.BCMExport) ([]byte, error) {
	return json.Marshal(map[string]any{"ExportArn": e.ExportARN})
}

// GetExportJSON builds GetExport response.
func GetExportJSON(e store.BCMExport) ([]byte, error) {
	return json.Marshal(map[string]any{"Export": exportMap(e)})
}

// ListExportsJSON builds ListExports response.
func ListExportsJSON(exports []store.BCMExport) ([]byte, error) {
	items := make([]map[string]any, 0, len(exports))
	for _, e := range exports {
		items = append(items, map[string]any{
			"ExportArn":  e.ExportARN,
			"ExportName": e.ExportName,
		})
	}
	return json.Marshal(map[string]any{"Exports": items})
}

// DeleteExportJSON is empty OK.
func DeleteExportJSON() ([]byte, error) { return []byte(`{}`), nil }
