package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAPIGatewayV2CreateMatchRoute(t *testing.T) {
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
	api, err := st.CreateAPIGatewayAPI(acct, "us-east-1", "lab", "HTTP")
	if err != nil {
		t.Fatal(err)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + acct + ":function:hello"
	in, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "AWS_PROXY", lambdaARN, "2.0", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAPIGatewayRoute(acct, api.APIID, "GET /hello", "integrations/"+in.IntegrationID, "NONE", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAPIGatewayStage(acct, api.APIID, "$default", true)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := st.MatchAPIGatewayRoute(acct, api.APIID, "GET", "/hello")
	if err != nil {
		t.Fatal(err)
	}
	if matched.RouteKey != "GET /hello" {
		t.Fatalf("route=%q", matched.RouteKey)
	}
	if _, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "HTTP_PROXY", "https://example.com", "", ""); err == nil {
		t.Fatal("expected HTTP_PROXY reject")
	}
}

func TestAppSyncCognitoCreateRequiresPoolConfig(t *testing.T) {
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
	if _, err := st.CreateAppSyncGraphqlAPIWithConfig(acct, "us-east-1", "c1", store.AppSyncAuthCognito, store.AppSyncUserPoolConfig{}); err == nil {
		t.Fatal("expected missing pool config reject")
	}
	api, err := st.CreateAppSyncGraphqlAPIWithConfig(acct, "us-east-1", "c2", store.AppSyncAuthCognito, store.AppSyncUserPoolConfig{
		UserPoolID: "us-east-1_abc", ClientID: "client-1", AwsRegion: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if api.AuthenticationType != store.AppSyncAuthCognito {
		t.Fatalf("auth=%q", api.AuthenticationType)
	}
	if api.UserPoolIssuer != store.CognitoIssuerURL("us-east-1", "us-east-1_abc") {
		t.Fatalf("issuer=%q", api.UserPoolIssuer)
	}
}
