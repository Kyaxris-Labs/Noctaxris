package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustCreateRESTMockAPI(
	t *testing.T, handler http.Handler, name, pathPart string, now time.Time,
) (apiID, resourceID string) {
	t.Helper()
	create := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis", map[string]any{
		"name": name,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateRestApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	apiID, _ = apiResp["id"].(string)
	rootID, _ := apiResp["rootResourceId"].(string)
	resRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/resources/"+rootID, map[string]any{
		"pathPart": pathPart,
	}, now)
	if resRec.Code != http.StatusOK {
		t.Fatalf("CreateResource status=%d body=%q", resRec.Code, resRec.Body.String())
	}
	var resResp map[string]any
	_ = json.Unmarshal(resRec.Body.Bytes(), &resResp)
	resourceID, _ = resResp["id"].(string)
	return apiID, resourceID
}

func TestAPIGatewayRESTTOKENAuthorizerDenyShortCircuit(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "rest-authz-deny-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/rest-authz-deny-role"
	for _, name := range []string{"rest-authz-deny-fn", "rest-int-deny-fn"} {
		fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
			"FunctionName": name, "Runtime": "python3.12", "Role": roleARN,
			"Handler": "app.handler", "Code": map[string]any{"ZipFile": testLambdaZipB64(t)},
		}, now)
		if fnRec.Code != http.StatusOK {
			t.Fatalf("CreateFunction %s status=%d", name, fnRec.Code)
		}
		mustAddLambdaServicePermission(t, handler, name, "apigateway.amazonaws.com", "perm-"+name, now)
	}
	authzARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:rest-authz-deny-fn"

	var integrationInvokes atomic.Int32
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, _ string) ([]byte, error) {
		if name == "rest-int-deny-fn" {
			integrationInvokes.Add(1)
			return []byte(`{"statusCode":200,"body":"ok"}`), nil
		}
		return []byte(`{"policyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"execute-api:Invoke","Resource":"*"}]}}`), nil
	})

	apiID, resourceID := mustCreateRESTMockAPI(t, handler, "rest-authz-deny", "secure", now)
	authzRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/authorizers", map[string]any{
		"name": "tok", "type": "TOKEN", "authorizerUri": authzARN,
		"identitySource": "method.request.header.Authorization",
	}, now)
	if authzRec.Code != http.StatusOK {
		t.Fatalf("CreateAuthorizer status=%d body=%q", authzRec.Code, authzRec.Body.String())
	}
	var authzResp map[string]any
	_ = json.Unmarshal(authzRec.Body.Bytes(), &authzResp)
	authorizerID, _ := authzResp["id"].(string)

	methodRec := mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET",
		map[string]any{"authorizationType": "TOKEN", "authorizerId": authorizerID}, now)
	if methodRec.Code != http.StatusCreated {
		t.Fatalf("PutMethod status=%d body=%q", methodRec.Code, methodRec.Body.String())
	}
	mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET/integration",
		map[string]any{"type": "MOCK", "requestTemplates": map[string]any{"application/json": `{"ok":true}`}}, now)
	mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/deployments", map[string]any{
		"stageName": "dev",
	}, now)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/restapis/"+apiID+"/dev/_user_request_/secure", nil)
	req.Header.Set("Authorization", "bad")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("deny status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if integrationInvokes.Load() != 0 {
		t.Fatalf("integration invoked on Deny")
	}
}

