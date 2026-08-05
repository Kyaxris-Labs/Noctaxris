package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func mustAPIGatewayREST(
	t *testing.T, handler http.Handler, method, path string, payload map[string]any, now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if payload != nil {
		var err error
		raw, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := mustNewRequest(t, method, "http://127.0.0.1:4566"+path, raw)
	if len(raw) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "apigateway", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAPIGatewayRESTMockInvoke(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis", map[string]any{
		"name": "lab-rest-mock",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateRestApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["id"].(string)
	rootID, _ := apiResp["rootResourceId"].(string)
	if apiID == "" || rootID == "" {
		t.Fatalf("missing api fields: %v", apiResp)
	}

	resRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/resources/"+rootID, map[string]any{
		"pathPart": "hello",
	}, now)
	if resRec.Code != http.StatusOK {
		t.Fatalf("CreateResource status=%d body=%q", resRec.Code, resRec.Body.String())
	}
	var resResp map[string]any
	_ = json.Unmarshal(resRec.Body.Bytes(), &resResp)
	resourceID, _ := resResp["id"].(string)

	methodRec := mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET",
		map[string]any{"authorizationType": "NONE"}, now)
	if methodRec.Code != http.StatusCreated {
		t.Fatalf("PutMethod status=%d body=%q", methodRec.Code, methodRec.Body.String())
	}

	intRec := mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET/integration",
		map[string]any{
			"type": "MOCK",
			"requestTemplates": map[string]any{
				"application/json": `{"message":"mock-ok"}`,
			},
		}, now)
	if intRec.Code != http.StatusCreated {
		t.Fatalf("PutIntegration status=%d body=%q", intRec.Code, intRec.Body.String())
	}

	depRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/deployments", map[string]any{
		"stageName": "dev",
	}, now)
	if depRec.Code != http.StatusCreated {
		t.Fatalf("CreateDeployment status=%d body=%q", depRec.Code, depRec.Body.String())
	}

	stageRec := mustAPIGatewayREST(t, handler, http.MethodGet, "/restapis/"+apiID+"/stages/dev", nil, now)
	if stageRec.Code != http.StatusOK {
		t.Fatalf("GetStage status=%d body=%q", stageRec.Code, stageRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/restapis/"+apiID+"/dev/_user_request_/hello", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("MOCK invoke status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "mock-ok") {
		t.Fatalf("body=%q", rec.Body.String())
	}

	list := mustAPIGatewayREST(t, handler, http.MethodGet, "/restapis", nil, now)
	if list.Code != http.StatusOK {
		t.Fatalf("GetRestApis status=%d", list.Code)
	}
	del := mustAPIGatewayREST(t, handler, http.MethodDelete, "/restapis/"+apiID, nil, now)
	if del.Code != http.StatusAccepted {
		t.Fatalf("DeleteRestApi status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestAPIGatewayRESTLambdaInvokeComputeUnavailable(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "apigw-rest-lambda-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/apigw-rest-lambda-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "apigw-rest-hello",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", fnRec.Code, fnRec.Body.String())
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:apigw-rest-hello"
	mustAddLambdaServicePermission(t, handler, "apigw-rest-hello", "apigateway.amazonaws.com", "apigw-rest-invoke", now)

	create := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis", map[string]any{
		"name": "lab-rest-lambda",
	}, now)
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["id"].(string)
	rootID, _ := apiResp["rootResourceId"].(string)

	resRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/resources/"+rootID, map[string]any{
		"pathPart": "proxy",
	}, now)
	var resResp map[string]any
	_ = json.Unmarshal(resRec.Body.Bytes(), &resResp)
	resourceID, _ := resResp["id"].(string)

	mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET",
		map[string]any{"authorizationType": "NONE"}, now)

	uri := "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/" + lambdaARN + "/invocations"
	intRec := mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET/integration",
		map[string]any{
			"type":       "AWS_PROXY",
			"uri":        uri,
			"httpMethod": "POST",
		}, now)
	if intRec.Code != http.StatusCreated {
		t.Fatalf("PutIntegration status=%d body=%q", intRec.Code, intRec.Body.String())
	}

	mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/deployments", map[string]any{
		"stageName": "prod",
	}, now)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/restapis/"+apiID+"/prod/_user_request_/proxy", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Lambda invoke status=%d want 503 body=%q", rec.Code, rec.Body.String())
	}
}

