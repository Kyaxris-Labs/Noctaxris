package cloudfront

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func distributionMap(d store.CloudFrontDistribution) map[string]any {
	var origins []store.CloudFrontOrigin
	_ = json.Unmarshal([]byte(d.OriginsJSON), &origins)
	items := make([]map[string]any, 0, len(origins))
	for _, o := range origins {
		items = append(items, map[string]any{
			"Id":         o.ID,
			"DomainName": o.DomainName,
			"OriginType": o.OriginType,
		})
	}
	return map[string]any{
		"Id":         d.ID,
		"ARN":        d.ARN,
		"DomainName": d.DomainName,
		"Status":     d.Status,
		"DistributionConfig": map[string]any{
			"CallerReference": d.CallerReference,
			"Comment":         d.Comment,
			"Enabled":         d.Enabled,
			"Origins": map[string]any{
				"Quantity": len(items),
				"Items":    items,
			},
		},
	}
}

// CreateDistributionJSON builds CreateDistribution response.
func CreateDistributionJSON(d store.CloudFrontDistribution) ([]byte, error) {
	return json.Marshal(map[string]any{"Distribution": distributionMap(d)})
}

// GetDistributionJSON builds GetDistribution response.
func GetDistributionJSON(d store.CloudFrontDistribution) ([]byte, error) {
	return json.Marshal(map[string]any{"Distribution": distributionMap(d)})
}

// ListDistributionsJSON builds ListDistributions response.
func ListDistributionsJSON(dists []store.CloudFrontDistribution) ([]byte, error) {
	items := make([]map[string]any, 0, len(dists))
	for _, d := range dists {
		items = append(items, map[string]any{
			"Id":         d.ID,
			"ARN":        d.ARN,
			"DomainName": d.DomainName,
			"Status":     d.Status,
			"Enabled":    d.Enabled,
			"Comment":    d.Comment,
		})
	}
	return json.Marshal(map[string]any{
		"DistributionList": map[string]any{
			"Quantity": len(items),
			"Items":    items,
		},
	})
}

// DeleteDistributionJSON is an empty OK body.
func DeleteDistributionJSON() ([]byte, error) { return []byte(`{}`), nil }
