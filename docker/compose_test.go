package docker_test

import (
	"os"
	"strings"
	"testing"
)

func TestSmokeNestedScriptPresent(t *testing.T) {
	b, err := os.ReadFile("smoke-nested.sh")
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	for _, needle := range []string{
		"compose.yaml",
		"/_noctaxris/ready",
		"noctaxris-engine",
		"healthy",
		"create-db-instance",
		"execute-statement",
		"nested-psql",
		"parameters",
		"lambda invoke",
		"package-type",
		"run-task",
		"AKIAROOTEXAMPLE01",
	} {
		if !strings.Contains(strings.ToLower(content), strings.ToLower(needle)) {
			t.Fatalf("smoke-nested.sh missing %q", needle)
		}
	}
}

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
	// Default bind is loopback; NOCTAXRIS_PUBLISH_ADDR may override for host-gateway labs.
	if !strings.Contains(string(b), "${NOCTAXRIS_PUBLISH_ADDR:-127.0.0.1}:4566:4566") {
		t.Fatal("compose must default-publish 127.0.0.1:4566 via NOCTAXRIS_PUBLISH_ADDR")
	}
	if strings.Contains(string(b), `"0.0.0.0:4566:4566"`) || strings.Contains(string(b), `- "4566:4566"`) {
		t.Fatal("default compose must not hardcode non-loopback host publish")
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

func TestComposeDoesNotDefaultOpenDataPlane(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	noctaxris := serviceBlock(string(b), "noctaxris")
	for _, line := range strings.Split(noctaxris, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if idx := strings.Index(trimmed, " #"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		if strings.Contains(trimmed, "NOCTAXRIS_ALLOW_OPEN_DATA_PLANE") {
			t.Fatal("default Compose must not set NOCTAXRIS_ALLOW_OPEN_DATA_PLANE (opt-in for NONE labs only)")
		}
		if strings.Contains(trimmed, "NOCTAXRIS_ALLOW_ANONYMOUS_S3") {
			t.Fatal("default Compose must not set NOCTAXRIS_ALLOW_ANONYMOUS_S3 (opt-in anonymous GetObject only)")
		}
	}
	if !strings.Contains(noctaxris, "NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN") {
		t.Fatal("noctaxris must set NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN for 0.0.0.0 container bind")
	}
	overlay, err := os.ReadFile("compose.lab-open.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(overlay), `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE: "1"`) {
		t.Fatal("compose.lab-open.yaml must opt in NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1")
	}
	hg, err := os.ReadFile("compose.lab-host-gateway.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hg), `NOCTAXRIS_INJECT_HOST_GATEWAY: "1"`) {
		t.Fatal("compose.lab-host-gateway.yaml must opt in NOCTAXRIS_INJECT_HOST_GATEWAY=1")
	}
	if strings.Contains(noctaxris, `NOCTAXRIS_INJECT_HOST_GATEWAY: "1"`) {
		t.Fatal("default Compose must not hardcode NOCTAXRIS_INJECT_HOST_GATEWAY=1")
	}
	ecsHG, err := os.ReadFile("compose.lab-ecs-host-gateway.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ecsHG), `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY: "1"`) {
		t.Fatal("compose.lab-ecs-host-gateway.yaml must opt in NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1")
	}
	if strings.Contains(noctaxris, `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY: "1"`) {
		t.Fatal("default Compose must not hardcode NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1")
	}
	if !strings.Contains(noctaxris, `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY: "${NOCTAXRIS_INJECT_ECS_HOST_GATEWAY:-0}"`) {
		t.Fatal("default Compose must pass ECS host-gateway env defaulting to 0")
	}
	nestedPorts, err := os.ReadFile("compose.lab-nested-ports.yaml")
	if err != nil {
		t.Fatal(err)
	}
	np := string(nestedPorts)
	if !strings.Contains(np, `NOCTAXRIS_NESTED_PORT_PUBLISH: "1"`) {
		t.Fatal("compose.lab-nested-ports.yaml must opt in NOCTAXRIS_NESTED_PORT_PUBLISH=1")
	}
	if strings.Contains(noctaxris, `NOCTAXRIS_NESTED_PORT_PUBLISH: "1"`) {
		t.Fatal("default Compose must not hardcode NOCTAXRIS_NESTED_PORT_PUBLISH=1")
	}
	if strings.Contains(noctaxris, `NOCTAXRIS_SHARED_KAFKA: "1"`) {
		t.Fatal("default Compose must not hardcode NOCTAXRIS_SHARED_KAFKA=1")
	}
	if strings.Contains(noctaxris, `NOCTAXRIS_BROKER_PORT_PUBLISH: "1"`) {
		t.Fatal("default Compose must not hardcode NOCTAXRIS_BROKER_PORT_PUBLISH=1")
	}
	if !strings.Contains(noctaxris, `NOCTAXRIS_SHARED_KAFKA: "${NOCTAXRIS_SHARED_KAFKA:-0}"`) {
		t.Fatal("default Compose must pass shared Kafka env defaulting to 0")
	}
	if !strings.Contains(noctaxris, `NOCTAXRIS_BROKER_PORT_PUBLISH: "${NOCTAXRIS_BROKER_PORT_PUBLISH:-0}"`) {
		t.Fatal("default Compose must pass broker port publish env defaulting to 0")
	}
	brokers, err := os.ReadFile("compose.lab-brokers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(brokers), `NOCTAXRIS_SHARED_KAFKA: "1"`) ||
		!strings.Contains(string(brokers), `NOCTAXRIS_SHARED_MQTT: "1"`) ||
		!strings.Contains(string(brokers), `NOCTAXRIS_BROKER_PORT_PUBLISH: "1"`) {
		t.Fatal("compose.lab-brokers.yaml must opt in shared broker flags")
	}
	for _, needle := range []string{
		`"127.0.0.1:5432:5432"`,
		`"127.0.0.1:3306:3306"`,
		`"127.0.0.1:6379:6379"`,
		`"127.0.0.1:27017:27017"`,
		`"127.0.0.1:8182:8182"`,
		`"127.0.0.1:9092:9092"`,
		`"127.0.0.1:1883:1883"`,
	} {
		if !strings.Contains(np, needle) {
			t.Fatalf("compose.lab-nested-ports.yaml missing loopback publish %s", needle)
		}
	}
	if strings.Contains(np, "0.0.0.0:") {
		t.Fatal("compose.lab-nested-ports.yaml must not publish nested ports on 0.0.0.0")
	}
	if hasDockerSockVolumeEntry(np) {
		t.Fatal("compose.lab-nested-ports.yaml must not mount docker.sock")
	}
	engineOverlay := serviceBlock(np, "noctaxris-engine")
	if engineOverlay == "" {
		t.Fatal("compose.lab-nested-ports.yaml must override noctaxris-engine")
	}
	if !strings.Contains(engineOverlay, "ports:") {
		t.Fatal("compose.lab-nested-ports.yaml must publish ports on noctaxris-engine (DinD hop)")
	}
	apiOverlay := serviceBlock(np, "noctaxris")
	if strings.Contains(apiOverlay, "ports:") {
		t.Fatal("compose.lab-nested-ports.yaml must not fake nested ports on the API service")
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

func TestComposeSplitsDataFromEngineComputeVolume(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	noctaxris := serviceBlock(content, "noctaxris")
	engine := serviceBlock(content, "noctaxris-engine")
	initSvc := serviceBlock(content, "noctaxris-compute-init")
	if !strings.Contains(noctaxris, "noctaxris-data:/var/lib/noctaxris") {
		t.Fatal("noctaxris must mount noctaxris-data for API state (state.db / sealed material)")
	}
	if !strings.Contains(noctaxris, "noctaxris-secrets:/var/lib/noctaxris-secrets") {
		t.Fatal("noctaxris must mount noctaxris-secrets for master.key outside the data root")
	}
	if !strings.Contains(noctaxris, `NOCTAXRIS_MASTER_KEY_FILE: "/var/lib/noctaxris-secrets/master.key"`) {
		t.Fatal("noctaxris must set NOCTAXRIS_MASTER_KEY_FILE on the secrets volume")
	}
	if !strings.Contains(noctaxris, "noctaxris-compute:/var/lib/noctaxris/lambda") {
		t.Fatal("noctaxris must mount noctaxris-compute for Lambda code shared with DinD")
	}
	if !strings.Contains(engine, "noctaxris-compute:/var/lib/noctaxris/lambda:ro") {
		t.Fatal("noctaxris-engine must mount noctaxris-compute read-only for code integrity")
	}
	if engineMountIsReadWrite(engine, "noctaxris-compute:/var/lib/noctaxris/lambda") {
		t.Fatal("noctaxris-engine noctaxris-compute mount must not be read-write")
	}
	if strings.Contains(engine, "noctaxris-data:") {
		t.Fatal("noctaxris-engine must not mount noctaxris-data (would expose sealed state to the nested engine)")
	}
	if strings.Contains(engine, "noctaxris-secrets:") {
		t.Fatal("noctaxris-engine must not mount noctaxris-secrets (would expose master.key to the nested engine)")
	}
	if !strings.Contains(content, "noctaxris-compute:") {
		t.Fatal("compose must declare noctaxris-compute volume")
	}
	if !strings.Contains(content, "noctaxris-secrets:") {
		t.Fatal("compose must declare noctaxris-secrets volume")
	}
	if !strings.Contains(initSvc, "chown -R 65532:65532 /lambda /secrets") {
		t.Fatal("noctaxris-compute-init must chown compute and secrets volumes to API UID 65532")
	}
	if !strings.Contains(initSvc, "chmod 700 /secrets") {
		t.Fatal("noctaxris-compute-init must chmod secrets volume to 0700")
	}
	if !strings.Contains(noctaxris, "noctaxris-compute-init:") ||
		!strings.Contains(noctaxris, "service_completed_successfully") {
		t.Fatal("noctaxris must wait for noctaxris-compute-init to finish before start")
	}
	if !strings.Contains(engine, "noctaxris-compute-init:") ||
		!strings.Contains(engine, "service_completed_successfully") {
		t.Fatal("noctaxris-engine must wait for noctaxris-compute-init before start")
	}
}

func TestComposeEngineDefaultRestricted(t *testing.T) {
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
		if idx := strings.Index(trimmed, " #"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		if trimmed == "privileged: true" {
			t.Fatal("default noctaxris-engine must not set privileged: true (use compose.engine-privileged.yaml)")
		}
	}
	if !strings.Contains(engine, "privileged: false") {
		t.Fatal("default noctaxris-engine must set privileged: false")
	}
	if !strings.Contains(engine, "cap_add:") {
		t.Fatal("default noctaxris-engine must declare cap_add for restricted DinD")
	}
	if !strings.Contains(engine, "SYS_ADMIN") {
		t.Fatal("default noctaxris-engine cap_add must include SYS_ADMIN")
	}
	if !strings.Contains(engine, "cgroup: host") {
		t.Fatal("default noctaxris-engine must set cgroup: host for cgroup v2 nesting")
	}
	if !strings.Contains(engine, "/sys/fs/cgroup:/sys/fs/cgroup:rw") {
		t.Fatal("default noctaxris-engine must mount /sys/fs/cgroup read-write")
	}
	if !strings.Contains(engine, "--ipv6=false") {
		t.Fatal("default noctaxris-engine must pass dockerd --ipv6=false for non-privileged nesting")
	}
	overlay, err := os.ReadFile("compose.engine-privileged.yaml")
	if err != nil {
		t.Fatal("compose.engine-privileged.yaml must exist for broken-host opt-in:", err)
	}
	ovEngine := serviceBlock(string(overlay), "noctaxris-engine")
	if ovEngine == "" {
		t.Fatal("compose.engine-privileged.yaml must override noctaxris-engine")
	}
	if !strings.Contains(ovEngine, "privileged: true") {
		t.Fatal("compose.engine-privileged.yaml must set privileged: true")
	}
}

func TestComposePinsEngineAndInitImages(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	if !strings.Contains(content, "docker:27-dind@sha256:") {
		t.Fatal("noctaxris-engine image must be pinned by digest")
	}
	if !strings.Contains(content, "busybox:1.36@sha256:") {
		t.Fatal("noctaxris-compute-init image must be pinned by digest")
	}
}

func TestComposeDependsOnEngineHealthy(t *testing.T) {
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	noctaxris := serviceBlock(string(b), "noctaxris")
	if !strings.Contains(noctaxris, "condition: service_healthy") {
		t.Fatal("noctaxris must depend_on noctaxris-engine with condition: service_healthy")
	}
	if !strings.Contains(noctaxris, "healthcheck:") {
		t.Fatal("noctaxris must define a healthcheck")
	}
	if !strings.Contains(noctaxris, "/noctaxris") || !strings.Contains(noctaxris, "healthcheck") {
		t.Fatal("noctaxris healthcheck must invoke /noctaxris healthcheck")
	}
	engine := serviceBlock(string(b), "noctaxris-engine")
	if !strings.Contains(engine, "healthcheck:") {
		t.Fatal("noctaxris-engine must define a healthcheck")
	}
}

// engineMountIsReadWrite reports a non-comment volume list item that mounts
// the given source:dest path without a :ro (or :ro,) suffix.
func engineMountIsReadWrite(engineBlock, sourceDest string) bool {
	for _, line := range strings.Split(engineBlock, "\n") {
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
		if rest == sourceDest {
			return true
		}
		if strings.HasPrefix(rest, sourceDest+":") {
			mode := strings.TrimPrefix(rest, sourceDest+":")
			if mode == "" || mode == "rw" || strings.HasPrefix(mode, "rw,") {
				return true
			}
		}
	}
	return false
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
