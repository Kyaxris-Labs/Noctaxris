package organizations_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/services/organizations"
)

const reqID = "rid-org"

func TestCreateAccountXML(t *testing.T) {
	raw, err := organizations.CreateAccountXML(organizations.CreateAccountResult{
		RequestID:   reqID,
		CreateID:    "car-abc",
		AccountName: "Member",
		State:       "SUCCEEDED",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "CreateAccountResponse") || !strings.Contains(body, "car-abc") {
		t.Fatalf("unexpected body %s", body)
	}
}

func TestDescribeCreateAccountStatusXML(t *testing.T) {
	raw, err := organizations.DescribeCreateAccountStatusXML(organizations.DescribeCreateAccountStatusResult{
		RequestID:     reqID,
		CreateID:      "car-abc",
		AccountName:   "Member",
		State:         "SUCCEEDED",
		AccountID:     "000000000002",
		FailureReason: "reason",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "000000000002") || !strings.Contains(body, "FailureReason") {
		t.Fatalf("unexpected body %s", body)
	}
}

func TestListAccountsXML(t *testing.T) {
	raw, err := organizations.ListAccountsXML(organizations.ListAccountsResult{
		RequestID:  reqID,
		AccountIDs: []string{"000000000001", "000000000002"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "ListAccountsResponse") || !strings.Contains(body, "ACTIVE") {
		t.Fatalf("body=%s", body)
	}
}

func TestOrganizationalUnitXML(t *testing.T) {
	ou := organizations.CreateOrganizationalUnitResult{
		RequestID: reqID,
		ID:        "ou-1",
		Name:      "Workloads",
		Arn:       "arn:aws:organizations::000000000001:ou/o-1/ou-1",
	}
	createRaw, err := organizations.CreateOrganizationalUnitXML(ou)
	if err != nil {
		t.Fatal(err)
	}
	listRaw, err := organizations.ListOrganizationalUnitsForParentXML(organizations.ListOrganizationalUnitsForParentResult{
		RequestID: reqID,
		Units:     []organizations.CreateOrganizationalUnitResult{ou},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(createRaw), "Workloads") || !strings.Contains(string(listRaw), "ou-1") {
		t.Fatalf("create=%s list=%s", createRaw, listRaw)
	}
}

func TestEnablePolicyTypeXML(t *testing.T) {
	raw, err := organizations.EnablePolicyTypeXML(organizations.EnablePolicyTypeResult{
		RequestID:   reqID,
		RootID:      "r-1",
		RootArn:     "arn:aws:organizations::000000000001:root/r-1",
		RootName:    "Root",
		PolicyTypes: []string{"SERVICE_CONTROL_POLICY", "RESOURCE_CONTROL_POLICY"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "ENABLED") || !strings.Contains(string(raw), "SERVICE_CONTROL_POLICY") {
		t.Fatalf("raw=%s", raw)
	}
}

func TestPolicyXML(t *testing.T) {
	pol := organizations.OrgPolicyResult{
		RequestID:   reqID,
		ID:          "p-1",
		Name:        "DenyS3",
		Type:        "SERVICE_CONTROL_POLICY",
		Arn:         "arn:aws:organizations::000000000001:policy/o-1/service_control_policy/p-1",
		Content:     `{"Version":"2012-10-17","Statement":[]}`,
		Description: "lab",
	}
	createRaw, err := organizations.CreatePolicyXML(pol)
	if err != nil {
		t.Fatal(err)
	}
	descRaw, err := organizations.DescribePolicyXML(pol)
	if err != nil {
		t.Fatal(err)
	}
	listRaw, err := organizations.ListPoliciesXML(organizations.ListPoliciesResult{
		RequestID: reqID,
		Policies:  []organizations.OrgPolicySummary{{ID: pol.ID, Name: pol.Name, Type: pol.Type, Arn: pol.Arn}},
	})
	if err != nil {
		t.Fatal(err)
	}
	listTarget, err := organizations.ListPoliciesForTargetXML(organizations.ListPoliciesResult{
		RequestID: reqID,
		Policies:  []organizations.OrgPolicySummary{{ID: pol.ID, Name: pol.Name, Type: pol.Type, Arn: pol.Arn}},
	})
	if err != nil {
		t.Fatal(err)
	}
	attach, err := organizations.AttachPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	detach, err := organizations.DetachPolicyXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(createRaw), "CreatePolicyResponse") || !strings.Contains(string(descRaw), "DenyS3") {
		t.Fatalf("policy create=%s desc=%s", createRaw, descRaw)
	}
	if !strings.Contains(string(listRaw), "ListPoliciesResponse") ||
		!strings.Contains(string(listTarget), "ListPoliciesForTargetResponse") ||
		!strings.Contains(string(attach), "AttachPolicyResponse") ||
		!strings.Contains(string(detach), "DetachPolicyResponse") {
		t.Fatalf("policy xml attach=%s detach=%s", attach, detach)
	}
}

func TestParentsAndAccountsForParentXML(t *testing.T) {
	parents, err := organizations.ListParentsXML(organizations.ListParentsResult{
		RequestID: reqID,
		Parents:   []organizations.OrgParentEntry{{ID: "r-1", Type: "ROOT"}, {ID: "ou-1", Type: "ORGANIZATIONAL_UNIT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	accts, err := organizations.ListAccountsForParentXML(organizations.ListAccountsForParentResult{
		RequestID:  reqID,
		AccountIDs: []string{"000000000001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	move, err := organizations.MoveAccountXML(reqID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(parents), "ORGANIZATIONAL_UNIT") ||
		!strings.Contains(string(accts), "ListAccountsForParentResponse") ||
		!strings.Contains(string(move), "MoveAccountResponse") {
		t.Fatalf("parents=%s accts=%s move=%s", parents, accts, move)
	}
}
