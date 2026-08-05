package store_test

import (
	"errors"
	"path/filepath"
	"strings"
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
	if _, err := st.CreateAppSyncGraphqlAPIWithConfig(acct, "us-east-1", "evil", store.AppSyncAuthCognito, store.AppSyncUserPoolConfig{
		UserPoolID: "us-east-1_abc", ClientID: "client-1", AwsRegion: "us-east-1",
		Issuer: "https://evil.example/oidc",
	}); err == nil {
		t.Fatal("expected remote issuer reject")
	}
}

func TestAPIGatewayAuthorizerRejectsRemoteIssuer(t *testing.T) {
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
	if _, err := st.CreateAPIGatewayAuthorizer(store.CreateAPIGatewayAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "jwt", AuthorizerType: store.APIGatewayAuthorizerJWT,
		IdentitySource: "$request.header.Authorization", JWTIssuer: "https://evil.example/", JWTAudience: []string{"aud"},
	}); err == nil {
		t.Fatal("expected remote issuer reject")
	}
	issuer := store.CognitoIssuerURL("us-east-1", "us-east-1_lab")
	a, err := st.CreateAPIGatewayAuthorizer(store.CreateAPIGatewayAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "jwt-lab", AuthorizerType: store.APIGatewayAuthorizerJWT,
		IdentitySource: "$request.header.Authorization", JWTIssuer: issuer, JWTAudience: []string{"aud"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.JWTIssuer != issuer {
		t.Fatalf("issuer=%q", a.JWTIssuer)
	}
}

func TestAPIGatewayREQUESTAuthorizerAndRejectTOKEN(t *testing.T) {
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
	api, err := st.CreateAPIGatewayAPI(acct, "us-east-1", "lab-req", "HTTP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAPIGatewayAuthorizer(store.CreateAPIGatewayAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "tok", AuthorizerType: store.APIGatewayAuthorizerTOKEN,
		AuthorizerURI: "arn:aws:lambda:us-east-1:" + acct + ":function:authz",
	}); err == nil {
		t.Fatal("expected TOKEN reject")
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + acct + ":function:authz"
	a, err := st.CreateAPIGatewayAuthorizer(store.CreateAPIGatewayAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "req", AuthorizerType: store.APIGatewayAuthorizerREQUEST,
		AuthorizerURI: lambdaARN, AuthorizerPayloadFormatVersion: "2.0",
		IdentitySource: "$request.header.Authorization",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.AuthorizerType != store.APIGatewayAuthorizerREQUEST || a.AuthorizerURI != lambdaARN {
		t.Fatalf("authorizer=%+v", a)
	}
	in, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "AWS_PROXY",
		"arn:aws:lambda:us-east-1:"+acct+":function:hello", "2.0", "")
	if err != nil {
		t.Fatal(err)
	}
	route, err := st.CreateAPIGatewayRoute(acct, api.APIID, "GET /secure", "integrations/"+in.IntegrationID,
		store.APIGatewayAuthCUSTOM, a.AuthorizerID)
	if err != nil {
		t.Fatal(err)
	}
	if route.AuthorizationType != store.APIGatewayAuthCUSTOM {
		t.Fatalf("auth=%q", route.AuthorizationType)
	}
}

func TestAPIGatewayV2CORSCRUDAndNotFound(t *testing.T) {
	st := openTestStore(t)
	const acct = "000000000001"

	api, err := st.CreateAPIGatewayAPIWithCORS(acct, "us-east-1", "cors-api", "HTTP", store.APIGatewayCORS{
		AllowOrigins: []string{"https://lab.example"},
		AllowMethods: []string{"GET", "OPTIONS"},
		MaxAge:       300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !api.CORS.HasCORS() || api.CORS.MaxAge != 300 {
		t.Fatalf("cors=%+v", api.CORS)
	}
	if ep := store.APIGatewayAPIEndpoint(api.APIID); ep == "" {
		t.Fatal("empty endpoint")
	}

	got, err := st.GetAPIGatewayAPI(acct, api.APIID)
	if err != nil || got.Name != "cors-api" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	acct2, byID, err := st.GetAPIGatewayAPIByID(api.APIID)
	if err != nil || acct2 != acct || byID.APIID != api.APIID {
		t.Fatalf("byID acct=%s api=%+v err=%v", acct2, byID, err)
	}

	updated, err := st.UpdateAPIGatewayAPICORS(acct, api.APIID, store.APIGatewayCORS{
		AllowOrigins: []string{"https://other.example"},
		AllowMethods: []string{"POST"},
	})
	if err != nil || updated.CORS.AllowOrigins[0] != "https://other.example" {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	if _, err := st.UpdateAPIGatewayAPICORS(acct, api.APIID, store.APIGatewayCORS{
		AllowOrigins: []string{"*"}, AllowCredentials: true,
	}); !errors.Is(err, store.ErrAPIGatewayBadRequest) {
		t.Fatalf("credentials+*: %v", err)
	}

	list, err := st.ListAPIGatewayAPIs(acct)
	if err != nil || len(list) < 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}

	in, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "AWS_PROXY",
		"arn:aws:lambda:us-east-1:"+acct+":function:hello", "2.0", "")
	if err != nil {
		t.Fatal(err)
	}
	gotIn, err := st.GetAPIGatewayIntegration(acct, api.APIID, in.IntegrationID)
	if err != nil || gotIn.IntegrationID != in.IntegrationID {
		t.Fatalf("get integration=%+v err=%v", gotIn, err)
	}
	ins, err := st.ListAPIGatewayIntegrations(acct, api.APIID)
	if err != nil || len(ins) != 1 {
		t.Fatalf("list integrations=%v err=%v", ins, err)
	}

	route, err := st.CreateAPIGatewayRoute(acct, api.APIID, "GET /items", "integrations/"+in.IntegrationID, "NONE", "")
	if err != nil {
		t.Fatal(err)
	}
	routes, err := st.ListAPIGatewayRoutes(acct, api.APIID)
	if err != nil || len(routes) != 1 || routes[0].RouteID != route.RouteID {
		t.Fatalf("routes=%v err=%v", routes, err)
	}
	matched, err := st.MatchAPIGatewayRoute(acct, api.APIID, "GET", "/items")
	if err != nil || matched.RouteKey != "GET /items" {
		t.Fatalf("match=%+v err=%v", matched, err)
	}
	if _, err := st.MatchAPIGatewayRoute(acct, api.APIID, "POST", "/items"); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("miss match: %v", err)
	}

	stage, err := st.CreateAPIGatewayStage(acct, api.APIID, "prod", false)
	if err != nil {
		t.Fatal(err)
	}
	gotStage, err := st.GetAPIGatewayStage(acct, api.APIID, "prod")
	if err != nil || gotStage.StageName != stage.StageName {
		t.Fatalf("stage=%+v err=%v", gotStage, err)
	}
	if _, err := st.GetAPIGatewayStage(acct, api.APIID, "missing"); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("stage missing: %v", err)
	}

	if err := st.DeleteAPIGatewayAPI(acct, api.APIID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetAPIGatewayAPI(acct, api.APIID); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if _, err := st.GetAPIGatewayAPI(acct, "nope"); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("missing api: %v", err)
	}
	if _, _, err := st.GetAPIGatewayAPIByID("nope"); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("missing by id: %v", err)
	}
}

