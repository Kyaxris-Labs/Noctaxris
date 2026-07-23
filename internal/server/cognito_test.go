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

func TestCognitoRefreshTokenAuthAndRevokeToken(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "refresh-http-pool",
	}, now)
	if createPool.Code != http.StatusOK {
		t.Fatalf("CreateUserPool status=%d body=%q", createPool.Code, createPool.Body.String())
	}
	var poolResp map[string]any
	_ = json.Unmarshal(createPool.Body.Bytes(), &poolResp)
	up, _ := poolResp["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)

	createClient := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPoolClient", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"ClientName": "refresh-client",
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
		"Username":          "dave",
		"TemporaryPassword": "Secret4!",
	}, now)
	if adminCreate.Code != http.StatusOK {
		t.Fatalf("AdminCreateUser status=%d body=%q", adminCreate.Code, adminCreate.Body.String())
	}

	auth := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "dave",
			"PASSWORD": "Secret4!",
		},
	}, now)
	if auth.Code != http.StatusOK {
		t.Fatalf("InitiateAuth status=%d body=%q", auth.Code, auth.Body.String())
	}
	var authResp map[string]any
	_ = json.Unmarshal(auth.Body.Bytes(), &authResp)
	result, _ := authResp["AuthenticationResult"].(map[string]any)
	refreshToken, _ := result["RefreshToken"].(string)
	if refreshToken == "" {
		t.Fatalf("missing refresh token: %s", auth.Body.String())
	}

	// Unsigned REFRESH_TOKEN_AUTH (public IdP).
	refreshBody, _ := json.Marshal(map[string]any{
		"ClientId": clientID,
		"AuthFlow": "REFRESH_TOKEN_AUTH",
		"AuthParameters": map[string]any{
			"REFRESH_TOKEN": refreshToken,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", strings.NewReader(string(refreshBody)))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.InitiateAuth")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("REFRESH_TOKEN_AUTH status=%d body=%q", rec.Code, rec.Body.String())
	}
	var refreshResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &refreshResp)
	refreshed, _ := refreshResp["AuthenticationResult"].(map[string]any)
	newAccess, _ := refreshed["AccessToken"].(string)
	newRefresh, _ := refreshed["RefreshToken"].(string)
	if newAccess == "" {
		t.Fatalf("missing access after refresh: %s", rec.Body.String())
	}
	if newRefresh == "" || newRefresh == refreshToken {
		t.Fatalf("expected rotated refresh token: %s", rec.Body.String())
	}

	revokeBody, _ := json.Marshal(map[string]any{
		"ClientId": clientID,
		"Token":    newRefresh,
	})
	revokeReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", strings.NewReader(string(revokeBody)))
	revokeReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	revokeReq.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.RevokeToken")
	revokeRec := httptest.NewRecorder()
	handler.ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("RevokeToken status=%d body=%q", revokeRec.Code, revokeRec.Body.String())
	}

	againBody, _ := json.Marshal(map[string]any{
		"ClientId": clientID,
		"AuthFlow": "REFRESH_TOKEN",
		"AuthParameters": map[string]any{
			"REFRESH_TOKEN": newRefresh,
		},
	})
	againReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", strings.NewReader(string(againBody)))
	againReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	againReq.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.InitiateAuth")
	againRec := httptest.NewRecorder()
	handler.ServeHTTP(againRec, againReq)
	if againRec.Code == http.StatusOK {
		t.Fatal("refresh after revoke must fail")
	}
	if !strings.Contains(againRec.Body.String(), "NotAuthorizedException") {
		t.Fatalf("expected NotAuthorizedException: %s", againRec.Body.String())
	}
}
