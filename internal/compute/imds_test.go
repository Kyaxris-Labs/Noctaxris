package compute

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFormatContainerCredentialsJSONShape(t *testing.T) {
	exp := time.Date(2026, 7, 26, 15, 4, 5, 0, time.UTC)
	raw, err := FormatContainerCredentialsJSON(ContainerCredentials{
		AccessKeyID:     "ASIAEXAMPLEKEY01",
		SecretAccessKey: "secret/value+1",
		Token:           "sessionToken==",
		Expiration:      exp,
		RoleArn:         "arn:aws:iam::123456789012:role/LabTask",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json: %v body=%s", err, raw)
	}
	want := map[string]string{
		"AccessKeyId":     "ASIAEXAMPLEKEY01",
		"SecretAccessKey": "secret/value+1",
		"Token":           "sessionToken==",
		"Expiration":      "2026-07-26T15:04:05Z",
		"RoleArn":         "arn:aws:iam::123456789012:role/LabTask",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s=%v want %q (body=%s)", k, got[k], v, raw)
		}
	}
}

func TestFormatContainerCredentialsJSONRequiresKeys(t *testing.T) {
	if _, err := FormatContainerCredentialsJSON(ContainerCredentials{AccessKeyID: "AKIA"}); err == nil {
		t.Fatal("expected error without secret")
	}
}

func TestFormatContainerCredentialsJSONDefaultExpiration(t *testing.T) {
	before := time.Now().UTC()
	raw, err := FormatContainerCredentialsJSON(ContainerCredentials{
		AccessKeyID:     "ASIAEXAMPLEKEY01",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got containerCredentialsJSON
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	exp, err := time.Parse(time.RFC3339, got.Expiration)
	if err != nil {
		t.Fatal(err)
	}
	if exp.Before(before.Add(50*time.Minute)) || exp.After(before.Add(70*time.Minute)) {
		t.Fatalf("Expiration=%s want ~1h from now", got.Expiration)
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := asMap["RoleArn"]; ok {
		t.Fatalf("RoleArn should be omitted when empty: %s", raw)
	}
}

func TestContainerCredentialsEnv(t *testing.T) {
	env := ContainerCredentialsEnv("abc-123")
	rel := env["AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"]
	full := env["AWS_CONTAINER_CREDENTIALS_FULL_URI"]
	if rel != "/v2/credentials/abc-123" {
		t.Fatalf("RELATIVE_URI=%q", rel)
	}
	wantFull := "http://169.254.170.2:9254/v2/credentials/abc-123"
	if full != wantFull {
		t.Fatalf("FULL_URI=%q want %q", full, wantFull)
	}
	if ContainerCredentialsEnv("  ") != nil {
		t.Fatal("empty id should yield nil")
	}
}

func TestRoleCredsFromEnv(t *testing.T) {
	_, _, _, ok := roleCredsFromEnv(nil)
	if ok {
		t.Fatal("nil env")
	}
	_, _, _, ok = roleCredsFromEnv(map[string]string{"AWS_ACCESS_KEY_ID": "AKIA"})
	if ok {
		t.Fatal("missing secret")
	}
	ak, sk, tok, ok := roleCredsFromEnv(map[string]string{
		"AWS_ACCESS_KEY_ID":     " ASIA1 ",
		"AWS_SECRET_ACCESS_KEY": " sec ",
		"AWS_SESSION_TOKEN":     " tok ",
	})
	if !ok || ak != "ASIA1" || sk != "sec" || tok != "tok" {
		t.Fatalf("got %q %q %q ok=%v", ak, sk, tok, ok)
	}
}

func TestECSTaskHostConfigMergesIMDSExtraHosts(t *testing.T) {
	t.Setenv(EnvInjectECSHostGateway, "")
	hc := ecsTaskHostConfig(128, []string{ECSIMDSLinkLocal + ":10.0.0.9"})
	if len(hc.ExtraHosts) != 1 || hc.ExtraHosts[0] != "169.254.170.2:10.0.0.9" {
		t.Fatalf("ExtraHosts=%#v", hc.ExtraHosts)
	}
	t.Setenv(EnvInjectECSHostGateway, "1")
	hc = ecsTaskHostConfig(128, []string{ECSIMDSLinkLocal + ":10.0.0.9"})
	if len(hc.ExtraHosts) != 2 {
		t.Fatalf("ExtraHosts=%#v want host-gateway + imds", hc.ExtraHosts)
	}
}

func TestAttachECSIMDSEnvNoopWithoutCreds(t *testing.T) {
	c := &Client{}
	hosts, uri, err := c.attachECSIMDSEnv(context.Background(), map[string]string{"FOO": "bar"})
	if err != nil {
		t.Fatal(err)
	}
	if hosts != nil || uri != nil {
		t.Fatalf("hosts=%v uri=%v", hosts, uri)
	}
}

func TestEnsureECSIMDSMirrorLiveSkipsWithoutEngine(t *testing.T) {
	host := strings.TrimSpace(os.Getenv("NOCTAXRIS_DOCKER_HOST"))
	if host == "" {
		t.Skip("NOCTAXRIS_DOCKER_HOST unset; skip ecs imds live test")
	}
	certPath := strings.TrimSpace(os.Getenv("NOCTAXRIS_DOCKER_CERT_PATH"))
	cli, err := NewClient(host, certPath, "127.0.0.1:4566")
	if err != nil {
		t.Skipf("compute client unavailable: %v", err)
	}
	defer cli.Close()
	ctx := context.Background()
	ip, err := cli.ensureECSIMDSMirror(ctx)
	if err != nil {
		t.Fatalf("ensureECSIMDSMirror: %v", err)
	}
	if ip == "" {
		t.Fatal("expected imds ip on noctaxris-ecs")
	}
	body, err := FormatContainerCredentialsJSON(ContainerCredentials{
		AccessKeyID:     "ASIALIVEKEY01",
		SecretAccessKey: "livesecret",
		Token:           "livetoken",
		Expiration:      time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	credID := "live-test-cred"
	if err := cli.registerECSIMDSCredentials(ctx, credID, body); err != nil {
		t.Fatalf("register: %v", err)
	}
}
