package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestFunctionURLNoneDeniesWhenResourcePolicyLacksPublicAllow(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	roleARN, err := st.CreateRole(testAccountID, "fn-url-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "url-none-deny",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", create.Code, create.Body.String())
	}
	urlCfg := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "url-none-deny",
		"AuthType":     "NONE",
	}, now)
	if urlCfg.Code != http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig status=%d body=%q", urlCfg.Code, urlCfg.Body.String())
	}

	// Policy present but only allows a named principal → anonymous NONE invoke denied.
	userARN := "arn:aws:iam::" + testAccountID + ":user/alice"
	if _, err := st.AddFunctionPermissionWithOpts(testAccountID, "url-none-deny", store.AddFunctionPermissionOpts{
		StatementID:         "NamedOnly",
		Action:              "lambda:InvokeFunctionUrl",
		Principal:           userARN,
		FunctionUrlAuthType: "NONE",
	}); err != nil {
		t.Fatal(err)
	}

	path := "/lambda-url/" + testAccountID + "/url-none-deny"
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566"+path, strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("NONE invoke without public Allow status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
}

func TestFunctionURLNoneDeniesWhenResourcePolicyEmpty(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	roleARN, err := st.CreateRole(testAccountID, "fn-url-empty-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "url-none-empty",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", create.Code, create.Body.String())
	}
	urlCfg := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "url-none-empty",
		"AuthType":     "NONE",
	}, now)
	if urlCfg.Code != http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig status=%d body=%q", urlCfg.Code, urlCfg.Body.String())
	}

	path := "/lambda-url/" + testAccountID + "/url-none-empty"
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566"+path, strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("NONE invoke with empty resource policy status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
}

func TestFunctionURLNoneDeniesWhenResourcePolicyExplicitDeny(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	roleARN, err := st.CreateRole(testAccountID, "fn-url-deny-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "url-none-explicit-deny",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", create.Code, create.Body.String())
	}
	urlCfg := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "url-none-explicit-deny",
		"AuthType":     "NONE",
	}, now)
	if urlCfg.Code != http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig status=%d body=%q", urlCfg.Code, urlCfg.Body.String())
	}
	if _, err := st.AddFunctionPermissionWithOpts(testAccountID, "url-none-explicit-deny", store.AddFunctionPermissionOpts{
		StatementID:         "PublicURL",
		Action:              "lambda:InvokeFunctionUrl",
		Principal:           "*",
		FunctionUrlAuthType: "NONE",
	}); err != nil {
		t.Fatal(err)
	}
	fn, err := st.GetFunction(testAccountID, "url-none-explicit-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyDoc := `{"Version":"2012-10-17","Statement":[` +
		`{"Sid":"PublicURL","Effect":"Allow","Principal":"*","Action":"lambda:InvokeFunctionUrl","Resource":"` + fn.FunctionARN + `","Condition":{"StringEquals":{"lambda:FunctionUrlAuthType":"NONE"}}},` +
		`{"Sid":"BlockPublic","Effect":"Deny","Principal":"*","Action":"lambda:InvokeFunctionUrl","Resource":"` + fn.FunctionARN + `"}` +
		`]}`
	if err := st.UnsafeSetFunctionResourcePolicyForTest(testAccountID, "url-none-explicit-deny", denyDoc); err != nil {
		t.Fatal(err)
	}

	path := "/lambda-url/" + testAccountID + "/url-none-explicit-deny"
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566"+path, strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("NONE invoke with explicit Deny status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
}

func TestFunctionURLNoneAllowsWhenResourcePolicyAllowsPublic(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	roleARN, err := st.CreateRole(testAccountID, "fn-url-allow-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	create := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "url-none-allow",
		"Runtime":      "nodejs22.x",
		"Role":         roleARN,
		"Handler":      "index.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", create.Code, create.Body.String())
	}
	urlCfg := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "url-none-allow",
		"AuthType":     "NONE",
	}, now)
	if urlCfg.Code != http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig status=%d body=%q", urlCfg.Code, urlCfg.Body.String())
	}
	if _, err := st.AddFunctionPermissionWithOpts(testAccountID, "url-none-allow", store.AddFunctionPermissionOpts{
		StatementID:         "PublicURL",
		Action:              "lambda:InvokeFunctionUrl",
		Principal:           "*",
		FunctionUrlAuthType: "NONE",
	}); err != nil {
		t.Fatal(err)
	}

	path := "/lambda-url/" + testAccountID + "/url-none-allow"
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566"+path, strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("NONE invoke with public Allow must not be 403 body=%q", rec.Body.String())
	}
}
