package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Pricing uses an embedded static catalog. No sqlite schema required.

// DefaultPricingRegion is the lab Pricing API region.
const DefaultPricingRegion = "us-east-1"

// pricingCatalogJSON is a tiny static AWS Price List shaped catalog for CTF/lab entry.
const pricingCatalogJSON = `[
  {
    "serviceCode": "AmazonEC2",
    "attributeNames": ["instanceType", "location", "operatingSystem"],
    "products": [
      {
        "sku": "NOCTAXRIS-EC2-T3MICRO",
        "attributes": {
          "instanceType": "t3.micro",
          "location": "US East (N. Virginia)",
          "operatingSystem": "Linux"
        },
        "pricePerUnit": "0.0104",
        "currency": "USD",
        "unit": "Hrs"
      },
      {
        "sku": "NOCTAXRIS-EC2-T3SMALL",
        "attributes": {
          "instanceType": "t3.small",
          "location": "US East (N. Virginia)",
          "operatingSystem": "Linux"
        },
        "pricePerUnit": "0.0208",
        "currency": "USD",
        "unit": "Hrs"
      }
    ]
  },
  {
    "serviceCode": "AmazonS3",
    "attributeNames": ["storageClass", "location"],
    "products": [
      {
        "sku": "NOCTAXRIS-S3-STANDARD",
        "attributes": {
          "storageClass": "General Purpose",
          "location": "US East (N. Virginia)"
        },
        "pricePerUnit": "0.023",
        "currency": "USD",
        "unit": "GB-Mo"
      }
    ]
  },
  {
    "serviceCode": "AWSLambda",
    "attributeNames": ["group", "location"],
    "products": [
      {
        "sku": "NOCTAXRIS-LAMBDA-REQUESTS",
        "attributes": {
          "group": "AWS-Lambda-Requests",
          "location": "US East (N. Virginia)"
        },
        "pricePerUnit": "0.0000002",
        "currency": "USD",
        "unit": "Request"
      }
    ]
  }
]`

type pricingServiceEntry struct {
	ServiceCode    string              `json:"serviceCode"`
	AttributeNames []string            `json:"attributeNames"`
	Products       []pricingProductRow `json:"products"`
}

type pricingProductRow struct {
	SKU          string            `json:"sku"`
	Attributes   map[string]string `json:"attributes"`
	PricePerUnit string            `json:"pricePerUnit"`
	Currency     string            `json:"currency"`
	Unit         string            `json:"unit"`
}

func loadPricingCatalog() ([]pricingServiceEntry, error) {
	var entries []pricingServiceEntry
	if err := json.Unmarshal([]byte(pricingCatalogJSON), &entries); err != nil {
		return nil, fmt.Errorf("parse pricing catalog: %w", err)
	}
	return entries, nil
}

// PricingServiceSummary is a DescribeServices row.
type PricingServiceSummary struct {
	ServiceCode    string
	AttributeNames []string
}

// PricingProduct is a GetProducts row (PriceList JSON string shape).
type PricingProduct struct {
	SKU        string
	Attributes map[string]string
	PriceList  string
}

// DescribePricingServices returns the static service list, optionally filtered by ServiceCode.
func (s *Store) DescribePricingServices(serviceCode string) ([]PricingServiceSummary, error) {
	_ = s
	entries, err := loadPricingCatalog()
	if err != nil {
		return nil, err
	}
	serviceCode = strings.TrimSpace(serviceCode)
	var out []PricingServiceSummary
	for _, e := range entries {
		if serviceCode != "" && !strings.EqualFold(e.ServiceCode, serviceCode) {
			continue
		}
		out = append(out, PricingServiceSummary{
			ServiceCode:    e.ServiceCode,
			AttributeNames: append([]string(nil), e.AttributeNames...),
		})
	}
	return out, nil
}

// GetPricingAttributeValues returns distinct attribute values for a service attribute.
func (s *Store) GetPricingAttributeValues(serviceCode, attributeName string) ([]string, error) {
	_ = s
	serviceCode = strings.TrimSpace(serviceCode)
	attributeName = strings.TrimSpace(attributeName)
	if serviceCode == "" || attributeName == "" {
		return nil, fmt.Errorf("ServiceCode and AttributeName required")
	}
	entries, err := loadPricingCatalog()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []string
	for _, e := range entries {
		if !strings.EqualFold(e.ServiceCode, serviceCode) {
			continue
		}
		for _, p := range e.Products {
			v, ok := p.Attributes[attributeName]
			if !ok || v == "" {
				continue
			}
			if _, exists := seen[v]; exists {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out, nil
}

// GetPricingProducts returns PriceList JSON strings matching optional attribute filters.
// filters maps attribute name -> required value.
func (s *Store) GetPricingProducts(serviceCode string, filters map[string]string) ([]string, error) {
	_ = s
	serviceCode = strings.TrimSpace(serviceCode)
	if serviceCode == "" {
		return nil, fmt.Errorf("ServiceCode required")
	}
	entries, err := loadPricingCatalog()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !strings.EqualFold(e.ServiceCode, serviceCode) {
			continue
		}
		for _, p := range e.Products {
			if !pricingAttrsMatch(p.Attributes, filters) {
				continue
			}
			pl, err := json.Marshal(map[string]any{
				"product": map[string]any{
					"sku":           p.SKU,
					"attributes":    p.Attributes,
					"productFamily": e.ServiceCode,
				},
				"terms": map[string]any{
					"OnDemand": map[string]any{
						"noctaxris": map[string]any{
							"priceDimensions": map[string]any{
								"noctaxris": map[string]any{
									"unit":         p.Unit,
									"pricePerUnit": map[string]string{p.Currency: p.PricePerUnit},
								},
							},
						},
					},
				},
			})
			if err != nil {
				return nil, err
			}
			out = append(out, string(pl))
		}
	}
	return out, nil
}

func pricingAttrsMatch(attrs, filters map[string]string) bool {
	if len(filters) == 0 {
		return true
	}
	for k, want := range filters {
		got, ok := attrs[k]
		if !ok || !strings.EqualFold(got, want) {
			return false
		}
	}
	return true
}
