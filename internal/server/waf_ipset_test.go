package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWAFIPSetCRUDAndARNEnforceHTTPAPI(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "waf-ipset-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/waf-ipset-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "waf-ipset-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d", fnRec.Code)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:waf-ipset-fn"
	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "waf-ipset-http", lambdaARN, now)
	mustAddLambdaServicePermission(t, handler, "waf-ipset-fn", "apigateway.amazonaws.com", "waf-ipset-apigw", now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /ok", "Target": "integrations/" + integrationID,
		"AuthorizationType": "NONE",
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	createSet := mustJSONTarget(t, handler, "AWSWAF_20190729.CreateIPSet", "wafv2", map[string]any{
		"Name": "block-peers", "Scope": "REGIONAL", "IPAddressVersion": "IPV4",
		"Addresses": []string{"192.0.2.0/24"},
	}, now)
	if createSet.Code != http.StatusOK {
		t.Fatalf("CreateIPSet status=%d body=%q", createSet.Code, createSet.Body.String())
	}
	var setOut map[string]any
	_ = json.Unmarshal(createSet.Body.Bytes(), &setOut)
	sum, _ := setOut["Summary"].(map[string]any)
	ipsetARN, _ := sum["ARN"].(string)
	ipsetID, _ := sum["Id"].(string)
	lock, _ := sum["LockToken"].(string)
	if ipsetARN == "" || ipsetID == "" {
		t.Fatalf("CreateIPSet summary incomplete: %s", createSet.Body.String())
	}

	getSet := mustJSONTarget(t, handler, "AWSWAF_20190729.GetIPSet", "wafv2", map[string]any{
		"Name": "block-peers", "Scope": "REGIONAL", "Id": ipsetID,
	}, now)
	if getSet.Code != http.StatusOK || !strings.Contains(getSet.Body.String(), "192.0.2.0/24") {
		t.Fatalf("GetIPSet status=%d body=%q", getSet.Code, getSet.Body.String())
	}

	listSets := mustJSONTarget(t, handler, "AWSWAF_20190729.ListIPSets", "wafv2", map[string]any{
		"Scope": "REGIONAL",
	}, now)
	if listSets.Code != http.StatusOK || !strings.Contains(listSets.Body.String(), ipsetID) {
		t.Fatalf("ListIPSets status=%d body=%q", listSets.Code, listSets.Body.String())
	}

	createACL := mustJSONTarget(t, handler, "AWSWAF_20190729.CreateWebACL", "wafv2", map[string]any{
		"Name": "ipset-arn-acl", "Scope": "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"Rules": []map[string]any{{
			"Name": "block-ipset-arn", "Priority": 1,
			"Action": map[string]any{"Block": map[string]any{}},
			"Statement": map[string]any{
				"IPSetReferenceStatement": map[string]any{"ARN": ipsetARN},
			},
		}},
	}, now)
	if createACL.Code != http.StatusOK {
		t.Fatalf("CreateWebACL status=%d body=%q", createACL.Code, createACL.Body.String())
	}
	var aclOut map[string]any
	_ = json.Unmarshal(createACL.Body.Bytes(), &aclOut)
	aclSum, _ := aclOut["Summary"].(map[string]any)
	aclARN, _ := aclSum["ARN"].(string)

	assoc := mustJSONTarget(t, handler, "AWSWAF_20190729.AssociateWebACL", "wafv2", map[string]any{
		"WebACLArn":   aclARN,
		"ResourceArn": "arn:aws:apigateway:us-east-1::/apis/" + apiID + "/stages/$default",
	}, now)
	if assoc.Code != http.StatusOK {
		t.Fatalf("AssociateWebACL status=%d body=%q", assoc.Code, assoc.Body.String())
	}

	blockReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/ok", nil)
	blockReq.RemoteAddr = "192.0.2.44:54321"
	blockRec := httptest.NewRecorder()
	handler.ServeHTTP(blockRec, blockReq)
	if blockRec.Code != http.StatusForbidden {
		t.Fatalf("blocked peer status=%d want 403 body=%q", blockRec.Code, blockRec.Body.String())
	}

	okReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/ok", nil)
	okReq.RemoteAddr = "203.0.113.9:54321"
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, okReq)
	if okRec.Code == http.StatusForbidden {
		t.Fatalf("allowed peer blocked body=%q", okRec.Body.String())
	}

	upd := mustJSONTarget(t, handler, "AWSWAF_20190729.UpdateIPSet", "wafv2", map[string]any{
		"Name": "block-peers", "Scope": "REGIONAL", "Id": ipsetID, "LockToken": lock,
		"Addresses": []string{"203.0.113.0/24"},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateIPSet status=%d body=%q", upd.Code, upd.Body.String())
	}
	var updOut map[string]any
	_ = json.Unmarshal(upd.Body.Bytes(), &updOut)
	nextLock, _ := updOut["NextLockToken"].(string)

	del := mustJSONTarget(t, handler, "AWSWAF_20190729.DeleteIPSet", "wafv2", map[string]any{
		"Name": "block-peers", "Scope": "REGIONAL", "Id": ipsetID, "LockToken": nextLock,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteIPSet status=%d body=%q", del.Code, del.Body.String())
	}

	// After delete, ARN reference fails closed (403) on invoke.
	failReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/ok", nil)
	failReq.RemoteAddr = "203.0.113.9:54321"
	failRec := httptest.NewRecorder()
	handler.ServeHTTP(failRec, failReq)
	if failRec.Code != http.StatusForbidden {
		t.Fatalf("unknown ARN after delete status=%d want 403 body=%q", failRec.Code, failRec.Body.String())
	}
}
