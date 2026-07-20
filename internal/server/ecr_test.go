package server_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustECRJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	return mustECRJSONWithCreds(t, handler, target, payload, testAccessKey, testSecret, now)
}

func mustECRJSONWithCreds(
	t *testing.T,
	handler http.Handler,
	target string,
	payload map[string]any,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonEC2ContainerRegistry_V20150921."+target)
	signHeader(t, req, raw, akid, secret, testRegion, "ecr", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestECRCreateRepositoryHappyPath(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustECRJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "noctaxris-lab",
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", rec.Code, rec.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	repo, ok := out["repository"].(map[string]any)
	if !ok {
		t.Fatalf("missing repository in %q", rec.Body.String())
	}
	name, _ := repo["repositoryName"].(string)
	arn, _ := repo["repositoryArn"].(string)
	uri, _ := repo["repositoryUri"].(string)
	if name != "noctaxris-lab" {
		t.Fatalf("repositoryName=%q want noctaxris-lab", name)
	}
	wantARN := "arn:aws:ecr:us-east-1:000000000001:repository/noctaxris-lab"
	if arn != wantARN {
		t.Fatalf("repositoryArn=%q want %q", arn, wantARN)
	}
	wantURI := "127.0.0.1:4566/000000000001/noctaxris-lab"
	if uri != wantURI {
		t.Fatalf("repositoryUri=%q want %q", uri, wantURI)
	}
}

func TestECRGetAuthorizationTokenShape(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustECRJSON(t, handler, "GetAuthorizationToken", map[string]any{}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetAuthorizationToken status=%d body=%q", rec.Code, rec.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	data, ok := out["authorizationData"].([]any)
	if !ok || len(data) == 0 {
		t.Fatalf("authorizationData missing in %q", rec.Body.String())
	}
	entry, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("authorizationData[0] not object: %q", rec.Body.String())
	}
	tokenB64, _ := entry["authorizationToken"].(string)
	if tokenB64 == "" {
		t.Fatalf("authorizationToken missing in %q", rec.Body.String())
	}
	if entry["expiresAt"] == nil {
		t.Fatalf("expiresAt missing in %q", rec.Body.String())
	}
	proxy, _ := entry["proxyEndpoint"].(string)
	if proxy != "http://127.0.0.1:4566" {
		t.Fatalf("proxyEndpoint=%q want http://127.0.0.1:4566", proxy)
	}

	raw, err := base64.StdEncoding.DecodeString(tokenB64)
	if err != nil {
		t.Fatalf("decode authorizationToken: %v", err)
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 || parts[0] != "AWS" || parts[1] == "" {
		t.Fatalf("decoded token=%q want AWS:<password>", string(raw))
	}
}

func TestECRRepositoryPolicyAloneGrantsDescribeRepositories(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustECRJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "shared-repo",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	_, guestARN, err := st.CreateUser(testAccountID, "ecr-guest")
	if err != nil {
		t.Fatal(err)
	}
	guestAKID, guestSecret, err := st.CreateUserAccessKey(testAccountID, "ecr-guest")
	if err != nil {
		t.Fatal(err)
	}

	allowGuest := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"ecr:DescribeRepositories","Resource":"*"}]}`, guestARN)
	setPolRec := mustECRJSON(t, handler, "SetRepositoryPolicy", map[string]any{
		"repositoryName": "shared-repo",
		"policyText":     allowGuest,
	}, now)
	if setPolRec.Code != http.StatusOK {
		t.Fatalf("SetRepositoryPolicy status=%d body=%q", setPolRec.Code, setPolRec.Body.String())
	}

	descRec := mustECRJSONWithCreds(t, handler, "DescribeRepositories", map[string]any{
		"repositoryNames": []string{"shared-repo"},
	}, guestAKID, guestSecret, now)
	if descRec.Code != http.StatusOK {
		t.Fatalf("DescribeRepositories via repository policy status=%d want 200 body=%q", descRec.Code, descRec.Body.String())
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRec.Body.Bytes(), &descOut); err != nil {
		t.Fatal(err)
	}
	repos, _ := descOut["repositories"].([]any)
	if len(repos) != 1 {
		t.Fatalf("repositories len=%d want 1 body=%q", len(repos), descRec.Body.String())
	}
}
