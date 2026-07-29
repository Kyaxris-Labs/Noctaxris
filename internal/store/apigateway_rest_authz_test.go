package store_test

import (
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
