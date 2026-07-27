package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRestAPICreateResourceMethodMockMatch(t *testing.T) {
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
	api, err := st.CreateRestAPI(acct, "lab-rest", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if api.RootResourceID == "" {
		t.Fatal("missing root resource")
	}
	root, err := st.GetRestResource(acct, api.APIID, api.RootResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if root.Path != "/" {
		t.Fatalf("root path=%q", root.Path)
	}

	res, err := st.CreateRestResource(acct, api.APIID, api.RootResourceID, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "/hello" {
		t.Fatalf("path=%q", res.Path)
	}

	_, err = st.PutRestMethod(acct, api.APIID, res.ResourceID, "GET", "NONE", "", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.PutRestIntegration(acct, api.APIID, res.ResourceID, "GET", "MOCK", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRestIntegration(acct, api.APIID, res.ResourceID, "GET", "HTTP_PROXY", "https://example.com", "GET", "", nil); err == nil {
		t.Fatal("expected HTTP_PROXY reject")
	}
	t.Setenv(store.EnvAPIGatewayHTTPProxy, "1")
	t.Setenv(store.EnvAPIGatewayHTTPProxyAllowlist, "https://example.com")
	if _, err := st.PutRestIntegration(acct, api.APIID, res.ResourceID, "GET", "HTTP_PROXY", "https://example.com/ok", "GET", "", nil); err != nil {
		t.Fatalf("HTTP_PROXY allowlisted: %v", err)
	}

	dep, err := st.CreateRestDeployment(acct, api.APIID, "v1", "dev")
	if err != nil {
		t.Fatal(err)
	}
	stage, err := st.GetRestStage(acct, api.APIID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if stage.DeploymentID != dep.DeploymentID {
		t.Fatalf("stage deployment=%q want %q", stage.DeploymentID, dep.DeploymentID)
	}

	matched, method, err := st.MatchRestAPIRoute(acct, api.APIID, "GET", "/hello")
	if err != nil {
		t.Fatal(err)
	}
	if matched.ResourceID != res.ResourceID {
		t.Fatalf("matched resource=%q", matched.ResourceID)
	}
	if method.HTTPMethod != "GET" {
		t.Fatalf("method=%q", method.HTTPMethod)
	}

	got, err := st.GetRestAPI(acct, api.APIID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "lab-rest" {
		t.Fatalf("name=%q", got.Name)
	}
	list, err := st.ListRestAPIs(acct)
	if err != nil || len(list) < 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteRestAPI(acct, api.APIID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetRestAPI(acct, api.APIID); err != store.ErrAPIGatewayNotFound {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestRestAPIProxyPathMatchAndLambdaURI(t *testing.T) {
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
	api, err := st.CreateRestAPI(acct, "proxy-api", "")
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := st.CreateRestResource(acct, api.APIID, api.RootResourceID, "{proxy+}")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.PutRestMethod(acct, api.APIID, proxy.ResourceID, "ANY", "NONE", "", false)
	if err != nil {
		t.Fatal(err)
	}
	lambdaURI := "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/arn:aws:lambda:us-east-1:" + acct + ":function:hello/invocations"
	in, err := st.PutRestIntegration(acct, api.APIID, proxy.ResourceID, "ANY", "AWS_PROXY", lambdaURI, "POST", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "arn:aws:lambda:us-east-1:" + acct + ":function:hello"
	if in.URI != want {
		t.Fatalf("uri=%q want %q", in.URI, want)
	}
	matched, method, err := st.MatchRestAPIRoute(acct, api.APIID, "ANY", "/a/b/c")
	if err != nil {
		t.Fatal(err)
	}
	if matched.Path != "/{proxy+}" {
		t.Fatalf("path=%q", matched.Path)
	}
	if method.HTTPMethod != "ANY" {
		t.Fatalf("method=%q", method.HTTPMethod)
	}
}
