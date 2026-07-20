package cloudcontrol

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// ProgressEventJSON builds a sync SUCCESS ProgressEvent.
func ProgressEventJSON(op, typeName, identifier, requestToken, resourceModel string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ProgressEvent": map[string]any{
			"TypeName":        typeName,
			"Identifier":      identifier,
			"RequestToken":    requestToken,
			"Operation":       op,
			"OperationStatus": "SUCCESS",
			"EventTime":       float64(time.Now().UTC().Unix()),
			"ResourceModel":   resourceModel,
		},
	})
}

// GetResourceJSON builds GetResource response.
func GetResourceJSON(r store.CloudControlResource) ([]byte, error) {
	return json.Marshal(map[string]any{
		"TypeName": r.TypeName,
		"ResourceDescription": map[string]any{
			"Identifier": r.Identifier,
			"Properties": r.Properties,
		},
	})
}

// ListResourcesJSON builds ListResources response.
func ListResourcesJSON(typeName string, resources []store.CloudControlResource) ([]byte, error) {
	items := make([]map[string]any, 0, len(resources))
	for _, r := range resources {
		items = append(items, map[string]any{
			"Identifier": r.Identifier,
			"Properties": r.Properties,
		})
	}
	return json.Marshal(map[string]any{
		"TypeName":             typeName,
		"ResourceDescriptions": items,
	})
}
