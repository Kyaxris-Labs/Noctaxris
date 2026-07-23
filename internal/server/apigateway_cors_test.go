package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPAPICORSPreflightAndUpdateApi(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cors-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cors-role"
	fnName := "cors-http-fn"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": fnName,
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", fnRec.Code, fnRec.Body.String())
	}
	mustAddLambdaServicePermission(t, handler, fnName, "apigateway.amazonaws.com", "apigw-cors", now)
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:" + fnName

	apiRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name":         "cors-api",
		"ProtocolType": "HTTP",
		"CorsConfiguration": map[string]any{
			"AllowOrigins": []any{"https://lab.example"},
			"AllowMethods": []any{"GET", "OPTIONS"},
			"AllowHeaders": []any{"authorization", "content-type"},
			"MaxAge":       600,
		},
	}, now)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("CreateApi status=%d body=%q", apiRec.Code, apiRec.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(apiRec.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["ApiId"].(string)
	if _, ok := apiResp["CorsConfiguration"].(map[string]any); !ok {
		t.Fatalf("CreateApi missing CorsConfiguration: %v", apiResp)
	}

	intRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateIntegration", "apigateway", map[string]any{
		"ApiId": apiID, "IntegrationType": "AWS_PROXY", "IntegrationUri": lambdaARN,
	}, now)
	if intRec.Code != http.StatusOK {
		t.Fatalf("CreateIntegration status=%d body=%q", intRec.Code, intRec.Body.String())
	}
	var intResp map[string]any
	_ = json.Unmarshal(intRec.Body.Bytes(), &intResp)
	integrationID, _ := intResp["IntegrationId"].(string)

	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /hello", "Target": "integrations/" + integrationID,
		"AuthorizationType": "NONE",
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	opt := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/hello", nil)
	opt.Header.Set("Origin", "https://lab.example")
	opt.Header.Set("Access-Control-Request-Method", "GET")
	opt.Header.Set("Access-Control-Request-Headers", "authorization")
	optRec := httptest.NewRecorder()
	handler.ServeHTTP(optRec, opt)
	if optRec.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d body=%q", optRec.Code, optRec.Body.String())
	}
	if got := optRec.Header().Get("Access-Control-Allow-Origin"); got != "https://lab.example" {
		t.Fatalf("ACAO=%q", got)
	}
	if got := optRec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "GET") {
		t.Fatalf("Allow-Methods=%q", got)
	}
	if got := optRec.Header().Get("Access-Control-Max-Age"); got != "600" {
		t.Fatalf("Max-Age=%q", got)
	}

	deny := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/hello", nil)
	deny.Header.Set("Origin", "https://evil.example")
	deny.Header.Set("Access-Control-Request-Method", "GET")
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, deny)
	if denyRec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin preflight status=%d want 403", denyRec.Code)
	}

	upd := mustJSONTarget(t, handler, "ApiGatewayV2.UpdateApi", "apigateway", map[string]any{
		"ApiId": apiID,
		"CorsConfiguration": map[string]any{
			"AllowOrigins": []any{"https://other.example"},
			"AllowMethods": []any{"GET", "POST", "OPTIONS"},
		},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateApi status=%d body=%q", upd.Code, upd.Body.String())
	}

	opt2 := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/hello", nil)
	opt2.Header.Set("Origin", "https://other.example")
	opt2.Header.Set("Access-Control-Request-Method", "POST")
	opt2Rec := httptest.NewRecorder()
	handler.ServeHTTP(opt2Rec, opt2)
	if opt2Rec.Code != http.StatusNoContent {
		t.Fatalf("updated preflight status=%d body=%q", opt2Rec.Code, opt2Rec.Body.String())
	}
	if got := opt2Rec.Header().Get("Access-Control-Allow-Origin"); got != "https://other.example" {
		t.Fatalf("updated ACAO=%q", got)
	}
}
