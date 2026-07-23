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

func TestCognitoSoftwareTokenMFAChallengeRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "mfa-http-pool",
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
		"ClientName": "mfa-client",
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
		"Username":          "mfa-user",
		"TemporaryPassword": "Secret5!",
	}, now)
	if adminCreate.Code != http.StatusOK {
		t.Fatalf("AdminCreateUser status=%d body=%q", adminCreate.Code, adminCreate.Body.String())
	}

	auth := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "mfa-user",
			"PASSWORD": "Secret5!",
		},
	}, now)
	if auth.Code != http.StatusOK {
		t.Fatalf("InitiateAuth status=%d body=%q", auth.Code, auth.Body.String())
	}
	var authResp map[string]any
	_ = json.Unmarshal(auth.Body.Bytes(), &authResp)
	result, _ := authResp["AuthenticationResult"].(map[string]any)
	accessToken, _ := result["AccessToken"].(string)
	if accessToken == "" {
		t.Fatalf("missing access token: %s", auth.Body.String())
	}

	assocBody, _ := json.Marshal(map[string]any{"AccessToken": accessToken})
	assocReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", strings.NewReader(string(assocBody)))
	assocReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	assocReq.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.AssociateSoftwareToken")
	assocRec := httptest.NewRecorder()
	handler.ServeHTTP(assocRec, assocReq)
	if assocRec.Code != http.StatusOK {
		t.Fatalf("AssociateSoftwareToken status=%d body=%q", assocRec.Code, assocRec.Body.String())
	}
	var assocResp map[string]any
	_ = json.Unmarshal(assocRec.Body.Bytes(), &assocResp)
	secretCode, _ := assocResp["SecretCode"].(string)
	assocSession, _ := assocResp["Session"].(string)
	if secretCode == "" || assocSession == "" {
		t.Fatalf("associate response: %s", assocRec.Body.String())
	}

	code, err := store.GenerateCognitoTOTP(secretCode, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	verifyBody, _ := json.Marshal(map[string]any{
		"Session":  assocSession,
		"UserCode": code,
	})
	verifyReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", strings.NewReader(string(verifyBody)))
	verifyReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	verifyReq.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.VerifySoftwareToken")
	verifyRec := httptest.NewRecorder()
	handler.ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("VerifySoftwareToken status=%d body=%q", verifyRec.Code, verifyRec.Body.String())
	}

	challenged := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "mfa-user",
			"PASSWORD": "Secret5!",
		},
	}, now)
	if challenged.Code != http.StatusOK {
		t.Fatalf("InitiateAuth MFA status=%d body=%q", challenged.Code, challenged.Body.String())
	}
	var challengeResp map[string]any
	_ = json.Unmarshal(challenged.Body.Bytes(), &challengeResp)
	if challengeResp["ChallengeName"] != "SOFTWARE_TOKEN_MFA" {
		t.Fatalf("want SOFTWARE_TOKEN_MFA: %s", challenged.Body.String())
	}
	session, _ := challengeResp["Session"].(string)
	if session == "" {
		t.Fatal("missing challenge session")
	}
	if _, ok := challengeResp["AuthenticationResult"]; ok {
		t.Fatal("must not return AuthenticationResult before MFA")
	}

	totp, err := store.GenerateCognitoTOTP(secretCode, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	respondBody, _ := json.Marshal(map[string]any{
		"ClientId":      clientID,
		"ChallengeName": "SOFTWARE_TOKEN_MFA",
		"Session":       session,
		"ChallengeResponses": map[string]any{
			"USERNAME":                "mfa-user",
			"SOFTWARE_TOKEN_MFA_CODE": totp,
		},
	})
	respondReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", strings.NewReader(string(respondBody)))
	respondReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	respondReq.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.RespondToAuthChallenge")
	respondRec := httptest.NewRecorder()
	handler.ServeHTTP(respondRec, respondReq)
	if respondRec.Code != http.StatusOK {
		t.Fatalf("RespondToAuthChallenge status=%d body=%q", respondRec.Code, respondRec.Body.String())
	}
	var respondResp map[string]any
	_ = json.Unmarshal(respondRec.Body.Bytes(), &respondResp)
	authResult, _ := respondResp["AuthenticationResult"].(map[string]any)
	if authResult["AccessToken"] == "" {
		t.Fatalf("missing access after MFA: %s", respondRec.Body.String())
	}

	badRespond, _ := json.Marshal(map[string]any{
		"ClientId":      clientID,
		"ChallengeName": "SOFTWARE_TOKEN_MFA",
		"Session":       mustInitiateMFASession(t, handler, clientID, now),
		"ChallengeResponses": map[string]any{
			"USERNAME":                "mfa-user",
			"SOFTWARE_TOKEN_MFA_CODE": "000000",
		},
	})
	badReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", strings.NewReader(string(badRespond)))
	badReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	badReq.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.RespondToAuthChallenge")
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code == http.StatusOK || !strings.Contains(badRec.Body.String(), "CodeMismatchException") {
		t.Fatalf("want CodeMismatchException: status=%d body=%q", badRec.Code, badRec.Body.String())
	}
}

