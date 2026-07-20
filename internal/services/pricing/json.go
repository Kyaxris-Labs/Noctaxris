package pricing

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// DescribeServicesJSON builds DescribeServices response.
func DescribeServicesJSON(services []store.PricingServiceSummary) ([]byte, error) {
	items := make([]map[string]any, 0, len(services))
	for _, s := range services {
		items = append(items, map[string]any{
			"ServiceCode":    s.ServiceCode,
			"AttributeNames": s.AttributeNames,
		})
	}
	return json.Marshal(map[string]any{"Services": items})
}

// GetAttributeValuesJSON builds GetAttributeValues response.
func GetAttributeValuesJSON(values []string) ([]byte, error) {
	items := make([]map[string]any, 0, len(values))
	for _, v := range values {
		items = append(items, map[string]any{"Value": v})
	}
	return json.Marshal(map[string]any{"AttributeValues": items})
}

// GetProductsJSON builds GetProducts response.
func GetProductsJSON(priceLists []string) ([]byte, error) {
	return json.Marshal(map[string]any{"PriceList": priceLists})
}
