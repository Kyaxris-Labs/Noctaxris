package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAppSyncManagementSARHandlers(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "mgmt-sar",
		"authenticationType": "API_KEY",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraphqlApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)
	if apiID == "" {
		t.Fatal("missing apiId")
	}

	before := mustJSONTarget(t, handler, "AWSAppSync.GetSchemaCreationStatus", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	if before.Code != http.StatusOK || !strings.Contains(before.Body.String(), "NOT_APPLICABLE") {
		t.Fatalf("GetSchemaCreationStatus before status=%d body=%q", before.Code, before.Body.String())
	}

	schema := mustJSONTarget(t, handler, "AWSAppSync.StartSchemaCreation", "appsync", map[string]any{
		"apiId":      apiID,
		"definition": "type Query { hello: String }",
	}, now)
	if schema.Code != http.StatusOK {
		t.Fatalf("StartSchemaCreation status=%d body=%q", schema.Code, schema.Body.String())
	}

	after := mustJSONTarget(t, handler, "AWSAppSync.GetSchemaCreationStatus", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	if after.Code != http.StatusOK || !strings.Contains(after.Body.String(), `"SUCCESS"`) {
		t.Fatalf("GetSchemaCreationStatus after status=%d body=%q", after.Code, after.Body.String())
	}

	keyRec := mustJSONTarget(t, handler, "AWSAppSync.CreateApiKey", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	if keyRec.Code != http.StatusOK {
		t.Fatalf("CreateApiKey status=%d body=%q", keyRec.Code, keyRec.Body.String())
	}
	var keyResp map[string]any
	_ = json.Unmarshal(keyRec.Body.Bytes(), &keyResp)
	apiKeyObj, _ := keyResp["apiKey"].(map[string]any)
	plaintext, _ := apiKeyObj["id"].(string)

	listKeys := mustJSONTarget(t, handler, "AWSAppSync.ListApiKeys", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	if listKeys.Code != http.StatusOK || !strings.Contains(listKeys.Body.String(), "apiKeys") {
		t.Fatalf("ListApiKeys status=%d body=%q", listKeys.Code, listKeys.Body.String())
	}

	delKey := mustJSONTarget(t, handler, "AWSAppSync.DeleteApiKey", "appsync", map[string]any{
		"apiId": apiID,
		"id":    plaintext,
	}, now)
	if delKey.Code != http.StatusOK {
		t.Fatalf("DeleteApiKey status=%d body=%q", delKey.Code, delKey.Body.String())
	}

	ds := mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID,
		"name":  "HelloDS",
		"type":  "AWS_LAMBDA",
		"lambdaConfig": map[string]any{
			"lambdaFunctionArn": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-hello",
		},
	}, now)
	if ds.Code != http.StatusOK {
		t.Fatalf("CreateDataSource status=%d body=%q", ds.Code, ds.Body.String())
	}

	getDS := mustJSONTarget(t, handler, "AWSAppSync.GetDataSource", "appsync", map[string]any{
		"apiId": apiID,
		"name":  "HelloDS",
	}, now)
	if getDS.Code != http.StatusOK || !strings.Contains(getDS.Body.String(), "HelloDS") {
		t.Fatalf("GetDataSource status=%d body=%q", getDS.Code, getDS.Body.String())
	}

	updDS := mustJSONTarget(t, handler, "AWSAppSync.UpdateDataSource", "appsync", map[string]any{
		"apiId": apiID,
		"name":  "HelloDS",
		"type":  "AWS_LAMBDA",
		"lambdaConfig": map[string]any{
			"lambdaFunctionArn": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-hello-v2",
		},
	}, now)
	if updDS.Code != http.StatusOK || !strings.Contains(updDS.Body.String(), "appsync-hello-v2") {
		t.Fatalf("UpdateDataSource status=%d body=%q", updDS.Code, updDS.Body.String())
	}

	listDS := mustJSONTarget(t, handler, "AWSAppSync.ListDataSources", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	if listDS.Code != http.StatusOK || !strings.Contains(listDS.Body.String(), "dataSources") {
		t.Fatalf("ListDataSources status=%d body=%q", listDS.Code, listDS.Body.String())
	}

	res := mustJSONTarget(t, handler, "AWSAppSync.CreateResolver", "appsync", map[string]any{
		"apiId":          apiID,
		"typeName":       "Query",
		"fieldName":      "hello",
		"dataSourceName": "HelloDS",
	}, now)
	if res.Code != http.StatusOK {
		t.Fatalf("CreateResolver status=%d body=%q", res.Code, res.Body.String())
	}

	getRes := mustJSONTarget(t, handler, "AWSAppSync.GetResolver", "appsync", map[string]any{
		"apiId":     apiID,
		"typeName":  "Query",
		"fieldName": "hello",
	}, now)
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), "HelloDS") {
		t.Fatalf("GetResolver status=%d body=%q", getRes.Code, getRes.Body.String())
	}

	other := mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID,
		"name":  "OtherDS",
		"type":  "AWS_LAMBDA",
		"lambdaConfig": map[string]any{
			"lambdaFunctionArn": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-other",
		},
	}, now)
	if other.Code != http.StatusOK {
		t.Fatalf("CreateDataSource OtherDS status=%d body=%q", other.Code, other.Body.String())
	}

	updRes := mustJSONTarget(t, handler, "AWSAppSync.UpdateResolver", "appsync", map[string]any{
		"apiId":          apiID,
		"typeName":       "Query",
		"fieldName":      "hello",
		"dataSourceName": "OtherDS",
	}, now)
	if updRes.Code != http.StatusOK || !strings.Contains(updRes.Body.String(), "OtherDS") {
		t.Fatalf("UpdateResolver status=%d body=%q", updRes.Code, updRes.Body.String())
	}

	listRes := mustJSONTarget(t, handler, "AWSAppSync.ListResolvers", "appsync", map[string]any{
		"apiId":    apiID,
		"typeName": "Query",
	}, now)
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), "resolvers") {
		t.Fatalf("ListResolvers status=%d body=%q", listRes.Code, listRes.Body.String())
	}

	inUse := mustJSONTarget(t, handler, "AWSAppSync.DeleteDataSource", "appsync", map[string]any{
		"apiId": apiID,
		"name":  "OtherDS",
	}, now)
	if inUse.Code == http.StatusOK {
		t.Fatalf("expected DeleteDataSource in-use reject, got %s", inUse.Body.String())
	}

	delRes := mustJSONTarget(t, handler, "AWSAppSync.DeleteResolver", "appsync", map[string]any{
		"apiId":     apiID,
		"typeName":  "Query",
		"fieldName": "hello",
	}, now)
	if delRes.Code != http.StatusOK {
		t.Fatalf("DeleteResolver status=%d body=%q", delRes.Code, delRes.Body.String())
	}

	delDS := mustJSONTarget(t, handler, "AWSAppSync.DeleteDataSource", "appsync", map[string]any{
		"apiId": apiID,
		"name":  "OtherDS",
	}, now)
	if delDS.Code != http.StatusOK {
		t.Fatalf("DeleteDataSource status=%d body=%q", delDS.Code, delDS.Body.String())
	}
}
