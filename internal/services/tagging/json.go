package tagging

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// TagResourcesJSON builds a TagResources response.
func TagResourcesJSON(failed map[string]string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"FailedResourcesMap": failedMapJSON(failed),
	})
}

// UntagResourcesJSON builds an UntagResources response.
func UntagResourcesJSON(failed map[string]string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"FailedResourcesMap": failedMapJSON(failed),
	})
}

// GetResourcesJSON builds a GetResources response.
func GetResourcesJSON(resources []store.TaggedResource) ([]byte, error) {
	list := make([]map[string]any, 0, len(resources))
	for _, r := range resources {
		tags := make([]map[string]string, 0, len(r.Tags))
		for _, t := range r.Tags {
			tags = append(tags, map[string]string{"Key": t.Key, "Value": t.Value})
		}
		list = append(list, map[string]any{
			"ResourceARN": r.ResourceARN,
			"Tags":        tags,
		})
	}
	return json.Marshal(map[string]any{
		"ResourceTagMappingList": list,
		"PaginationToken":        "",
	})
}

func failedMapJSON(failed map[string]string) map[string]any {
	out := map[string]any{}
	for arn, status := range failed {
		out[arn] = map[string]any{
			"StatusCode":   400,
			"ErrorCode":    "InvalidParameterException",
			"ErrorMessage": status,
		}
	}
	return out
}