func TestCognitoUSER_SRP_AUTHHTTPRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "srp-http-pool",
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
		"ClientName": "srp-http-client",
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
		"Username":          "srp-http",
		"TemporaryPassword": "Secret9!",
	}, now)
	if adminCreate.Code != http.StatusOK {
		t.Fatalf("AdminCreateUser status=%d body=%q", adminCreate.Code, adminCreate.Body.String())
	}

	srpClient, err := store.NewCognitoSRPClient(poolID, "srp-http", "Secret9!")
	if err != nil {
		t.Fatal(err)
	}
	init := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_SRP_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "srp-http",
			"SRP_A":    srpClient.SRPAHex(),
		},
	}, now)
	if init.Code != http.StatusOK {
		t.Fatalf("InitiateAuth USER_SRP_AUTH status=%d body=%q", init.Code, init.Body.String())
	}
	var initResp map[string]any
	_ = json.Unmarshal(init.Body.Bytes(), &initResp)
	if initResp["ChallengeName"] != "PASSWORD_VERIFIER" {
		t.Fatalf("ChallengeName=%v body=%s", initResp["ChallengeName"], init.Body.String())
	}
	session, _ := initResp["Session"].(string)
	chalParamsAny, _ := initResp["ChallengeParameters"].(map[string]any)
	chalParams := map[string]string{}
	for k, v := range chalParamsAny {
		if s, ok := v.(string); ok {
			chalParams[k] = s
		}
	}
	responses, err := srpClient.PasswordVerifierChallengeResponses(chalParams, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	respMap := map[string]any{}
	for k, v := range responses {
		respMap[k] = v
	}
	respond := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.RespondToAuthChallenge", "cognito-idp", map[string]any{
		"ClientId":           clientID,
		"ChallengeName":      "PASSWORD_VERIFIER",
		"Session":            session,
		"ChallengeResponses": respMap,
	}, now)
	if respond.Code != http.StatusOK {
		t.Fatalf("RespondToAuthChallenge status=%d body=%q", respond.Code, respond.Body.String())
	}
	var authResp map[string]any
	_ = json.Unmarshal(respond.Body.Bytes(), &authResp)
	result, _ := authResp["AuthenticationResult"].(map[string]any)
	if result["AccessToken"] == nil || result["IdToken"] == nil {
		t.Fatalf("missing tokens: %s", respond.Body.String())
	}
}

func TestCognitoUpdateUserPoolRoleArnPassRole(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "trigger-pool",
	}, now)
	if createPool.Code != http.StatusOK {
		t.Fatalf("CreateUserPool status=%d body=%q", createPool.Code, createPool.Body.String())
	}
	var poolResp map[string]any
	_ = json.Unmarshal(createPool.Body.Bytes(), &poolResp)
	up, _ := poolResp["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)
	poolARN, _ := up["Arn"].(string)
	if poolARN == "" {
		poolARN = "arn:aws:cognito-idp:us-east-1:" + testAccountID + ":userpool/" + poolID
	}

	mustCreateIAMRole(t, handler, "cognito-trigger-ok",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"cognito-idp.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	mustCreateIAMRole(t, handler, "cognito-trigger-deny",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	okARN := "arn:aws:iam::" + testAccountID + ":role/cognito-trigger-ok"
	denyARN := "arn:aws:iam::" + testAccountID + ":role/cognito-trigger-deny"
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:pre-signup"

	deny := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.UpdateUserPool", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"RoleArn":    denyARN,
		"LambdaConfig": map[string]any{
			"PreSignUp": lambdaARN,
		},
	}, now)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("wrong trust UpdateUserPool status=%d want 403 body=%q", deny.Code, deny.Body.String())
	}

	allow := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.UpdateUserPool", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"RoleArn":    okARN,
		"LambdaConfig": map[string]any{
			"PreSignUp": lambdaARN,
		},
	}, now)
	if allow.Code != http.StatusOK {
		t.Fatalf("UpdateUserPool status=%d body=%q", allow.Code, allow.Body.String())
	}

	desc := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.DescribeUserPool", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeUserPool status=%d body=%q", desc.Code, desc.Body.String())
	}
	if !strings.Contains(desc.Body.String(), okARN) || !strings.Contains(desc.Body.String(), lambdaARN) {
		t.Fatalf("Describe missing RoleArn/LambdaConfig: %s", desc.Body.String())
	}

	mustCreateIAMRole(t, handler, "cognito-srcarn-ok", trustWithSourceArn("cognito-idp.amazonaws.com", poolARN), now)
	mustCreateIAMRole(t, handler, "cognito-srcarn-bad", trustWithSourceArn("cognito-idp.amazonaws.com",
		"arn:aws:cognito-idp:us-east-1:"+testAccountID+":userpool/other"), now)
	srcOK := "arn:aws:iam::" + testAccountID + ":role/cognito-srcarn-ok"
	srcBad := "arn:aws:iam::" + testAccountID + ":role/cognito-srcarn-bad"

	srcDeny := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.UpdateUserPool", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"RoleArn":    srcBad,
		"LambdaConfig": map[string]any{
			"PostConfirmation": lambdaARN,
		},
	}, now)
	if srcDeny.Code != http.StatusForbidden {
		t.Fatalf("wrong SourceArn status=%d want 403 body=%q", srcDeny.Code, srcDeny.Body.String())
	}
	srcAllow := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.UpdateUserPool", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"RoleArn":    srcOK,
		"LambdaConfig": map[string]any{
			"PostConfirmation": lambdaARN,
		},
	}, now)
	if srcAllow.Code != http.StatusOK {
		t.Fatalf("matching SourceArn status=%d body=%q", srcAllow.Code, srcAllow.Body.String())
	}
}

func mustInitiateMFASession(t *testing.T, handler http.Handler, clientID string, now time.Time) string {
	t.Helper()
	challenged := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "mfa-user",
			"PASSWORD": "Secret5!",
		},
	}, now)
	if challenged.Code != http.StatusOK {
		t.Fatalf("InitiateAuth for bad TOTP status=%d body=%q", challenged.Code, challenged.Body.String())
	}
	var challengeResp map[string]any
	_ = json.Unmarshal(challenged.Body.Bytes(), &challengeResp)
	session, _ := challengeResp["Session"].(string)
	if session == "" {
		t.Fatalf("missing session: %s", challenged.Body.String())
	}
	return session
}
