package store

import (
	"fmt"
	"strings"
)

// CostExplorerResultByTime is a seeded Cost Explorer time bucket.
type CostExplorerResultByTime struct {
	Start string
	End   string
	Total map[string]CostExplorerMetricValue
}

// CostExplorerMetricValue is amount + unit.
type CostExplorerMetricValue struct {
	Amount string
	Unit   string
}

// CostExplorerGetCostAndUsage returns seeded lab cost rows (no live AWS sync).
func (s *Store) CostExplorerGetCostAndUsage(granularity string, metrics []string) ([]CostExplorerResultByTime, error) {
	_ = s
	granularity = strings.ToUpper(strings.TrimSpace(granularity))
	if granularity == "" {
		return nil, fmt.Errorf("Granularity required")
	}
	if granularity != "DAILY" && granularity != "MONTHLY" && granularity != "HOURLY" {
		return nil, fmt.Errorf("Granularity must be DAILY, MONTHLY, or HOURLY")
	}
	if len(metrics) == 0 {
		return nil, fmt.Errorf("Metrics required")
	}
	allowed := map[string]struct{}{
		"UnblendedCost": {}, "BlendedCost": {}, "AmortizedCost": {},
		"NetUnblendedCost": {}, "NetAmortizedCost": {},
		"UsageQuantity": {}, "NormalizedUsageAmount": {},
	}
	total := map[string]CostExplorerMetricValue{}
	for _, m := range metrics {
		m = strings.TrimSpace(m)
		if _, ok := allowed[m]; !ok {
			return nil, fmt.Errorf("unsupported metric %q", m)
		}
		amount := "12.34"
		unit := "USD"
		if m == "UsageQuantity" || m == "NormalizedUsageAmount" {
			amount = "100"
			unit = "N/A"
		}
		total[m] = CostExplorerMetricValue{Amount: amount, Unit: unit}
	}
	start, end := "2026-07-01", "2026-08-01"
	if granularity == "DAILY" {
		start, end = "2026-07-01", "2026-07-02"
	}
	if granularity == "HOURLY" {
		start, end = "2026-07-01T00:00:00Z", "2026-07-01T01:00:00Z"
	}
	return []CostExplorerResultByTime{{
		Start: start,
		End:   end,
		Total: total,
	}}, nil
}

// CostExplorerGetCostForecast returns a seeded forecast total.
func (s *Store) CostExplorerGetCostForecast(metric string) (CostExplorerMetricValue, string, string, error) {
	_ = s
	metric = strings.TrimSpace(metric)
	if metric == "" {
		metric = "UNBLENDED_COST"
	}
	switch strings.ToUpper(metric) {
	case "UNBLENDED_COST", "BLENDED_COST", "AMORTIZED_COST", "NET_UNBLENDED_COST", "NET_AMORTIZED_COST":
		return CostExplorerMetricValue{Amount: "45.67", Unit: "USD"}, "2026-08-01", "2026-09-01", nil
	default:
		return CostExplorerMetricValue{}, "", "", fmt.Errorf("unsupported Metric %q", metric)
	}
}
