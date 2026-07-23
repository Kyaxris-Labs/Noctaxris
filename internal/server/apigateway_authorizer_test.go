package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAPIGatewayLambdaAuthorizerDenyShortCircuit(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "authz-deny-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/authz-deny-role"
	for _, name := range []string{"authz-deny-fn", "integration-deny-fn"} {
		fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
			"FunctionName": name,
			"Runtime":      "python3.12",
			"Role":         roleARN,
			"Handler":      "app.handler",
			"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		}, now)
		if fnRec.Code != http.StatusOK {
			t.Fatalf("CreateFunction %s status=%d", name, fnRec.Code)
		}
		mustAddLambdaServicePermission(t, handler, name, "apigateway.amazonaws.com", "perm-"+name, now)
	}
	authzARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:authz-deny-fn"
	integrationARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:integration-deny-fn"

	var integrationInvokes atomic.Int32
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, _ string) ([]byte, error) {
		if name == "integration-deny-fn" {
			integrationInvokes.Add(1)
			return []byte(`{"statusCode":200,"body":"ok"}`), nil
		}
		return []byte(`{"isAuthorized":false}`), nil
	})

	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "authz-deny-api", integrationARN, now)
	authzRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateAuthorizer", "apigateway", map[string]any{
		"ApiId": apiID, "Name": "lambda-authz", "AuthorizerType": "REQUEST",
		"AuthorizerUri": authzARN, "AuthorizerPayloadFormatVersion": "2.0",
		"EnableSimpleResponses": true,
		"IdentitySource":        []any{"$request.header.Authorization"},
	}, now)
	if authzRec.Code != http.StatusOK {
		t.Fatalf("CreateAuthorizer status=%d body=%q", authzRec.Code, authzRec.Body.String())
	}
	var authzResp map[string]any
	_ = json.Unmarshal(authzRec.Body.Bytes(), &authzResp)
	authorizerID, _ := authzResp["AuthorizerId"].(string)

	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /secure", "Target": "integrations/" + integrationID,
		"AuthorizationType": "CUSTOM", "AuthorizerId": authorizerID,
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/secure", nil)
	req.Header.Set("Authorization", "nope")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("deny status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if integrationInvokes.Load() != 0 {
		t.Fatalf("integration invoked %d times on Deny", integrationInvokes.Load())
	}
}

func TestAPIGatewayLambdaAuthorizerAllowReachesIntegration(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "authz-allow-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/authz-allow-role"
	for _, name := range []string{"authz-allow-fn", "integration-allow-fn"} {
		fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
			"FunctionName": name,
			"Runtime":      "python3.12",
			"Role":         roleARN,
			"Handler":      "app.handler",
			"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		}, now)
		if fnRec.Code != http.StatusOK {
			t.Fatalf("CreateFunction %s status=%d", name, fnRec.Code)
		}
		mustAddLambdaServicePermission(t, handler, name, "apigateway.amazonaws.com", "perm-"+name, now)
	}
	authzARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:authz-allow-fn"
	integrationARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:integration-allow-fn"

	var integrationInvokes atomic.Int32
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		if name == "authz-allow-fn" {
			if !strings.Contains(eventJSON, `"type":"REQUEST"`) && !strings.Contains(eventJSON, `"type": "REQUEST"`) {
				t.Fatalf("authorizer event missing type REQUEST: %s", eventJSON)
			}
			return []byte(`{"isAuthorized":true}`), nil
		}
		integrationInvokes.Add(1)
		return []byte(`{"statusCode":200,"body":"{\"ok\":true}","headers":{"Set-Cookie":"x=1","X-Lab":"1"}}`), nil
	})

	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "authz-allow-api", integrationARN, now)
	authzRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateAuthorizer", "apigateway", map[string]any{
		"ApiId": apiID, "Name": "lambda-authz", "AuthorizerType": "REQUEST",
		"AuthorizerUri": authzARN, "AuthorizerPayloadFormatVersion": "2.0",
		"IdentitySource": []any{"$request.header.Authorization"},
	}, now)
	if authzRec.Code != http.StatusOK {
		t.Fatalf("CreateAuthorizer status=%d body=%q", authzRec.Code, authzRec.Body.String())
	}
	var authzResp map[string]any
	_ = json.Unmarshal(authzRec.Body.Bytes(), &authzResp)
	authorizerID, _ := authzResp["AuthorizerId"].(string)

	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /secure", "Target": "integrations/" + integrationID,
		"AuthorizationType": "CUSTOM", "AuthorizerId": authorizerID,
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/secure", nil)
	req.Header.Set("Authorization", "secretToken")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("allow status=%d want 200 body=%q", rec.Code, rec.Body.String())
	}
	if integrationInvokes.Load() != 1 {
		t.Fatalf("integration invokes=%d want 1", integrationInvokes.Load())
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("Set-Cookie should be stripped by default, got %q", rec.Header().Get("Set-Cookie"))
	}
	if rec.Header().Get("X-Lab") != "1" {
		t.Fatalf("X-Lab=%q", rec.Header().Get("X-Lab"))
	}
}

