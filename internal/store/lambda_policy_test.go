package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLambdaFunctionPolicyLifecycle(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		FunctionName: "policy-fn",
		RoleARN:      "arn:aws:iam::" + account + ":role/exec",
		Runtime:      "python3.12",
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}

	principal := "arn:aws:iam::" + account + ":user/guest"
	if _, err := st.GetFunctionPolicy(account, "policy-fn"); !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("want ErrNoSuchResourcePolicy, got %v", err)
	}

	_, err = st.AddFunctionPermission(account, "policy-fn", "guest-invoke", "lambda:InvokeFunction", principal, "", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetFunctionPolicy(account, "policy-fn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"Sid":"guest-invoke"`) {
		t.Fatalf("policy=%q missing guest-invoke sid", got)
	}
	if !strings.Contains(got, fn.FunctionARN) {
		t.Fatalf("policy=%q missing function arn %s", got, fn.FunctionARN)
	}

	if err := st.RemoveFunctionPermission(account, "policy-fn", "guest-invoke"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetFunctionPolicy(account, "policy-fn"); !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("want ErrNoSuchResourcePolicy after remove, got %v", err)
	}
}

func TestLambdaAddFunctionPermissionDuplicateSid(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		FunctionName: "dup-sid",
		RoleARN:      "arn:aws:iam::" + account + ":role/exec",
		Runtime:      "python3.12",
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		Zip:          zip,
	}); err != nil {
		t.Fatal(err)
	}
	principal := "arn:aws:iam::" + account + ":user/guest"
	if _, err := st.AddFunctionPermission(account, "dup-sid", "sid-1", "lambda:InvokeFunction", principal, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(account, "dup-sid", "sid-1", "lambda:InvokeFunction", principal, "", ""); !errors.Is(err, store.ErrLambdaPolicyStatementExists) {
		t.Fatalf("want ErrLambdaPolicyStatementExists, got %v", err)
	}
}

func TestLambdaAddFunctionPermissionAllowsLabCrossAccountPrincipal(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		FunctionName: "cross-acct",
		RoleARN:      "arn:aws:iam::" + account + ":role/exec",
		Runtime:      "python3.12",
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		Zip:          zip,
	}); err != nil {
		t.Fatal(err)
	}
	_, otherAccount, err := st.CreateMemberAccount(account, "xa@example.com", "XA")
	if err != nil {
		t.Fatal(err)
	}
	other := "arn:aws:iam::" + otherAccount + ":user/outsider"
	stmt, err := st.AddFunctionPermission(account, "cross-acct", "x", "lambda:InvokeFunction", other, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stmt, other) {
		t.Fatalf("statement=%q missing principal %s", stmt, other)
	}
}

func TestLambdaAddFunctionPermissionRejectsUnknownAccountPrincipal(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		FunctionName: "unknown-acct",
		RoleARN:      "arn:aws:iam::" + account + ":role/exec",
		Runtime:      "python3.12",
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		Zip:          zip,
	}); err != nil {
		t.Fatal(err)
	}
	other := "arn:aws:iam::000000000099:user/outsider"
	if _, err := st.AddFunctionPermission(account, "unknown-acct", "x", "lambda:InvokeFunction", other, "", ""); err == nil {
		t.Fatal("expected unknown principal account error")
	}
}

func TestLambdaAddFunctionPermissionServicePrincipalForeignSourceAccount(t *testing.T) {
	st := openLambdaStore(t)
	fnOwner := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    fnOwner,
		FunctionName: "svc-xa",
		RoleARN:      "arn:aws:iam::" + fnOwner + ":role/exec",
		Runtime:      "python3.12",
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		Zip:          zip,
	}); err != nil {
		t.Fatal(err)
	}
	sourceAccount := "000000000002"
	if err := st.EnsureRoot(sourceAccount, "AKIAROOT000000000002", "secret-b"); err != nil {
		t.Fatal(err)
	}
	sourceARN := "arn:aws:events:us-east-1:" + sourceAccount + ":rule/default/xa-lab"
	stmt, err := st.AddFunctionPermission(fnOwner, "svc-xa", "eb-xa", "lambda:InvokeFunction",
		"events.amazonaws.com", sourceAccount, sourceARN)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stmt, `"Service":"events.amazonaws.com"`) {
		t.Fatalf("statement missing service principal: %s", stmt)
	}
	if !strings.Contains(stmt, `"aws:SourceAccount"`) || !strings.Contains(stmt, sourceAccount) {
		t.Fatalf("statement missing foreign SourceAccount: %s", stmt)
	}
	if !strings.Contains(stmt, `"aws:SourceArn"`) || !strings.Contains(stmt, sourceARN) {
		t.Fatalf("statement missing SourceArn: %s", stmt)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + fnOwner + ":function:svc-xa"
	if !st.DeliveryTargetResourcePolicyAllows(sourceAccount, fnARN, "lambda:InvokeFunction", "events.amazonaws.com", sourceARN) {
		t.Fatal("foreign source with matching SourceArn must Allow")
	}
	wrong := "arn:aws:events:us-east-1:" + sourceAccount + ":rule/default/other"
	if st.DeliveryTargetResourcePolicyAllows(sourceAccount, fnARN, "lambda:InvokeFunction", "events.amazonaws.com", wrong) {
		t.Fatal("mismatched SourceArn must Deny")
	}
}

func TestLambdaAddFunctionPermissionNormalizesAccountIDPrincipal(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		FunctionName: "acct-id-prin",
		RoleARN:      "arn:aws:iam::" + account + ":role/exec",
		Runtime:      "python3.12",
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		Zip:          zip,
	}); err != nil {
		t.Fatal(err)
	}
	_, otherAccount, err := st.CreateMemberAccount(account, "acctid@example.com", "AcctID")
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := st.AddFunctionPermission(account, "acct-id-prin", "y", "lambda:InvokeFunction", otherAccount, "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := "arn:aws:iam::" + otherAccount + ":root"
	if !strings.Contains(stmt, want) {
		t.Fatalf("statement=%q want root principal %s", stmt, want)
	}
}
