package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAppSyncManagementSAR(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	api, err := st.CreateAppSyncGraphqlAPI(account, "us-east-1", "mgmt-api", store.AppSyncAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}

	before, err := st.GetAppSyncSchemaCreationStatus(account, api.APIID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != store.AppSyncSchemaStatusNotApplicable {
		t.Fatalf("before status=%q", before.Status)
	}

	if err := st.StartAppSyncSchemaCreation(account, api.APIID, "type Query { hello: String }"); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetAppSyncSchemaCreationStatus(account, api.APIID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != store.AppSyncSchemaStatusSuccess {
		t.Fatalf("after status=%q", after.Status)
	}

	key, err := st.CreateAppSyncAPIKey(account, api.APIID, 0)
	if err != nil || !strings.HasPrefix(key.APIKey, "da2-") {
		t.Fatalf("create key: %v %#v", err, key)
	}
	listed, err := st.ListAppSyncAPIKeys(account, api.APIID)
	if err != nil || len(listed) != 1 || listed[0].ID != key.ID {
		t.Fatalf("list keys: %v %#v", err, listed)
	}
	if err := st.DeleteAppSyncAPIKey(account, api.APIID, key.APIKey); err != nil {
		t.Fatalf("delete by plaintext: %v", err)
	}
	listed, err = st.ListAppSyncAPIKeys(account, api.APIID)
	if err != nil || len(listed) != 0 {
		t.Fatalf("list after delete: %v %#v", err, listed)
	}

	ds, err := st.CreateAppSyncDataSource(account, api.APIID, "HelloDS", "AWS_LAMBDA",
		"arn:aws:lambda:us-east-1:"+account+":function:hello", "")
	if err != nil {
		t.Fatal(err)
	}
	gotDS, err := st.GetAppSyncDataSource(account, api.APIID, "HelloDS")
	if err != nil || gotDS.LambdaFunctionARN != ds.LambdaFunctionARN {
		t.Fatalf("get ds: %v %#v", err, gotDS)
	}
	updated, err := st.UpdateAppSyncDataSource(account, api.APIID, "HelloDS", "AWS_LAMBDA",
		"arn:aws:lambda:us-east-1:"+account+":function:hello2", "")
	if err != nil || !strings.Contains(updated.LambdaFunctionARN, "hello2") {
		t.Fatalf("update ds: %v %#v", err, updated)
	}
	sources, err := st.ListAppSyncDataSources(account, api.APIID)
	if err != nil || len(sources) != 1 {
		t.Fatalf("list ds: %v %#v", err, sources)
	}

	res, err := st.CreateAppSyncResolver(account, api.APIID, "Query", "hello", "HelloDS")
	if err != nil {
		t.Fatal(err)
	}
	gotRes, err := st.GetAppSyncResolver(account, api.APIID, "Query", "hello")
	if err != nil || gotRes.DataSourceName != res.DataSourceName {
		t.Fatalf("get resolver: %v %#v", err, gotRes)
	}
	if err := st.DeleteAppSyncDataSource(account, api.APIID, "HelloDS"); !errors.Is(err, store.ErrAppSyncBadRequest) {
		t.Fatalf("delete in-use ds want bad request, got %v", err)
	}

	_, err = st.CreateAppSyncDataSource(account, api.APIID, "OtherDS", "AWS_LAMBDA",
		"arn:aws:lambda:us-east-1:"+account+":function:other", "")
	if err != nil {
		t.Fatal(err)
	}
	moved, err := st.UpdateAppSyncResolver(account, api.APIID, "Query", "hello", "OtherDS")
	if err != nil || moved.DataSourceName != "OtherDS" {
		t.Fatalf("update resolver: %v %#v", err, moved)
	}
	resolvers, err := st.ListAppSyncResolvers(account, api.APIID, "Query")
	if err != nil || len(resolvers) != 1 || resolvers[0].FieldName != "hello" {
		t.Fatalf("list resolvers: %v %#v", err, resolvers)
	}
	if err := st.DeleteAppSyncResolver(account, api.APIID, "Query", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAppSyncDataSource(account, api.APIID, "HelloDS"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAppSyncDataSource(account, api.APIID, "OtherDS"); err != nil {
		t.Fatal(err)
	}
}
