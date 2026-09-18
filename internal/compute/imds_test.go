package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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
	credID := uuid.NewString()
	if err := cli.registerECSIMDSCredentials(ctx, credID, body); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := cli.unregisterECSIMDSCredentials(ctx, credID); err != nil {
		t.Fatalf("unregister: %v", err)
	}
}

func TestMatchECSIMDSCredPath(t *testing.T) {
	id := uuid.MustParse("12345678-90ab-cdef-1234-567890abcdef").String()
	okID, ok := matchECSIMDSCredPath("/v2/credentials/" + id)
	if !ok || okID != id {
		t.Fatalf("exact path id=%q ok=%v", okID, ok)
	}
	if _, ok := matchECSIMDSCredPath("/v2/credentials/" + strings.ToUpper(id) + "?x=1"); !ok {
		t.Fatal("uppercase UUID with query should match")
	}
	deny := []string{
		"",
		"/",
		"/v2/credentials",
		"/v2/credentials/",
		"/v2/credentials/" + id + "/",
		"/v2/credentials/" + id + "/extra",
		"/v2/credentials/../" + id,
		"/v2/credentials/not-a-uuid",
		"/v2/credentials/00000000-0000-0000-0000-000000000000",
		"/latest/meta-data/",
	}
	for _, p := range deny {
		if got, ok := matchECSIMDSCredPath(p); ok {
			t.Fatalf("path %q matched id %q", p, got)
		}
	}
}

func TestNormalizeECSIMDSCredID(t *testing.T) {
	id := uuid.NewString()
	got, err := normalizeECSIMDSCredID(" " + strings.ToUpper(id) + " ")
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("got %q want %q", got, id)
	}
	if _, err := normalizeECSIMDSCredID(""); err == nil {
		t.Fatal("empty")
	}
	if _, err := normalizeECSIMDSCredID("../x"); err == nil {
		t.Fatal("traversal")
	}
	if _, err := normalizeECSIMDSCredID(uuid.Nil.String()); err == nil {
		t.Fatal("nil uuid")
	}
}

func TestECSIMDSCredIDFromEnv(t *testing.T) {
	id := uuid.NewString()
	env := ContainerCredentialsEnv(id)
	if got := ecsIMDSCredIDFromEnvMap(env); got != id {
		t.Fatalf("map id=%q want %q", got, id)
	}
	if got := ecsIMDSCredIDFromEnvList([]string{
		"AWS_CONTAINER_CREDENTIALS_FULL_URI=" + env["AWS_CONTAINER_CREDENTIALS_FULL_URI"],
	}); got != id {
		t.Fatalf("full-uri only id=%q", got)
	}
	if got := ecsIMDSCredIDFromEnvList(nil); got != "" {
		t.Fatalf("empty env id=%q", got)
	}
}

func TestECSIMDSSidecarCmdFailsClosed(t *testing.T) {
	cmd := ecsIMDSSidecarCmd()
	if strings.Contains(cmd, "python -m http.server") {
		t.Fatal("sidecar must not use python -m http.server directory listing")
	}
	if strings.Contains(cmd, "SimpleHTTPRequestHandler") {
		t.Fatal("sidecar must not use SimpleHTTPRequestHandler")
	}
	if !strings.Contains(cmd, "BaseHTTPRequestHandler") {
		t.Fatal("expected exact-path BaseHTTPRequestHandler")
	}
	if !strings.Contains(cmd, ecsIMDSHandlerScript) {
		t.Fatal("expected handler script path")
	}
	if !strings.Contains(cmd, "send_error(404)") {
		t.Fatal("expected 404 deny")
	}
	if !ecsIMDSSidecarServesExactPaths([]string{"/bin/sh", "-c", cmd}) {
		t.Fatal("new sidecar cmd should count as exact-path")
	}
	if ecsIMDSSidecarServesExactPaths([]string{
		"/bin/sh", "-c",
		"python -m http.server 9254 --bind 0.0.0.0 --directory /www",
	}) {
		t.Fatal("directory listing cmd must not count as exact-path")
	}
}

func lookPathPython() (string, bool) {
	for _, name := range []string{"python3", "python"} {
		p, err := exec.LookPath(name)
		if err == nil {
			return p, true
		}
	}
	return "", false
}

func TestECSIMDSHandlerPythonRejectsDirectoryListing(t *testing.T) {
	py, ok := lookPathPython()
	if !ok {
		t.Skip("python not on PATH; skip sidecar handler listing test")
	}
	root := t.TempDir()
	credDir := filepath.Join(root, "v2", "credentials")
	if err := os.MkdirAll(credDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	body := []byte(`{"AccessKeyId":"ASIA1","SecretAccessKey":"secret"}`)
	if err := os.WriteFile(filepath.Join(credDir, id), body, 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	script := filepath.Join(t.TempDir(), "noctaxris-ecs-imds.py")
	src := ecsIMDSHandlerPython(filepath.ToSlash(root), port, "127.0.0.1")
	if err := os.WriteFile(script, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(py, script)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Skipf("python start: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(8 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/v2/credentials/" + id)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				lastErr = nil
				break
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("handler never served exact UUID: %v", lastErr)
	}

	listing, err := http.Get(base + "/v2/credentials/")
	if err != nil {
		t.Fatalf("GET directory: %v", err)
	}
	listed, _ := io.ReadAll(listing.Body)
	_ = listing.Body.Close()
	if listing.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /v2/credentials/ status=%d want 404 body=%s", listing.StatusCode, listed)
	}
	if strings.Contains(string(listed), id) {
		t.Fatalf("directory response leaked credential id: %s", listed)
	}

	bare, err := http.Get(base + "/v2/credentials")
	if err != nil {
		t.Fatalf("GET /v2/credentials: %v", err)
	}
	_, _ = io.Copy(io.Discard, bare.Body)
	_ = bare.Body.Close()
	if bare.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /v2/credentials status=%d want 404", bare.StatusCode)
	}

	missing, err := http.Get(base + "/v2/credentials/" + uuid.NewString())
	if err != nil {
		t.Fatalf("GET missing: %v", err)
	}
	_, _ = io.Copy(io.Discard, missing.Body)
	_ = missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("GET missing UUID status=%d want 404", missing.StatusCode)
	}

	got, err := http.Get(base + "/v2/credentials/" + id)
	if err != nil {
		t.Fatalf("GET exact: %v", err)
	}
	payload, _ := io.ReadAll(got.Body)
	_ = got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("GET exact UUID status=%d", got.StatusCode)
	}
	if !strings.Contains(string(payload), "ASIA1") {
		t.Fatalf("exact GET body=%s", payload)
	}
}
