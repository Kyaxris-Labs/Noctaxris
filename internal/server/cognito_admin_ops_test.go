package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCognitoAdminUserOpsCreateGetListSetDisableDelete(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	pool := cognitoMustOK(t, handler, "CreateUserPool", map[string]any{"PoolName": "admin-ops-pool"}, now)
	up, _ := pool["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)
	if poolID == "" {
		t.Fatalf("missing pool id: %v", pool)
	}
	client := cognitoMustOK(t, handler, "CreateUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientName": "admin-ops-client",
	}, now)
	upc, _ := client["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	created := cognitoMustOK(t, handler, "AdminCreateUser", map[string]any{
		"UserPoolId": poolID, "Username": "admin-ops-user", "TemporaryPassword": "TempPass1!",
	}, now)
	userObj, _ := created["User"].(map[string]any)
	if userObj["Username"] != "admin-ops-user" {
		t.Fatalf("AdminCreateUser: %v", created)
	}

	getUser := cognitoMustOK(t, handler, "AdminGetUser", map[string]any{
		"UserPoolId": poolID, "Username": "admin-ops-user",
	}, now)
	if getUser["Username"] != "admin-ops-user" || getUser["Enabled"] != true {
		t.Fatalf("AdminGetUser: %v", getUser)
	}

	listUsers := cognitoMustOK(t, handler, "ListUsers", map[string]any{"UserPoolId": poolID}, now)
	if !strings.Contains(mustMarshal(t, listUsers), "admin-ops-user") {
		t.Fatalf("ListUsers: %v", listUsers)
	}

	cognitoMustOK(t, handler, "AdminSetUserPassword", map[string]any{
		"UserPoolId": poolID, "Username": "admin-ops-user", "Password": "NewPass9!", "Permanent": true,
	}, now)
	auth := cognitoMustOK(t, handler, "AdminInitiateAuth", map[string]any{
		"UserPoolId": poolID,
		"ClientId":   clientID,
		"AuthFlow":   "ADMIN_USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "admin-ops-user",
			"PASSWORD": "NewPass9!",
		},
	}, now)
	if auth["AuthenticationResult"] == nil {
		t.Fatalf("AdminInitiateAuth after set password: %v", auth)
	}

	cognitoMustOK(t, handler, "AdminDisableUser", map[string]any{
		"UserPoolId": poolID, "Username": "admin-ops-user",
	}, now)
	disabled := cognitoMustOK(t, handler, "AdminGetUser", map[string]any{
		"UserPoolId": poolID, "Username": "admin-ops-user",
	}, now)
	if disabled["Enabled"] != false {
		t.Fatalf("AdminGetUser after disable: %v", disabled)
	}
	denyAuth := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminInitiateAuth", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"ClientId":   clientID,
		"AuthFlow":   "ADMIN_USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "admin-ops-user",
			"PASSWORD": "NewPass9!",
		},
	}, now)
	if denyAuth.Code == http.StatusOK || !strings.Contains(denyAuth.Body.String(), "NotAuthorizedException") {
		t.Fatalf("disabled user auth status=%d body=%q", denyAuth.Code, denyAuth.Body.String())
	}

	cognitoMustOK(t, handler, "AdminDeleteUser", map[string]any{
		"UserPoolId": poolID, "Username": "admin-ops-user",
	}, now)
	gone := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminGetUser", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "Username": "admin-ops-user",
	}, now)
	if gone.Code == http.StatusOK || !strings.Contains(gone.Body.String(), "UserNotFoundException") {
		t.Fatalf("AdminGetUser after delete status=%d body=%q", gone.Code, gone.Body.String())
	}

	_ = st
}

func TestCognitoAdminUserOpsNegatives(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	pool := cognitoMustOK(t, handler, "CreateUserPool", map[string]any{"PoolName": "admin-neg-pool"}, now)
	up, _ := pool["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)

	cognitoMustOK(t, handler, "AdminCreateUser", map[string]any{
		"UserPoolId": poolID, "Username": "neg-user", "TemporaryPassword": "TempPass1!",
	}, now)

	dup := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminCreateUser", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "Username": "neg-user", "TemporaryPassword": "TempPass1!",
	}, now)
	if dup.Code == http.StatusOK || !strings.Contains(dup.Body.String(), "UsernameExistsException") {
		t.Fatalf("duplicate create status=%d body=%q", dup.Code, dup.Body.String())
	}

	missingPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminCreateUser", "cognito-idp", map[string]any{
		"UserPoolId": "us-east-1_missing", "Username": "x", "TemporaryPassword": "TempPass1!",
	}, now)
	if missingPool.Code == http.StatusOK || !strings.Contains(missingPool.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("missing pool create status=%d body=%q", missingPool.Code, missingPool.Body.String())
	}

	emptyUser := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminCreateUser", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "Username": " ", "TemporaryPassword": "TempPass1!",
	}, now)
	if emptyUser.Code == http.StatusOK || !strings.Contains(emptyUser.Body.String(), "InvalidParameterException") {
		t.Fatalf("empty username status=%d body=%q", emptyUser.Code, emptyUser.Body.String())
	}

	missingGet := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminGetUser", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "Username": "no-such-user",
	}, now)
	if missingGet.Code == http.StatusOK || !strings.Contains(missingGet.Body.String(), "UserNotFoundException") {
		t.Fatalf("AdminGetUser missing status=%d body=%q", missingGet.Code, missingGet.Body.String())
	}

	missingSet := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminSetUserPassword", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "Username": "no-such-user", "Password": "NewPass9!", "Permanent": true,
	}, now)
	if missingSet.Code == http.StatusOK || !strings.Contains(missingSet.Body.String(), "UserNotFoundException") {
		t.Fatalf("AdminSetUserPassword missing status=%d body=%q", missingSet.Code, missingSet.Body.String())
	}

	missingDel := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminDeleteUser", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "Username": "no-such-user",
	}, now)
	if missingDel.Code == http.StatusOK || !strings.Contains(missingDel.Body.String(), "UserNotFoundException") {
		t.Fatalf("AdminDeleteUser missing status=%d body=%q", missingDel.Code, missingDel.Body.String())
	}

	missingDisable := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminDisableUser", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "Username": "no-such-user",
	}, now)
	if missingDisable.Code == http.StatusOK || !strings.Contains(missingDisable.Body.String(), "UserNotFoundException") {
		t.Fatalf("AdminDisableUser missing status=%d body=%q", missingDisable.Code, missingDisable.Body.String())
	}

	listMissingPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.ListUsers", "cognito-idp", map[string]any{
		"UserPoolId": "us-east-1_missing",
	}, now)
	if listMissingPool.Code == http.StatusOK || !strings.Contains(listMissingPool.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("ListUsers missing pool status=%d body=%q", listMissingPool.Code, listMissingPool.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "cognito-admin-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "cognito-admin-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny-admin-get", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"cognito-idp:AdminGetUser","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"UserPoolId": poolID, "Username": "neg-user"})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.AdminGetUser")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "cognito-idp", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}
