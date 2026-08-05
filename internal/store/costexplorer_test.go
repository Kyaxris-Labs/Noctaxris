package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openCostExplorerStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCostExplorerGetCostAndUsage(t *testing.T) {
	st := openCostExplorerStore(t)

	_, err := st.CostExplorerGetCostAndUsage("", []string{"UnblendedCost"})
	if err == nil || !strings.Contains(err.Error(), "Granularity required") {
		t.Fatalf("empty granularity err=%v", err)
	}
	_, err = st.CostExplorerGetCostAndUsage("WEEKLY", []string{"UnblendedCost"})
	if err == nil || !strings.Contains(err.Error(), "DAILY") {
		t.Fatalf("bad granularity err=%v", err)
	}
	_, err = st.CostExplorerGetCostAndUsage("MONTHLY", nil)
	if err == nil || !strings.Contains(err.Error(), "Metrics required") {
		t.Fatalf("empty metrics err=%v", err)
	}
	_, err = st.CostExplorerGetCostAndUsage("MONTHLY", []string{"Nope"})
	if err == nil || !strings.Contains(err.Error(), "unsupported metric") {
		t.Fatalf("bad metric err=%v", err)
	}

	monthly, err := st.CostExplorerGetCostAndUsage("monthly", []string{"UnblendedCost", "UsageQuantity"})
	if err != nil || len(monthly) != 1 {
		t.Fatalf("monthly=%v err=%v", monthly, err)
	}
	if monthly[0].Start != "2026-07-01" || monthly[0].End != "2026-08-01" {
		t.Fatalf("monthly window=%+v", monthly[0])
	}
	if monthly[0].Total["UnblendedCost"].Unit != "USD" || monthly[0].Total["UsageQuantity"].Amount != "100" {
		t.Fatalf("totals=%+v", monthly[0].Total)
	}

	daily, err := st.CostExplorerGetCostAndUsage("DAILY", []string{"BlendedCost"})
	if err != nil || daily[0].End != "2026-07-02" {
		t.Fatalf("daily=%v err=%v", daily, err)
	}
	hourly, err := st.CostExplorerGetCostAndUsage("HOURLY", []string{"NormalizedUsageAmount"})
	if err != nil || !strings.Contains(hourly[0].Start, "T00:00:00Z") {
		t.Fatalf("hourly=%v err=%v", hourly, err)
	}
}

func TestCostExplorerGetCostForecast(t *testing.T) {
	st := openCostExplorerStore(t)

	total, start, end, err := st.CostExplorerGetCostForecast("")
	if err != nil || total.Amount != "45.67" || start == "" || end == "" {
		t.Fatalf("default forecast=%+v %s %s err=%v", total, start, end, err)
	}
	_, _, _, err = st.CostExplorerGetCostForecast("NET_AMORTIZED_COST")
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = st.CostExplorerGetCostForecast("bogus")
	if err == nil || !strings.Contains(err.Error(), "unsupported Metric") {
		t.Fatalf("bad forecast err=%v", err)
	}
}
