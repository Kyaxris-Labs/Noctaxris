package pricing_test

import (
	"encoding/json"
	"testing"

	pricingsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/pricing"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestPricingJSON(t *testing.T) {
	svcRaw, err := pricingsvc.DescribeServicesJSON([]store.PricingServiceSummary{{
		ServiceCode: "AmazonEC2", AttributeNames: []string{"instanceType"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var svcOut map[string]any
	if err := json.Unmarshal(svcRaw, &svcOut); err != nil {
		t.Fatal(err)
	}
	services, _ := svcOut["Services"].([]any)
	if len(services) != 1 {
		t.Fatalf("services=%v", svcOut)
	}

	attrRaw, err := pricingsvc.GetAttributeValuesJSON([]string{"t3.micro", ""})
	if err != nil {
		t.Fatal(err)
	}
	var attrOut map[string]any
	if err := json.Unmarshal(attrRaw, &attrOut); err != nil {
		t.Fatal(err)
	}
	vals, _ := attrOut["AttributeValues"].([]any)
	if len(vals) != 2 {
		t.Fatalf("attrs=%v", attrOut)
	}

	prodRaw, err := pricingsvc.GetProductsJSON([]string{`{"sku":"x"}`})
	if err != nil {
		t.Fatal(err)
	}
	var prodOut map[string]any
	if err := json.Unmarshal(prodRaw, &prodOut); err != nil {
		t.Fatal(err)
	}
	pl, _ := prodOut["PriceList"].([]any)
	if len(pl) != 1 {
		t.Fatalf("products=%v", prodOut)
	}

	_, err = pricingsvc.DescribeServicesJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
}
