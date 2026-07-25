package server_test

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLambdaInvokeErrorOpaqueNoPathLeak(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	leakPath := filepath.Join(string(filepath.Separator)+"tmp", "noctaxris-data", "lambda", "000000000001", "opaque-fn", "invoke-events")
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, _ string, _ store.LambdaFunction, _, _ string) ([]byte, error) {
		return nil, errors.New("prepare image invoke event dir: mkdir " + leakPath + ": permission denied")
	})

	mustCreateIAMRole(t, handler, "lambda-opaque-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-opaque-exec"
	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "opaque-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	invokeRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName": "opaque-fn",
		"Payload":      `{"ping":true}`,
	}, now)
	if invokeRec.Code != http.StatusInternalServerError {
		t.Fatalf("Invoke status=%d want 500 body=%q", invokeRec.Code, invokeRec.Body.String())
	}
	body := invokeRec.Body.String()
	if strings.Contains(body, leakPath) || strings.Contains(body, "noctaxris-data") || strings.Contains(body, "permission denied") {
		t.Fatalf("client error leaked path/detail: %q", body)
	}
	if !strings.Contains(body, "Invoke failed.") {
		t.Fatalf("expected opaque Invoke failed message in %q", body)
	}
	if strings.Contains(body, "Invoke failed:") {
		t.Fatalf("expected no raw err suffix in %q", body)
	}
}
