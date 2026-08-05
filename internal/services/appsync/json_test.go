package appsync_test

import (
	"encoding/json"
	"testing"

	appsyncsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/appsync"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAppSyncJSON(t *testing.T) {
	api := store.AppSyncAPI{
		APIID: "api-1", Name: "lab-api", ARN: "arn:aws:appsync:us-east-1:1:apis/api-1",
		AuthenticationType: "API_KEY",
	}
	if _, err := appsyncsvc.CreateGraphqlApiJSON(api); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.GetGraphqlApiJSON(api); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.ListGraphqlApisJSON([]store.AppSyncAPI{api}); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.DeleteGraphqlApiJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.StartSchemaCreationJSON(); err != nil {
		t.Fatal(err)
	}
	st := store.AppSyncSchemaCreationStatus{Status: "SUCCESS"}
	if _, err := appsyncsvc.GetSchemaCreationStatusJSON(st); err != nil {
		t.Fatal(err)
	}

	key := store.AppSyncAPIKey{APIID: api.APIID, ID: "key-1", APIKey: "da2-lab"}
	if _, err := appsyncsvc.CreateApiKeyJSON(key); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.ListApiKeysJSON([]store.AppSyncAPIKey{key}); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.DeleteApiKeyJSON(); err != nil {
		t.Fatal(err)
	}

	ds := store.AppSyncDataSource{
		APIID: api.APIID, Name: "ds1", Type: "AWS_LAMBDA",
		LambdaFunctionARN: "arn:aws:lambda:us-east-1:1:function:fn",
	}
	if _, err := appsyncsvc.CreateDataSourceJSON(ds); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.GetDataSourceJSON(ds); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.UpdateDataSourceJSON(ds); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.ListDataSourcesJSON([]store.AppSyncDataSource{ds}); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.DeleteDataSourceJSON(); err != nil {
		t.Fatal(err)
	}

	res := store.AppSyncResolver{APIID: api.APIID, TypeName: "Query", FieldName: "ping", DataSourceName: ds.Name}
	if _, err := appsyncsvc.CreateResolverJSON(res); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.GetResolverJSON(res); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.UpdateResolverJSON(res); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.ListResolversJSON([]store.AppSyncResolver{res}); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.DeleteResolverJSON(); err != nil {
		t.Fatal(err)
	}

	gql, err := appsyncsvc.GraphQLDataJSON("ping", "pong")
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(gql, &data); err != nil {
		t.Fatal(err)
	}
	if _, err := appsyncsvc.GraphQLErrorsJSON("bad request"); err != nil {
		t.Fatal(err)
	}
}