func TestAPIGatewayRESTREQUESTAuthorizerAllow(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "rest-authz-allow-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/rest-authz-allow-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "rest-authz-allow-fn", "Runtime": "python3.12", "Role": roleARN,
		"Handler": "app.handler", "Code": map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d", fnRec.Code)
	}
	mustAddLambdaServicePermission(t, handler, "rest-authz-allow-fn", "apigateway.amazonaws.com", "perm-rest-authz", now)
	authzARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:rest-authz-allow-fn"

	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		if name == "rest-authz-allow-fn" {
			if !strings.Contains(eventJSON, `"type":"REQUEST"`) && !strings.Contains(eventJSON, `"type": "REQUEST"`) {
				t.Fatalf("authorizer event missing type REQUEST: %s", eventJSON)
			}
			return []byte(`{"policyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"execute-api:Invoke","Resource":"*"}]}}`), nil
		}
		return []byte(`{"statusCode":200,"body":"ok"}`), nil
	})

	apiID, resourceID := mustCreateRESTMockAPI(t, handler, "rest-authz-allow", "secure", now)
	authzRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/authorizers", map[string]any{
		"name": "req", "type": "REQUEST", "authorizerUri": authzARN,
		"identitySource": "method.request.header.Authorization",
	}, now)
	var authzResp map[string]any
	_ = json.Unmarshal(authzRec.Body.Bytes(), &authzResp)
	authorizerID, _ := authzResp["id"].(string)

	mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET",
		map[string]any{"authorizationType": "CUSTOM", "authorizerId": authorizerID}, now)
	mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET/integration",
		map[string]any{"type": "MOCK", "requestTemplates": map[string]any{"application/json": `{"message":"allowed"}`}}, now)
	mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/deployments", map[string]any{
		"stageName": "dev",
	}, now)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/restapis/"+apiID+"/dev/_user_request_/secure", nil)
	req.Header.Set("Authorization", "ok")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("allow status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "allowed") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestAPIGatewayRESTApiKeyUsagePlanLite(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	apiID, resourceID := mustCreateRESTMockAPI(t, handler, "rest-apikey", "paid", now)
	mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET",
		map[string]any{"authorizationType": "NONE", "apiKeyRequired": true}, now)
	mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET/integration",
		map[string]any{"type": "MOCK", "requestTemplates": map[string]any{"application/json": `{"message":"keyed"}`}}, now)
	mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/deployments", map[string]any{
		"stageName": "dev",
	}, now)

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/restapis/"+apiID+"/dev/_user_request_/paid", nil))
	if missing.Code != http.StatusForbidden {
		t.Fatalf("missing key status=%d want 403", missing.Code)
	}

	keyRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/apikeys", map[string]any{
		"name": "lab", "enabled": true,
	}, now)
	if keyRec.Code != http.StatusOK {
		t.Fatalf("CreateApiKey status=%d body=%q", keyRec.Code, keyRec.Body.String())
	}
	var keyResp map[string]any
	_ = json.Unmarshal(keyRec.Body.Bytes(), &keyResp)
	keyID, _ := keyResp["id"].(string)
	keyValue, _ := keyResp["value"].(string)
	if keyID == "" || keyValue == "" {
		t.Fatalf("api key fields: %v", keyResp)
	}

	planRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/usageplans", map[string]any{
		"name": "plan", "apiStages": []map[string]any{{"apiId": apiID, "stage": "dev"}},
	}, now)
	if planRec.Code != http.StatusOK {
		t.Fatalf("CreateUsagePlan status=%d body=%q", planRec.Code, planRec.Body.String())
	}
	var planResp map[string]any
	_ = json.Unmarshal(planRec.Body.Bytes(), &planResp)
	planID, _ := planResp["id"].(string)

	assoc := mustAPIGatewayREST(t, handler, http.MethodPost, "/usageplans/"+planID+"/keys", map[string]any{
		"keyId": keyID, "keyType": "API_KEY",
	}, now)
	if assoc.Code != http.StatusOK {
		t.Fatalf("CreateUsagePlanKey status=%d body=%q", assoc.Code, assoc.Body.String())
	}

	bad := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/restapis/"+apiID+"/dev/_user_request_/paid", nil)
	badReq.Header.Set("x-api-key", "wrong")
	handler.ServeHTTP(bad, badReq)
	if bad.Code != http.StatusForbidden {
		t.Fatalf("bad key status=%d", bad.Code)
	}

	ok := httptest.NewRecorder()
	okReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/restapis/"+apiID+"/dev/_user_request_/paid", nil)
	okReq.Header.Set("x-api-key", keyValue)
	handler.ServeHTTP(ok, okReq)
	if ok.Code != http.StatusOK {
		t.Fatalf("valid key status=%d body=%q", ok.Code, ok.Body.String())
	}
	if !strings.Contains(ok.Body.String(), "keyed") {
		t.Fatalf("body=%q", ok.Body.String())
	}

	listKeys := mustAPIGatewayREST(t, handler, http.MethodGet, "/usageplans/"+planID+"/keys", nil, now)
	if listKeys.Code != http.StatusOK {
		t.Fatalf("GetUsagePlanKeys status=%d", listKeys.Code)
	}
	delKey := mustAPIGatewayREST(t, handler, http.MethodDelete, "/usageplans/"+planID+"/keys/"+keyID, nil, now)
	if delKey.Code != http.StatusAccepted {
		t.Fatalf("DeleteUsagePlanKey status=%d", delKey.Code)
	}
}

