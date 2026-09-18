package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const (
	// ECSIMDSLinkLocal is the ECS task-credentials link-local address AWS SDKs use
	// with AWS_CONTAINER_CREDENTIALS_RELATIVE_URI (http://169.254.170.2<path>).
	ECSIMDSLinkLocal = "169.254.170.2"

	// ECSIMDSListenPort is the sidecar listen port inside noctaxris-ecs.
	// High port keeps CapDrop ALL (no CAP_NET_BIND_SERVICE). Labs and the AWS SDK
	// should use AWS_CONTAINER_CREDENTIALS_FULL_URI (includes host+port). Relative
	// URI alone still assumes :80, which this lab mirror does not bind.
	ECSIMDSListenPort = 9254

	// ECSIMDSContainerName is the long-lived DinD sidecar on noctaxris-ecs.
	ECSIMDSContainerName = "noctaxris-ecs-imds"

	// ECSIMDSImage is the allowlisted base for the credentials HTTP mirror.
	// alpine:3.20 busybox has no httpd applet (sidecar exited 127); python slim is pinned.
	ECSIMDSImage = "python:3.12-slim"

	ecsIMDSRelativePrefix = "/v2/credentials/"
	ecsIMDSDocRoot        = "/www"
	ecsIMDSHandlerScript  = "/tmp/noctaxris-ecs-imds.py"
	labelECSIMDS          = "noctaxris.imds"
)

// ContainerCredentials is the ECS task-role credentials JSON shape returned by
// GET http://169.254.170.2$AWS_CONTAINER_CREDENTIALS_RELATIVE_URI (lab: FULL_URI).
type ContainerCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	Token           string
	Expiration      time.Time
	RoleArn         string
}

// containerCredentialsJSON is the wire encoding AWS SDKs expect from the
// container credentials HTTP endpoint.
type containerCredentialsJSON struct {
	AccessKeyId     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	Token           string `json:"Token"`
	Expiration      string `json:"Expiration"`
	RoleArn         string `json:"RoleArn,omitempty"`
}

// FormatContainerCredentialsJSON encodes creds in the ECS container-metadata shape.
func FormatContainerCredentialsJSON(c ContainerCredentials) ([]byte, error) {
	ak := strings.TrimSpace(c.AccessKeyID)
	sk := strings.TrimSpace(c.SecretAccessKey)
	if ak == "" || sk == "" {
		return nil, fmt.Errorf("compute: container credentials require AccessKeyID and SecretAccessKey")
	}
	exp := c.Expiration.UTC()
	if exp.IsZero() {
		exp = time.Now().UTC().Add(time.Hour)
	}
	body := containerCredentialsJSON{
		AccessKeyId:     ak,
		SecretAccessKey: sk,
		Token:           strings.TrimSpace(c.Token),
		Expiration:      exp.Format(time.RFC3339),
		RoleArn:         strings.TrimSpace(c.RoleArn),
	}
	return json.Marshal(body)
}

// ContainerCredentialsEnv returns ECS relative + full URI env vars for credID.
// FULL_URI points at the lab link-local ExtraHosts mapping on ECSIMDSListenPort.
func ContainerCredentialsEnv(credID string) map[string]string {
	id := strings.TrimSpace(credID)
	if id == "" {
		return nil
	}
	rel := ecsIMDSRelativePrefix + id
	full := fmt.Sprintf("http://%s:%d%s", ECSIMDSLinkLocal, ECSIMDSListenPort, rel)
	return map[string]string{
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": rel,
		"AWS_CONTAINER_CREDENTIALS_FULL_URI":     full,
	}
}

// roleCredsFromEnv reads minted AWS_* keys used to mirror container credentials.
func roleCredsFromEnv(env map[string]string) (accessKeyID, secret, token string, ok bool) {
	if env == nil {
		return "", "", "", false
	}
	accessKeyID = strings.TrimSpace(env["AWS_ACCESS_KEY_ID"])
	secret = strings.TrimSpace(env["AWS_SECRET_ACCESS_KEY"])
	token = strings.TrimSpace(env["AWS_SESSION_TOKEN"])
	ok = accessKeyID != "" && secret != ""
	return accessKeyID, secret, token, ok
}

