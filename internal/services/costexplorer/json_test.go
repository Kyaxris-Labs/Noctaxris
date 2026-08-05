package costexplorer_test

import (
	"encoding/json"
	"testing"

	cesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/costexplorer"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCostExplorerJSON(t *testing.T) {
	rows := []store.CostExplorerResultByTime{{
		Start: "2024-01-01", End: "2024-01-31",
		Total: map[string]store.CostExplorerMetricValue{
			"BlendedCost": {Amount: "12.34", Unit: "USD"},
		},
	}}
	usageRaw, err := cesvc.GetCostAndUsageJSON(rows)
	if err != nil {
		t.Fatal(err)
	}
	var usageOut map[string]any
	if err := json.Unmarshal(usageRaw, &usageOut); err != nil {
		t.Fatal(err)
	}
	rbt, _ := usageOut["ResultsByTime"].([]any)
	if len(rbt) != 1 {
		t.Fatalf("usage=%v", usageOut)
	}

	_, err = cesvc.GetCostAndUsageJSON(nil)
	if err != nil {
		t.Fatal(err)
	}

	fcRaw, err := cesvc.GetCostForecastJSON(store.CostExplorerMetricValue{Amount: "5.00", Unit: "USD"}, "2024-02-01", "2024-02-28")
	if err != nil {
		t.Fatal(err)
	}
	var fcOut map[string]any
	if err := json.Unmarshal(fcRaw, &fcOut); err != nil {
		t.Fatal(err)
	}
	total, _ := fcOut["Total"].(map[string]any)
	if total["Amount"] != "5.00" {
		t.Fatalf("forecast=%v", fcOut)
	}
}
