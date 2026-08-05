package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openBudgetsStore(t *testing.T) *store.Store {
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
	if err := st.EnsureBudgetsSchema(); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestBudgetsCRUDAndSNSNotify(t *testing.T) {
	st := openBudgetsStore(t)
	account := "000000000001"

	if err := store.EnsureBudgetsSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}

	_, err := st.CreateBudget(account, "", "COST", "MONTHLY", "10", "USD", nil)
	if !errors.Is(err, store.ErrBudgetBadRequest) {
		t.Fatalf("empty name err=%v", err)
	}
	_, err = st.CreateBudget(account, "b1", "", "", "", "USD", nil)
	if !errors.Is(err, store.ErrBudgetBadRequest) {
		t.Fatalf("empty amount err=%v", err)
	}

	topic, err := st.CreateTopic(account, "us-east-1", "budget-alerts", nil)
	if err != nil {
		t.Fatal(err)
	}
	notifs := []any{
		map[string]any{
			"Subscribers": []any{
				map[string]any{"SubscriptionType": "SNS", "Address": topic.TopicARN},
				map[string]any{"subscriptionType": "EMAIL", "address": "a@example.com"},
				map[string]any{"SubscriptionType": "SNS", "Address": "not-an-arn"},
				map[string]any{"SubscriptionType": "SNS", "Address": topic.TopicARN},
			},
		},
		"skip-me",
	}
	b, err := st.CreateBudget(account, "lab-budget", "", "", "100.0", "", notifs)
	if err != nil {
		t.Fatal(err)
	}
	if b.BudgetType != "COST" || b.TimeUnit != "MONTHLY" || b.LimitUnit != "USD" {
		t.Fatalf("defaults=%+v", b)
	}
	_, err = st.CreateBudget(account, "lab-budget", "COST", "MONTHLY", "1", "USD", nil)
	if !errors.Is(err, store.ErrBudgetExists) {
		t.Fatalf("dup err=%v", err)
	}

	got, err := st.DescribeBudget(account, "lab-budget")
	if err != nil || got.LimitAmount != "100.0" {
		t.Fatalf("describe=%+v err=%v", got, err)
	}
	_, err = st.DescribeBudget(account, "missing")
	if !errors.Is(err, store.ErrBudgetNotFound) {
		t.Fatalf("missing describe err=%v", err)
	}
	list, err := st.DescribeBudgets(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteBudget(account, "lab-budget"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteBudget(account, "lab-budget"); !errors.Is(err, store.ErrBudgetNotFound) {
		t.Fatalf("delete missing err=%v", err)
	}
}