// attachECSIMDSEnv mirrors minted AWS_* into the ECS IMDS sidecar and returns
// ExtraHosts plus credential URI env overlays. No-op when AWS_* are absent.
// The sidecar stays on Internal noctaxris-ecs only (no host port publish).
func (c *Client) attachECSIMDSEnv(ctx context.Context, env map[string]string) (extraHosts []string, uriEnv map[string]string, err error) {
	ak, sk, tok, ok := roleCredsFromEnv(env)
	if !ok {
		return nil, nil, nil
	}
	ip, err := c.ensureECSIMDSMirror(ctx)
	if err != nil {
		return nil, nil, err
	}
	id, err := normalizeECSIMDSCredID(uuid.NewString())
	if err != nil {
		return nil, nil, err
	}
	body, err := FormatContainerCredentialsJSON(ContainerCredentials{
		AccessKeyID:     ak,
		SecretAccessKey: sk,
		Token:           tok,
		Expiration:      time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		return nil, nil, err
	}
	if err := c.registerECSIMDSCredentials(ctx, id, body); err != nil {
		return nil, nil, err
	}
	return []string{ECSIMDSLinkLocal + ":" + ip}, ContainerCredentialsEnv(id), nil
}

func (c *Client) ensureECSIMDSMirror(ctx context.Context) (string, error) {
	if c == nil || c.cli == nil {
		return "", fmt.Errorf("compute: client unavailable")
	}
	if _, err := c.EnsureECSNetwork(ctx); err != nil {
		return "", err
	}
	if ip, ok := c.ecsIMDSRunningIP(ctx); ok {
		return ip, nil
	}
	_ = c.removeECSIMDSContainer(ctx)

	if err := c.pullImage(ctx, ECSIMDSImage); err != nil {
		return "", fmt.Errorf("compute: pull ecs imds image: %w", err)
	}

	cmd := ecsIMDSSidecarCmd()
	hostConfig := ecsIMDSHostConfig()
	cfg := &container.Config{
		Image: ECSIMDSImage,
		Cmd:   []string{"/bin/sh", "-c", cmd},
		Labels: map[string]string{
			LabelManaged: "true",
			labelECSIMDS: "ecs",
		},
	}
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			ECSNetworkName: {},
		},
	}
	create, err := c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           cfg,
		HostConfig:       hostConfig,
		NetworkingConfig: netCfg,
		Name:             ECSIMDSContainerName,
	})
	if err != nil {
		// Stale name after a failed remove or race: force remove and retry once.
		_ = c.removeECSIMDSContainer(ctx)
		create, err = c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
			Config:           cfg,
			HostConfig:       hostConfig,
			NetworkingConfig: netCfg,
			Name:             ECSIMDSContainerName,
		})
		if err != nil {
			return "", fmt.Errorf("compute: ecs imds create: %w", err)
		}
	}
	if _, err := c.cli.ContainerStart(ctx, create.ID, client.ContainerStartOptions{}); err != nil {
		_, _ = c.cli.ContainerRemove(context.Background(), create.ID, client.ContainerRemoveOptions{Force: true})
		return "", fmt.Errorf("compute: ecs imds start: %w", err)
	}
	ip, ok := c.ecsIMDSRunningIP(ctx)
	if !ok {
		return "", fmt.Errorf("compute: ecs imds has no address on %s", ECSNetworkName)
	}
	return ip, nil
}

func ecsIMDSHostConfig() *container.HostConfig {
	sec := nestedTaskSecurity(64)
	return &container.HostConfig{
		AutoRemove:  false,
		NetworkMode: container.NetworkMode(ECSNetworkName),
		// Fail closed: never publish IMDS to the operator host.
		PortBindings: nil,
		Privileged:   false,
		CapDrop:      append([]string(nil), sec.CapDrop...),
		SecurityOpt:  append([]string(nil), sec.SecurityOpt...),
		Resources:    nestedTaskResources(sec),
	}
}

