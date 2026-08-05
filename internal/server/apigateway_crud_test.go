package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustAPIGWV2(
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

func TestAPIGatewayV2GetDeleteAndListHandlers(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name": "crud-http", "ProtocolType": "HTTP",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["apiId"].(string)
	if apiID == "" {
		t.Fatal("missing apiId")
	}

	get := mustJSONTarget(t, handler, "ApiGatewayV2.GetApi", "apigateway", map[string]any{
		"ApiId": apiID,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), apiID) {
		t.Fatalf("GetApi status=%d body=%q", get.Code, get.Body.String())
	}

	getREST := mustAPIGWV2(t, handler, http.MethodGet, "/v2/apis/"+apiID, nil, now)
	if getREST.Code != http.StatusOK || !strings.Contains(getREST.Body.String(), "crud-http") {
		t.Fatalf("GetApi REST status=%d body=%q", getREST.Code, getREST.Body.String())
	}

	list := mustJSONTarget(t, handler, "ApiGatewayV2.GetApis", "apigateway", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), apiID) {
		t.Fatalf("GetApis status=%d body=%q", list.Code, list.Body.String())
	}
	listREST := mustAPIGWV2(t, handler, http.MethodGet, "/v2/apis", nil, now)
	if listREST.Code != http.StatusOK || !strings.Contains(listREST.Body.String(), apiID) {
		t.Fatalf("GetApis REST status=%d body=%q", listREST.Code, listREST.Body.String())
	}

	miss := mustJSONTarget(t, handler, "ApiGatewayV2.GetApi", "apigateway", map[string]any{
		"ApiId": "missing-api",
	}, now)
	if miss.Code != http.StatusNotFound {
		t.Fatalf("GetApi missing status=%d body=%q", miss.Code, miss.Body.String())
	}
	empty := mustJSONTarget(t, handler, "ApiGatewayV2.GetApi", "apigateway", map[string]any{
		"ApiId": "",
	}, now)
	if empty.Code != http.StatusNotFound && empty.Code != http.StatusBadRequest {
		t.Fatalf("GetApi empty id status=%d body=%q", empty.Code, empty.Body.String())
	}

	poolID, clientID := mustCreateCognitoPoolClientUser(t, handler, "crud-pool", "crud-client", "cruduser", "Secret9!", now)
	issuer := store.CognitoIssuerURL("us-east-1", poolID)
	authz := mustJSONTarget(t, handler, "ApiGatewayV2.CreateAuthorizer", "apigateway", map[string]any{
		"ApiId": apiID, "Name": "jwt-list", "AuthorizerType": "JWT",
		"IdentitySource": []any{"$request.header.Authorization"},
		"JwtConfiguration": map[string]any{
			"Issuer":   issuer,
			"Audience": []any{clientID},
		},
	}, now)
	if authz.Code != http.StatusOK {
		t.Fatalf("CreateAuthorizer status=%d body=%q", authz.Code, authz.Body.String())
	}
	listAuthz := mustJSONTarget(t, handler, "ApiGatewayV2.GetAuthorizers", "apigateway", map[string]any{
		"ApiId": apiID,
	}, now)
	if listAuthz.Code != http.StatusOK || !strings.Contains(listAuthz.Body.String(), "jwt-list") {
		t.Fatalf("GetAuthorizers status=%d body=%q", listAuthz.Code, listAuthz.Body.String())
	}
	listAuthzREST := mustAPIGWV2(t, handler, http.MethodGet, "/v2/apis/"+apiID+"/authorizers", nil, now)
	if listAuthzREST.Code != http.StatusOK || !strings.Contains(listAuthzREST.Body.String(), "jwt-list") {
		t.Fatalf("GetAuthorizers REST status=%d body=%q", listAuthzREST.Code, listAuthzREST.Body.String())
	}
	missAuthz := mustJSONTarget(t, handler, "ApiGatewayV2.GetAuthorizers", "apigateway", map[string]any{
		"ApiId": "nope",
	}, now)
	if missAuthz.Code != http.StatusNotFound {
		t.Fatalf("GetAuthorizers missing api status=%d body=%q", missAuthz.Code, missAuthz.Body.String())
	}

	delMiss := mustJSONTarget(t, handler, "ApiGatewayV2.DeleteApi", "apigateway", map[string]any{
		"ApiId": "missing-api",
	}, now)
	if delMiss.Code != http.StatusNotFound {
		t.Fatalf("DeleteApi missing status=%d body=%q", delMiss.Code, delMiss.Body.String())
	}
	delREST := mustAPIGWV2(t, handler, http.MethodDelete, "/v2/apis/"+apiID, nil, now)
	if delREST.Code != http.StatusNoContent && delREST.Code != http.StatusOK && delREST.Code != http.StatusAccepted {
		t.Fatalf("DeleteApi REST status=%d body=%q", delREST.Code, delREST.Body.String())
	}
	getAfter := mustJSONTarget(t, handler, "ApiGatewayV2.GetApi", "apigateway", map[string]any{
		"ApiId": apiID,
	}, now)
	if getAfter.Code != http.StatusNotFound {
		t.Fatalf("GetApi after delete status=%d body=%q", getAfter.Code, getAfter.Body.String())
	}
}

func TestAPIGatewayV2CreateRouteOpenDataPlaneDenied(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.ListenAddr = "0.0.0.0:4566"
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	apiRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name": "odp-deny", "ProtocolType": "HTTP",
	}, now)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("CreateApi status=%d body=%q", apiRec.Code, apiRec.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(apiRec.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["apiId"].(string)

	deny := mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /open", "Target": "integrations/none",
		"AuthorizationType": "NONE",
	}, now)
	if deny.Code != http.StatusBadRequest || !strings.Contains(deny.Body.String(), "NOCTAXRIS_ALLOW_OPEN_DATA_PLANE") {
		t.Fatalf("NONE on non-loopback status=%d body=%q", deny.Code, deny.Body.String())
	}
}
