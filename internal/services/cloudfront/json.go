package cloudfront

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func distributionConfigMap(d store.CloudFrontDistribution) map[string]any {
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
	var behaviors []store.CloudFrontCacheBehavior
	if strings.TrimSpace(d.BehaviorsJSON) != "" {
		_ = json.Unmarshal([]byte(d.BehaviorsJSON), &behaviors)
	}
	var defaultTarget string
	cacheItems := make([]map[string]any, 0)
	for _, b := range behaviors {
		if b.PathPattern == "*" && defaultTarget == "" {
			defaultTarget = b.TargetOriginId
			continue
		}
		cacheItems = append(cacheItems, map[string]any{
			"PathPattern":    b.PathPattern,
			"TargetOriginId": b.TargetOriginId,
		})
	}
	cfg := map[string]any{
		"CallerReference": d.CallerReference,
		"Comment":         d.Comment,
		"Enabled":         d.Enabled,
		"Origins": map[string]any{
			"Quantity": len(items),
			"Items":    items,
		},
	}
	if defaultTarget != "" {
		cfg["DefaultCacheBehavior"] = map[string]any{
			"TargetOriginId": defaultTarget,
		}
	}
	if len(cacheItems) > 0 {
		cfg["CacheBehaviors"] = map[string]any{
			"Quantity": len(cacheItems),
			"Items":    cacheItems,
		}
	}
	if d.LoggingEnabled || d.LoggingBucket != "" {
		cfg["Logging"] = map[string]any{
			"Enabled": d.LoggingEnabled,
			"Bucket":  d.LoggingBucket,
			"Prefix":  d.LoggingPrefix,
		}
	}
	return cfg
}

func distributionMap(d store.CloudFrontDistribution) map[string]any {
	out := map[string]any{
		"Id":                 d.ID,
		"ARN":                d.ARN,
		"Status":             d.Status,
		"DistributionConfig": distributionConfigMap(d),
	}
	if d.DomainName != "" {
		out["DomainName"] = d.DomainName
	}
	return out
}

func invalidationMap(inv store.CloudFrontInvalidation) map[string]any {
	var paths []string
	_ = json.Unmarshal([]byte(inv.PathsJSON), &paths)
	if paths == nil {
		paths = []string{}
	}
	return map[string]any{
		"Id":         inv.ID,
		"Status":     inv.Status,
		"CreateTime": time.UnixMilli(inv.CreatedAt).UTC().Format(time.RFC3339),
		"InvalidationBatch": map[string]any{
			"CallerReference": inv.CallerReference,
			"Paths": map[string]any{
				"Quantity": len(paths),
				"Items":    paths,
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

// GetDistributionConfigJSON builds GetDistributionConfig response (config + ETag).
func GetDistributionConfigJSON(d store.CloudFrontDistribution) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ETag":               d.ETag,
		"DistributionConfig": distributionConfigMap(d),
	})
}

// UpdateDistributionJSON builds UpdateDistribution response (Distribution + ETag).
func UpdateDistributionJSON(d store.CloudFrontDistribution) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ETag":         d.ETag,
		"Distribution": distributionMap(d),
	})
}

// ListDistributionsJSON builds ListDistributions response.
func ListDistributionsJSON(dists []store.CloudFrontDistribution) ([]byte, error) {
	items := make([]map[string]any, 0, len(dists))
	for _, d := range dists {
		item := map[string]any{
			"Id":      d.ID,
			"ARN":     d.ARN,
			"Status":  d.Status,
			"Enabled": d.Enabled,
			"Comment": d.Comment,
		}
		if d.DomainName != "" {
			item["DomainName"] = d.DomainName
		}
		items = append(items, item)
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

// CreateInvalidationJSON builds CreateInvalidation response.
func CreateInvalidationJSON(inv store.CloudFrontInvalidation) ([]byte, error) {
	return json.Marshal(map[string]any{"Invalidation": invalidationMap(inv)})
}

// GetInvalidationJSON builds GetInvalidation response.
func GetInvalidationJSON(inv store.CloudFrontInvalidation) ([]byte, error) {
	return json.Marshal(map[string]any{"Invalidation": invalidationMap(inv)})
}

// ListInvalidationsJSON builds ListInvalidations response.
func ListInvalidationsJSON(invs []store.CloudFrontInvalidation) ([]byte, error) {
	items := make([]map[string]any, 0, len(invs))
	for _, inv := range invs {
		items = append(items, map[string]any{
			"Id":         inv.ID,
			"Status":     inv.Status,
			"CreateTime": time.UnixMilli(inv.CreatedAt).UTC().Format(time.RFC3339),
		})
	}
	return json.Marshal(map[string]any{
		"InvalidationList": map[string]any{
			"Quantity":    len(items),
			"Items":       items,
			"IsTruncated": false,
		},
	})
}
