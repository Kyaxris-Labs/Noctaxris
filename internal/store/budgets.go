package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBudgetNotFound   = errors.New("NotFoundException")
	ErrBudgetBadRequest = errors.New("InvalidParameterException")
	ErrBudgetExists     = errors.New("DuplicateRecordException")
)

const budgetsSchema = `
CREATE TABLE IF NOT EXISTS budgets (
  account_id TEXT NOT NULL,
  budget_name TEXT NOT NULL,
  budget_type TEXT NOT NULL,
  time_unit TEXT NOT NULL,
  limit_amount TEXT NOT NULL,
  limit_unit TEXT NOT NULL,
  notifications_json TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, budget_name)
);
`

// Budget is a lab AWS Budgets row.
type Budget struct {
	AccountID     string
	BudgetName    string
	BudgetType    string
	TimeUnit      string
	LimitAmount   string
	LimitUnit     string
	Notifications string
	CreatedAt     int64
}

// EnsureBudgetsSchema creates Budgets tables if missing.
func EnsureBudgetsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure budgets schema: db is nil")
	}
	if _, err := db.Exec(budgetsSchema); err != nil {
		return fmt.Errorf("ensure budgets schema: %w", err)
	}
	return nil
}

// EnsureBudgetsSchema ensures Budgets tables on an open store.
func (s *Store) EnsureBudgetsSchema() error {
	return EnsureBudgetsSchema(s.db)
}

// CreateBudget creates a budget with optional notification stubs.
// When NotificationsWithSubscribers include SNS subscriber ARNs, publishes one lab threshold alert per topic (best-effort).
func (s *Store) CreateBudget(accountID, name, budgetType, timeUnit, amount, unit string, notifications any) (Budget, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Budget{}, fmt.Errorf("%w: BudgetName required", ErrBudgetBadRequest)
	}
	if budgetType == "" {
		budgetType = "COST"
	}
	if timeUnit == "" {
		timeUnit = "MONTHLY"
	}
	if amount == "" {
		return Budget{}, fmt.Errorf("%w: BudgetLimit.Amount required", ErrBudgetBadRequest)
	}
	if unit == "" {
		unit = "USD"
	}
	notifJSON := "[]"
	if notifications != nil {
		raw, err := json.Marshal(notifications)
		if err != nil {
			return Budget{}, fmt.Errorf("%w: notifications", ErrBudgetBadRequest)
		}
		notifJSON = string(raw)
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO budgets (account_id, budget_name, budget_type, time_unit, limit_amount, limit_unit, notifications_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, budgetType, timeUnit, amount, unit, notifJSON, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Budget{}, ErrBudgetExists
		}
		return Budget{}, fmt.Errorf("create budget: %w", err)
	}
	b := Budget{
		AccountID: accountID, BudgetName: name, BudgetType: budgetType, TimeUnit: timeUnit,
		LimitAmount: amount, LimitUnit: unit, Notifications: notifJSON, CreatedAt: now,
	}
	s.notifyBudgetSNS(accountID, name, amount, unit, notifications)
	return b, nil
}

// notifyBudgetSNS publishes a lab ACTUAL threshold alert to SNS topics listed under NotificationsWithSubscribers.
func (s *Store) notifyBudgetSNS(accountID, budgetName, amount, unit string, notifications any) {
	topicARNs := extractBudgetSNSTopicARNs(notifications)
	if len(topicARNs) == 0 {
		return
	}
	msg := fmt.Sprintf(
		`{"notificationType":"ACTUAL","budgetName":"%s","budgetLimit":{"amount":"%s","unit":"%s"},"message":"Noctaxris lab budget threshold notification"}`,
		budgetName, amount, unit,
	)
	for _, arn := range topicARNs {
		topic, err := s.GetTopicByARN(arn)
		if err != nil || topic.AccountID != accountID {
			continue
		}
		_, _ = s.Publish(accountID, topic.TopicName, msg, "AWS Budgets Notification", nil)
	}
}

func extractBudgetSNSTopicARNs(notifications any) []string {
	list, ok := notifications.([]any)
	if !ok {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		subs, _ := m["Subscribers"].([]any)
		for _, sub := range subs {
			sm, ok := sub.(map[string]any)
			if !ok {
				continue
			}
			subType, _ := sm["SubscriptionType"].(string)
			if subType == "" {
				subType, _ = sm["subscriptionType"].(string)
			}
			if !strings.EqualFold(subType, "SNS") {
				continue
			}
			addr, _ := sm["Address"].(string)
			if addr == "" {
				addr, _ = sm["address"].(string)
			}
			addr = strings.TrimSpace(addr)
			if addr == "" || !strings.HasPrefix(addr, "arn:aws:sns:") {
				continue
			}
			if _, dup := seen[addr]; dup {
				continue
			}
			seen[addr] = struct{}{}
			out = append(out, addr)
		}
	}
	return out
}

// DescribeBudget returns one budget.
func (s *Store) DescribeBudget(accountID, name string) (Budget, error) {
	name = strings.TrimSpace(name)
	var b Budget
	err := s.db.QueryRow(
		`SELECT account_id, budget_name, budget_type, time_unit, limit_amount, limit_unit, notifications_json, created_at
		 FROM budgets WHERE account_id = ? AND budget_name = ?`,
		accountID, name,
	).Scan(&b.AccountID, &b.BudgetName, &b.BudgetType, &b.TimeUnit, &b.LimitAmount, &b.LimitUnit, &b.Notifications, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Budget{}, ErrBudgetNotFound
	}
	if err != nil {
		return Budget{}, fmt.Errorf("describe budget: %w", err)
	}
	return b, nil
}

// DescribeBudgets lists budgets for an account.
func (s *Store) DescribeBudgets(accountID string) ([]Budget, error) {
	rows, err := s.db.Query(
		`SELECT account_id, budget_name, budget_type, time_unit, limit_amount, limit_unit, notifications_json, created_at
		 FROM budgets WHERE account_id = ? ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("describe budgets: %w", err)
	}
	defer rows.Close()
	var out []Budget
	for rows.Next() {
		var b Budget
		if err := rows.Scan(&b.AccountID, &b.BudgetName, &b.BudgetType, &b.TimeUnit, &b.LimitAmount, &b.LimitUnit, &b.Notifications, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("describe budgets scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteBudget deletes a budget by name.
func (s *Store) DeleteBudget(accountID, name string) error {
	res, err := s.db.Exec(`DELETE FROM budgets WHERE account_id = ? AND budget_name = ?`, accountID, strings.TrimSpace(name))
	if err != nil {
		return fmt.Errorf("delete budget: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete budget: %w", err)
	}
	if n == 0 {
		return ErrBudgetNotFound
	}
	return nil
}
