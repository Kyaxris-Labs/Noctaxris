package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/federation"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestAssumeRoleChainDurationSecondsCapped(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	trust := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":{"AWS":"arn:aws:iam::` + testAccountID + `:root"},
			"Action":"sts:AssumeRole"
		}]
	}`
	roleA := "chain-a"
	roleB := "chain-b"
	arnA := "arn:aws:iam::" + testAccountID + ":role/" + roleA
	arnB := "arn:aws:iam::" + testAccountID + ":role/" + roleB
	if _, err := st.CreateRoleOpts(testAccountID, roleA, trust, 14400); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRoleOpts(testAccountID, roleB, trust, 14400); err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(arnA, "assume-next", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:AssumeRole","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}

	longBody := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(arnA) +
		"&RoleSessionName=long&DurationSeconds=14400")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", longBody)
	signHeader(t, req, longBody, testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("long-term 4h AssumeRole: %d %s", rec.Code, rec.Body.String())
	}
	akid := xmlTag(t, rec.Body.String(), "AccessKeyId")
	secret := xmlTag(t, rec.Body.String(), "SecretAccessKey")
	token := xmlTag(t, rec.Body.String(), "SessionToken")

	denyBody := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(arnB) +
		"&RoleSessionName=chain4h&DurationSeconds=14400")
	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", denyBody)
	signHeader(t, req, denyBody, akid, secret, testRegion, "sts", now)
	req.Header.Set("X-Amz-Security-Token", token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("role-chain 4h should ValidationError, got OK: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ValidationError") && !strings.Contains(rec.Body.String(), "1 hour") {
		t.Fatalf("expected role-chain ValidationError, got: %s", rec.Body.String())
	}

	okBody := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(arnB) +
		"&RoleSessionName=chain1h&DurationSeconds=3600")
	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", okBody)
	signHeader(t, req, okBody, akid, secret, testRegion, "sts", now)
	req.Header.Set("X-Amz-Security-Token", token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("role-chain 1h AssumeRole: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOIDCWebIdentityClaimConditionKeys(t *testing.T) {
	providerARN := "arn:aws:iam::" + testAccountID + ":oidc-provider/token.actions.githubusercontent.com"
	roleARN := "arn:aws:iam::" + testAccountID + ":role/gha"
	trustAllow := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":{"Federated":"` + providerARN + `"},
			"Action":"sts:AssumeRoleWithWebIdentity",
			"Condition":{"StringEquals":{
				"token.actions.githubusercontent.com:sub":"repo:org/app:ref:refs/heads/main",
				"token.actions.githubusercontent.com:aud":"sts.amazonaws.com"
			}}
		}]
	}`
	callerDocs := []string{`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:AssumeRoleWithWebIdentity","Resource":"*"}]}`}
	fed := identity.FederatedProviderPrincipal(testAccountID, providerARN, "oidc", "ASIA")

	mismatch := authz.EvaluateCrossAccount(authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: fed,
			Action:    "sts:AssumeRoleWithWebIdentity",
			Resource:  roleARN,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount":                    testAccountID,
				"token.actions.githubusercontent.com:sub": "repo:org/app:ref:refs/heads/other",
				"token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
			},
		},
		CallerIdentityDocs: callerDocs,
		TrustPolicyDoc:     trustAllow,
	})
	if mismatch != authz.Deny {
		t.Fatalf("claim mismatch got %v, want Deny", mismatch)
	}

	matchKeys := federation.WebIdentityConditionKeys(federation.OIDCClaims{
		Issuer:   "https://token.actions.githubusercontent.com",
		Subject:  "repo:org/app:ref:refs/heads/main",
		Audience: []string{"sts.amazonaws.com"},
		StringClaims: map[string]string{
			"repository": "org/app",
			"ref":        "refs/heads/main",
		},
	})
	matchKeys["aws:PrincipalAccount"] = testAccountID
	match := authz.EvaluateCrossAccount(authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal:     fed,
			Action:        "sts:AssumeRoleWithWebIdentity",
			Resource:      roleARN,
			ConditionKeys: matchKeys,
		},
		CallerIdentityDocs: callerDocs,
		TrustPolicyDoc:     trustAllow,
	})
	if match != authz.Allow {
		t.Fatalf("claim match got %v, want Allow", match)
	}
}

func TestConditionKeysUsernamePrincipalType(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, userARN, err := st.CreateUser(testAccountID, "ctx-alice")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "ctx-alice")
	if err != nil {
		t.Fatal(err)
	}
	policy := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sqs:CreateQueue",
			"Resource":"*",
			"Condition":{"StringEquals":{
				"aws:username":"ctx-alice",
				"aws:PrincipalType":"User"
			}}
		}]
	}`
	if err := st.PutInlinePolicy(userARN, "cond", policy); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]any{"QueueName": "username-cond-q"})
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS.CreateQueue")
	signHeader(t, req, body, userAKID, userSecret, testRegion, "sqs", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateQueue with username/PrincipalType: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeliveryRoleSessionConditionKeysPopulate(t *testing.T) {
	_, st, _ := newTestServerStore(t)
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(testAccountID, "deliver-keys", trust)
	if err != nil {
		t.Fatal(err)
	}
	roleDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sqs:SendMessage",
			"Resource":"*",
			"Condition":{"StringEquals":{
				"aws:PrincipalType":"AssumedRole",
				"aws:PrincipalAccount":"` + testAccountID + `"
			}}
		}]
	}`
	if err := st.PutInlinePolicy(roleARN, "deliver", roleDoc); err != nil {
		t.Fatal(err)
	}
	qARN := fmt.Sprintf("arn:aws:sqs:%s:%s:deliver-keys-q", testRegion, testAccountID)
	if !st.RoleSessionAllows(testAccountID, roleARN, "sqs:SendMessage", qARN, "test", testRegion) {
		t.Fatal("delivery role session with PrincipalType ConditionKeys should Allow")
	}
}
