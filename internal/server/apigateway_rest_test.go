package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