func TestAPIGatewayRESTAuthzHandlersCoverage(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis", map[string]any{"name": "authz-cov"}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateRestApi %d %s", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["id"].(string)

	emptyGroup := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/authorizers", map[string]any{
		"name": "", "type": "TOKEN",
	}, now)
	if emptyGroup.Code != http.StatusBadRequest {
		t.Fatalf("CreateAuthorizer empty name want 400 got %d", emptyGroup.Code)
	}

	authzRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/authorizers", map[string]any{
		"name": "tok2", "type": "TOKEN",
		"authorizerUri":  "arn:aws:lambda:us-east-1:" + testAccountID + ":function:authz-cov",
		"identitySource": "method.request.header.Authorization",
	}, now)
	if authzRec.Code != http.StatusOK {
		t.Fatalf("CreateAuthorizer %d %s", authzRec.Code, authzRec.Body.String())
	}

	missAPIAuthz := mustAPIGatewayREST(t, handler, http.MethodGet, "/restapis/missing-api/authorizers", nil, now)
	if missAPIAuthz.Code != http.StatusNotFound {
		t.Fatalf("GetAuthorizers missing api want 404 got %d", missAPIAuthz.Code)
	}

	keyRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/apikeys", map[string]any{
		"name": "disabled-key", "enabled": false,
	}, now)
	if keyRec.Code != http.StatusOK {
		t.Fatalf("CreateApiKey %d %s", keyRec.Code, keyRec.Body.String())
	}
	var keyResp map[string]any
	_ = json.Unmarshal(keyRec.Body.Bytes(), &keyResp)
	apiKeyID, _ := keyResp["id"].(string)

	badPlanStages := mustAPIGatewayREST(t, handler, http.MethodPost, "/usageplans", map[string]any{
		"name": "bad-stages",
		"apiStages": []map[string]any{{
			"apiId": "missing", "stage": "prod",
		}},
	}, now)
	if badPlanStages.Code != http.StatusNotFound && badPlanStages.Code != http.StatusBadRequest {
		t.Fatalf("CreateUsagePlan bad stage want 4xx got %d %s", badPlanStages.Code, badPlanStages.Body.String())
	}

	planRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/usageplans", map[string]any{
		"name": "cov-plan-only",
	}, now)
	if planRec.Code != http.StatusOK {
		t.Fatalf("CreateUsagePlan %d %s", planRec.Code, planRec.Body.String())
	}
	var planResp map[string]any
	_ = json.Unmarshal(planRec.Body.Bytes(), &planResp)
	planID, _ := planResp["id"].(string)

	badKeyType := mustAPIGatewayREST(t, handler, http.MethodPost, "/usageplans/"+planID+"/keys", map[string]any{
		"keyId": apiKeyID, "keyType": "CUSTOM",
	}, now)
	if badKeyType.Code != http.StatusBadRequest || !strings.Contains(badKeyType.Body.String(), "API_KEY") {
		t.Fatalf("CreateUsagePlanKey bad type want 400 got %d %s", badKeyType.Code, badKeyType.Body.String())
	}

	missPlanKeys := mustAPIGatewayREST(t, handler, http.MethodGet, "/usageplans/missing-plan/keys", nil, now)
	if missPlanKeys.Code != http.StatusNotFound {
		t.Fatalf("GetUsagePlanKeys missing plan want 404 got %d", missPlanKeys.Code)
	}

	delMissPlanKey := mustAPIGatewayREST(t, handler, http.MethodDelete, "/usageplans/"+planID+"/keys/missing-key", nil, now)
	if delMissPlanKey.Code != http.StatusNotFound {
		t.Fatalf("DeleteUsagePlanKey missing want 404 got %d", delMissPlanKey.Code)
	}
}