func TestAPIGatewayV2AuthorizerListGetAndRouteAuth(t *testing.T) {
	st := openTestStore(t)
	const acct = "000000000001"
	api, err := st.CreateAPIGatewayAPI(acct, "us-east-1", "auth-api", "HTTP")
	if err != nil {
		t.Fatal(err)
	}
	issuer := store.CognitoIssuerURL("us-east-1", "us-east-1_lab")
	a, err := st.CreateAPIGatewayAuthorizer(store.CreateAPIGatewayAuthorizerInput{
		AccountID: acct, APIID: api.APIID, Name: "jwt", AuthorizerType: store.APIGatewayAuthorizerJWT,
		IdentitySource: "$request.header.Authorization", JWTIssuer: issuer, JWTAudience: []string{"aud"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAPIGatewayAuthorizer(acct, api.APIID, a.AuthorizerID)
	if err != nil || got.Name != "jwt" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	listed, err := st.ListAPIGatewayAuthorizers(acct, api.APIID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	if _, err := st.GetAPIGatewayAuthorizer(acct, api.APIID, "missing"); !errors.Is(err, store.ErrAPIGatewayNotFound) {
		t.Fatalf("missing authorizer: %v", err)
	}

	in, err := st.CreateAPIGatewayIntegration(acct, api.APIID, "AWS_PROXY",
		"arn:aws:lambda:us-east-1:"+acct+":function:hello", "2.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAPIGatewayRoute(acct, api.APIID, "GET /jwt", "integrations/"+in.IntegrationID,
		store.APIGatewayAuthJWT, a.AuthorizerID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAPIGatewayRoute(acct, api.APIID, "GET /bad", "integrations/"+in.IntegrationID,
		store.APIGatewayAuthJWT, ""); err == nil {
		t.Fatal("expected authorizerId required")
	}
}

func TestParseAPIGatewayRouteKeyAndHelpers(t *testing.T) {
	method, path, ok := store.ParseAPIGatewayRouteKey("GET /hello")
	if !ok || method != "GET" || path != "/hello" {
		t.Fatalf("parse=%q %q %v", method, path, ok)
	}
	if _, _, ok := store.ParseAPIGatewayRouteKey("bad"); ok {
		t.Fatal("expected reject")
	}
	if _, _, ok := store.ParseAPIGatewayRouteKey("GET"); ok {
		t.Fatal("expected method-only reject")
	}
	arn := store.APIGatewayRouteARN("us-east-1", "000000000001", "api1", "$default", "GET", "/x")
	if !strings.Contains(arn, "/$default/GET/x") {
		t.Fatalf("arn=%q", arn)
	}
	if id := store.IntegrationIDFromTarget("integrations/abc"); id != "abc" {
		t.Fatalf("id=%q", id)
	}
	if id := store.IntegrationIDFromTarget("integrations/"); id != "" {
		t.Fatalf("empty id=%q", id)
	}
	uri, err := store.NormalizeAPIGatewayAuthorizerURI("arn:aws:lambda:us-east-1:000000000001:function:fn")
	if err != nil || uri == "" {
		t.Fatalf("uri=%q err=%v", uri, err)
	}
	if _, err := store.NormalizeAPIGatewayAuthorizerURI("https://evil.example/auth"); err == nil {
		t.Fatal("expected remote uri reject")
	}
}
