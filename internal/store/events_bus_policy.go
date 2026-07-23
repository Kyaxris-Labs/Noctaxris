package store

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

// ParseEventBusARN parses arn:aws:events:region:account:event-bus/NAME.
func ParseEventBusARN(arn string) (region, accountID, busName string, ok bool) {
	arn = strings.TrimSpace(arn)
	parts := strings.Split(arn, ":")
	if len(parts) < 6 || parts[0] != "arn" || parts[2] != "events" {
		return "", "", "", false
	}
	resource := strings.Join(parts[5:], ":")
	if !strings.HasPrefix(resource, "event-bus/") {
		return "", "", "", false
	}
	busName = strings.TrimPrefix(resource, "event-bus/")
	if busName == "" {
		return "", "", "", false
	}
	return parts[3], parts[4], busName, true
}

// ResolveEventBusRef resolves a bus name or ARN to owner account and bus name.
// Bare names use callerAccountID.
func ResolveEventBusRef(callerAccountID, busNameOrARN string) (ownerAccountID, busName string, ok bool) {
	busNameOrARN = strings.TrimSpace(busNameOrARN)
	if busNameOrARN == "" {
		return callerAccountID, DefaultEventBusName, true
	}
	if _, acct, name, parsed := ParseEventBusARN(busNameOrARN); parsed {
		return acct, name, true
	}
	return callerAccountID, normalizeEventBusName(busNameOrARN), true
}

// PutEventBusPolicy replaces the bus resource policy JSON (lab PutPermission depth).
func (s *Store) PutEventBusPolicy(accountID, busName, policyJSON string) error {
	busName = normalizeEventBusName(busName)
	if busName == DefaultEventBusName {
		if err := s.ensureDefaultEventBus(accountID, DefaultEventsRegion); err != nil {
			return err
		}
	}
	if _, err := s.GetEventBus(accountID, busName); err != nil {
		return err
	}
	policyJSON = strings.TrimSpace(policyJSON)
	if policyJSON != "" && !json.Valid([]byte(policyJSON)) {
		return fmt.Errorf("put event bus policy: invalid json")
	}
	_, err := s.db.Exec(
		`UPDATE event_buses SET policy_json = ? WHERE account_id = ? AND bus_name = ?`,
		policyJSON, accountID, busName,
	)
	if err != nil {
		return fmt.Errorf("put event bus policy: %w", err)
	}
	return nil
}

// PutEventBusPermission appends an Allow statement for events:PutEvents (lab PutPermission).
// Optional condition is an IAM Condition object (e.g. StringEquals aws:SourceAccount).
func (s *Store) PutEventBusPermission(accountID, busName, statementID, principal string, actions []string, condition map[string]any) error {
	bus, err := s.DescribeEventBus(accountID, busName)
	if err != nil {
		return err
	}
	statementID = strings.TrimSpace(statementID)
	principal = strings.TrimSpace(principal)
	if statementID == "" || principal == "" {
		return fmt.Errorf("put permission: StatementId and Principal are required")
	}
	if len(actions) == 0 {
		actions = []string{"events:PutEvents"}
	}
	doc := map[string]any{
		"Version":   "2012-10-17",
		"Statement": []any{},
	}
	if strings.TrimSpace(bus.Policy) != "" {
		if err := json.Unmarshal([]byte(bus.Policy), &doc); err != nil {
			return fmt.Errorf("put permission: existing policy: %w", err)
		}
	}
	stmts, _ := doc["Statement"].([]any)
	for _, raw := range stmts {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if sid, _ := m["Sid"].(string); sid == statementID {
			return fmt.Errorf("put permission: statement id already exists")
		}
	}
	principalObj := map[string]any{}
	if strings.Contains(principal, "amazonaws.com") {
		principalObj["Service"] = principal
	} else {
		principalObj["AWS"] = principal
	}
	var actionVal any = actions
	if len(actions) == 1 {
		actionVal = actions[0]
	}
	stmt := map[string]any{
		"Sid":       statementID,
		"Effect":    "Allow",
		"Principal": principalObj,
		"Action":    actionVal,
		"Resource":  bus.ARN,
	}
	if len(condition) > 0 {
		stmt["Condition"] = condition
	}
	stmts = append(stmts, stmt)
	doc["Statement"] = stmts
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return s.PutEventBusPolicy(accountID, busName, string(raw))
}

// EvaluateEventBusPutEvents reports whether caller may PutEvents on the bus (dual-eval).
func EvaluateEventBusPutEvents(caller identity.Principal, identityDocs []string, bus EventBus, region string, conditionKeys map[string]string) authz.Decision {
	ctx := authz.RequestContext{
		Principal:     caller,
		Action:        "events:PutEvents",
		Resource:      bus.ARN,
		Region:        region,
		ConditionKeys: conditionKeys,
	}
	return authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
		Caller:            ctx,
		IdentityDocs:      identityDocs,
		ResourcePolicyDoc: bus.Policy,
		ResourceAccountID: bus.AccountID,
	})
}

// RemoveEventBusPermission removes a statement by Sid from the bus policy.
func (s *Store) RemoveEventBusPermission(accountID, busName, statementID string) error {
	bus, err := s.DescribeEventBus(accountID, busName)
	if err != nil {
		return err
	}
	statementID = strings.TrimSpace(statementID)
	if statementID == "" {
		return fmt.Errorf("remove permission: StatementId is required")
	}
	if strings.TrimSpace(bus.Policy) == "" {
		return fmt.Errorf("remove permission: statement not found")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(bus.Policy), &doc); err != nil {
		return fmt.Errorf("remove permission: existing policy: %w", err)
	}
	stmts, _ := doc["Statement"].([]any)
	filtered := make([]any, 0, len(stmts))
	found := false
	for _, raw := range stmts {
		m, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		if sid, _ := m["Sid"].(string); sid == statementID {
			found = true
			continue
		}
		filtered = append(filtered, m)
	}
	if !found {
		return fmt.Errorf("remove permission: statement not found")
	}
	doc["Statement"] = filtered
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return s.PutEventBusPolicy(accountID, busName, string(raw))
}
