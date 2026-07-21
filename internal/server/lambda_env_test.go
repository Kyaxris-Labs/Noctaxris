package server

import (
	"strings"
	"testing"
)

func TestValidateLambdaEnvKeysRejectsReserved(t *testing.T) {
	cases := []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_SECURITY_TOKEN",
		"AWS_REGION",
		"AWS_DEFAULT_REGION",
		"AWS_LAMBDA_FUNCTION_NAME",
		"AWS_ENDPOINT_URL",
		"AWS_ENDPOINT_URL_S3",
	}
	for _, key := range cases {
		err := validateLambdaEnvKeys(map[string]string{key: "x", "OK": "1"})
		if err == nil {
			t.Fatalf("expected reject for %s", key)
		}
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error for %s = %v", key, err)
		}
	}
}

func TestValidateLambdaEnvKeysAllowsCustom(t *testing.T) {
	if err := validateLambdaEnvKeys(map[string]string{"STAGE": "lab", "BUCKET": "b"}); err != nil {
		t.Fatal(err)
	}
}

func TestMergeLambdaInvokeEnvOverlaysMintedLast(t *testing.T) {
	fnEnv := map[string]string{
		"STAGE":             "lab",
		"AWS_ACCESS_KEY_ID": "AKIACLIENT",
		"AWS_ENDPOINT_URL":  "http://evil.example",
	}
	minted := map[string]string{
		"AWS_ACCESS_KEY_ID":     "ASIAMINTED",
		"AWS_SECRET_ACCESS_KEY": "secret",
		"AWS_SESSION_TOKEN":     "token",
		"AWS_ENDPOINT_URL":      "http://host.docker.internal:4566",
	}
	got := mergeLambdaInvokeEnv(fnEnv, minted)
	if got["STAGE"] != "lab" {
		t.Fatalf("STAGE=%q", got["STAGE"])
	}
	if got["AWS_ACCESS_KEY_ID"] != "ASIAMINTED" {
		t.Fatalf("AWS_ACCESS_KEY_ID=%q want minted", got["AWS_ACCESS_KEY_ID"])
	}
	if got["AWS_ENDPOINT_URL"] != "http://host.docker.internal:4566" {
		t.Fatalf("AWS_ENDPOINT_URL=%q want lab endpoint", got["AWS_ENDPOINT_URL"])
	}
}

func TestClampLambdaTimeoutAndMemory(t *testing.T) {
	if got := clampLambdaTimeout(0); got != minLambdaTimeoutSec {
		t.Fatalf("timeout 0 -> %d", got)
	}
	if got := clampLambdaTimeout(1000); got != maxLambdaTimeoutSec {
		t.Fatalf("timeout 1000 -> %d", got)
	}
	if got := clampLambdaTimeout(42); got != 42 {
		t.Fatalf("timeout 42 -> %d", got)
	}
	if got := clampLambdaMemory(64); got != minLambdaMemoryMB {
		t.Fatalf("memory 64 -> %d", got)
	}
	if got := clampLambdaMemory(20000); got != maxLambdaMemoryMB {
		t.Fatalf("memory 20000 -> %d", got)
	}
	if got := clampLambdaMemory(512); got != 512 {
		t.Fatalf("memory 512 -> %d", got)
	}
}
