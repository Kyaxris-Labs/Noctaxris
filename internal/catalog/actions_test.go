package catalog_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
)

func TestKnownActionPhase2(t *testing.T) {
	known := []string{
		catalog.ActionSTSGetCallerIdentity,
		catalog.ActionSTSAssumeRole,
		catalog.ActionOrgsCreateAccount,
		catalog.ActionOrgsDescribeCreateAccountStatus,
		"sts:GetCallerIdentity",
		"sts:AssumeRole",
		"organizations:CreateAccount",
		"organizations:DescribeCreateAccountStatus",
	}
	for _, action := range known {
		if !catalog.KnownAction(action) {
			t.Fatalf("KnownAction(%q) = false, want true", action)
		}
	}
}

func TestKnownActionUnknown(t *testing.T) {
	cases := []string{
		"",
		"iam:GetUser",
		"s3:ListBucket",
		"GetCallerIdentity",
		"sts:getcalleridentity",
		"organizations:ListAccounts",
	}
	for _, action := range cases {
		if catalog.KnownAction(action) {
			t.Fatalf("KnownAction(%q) = true, want false", action)
		}
	}
}

func TestActionConstants(t *testing.T) {
	cases := map[string]string{
		catalog.ActionSTSGetCallerIdentity:            "sts:GetCallerIdentity",
		catalog.ActionSTSAssumeRole:                   "sts:AssumeRole",
		catalog.ActionOrgsCreateAccount:               "organizations:CreateAccount",
		catalog.ActionOrgsDescribeCreateAccountStatus: "organizations:DescribeCreateAccountStatus",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("constant = %q, want %q", got, want)
		}
	}
}
