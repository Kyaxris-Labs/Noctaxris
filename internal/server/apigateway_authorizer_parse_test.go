package server

import (
	"net/http/httptest"
	"testing"
)

func TestParseHTTPAPIAuthorizerResponse(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"simple allow", `{"isAuthorized":true}`, true},
		{"simple deny", `{"isAuthorized":false}`, false},
		{"iam allow", `{"policyDocument":{"Statement":[{"Effect":"Allow"}]}}`, true},
		{"iam deny", `{"policyDocument":{"Statement":[{"Effect":"Deny"}]}}`, false},
		{"iam mixed", `{"policyDocument":{"Statement":[{"Effect":"Allow"},{"Effect":"Deny"}]}}`, false},
		{"garbage", `not-json`, false},
		{"empty", `{}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseHTTPAPIAuthorizerResponse([]byte(tc.body), true); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestWriteHTTPAPIProxyResponseStripsHopByHop(t *testing.T) {
	rec := httptest.NewRecorder()
	writeHTTPAPIProxyResponseOpts(rec, []byte(`{"statusCode":200,"headers":{"Connection":"keep-alive","Transfer-Encoding":"chunked","Set-Cookie":"a=1","X-Ok":"1"},"body":"hi"}`), false)
	if rec.Header().Get("Connection") != "" || rec.Header().Get("Transfer-Encoding") != "" || rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("headers not stripped: %v", rec.Header())
	}
	if rec.Header().Get("X-Ok") != "1" {
		t.Fatalf("X-Ok missing")
	}
	allow := httptest.NewRecorder()
	writeHTTPAPIProxyResponseOpts(allow, []byte(`{"statusCode":200,"headers":{"Set-Cookie":"a=1"},"body":"hi"}`), true)
	if allow.Header().Get("Set-Cookie") != "a=1" {
		t.Fatalf("Set-Cookie should pass when allowed")
	}
}
