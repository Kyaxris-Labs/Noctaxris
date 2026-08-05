package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func cognitoMustOK(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) map[string]any {
	t.Helper()
	rec := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService."+target, "cognito-idp", payload, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status=%d body=%q", target, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCognitoPoolClientListDescribeDelete(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	pool := cognitoMustOK(t, handler, "CreateUserPool", map[string]any{"PoolName": "mgmt-pool"}, now)
	up, _ := pool["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)

	client := cognitoMustOK(t, handler, "CreateUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientName": "mgmt-client",
	}, now)
	upc, _ := client["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	listPools := cognitoMustOK(t, handler, "ListUserPools", map[string]any{"MaxResults": 10}, now)
	if !strings.Contains(mustMarshal(t, listPools), poolID) {
		t.Fatalf("ListUserPools missing pool: %v", listPools)
	}

	descClient := cognitoMustOK(t, handler, "DescribeUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientId": clientID,
	}, now)
	if !strings.Contains(mustMarshal(t, descClient), clientID) {
		t.Fatalf("DescribeUserPoolClient: %v", descClient)
	}

	listClients := cognitoMustOK(t, handler, "ListUserPoolClients", map[string]any{
		"UserPoolId": poolID,
	}, now)
	if !strings.Contains(mustMarshal(t, listClients), clientID) {
		t.Fatalf("ListUserPoolClients: %v", listClients)
	}

	missing := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.DescribeUserPoolClient", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "ClientId": "missing-client",
	}, now)
	if missing.Code == http.StatusOK || !strings.Contains(missing.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("missing client status=%d body=%q", missing.Code, missing.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "cognito-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "cognito-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny-list", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"cognito-idp:ListUserPools","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"MaxResults": 10})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.ListUserPools")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "cognito-idp", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}

	delClient := cognitoMustOK(t, handler, "DeleteUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientId": clientID,
	}, now)
	_ = delClient
	delPool := cognitoMustOK(t, handler, "DeleteUserPool", map[string]any{"UserPoolId": poolID}, now)
	_ = delPool

	gone := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.DeleteUserPool", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
	}, now)
	if gone.Code == http.StatusOK || !strings.Contains(gone.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("delete missing pool status=%d body=%q", gone.Code, gone.Body.String())
	}
}