func ecsIMDSSidecarCmd() string {
	return fmt.Sprintf(
		"mkdir -p %s%s && cat > %s <<'PY'\n%sPY\nexec python %s",
		ecsIMDSDocRoot,
		ecsIMDSRelativePrefix,
		ecsIMDSHandlerScript,
		ecsIMDSHandlerPython(ecsIMDSDocRoot, ECSIMDSListenPort, "0.0.0.0"),
		ecsIMDSHandlerScript,
	)
}

// ecsIMDSHandlerPython is the sidecar HTTP server: GET of an exact
// /v2/credentials/<uuid> file only. Directory URLs, other methods, and
// non-UUID paths return 404 (OWASP deny-by-default; no SimpleHTTP listing).
func ecsIMDSHandlerPython(root string, port int, bind string) string {
	if strings.TrimSpace(bind) == "" {
		bind = "0.0.0.0"
	}
	return fmt.Sprintf(`from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import re
ROOT = Path(%q)
PORT = %d
BIND = %q
CRED_RE = re.compile(r"^/v2/credentials/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$")

class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        return
    def _deny(self):
        self.send_error(404)
    def do_GET(self):
        path = self.path.split("?", 1)[0]
        m = CRED_RE.fullmatch(path)
        if not m:
            self._deny()
            return
        cred_id = m.group(1).lower()
        try:
            base = (ROOT / "v2" / "credentials").resolve()
            target = (base / cred_id).resolve()
            if target.parent != base or not target.is_file():
                self._deny()
                return
            data = target.read_bytes()
        except Exception:
            self._deny()
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)
    def do_HEAD(self):
        self._deny()
    def do_POST(self):
        self._deny()
    def do_PUT(self):
        self._deny()
    def do_DELETE(self):
        self._deny()

ThreadingHTTPServer((BIND, PORT), H).serve_forever()
`, root, port, bind)
}

func ecsIMDSSidecarServesExactPaths(cmd []string) bool {
	joined := strings.Join(cmd, " ")
	if strings.Contains(joined, "python -m http.server") {
		return false
	}
	return strings.Contains(joined, ecsIMDSHandlerScript)
}

func (c *Client) ecsIMDSRunningIP(ctx context.Context) (string, bool) {
	inspRes, err := c.cli.ContainerInspect(ctx, ECSIMDSContainerName, client.ContainerInspectOptions{})
	if err != nil {
		return "", false
	}
	insp := inspRes.Container
	if insp.State == nil || !insp.State.Running {
		return "", false
	}
	if insp.Config != nil && !ecsIMDSSidecarServesExactPaths(insp.Config.Cmd) {
		return "", false
	}
	if insp.NetworkSettings == nil || insp.NetworkSettings.Networks == nil {
		return "", false
	}
	ep, ok := insp.NetworkSettings.Networks[ECSNetworkName]
	if !ok || ep == nil {
		return "", false
	}
	ip := ep.IPAddress
	if !ip.IsValid() {
		return "", false
	}
	return ip.String(), true
}

func (c *Client) removeECSIMDSContainer(ctx context.Context) error {
	_, err := c.cli.ContainerRemove(ctx, ECSIMDSContainerName, client.ContainerRemoveOptions{Force: true})
	if err == nil || cerrdefs.IsNotFound(err) {
		return nil
	}
	return err
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeECSIMDSCredID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("compute: imds credential id is required")
	}
	u, err := uuid.Parse(id)
	if err != nil || u == uuid.Nil {
		return "", fmt.Errorf("compute: imds credential id is invalid")
	}
	return u.String(), nil
}

func ecsIMDSCredFilePath(id string) string {
	return ecsIMDSDocRoot + ecsIMDSRelativePrefix + id
}

// matchECSIMDSCredPath accepts AWS RELATIVE_URI shape /v2/credentials/<uuid>
// (optional query stripped). Directory URLs, traversal, and non-UUID tails fail closed.
func matchECSIMDSCredPath(raw string) (string, bool) {
	path := raw
	if i := strings.Index(raw, "?"); i >= 0 {
		path = raw[:i]
	}
	if !strings.HasPrefix(path, ecsIMDSRelativePrefix) {
		return "", false
	}
	id, err := normalizeECSIMDSCredID(path[len(ecsIMDSRelativePrefix):])
	if err != nil {
		return "", false
	}
	return id, true
}

