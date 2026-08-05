package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const (
	// EC2IMDSLinkLocal is the classic EC2 metadata address (ExtraHosts → sidecar).
	EC2IMDSLinkLocal = "169.254.169.254"

	// EC2IMDSListenPort is the sidecar listen port inside noctaxris-ec2.
	// High port keeps CapDrop ALL (no CAP_NET_BIND_SERVICE). Labs and SDKs
	// should use AWS_EC2_METADATA_SERVICE_ENDPOINT (includes host+port).
	EC2IMDSListenPort = 9255

	// EC2IMDSContainerName is the long-lived DinD sidecar on noctaxris-ec2.
	EC2IMDSContainerName = "noctaxris-ec2-imds"

	// EC2IMDSImage reuses the ECS credentials mirror base (allowlisted).
	EC2IMDSImage = ECSIMDSImage

	ec2IMDSDocRoot = "/www/by-ip"
	labelEC2IMDS   = "noctaxris.imds"
)

// EC2IMDSMeta is the per-instance payload served by the EC2 IMDS lite sidecar.
// Lookup is by the requesting container's noctaxris-ec2 address (Floci-shaped).
type EC2IMDSMeta struct {
	InstanceID string `json:"instance-id"`
	LocalIPv4  string `json:"local-ipv4"`
	AMIID      string `json:"ami-id"`
	// IAMRole is listed under iam/security-credentials/ when non-empty.
	IAMRole string `json:"iam-role,omitempty"`
}

// EC2IMDSEndpointEnv returns AWS_EC2_METADATA_SERVICE_ENDPOINT for the lab port.
func EC2IMDSEndpointEnv() map[string]string {
	return map[string]string{
		"AWS_EC2_METADATA_SERVICE_ENDPOINT": fmt.Sprintf(
			"http://%s:%d", EC2IMDSLinkLocal, EC2IMDSListenPort,
		),
	}
}

// attachEC2IMDSHosts ensures the EC2 IMDS sidecar and returns ExtraHosts + endpoint env.
// Failures are returned to the caller; StartEC2Instance treats them as best-effort.
func (c *Client) attachEC2IMDSHosts(ctx context.Context) (extraHosts []string, endpointEnv map[string]string, err error) {
	ip, err := c.ensureEC2IMDSMirror(ctx)
	if err != nil {
		return nil, nil, err
	}
	return []string{EC2IMDSLinkLocal + ":" + ip}, EC2IMDSEndpointEnv(), nil
}

func (c *Client) ensureEC2IMDSMirror(ctx context.Context) (string, error) {
	if c == nil || c.cli == nil {
		return "", fmt.Errorf("compute: client unavailable")
	}
	if _, err := c.EnsureEC2Network(ctx); err != nil {
		return "", err
	}
	if ip, ok := c.ec2IMDSRunningIP(ctx); ok {
		return ip, nil
	}
	_ = c.removeEC2IMDSContainer(ctx)

	if err := c.pullImage(ctx, EC2IMDSImage); err != nil {
		return "", fmt.Errorf("compute: pull ec2 imds image: %w", err)
	}

	// Custom handler: route by client IP so multi-instance labs share one sidecar
	// without host :9169. No PortBindings (DinD-internal only).
	cmd := fmt.Sprintf(`mkdir -p %s && cat > /tmp/noctaxris-ec2-imds.py <<'PY'
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
ROOT = Path(%q)
PORT = %d
ROOT.mkdir(parents=True, exist_ok=True)

def meta_for(ip):
    p = ROOT / (ip + ".json")
    if not p.is_file():
        return None
    try:
        return json.loads(p.read_text())
    except Exception:
        return None

class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        return
    def do_GET(self):
        meta = meta_for(self.client_address[0])
        if meta is None:
            self.send_error(404)
            return
        path = self.path.split("?", 1)[0].rstrip("/") or "/"
        body = None
        if path == "/latest/meta-data/instance-id":
            body = meta.get("instance-id", "")
        elif path == "/latest/meta-data/local-ipv4":
            body = meta.get("local-ipv4", "")
        elif path == "/latest/meta-data/ami-id":
            body = meta.get("ami-id", "")
        elif path == "/latest/meta-data/iam/security-credentials":
            role = (meta.get("iam-role") or "").strip()
            body = (role + "\n") if role else ""
        elif path.startswith("/latest/meta-data/iam/security-credentials/"):
            role = path.rsplit("/", 1)[-1]
            want = (meta.get("iam-role") or "").strip()
            if not want or role != want:
                self.send_error(404)
                return
            body = want + "\n"
        else:
            self.send_error(404)
            return
        data = body.encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

ThreadingHTTPServer(("0.0.0.0", PORT), H).serve_forever()
PY
exec python /tmp/noctaxris-ec2-imds.py`, ec2IMDSDocRoot, ec2IMDSDocRoot, EC2IMDSListenPort)

	sec := nestedTaskSecurity(64)
	hostConfig := &container.HostConfig{
		AutoRemove:   false,
		NetworkMode:  container.NetworkMode(EC2NetworkName),
		PortBindings: nil,
		Privileged:   false,
		CapDrop:      append([]string(nil), sec.CapDrop...),
		SecurityOpt:  append([]string(nil), sec.SecurityOpt...),
		Resources:    nestedTaskResources(sec),
	}
	cfg := &container.Config{
		Image: EC2IMDSImage,
		Cmd:   []string{"/bin/sh", "-c", cmd},
		Labels: map[string]string{
			LabelManaged: "true",
			labelEC2IMDS: "ec2",
		},
	}
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			EC2NetworkName: {},
		},
	}
	create, err := c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           cfg,
		HostConfig:       hostConfig,
		NetworkingConfig: netCfg,
		Name:             EC2IMDSContainerName,
	})
	if err != nil {
		_ = c.removeEC2IMDSContainer(ctx)
		create, err = c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
			Config:           cfg,
			HostConfig:       hostConfig,
			NetworkingConfig: netCfg,
			Name:             EC2IMDSContainerName,
		})
		if err != nil {
			return "", fmt.Errorf("compute: ec2 imds create: %w", err)
		}
	}
	if _, err := c.cli.ContainerStart(ctx, create.ID, client.ContainerStartOptions{}); err != nil {
		_, _ = c.cli.ContainerRemove(context.Background(), create.ID, client.ContainerRemoveOptions{Force: true})
		return "", fmt.Errorf("compute: ec2 imds start: %w", err)
	}
	ip, ok := c.ec2IMDSRunningIP(ctx)
	if !ok {
		return "", fmt.Errorf("compute: ec2 imds has no address on %s", EC2NetworkName)
	}
	return ip, nil
}

