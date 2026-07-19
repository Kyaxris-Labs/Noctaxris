package organizations_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/services/organizations"
)

func TestCreateAccountXML(t *testing.T) {
	raw, err := organizations.CreateAccountXML(organizations.CreateAccountResult{
		RequestID:   "rid",
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
		RequestID:   "rid",
		CreateID:    "car-abc",
		AccountName: "Member",
		State:       "SUCCEEDED",
		AccountID:   "000000000002",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "000000000002") || !strings.Contains(body, "DescribeCreateAccountStatusResponse") {
		t.Fatalf("unexpected body %s", body)
	}
}