func ecsIMDSCredIDFromEnvList(env []string) string {
	var rel, full string
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		switch k {
		case "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI":
			rel = v
		case "AWS_CONTAINER_CREDENTIALS_FULL_URI":
			full = v
		}
	}
	if id, ok := matchECSIMDSCredPath(rel); ok {
		return id
	}
	if full == "" {
		return ""
	}
	u, err := url.Parse(full)
	if err != nil {
		return ""
	}
	id, ok := matchECSIMDSCredPath(u.Path)
	if !ok {
		return ""
	}
	return id
}

func ecsIMDSCredIDFromEnvMap(env map[string]string) string {
	if env == nil {
		return ""
	}
	list := make([]string, 0, len(env))
	for k, v := range env {
		list = append(list, k+"="+v)
	}
	return ecsIMDSCredIDFromEnvList(list)
}

func (c *Client) registerECSIMDSCredentials(ctx context.Context, credID string, body []byte) error {
	id := strings.TrimSpace(credID)
	if id == "" {
		return fmt.Errorf("compute: imds credential id is required")
	}
	if len(body) == 0 {
		return fmt.Errorf("compute: imds credential body is empty")
	}
	id, err := normalizeECSIMDSCredID(id)
	if err != nil {
		return err
	}
	inspRes, err := c.cli.ContainerInspect(ctx, ECSIMDSContainerName, client.ContainerInspectOptions{})
	if err != nil {
		return fmt.Errorf("compute: imds register inspect: %w", err)
	}
	insp := inspRes.Container
	path := ecsIMDSCredFilePath(id)
	res, err := c.Exec(ctx, ExecOpts{
		ContainerID: insp.ID,
		Cmd: []string{
			"/bin/sh", "-c",
			`mkdir -p "$(dirname "$NOCTAXRIS_IMDS_PATH")" && printf '%s' "$NOCTAXRIS_IMDS_JSON" > "$NOCTAXRIS_IMDS_PATH"`,
		},
		Env: []string{
			"NOCTAXRIS_IMDS_PATH=" + path,
			"NOCTAXRIS_IMDS_JSON=" + string(body),
		},
	})
	if err != nil {
		return fmt.Errorf("compute: imds register exec: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("compute: imds register exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func (c *Client) unregisterECSIMDSCredentials(ctx context.Context, credID string) error {
	id, err := normalizeECSIMDSCredID(credID)
	if err != nil {
		return err
	}
	if c == nil || c.cli == nil {
		return fmt.Errorf("compute: client unavailable")
	}
	inspRes, err := c.cli.ContainerInspect(ctx, ECSIMDSContainerName, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("compute: imds unregister inspect: %w", err)
	}
	insp := inspRes.Container
	res, err := c.Exec(ctx, ExecOpts{
		ContainerID: insp.ID,
		Cmd: []string{
			"/bin/sh", "-c",
			`rm -f -- "$NOCTAXRIS_IMDS_PATH"`,
		},
		Env: []string{
			"NOCTAXRIS_IMDS_PATH=" + ecsIMDSCredFilePath(id),
		},
	})
	if err != nil {
		return fmt.Errorf("compute: imds unregister exec: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("compute: imds unregister exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func (c *Client) unregisterECSIMDSCredentialsForContainer(ctx context.Context, containerID string) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("compute: client unavailable")
	}
	cid := strings.TrimSpace(containerID)
	if cid == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	inspRes, err := c.cli.ContainerInspect(ctx, cid, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("compute: imds unregister task inspect: %w", err)
	}
	insp := inspRes.Container
	var env []string
	if insp.Config != nil {
		env = insp.Config.Env
	}
	id := ecsIMDSCredIDFromEnvList(env)
	if id == "" {
		return nil
	}
	return c.unregisterECSIMDSCredentials(ctx, id)
}
