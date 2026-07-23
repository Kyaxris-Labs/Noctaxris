package opensearch

import (
	"encoding/json"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func domainStatus(d store.OpenSearchDomain) map[string]any {
	status := d.DomainStatus
	if status == "" {
		status = store.OpenSearchDomainStatusCreateFailed
	}
	// Active only with a nested container. Never claim Active on stub://.
	if status == store.OpenSearchDomainStatusActive {
		if d.ContainerID == "" || strings.HasPrefix(d.StubEndpoint, "stub://") {
			status = store.OpenSearchDomainStatusCreateFailed
		}
	}
	created := status == store.OpenSearchDomainStatusActive
	processing := status == "Processing" || status == store.OpenSearchDomainStatusCreating
	return map[string]any{
		"DomainId":          d.DomainID,
		"DomainName":        d.DomainName,
		"ARN":               d.DomainARN,
		"Created":           created,
		"Deleted":           false,
		"Endpoint":          d.StubEndpoint,
		"Processing":        processing,
		"UpgradeProcessing": false,
		"EngineVersion":     d.EngineVersion,
		// Lab status string for honesty; not a field on AWS DomainStatus.
		"DomainStatus": status,
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
