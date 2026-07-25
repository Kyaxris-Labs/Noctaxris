package server_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCognitoPostConfirmationTriggerInvoke(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	var mu sync.Mutex
	var invokedName, invokedEvent string
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		invokedName = name
		invokedEvent = eventJSON
		return []byte(`{}`), nil
	})

	mustCreateIAMRole(t, handler, "cognito-trig-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cognito-trig-role"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "post-confirm-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createFn.Code, createFn.Body.String())
	}
	var fnOut map[string]any
	_ = json.Unmarshal(createFn.Body.Bytes(), &fnOut)
	fnARN, _ := fnOut["FunctionArn"].(string)

	mustCreateIAMRole(t, handler, "cognito-pool-role",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"cognito-idp.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	poolRole := "arn:aws:iam::" + testAccountID + ":role/cognito-pool-role"

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "invoke-pool",
		"RoleArn":  poolRole,
		"LambdaConfig": map[string]any{
			"PostConfirmation": fnARN,
		},
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
		"ClientName": "app",
	}, now)
	if createClient.Code != http.StatusOK {
		t.Fatalf("CreateUserPoolClient status=%d body=%q", createClient.Code, createClient.Body.String())
	}
	var clientResp map[string]any
	_ = json.Unmarshal(createClient.Body.Bytes(), &clientResp)
	upc, _ := clientResp["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	signUp := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.SignUp", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"Username": "dave",
		"Password": "Secret9!",
	}, now)
	if signUp.Code != http.StatusOK {
		t.Fatalf("SignUp status=%d body=%q", signUp.Code, signUp.Body.String())
	}
	code, err := st.PeekCognitoConfirmationCode(testAccountID, poolID, "dave", store.CognitoConfirmPurposeSignUp)
	if err != nil {
		t.Fatal(err)
	}

	confirm := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.ConfirmSignUp", "cognito-idp", map[string]any{
		"ClientId":         clientID,
		"Username":         "dave",
		"ConfirmationCode": code,
	}, now)
	if confirm.Code != http.StatusOK {
		t.Fatalf("ConfirmSignUp status=%d body=%q", confirm.Code, confirm.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if invokedName != "post-confirm-fn" {
		t.Fatalf("invoked=%q", invokedName)
	}
	if !strings.Contains(invokedEvent, "PostConfirmation_ConfirmSignUp") {
		t.Fatalf("event=%s", invokedEvent)
	}
}

func TestCognitoPostConfirmationMissingFunctionFailClosed(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cognito-pool-role2",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"cognito-idp.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	poolRole := "arn:aws:iam::" + testAccountID + ":role/cognito-pool-role2"
	missingARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:does-not-exist"

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "fail-pool",
		"RoleArn":  poolRole,
		"LambdaConfig": map[string]any{
			"PostConfirmation": missingARN,
		},
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
		"ClientName": "app",
	}, now)
	var clientResp map[string]any
	_ = json.Unmarshal(createClient.Body.Bytes(), &clientResp)
	upc, _ := clientResp["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	signUp := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.SignUp", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"Username": "erin",
		"Password": "Secret9!",
	}, now)
	if signUp.Code != http.StatusOK {
		t.Fatalf("SignUp status=%d body=%q", signUp.Code, signUp.Body.String())
	}
	code, err := st.PeekCognitoConfirmationCode(testAccountID, poolID, "erin", store.CognitoConfirmPurposeSignUp)
	if err != nil {
		t.Fatal(err)
	}

	confirm := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.ConfirmSignUp", "cognito-idp", map[string]any{
		"ClientId":         clientID,
		"Username":         "erin",
		"ConfirmationCode": code,
	}, now)
	if confirm.Code != http.StatusBadRequest {
		t.Fatalf("ConfirmSignUp status=%d want 400 body=%q", confirm.Code, confirm.Body.String())
	}
	if !strings.Contains(confirm.Body.String(), "UnexpectedLambdaException") {
		t.Fatalf("body=%q", confirm.Body.String())
	}
	if !strings.Contains(confirm.Body.String(), "Configured Lambda trigger failed.") {
		t.Fatalf("expected opaque trigger message in %q", confirm.Body.String())
	}
	if strings.Contains(confirm.Body.String(), "does-not-exist") || strings.Contains(confirm.Body.String(), "no such") {
		t.Fatalf("client error leaked trigger detail: %q", confirm.Body.String())
	}
}

