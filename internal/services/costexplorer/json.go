package costexplorer

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// GetCostAndUsageJSON builds GetCostAndUsage response.
func GetCostAndUsageJSON(rows []store.CostExplorerResultByTime) ([]byte, error) {
	results := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		total := map[string]any{}
		for k, v := range r.Total {
			total[k] = map[string]string{"Amount": v.Amount, "Unit": v.Unit}
		}
		results = append(results, map[string]any{
			"TimePeriod": map[string]string{"Start": r.Start, "End": r.End},
			"Total":      total,
			"Estimated":  false,
		})
	}
	return json.Marshal(map[string]any{"ResultsByTime": results, "DimensionValueAttributes": []any{}})
}

// GetCostForecastJSON builds GetCostForecast response.
func GetCostForecastJSON(total store.CostExplorerMetricValue, start, end string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Total":      map[string]string{"Amount": total.Amount, "Unit": total.Unit},
		"ForecastResultsByTime": []map[string]any{{
			"TimePeriod": map[string]string{"Start": start, "End": end},
			"MeanValue":  total.Amount,
		}},
	})
}
