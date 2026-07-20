package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAPIGatewayHTTPAPINoneInvoke(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "apigw-lambda-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/apigw-lambda-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "apigw-hello",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", fnRec.Code, fnRec.Body.String())
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:apigw-hello"

	apiRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name": "lab-http", "ProtocolType": "HTTP",
	}, now)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("CreateApi status=%d body=%q", apiRec.Code, apiRec.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(apiRec.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["ApiId"].(string)

	intRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateIntegration", "apigateway", map[string]any{
		"ApiId": apiID, "IntegrationType": "AWS_PROXY", "IntegrationUri": lambdaARN,
	}, now)
	if intRec.Code != http.StatusOK {
		t.Fatalf("CreateIntegration status=%d body=%q", intRec.Code, intRec.Body.String())
	}
	var intResp map[string]any
	_ = json.Unmarshal(intRec.Body.Bytes(), &intResp)
	integrationID, _ := intResp["IntegrationId"].(string)

	routeRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /hello", "Target": "integrations/" + integrationID,
		"AuthorizationType": "NONE",
	}, now)
	if routeRec.Code != http.StatusOK {
		t.Fatalf("CreateRoute status=%d body=%q", routeRec.Code, routeRec.Body.String())
	}

	stageRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default", "AutoDeploy": true,
	}, now)
	if stageRec.Code != http.StatusOK {
		t.Fatalf("CreateStage status=%d body=%q", stageRec.Code, stageRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/hello", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("NONE invoke status=%d want 503 body=%q", rec.Code, rec.Body.String())
	}
}

func TestAPIGatewayIAMAuthorizerRejectsUnsigned(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "apigw-iam-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/apigw-iam-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "apigw-iam-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d", fnRec.Code)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:apigw-iam-fn"

	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "iam-api", lambdaARN, now)
	routeRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /secure", "Target": "integrations/" + integrationID,
		"AuthorizationType": "AWS_IAM",
	}, now)
	if routeRec.Code != http.StatusOK {
		t.Fatalf("CreateRoute status=%d body=%q", routeRec.Code, routeRec.Body.String())
	}
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/secure", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unsigned IAM route status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
}

func TestAPIGatewayIAMAuthorizerAllowsSigV4(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "apigw-iam2-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/apigw-iam2-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "apigw-iam2-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d", fnRec.Code)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:apigw-iam2-fn"

	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "iam2-api", lambdaARN, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /secure", "Target": "integrations/" + integrationID,
		"AuthorizationType": "AWS_IAM",
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/secure", nil)
	signHeader(t, req, nil, testAccessKey, testSecret, testRegion, "execute-api", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("signed IAM invoke status=%d want 503 body=%q", rec.Code, rec.Body.String())
	}
}

func TestAPIGatewayJWTAuthorizerWithCognito(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "apigw-jwt-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/apigw-jwt-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "apigw-jwt-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d", fnRec.Code)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:apigw-jwt-fn"

	poolID, clientID := mustCreateCognitoPoolClientUser(t, handler, "gw-pool", "gw-client", "dave", "Secret4!", now)
	issuer := store.CognitoIssuerURL("us-east-1", poolID)

	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "jwt-api", lambdaARN, now)
	authzRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateAuthorizer", "apigateway", map[string]any{
		"ApiId": apiID, "Name": "cognito-jwt", "AuthorizerType": "JWT",
		"IdentitySource": []any{"$request.header.Authorization"},
		"JwtConfiguration": map[string]any{
			"Issuer":   issuer,
			"Audience": []any{clientID},
		},
	}, now)
	if authzRec.Code != http.StatusOK {
		t.Fatalf("CreateAuthorizer status=%d body=%q", authzRec.Code, authzRec.Body.String())
	}
	var authzResp map[string]any
	_ = json.Unmarshal(authzRec.Body.Bytes(), &authzResp)
	authorizerID, _ := authzResp["AuthorizerId"].(string)

	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /jwt", "Target": "integrations/" + integrationID,
		"AuthorizationType": "JWT", "AuthorizerId": authorizerID,
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	// Missing Bearer -> 401
	miss := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/jwt", nil)
	missRec := httptest.NewRecorder()
	handler.ServeHTTP(missRec, miss)
	if missRec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d want 401", missRec.Code)
	}

	// Bad token -> 401
	bad := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/jwt", nil)
	bad.Header.Set("Authorization", "Bearer not.a.jwt")
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status=%d want 401", badRec.Code)
	}

	accessToken := mustCognitoAccessToken(t, handler, clientID, "dave", "Secret4!", now)
	okReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/jwt", nil)
	okReq.Header.Set("Authorization", "Bearer "+accessToken)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("valid JWT invoke status=%d want 503 body=%q", okRec.Code, okRec.Body.String())
	}
}