func TestAPIGatewayRESTGetDeleteStageAuthzKeysUsagePlan(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis", map[string]any{
		"name": "rest-crud",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateRestApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["id"].(string)
	rootID, _ := apiResp["rootResourceId"].(string)

	getAPI := mustAPIGatewayREST(t, handler, http.MethodGet, "/restapis/"+apiID, nil, now)
	if getAPI.Code != http.StatusOK || !strings.Contains(getAPI.Body.String(), "rest-crud") {
		t.Fatalf("GetRestApi status=%d body=%q", getAPI.Code, getAPI.Body.String())
	}
	missAPI := mustAPIGatewayREST(t, handler, http.MethodGet, "/restapis/missing", nil, now)
	if missAPI.Code != http.StatusNotFound {
		t.Fatalf("GetRestApi missing status=%d body=%q", missAPI.Code, missAPI.Body.String())
	}

	resRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/resources/"+rootID, map[string]any{
		"pathPart": "ping",
	}, now)
	if resRec.Code != http.StatusOK {
		t.Fatalf("CreateResource status=%d body=%q", resRec.Code, resRec.Body.String())
	}
	var resResp map[string]any
	_ = json.Unmarshal(resRec.Body.Bytes(), &resResp)
	resourceID, _ := resResp["id"].(string)

	listRes := mustAPIGatewayREST(t, handler, http.MethodGet, "/restapis/"+apiID+"/resources", nil, now)
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), resourceID) {
		t.Fatalf("GetResources status=%d body=%q", listRes.Code, listRes.Body.String())
	}

	methodRec := mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET",
		map[string]any{"authorizationType": "NONE"}, now)
	if methodRec.Code != http.StatusCreated {
		t.Fatalf("PutMethod status=%d body=%q", methodRec.Code, methodRec.Body.String())
	}
	getMethod := mustAPIGatewayREST(t, handler, http.MethodGet,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET", nil, now)
	if getMethod.Code != http.StatusOK {
		t.Fatalf("GetMethod status=%d body=%q", getMethod.Code, getMethod.Body.String())
	}
	missMethod := mustAPIGatewayREST(t, handler, http.MethodGet,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/POST", nil, now)
	if missMethod.Code != http.StatusNotFound {
		t.Fatalf("GetMethod missing status=%d body=%q", missMethod.Code, missMethod.Body.String())
	}

	intRec := mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET/integration",
		map[string]any{
			"type": "MOCK",
			"requestTemplates": map[string]any{
				"application/json": `{"ok":true}`,
			},
		}, now)
	if intRec.Code != http.StatusCreated {
		t.Fatalf("PutIntegration status=%d body=%q", intRec.Code, intRec.Body.String())
	}
	getInt := mustAPIGatewayREST(t, handler, http.MethodGet,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET/integration", nil, now)
	if getInt.Code != http.StatusOK || !strings.Contains(getInt.Body.String(), "MOCK") {
		t.Fatalf("GetIntegration status=%d body=%q", getInt.Code, getInt.Body.String())
	}

	depRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/deployments", map[string]any{
		"description": "lab",
	}, now)
	if depRec.Code != http.StatusCreated {
		t.Fatalf("CreateDeployment status=%d body=%q", depRec.Code, depRec.Body.String())
	}
	var depResp map[string]any
	_ = json.Unmarshal(depRec.Body.Bytes(), &depResp)
	deploymentID, _ := depResp["id"].(string)
	if deploymentID == "" {
		t.Fatalf("missing deployment id: %v", depResp)
	}

	stageRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/stages", map[string]any{
		"stageName":    "prod",
		"deploymentId": deploymentID,
	}, now)
	if stageRec.Code != http.StatusCreated {
		t.Fatalf("CreateStage status=%d body=%q", stageRec.Code, stageRec.Body.String())
	}
	badStage := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/stages", map[string]any{
		"stageName":    "bad",
		"deploymentId": "missing-dep",
	}, now)
	if badStage.Code != http.StatusNotFound {
		t.Fatalf("CreateStage bad dep status=%d body=%q", badStage.Code, badStage.Body.String())
	}

	authzRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis/"+apiID+"/authorizers", map[string]any{
		"name": "tok", "type": "TOKEN",
		"authorizerUri":  "arn:aws:lambda:us-east-1:" + testAccountID + ":function:authz",
		"identitySource": "method.request.header.Authorization",
	}, now)
	if authzRec.Code != http.StatusOK {
		t.Fatalf("CreateAuthorizer status=%d body=%q", authzRec.Code, authzRec.Body.String())
	}
	var authzResp map[string]any
	_ = json.Unmarshal(authzRec.Body.Bytes(), &authzResp)
	authorizerID, _ := authzResp["id"].(string)

	getAuthz := mustAPIGatewayREST(t, handler, http.MethodGet, "/restapis/"+apiID+"/authorizers/"+authorizerID, nil, now)
	if getAuthz.Code != http.StatusOK || !strings.Contains(getAuthz.Body.String(), "tok") {
		t.Fatalf("GetAuthorizer status=%d body=%q", getAuthz.Code, getAuthz.Body.String())
	}
	listAuthz := mustAPIGatewayREST(t, handler, http.MethodGet, "/restapis/"+apiID+"/authorizers", nil, now)
	if listAuthz.Code != http.StatusOK || !strings.Contains(listAuthz.Body.String(), authorizerID) {
		t.Fatalf("GetAuthorizers status=%d body=%q", listAuthz.Code, listAuthz.Body.String())
	}
	missAuthz := mustAPIGatewayREST(t, handler, http.MethodGet, "/restapis/"+apiID+"/authorizers/missing", nil, now)
	if missAuthz.Code != http.StatusNotFound {
		t.Fatalf("GetAuthorizer missing status=%d body=%q", missAuthz.Code, missAuthz.Body.String())
	}

	keyRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/apikeys", map[string]any{
		"name": "lab-key", "enabled": true,
	}, now)
	if keyRec.Code != http.StatusOK {
		t.Fatalf("CreateApiKey status=%d body=%q", keyRec.Code, keyRec.Body.String())
	}
	var keyResp map[string]any
	_ = json.Unmarshal(keyRec.Body.Bytes(), &keyResp)
	apiKeyID, _ := keyResp["id"].(string)
	getKey := mustAPIGatewayREST(t, handler, http.MethodGet, "/apikeys/"+apiKeyID, nil, now)
	if getKey.Code != http.StatusOK || !strings.Contains(getKey.Body.String(), "lab-key") {
		t.Fatalf("GetApiKey status=%d body=%q", getKey.Code, getKey.Body.String())
	}
	listKeys := mustAPIGatewayREST(t, handler, http.MethodGet, "/apikeys", nil, now)
	if listKeys.Code != http.StatusOK || !strings.Contains(listKeys.Body.String(), apiKeyID) {
		t.Fatalf("GetApiKeys status=%d body=%q", listKeys.Code, listKeys.Body.String())
	}
	missKey := mustAPIGatewayREST(t, handler, http.MethodGet, "/apikeys/missing", nil, now)
	if missKey.Code != http.StatusNotFound {
		t.Fatalf("GetApiKey missing status=%d body=%q", missKey.Code, missKey.Body.String())
	}

	planRec := mustAPIGatewayREST(t, handler, http.MethodPost, "/usageplans", map[string]any{
		"name": "lab-plan",
	}, now)
	if planRec.Code != http.StatusOK {
		t.Fatalf("CreateUsagePlan status=%d body=%q", planRec.Code, planRec.Body.String())
	}
	var planResp map[string]any
	_ = json.Unmarshal(planRec.Body.Bytes(), &planResp)
	planID, _ := planResp["id"].(string)
	getPlan := mustAPIGatewayREST(t, handler, http.MethodGet, "/usageplans/"+planID, nil, now)
	if getPlan.Code != http.StatusOK || !strings.Contains(getPlan.Body.String(), "lab-plan") {
		t.Fatalf("GetUsagePlan status=%d body=%q", getPlan.Code, getPlan.Body.String())
	}
	listPlans := mustAPIGatewayREST(t, handler, http.MethodGet, "/usageplans", nil, now)
	if listPlans.Code != http.StatusOK || !strings.Contains(listPlans.Body.String(), planID) {
		t.Fatalf("GetUsagePlans status=%d body=%q", listPlans.Code, listPlans.Body.String())
	}
	missPlan := mustAPIGatewayREST(t, handler, http.MethodGet, "/usageplans/missing", nil, now)
	if missPlan.Code != http.StatusNotFound {
		t.Fatalf("GetUsagePlan missing status=%d body=%q", missPlan.Code, missPlan.Body.String())
	}

	planKey := mustAPIGatewayREST(t, handler, http.MethodPost, "/usageplans/"+planID+"/keys", map[string]any{
		"keyId":   apiKeyID,
		"keyType": "API_KEY",
	}, now)
	if planKey.Code != http.StatusCreated && planKey.Code != http.StatusOK {
		t.Fatalf("CreateUsagePlanKey status=%d body=%q", planKey.Code, planKey.Body.String())
	}
	listPlanKeys := mustAPIGatewayREST(t, handler, http.MethodGet, "/usageplans/"+planID+"/keys", nil, now)
	if listPlanKeys.Code != http.StatusOK || !strings.Contains(listPlanKeys.Body.String(), apiKeyID) {
		t.Fatalf("GetUsagePlanKeys status=%d body=%q", listPlanKeys.Code, listPlanKeys.Body.String())
	}

	delPlanKey := mustAPIGatewayREST(t, handler, http.MethodDelete, "/usageplans/"+planID+"/keys/"+apiKeyID, nil, now)
	if delPlanKey.Code != http.StatusAccepted && delPlanKey.Code != http.StatusOK && delPlanKey.Code != http.StatusNoContent {
		t.Fatalf("DeleteUsagePlanKey status=%d body=%q", delPlanKey.Code, delPlanKey.Body.String())
	}
	delPlan := mustAPIGatewayREST(t, handler, http.MethodDelete, "/usageplans/"+planID, nil, now)
	if delPlan.Code != http.StatusAccepted && delPlan.Code != http.StatusOK && delPlan.Code != http.StatusNoContent {
		t.Fatalf("DeleteUsagePlan status=%d body=%q", delPlan.Code, delPlan.Body.String())
	}
	delKey := mustAPIGatewayREST(t, handler, http.MethodDelete, "/apikeys/"+apiKeyID, nil, now)
	if delKey.Code != http.StatusAccepted && delKey.Code != http.StatusOK && delKey.Code != http.StatusNoContent {
		t.Fatalf("DeleteApiKey status=%d body=%q", delKey.Code, delKey.Body.String())
	}
	delAuthz := mustAPIGatewayREST(t, handler, http.MethodDelete, "/restapis/"+apiID+"/authorizers/"+authorizerID, nil, now)
	if delAuthz.Code != http.StatusAccepted && delAuthz.Code != http.StatusOK && delAuthz.Code != http.StatusNoContent {
		t.Fatalf("DeleteAuthorizer status=%d body=%q", delAuthz.Code, delAuthz.Body.String())
	}

	delMethod := mustAPIGatewayREST(t, handler, http.MethodDelete,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET", nil, now)
	if delMethod.Code != http.StatusAccepted && delMethod.Code != http.StatusOK && delMethod.Code != http.StatusNoContent {
		t.Fatalf("DeleteMethod status=%d body=%q", delMethod.Code, delMethod.Body.String())
	}
	delRes := mustAPIGatewayREST(t, handler, http.MethodDelete, "/restapis/"+apiID+"/resources/"+resourceID, nil, now)
	if delRes.Code != http.StatusAccepted && delRes.Code != http.StatusOK && delRes.Code != http.StatusNoContent {
		t.Fatalf("DeleteResource status=%d body=%q", delRes.Code, delRes.Body.String())
	}
	delMissRes := mustAPIGatewayREST(t, handler, http.MethodDelete, "/restapis/"+apiID+"/resources/missing", nil, now)
	if delMissRes.Code != http.StatusNotFound {
		t.Fatalf("DeleteResource missing status=%d body=%q", delMissRes.Code, delMissRes.Body.String())
	}
}

func TestAPIGatewayRESTPutMethodOpenDataPlaneDenied(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.ListenAddr = "0.0.0.0:4566"
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	apiID, resourceID := mustCreateRESTMockAPI(t, handler, "rest-odp", "secure", now)
	deny := mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+resourceID+"/methods/GET",
		map[string]any{"authorizationType": "NONE"}, now)
	if deny.Code != http.StatusBadRequest || !strings.Contains(deny.Body.String(), "NOCTAXRIS_ALLOW_OPEN_DATA_PLANE") {
		t.Fatalf("NONE PutMethod on non-loopback status=%d body=%q", deny.Code, deny.Body.String())
	}
}
