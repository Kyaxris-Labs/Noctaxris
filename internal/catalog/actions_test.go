package catalog_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
)

func TestKnownActionGetCallerIdentity(t *testing.T) {
	if !catalog.KnownAction(catalog.ActionSTSGetCallerIdentity) {
		t.Fatalf("KnownAction(%q) = false, want true", catalog.ActionSTSGetCallerIdentity)
	}
	if !catalog.KnownAction("sts:GetCallerIdentity") {
		t.Fatal(`KnownAction("sts:GetCallerIdentity") = false, want true`)
	}
}

func TestKnownActionUnknown(t *testing.T) {
	cases := []string{
		"",
		"sts:AssumeRole",
		"iam:GetUser",
		"s3:ListBucket",
		"GetCallerIdentity",
		"sts:getcalleridentity",
	}
	for _, action := range cases {
		if catalog.KnownAction(action) {
			t.Fatalf("KnownAction(%q) = true, want false", action)
		}
	}
}

func TestActionSTSGetCallerIdentityConstant(t *testing.T) {
	const want = "sts:GetCallerIdentity"
	if catalog.ActionSTSGetCallerIdentity != want {
		t.Fatalf("ActionSTSGetCallerIdentity = %q, want %q", catalog.ActionSTSGetCallerIdentity, want)
	}
}
