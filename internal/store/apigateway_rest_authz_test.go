package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRestAuthorizerApiKeyUsagePlan(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const acct = "000000000001"
	api, err := st.CreateRestAPI(acct, "authz-api", "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := st.CreateRestResource(acct, api.APIID, api.RootResourceID, "secure")
	if err != nil {
		t.Fatal(err)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + acct + ":function:authz-fn"
	authz, err := st.CreateRestAuthorizer(store.CreateRestAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "tok", Type: store.APIGatewayAuthorizerTOKEN,
		AuthorizerURI: lambdaARN, IdentitySource: "method.request.header.Authorization",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRestAuthorizer(store.CreateRestAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "bad", Type: "JWT",
		AuthorizerURI: lambdaARN,
	}); err == nil {
		t.Fatal("expected JWT reject")
	}
	_, err = st.PutRestMethod(acct, api.APIID, res.ResourceID, "GET", store.APIGatewayRESTAuthTOKEN, authz.AuthorizerID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRestMethod(acct, api.APIID, res.ResourceID, "POST", store.APIGatewayRESTAuthTOKEN, "", false); err == nil {
		t.Fatal("expected authorizerId required")
	}

	if _, err := st.CreateRestDeployment(acct, api.APIID, "v1", "dev"); err != nil {
		t.Fatal(err)
	}
	apiKey, err := st.CreateRestAPIKey(acct, "lab-key", true)
	if err != nil || apiKey.Value == "" {
		t.Fatalf("CreateRestAPIKey: %+v err=%v", apiKey, err)
	}
	got, err := st.GetRestAPIKey(acct, apiKey.ID)
	if err != nil || got.Value != "" {
		t.Fatalf("GetRestAPIKey should omit plaintext, got=%+v err=%v", got, err)
	}
	plan, err := st.CreateRestUsagePlan(acct, "plan", "", []store.RestUsagePlanStage{
		{APIID: api.APIID, Stage: "dev"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRestUsagePlanKey(acct, plan.ID, apiKey.ID); err != nil {
		t.Fatal(err)
	}
	if !st.ValidateRestAPIKeyForStage(acct, api.APIID, "dev", apiKey.Value) {
		t.Fatal("expected valid key for stage")
	}
	if st.ValidateRestAPIKeyForStage(acct, api.APIID, "dev", "nope") {
		t.Fatal("expected invalid key reject")
	}
	has, err := st.RestStageHasUsagePlan(acct, api.APIID, "dev")
	if err != nil || !has {
		t.Fatalf("expected usage plan on stage, has=%v err=%v", has, err)
	}
	listed, err := st.ListRestAuthorizers(acct, api.APIID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("authorizers=%v err=%v", listed, err)
	}
	if err := st.DeleteRestAuthorizer(acct, api.APIID, authz.AuthorizerID); err != nil {
		t.Fatal(err)
	}
}

func TestRestAuthzCRUDDeletesAndParseStages(t *testing.T) {
	st := openTestStore(t)
	const acct = "000000000001"
	api, err := st.CreateRestAPI(acct, "authz-cov", "")
	if err != nil {
		t.Fatal(err)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + acct + ":function:authz"
	authz, err := st.CreateRestAuthorizer(store.CreateRestAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "tok", Type: store.APIGatewayAuthorizerTOKEN,
		AuthorizerURI: lambdaARN, IdentitySource: "method.request.header.Authorization",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRestAuthorizer(acct, api.APIID, authz.AuthorizerID)
	if err != nil || got.Name != "tok" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	if _, err := st.GetRestAuthorizer(acct, api.APIID, "missing"); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("missing authorizer: %v", err)
	}

	if _, err := st.CreateRestDeployment(acct, api.APIID, "v1", "dev"); err != nil {
		t.Fatal(err)
	}
	key, err := st.CreateRestAPIKey(acct, "k1", true)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := st.ListRestAPIKeys(acct)
	if err != nil || len(keys) < 1 {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
	if _, err := st.GetRestAPIKey(acct, "missing"); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("missing key: %v", err)
	}

	stages, err := store.ParseRestAPIStagesJSON([]any{
		map[string]any{"apiId": api.APIID, "stage": "dev"},
		map[string]any{"ApiId": api.APIID, "Stage": "prod"},
	})
	if err != nil || len(stages) != 2 {
		t.Fatalf("parse stages=%v err=%v", stages, err)
	}
	if _, err := store.ParseRestAPIStagesJSON("bad"); err == nil {
		t.Fatal("expected parse reject")
	}

	plan, err := st.CreateRestUsagePlan(acct, "plan-cov", "d", []store.RestUsagePlanStage{
		{APIID: api.APIID, Stage: "dev"},
	})
	if err != nil {
		t.Fatal(err)
	}
	gotPlan, err := st.GetRestUsagePlan(acct, plan.ID)
	if err != nil || gotPlan.Name != "plan-cov" || len(gotPlan.APIStages) != 1 {
		t.Fatalf("plan=%+v err=%v", gotPlan, err)
	}
	plans, err := st.ListRestUsagePlans(acct)
	if err != nil || len(plans) < 1 {
		t.Fatalf("plans=%v err=%v", plans, err)
	}
	if _, err := st.GetRestUsagePlan(acct, "missing"); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("missing plan: %v", err)
	}

	pk, err := st.CreateRestUsagePlanKey(acct, plan.ID, key.ID)
	if err != nil || pk.ID != key.ID {
		t.Fatalf("plan key=%+v err=%v", pk, err)
	}
	pkeys, err := st.ListRestUsagePlanKeys(acct, plan.ID)
	if err != nil || len(pkeys) != 1 {
		t.Fatalf("plan keys=%v err=%v", pkeys, err)
	}
	if !st.ValidateRestAPIKeyForStage(acct, api.APIID, "dev", key.Value) {
		t.Fatal("expected key valid")
	}
	if st.ValidateRestAPIKeyForStage(acct, api.APIID, "prod", key.Value) {
		t.Fatal("expected no plan on prod")
	}

	if err := st.DeleteRestUsagePlanKey(acct, plan.ID, key.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteRestUsagePlanKey(acct, plan.ID, key.ID); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("second plan key delete: %v", err)
	}
	if err := st.DeleteRestUsagePlan(acct, plan.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteRestAPIKey(acct, key.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteRestAPIKey(acct, key.ID); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("second key delete: %v", err)
	}
	if err := st.DeleteRestAuthorizer(acct, api.APIID, authz.AuthorizerID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteRestAuthorizer(acct, api.APIID, authz.AuthorizerID); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("second authorizer delete: %v", err)
	}

	if arn := store.RestAPIControlPlaneARN("us-east-1", api.APIID); arn == "" {
		t.Fatal("empty control plane arn")
	}
}

func TestRestAuthorizerREQUESTAndIdentityFailClosed(t *testing.T) {
	st := openTestStore(t)
	const acct = "000000000001"
	api, err := st.CreateRestAPI(acct, "authz-req", "")
	if err != nil {
		t.Fatal(err)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + acct + ":function:authz"
	if _, err := st.CreateRestAuthorizer(store.CreateRestAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "req", Type: store.APIGatewayAuthorizerREQUEST,
		AuthorizerURI: lambdaARN, IdentitySource: "method.request.header.Authorization",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRestAuthorizer(store.CreateRestAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "bad", Type: store.APIGatewayAuthorizerTOKEN,
		AuthorizerURI: "https://evil.example/x", IdentitySource: "method.request.header.Authorization",
	}); err == nil {
		t.Fatal("expected remote uri reject")
	}
	if _, err := st.CreateRestAuthorizer(store.CreateRestAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "bad2", Type: store.APIGatewayAuthorizerTOKEN,
		AuthorizerURI: lambdaARN, IdentitySource: "context.authorizer.claims.sub",
	}); err == nil {
		t.Fatal("expected identity source reject")
	}
}
