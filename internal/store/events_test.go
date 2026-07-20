package store_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openEventsStore(t *testing.T) *store.Store {
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

func TestPutRuleCreatesRuleOnDefaultBus(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	pattern := `{"source":["noctaxris.lab"]}`

	rule, err := st.PutRule(account, "us-east-1", "default", "lab-rule", pattern, "lab rule", store.RuleStateEnabled)
	if err != nil {
		t.Fatal(err)
	}
	wantARN := "arn:aws:events:us-east-1:000000000001:rule/lab-rule"
	if rule.ARN != wantARN {
		t.Fatalf("arn=%q want %q", rule.ARN, wantARN)
	}
	if rule.Pattern != pattern || rule.State != store.RuleStateEnabled {
		t.Fatalf("rule=%+v", rule)
	}

	described, err := st.DescribeRule(account, "default", "lab-rule")
	if err != nil {
		t.Fatal(err)
	}
	if described.Name != "lab-rule" || described.BusName != "default" {
		t.Fatalf("described=%+v", described)
	}

	rules, err := st.ListRules(account, "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Name != "lab-rule" {
		t.Fatalf("rules=%+v", rules)
	}
}

func TestPutTargetsPersistsTargets(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	if _, err := st.PutRule(account, "us-east-1", "default", "lab-rule", `{"source":["x"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	queue, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "evt-target", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := st.PutTargets(account, "default", "lab-rule", []store.EventTargetInput{{
		ID:  "sqs-1",
		ARN: queue.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}

	targets, err := st.ListTargetsByRule(account, "default", "lab-rule")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].ID != "sqs-1" || targets[0].ARN != queue.QueueARN {
		t.Fatalf("targets=%+v", targets)
	}

	if err := st.RemoveTargets(account, "default", "lab-rule", []string{"sqs-1"}); err != nil {
		t.Fatal(err)
	}
	targets, err = st.ListTargetsByRule(account, "default", "lab-rule")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("targets=%+v want empty", targets)
	}
}

func TestPutEventsMatchesRuleAndRecordsTargets(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	pattern := `{"source":["noctaxris.lab"],"detail-type":["demo"],"detail":{"ok":[true]}}`
	if _, err := st.PutRule(account, "us-east-1", "default", "lab-rule", pattern, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	queue, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "evt-match", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(account, "default", "lab-rule", []store.EventTargetInput{{
		ID:  "1",
		ARN: queue.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}

	result, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source:     "noctaxris.lab",
		DetailType: "demo",
		Detail:     `{"ok":true}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.FailedEntryCount != 0 || len(result.Entries) != 1 || result.Entries[0].EventID == "" {
		t.Fatalf("result=%+v", result)
	}

	matches, err := st.GetPutEventsMatches(result.Entries[0].EventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches=%+v", matches)
	}
	if matches[0].TargetID != "1" || matches[0].TargetARN != queue.QueueARN {
		t.Fatalf("match=%+v", matches[0])
	}
	wantRuleARN := "arn:aws:events:us-east-1:000000000001:rule/lab-rule"
	if matches[0].RuleARN != wantRuleARN {
		t.Fatalf("rule arn=%q want %q", matches[0].RuleARN, wantRuleARN)
	}
}

func TestPutEventsSkipsDisabledRule(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	if _, err := st.PutRule(account, "us-east-1", "default", "off-rule", `{"source":["x"]}`, "", store.RuleStateDisabled); err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(account, "default", "off-rule", []store.EventTargetInput{{
		ID:  "1",
		ARN: "arn:aws:sqs:us-east-1:000000000001:noop",
	}}); err != nil {
		t.Fatal(err)
	}

	result, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source:     "x",
		DetailType: "y",
		Detail:     `{}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := st.GetPutEventsMatches(result.Entries[0].EventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches=%+v want none", matches)
	}
}

func TestPutEventsNoMatchOnPatternMismatch(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	if _, err := st.PutRule(account, "us-east-1", "default", "lab-rule", `{"source":["expected"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}

	result, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source:     "other",
		DetailType: "demo",
		Detail:     `{}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := st.GetPutEventsMatches(result.Entries[0].EventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches=%+v", matches)
	}
}

func TestCreateEventBusAndList(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"

	bus, err := st.CreateEventBus(account, "us-east-1", "custom")
	if err != nil {
		t.Fatal(err)
	}
	wantARN := "arn:aws:events:us-east-1:000000000001:event-bus/custom"
	if bus.ARN != wantARN {
		t.Fatalf("arn=%q want %q", bus.ARN, wantARN)
	}

	if _, err := st.CreateEventBus(account, "us-east-1", "custom"); !errors.Is(err, store.ErrEventBusAlreadyExists) {
		t.Fatalf("want ErrEventBusAlreadyExists, got %v", err)
	}

	buses, err := st.ListEventBuses(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(buses) < 2 {
		t.Fatalf("buses=%+v want default+custom", buses)
	}
}

func TestDeleteEventBusRejectsDefault(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	if _, err := st.ListEventBuses(account); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteEventBus(account, store.DefaultEventBusName); !errors.Is(err, store.ErrCannotDeleteDefaultEventBus) {
		t.Fatalf("want ErrCannotDeleteDefaultEventBus, got %v", err)
	}
}

func TestEnableDisableRule(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	if _, err := st.PutRule(account, "us-east-1", "default", "toggle", `{"source":["a"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	if err := st.DisableRule(account, "default", "toggle"); err != nil {
		t.Fatal(err)
	}
	rule, err := st.DescribeRule(account, "default", "toggle")
	if err != nil {
		t.Fatal(err)
	}
	if rule.State != store.RuleStateDisabled {
		t.Fatalf("state=%q", rule.State)
	}
	if err := st.EnableRule(account, "default", "toggle"); err != nil {
		t.Fatal(err)
	}
	rule, err = st.DescribeRule(account, "default", "toggle")
	if err != nil {
		t.Fatal(err)
	}
	if rule.State != store.RuleStateEnabled {
		t.Fatalf("state=%q", rule.State)
	}
}

func TestPutRuleCustomBusARN(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	if _, err := st.CreateEventBus(account, "us-east-1", "custom"); err != nil {
		t.Fatal(err)
	}
	rule, err := st.PutRule(account, "us-east-1", "custom", "bus-rule", `{"source":["x"]}`, "", store.RuleStateEnabled)
	if err != nil {
		t.Fatal(err)
	}
	wantARN := "arn:aws:events:us-east-1:000000000001:rule/custom/bus-rule"
	if rule.ARN != wantARN {
		t.Fatalf("arn=%q want %q", rule.ARN, wantARN)
	}
}

func TestPutEventsSkipsDeliveryWithoutResourcePolicy(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	if _, err := st.PutRule(account, "us-east-1", "default", "lab-rule", `{"source":["noctaxris.lab"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	queue, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "evt-no-policy", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(account, "default", "lab-rule", []store.EventTargetInput{{
		ID:  "1",
		ARN: queue.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}

	result, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source:     "noctaxris.lab",
		DetailType: "demo",
		Detail:     `{"ok":true}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.FailedEntryCount != 0 {
		t.Fatalf("result=%+v", result)
	}

	msgs, err := st.ReceiveMessages(account, "evt-no-policy", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected no delivery without resource policy, got %d messages", len(msgs))
	}
}

func eventsQueuePolicyForDelivery(queueARN string) string {
	return `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + queueARN + `"}]}`
}

func TestPutEventsDeliversWithEventsServiceQueuePolicy(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	if _, err := st.PutRule(account, "us-east-1", "default", "lab-rule", `{"source":["noctaxris.lab"]}`, "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}
	queue, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "evt-policy", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetQueueAttributes(account, "evt-policy", map[string]string{
		"Policy": eventsQueuePolicyForDelivery(queue.QueueARN),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutTargets(account, "default", "lab-rule", []store.EventTargetInput{{
		ID:  "1",
		ARN: queue.QueueARN,
	}}); err != nil {
		t.Fatal(err)
	}

	result, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source:     "noctaxris.lab",
		DetailType: "demo",
		Detail:     `{"ok":true}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.FailedEntryCount != 0 {
		t.Fatalf("result=%+v", result)
	}

	msgs, err := st.ReceiveMessages(account, "evt-policy", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected delivery with events policy, got %d messages", len(msgs))
	}
}

func TestPutEventsDetailKeyMismatch(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"
	patternObj := map[string]any{
		"source":      []string{"noctaxris.lab"},
		"detail-type": []string{"demo"},
		"detail":      map[string]any{"ok": []any{true}},
	}
	raw, _ := json.Marshal(patternObj)
	if _, err := st.PutRule(account, "us-east-1", "default", "detail-rule", string(raw), "", store.RuleStateEnabled); err != nil {
		t.Fatal(err)
	}

	result, err := st.PutEvents(account, []store.PutEventsEntry{{
		Source:     "noctaxris.lab",
		DetailType: "demo",
		Detail:     `{"ok":false}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := st.GetPutEventsMatches(result.Entries[0].EventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches=%+v want none", matches)
	}
}
