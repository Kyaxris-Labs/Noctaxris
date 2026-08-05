package authz

import (
	"testing"
	"time"
)

func TestEvaluateDateConditionOperators(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	iso := now.Format(time.RFC3339)

	docsEquals := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"DateEquals":{"aws:CurrentTime":"` + iso + `"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:CurrentTime": iso},
	}
	if Evaluate(ctx, docsEquals) != Allow {
		t.Fatal("DateEquals match must Allow")
	}
	ctx.ConditionKeys["aws:CurrentTime"] = now.Add(time.Hour).Format(time.RFC3339)
	if Evaluate(ctx, docsEquals) != Deny {
		t.Fatal("DateEquals mismatch must not Allow")
	}

	docsLT := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"DateLessThan":{"aws:CurrentTime":"` + now.Add(time.Hour).Format(time.RFC3339) + `"}}
		}]
	}`}
	ctx.ConditionKeys["aws:CurrentTime"] = iso
	if Evaluate(ctx, docsLT) != Allow {
		t.Fatal("DateLessThan must Allow when actual before expected")
	}

	docsGT := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"DateGreaterThan":{"aws:CurrentTime":"` + now.Add(-time.Hour).Format(time.RFC3339) + `"}}
		}]
	}`}
	if Evaluate(ctx, docsGT) != Allow {
		t.Fatal("DateGreaterThan must Allow when actual after expected")
	}

	docsLTE := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"DateLessThanEquals":{"aws:CurrentTime":"` + iso + `"}}
		}]
	}`}
	if Evaluate(ctx, docsLTE) != Allow {
		t.Fatal("DateLessThanEquals equal must Allow")
	}

	docsGTE := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"DateGreaterThanEquals":{"aws:CurrentTime":"` + iso + `"}}
		}]
	}`}
	if Evaluate(ctx, docsGTE) != Allow {
		t.Fatal("DateGreaterThanEquals equal must Allow")
	}

	docsNE := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"DateNotEquals":{"aws:CurrentTime":"` + now.Add(time.Hour).Format(time.RFC3339) + `"}}
		}]
	}`}
	if Evaluate(ctx, docsNE) != Allow {
		t.Fatal("DateNotEquals different times must Allow")
	}

	docsUnix := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"DateEquals":{"aws:EpochTime":"1700000000"}}
		}]
	}`}
	ctxUnix := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:EpochTime": "1700000000"},
	}
	if Evaluate(ctxUnix, docsUnix) != Allow {
		t.Fatal("unix epoch DateEquals must Allow")
	}
}

func TestEvaluateDateCondition_invalidActualFailClosed(t *testing.T) {
	docs := []string{`{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*",
			"Condition":{"DateEquals":{"aws:CurrentTime":"2026-08-01T12:00:00Z"}}
		}]
	}`}
	ctx := RequestContext{
		Principal:     testUserPrincipal(),
		Action:        "s3:GetObject",
		Resource:      "*",
		ConditionKeys: map[string]string{"aws:CurrentTime": "not-a-time"},
	}
	if Evaluate(ctx, docs) != Deny {
		t.Fatal("invalid actual time must fail closed")
	}
}

func TestOrgFiltersDeny(t *testing.T) {
	allowDoc := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]
	}`
	denyish := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"s3:PutObject","Resource":"*"}]
	}`
	ctx := RequestContext{
		Principal: testUserPrincipal(),
		Action:    "s3:GetObject",
		Resource:  "*",
	}
	if OrgFiltersDeny(ctx, EvalInputs{}) {
		t.Fatal("empty filters must not deny")
	}
	if OrgFiltersDeny(ctx, EvalInputs{RCPDocs: []string{allowDoc}}) {
		t.Fatal("allowing RCP must not deny")
	}
	if !OrgFiltersDeny(ctx, EvalInputs{RCPDocs: []string{denyish}}) {
		t.Fatal("non-allowing RCP must deny")
	}
	if !OrgFiltersDeny(ctx, EvalInputs{
		SCPDocs:             []string{denyish},
		IsManagementAccount: false,
	}) {
		t.Fatal("non-allowing SCP must deny for member account")
	}
	if OrgFiltersDeny(ctx, EvalInputs{
		SCPDocs:             []string{denyish},
		IsManagementAccount: true,
	}) {
		t.Fatal("management account skips SCP deny")
	}
}
