package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIGatewayWebSocketManagementAndPostToConnection(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "apigw-ws-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/apigw-ws-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "apigw-ws-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", fnRec.Code, fnRec.Body.String())
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:apigw-ws-fn"
	mustAddLambdaServicePermission(t, handler, "apigw-ws-fn", "apigateway.amazonaws.com", "apigw-ws", now)

	apiRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name": "lab-ws", "ProtocolType": "WEBSOCKET",
	}, now)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("CreateApi status=%d body=%q", apiRec.Code, apiRec.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(apiRec.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["apiId"].(string)
	endpoint, _ := apiResp["apiEndpoint"].(string)
	if endpoint == "" || apiID == "" {
		t.Fatalf("CreateApi response=%v", apiResp)
	}

	intRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateIntegration", "apigateway", map[string]any{
		"ApiId": apiID, "IntegrationType": "AWS_PROXY", "IntegrationUri": lambdaARN,
	}, now)
	if intRec.Code != http.StatusOK {
		t.Fatalf("CreateIntegration status=%d body=%q", intRec.Code, intRec.Body.String())
	}
	var intResp map[string]any
	_ = json.Unmarshal(intRec.Body.Bytes(), &intResp)
	integrationID, _ := intResp["integrationId"].(string)

	for _, rk := range []string{"$connect", "$disconnect", "$default"} {
		routeRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
			"ApiId": apiID, "RouteKey": rk, "Target": "integrations/" + integrationID,
			"AuthorizationType": "NONE",
		}, now)
		if routeRec.Code != http.StatusOK {
			t.Fatalf("CreateRoute %s status=%d body=%q", rk, routeRec.Code, routeRec.Body.String())
		}
	}
	stageRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default", "AutoDeploy": true,
	}, now)
	if stageRec.Code != http.StatusOK {
		t.Fatalf("CreateStage status=%d body=%q", stageRec.Code, stageRec.Body.String())
	}

	// Lab $connect without nested compute → 503; connection rolled back.
	connectReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/ws-api/"+apiID+"/$default/$connect", nil)
	connectRec := httptest.NewRecorder()
	handler.ServeHTTP(connectRec, connectReq)
	if connectRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("$connect status=%d want 503 body=%q", connectRec.Code, connectRec.Body.String())
	}

	conn := st.CreateAPIGatewayWebSocketConnection(testAccountID, apiID, "$default")
	postBody := []byte("hello-ws")
	postReq := mustNewRequest(t, http.MethodPost,
		"http://127.0.0.1:4566/execute-api/"+apiID+"/$default/@connections/"+conn.ConnectionID, postBody)
	signHeader(t, postReq, postBody, testAccessKey, testSecret, testRegion, "execute-api", now)
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("PostToConnection status=%d body=%q", postRec.Code, postRec.Body.String())
	}
	got, err := st.GetAPIGatewayWebSocketConnection(testAccountID, apiID, "$default", conn.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 || got.Messages[0] != "hello-ws" {
		t.Fatalf("messages=%v", got.Messages)
	}

	getReq := mustNewRequest(t, http.MethodGet,
		"http://127.0.0.1:4566/execute-api/"+apiID+"/$default/@connections/"+conn.ConnectionID, nil)
	signHeader(t, getReq, nil, testAccessKey, testSecret, testRegion, "execute-api", now)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetConnection status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var getResp map[string]any
	_ = json.Unmarshal(getRec.Body.Bytes(), &getResp)
	msgs, _ := getResp["Messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("GetConnection messages=%v", getResp)
	}
}