func TestAppSyncCognitoAuthRejectsMissingBearer(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	poolID, clientID := mustCreateCognitoPoolClientUser(t, handler, "appsync-pool", "appsync-client", "erin", "Secret5!", now)
	issuer := store.CognitoIssuerURL("us-east-1", poolID)

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "cognito-gql",
		"authenticationType": "AMAZON_COGNITO_USER_POOLS",
		"userPoolConfig": map[string]any{
			"userPoolId": poolID,
			"awsRegion":  "us-east-1",
			"clientId":   clientID,
			"issuer":     issuer,
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraphqlApi Cognito status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)

	body, _ := json.Marshal(map[string]any{"query": "{ hello }"})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/appsync/"+apiID+"/graphql", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without Bearer, got %d body=%q", rec.Code, rec.Body.String())
	}

	token := mustCognitoAccessToken(t, handler, clientID, "erin", "Secret5!", now)
	okReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/appsync/"+apiID+"/graphql", body)
	okReq.Header.Set("Content-Type", "application/json")
	okReq.Header.Set("Authorization", "Bearer "+token)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, okReq)
	// Resolver missing -> 400 with errors (auth passed)
	if okRec.Code == http.StatusUnauthorized || okRec.Code == http.StatusForbidden {
		t.Fatalf("valid Cognito token should pass auth, got %d body=%q", okRec.Code, okRec.Body.String())
	}
	if !strings.Contains(okRec.Body.String(), "errors") && !strings.Contains(okRec.Body.String(), "not found") {
		t.Fatalf("expected resolver error after auth, body=%q", okRec.Body.String())
	}
}

func mustCreateHTTPAPIWithIntegration(t *testing.T, handler http.Handler, name, lambdaARN string, now time.Time) (apiID, integrationID string) {
	t.Helper()
	apiRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name": name, "ProtocolType": "HTTP",
	}, now)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("CreateApi status=%d body=%q", apiRec.Code, apiRec.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(apiRec.Body.Bytes(), &apiResp)
	apiID, _ = apiResp["ApiId"].(string)
	intRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateIntegration", "apigateway", map[string]any{
		"ApiId": apiID, "IntegrationType": "AWS_PROXY", "IntegrationUri": lambdaARN,
	}, now)
	if intRec.Code != http.StatusOK {
		t.Fatalf("CreateIntegration status=%d body=%q", intRec.Code, intRec.Body.String())
	}
	var intResp map[string]any
	_ = json.Unmarshal(intRec.Body.Bytes(), &intResp)
	integrationID, _ = intResp["IntegrationId"].(string)
	return apiID, integrationID
}

func mustCreateCognitoPoolClientUser(t *testing.T, handler http.Handler, poolName, clientName, user, pass string, now time.Time) (poolID, clientID string) {
	t.Helper()
	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": poolName,
	}, now)
	if createPool.Code != http.StatusOK {
		t.Fatalf("CreateUserPool status=%d body=%q", createPool.Code, createPool.Body.String())
	}
	var poolResp map[string]any
	_ = json.Unmarshal(createPool.Body.Bytes(), &poolResp)
	up, _ := poolResp["UserPool"].(map[string]any)
	poolID, _ = up["Id"].(string)

	createClient := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPoolClient", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "ClientName": clientName,
	}, now)
	if createClient.Code != http.StatusOK {
		t.Fatalf("CreateUserPoolClient status=%d body=%q", createClient.Code, createClient.Body.String())
	}
	var clientResp map[string]any
	_ = json.Unmarshal(createClient.Body.Bytes(), &clientResp)
	upc, _ := clientResp["UserPoolClient"].(map[string]any)
	clientID, _ = upc["ClientId"].(string)

	adminCreate := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminCreateUser", "cognito-idp", map[string]any{
		"UserPoolId": poolID, "Username": user, "TemporaryPassword": pass,
	}, now)
	if adminCreate.Code != http.StatusOK {
		t.Fatalf("AdminCreateUser status=%d body=%q", adminCreate.Code, adminCreate.Body.String())
	}
	return poolID, clientID
}

func mustCognitoAccessToken(t *testing.T, handler http.Handler, clientID, user, pass string, now time.Time) string {
	t.Helper()
	auth := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.InitiateAuth", "cognito-idp", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": user,
			"PASSWORD": pass,
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
		t.Fatalf("missing AccessToken: %s", auth.Body.String())
	}
	return accessToken
}

const apigatewayTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"apigateway.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

func TestAPIGatewayCreateIntegrationCredentialsArnPassRole(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "apigw-cred-ok", apigatewayTrustOK, now)
	mustCreateIAMRole(t, handler, "apigw-cred-bad", lambdaTrustOK, now)
	okARN := "arn:aws:iam::" + testAccountID + ":role/apigw-cred-ok"
	badARN := "arn:aws:iam::" + testAccountID + ":role/apigw-cred-bad"
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:unused"

	apiRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name": "cred-api", "ProtocolType": "HTTP",
	}, now)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("CreateApi status=%d body=%q", apiRec.Code, apiRec.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(apiRec.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["ApiId"].(string)

	deny := mustJSONTarget(t, handler, "ApiGatewayV2.CreateIntegration", "apigateway", map[string]any{
		"ApiId": apiID, "IntegrationType": "AWS_PROXY", "IntegrationUri": lambdaARN,
		"CredentialsArn": badARN,
	}, now)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("bad CredentialsArn status=%d want 403 body=%q", deny.Code, deny.Body.String())
	}

	allow := mustJSONTarget(t, handler, "ApiGatewayV2.CreateIntegration", "apigateway", map[string]any{
		"ApiId": apiID, "IntegrationType": "AWS_PROXY", "IntegrationUri": lambdaARN,
		"CredentialsArn": okARN,
	}, now)
	if allow.Code != http.StatusOK {
		t.Fatalf("ok CredentialsArn status=%d body=%q", allow.Code, allow.Body.String())
	}
	if !strings.Contains(allow.Body.String(), okARN) {
		t.Fatalf("response missing CredentialsArn: %s", allow.Body.String())
	}
}
