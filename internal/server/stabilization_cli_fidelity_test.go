package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIGatewayV2RESTNotStolenByRegistry(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	body := []byte(`{"Name":"cli-http","ProtocolType":"HTTP"}`)
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/v2/apis", body)
	req.Header.Set("Content-Type", "application/json")
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "apigateway", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateApi REST status=%d body=%q (must not be registry 401)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "UNAUTHORIZED") || rec.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("registry stole /v2/apis: status=%d www=%q body=%q", rec.Code, rec.Header().Get("WWW-Authenticate"), rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	apiID, _ := out["ApiId"].(string)
	if apiID == "" {
		t.Fatalf("missing ApiId: %s", rec.Body.String())
	}

	// Registry root still 401 without token.
	regReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/v2/", nil)
	regRec := httptest.NewRecorder()
	handler.ServeHTTP(regRec, regReq)
	if regRec.Code != http.StatusUnauthorized {
		t.Fatalf("registry /v2/ status=%d want 401", regRec.Code)
	}
}

func TestAPIGatewayV2GetIntegrationsRoutesAuthorizers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "apigw-list-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/apigw-list-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "apigw-list-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", fnRec.Code, fnRec.Body.String())
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:apigw-list-fn"

	apiRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name": "list-api", "ProtocolType": "HTTP",
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
		"ApiId": apiID, "RouteKey": "GET /listed", "Target": "integrations/" + integrationID,
		"AuthorizationType": "NONE",
	}, now)
	if routeRec.Code != http.StatusOK {
		t.Fatalf("CreateRoute status=%d body=%q", routeRec.Code, routeRec.Body.String())
	}

	listInt := mustJSONTarget(t, handler, "ApiGatewayV2.GetIntegrations", "apigateway", map[string]any{
		"ApiId": apiID,
	}, now)
	if listInt.Code != http.StatusOK {
		t.Fatalf("GetIntegrations status=%d body=%q", listInt.Code, listInt.Body.String())
	}
	var intList map[string]any
	_ = json.Unmarshal(listInt.Body.Bytes(), &intList)
	if items, _ := intList["Items"].([]any); len(items) != 1 {
		t.Fatalf("GetIntegrations Items=%v", intList["Items"])
	}

	listRoutes := mustJSONTarget(t, handler, "ApiGatewayV2.GetRoutes", "apigateway", map[string]any{
		"ApiId": apiID,
	}, now)
	if listRoutes.Code != http.StatusOK {
		t.Fatalf("GetRoutes status=%d body=%q", listRoutes.Code, listRoutes.Body.String())
	}

	// REST list path used by aws apigatewayv2 get-integrations
	restBody := []byte(`{}`)
	restReq := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/v2/apis/"+apiID+"/integrations", restBody)
	signHeader(t, restReq, restBody, testAccessKey, testSecret, testRegion, "apigateway", now)
	restRec := httptest.NewRecorder()
	handler.ServeHTTP(restRec, restReq)
	if restRec.Code != http.StatusOK {
		t.Fatalf("GetIntegrations REST status=%d body=%q", restRec.Code, restRec.Body.String())
	}
}

func TestLambdaAddPermissionAndFunctionURLREST(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-rest-url-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-rest-url-role"
	fnName := "rest-url-fn"
	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": fnName,
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	permBody := []byte(`{"Action":"lambda:InvokeFunction","Principal":"apigateway.amazonaws.com","StatementId":"rest-perm"}`)
	permReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/2015-03-31/functions/"+fnName+"/policy", permBody)
	permReq.Header.Set("Content-Type", "application/json")
	signHeader(t, permReq, permBody, testAccessKey, testSecret, testRegion, "lambda", now)
	permRec := httptest.NewRecorder()
	handler.ServeHTTP(permRec, permReq)
	if permRec.Code != http.StatusOK && permRec.Code != http.StatusCreated {
		t.Fatalf("AddPermission REST status=%d body=%q", permRec.Code, permRec.Body.String())
	}

	urlBody := []byte(`{"AuthType":"AWS_IAM"}`)
	urlReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/2021-10-31/functions/"+fnName+"/url", urlBody)
	urlReq.Header.Set("Content-Type", "application/json")
	signHeader(t, urlReq, urlBody, testAccessKey, testSecret, testRegion, "lambda", now)
	urlRec := httptest.NewRecorder()
	handler.ServeHTTP(urlRec, urlReq)
	if urlRec.Code != http.StatusOK && urlRec.Code != http.StatusCreated {
		t.Fatalf("CreateFunctionUrlConfig REST status=%d body=%q", urlRec.Code, urlRec.Body.String())
	}
	var cfg map[string]any
	if err := json.Unmarshal(urlRec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg["FunctionUrl"].(string); !ok {
		t.Fatalf("missing FunctionUrl: %s", urlRec.Body.String())
	}
}

func TestCognitoInitiateAuthWithoutSigV4(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createPool := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPool", "cognito-idp", map[string]any{
		"PoolName": "cli-auth-pool",
	}, now)
	if createPool.Code != http.StatusOK {
		t.Fatalf("CreateUserPool status=%d body=%q", createPool.Code, createPool.Body.String())
	}
	var poolResp map[string]any
	_ = json.Unmarshal(createPool.Body.Bytes(), &poolResp)
	pool, _ := poolResp["UserPool"].(map[string]any)
	poolID, _ := pool["Id"].(string)

	createClient := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.CreateUserPoolClient", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
		"ClientName": "cli-client",
	}, now)
	if createClient.Code != http.StatusOK {
		t.Fatalf("CreateUserPoolClient status=%d body=%q", createClient.Code, createClient.Body.String())
	}
	var clientResp map[string]any
	_ = json.Unmarshal(createClient.Body.Bytes(), &clientResp)
	client, _ := clientResp["UserPoolClient"].(map[string]any)
	clientID, _ := client["ClientId"].(string)

	adminCreate := mustJSONTarget(t, handler, "AWSCognitoIdentityProviderService.AdminCreateUser", "cognito-idp", map[string]any{
		"UserPoolId":        poolID,
		"Username":          "cli-user",
		"TemporaryPassword": "Secret3!",
	}, now)
	if adminCreate.Code != http.StatusOK {
		t.Fatalf("AdminCreateUser status=%d body=%q", adminCreate.Code, adminCreate.Body.String())
	}

	// Unsigned InitiateAuth (AWS CLI shape): X-Amz-Target, no Authorization.
	raw, err := json.Marshal(map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "cli-user",
			"PASSWORD": "Secret3!",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.InitiateAuth")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("InitiateAuth without SigV4 must not return MissingAuthenticationToken: %s", rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("InitiateAuth unsigned status=%d body=%q", rec.Code, rec.Body.String())
	}
	var authResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &authResp); err != nil {
		t.Fatal(err)
	}
	result, _ := authResp["AuthenticationResult"].(map[string]any)
	if result["IdToken"] == nil || result["AccessToken"] == nil {
		t.Fatalf("missing tokens: %s", rec.Body.String())
	}
}
