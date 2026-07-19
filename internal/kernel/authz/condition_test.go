package authz

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func testUserPrincipal() identity.Principal {
	return identity.Principal{
		AccountID: "000000000001",
		IsRoot:    false,
		Kind:      identity.KindUser,
		UserName:  "alice",
	}
}

func TestEvaluateDenyOnCatalogUnknownConditionKey(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"StringEquals":{"aws:NotARealKeyZZZ":"x"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:PrincipalAccount": "000000000001"},
	}
	if Evaluate(ctx, docs) != Deny {
		t.Fatal("catalog-unknown must Deny")
	}
}

func TestEvaluateAllowWithKnownPrincipalAccount(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"StringEquals":{"aws:PrincipalAccount":"000000000001"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:PrincipalAccount": "000000000001"},
	}
	if Evaluate(ctx, docs) != Allow {
		t.Fatal("expected Allow with matching known key")
	}
}

func TestEvaluateUnpopulatedKnownKeyDoesNotAllow(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"StringEquals":{"aws:RequestedRegion":"us-east-1"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{},
	}
	if Evaluate(ctx, docs) != Deny {
		t.Fatal("unpopulated known key on StringEquals must not Allow")
	}
}

func TestStringNotEqualsMissingKeyMatches(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"StringNotEquals":{"aws:RequestedRegion":"eu-west-1"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{},
	}
	if Evaluate(ctx, docs) != Allow {
		t.Fatal("StringNotEquals with missing key should match per AWS")
	}
}

func TestNullTrueRequiresAbsentKey(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"Null":{"aws:RequestedRegion":"true"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{},
	}
	if Evaluate(ctx, docs) != Allow {
		t.Fatal("Null true with absent key should Allow")
	}
	ctx.ConditionKeys = map[string]string{"aws:RequestedRegion": "us-east-1"}
	if Evaluate(ctx, docs) != Deny {
		t.Fatal("Null true with present key should not Allow")
	}
}

func TestStringEqualsIfExistsAbsentAllows(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"StringEqualsIfExists":{"aws:RequestedRegion":"us-east-1"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{},
	}
	if Evaluate(ctx, docs) != Allow {
		t.Fatal("StringEqualsIfExists with absent key should Allow")
	}
}
