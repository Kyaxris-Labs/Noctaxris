package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/google/uuid"
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

	// ECSIMDSImage is the allowlisted alpine base for the credentials httpd.
	ECSIMDSImage = "alpine:3.20"

	ecsIMDSRelativePrefix = "/v2/credentials/"
	ecsIMDSDocRoot        = "/www"
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
	credID := uuid.NewString()
	body, err := FormatContainerCredentialsJSON(ContainerCredentials{
		AccessKeyID:     ak,
		SecretAccessKey: sk,
		Token:           tok,
		Expiration:      time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		return nil, nil, err
	}
	if err := c.registerECSIMDSCredentials(ctx, credID, body); err != nil {
		return nil, nil, err
	}
	return []string{ECSIMDSLinkLocal + ":" + ip}, ContainerCredentialsEnv(credID), nil
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

	cmd := fmt.Sprintf(
		"mkdir -p %s%s && exec httpd -f -p %d -h %s",
		ecsIMDSDocRoot, ecsIMDSRelativePrefix, ECSIMDSListenPort, ecsIMDSDocRoot,
	)
	sec := nestedTaskSecurity(64)
	hostConfig := &container.HostConfig{
		AutoRemove:  false,
		NetworkMode: container.NetworkMode(ECSNetworkName),
		// Fail closed: never publish IMDS to the operator host.
		PortBindings: nil,
		Privileged:   false,
		CapDrop:      append([]string(nil), sec.CapDrop...),
		SecurityOpt:  append([]string(nil), sec.SecurityOpt...),
		Resources:    nestedTaskResources(sec),
	}
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
	create, err := c.cli.ContainerCreate(ctx, cfg, hostConfig, netCfg, nil, ECSIMDSContainerName)
	if err != nil {
		return "", fmt.Errorf("compute: ecs imds create: %w", err)
	}
	if err := c.cli.ContainerStart(ctx, create.ID, container.StartOptions{}); err != nil {
		_ = c.cli.ContainerRemove(context.Background(), create.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("compute: ecs imds start: %w", err)
	}
	ip, ok := c.ecsIMDSRunningIP(ctx)
	if !ok {
		return "", fmt.Errorf("compute: ecs imds has no address on %s", ECSNetworkName)
	}
	return ip, nil
}

func (c *Client) ecsIMDSRunningIP(ctx context.Context) (string, bool) {
	insp, err := c.cli.ContainerInspect(ctx, ECSIMDSContainerName)
	if err != nil {
		return "", false
	}
	if insp.State == nil || !insp.State.Running {
		return "", false
	}
	if insp.NetworkSettings == nil || insp.NetworkSettings.Networks == nil {
		return "", false
	}
	ep, ok := insp.NetworkSettings.Networks[ECSNetworkName]
	if !ok || ep == nil {
		return "", false
	}
	ip := strings.TrimSpace(ep.IPAddress)
	if ip == "" {
		return "", false
	}
	return ip, true
}

func (c *Client) removeECSIMDSContainer(ctx context.Context) error {
	err := c.cli.ContainerRemove(ctx, ECSIMDSContainerName, container.RemoveOptions{Force: true})
	if err == nil || errdefs.IsNotFound(err) {
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

func (c *Client) registerECSIMDSCredentials(ctx context.Context, credID string, body []byte) error {
	id := strings.TrimSpace(credID)
	if id == "" {
		return fmt.Errorf("compute: imds credential id is required")
	}
	if len(body) == 0 {
		return fmt.Errorf("compute: imds credential body is empty")
	}
	insp, err := c.cli.ContainerInspect(ctx, ECSIMDSContainerName)
	if err != nil {
		return fmt.Errorf("compute: imds register inspect: %w", err)
	}
	path := ecsIMDSDocRoot + ecsIMDSRelativePrefix + id
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
