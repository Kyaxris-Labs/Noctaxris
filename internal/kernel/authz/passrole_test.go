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
