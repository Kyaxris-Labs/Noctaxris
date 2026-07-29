package wafv2

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func webACLSummary(a store.WAFWebACL) map[string]any {
	return map[string]any{
		"Name":         a.Name,
		"Id":           a.ID,
		"ARN":          a.ARN,
		"Description":  a.Description,
		"LockToken":    a.LockToken,
	}
}

func webACLDetail(a store.WAFWebACL) map[string]any {
	var rules any
	_ = json.Unmarshal([]byte(a.RulesJSON), &rules)
	return map[string]any{
		"Name":          a.Name,
		"Id":            a.ID,
		"ARN":           a.ARN,
		"Description":   a.Description,
		"DefaultAction": map[string]any{a.DefaultAction: map[string]any{}},
		"Rules":         rules,
	}
}

// CreateWebACLJSON builds a CreateWebACL response.
func CreateWebACLJSON(a store.WAFWebACL) ([]byte, error) {
	return json.Marshal(map[string]any{"Summary": webACLSummary(a)})
}

// UpdateWebACLJSON builds an UpdateWebACL response.
func UpdateWebACLJSON(a store.WAFWebACL) ([]byte, error) {
	return json.Marshal(map[string]any{"NextLockToken": a.LockToken})
}

// GetWebACLJSON builds a GetWebACL response.
func GetWebACLJSON(a store.WAFWebACL) ([]byte, error) {
	return json.Marshal(map[string]any{"WebACL": webACLDetail(a), "LockToken": a.LockToken})
}

// ListWebACLsJSON builds a ListWebACLs response.
func ListWebACLsJSON(acls []store.WAFWebACL) ([]byte, error) {
	items := make([]map[string]any, 0, len(acls))
	for _, a := range acls {
		items = append(items, webACLSummary(a))
	}
	return json.Marshal(map[string]any{"WebACLs": items})
}

// CreateRuleGroupJSON builds a CreateRuleGroup response.
func CreateRuleGroupJSON(g store.WAFRuleGroup) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Summary": map[string]any{
			"Name": g.Name, "Id": g.ID, "ARN": g.ARN, "LockToken": g.LockToken,
		},
	})
}

// AssociateWebACLJSON is an empty OK body.
func AssociateWebACLJSON() ([]byte, error) { return []byte(`{}`), nil }

// EvaluateJSON builds a lab Evaluate helper response (not a real AWS API).
func EvaluateJSON(action string) ([]byte, error) {
	return json.Marshal(map[string]any{"Action": action})
}

func ipSetSummary(ip store.WAFIPSet) map[string]any {
	return map[string]any{
		"Name":        ip.Name,
		"Id":          ip.ID,
		"ARN":         ip.ARN,
		"Description": ip.Description,
		"LockToken":   ip.LockToken,
	}
}

func ipSetDetail(ip store.WAFIPSet) map[string]any {
	addrs := ip.Addresses
	if addrs == nil {
		addrs = []string{}
	}
	return map[string]any{
		"Name":             ip.Name,
		"Id":               ip.ID,
		"ARN":              ip.ARN,
		"Description":      ip.Description,
		"IPAddressVersion": ip.IPAddressVersion,
		"Addresses":        addrs,
	}
}

// CreateIPSetJSON builds a CreateIPSet response.
func CreateIPSetJSON(ip store.WAFIPSet) ([]byte, error) {
	return json.Marshal(map[string]any{"Summary": ipSetSummary(ip)})
}

// GetIPSetJSON builds a GetIPSet response.
func GetIPSetJSON(ip store.WAFIPSet) ([]byte, error) {
	return json.Marshal(map[string]any{"IPSet": ipSetDetail(ip), "LockToken": ip.LockToken})
}

// UpdateIPSetJSON builds an UpdateIPSet response.
func UpdateIPSetJSON(ip store.WAFIPSet) ([]byte, error) {
	return json.Marshal(map[string]any{"NextLockToken": ip.LockToken})
}

// DeleteIPSetJSON is an empty OK body.
func DeleteIPSetJSON() ([]byte, error) { return []byte(`{}`), nil }

// ListIPSetsJSON builds a ListIPSets response.
func ListIPSetsJSON(sets []store.WAFIPSet) ([]byte, error) {
	items := make([]map[string]any, 0, len(sets))
	for _, ip := range sets {
		items = append(items, ipSetSummary(ip))
	}
	return json.Marshal(map[string]any{"IPSets": items})
}