func TestCognitoForgotPasswordConfirmAndAttributes(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.CognitoInsecureCodes = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	pool := cognitoMustOK(t, handler, "CreateUserPool", map[string]any{"PoolName": "forgot-pool"}, now)
	up, _ := pool["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)
	client := cognitoMustOK(t, handler, "CreateUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientName": "forgot-client",
	}, now)
	upc, _ := client["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	cognitoMustOK(t, handler, "AdminCreateUser", map[string]any{
		"UserPoolId": poolID, "Username": "forgot-user", "TemporaryPassword": "TempPass1!",
	}, now)

	forgot := cognitoMustOK(t, handler, "ForgotPassword", map[string]any{
		"ClientId": clientID, "Username": "forgot-user",
	}, now)
	if forgot["CodeDeliveryDetails"] == nil {
		t.Fatalf("ForgotPassword missing delivery: %v", forgot)
	}
	code, err := st.PeekCognitoConfirmationCode(testAccountID, poolID, "forgot-user", store.CognitoConfirmPurposeForgotPassword)
	if err != nil || code == "" {
		t.Fatalf("peek forgot code: %v %q", err, code)
	}

	badConfirm := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.ConfirmForgotPassword", "cognito-idp", map[string]any{
		"ClientId": clientID, "Username": "forgot-user", "ConfirmationCode": "000000", "Password": "NewPass9!",
	}, now)
	if badConfirm.Code == http.StatusOK || !strings.Contains(badConfirm.Body.String(), "CodeMismatchException") {
		t.Fatalf("bad confirm status=%d body=%q", badConfirm.Code, badConfirm.Body.String())
	}

	cognitoMustOK(t, handler, "ConfirmForgotPassword", map[string]any{
		"ClientId": clientID, "Username": "forgot-user", "ConfirmationCode": code, "Password": "NewPass9!",
	}, now)

	adminAuth := cognitoMustOK(t, handler, "AdminInitiateAuth", map[string]any{
		"UserPoolId": poolID,
		"ClientId":   clientID,
		"AuthFlow":   "ADMIN_USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "forgot-user",
			"PASSWORD": "NewPass9!",
		},
	}, now)
	result, _ := adminAuth["AuthenticationResult"].(map[string]any)
	accessToken, _ := result["AccessToken"].(string)
	if accessToken == "" {
		t.Fatalf("AdminInitiateAuth missing token: %v", adminAuth)
	}

	cognitoMustOK(t, handler, "UpdateUserAttributes", map[string]any{
		"AccessToken": accessToken,
		"UserAttributes": []map[string]any{
			{"Name": "email", "Value": "forgot-user@example.com"},
		},
	}, now)
	cognitoMustOK(t, handler, "GetUserAttributeVerificationCode", map[string]any{
		"AccessToken":   accessToken,
		"AttributeName": "email",
	}, now)
	attrCode, err := st.PeekCognitoConfirmationCode(testAccountID, poolID, "forgot-user", store.CognitoConfirmPurposeAttrVerify)
	if err != nil || attrCode == "" {
		t.Fatalf("peek attr code: %v %q", err, attrCode)
	}
	cognitoMustOK(t, handler, "VerifyUserAttribute", map[string]any{
		"AccessToken":   accessToken,
		"AttributeName": "email",
		"Code":          attrCode,
	}, now)

	badFlow := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminInitiateAuth", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "ClientId": clientID, "AuthFlow": "CUSTOM_AUTH",
		"AuthParameters": map[string]any{"USERNAME": "forgot-user"},
	}, now)
	if badFlow.Code == http.StatusOK || !strings.Contains(badFlow.Body.String(), "InvalidParameterException") {
		t.Fatalf("bad AdminInitiateAuth flow status=%d body=%q", badFlow.Code, badFlow.Body.String())
	}
}

func TestCognitoSignUpResendConfirmation(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.CognitoInsecureCodes = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	pool := cognitoMustOK(t, handler, "CreateUserPool", map[string]any{"PoolName": "signup-pool"}, now)
	up, _ := pool["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)
	client := cognitoMustOK(t, handler, "CreateUserPoolClient", map[string]any{
		"UserPoolId": poolID, "ClientName": "signup-client",
	}, now)
	upc, _ := client["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	cognitoMustOK(t, handler, "SignUp", map[string]any{
		"ClientId": clientID,
		"Username": "signup-user",
		"Password": "SignUp1!",
		"UserAttributes": []map[string]any{
			{"Name": "email", "Value": "signup@example.com"},
		},
	}, now)

	resend := cognitoMustOK(t, handler, "ResendConfirmationCode", map[string]any{
		"ClientId": clientID, "Username": "signup-user",
	}, now)
	if resend["CodeDeliveryDetails"] == nil {
		t.Fatalf("ResendConfirmationCode: %v", resend)
	}
	code, err := st.PeekCognitoConfirmationCode(testAccountID, poolID, "signup-user", store.CognitoConfirmPurposeSignUp)
	if err != nil || code == "" {
		t.Fatalf("peek signup code: %v %q", err, code)
	}
	cognitoMustOK(t, handler, "ConfirmSignUp", map[string]any{
		"ClientId":         clientID,
		"Username":         "signup-user",
		"ConfirmationCode": code,
	}, now)

	missingUser := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.ResendConfirmationCode", "cognito-idp", map[string]any{
		"ClientId": clientID, "Username": "no-such-user",
	}, now)
	if missingUser.Code == http.StatusOK || !strings.Contains(missingUser.Body.String(), "UserNotFoundException") {
		t.Fatalf("resend missing user status=%d body=%q", missingUser.Code, missingUser.Body.String())
	}
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
