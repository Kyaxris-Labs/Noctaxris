package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIoTTopicRuleARNAndFilterHelpers(t *testing.T) {
	arn := store.IoTTopicRuleARN("us-west-2", "000000000001", "my-rule")
	if arn != "arn:aws:iot:us-west-2:000000000001:rule/my-rule" {
		t.Fatalf("arn=%q", arn)
	}
	if got := store.ExtractIoTTopicFilter(""); got != "" {
		t.Fatalf("empty sql filter=%q", got)
	}
	if got := store.ExtractIoTTopicFilter("SELECT *"); got != "" {
		t.Fatalf("no FROM filter=%q", got)
	}
	if got := store.ExtractIoTTopicFilter("SELECT * FROM "); got != "" {
		t.Fatalf("short FROM filter=%q", got)
	}
	if got := store.ExtractIoTTopicFilter("SELECT * FROM topic"); got != "" {
		t.Fatalf("unquoted FROM filter=%q", got)
	}
	if got := store.ExtractIoTTopicFilter("SELECT * FROM 'open"); got != "" {
		t.Fatalf("unclosed quote filter=%q", got)
	}
	if got := store.ExtractIoTTopicFilter("SELECT * FROM 'dt/+/telem'"); got != "dt/+/telem" {
		t.Fatalf("filter=%q", got)
	}
	if got := store.ExtractIoTTopicFilter(`SELECT * FROM "dt/sensor"`); got != "dt/sensor" {
		t.Fatalf("double-quote filter=%q", got)
	}

	if store.IoTTopicMatches("", "a") || store.IoTTopicMatches("a", "") {
		t.Fatal("empty filter/topic must not match")
	}
	if !store.IoTTopicMatches("dt/+/telem", "dt/dev1/telem") {
		t.Fatal("single-level + should match")
	}
	if store.IoTTopicMatches("dt/+/telem", "dt/dev1/extra/telem") {
		t.Fatal("+ must not span levels")
	}
	if !store.IoTTopicMatches("dt/#", "dt/a/b") {
		t.Fatal("# should match remainder")
	}
	if store.IoTTopicMatches("dt/#/x", "dt/a") {
		t.Fatal("# not last must fail")
	}
	if store.IoTTopicMatches("a/b", "a") {
		t.Fatal("filter longer than topic must fail")
	}
	if !store.IoTTopicMatches("a/b", "a/b") {
		t.Fatal("exact match")
	}
}

func TestIoTTopicRuleCRUDAndBoundaries(t *testing.T) {
	st := openIoTStore(t)
	account := "000000000001"
	region := "us-east-1"
	sqlOK := "SELECT * FROM 'lab/+/events'"

	_, err := st.CreateIoTTopicRule(account, region, "", sqlOK, "", "[]", false)
	if !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("empty name err=%v", err)
	}
	_, err = st.CreateIoTTopicRule(account, region, "r1", "", "", "[]", false)
	if !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("empty sql err=%v", err)
	}
	_, err = st.CreateIoTTopicRule(account, region, "r1", "SELECT 1", "", "[]", false)
	if !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("bad sql err=%v", err)
	}

	rule, err := st.CreateIoTTopicRule(account, region, "rule-a", sqlOK, "desc", `{not-json`, true)
	if err != nil {
		t.Fatal(err)
	}
	if rule.RuleARN == "" || !rule.RuleDisabled || rule.ActionsJSON != "[]" {
		t.Fatalf("rule=%+v", rule)
	}
	_, err = st.CreateIoTTopicRule(account, region, "rule-a", sqlOK, "", "[]", false)
	if !errors.Is(err, store.ErrIoTConflict) {
		t.Fatalf("dup err=%v", err)
	}

	_, err = st.GetIoTTopicRule(account, region, "missing")
	if !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("get missing err=%v", err)
	}

	replaced, err := st.ReplaceIoTTopicRule(account, region, "rule-a", "SELECT * FROM 'lab/other'", "upd", `[{"sqs":{}}]`, false)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.RuleDisabled || replaced.CreatedAt != rule.CreatedAt || replaced.SQL != "SELECT * FROM 'lab/other'" {
		t.Fatalf("replaced=%+v", replaced)
	}
	_, err = st.ReplaceIoTTopicRule(account, region, "rule-a", "", "", "[]", false)
	if !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("replace empty sql err=%v", err)
	}
	_, err = st.ReplaceIoTTopicRule(account, region, "missing", sqlOK, "", "[]", false)
	if !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("replace missing err=%v", err)
	}

	enabled, err := st.CreateIoTTopicRule(account, region, "rule-b", sqlOK, "", "[]", false)
	if err != nil {
		t.Fatal(err)
	}
	list, err := st.ListIoTTopicRules(account, region)
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	enabledList, err := st.ListEnabledIoTTopicRulesByRegion(region)
	if err != nil {
		t.Fatal(err)
	}
	foundEnabled := false
	for _, r := range enabledList {
		if r.RuleName == enabled.RuleName {
			foundEnabled = true
		}
		if r.RuleName == "rule-a" && r.RuleDisabled {
			t.Fatal("disabled rule must not appear in enabled list")
		}
	}
	if !foundEnabled {
		t.Fatalf("enabled list missing rule-b: %+v", enabledList)
	}

	if err := st.SetIoTTopicRuleDisabled(account, region, "rule-b", true); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetIoTTopicRule(account, region, "rule-b")
	if err != nil || !got.RuleDisabled {
		t.Fatalf("disabled got=%+v err=%v", got, err)
	}
	if err := st.SetIoTTopicRuleDisabled(account, region, "missing", true); !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("set disabled missing err=%v", err)
	}

	if err := st.DeleteIoTTopicRule(account, region, "rule-a"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteIoTTopicRule(account, region, "rule-a"); !errors.Is(err, store.ErrIoTNotFound) {
		t.Fatalf("delete missing err=%v", err)
	}
}
