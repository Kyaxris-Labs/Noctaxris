package docker_test

import (
	"os"
	"strings"
	"testing"
)

func TestComposeFileHasNoDockerSock(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	if strings.Contains(content, "/var/run/docker.sock") {
		t.Fatal("compose must not mount /var/run/docker.sock")
	}
	if hasDockerSockVolumeEntry(content) {
		t.Fatal("compose must not bind or volume-mount docker.sock")
	}
}

func TestComposePublishesLocalhostOnly(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "127.0.0.1:4566") {
		t.Fatal("compose must publish 127.0.0.1:4566")
	}
}

func TestComposeEngineHasNoHostPortPublish(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	engine := serviceBlock(string(b), "noctaxris-engine")
	if engine == "" {
		t.Fatal("compose must define noctaxris-engine")
	}
	for _, line := range strings.Split(engine, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "ports:") {
			t.Fatal("noctaxris-engine must not publish ports to the host")
		}
		if strings.Contains(trimmed, "2375:") || strings.HasSuffix(trimmed, ":2375") {
			t.Fatal("noctaxris-engine must not map host port 2375")
		}
		if strings.Contains(trimmed, "2376:") || strings.HasSuffix(trimmed, ":2376") {
			t.Fatal("noctaxris-engine must not map host port 2376")
		}
	}
}

func TestComposeSetsDockerHost(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	noctaxris := serviceBlock(string(b), "noctaxris")
	if !strings.Contains(noctaxris, "NOCTAXRIS_DOCKER_HOST") {
		t.Fatal("noctaxris must set NOCTAXRIS_DOCKER_HOST")
	}
	if !strings.Contains(noctaxris, "tcp://noctaxris-engine:2376") {
		t.Fatal("NOCTAXRIS_DOCKER_HOST must point at noctaxris-engine:2376 (TLS)")
	}
	if strings.Contains(noctaxris, ":2375") {
		t.Fatal("NOCTAXRIS_DOCKER_HOST must not use plain TCP port 2375")
	}
	if !strings.Contains(noctaxris, "NOCTAXRIS_DOCKER_CERT_PATH") {
		t.Fatal("noctaxris must set NOCTAXRIS_DOCKER_CERT_PATH for engine TLS")
	}
}

func TestComposeEngineTLSNotDisabled(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	engine := serviceBlock(string(b), "noctaxris-engine")
	if engine == "" {
		t.Fatal("compose must define noctaxris-engine")
	}
	if strings.Contains(engine, `DOCKER_TLS_CERTDIR: ""`) {
		t.Fatal("noctaxris-engine must not disable TLS (DOCKER_TLS_CERTDIR must not be empty)")
	}
}

func TestComposeSharesEngineCertsVolume(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	noctaxris := serviceBlock(content, "noctaxris")
	engine := serviceBlock(content, "noctaxris-engine")
	if !strings.Contains(noctaxris, "noctaxris-engine-certs") {
		t.Fatal("noctaxris must mount noctaxris-engine-certs for TLS client PEMs")
	}
	if !strings.Contains(engine, "noctaxris-engine-certs") {
		t.Fatal("noctaxris-engine must mount noctaxris-engine-certs for TLS cert generation")
	}
	if !strings.Contains(content, "noctaxris-engine-certs:") {
		t.Fatal("compose must declare noctaxris-engine-certs volume")
	}
}

// hasDockerSockVolumeEntry reports a non-comment YAML volume list item that
// mounts a path ending in docker.sock or uses a docker.sock: host bind.
// English comments such as "# DO NOT add docker.sock" are allowed.
func hasDockerSockVolumeEntry(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if idx := strings.Index(trimmed, " #"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		if !strings.HasPrefix(trimmed, "-") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		if strings.HasSuffix(rest, "docker.sock") || strings.Contains(rest, "docker.sock:") {
			return true
		}
	}
	return false
}

// serviceBlock returns the indented body of a top-level Compose service.
func serviceBlock(content, name string) string {
	lines := strings.Split(content, "\n")
	start := -1
	want := "  " + name + ":"
	for i, line := range lines {
		if strings.TrimSuffix(line, "\r") == want {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	var b strings.Builder
	for _, raw := range lines[start:] {
		line := strings.TrimSuffix(raw, "\r")
		if line != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
				break
			}
			if line[0] != ' ' && line[0] != '\t' {
				break
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
