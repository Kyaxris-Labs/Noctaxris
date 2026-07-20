package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustCreateIAMRoleWithCreds(t *testing.T, handler http.Handler, roleName, trust, akid, secret string, now time.Time) {
	t.Helper()
	body := []byte("Action=CreateRole&Version=2010-05-08&RoleName=" + url.QueryEscape(roleName) +
		"&AssumeRolePolicyDocument=" + url.QueryEscape(trust))
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, body, akid, secret, testRegion, "iam", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateRole status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func mustSNSFormWithCreds(
	t *testing.T,
	handler http.Handler,
	form string,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(form)
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, body, akid, secret, testRegion, "sns", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestLambdaCrossAccountIdentityAndFunctionPolicyAllow(t *testing.T) {
	handler, st, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRoleWithCreds(t, handler, "xa-lambda-exec", lambdaTrustOK, ownerAKID, ownerSecret, now)
	roleARN := "arn:aws:iam::" + ownerAccount + ":role/xa-lambda-exec"
	createRec := mustLambdaJSONWithCreds(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "xa-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	fnARN := store.FunctionARN(ownerAccount, testRegion, "xa-fn")

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"lambda:InvokeFunction","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "laminvoke", identityAllow); err != nil {
		t.Fatal(err)
	}
	addRec := mustLambdaJSONWithCreds(t, handler, "AddPermission", map[string]any{
		"FunctionName": "xa-fn",
		"StatementId":  "xa-invoke",
		"Action":       "lambda:InvokeFunction",
		"Principal":    callerUserARN,
	}, ownerAKID, ownerSecret, now)
	if addRec.Code != http.StatusOK {
		t.Fatalf("AddPermission status=%d body=%q", addRec.Code, addRec.Body.String())
	}

	invokeRec := mustLambdaJSONWithCreds(t, handler, "Invoke", map[string]any{
		"FunctionName": fnARN,
		"Payload":      `{"ping":true}`,
	}, callerAKID, callerSecret, now)
	if invokeRec.Code == http.StatusForbidden {
		t.Fatalf("Invoke denied body=%q", invokeRec.Body.String())
	}
	if invokeRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Invoke status=%d want 503 body=%q", invokeRec.Code, invokeRec.Body.String())
	}
}

func TestLambdaCrossAccountIdentityOnlyDeny(t *testing.T) {
	handler, st, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRoleWithCreds(t, handler, "xa-lambda-id", lambdaTrustOK, ownerAKID, ownerSecret, now)
	roleARN := "arn:aws:iam::" + ownerAccount + ":role/xa-lambda-id"
	createRec := mustLambdaJSONWithCreds(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "xa-fn-id",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	fnARN := store.FunctionARN(ownerAccount, testRegion, "xa-fn-id")

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"lambda:InvokeFunction","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "laminvoke", identityAllow); err != nil {
		t.Fatal(err)
	}

	invokeRec := mustLambdaJSONWithCreds(t, handler, "Invoke", map[string]any{
		"FunctionName": fnARN,
		"Payload":      `{"ping":true}`,
	}, callerAKID, callerSecret, now)
	if invokeRec.Code != http.StatusForbidden {
		t.Fatalf("Invoke status=%d want 403 body=%q", invokeRec.Code, invokeRec.Body.String())
	}
}

func TestLambdaCrossAccountPolicyOnlyDeny(t *testing.T) {
	handler, _, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRoleWithCreds(t, handler, "xa-lambda-pol", lambdaTrustOK, ownerAKID, ownerSecret, now)
	roleARN := "arn:aws:iam::" + ownerAccount + ":role/xa-lambda-pol"
	createRec := mustLambdaJSONWithCreds(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "xa-fn-pol",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	fnARN := store.FunctionARN(ownerAccount, testRegion, "xa-fn-pol")

	addRec := mustLambdaJSONWithCreds(t, handler, "AddPermission", map[string]any{
		"FunctionName": "xa-fn-pol",
		"StatementId":  "xa-pol",
		"Action":       "lambda:InvokeFunction",
		"Principal":    callerUserARN,
	}, ownerAKID, ownerSecret, now)
	if addRec.Code != http.StatusOK {
		t.Fatalf("AddPermission status=%d body=%q", addRec.Code, addRec.Body.String())
	}

	invokeRec := mustLambdaJSONWithCreds(t, handler, "Invoke", map[string]any{
		"FunctionName": fnARN,
		"Payload":      `{"ping":true}`,
	}, callerAKID, callerSecret, now)
	if invokeRec.Code != http.StatusForbidden {
		t.Fatalf("Invoke status=%d want 403 body=%q", invokeRec.Code, invokeRec.Body.String())
	}
}

func TestECRCrossAccountIdentityAndRepositoryPolicyAllow(t *testing.T) {
	handler, st, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustECRJSONWithCreds(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "xa-repo",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ecr:DescribeRepositories","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "ecrdesc", identityAllow); err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"ecr:DescribeRepositories","Resource":"*"}]}`,
		callerUserARN,
	)
	setPolRec := mustECRJSONWithCreds(t, handler, "SetRepositoryPolicy", map[string]any{
		"repositoryName": "xa-repo",
		"policyText":     policy,
	}, ownerAKID, ownerSecret, now)
	if setPolRec.Code != http.StatusOK {
		t.Fatalf("SetRepositoryPolicy status=%d body=%q", setPolRec.Code, setPolRec.Body.String())
	}

	descRec := mustECRJSONWithCreds(t, handler, "DescribeRepositories", map[string]any{
		"registryId":      ownerAccount,
		"repositoryNames": []string{"xa-repo"},
	}, callerAKID, callerSecret, now)
	if descRec.Code != http.StatusOK {
		t.Fatalf("DescribeRepositories status=%d want 200 body=%q", descRec.Code, descRec.Body.String())
	}
}

func TestECRCrossAccountIdentityOnlyDeny(t *testing.T) {
	handler, st, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustECRJSONWithCreds(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "xa-repo-id",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ecr:DescribeRepositories","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "ecrdesc", identityAllow); err != nil {
		t.Fatal(err)
	}

	descRec := mustECRJSONWithCreds(t, handler, "DescribeRepositories", map[string]any{
		"registryId":      ownerAccount,
		"repositoryNames": []string{"xa-repo-id"},
	}, callerAKID, callerSecret, now)
	if descRec.Code != http.StatusForbidden {
		t.Fatalf("DescribeRepositories status=%d want 403 body=%q", descRec.Code, descRec.Body.String())
	}
}

func TestECRCrossAccountPolicyOnlyDeny(t *testing.T) {
	handler, _, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustECRJSONWithCreds(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "xa-repo-pol",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"ecr:DescribeRepositories","Resource":"*"}]}`,
		callerUserARN,
	)
	setPolRec := mustECRJSONWithCreds(t, handler, "SetRepositoryPolicy", map[string]any{
		"repositoryName": "xa-repo-pol",
		"policyText":     policy,
	}, ownerAKID, ownerSecret, now)
	if setPolRec.Code != http.StatusOK {
		t.Fatalf("SetRepositoryPolicy status=%d body=%q", setPolRec.Code, setPolRec.Body.String())
	}

	descRec := mustECRJSONWithCreds(t, handler, "DescribeRepositories", map[string]any{
		"registryId":      ownerAccount,
		"repositoryNames": []string{"xa-repo-pol"},
	}, callerAKID, callerSecret, now)
	if descRec.Code != http.StatusForbidden {
		t.Fatalf("DescribeRepositories status=%d want 403 body=%q", descRec.Code, descRec.Body.String())
	}
}