func (c *Client) ec2IMDSRunningIP(ctx context.Context) (string, bool) {
	inspRes, err := c.cli.ContainerInspect(ctx, EC2IMDSContainerName, client.ContainerInspectOptions{})
	if err != nil {
		return "", false
	}
	insp := inspRes.Container
	if insp.State == nil || !insp.State.Running {
		return "", false
	}
	if insp.NetworkSettings == nil || insp.NetworkSettings.Networks == nil {
		return "", false
	}
	ep, ok := insp.NetworkSettings.Networks[EC2NetworkName]
	if !ok || ep == nil {
		return "", false
	}
	ip := ep.IPAddress
	if !ip.IsValid() {
		return "", false
	}
	return ip.String(), true
}

func (c *Client) removeEC2IMDSContainer(ctx context.Context) error {
	_, err := c.cli.ContainerRemove(ctx, EC2IMDSContainerName, client.ContainerRemoveOptions{Force: true})
	if err == nil || cerrdefs.IsNotFound(err) {
		return nil
	}
	return err
}

// registerEC2IMDSMeta writes instance metadata keyed by the instance container IP.
func (c *Client) registerEC2IMDSMeta(ctx context.Context, clientIP string, meta EC2IMDSMeta) error {
	ip := strings.TrimSpace(clientIP)
	if ip == "" {
		return fmt.Errorf("compute: ec2 imds client IP is required")
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("compute: ec2 imds meta json: %w", err)
	}
	inspRes, err := c.cli.ContainerInspect(ctx, EC2IMDSContainerName, client.ContainerInspectOptions{})
	if err != nil {
		return fmt.Errorf("compute: ec2 imds register inspect: %w", err)
	}
	insp := inspRes.Container
	path := ec2IMDSDocRoot + "/" + ip + ".json"
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
		return fmt.Errorf("compute: ec2 imds register exec: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("compute: ec2 imds register exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return nil
}

// ContainerNetworkIP returns the container IPv4 on networkName (e.g. noctaxris-ec2).
func (c *Client) ContainerNetworkIP(ctx context.Context, containerID, networkName string) (string, error) {
	if c == nil || c.cli == nil {
		return "", fmt.Errorf("compute: client unavailable")
	}
	cid := strings.TrimSpace(containerID)
	netName := strings.TrimSpace(networkName)
	if cid == "" || netName == "" {
		return "", fmt.Errorf("compute: container ID and network name are required")
	}
	inspRes, err := c.cli.ContainerInspect(ctx, cid, client.ContainerInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("compute: container inspect: %w", err)
	}
	insp := inspRes.Container
	if insp.NetworkSettings == nil || insp.NetworkSettings.Networks == nil {
		return "", fmt.Errorf("compute: container has no network settings")
	}
	ep, ok := insp.NetworkSettings.Networks[netName]
	if !ok || ep == nil {
		return "", fmt.Errorf("compute: container not on network %s", netName)
	}
	ip := ep.IPAddress
	if !ip.IsValid() {
		return "", fmt.Errorf("compute: container has no IP on %s", netName)
	}
	return ip.String(), nil
}
