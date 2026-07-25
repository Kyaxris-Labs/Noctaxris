package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAppSyncNestedSelectionResolvesParentAndChild(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "appsync-nest-exec", lambdaTrustOK, now)
	execARN := "arn:aws:iam::" + testAccountID + ":role/appsync-nest-exec"
	for _, name := range []string{"appsync-get-user", "appsync-user-email"} {
		fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
			"FunctionName": name,
			"Runtime":      "python3.12",
			"Role":         execARN,
			"Handler":      "app.handler",
			"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		}, now)
		if fnRec.Code != http.StatusOK {
			t.Fatalf("CreateFunction %s status=%d body=%q", name, fnRec.Code, fnRec.Body.String())
		}
		mustAddLambdaServicePermission(t, handler, name, "appsync.amazonaws.com", "perm-"+name, now)
	}

	var emailInvokes int
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		switch name {
		case "appsync-get-user":
			if !strings.Contains(eventJSON, `"field":"getUser"`) && !strings.Contains(eventJSON, `"field": "getUser"`) {
				t.Fatalf("getUser event: %s", eventJSON)
			}
			if !strings.Contains(eventJSON, `"parentTypeName":"Query"`) && !strings.Contains(eventJSON, `"parentTypeName": "Query"`) {
				t.Fatalf("getUser parentType: %s", eventJSON)
			}
			return []byte(`{"id":"u1","name":"Ada","email":"hidden"}`), nil
		case "appsync-user-email":
			emailInvokes++
			if !strings.Contains(eventJSON, `"parentTypeName":"User"`) && !strings.Contains(eventJSON, `"parentTypeName": "User"`) {
				t.Fatalf("email parentType: %s", eventJSON)
			}
			if !strings.Contains(eventJSON, `"source"`) {
				t.Fatalf("email missing source: %s", eventJSON)
			}
			if !strings.Contains(eventJSON, `"id":"u1"`) && !strings.Contains(eventJSON, `"id": "u1"`) {
				t.Fatalf("email source id: %s", eventJSON)
			}
			return []byte(`"ada@example.com"`), nil
		default:
			return nil, context.Canceled
		}
	})

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "nest-gql",
		"authenticationType": "API_KEY",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraphqlApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)

	schema := "type Query { getUser: User } type User { id: ID name: String email: String }"
	mustJSONTarget(t, handler, "AWSAppSync.StartSchemaCreation", "appsync", map[string]any{
		"apiId": apiID, "definition": schema,
	}, now)
	keyRec := mustJSONTarget(t, handler, "AWSAppSync.CreateApiKey", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	var keyResp map[string]any
	_ = json.Unmarshal(keyRec.Body.Bytes(), &keyResp)
	apiKeyObj, _ := keyResp["apiKey"].(map[string]any)
	apiKey, _ := apiKeyObj["id"].(string)

	getUserARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-get-user"
	emailARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-user-email"
	mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "GetUserDS", "type": "AWS_LAMBDA",
		"lambdaConfig": map[string]any{"lambdaFunctionArn": getUserARN},
	}, now)
	mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "EmailDS", "type": "AWS_LAMBDA",
		"lambdaConfig": map[string]any{"lambdaFunctionArn": emailARN},
	}, now)
	mustJSONTarget(t, handler, "AWSAppSync.CreateResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query", "fieldName": "getUser", "dataSourceName": "GetUserDS",
	}, now)
	mustJSONTarget(t, handler, "AWSAppSync.CreateResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "User", "fieldName": "email", "dataSourceName": "EmailDS",
	}, now)

	body, _ := json.Marshal(map[string]any{"query": "{ getUser { name email } }"})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/appsync/"+apiID+"/graphql", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("graphql status=%d body=%q", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["errors"]; ok {
		t.Fatalf("unexpected errors: %s", rec.Body.String())
	}
	data, _ := out["data"].(map[string]any)
	user, _ := data["getUser"].(map[string]any)
	if user["name"] != "Ada" || user["email"] != "ada@example.com" {
		t.Fatalf("want name=Ada email=ada@example.com, got %#v body=%s", user, rec.Body.String())
	}
	if emailInvokes != 1 {
		t.Fatalf("email resolver invokes=%d want 1", emailInvokes)
	}
}

func TestAppSyncNestedSelectionDepthLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "depth-gql",
		"authenticationType": "API_KEY",
	}, now)
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)
	keyRec := mustJSONTarget(t, handler, "AWSAppSync.CreateApiKey", "appsync", map[string]any{"apiId": apiID}, now)
	var keyResp map[string]any
	_ = json.Unmarshal(keyRec.Body.Bytes(), &keyResp)
	apiKeyObj, _ := keyResp["apiKey"].(map[string]any)
	apiKey, _ := apiKeyObj["id"].(string)

	body, _ := json.Marshal(map[string]any{"query": "{ a { b { c { d } } } }"})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/appsync/"+apiID+"/graphql", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "depth") {
		t.Fatalf("want depth error, got %s", rec.Body.String())
	}
}
