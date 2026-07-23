package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const secretsManagerTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":["lambda.amazonaws.com","secretsmanager.amazonaws.com"]},"Action":"sts:AssumeRole"}]}`

const secretsManagerTrustLambdaOnly = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

func TestRotateSecretLambdaPassRoleDeny(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "rot-deny-role", secretsManagerTrustLambdaOnly, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/rot-deny-role"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "rot-deny-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createFn.Code, createFn.Body.String())
	}
	var fnOut map[string]any
	if err := json.Unmarshal(createFn.Body.Bytes(), &fnOut); err != nil {
		t.Fatal(err)
	}
	fnARN, _ := fnOut["FunctionArn"].(string)
	if fnARN == "" {
		t.Fatalf("missing FunctionArn: %s", createFn.Body.String())
	}

	createSec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "rot-deny-secret",
		"SecretString": "before",
	}, now)
	if createSec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createSec.Code, createSec.Body.String())
	}

	rot := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{
		"SecretId":          "rot-deny-secret",
		"RotationLambdaARN": fnARN,
	}, now)
	if rot.Code != http.StatusForbidden {
		t.Fatalf("RotateSecret status=%d want 403 body=%q", rot.Code, rot.Body.String())
	}
	if !strings.Contains(rot.Body.String(), "AccessDeniedException") {
		t.Fatalf("body=%q", rot.Body.String())
	}
	got, err := st.GetSecretValue(testAccountID, "rot-deny-secret")
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretString != "before" {
		t.Fatalf("secret changed despite PassRole deny: %q", got.SecretString)
	}
}

func TestRotateSecretLambdaPassRoleAllowFinish(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	var seenSteps []string
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		if name != "rot-ok-fn" {
			t.Errorf("unexpected function %q", name)
		}
		var evt map[string]string
		if err := json.Unmarshal([]byte(eventJSON), &evt); err != nil {
			t.Errorf("event json: %v", err)
		}
		seenSteps = append(seenSteps, evt["Step"])
		if evt["ClientRequestToken"] != "client-tok-1" {
			t.Errorf("token=%q", evt["ClientRequestToken"])
		}
		return []byte(`{"ok":true}`), nil
	})

	mustCreateIAMRole(t, handler, "rot-ok-role", secretsManagerTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/rot-ok-role"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "rot-ok-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createFn.Code, createFn.Body.String())
	}
	var fnOut map[string]any
	if err := json.Unmarshal(createFn.Body.Bytes(), &fnOut); err != nil {
		t.Fatal(err)
	}
	fnARN, _ := fnOut["FunctionArn"].(string)

	createSec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "rot-ok-secret",
		"SecretString": "before",
	}, now)
	if createSec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createSec.Code, createSec.Body.String())
	}

	rot := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{
		"SecretId":           "rot-ok-secret",
		"RotationLambdaARN":  fnARN,
		"ClientRequestToken": "client-tok-1",
	}, now)
	if rot.Code != http.StatusOK {
		t.Fatalf("RotateSecret status=%d body=%q", rot.Code, rot.Body.String())
	}
	wantSteps := []string{
		store.SecretRotationStepCreateSecret,
		store.SecretRotationStepSetSecret,
		store.SecretRotationStepTestSecret,
		store.SecretRotationStepFinishSecret,
	}
	if strings.Join(seenSteps, ",") != strings.Join(wantSteps, ",") {
		t.Fatalf("steps=%v want %v", seenSteps, wantSteps)
	}
	job, err := st.LatestAsyncInvocation(testAccountID, "rot-ok-fn")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "succeeded" {
		t.Fatalf("async job status=%q want succeeded", job.Status)
	}
	got, err := st.GetSecretValue(testAccountID, "rot-ok-secret")
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretString == "" || got.SecretString == "before" {
		t.Fatalf("finishSecret did not rotate value: %+v", got)
	}
	desc := mustSecretsJSON(t, handler, "DescribeSecret", map[string]any{"SecretId": "rot-ok-secret"}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeSecret status=%d body=%q", desc.Code, desc.Body.String())
	}
	if !strings.Contains(desc.Body.String(), fnARN) {
		t.Fatalf("DescribeSecret missing RotationLambdaARN: %s", desc.Body.String())
	}
	if !strings.Contains(desc.Body.String(), "AWSCURRENT") || !strings.Contains(desc.Body.String(), "AWSPREVIOUS") {
		t.Fatalf("DescribeSecret missing version stages: %s", desc.Body.String())
	}
}

func TestRotateSecretWithoutLambdaStillRandom(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createSec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "rot-random",
		"SecretString": "v1",
	}, now)
	if createSec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createSec.Code, createSec.Body.String())
	}
	rot := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{"SecretId": "rot-random"}, now)
	if rot.Code != http.StatusOK {
		t.Fatalf("RotateSecret status=%d body=%q", rot.Code, rot.Body.String())
	}
	got, err := st.GetSecretValue(testAccountID, "rot-random")
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretString == "" || got.SecretString == "v1" {
		t.Fatalf("random rotate failed: %+v", got)
	}
}

func TestRotateSecretLambdaInvokeFailureDoesNotFinish(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, _ string, _ store.LambdaFunction, _, _ string) ([]byte, error) {
		return nil, context.DeadlineExceeded
	})

	mustCreateIAMRole(t, handler, "rot-fail-role", secretsManagerTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/rot-fail-role"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "rot-fail-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createFn.Code, createFn.Body.String())
	}
	var fnOut map[string]any
	if err := json.Unmarshal(createFn.Body.Bytes(), &fnOut); err != nil {
		t.Fatal(err)
	}
	fnARN, _ := fnOut["FunctionArn"].(string)

	if mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name": "rot-fail-secret", "SecretString": "keep-me",
	}, now).Code != http.StatusOK {
		t.Fatal("CreateSecret failed")
	}
	rot := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{
		"SecretId": "rot-fail-secret", "RotationLambdaARN": fnARN,
	}, now)
	if rot.Code == http.StatusOK {
		t.Fatalf("want rotate failure, got 200 body=%q", rot.Body.String())
	}
	got, err := st.GetSecretValue(testAccountID, "rot-fail-secret")
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretString != "keep-me" {
		t.Fatalf("value changed after invoke failure: %q", got.SecretString)
	}
}
