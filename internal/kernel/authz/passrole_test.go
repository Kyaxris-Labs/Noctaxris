package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

const (
	passRoleAccountID = "000000000001"
	lambdaExecRoleARN = "arn:aws:iam::000000000001:role/lambda-exec"
)

const lambdaTrustDoc = `{
	"Version":"2012-10-17",
	"Statement":[{
		"Effect":"Allow",
		"Principal":{"Service":"lambda.amazonaws.com"},
		"Action":"sts:AssumeRole"
	}]
}`

func TestCheckPassRoleRootWithLambdaTrust(t *testing.T) {
	req := authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal: identity.RootPrincipal(passRoleAccountID, "AKIAROOT"),
		},
		RoleARN:          lambdaExecRoleARN,
		TrustPolicyDoc:   lambdaTrustDoc,
		ServicePrincipal: authz.ServicePrincipalLambda,
	}
	if got := authz.CheckPassRole(req); got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}

func TestCheckPassRoleNonRootPassRoleAllow(t *testing.T) {
	req := authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal: identity.UserPrincipal(passRoleAccountID, "alice", "AKIAA"),
		},
		IdentityDocs: []string{`{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Action":"iam:PassRole",
				"Resource":"` + lambdaExecRoleARN + `"
			}]
		}`},
		RoleARN:          lambdaExecRoleARN,
		TrustPolicyDoc:   lambdaTrustDoc,
		ServicePrincipal: authz.ServicePrincipalLambda,
	}
	if got := authz.CheckPassRole(req); got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}

func TestCheckPassRoleMissingPassRoleDeny(t *testing.T) {
	req := authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal: identity.UserPrincipal(passRoleAccountID, "alice", "AKIAA"),
		},
		IdentityDocs:     nil,
		RoleARN:          lambdaExecRoleARN,
		TrustPolicyDoc:   lambdaTrustDoc,
		ServicePrincipal: authz.ServicePrincipalLambda,
	}
	if got := authz.CheckPassRole(req); got != authz.Deny {
		t.Fatalf("missing PassRole got %v, want Deny", got)
	}
}

func TestCheckPassRoleAWSPrincipalOnlyDeny(t *testing.T) {
	req := authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal: identity.RootPrincipal(passRoleAccountID, "AKIAROOT"),
		},
		RoleARN: lambdaExecRoleARN,
		TrustPolicyDoc: `{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Principal":{"AWS":"arn:aws:iam::000000000001:root"},
				"Action":"sts:AssumeRole"
			}]
		}`,
		ServicePrincipal: authz.ServicePrincipalLambda,
	}
	if got := authz.CheckPassRole(req); got != authz.Deny {
		t.Fatalf("AWS-only trust got %v, want Deny", got)
	}
}

func passRoleIdentityAllow() string {
	return `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"iam:PassRole",
			"Resource":"` + lambdaExecRoleARN + `"
		}]
	}`
}

func passRoleNonRootBase() authz.PassRoleRequest {
	return authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal: identity.UserPrincipal(passRoleAccountID, "alice", "AKIAA"),
		},
		IdentityDocs:     []string{passRoleIdentityAllow()},
		RoleARN:          lambdaExecRoleARN,
		TrustPolicyDoc:   lambdaTrustDoc,
		ServicePrincipal: authz.ServicePrincipalLambda,
	}
}

func TestCheckPassRoleSCPDenyDespiteIdentityAllow(t *testing.T) {
	req := passRoleNonRootBase()
	req.EvalInputs = authz.EvalInputs{
		SCPDocs: []string{`{
			"Version":"2012-10-17",
			"Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]
		}`},
		IsManagementAccount: false,
	}
	if got := authz.CheckPassRole(req); got != authz.Deny {
		t.Fatalf("SCP without PassRole Allow got %v, want Deny", got)
	}
}

func TestCheckPassRoleBoundaryDenyDespiteIdentityAllow(t *testing.T) {
	req := passRoleNonRootBase()
	req.EvalInputs = authz.EvalInputs{
		BoundaryDoc: `{
			"Version":"2012-10-17",
			"Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]
		}`,
	}
	if got := authz.CheckPassRole(req); got != authz.Deny {
		t.Fatalf("boundary without PassRole Allow got %v, want Deny", got)
	}
}

func TestCheckPassRoleManagementAccountSCPExempt(t *testing.T) {
	req := passRoleNonRootBase()
	req.EvalInputs = authz.EvalInputs{
		SCPDocs: []string{`{
			"Version":"2012-10-17",
			"Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]
		}`},
		IsManagementAccount: true,
	}
	if got := authz.CheckPassRole(req); got != authz.Allow {
		t.Fatalf("management SCP exempt got %v, want Allow", got)
	}
}

