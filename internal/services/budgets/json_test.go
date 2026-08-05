package budgets_test

import (
	"encoding/json"
	"testing"

	budgetssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/budgets"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestBudgetsJSON(t *testing.T) {
	b := store.Budget{
		BudgetName: "lab", BudgetType: "COST", TimeUnit: "MONTHLY",
		LimitAmount: "100.00", LimitUnit: "USD",
	}
	descRaw, err := budgetssvc.DescribeBudgetJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRaw, &descOut); err != nil {
		t.Fatal(err)
	}
	budget, _ := descOut["Budget"].(map[string]any)
	limit, _ := budget["BudgetLimit"].(map[string]any)
	if limit["Amount"] != "100.00" {
		t.Fatalf("budget=%v", descOut)
	}

	listRaw, err := budgetssvc.DescribeBudgetsJSON([]store.Budget{b})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	items, _ := listOut["Budgets"].([]any)
	if len(items) != 1 {
		t.Fatalf("list=%v", listOut)
	}

	createRaw, err := budgetssvc.CreateBudgetJSON()
	if err != nil || string(createRaw) != "{}" {
		t.Fatalf("create=%s", createRaw)
	}
	delRaw, err := budgetssvc.DeleteBudgetJSON()
	if err != nil || string(delRaw) != "{}" {
		t.Fatalf("delete=%s", delRaw)
	}
}
