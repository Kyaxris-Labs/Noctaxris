package opensearch

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func domainStatus(d store.OpenSearchDomain) map[string]any {
	return map[string]any{
		"DomainId":      d.DomainID,
		"DomainName":    d.DomainName,
		"ARN":           d.DomainARN,
		"Created":       true,
		"Deleted":       false,
		"Endpoint":      d.StubEndpoint,
		"Processing":    false,
		"UpgradeProcessing": false,
		"EngineVersion": d.EngineVersion,
		"DomainStatus":  d.DomainStatus,
	}
}

// CreateDomainJSON builds CreateDomain response.
func CreateDomainJSON(d store.OpenSearchDomain) ([]byte, error) {
	return json.Marshal(map[string]any{"DomainStatus": domainStatus(d)})
}

// DescribeDomainJSON builds DescribeDomain response.
func DescribeDomainJSON(d store.OpenSearchDomain) ([]byte, error) {
	return json.Marshal(map[string]any{"DomainStatus": domainStatus(d)})
}

// ListDomainNamesJSON builds ListDomainNames response.
func ListDomainNamesJSON(domains []store.OpenSearchDomain) ([]byte, error) {
	names := make([]map[string]any, 0, len(domains))
	for _, d := range domains {
		names = append(names, map[string]any{
			"DomainName": d.DomainName,
			"EngineType": "OpenSearch",
		})
	}
	return json.Marshal(map[string]any{"DomainNames": names})
}

// DeleteDomainJSON builds DeleteDomain response.
func DeleteDomainJSON(d store.OpenSearchDomain) ([]byte, error) {
	st := domainStatus(d)
	st["Deleted"] = true
	st["DomainStatus"] = "Deleting"
	return json.Marshal(map[string]any{"DomainStatus": st})
}
