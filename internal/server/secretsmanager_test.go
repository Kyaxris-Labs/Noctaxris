package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustSecretsJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	return mustSecretsJSONWithCreds(t, handler, target, payload, testAccessKey, testSecret, now)
}

func mustSecretsJSONWithCreds(
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
	req.Header.Set("X-Amz-Target", "secretsmanager."+target)
	signHeader(t, req, raw, akid, secret, testRegion, "secretsmanager", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSecretsCreateGetRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "app/db-password",
		"SecretString": "s3cr3t!",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	arn, _ := createOut["ARN"].(string)
	if arn == "" {
		t.Fatalf("missing ARN in %q", createRec.Body.String())
	}

	getRec := mustSecretsJSON(t, handler, "GetSecretValue", map[string]any{
		"SecretId": "app/db-password",
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetSecretValue status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	val, _ := getOut["SecretString"].(string)
	if val != "s3cr3t!" {
		t.Fatalf("SecretString=%q want s3cr3t! body=%q", val, getRec.Body.String())
	}
}

func TestSecretsCreateAlreadyExists(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	first := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "dup-secret",
		"SecretString": "v1",
	}, now)
	if first.Code != http.StatusOK {
		t.Fatalf("first CreateSecret status=%d body=%q", first.Code, first.Body.String())
	}

	second := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "dup-secret",
		"SecretString": "v2",
	}, now)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("second CreateSecret status=%d want 400 body=%q", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "ResourceExistsException") {
		t.Fatalf("expected ResourceExistsException in %q", second.Body.String())
	}
}

func TestSecretsPutDescribeListDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "ops/token",
		"SecretString": "initial",
		"Description":  "lab token",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	putRec := mustSecretsJSON(t, handler, "PutSecretValue", map[string]any{
		"SecretId":     "ops/token",
		"SecretString": "rotated",
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutSecretValue status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	descRec := mustSecretsJSON(t, handler, "DescribeSecret", map[string]any{
		"SecretId": "ops/token",
	}, now)
	if descRec.Code != http.StatusOK {
		t.Fatalf("DescribeSecret status=%d body=%q", descRec.Code, descRec.Body.String())
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRec.Body.Bytes(), &descOut); err != nil {
		t.Fatal(err)
	}
	if desc, _ := descOut["Description"].(string); desc != "lab token" {
		t.Fatalf("Description=%q want lab token body=%q", desc, descRec.Body.String())
	}

	listRec := mustSecretsJSON(t, handler, "ListSecrets", map[string]any{}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListSecrets status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	secretList, _ := listOut["SecretList"].([]any)
	if len(secretList) < 1 {
		t.Fatalf("SecretList len=%d want >=1 body=%q", len(secretList), listRec.Body.String())
	}

	delRec := mustSecretsJSON(t, handler, "DeleteSecret", map[string]any{
		"SecretId": "ops/token",
	}, now)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DeleteSecret status=%d body=%q", delRec.Code, delRec.Body.String())
	}

	getRec := mustSecretsJSON(t, handler, "GetSecretValue", map[string]any{
		"SecretId": "ops/token",
	}, now)
	if getRec.Code != http.StatusBadRequest {
		t.Fatalf("GetSecretValue after delete status=%d want 400 body=%q", getRec.Code, getRec.Body.String())
	}
}

func TestSecretsResourcePolicyLifecycle(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "policy-target",
		"SecretString": "x",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`
	putPolRec := mustSecretsJSON(t, handler, "PutResourcePolicy", map[string]any{
		"SecretId":       "policy-target",
		"ResourcePolicy": policy,
	}, now)
	if putPolRec.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPolRec.Code, putPolRec.Body.String())
	}

	getPolRec := mustSecretsJSON(t, handler, "GetResourcePolicy", map[string]any{
		"SecretId": "policy-target",
	}, now)
	if getPolRec.Code != http.StatusOK {
		t.Fatalf("GetResourcePolicy status=%d body=%q", getPolRec.Code, getPolRec.Body.String())
	}
	var polOut map[string]any
	if err := json.Unmarshal(getPolRec.Body.Bytes(), &polOut); err != nil {
		t.Fatal(err)
	}
	gotPolicy, _ := polOut["ResourcePolicy"].(string)
	if gotPolicy != policy {
		t.Fatalf("ResourcePolicy=%q want %q", gotPolicy, policy)
	}

	delPolRec := mustSecretsJSON(t, handler, "DeleteResourcePolicy", map[string]any{
		"SecretId": "policy-target",
	}, now)
	if delPolRec.Code != http.StatusOK {
		t.Fatalf("DeleteResourcePolicy status=%d body=%q", delPolRec.Code, delPolRec.Body.String())
	}
}

func TestSecretsResourcePolicyAloneGrantsGetSecretValue(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "shared-secret",
		"SecretString": "classified",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	_, guestARN, err := st.CreateUser(testAccountID, "guest")
	if err != nil {
		t.Fatal(err)
	}
	guestAKID, guestSecret, err := st.CreateUserAccessKey(testAccountID, "guest")
	if err != nil {
		t.Fatal(err)
	}

	allowGuest := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`, guestARN)
	putPolRec := mustSecretsJSON(t, handler, "PutResourcePolicy", map[string]any{
		"SecretId":       "shared-secret",
		"ResourcePolicy": allowGuest,
	}, now)
	if putPolRec.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPolRec.Code, putPolRec.Body.String())
	}
	// Caller EvaluateKMS still requires identity kms:Decrypt (default key policy allows account).
	if err := st.PutInlinePolicy(guestARN, "kms-decrypt",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:Decrypt","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}

	getRec := mustSecretsJSONWithCreds(t, handler, "GetSecretValue", map[string]any{
		"SecretId": "shared-secret",
	}, guestAKID, guestSecret, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetSecretValue via resource policy status=%d want 200 body=%q", getRec.Code, getRec.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	val, _ := getOut["SecretString"].(string)
	if val != "classified" {
		t.Fatalf("SecretString=%q want classified body=%q", val, getRec.Body.String())
	}
}

func TestSecretsAccessDeniedWithoutPolicy(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "locked",
		"SecretString": "nope",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	_, _, err := st.CreateUser(testAccountID, "denied")
	if err != nil {
		t.Fatal(err)
	}
	denyAKID, denySecret, err := st.CreateUserAccessKey(testAccountID, "denied")
	if err != nil {
		t.Fatal(err)
	}

	getRec := mustSecretsJSONWithCreds(t, handler, "GetSecretValue", map[string]any{
		"SecretId": "locked",
	}, denyAKID, denySecret, now)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetSecretValue status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("expected AccessDeniedException in %q", getRec.Body.String())
	}
}

func TestSecretsPutResourcePolicyNotRoutedToDynamoDB(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "route-check",
		"SecretString": "x",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`
	putPolRec := mustSecretsJSON(t, handler, "PutResourcePolicy", map[string]any{
		"SecretId":       "route-check",
		"ResourcePolicy": policy,
	}, now)
	if putPolRec.Code != http.StatusOK {
		t.Fatalf("secretsmanager PutResourcePolicy status=%d body=%q", putPolRec.Code, putPolRec.Body.String())
	}
	if strings.Contains(putPolRec.Body.String(), "TableName") {
		t.Fatalf("expected Secrets Manager handler, got DynamoDB-shaped error: %q", putPolRec.Body.String())
	}
}
