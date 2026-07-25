package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestParseAppSyncQueryFieldsMulti(t *testing.T) {
	fields, err := store.ParseAppSyncQueryFields("{ hello world }")
	if err != nil || len(fields) != 2 || fields[0] != "hello" || fields[1] != "world" {
		t.Fatalf("multi: %v %#v", err, fields)
	}
	fields, err = store.ParseAppSyncQueryFields("query { hello world }")
	if err != nil || len(fields) != 2 {
		t.Fatalf("query prefix: %v %#v", err, fields)
	}
	one, err := store.ParseAppSyncQueryField("{ hello }")
	if err != nil || one != "hello" {
		t.Fatalf("single: %v %q", err, one)
	}
}

func TestAppSyncDataSourceServiceRoleArnPersisted(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	api, err := st.CreateAppSyncGraphqlAPI(account, "us-east-1", "role-api", store.AppSyncAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	roleARN := "arn:aws:iam::" + account + ":role/appsync-ds"
	ds, err := st.CreateAppSyncDataSource(account, api.APIID, "HelloDS", "AWS_LAMBDA",
		"arn:aws:lambda:us-east-1:"+account+":function:hello", roleARN)
	if err != nil {
		t.Fatal(err)
	}
	if ds.ServiceRoleArn != roleARN {
		t.Fatalf("create ServiceRoleArn=%q", ds.ServiceRoleArn)
	}
	got, err := st.GetAppSyncDataSource(account, api.APIID, "HelloDS")
	if err != nil || got.ServiceRoleArn != roleARN {
		t.Fatalf("get: %v %#v", err, got)
	}
	if !strings.Contains(got.LambdaFunctionARN, "function:hello") {
		t.Fatalf("lambda arn=%q", got.LambdaFunctionARN)
	}
}
