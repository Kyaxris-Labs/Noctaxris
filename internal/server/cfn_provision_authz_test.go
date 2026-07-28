package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustCFNFormCreds(
	t *testing.T,
	handler http.Handler,
	values url.Values,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(values.Encode())
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, body, akid, secret, testRegion, "cloudformation", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustCloudControlJSONCreds(
	t *testing.T,
	handler http.Handler,
	target string,
	payload map[string]any,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signHeader(t, req, raw, akid, secret, testRegion, "cloudcontrol", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func setupCFNOnlyUser(t *testing.T, st *store.Store, userName, policyName, allowDoc string) (akid, secret string) {
	t.Helper()
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	policyARN, err := st.CreateManagedPolicy(testAccountID, policyName, allowDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, policyARN); err != nil {
		t.Fatal(err)
	}
	return akid, secret
}

func TestCFNCreateStackIAMElevateDeniedWithoutIAMActions(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	akid, secret := setupCFNOnlyUser(t, st, "cfn-only", "CFNCreateOnly",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"cloudformation:CreateStack","Resource":"*"}]}`)

	tpl := `{"Resources":{"R":{"Type":"AWS::IAM::Role","Properties":{"RoleName":"EscalatedRole","AssumeRolePolicyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}}}}}`
	rec := mustCFNFormCreds(t, handler, url.Values{
		"Action":                {"CreateStack"},
		"Version":               {"2010-05-15"},
		"StackName":             {"iam-elevate"},
		"TemplateBody":          {tpl},
		"Capabilities.member.1": {"CAPABILITY_NAMED_IAM"},
	}, akid, secret, now)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("CreateStack IAM elevate want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}
	if _, _, err := st.GetRole(testAccountID, "EscalatedRole"); err == nil {
		t.Fatal("EscalatedRole must not exist after denied CreateStack")
	}
}

func TestCFNCreateStackIAMRequiresCapability(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	tpl := `{"Resources":{"R":{"Type":"AWS::IAM::Role","Properties":{"RoleName":"NeedsCap","AssumeRolePolicyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}}}}}`
	rec := mustCFNForm(t, handler, url.Values{
		"Action":       {"CreateStack"},
		"Version":      {"2010-05-15"},
		"StackName":    {"iam-nocap"},
		"TemplateBody": {tpl},
	}, now)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "InsufficientCapabilities") {
		t.Fatalf("CreateStack without IAM capability want InsufficientCapabilities status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCFNCreateStackLambdaPassRoleDenied(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	roleARN, err := st.CreateRole(testAccountID, "lambda-exec-cfn",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}

	akid, secret := setupCFNOnlyUser(t, st, "cfn-lambda", "CFNCreateLambdaNoPass",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["cloudformation:CreateStack","lambda:CreateFunction"],"Resource":"*"}]}`)

	tpl := `{"Resources":{"F":{"Type":"AWS::Lambda::Function","Properties":{"FunctionName":"cfn-denied-fn","Runtime":"python3.12","Handler":"index.handler","Role":"` + roleARN + `","Code":{"ZipFile":"def handler(event, context):\n  return {'ok': True}\n"}}}}}`
	rec := mustCFNFormCreds(t, handler, url.Values{
		"Action":       {"CreateStack"},
		"Version":      {"2010-05-15"},
		"StackName":    {"lambda-passrole"},
		"TemplateBody": {tpl},
	}, akid, secret, now)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("CreateStack Lambda without PassRole want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}
	if _, err := st.GetFunction(testAccountID, "cfn-denied-fn"); err == nil {
		t.Fatal("cfn-denied-fn must not exist after PassRole deny")
	}
}

func TestCloudControlCreateIAMRoleDeniedWithoutIAMActions(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	akid, secret := setupCFNOnlyUser(t, st, "cc-only", "CCCreateOnly",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"cloudcontrol:CreateResource","Resource":"*"}]}`)

	desired := `{"RoleName":"CCEscalated","AssumeRolePolicyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}}`
	rec := mustCloudControlJSONCreds(t, handler, "CloudControlApi.CreateResource", map[string]any{
		"TypeName":     "AWS::IAM::Role",
		"DesiredState": desired,
	}, akid, secret, now)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("CloudControl CreateResource IAM elevate want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}
	if _, _, err := st.GetRole(testAccountID, "CCEscalated"); err == nil {
		t.Fatal("CCEscalated must not exist after denied CreateResource")
	}
}

func TestCFNUpdateStackIAMRoleAttachDeniedWithoutPutAttach(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	baseTpl := `{"Resources":{"R":{"Type":"AWS::IAM::Role","Properties":{"RoleName":"CfnAttachGateRole","AssumeRolePolicyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}}}}}`
	create := mustCFNForm(t, handler, url.Values{
		"Action":                {"CreateStack"},
		"Version":               {"2010-05-15"},
		"StackName":             {"iam-attach-gate"},
		"TemplateBody":          {baseTpl},
		"Capabilities.member.1": {"CAPABILITY_NAMED_IAM"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStack status=%d body=%q", create.Code, create.Body.String())
	}

	policyARN, err := st.CreateManagedPolicy(testAccountID, "CfnAttachGateMP",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}

	akid, secret := setupCFNOnlyUser(t, st, "cfn-attach-only", "CFNUpdateNoAttach",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["cloudformation:UpdateStack","iam:UpdateAssumeRolePolicy"],"Resource":"*"}]}`)

	updTpl := `{"Resources":{"R":{"Type":"AWS::IAM::Role","Properties":{"RoleName":"CfnAttachGateRole","AssumeRolePolicyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]},"ManagedPolicyArns":["` + policyARN + `"],"Policies":[{"PolicyName":"EscalateInline","PolicyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}}]}}}}`
	rec := mustCFNFormCreds(t, handler, url.Values{
		"Action":                {"UpdateStack"},
		"Version":               {"2010-05-15"},
		"StackName":             {"iam-attach-gate"},
		"TemplateBody":          {updTpl},
		"Capabilities.member.1": {"CAPABILITY_NAMED_IAM"},
	}, akid, secret, now)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("UpdateStack IAM attach elevate want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}

	roleARN, _, err := st.GetRole(testAccountID, "CfnAttachGateRole")
	if err != nil {
		t.Fatal(err)
	}
	attached, err := st.ListAttachedPolicyRefs(roleARN)
	if err != nil {
		t.Fatal(err)
	}
	if len(attached) != 0 {
		t.Fatalf("role managed policies must be unchanged, got %+v", attached)
	}
	inline, err := st.ListInlinePolicies(roleARN)
	if err != nil {
		t.Fatal(err)
	}
	if len(inline) != 0 {
		t.Fatalf("role inline policies must be unchanged, got %+v", inline)
	}
}

func TestCloudControlUpdateIAMRoleAttachDeniedWithoutPutAttach(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	roleARN, err := st.CreateRole(testAccountID, "CCAttachGateRole",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	policyARN, err := st.CreateManagedPolicy(testAccountID, "CCAttachGateMP",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}

	akid, secret := setupCFNOnlyUser(t, st, "cc-attach-only", "CCUpdateNoAttach",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["cloudcontrol:UpdateResource","iam:UpdateAssumeRolePolicy"],"Resource":"*"}]}`)

	patch, err := json.Marshal(map[string]any{
		"ManagedPolicyArns": []any{policyARN},
		"Policies": []any{
			map[string]any{
				"PolicyName": "CCEscalateInline",
				"PolicyDocument": map[string]any{
					"Version": "2012-10-17",
					"Statement": []any{
						map[string]any{"Effect": "Allow", "Action": "*", "Resource": "*"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := mustCloudControlJSONCreds(t, handler, "CloudControlApi.UpdateResource", map[string]any{
		"TypeName":      "AWS::IAM::Role",
		"Identifier":    "CCAttachGateRole",
		"PatchDocument": string(patch),
	}, akid, secret, now)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("CloudControl UpdateResource IAM attach elevate want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}

	attached, err := st.ListAttachedPolicyRefs(roleARN)
	if err != nil {
		t.Fatal(err)
	}
	if len(attached) != 0 {
		t.Fatalf("role managed policies must be unchanged, got %+v", attached)
	}
	inline, err := st.ListInlinePolicies(roleARN)
	if err != nil {
		t.Fatal(err)
	}
	if len(inline) != 0 {
		t.Fatalf("role inline policies must be unchanged, got %+v", inline)
	}
}

func TestCloudControlDeleteBucketDeniedWithoutDeleteBucket(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "cc-del-gate-bucket"); err != nil {
		t.Fatal(err)
	}

	akid, secret := setupCFNOnlyUser(t, st, "cc-del-only", "CCDeleteOnly",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"cloudcontrol:DeleteResource","Resource":"*"}]}`)

	rec := mustCloudControlJSONCreds(t, handler, "CloudControlApi.DeleteResource", map[string]any{
		"TypeName":   "AWS::S3::Bucket",
		"Identifier": "cc-del-gate-bucket",
	}, akid, secret, now)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("CloudControl DeleteResource without s3:DeleteBucket want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}
	if _, err := st.GetBucket(testAccountID, "cc-del-gate-bucket"); err != nil {
		t.Fatal("bucket must remain after denied DeleteResource")
	}
}

func TestCFNUpdateStackUsesStoredRoleARN(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	roleARN, err := st.CreateRole(testAccountID, "cfn-update-stack-role",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"cloudformation.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "s3-create-only",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:CreateBucket","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}

	baseTpl := `{"Resources":{"B":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-role-rebind-bucket"}}}}`
	create := mustCFNForm(t, handler, url.Values{
		"Action":       {"CreateStack"},
		"Version":      {"2010-05-15"},
		"StackName":    {"role-rebind"},
		"TemplateBody": {baseTpl},
		"RoleARN":      {roleARN},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStack status=%d body=%q", create.Code, create.Body.String())
	}

	// Admin caller has dynamodb:CreateTable; stack RoleARN does not. Update must evaluate as the stack role.
	updTpl := `{"Resources":{"B":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-role-rebind-bucket"}},"T":{"Type":"AWS::DynamoDB::Table","Properties":{"TableName":"cfn-role-rebind-table","AttributeDefinitions":[{"AttributeName":"id","AttributeType":"S"}],"KeySchema":[{"AttributeName":"id","KeyType":"HASH"}],"BillingMode":"PAY_PER_REQUEST"}}}}`
	rec := mustCFNForm(t, handler, url.Values{
		"Action":       {"UpdateStack"},
		"Version":      {"2010-05-15"},
		"StackName":    {"role-rebind"},
		"TemplateBody": {updTpl},
	}, now)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("UpdateStack with stack RoleARN lacking dynamodb:CreateTable want AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}
	if _, err := st.GetTable(testAccountID, "cfn-role-rebind-table"); err == nil {
		t.Fatal("cfn-role-rebind-table must not exist after denied UpdateStack")
	}
}