func TestCognitoPreTokenGenerationTriggerInvoke(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	var sources []string
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		if name != "pre-token-fn" {
			return nil, errors.New("wrong function")
		}
		var evt map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &evt)
		sources = append(sources, evt["triggerSource"].(string))
		resp, _ := evt["response"].(map[string]any)
		if resp == nil {
			resp = map[string]any{}
			evt["response"] = resp
		}
		resp["claimsOverrideDetails"] = map[string]any{
			"claimsToAddOrOverride": map[string]any{"http_claim": "1"},
			"claimsToSuppress":      []any{"email_verified"},
		}
		b, _ := json.Marshal(evt)
		return b, nil
	})

	mustCreateIAMRole(t, handler, "cognito-trig-role3", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cognito-trig-role3"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "pre-token-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	var fnOut map[string]any
	_ = json.Unmarshal(createFn.Body.Bytes(), &fnOut)
	fnARN, _ := fnOut["FunctionArn"].(string)

	mustCreateIAMRole(t, handler, "cognito-pool-role3",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"cognito-idp.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	poolRole := "arn:aws:iam::" + testAccountID + ":role/cognito-pool-role3"

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "pretoken-http",
		"RoleArn":  poolRole,
		"LambdaConfig": map[string]any{
			"PreTokenGeneration": fnARN,
		},
	}, now)
	var poolResp map[string]any
	_ = json.Unmarshal(createPool.Body.Bytes(), &poolResp)
	up, _ := poolResp["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)

	createClient := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPoolClient", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"ClientName": "app",
	}, now)
	var clientResp map[string]any
	_ = json.Unmarshal(createClient.Body.Bytes(), &clientResp)
	upc, _ := clientResp["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	adminCreate := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminCreateUser", "cognito-idp", map[string]any{
		"UserPoolId":        poolID,
		"Username":          "frank",
		"TemporaryPassword": "Secret8!",
	}, now)
	if adminCreate.Code != http.StatusOK {
		t.Fatalf("AdminCreateUser status=%d body=%q", adminCreate.Code, adminCreate.Body.String())
	}

	auth := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "frank",
			"PASSWORD": "Secret8!",
		},
	}, now)
	if auth.Code != http.StatusOK {
		t.Fatalf("InitiateAuth status=%d body=%q", auth.Code, auth.Body.String())
	}
	if len(sources) != 1 || sources[0] != "TokenGeneration_Authentication" {
		t.Fatalf("sources=%v", sources)
	}
	var authOut map[string]any
	_ = json.Unmarshal(auth.Body.Bytes(), &authOut)
	ar, _ := authOut["AuthenticationResult"].(map[string]any)
	idToken, _ := ar["IdToken"].(string)
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		t.Fatalf("bad id token")
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["http_claim"] != "1" {
		t.Fatalf("claims=%v", claims)
	}
	if _, ok := claims["email_verified"]; ok {
		t.Fatalf("email_verified should be suppressed")
	}
}

func TestCognitoCustomMessageTriggerInvoke(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	var invokedName, invokedEvent string
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		invokedName = name
		invokedEvent = eventJSON
		return []byte(eventJSON), nil
	})

	mustCreateIAMRole(t, handler, "cognito-trig-role-cm", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cognito-trig-role-cm"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "custom-msg-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	var fnOut map[string]any
	_ = json.Unmarshal(createFn.Body.Bytes(), &fnOut)
	fnARN, _ := fnOut["FunctionArn"].(string)

	mustCreateIAMRole(t, handler, "cognito-pool-role-cm",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"cognito-idp.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	poolRole := "arn:aws:iam::" + testAccountID + ":role/cognito-pool-role-cm"

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "custom-msg-http",
		"RoleArn":  poolRole,
		"LambdaConfig": map[string]any{
			"CustomMessage": fnARN,
		},
	}, now)
	var poolResp map[string]any
	_ = json.Unmarshal(createPool.Body.Bytes(), &poolResp)
	up, _ := poolResp["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)

	createClient := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPoolClient", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"ClientName": "app",
	}, now)
	var clientResp map[string]any
	_ = json.Unmarshal(createClient.Body.Bytes(), &clientResp)
	upc, _ := clientResp["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	signUp := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.SignUp", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"Username": "cm-user",
		"Password": "Secret9!",
	}, now)
	if signUp.Code != http.StatusOK {
		t.Fatalf("SignUp status=%d body=%q", signUp.Code, signUp.Body.String())
	}
	if invokedName != "custom-msg-fn" {
		t.Fatalf("invoked=%q", invokedName)
	}
	if !strings.Contains(invokedEvent, "CustomMessage_SignUp") {
		t.Fatalf("event=%s", invokedEvent)
	}
}

func TestCognitoUserMigrationTriggerInvoke(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		if name != "migrate-fn" {
			return nil, errors.New("wrong function")
		}
		var evt map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &evt)
		evt["response"] = map[string]any{
			"userAttributes": map[string]any{
				"email": "mig@example.com",
			},
			"finalUserStatus": "CONFIRMED",
		}
		b, _ := json.Marshal(evt)
		return b, nil
	})

	mustCreateIAMRole(t, handler, "cognito-trig-role-mig", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cognito-trig-role-mig"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "migrate-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	var fnOut map[string]any
	_ = json.Unmarshal(createFn.Body.Bytes(), &fnOut)
	fnARN, _ := fnOut["FunctionArn"].(string)

	mustCreateIAMRole(t, handler, "cognito-pool-role-mig",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"cognito-idp.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	poolRole := "arn:aws:iam::" + testAccountID + ":role/cognito-pool-role-mig"

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "migrate-http",
		"RoleArn":  poolRole,
		"LambdaConfig": map[string]any{
			"UserMigration": fnARN,
		},
	}, now)
	var poolResp map[string]any
	_ = json.Unmarshal(createPool.Body.Bytes(), &poolResp)
	up, _ := poolResp["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)

	createClient := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPoolClient", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"ClientName": "app",
	}, now)
	var clientResp map[string]any
	_ = json.Unmarshal(createClient.Body.Bytes(), &clientResp)
	upc, _ := clientResp["UserPoolClient"].(map[string]any)
	clientID, _ := upc["ClientId"].(string)

	auth := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "migrated-http",
			"PASSWORD": "Migrated9!",
		},
	}, now)
	if auth.Code != http.StatusOK {
		t.Fatalf("InitiateAuth status=%d body=%q", auth.Code, auth.Body.String())
	}
	var authOut map[string]any
	_ = json.Unmarshal(auth.Body.Bytes(), &authOut)
	ar, _ := authOut["AuthenticationResult"].(map[string]any)
	if ar == nil || ar["AccessToken"] == nil {
		t.Fatalf("body=%s", auth.Body.String())
	}
}