func TestSNSCrossAccountIdentityAndTopicPolicyAllow(t *testing.T) {
	handler, st, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSNSFormWithCreds(t, handler,
		"Action=CreateTopic&Version=2010-03-31&Name=xa-topic",
		ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTopic status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	topicARN := snsTopicARNFromCreate(t, createRec.Body.String())

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sns:Publish","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "snspub", identityAllow); err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"sns:Publish","Resource":"%s"}]}`,
		callerUserARN, topicARN,
	)
	setRec := mustSNSFormWithCreds(t, handler,
		"Action=SetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&AttributeName=Policy&AttributeValue="+url.QueryEscape(policy),
		ownerAKID, ownerSecret, now)
	if setRec.Code != http.StatusOK {
		t.Fatalf("SetTopicAttributes status=%d body=%q", setRec.Code, setRec.Body.String())
	}

	pubRec := mustSNSFormWithCreds(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Message="+url.QueryEscape("xa-hello"),
		callerAKID, callerSecret, now)
	if pubRec.Code != http.StatusOK {
		t.Fatalf("Publish status=%d want 200 body=%q", pubRec.Code, pubRec.Body.String())
	}
	if !strings.Contains(pubRec.Body.String(), "MessageId") {
		t.Fatalf("Publish body missing MessageId: %q", pubRec.Body.String())
	}
}

func TestSNSCrossAccountIdentityOnlyDeny(t *testing.T) {
	handler, st, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSNSFormWithCreds(t, handler,
		"Action=CreateTopic&Version=2010-03-31&Name=xa-topic-id",
		ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTopic status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	topicARN := snsTopicARNFromCreate(t, createRec.Body.String())

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sns:Publish","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "snspub", identityAllow); err != nil {
		t.Fatal(err)
	}

	pubRec := mustSNSFormWithCreds(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Message="+url.QueryEscape("nope"),
		callerAKID, callerSecret, now)
	if pubRec.Code != http.StatusForbidden {
		t.Fatalf("Publish status=%d want 403 body=%q", pubRec.Code, pubRec.Body.String())
	}
}

func TestSNSCrossAccountPolicyOnlyDeny(t *testing.T) {
	handler, _, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSNSFormWithCreds(t, handler,
		"Action=CreateTopic&Version=2010-03-31&Name=xa-topic-pol",
		ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTopic status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	topicARN := snsTopicARNFromCreate(t, createRec.Body.String())

	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"sns:Publish","Resource":"%s"}]}`,
		callerUserARN, topicARN,
	)
	setRec := mustSNSFormWithCreds(t, handler,
		"Action=SetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&AttributeName=Policy&AttributeValue="+url.QueryEscape(policy),
		ownerAKID, ownerSecret, now)
	if setRec.Code != http.StatusOK {
		t.Fatalf("SetTopicAttributes status=%d body=%q", setRec.Code, setRec.Body.String())
	}

	pubRec := mustSNSFormWithCreds(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Message="+url.QueryEscape("nope"),
		callerAKID, callerSecret, now)
	if pubRec.Code != http.StatusForbidden {
		t.Fatalf("Publish status=%d want 403 body=%q", pubRec.Code, pubRec.Body.String())
	}
}

func snsTopicARNFromCreate(t *testing.T, body string) string {
	t.Helper()
	const marker = "<TopicArn>"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("CreateTopic body missing TopicArn: %q", body)
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, "</TopicArn>")
	if j < 0 {
		t.Fatalf("CreateTopic body missing TopicArn close: %q", body)
	}
	return rest[:j]
}