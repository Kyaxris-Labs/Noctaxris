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

const appsyncTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"appsync.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

func TestAppSyncCreateDataSourcePassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "appsync-ds-ok", appsyncTrustOK, now)
	mustCreateIAMRole(t, handler, "appsync-ds-bad", lambdaTrustOK, now)
	okARN := "arn:aws:iam::" + testAccountID + ":role/appsync-ds-ok"
	badARN := "arn:aws:iam::" + testAccountID + ":role/appsync-ds-bad"
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-pass"

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "passrole-gql",
		"authenticationType": "API_KEY",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraphqlApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)

	deny := mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "BadDS", "type": "AWS_LAMBDA",
		"serviceRoleArn": badARN,
		"lambdaConfig":   map[string]any{"lambdaFunctionArn": lambdaARN},
	}, now)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("bad trust status=%d want 403 body=%q", deny.Code, deny.Body.String())
	}

	allow := mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "OkDS", "type": "AWS_LAMBDA",
		"serviceRoleArn": okARN,
		"lambdaConfig":   map[string]any{"lambdaFunctionArn": lambdaARN},
	}, now)
	if allow.Code != http.StatusOK {
		t.Fatalf("ok trust status=%d body=%q", allow.Code, allow.Body.String())
	}
	if !strings.Contains(allow.Body.String(), okARN) {
		t.Fatalf("response missing serviceRoleArn: %s", allow.Body.String())
	}
}

func TestAppSyncMultiFieldQueryReturnsBothKeys(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "appsync-multi-exec", lambdaTrustOK, now)
	execARN := "arn:aws:iam::" + testAccountID + ":role/appsync-multi-exec"
	for _, name := range []string{"appsync-hello-fn", "appsync-world-fn"} {
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

	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		switch name {
		case "appsync-hello-fn":
			if !strings.Contains(eventJSON, `"field":"hello"`) && !strings.Contains(eventJSON, `"field": "hello"`) {
				t.Fatalf("hello event missing field: %s", eventJSON)
			}
			return []byte(`"hi"`), nil
		case "appsync-world-fn":
			return []byte(`"earth"`), nil
		default:
			return nil, context.Canceled
		}
	})

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "multi-gql",
		"authenticationType": "API_KEY",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraphqlApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)

	mustJSONTarget(t, handler, "AWSAppSync.StartSchemaCreation", "appsync", map[string]any{
		"apiId": apiID, "definition": "type Query { hello: String world: String }",
	}, now)
	keyRec := mustJSONTarget(t, handler, "AWSAppSync.CreateApiKey", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	var keyResp map[string]any
	_ = json.Unmarshal(keyRec.Body.Bytes(), &keyResp)
	apiKeyObj, _ := keyResp["apiKey"].(map[string]any)
	apiKey, _ := apiKeyObj["id"].(string)

	helloARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-hello-fn"
	worldARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-world-fn"
	mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "HelloDS", "type": "AWS_LAMBDA",
		"lambdaConfig": map[string]any{"lambdaFunctionArn": helloARN},
	}, now)
	mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID, "name": "WorldDS", "type": "AWS_LAMBDA",
		"lambdaConfig": map[string]any{"lambdaFunctionArn": worldARN},
	}, now)
	mustJSONTarget(t, handler, "AWSAppSync.CreateResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query", "fieldName": "hello", "dataSourceName": "HelloDS",
	}, now)
	mustJSONTarget(t, handler, "AWSAppSync.CreateResolver", "appsync", map[string]any{
		"apiId": apiID, "typeName": "Query", "fieldName": "world", "dataSourceName": "WorldDS",
	}, now)

	body, _ := json.Marshal(map[string]any{"query": "{ hello world }"})
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
	data, _ := out["data"].(map[string]any)
	if data["hello"] != "hi" || data["world"] != "earth" {
		t.Fatalf("want hello=hi world=earth, got %#v body=%s", data, rec.Body.String())
	}
}
