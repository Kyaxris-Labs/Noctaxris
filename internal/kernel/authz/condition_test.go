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

func TestEvaluateDenyOnUnknownConditionOperator(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"},
			{"Effect":"Deny","Action":"s3:GetObject","Resource":"*",
			 "Condition":{"BinaryEquals":{"aws:PrincipalAccount":"AA=="}}}
		]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:PrincipalAccount": "000000000001"},
	}
	if Evaluate(ctx, docs) != Deny {
		t.Fatal("unknown operator on Deny must fail closed (request Deny)")
	}
}

func TestEvaluateAllowUnknownOperatorDoesNotAllow(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"BinaryEquals":{"aws:PrincipalAccount":"AA=="}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:PrincipalAccount": "000000000001"},
	}
	if Evaluate(ctx, docs) != Deny {
		t.Fatal("Allow with unknown operator must not Allow")
	}
}

func TestEvaluateDenyIpAddressInsideCIDR(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"},
			{"Effect":"Deny","Action":"s3:GetObject","Resource":"*",
			 "Condition":{"IpAddress":{"aws:SourceIp":["203.0.113.0/24"]}}}
		]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:SourceIp": "203.0.113.9"},
	}
	if Evaluate(ctx, docs) != Deny {
		t.Fatal("Deny+IpAddress with SourceIp inside CIDR must Deny")
	}
	ctx.ConditionKeys["aws:SourceIp"] = "198.51.100.9"
	if Evaluate(ctx, docs) != Allow {
		t.Fatal("Deny+IpAddress with SourceIp outside CIDR must not match Deny")
	}
}

func TestEvaluateIpAddressInsideCIDRAllows(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"IpAddress":{"aws:SourceIp":["203.0.113.0/24"]}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:SourceIp": "203.0.113.9"},
	}
	if Evaluate(ctx, docs) != Allow {
		t.Fatal("SourceIp inside CIDR must Allow")
	}
	ctx.ConditionKeys["aws:SourceIp"] = "198.51.100.9"
	if Evaluate(ctx, docs) != Deny {
		t.Fatal("SourceIp outside CIDR must not Allow")
	}
}

func TestEvaluateBoolAndStringNotLike(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{
				"Bool":{"aws:SecureTransport":"true"},
				"StringNotLike":{"aws:username":"blocked-*"}
			}
		}]
	}`}
	ctx := RequestContext{
		Principal: testUserPrincipal(),
		Action:    "s3:GetObject",
		Resource:  "*",
		ConditionKeys: map[string]string{
			"aws:SecureTransport": "true",
			"aws:username":        "alice",
		},
	}
	if Evaluate(ctx, docs) != Allow {
		t.Fatal("Bool+StringNotLike match must Allow")
	}
	ctx.ConditionKeys["aws:username"] = "blocked-user"
	if Evaluate(ctx, docs) != Deny {
		t.Fatal("StringNotLike mismatch must not Allow")
	}
}

func TestEvaluateForAnyValueStringLike(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"ForAnyValue:StringLike":{"aws:PrincipalOrgPaths":"o-lab/*"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:PrincipalOrgPaths": "o-lab/r-1"},
	}
	if Evaluate(ctx, docs) != Allow {
		t.Fatal("ForAnyValue:StringLike match must Allow")
	}
}

func TestEvaluateNumericLessThan(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"NumericLessThan":{"aws:MultiFactorAuthAge":"300"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:MultiFactorAuthAge": "120"},
	}
	if Evaluate(ctx, docs) != Allow {
		t.Fatal("NumericLessThan match must Allow")
	}
}
