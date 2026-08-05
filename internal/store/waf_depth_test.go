package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestWAFCoverageWave2ACLRuleGroupEvaluate(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := store.EnsureWAFSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}
	if err := st.EnsureWAFSchema(); err != nil {
		t.Fatal(err)
	}

	empty, err := st.ListWAFWebACLs(account, "REGIONAL")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty acls=%v err=%v", empty, err)
	}
	if _, err := st.GetWAFWebACL(account, "missing", "REGIONAL", ""); !errors.Is(err, store.ErrWAFNotFound) {
		t.Fatalf("missing get: %v", err)
	}
	if _, err := st.UpdateWAFWebACL(account, "missing", "REGIONAL", "", "Allow", nil); !errors.Is(err, store.ErrWAFNotFound) {
		t.Fatalf("missing update: %v", err)
	}
	if _, err := st.CreateWAFRuleGroup(account, "", "", "REGIONAL", 0, nil); !errors.Is(err, store.ErrWAFBadRequest) {
		t.Fatalf("empty rule group name: %v", err)
	}

	acl, err := st.CreateWAFWebACL(account, "us-east-1", "wave2-acl", "REGIONAL", "d", "Allow", []store.WAFRule{
		{
			Name: "block-evil", Priority: 1, Action: "Block",
			ByteMatchStatement: &store.WAFByteMatchStatement{
				SearchString: "evil", FieldToMatchType: "UriPath", PositionalConstraint: "CONTAINS",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListWAFWebACLs(account, "REGIONAL")
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	got, err := st.GetWAFWebACL(account, "wave2-acl", "REGIONAL", "")
	if err != nil || got.Name != "wave2-acl" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	if _, err := st.UpdateWAFWebACL(account, "wave2-acl", "REGIONAL", "wrong-lock", "Block", nil); !errors.Is(err, store.ErrWAFBadRequest) {
		t.Fatalf("lock mismatch: %v", err)
	}
	updated, err := st.UpdateWAFWebACL(account, "wave2-acl", "REGIONAL", acl.LockToken, "Block", []store.WAFRule{})
	if err != nil || updated.DefaultAction != "Block" || updated.LockToken == acl.LockToken {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}

	rg, err := st.CreateWAFRuleGroup(account, "", "wave2-rg", "REGIONAL", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rg.ARN, "us-east-1") || !strings.Contains(rg.ARN, "rulegroup/wave2-rg") {
		t.Fatalf("rule group arn=%q", rg.ARN)
	}
	if _, err := st.CreateWAFRuleGroup(account, "us-east-1", "wave2-rg", "REGIONAL", 1, nil); !errors.Is(err, store.ErrWAFAlreadyExists) {
		t.Fatalf("dup rule group: %v", err)
	}

	resource := "arn:aws:apigateway:us-east-1::/apis/wave2api/stages/$default"
	if err := st.AssociateWAFWebACL(account, acl.ARN, resource); err != nil {
		t.Fatal(err)
	}
	action, associated, err := st.EvaluateAssociatedWAFWithView(account, []string{"", resource, resource}, "", &store.WAFRequestView{
		URI: "/path",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !associated || action != "Block" {
		t.Fatalf("action=%q associated=%v", action, associated)
	}
	none, assocNone, err := st.EvaluateAssociatedWAFWithView(account, []string{"arn:aws:apigateway:us-east-1::/apis/none/stages/$default"}, "x", nil)
	if err != nil || assocNone || none != "" {
		t.Fatalf("none action=%q assoc=%v err=%v", none, assocNone, err)
	}
}
