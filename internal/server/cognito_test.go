package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCognitoPoolJWKSAndInitiateAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "http-pool",
	}, now)
	if createPool.Code != http.StatusOK {
		t.Fatalf("CreateUserPool status=%d body=%q", createPool.Code, createPool.Body.String())
	}
	var poolResp map[string]any
	_ = json.Unmarshal(createPool.Body.Bytes(), &poolResp)
	up, _ := poolResp["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)
	if poolID == "" {
		t.Fatalf("missing pool id: %s", createPool.Body.String())
	}

	createClient := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPoolClient", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"ClientName": "http-client",
	}, now)
	if createClient.Code != http.StatusOK {
		t.Fatalf("CreateUserPoolClient status=%d body=%q", createClient.Code, createClient.Body.String())
	}
	var clientResp map[string]any
	_ = json.Unmarshal(createClient.Body.Bytes(), &clientResp)
	upc, _ := clientResp["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	adminCreate := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminCreateUser", "cognito-idp", map[string]any{
		"UserPoolId":        poolID,
		"Username":          "carol",
		"TemporaryPassword": "Secret3!",
	}, now)
	if adminCreate.Code != http.StatusOK {
		t.Fatalf("AdminCreateUser status=%d body=%q", adminCreate.Code, adminCreate.Body.String())
	}

	jwksPath := store.CognitoJWKSPath("us-east-1", poolID)
	jwksReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566"+jwksPath, nil)
	jwksRec := httptest.NewRecorder()
	handler.ServeHTTP(jwksRec, jwksReq)
	if jwksRec.Code != http.StatusOK {
		t.Fatalf("JWKS status=%d body=%q", jwksRec.Code, jwksRec.Body.String())
	}
	jwksBody, _ := io.ReadAll(jwksRec.Body)

	auth := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "carol",
			"PASSWORD": "Secret3!",
		},
	}, now)
	if auth.Code != http.StatusOK {
		t.Fatalf("InitiateAuth status=%d body=%q", auth.Code, auth.Body.String())
	}
	var authResp map[string]any
	_ = json.Unmarshal(auth.Body.Bytes(), &authResp)
	result, _ := authResp["AuthenticationResult"].(map[string]any)
	idToken, _ := result["IdToken"].(string)
	accessToken, _ := result["AccessToken"].(string)
	if idToken == "" || accessToken == "" {
		t.Fatalf("missing tokens: %s", auth.Body.String())
	}

	claims, err := jwtutil.VerifyCompactRS256(accessToken, jwksBody)
	if err != nil {
		t.Fatal(err)
	}
	if jwtutil.ClaimString(claims, "token_use") != "access" {
		t.Fatalf("token_use=%v", claims["token_use"])
	}
	if !strings.Contains(jwtutil.ClaimString(claims, "iss"), poolID) {
		t.Fatalf("iss=%v", claims["iss"])
	}

	bad := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "carol",
			"PASSWORD": "nope",
		},
	}, now)
	if bad.Code == http.StatusOK {
		t.Fatal("bad password should fail")
	}
	if !strings.Contains(bad.Body.String(), "NotAuthorizedException") {
		t.Fatalf("expected NotAuthorizedException: %s", bad.Body.String())
	}
}
