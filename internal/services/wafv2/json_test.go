package wafv2_test

import (
	"encoding/json"
	"testing"

	wafv2svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/wafv2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestWAFv2JSON(t *testing.T) {
	acl := store.WAFWebACL{
		Name: "lab", ID: "id1", ARN: "arn:acl", Description: "d", LockToken: "tok",
		DefaultAction: "Allow", RulesJSON: `[{"Name":"r1"}]`,
	}
	raw, err := wafv2svc.CreateWebACLJSON(acl)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	raw, _ = wafv2svc.UpdateWebACLJSON(acl)
	_ = json.Unmarshal(raw, &out)
	raw, _ = wafv2svc.GetWebACLJSON(acl)
	_ = json.Unmarshal(raw, &out)
	acl.RulesJSON = `{invalid`
	raw, _ = wafv2svc.GetWebACLJSON(acl)

	raw, _ = wafv2svc.ListWebACLsJSON([]store.WAFWebACL{acl})
	_ = json.Unmarshal(raw, &out)

	rg := store.WAFRuleGroup{Name: "rg", ID: "rg1", ARN: "arn:rg", LockToken: "t"}
	raw, _ = wafv2svc.CreateRuleGroupJSON(rg)

	if _, err := wafv2svc.AssociateWebACLJSON(); err != nil {
		t.Fatal(err)
	}
	raw, _ = wafv2svc.EvaluateJSON("BLOCK")
	_ = json.Unmarshal(raw, &out)

	ip := store.WAFIPSet{
		Name: "ips", ID: "ip1", ARN: "arn:ip", Description: "d", LockToken: "t",
		IPAddressVersion: "IPV4", Addresses: []string{"1.2.3.4/32"},
	}
	raw, _ = wafv2svc.CreateIPSetJSON(ip)
	raw, _ = wafv2svc.GetIPSetJSON(ip)
	ip.Addresses = nil
	raw, _ = wafv2svc.GetIPSetJSON(ip)
	raw, _ = wafv2svc.UpdateIPSetJSON(ip)
	if _, err := wafv2svc.DeleteIPSetJSON(); err != nil {
		t.Fatal(err)
	}
	raw, _ = wafv2svc.ListIPSetsJSON([]store.WAFIPSet{ip})
	_ = json.Unmarshal(raw, &out)
}