func TestAPIGatewayLambdaAuthorizerRequiresResourcePolicy(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "authz-pol-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/authz-pol-role"
	for _, name := range []string{"authz-pol-fn", "integration-pol-fn"} {
		fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
			"FunctionName": name,
			"Runtime":      "python3.12",
			"Role":         roleARN,
			"Handler":      "app.handler",
			"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		}, now)
		if fnRec.Code != http.StatusOK {
			t.Fatalf("CreateFunction %s status=%d", name, fnRec.Code)
		}
	}
	// Integration permission only — authorizer missing resource policy.
	mustAddLambdaServicePermission(t, handler, "integration-pol-fn", "apigateway.amazonaws.com", "int-pol", now)
	authzARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:authz-pol-fn"
	integrationARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:integration-pol-fn"

	var authzInvokes atomic.Int32
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, _ string) ([]byte, error) {
		if name == "authz-pol-fn" {
			authzInvokes.Add(1)
		}
		return []byte(`{"isAuthorized":true}`), nil
	})

	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "authz-pol-api", integrationARN, now)
	authzRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateAuthorizer", "apigateway", map[string]any{
		"ApiId": apiID, "Name": "lambda-authz", "AuthorizerType": "REQUEST",
		"AuthorizerUri": authzARN, "AuthorizerPayloadFormatVersion": "2.0",
		"IdentitySource": []any{"$request.header.Authorization"},
	}, now)
	if authzRec.Code != http.StatusOK {
		t.Fatalf("CreateAuthorizer status=%d body=%q", authzRec.Code, authzRec.Body.String())
	}
	var authzResp map[string]any
	_ = json.Unmarshal(authzRec.Body.Bytes(), &authzResp)
	authorizerID, _ := authzResp["AuthorizerId"].(string)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /secure", "Target": "integrations/" + integrationID,
		"AuthorizationType": "CUSTOM", "AuthorizerId": authorizerID,
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	deny := httptest.NewRecorder()
	handler.ServeHTTP(deny, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/secure", nil))
	if deny.Code != http.StatusForbidden {
		t.Fatalf("missing authorizer policy status=%d want 403 body=%q", deny.Code, deny.Body.String())
	}
	if authzInvokes.Load() != 0 {
		t.Fatalf("authorizer should not invoke without resource policy")
	}
}

func TestAPIGatewayCreateAuthorizerRejectsTOKEN(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	apiRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name": "tok-api", "ProtocolType": "HTTP",
	}, now)
	var apiResp map[string]any
	_ = json.Unmarshal(apiRec.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["ApiId"].(string)
	bad := mustJSONTarget(t, handler, "ApiGatewayV2.CreateAuthorizer", "apigateway", map[string]any{
		"ApiId": apiID, "Name": "tok", "AuthorizerType": "TOKEN",
		"AuthorizerUri": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:x",
	}, now)
	if bad.Code == http.StatusOK {
		t.Fatalf("TOKEN should be rejected, body=%q", bad.Body.String())
	}
}

func TestAPIGatewayLambdaAuthorizerNestedDeny(t *testing.T) {
	if os.Getenv("NOCTAXRIS_NESTED") != "1" {
		t.Skip("opt-in nested: set NOCTAXRIS_NESTED=1 with DinD; unit Deny short-circuit covered above")
	}
	t.Skip("operator nested smoke: authorizer Lambda returning isAuthorized:false")
}

func TestParseHTTPAPIAuthorizerResponseShapes(t *testing.T) {
	// Exercise exported behavior via Allow path IAM policy response through hook.
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "authz-iam-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/authz-iam-role"
	for _, name := range []string{"authz-iam-fn", "integration-iam-fn"} {
		fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
			"FunctionName": name,
			"Runtime":      "python3.12",
			"Role":         roleARN,
			"Handler":      "app.handler",
			"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		}, now)
		if fnRec.Code != http.StatusOK {
			t.Fatalf("CreateFunction %s status=%d", name, fnRec.Code)
		}
		mustAddLambdaServicePermission(t, handler, name, "apigateway.amazonaws.com", "perm-"+name, now)
	}
	authzARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:authz-iam-fn"
	integrationARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:integration-iam-fn"
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, _ string) ([]byte, error) {
		if name == "authz-iam-fn" {
			return []byte(`{"principalId":"u","policyDocument":{"Version":"2012-10-17","Statement":[{"Action":"execute-api:Invoke","Effect":"Allow","Resource":"*"}]}}`), nil
		}
		return []byte(`{"statusCode":200,"body":"ok"}`), nil
	})
	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "authz-iam-api", integrationARN, now)
	authzRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateAuthorizer", "apigateway", map[string]any{
		"ApiId": apiID, "Name": "lambda-authz", "AuthorizerType": "REQUEST",
		"AuthorizerUri": authzARN, "AuthorizerPayloadFormatVersion": "2.0",
		"EnableSimpleResponses": false,
		"IdentitySource":        []any{"$request.header.Authorization"},
	}, now)
	var authzResp map[string]any
	_ = json.Unmarshal(authzRec.Body.Bytes(), &authzResp)
	authorizerID, _ := authzResp["AuthorizerId"].(string)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /secure", "Target": "integrations/" + integrationID,
		"AuthorizationType": "CUSTOM", "AuthorizerId": authorizerID,
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/secure", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("IAM policy Allow status=%d body=%q", rec.Code, rec.Body.String())
	}
}