func TestCheckPassRoleWrongServiceDeny(t *testing.T) {
	req := authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal: identity.RootPrincipal(passRoleAccountID, "AKIAROOT"),
		},
		RoleARN: lambdaExecRoleARN,
		TrustPolicyDoc: `{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Principal":{"Service":"s3.amazonaws.com"},
				"Action":"sts:AssumeRole"
			}]
		}`,
		ServicePrincipal: authz.ServicePrincipalLambda,
	}
	if got := authz.CheckPassRole(req); got != authz.Deny {
		t.Fatalf("s3 trust for lambda got %v, want Deny", got)
	}
}

func TestCheckPassRolePassedToServiceCondition(t *testing.T) {
	req := authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal: identity.UserPrincipal(passRoleAccountID, "alice", "AKIAA"),
		},
		IdentityDocs: []string{`{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Action":"iam:PassRole",
				"Resource":"` + lambdaExecRoleARN + `",
				"Condition":{"StringEquals":{"iam:PassedToService":"lambda.amazonaws.com"}}
			}]
		}`},
		RoleARN:          lambdaExecRoleARN,
		TrustPolicyDoc:   lambdaTrustDoc,
		ServicePrincipal: authz.ServicePrincipalLambda,
	}
	if got := authz.CheckPassRole(req); got != authz.Allow {
		t.Fatalf("PassedToService lambda got %v, want Allow", got)
	}
	req.ServicePrincipal = authz.ServicePrincipalAPIGateway
	req.TrustPolicyDoc = `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":{"Service":"apigateway.amazonaws.com"},
			"Action":"sts:AssumeRole"
		}]
	}`
	if got := authz.CheckPassRole(req); got != authz.Deny {
		t.Fatalf("PassedToService mismatch got %v, want Deny", got)
	}
}

func TestCheckPassRoleTrustSourceAccountCondition(t *testing.T) {
	trust := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":{"Service":"lambda.amazonaws.com"},
			"Action":"sts:AssumeRole",
			"Condition":{"StringEquals":{"aws:SourceAccount":"` + passRoleAccountID + `"}}
		}]
	}`
	req := authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal: identity.RootPrincipal(passRoleAccountID, "AKIAROOT"),
		},
		RoleARN:          lambdaExecRoleARN,
		TrustPolicyDoc:   trust,
		ServicePrincipal: authz.ServicePrincipalLambda,
	}
	if got := authz.CheckPassRole(req); got != authz.Allow {
		t.Fatalf("SourceAccount match got %v, want Allow", got)
	}

	req.Caller.Principal = identity.RootPrincipal("999999999999", "AKIAROOT")
	if got := authz.CheckPassRole(req); got != authz.Deny {
		t.Fatalf("SourceAccount mismatch got %v, want Deny", got)
	}
}

func TestTrustAllowsServiceConditionFailClosedWithoutKeys(t *testing.T) {
	trust := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":{"Service":"lambda.amazonaws.com"},
			"Action":"sts:AssumeRole",
			"Condition":{"StringEquals":{"aws:SourceAccount":"000000000001"}}
		}]
	}`
	if authz.TrustAllowsService(trust, authz.ServicePrincipalLambda) {
		t.Fatal("StringEquals SourceAccount without keys must fail closed")
	}
	if !authz.TrustAllowsServiceWithKeys(trust, authz.ServicePrincipalLambda, map[string]string{
		"aws:SourceAccount": "000000000001",
	}) {
		t.Fatal("expected Allow with matching SourceAccount")
	}
}

func TestCheckPassRoleTrustSourceArnCondition(t *testing.T) {
	fnARN := "arn:aws:lambda:us-east-1:" + passRoleAccountID + ":function:lab"
	trust := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":{"Service":"lambda.amazonaws.com"},
			"Action":"sts:AssumeRole",
			"Condition":{"ArnLike":{"aws:SourceArn":"arn:aws:lambda:us-east-1:` + passRoleAccountID + `:function:*"}}
		}]
	}`
	req := authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal: identity.RootPrincipal(passRoleAccountID, "AKIAROOT"),
		},
		RoleARN:          lambdaExecRoleARN,
		TrustPolicyDoc:   trust,
		ServicePrincipal: authz.ServicePrincipalLambda,
	}
	if got := authz.CheckPassRole(req); got != authz.Deny {
		t.Fatalf("missing SourceArn got %v, want Deny", got)
	}
	req.SourceArn = fnARN
	if got := authz.CheckPassRole(req); got != authz.Allow {
		t.Fatalf("SourceArn match got %v, want Allow", got)
	}
	req.SourceArn = "arn:aws:lambda:us-east-1:" + passRoleAccountID + ":function:other-acct-shape"
	// still matches function/* — mismatch path:
	req.SourceArn = "arn:aws:sqs:us-east-1:" + passRoleAccountID + ":q"
	if got := authz.CheckPassRole(req); got != authz.Deny {
		t.Fatalf("SourceArn mismatch got %v, want Deny", got)
	}
}
