package budgets

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func budgetMap(b store.Budget) map[string]any {
	return map[string]any{
		"BudgetName": b.BudgetName,
		"BudgetType": b.BudgetType,
		"TimeUnit":   b.TimeUnit,
		"BudgetLimit": map[string]string{
			"Amount": b.LimitAmount,
			"Unit":   b.LimitUnit,
		},
	}
}

// CreateBudgetJSON is empty OK.
func CreateBudgetJSON() ([]byte, error) { return []byte(`{}`), nil }

// DescribeBudgetJSON builds DescribeBudget response.
func DescribeBudgetJSON(b store.Budget) ([]byte, error) {
	return json.Marshal(map[string]any{"Budget": budgetMap(b)})
}

// DescribeBudgetsJSON builds DescribeBudgets response.
func DescribeBudgetsJSON(budgets []store.Budget) ([]byte, error) {
	items := make([]map[string]any, 0, len(budgets))
	for _, b := range budgets {
		items = append(items, budgetMap(b))
	}
	return json.Marshal(map[string]any{"Budgets": items})
}

// DeleteBudgetJSON is empty OK.
func DeleteBudgetJSON() ([]byte, error) { return []byte(`{}`), nil }
