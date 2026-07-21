package compute

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/google/uuid"
)

const (
	// ECSNetworkName is the DinD-internal network for ECS task containers.
	ECSNetworkName = "noctaxris-ecs"
)

// ECSRunOpts configures a lab ECS task container inside DinD.
type ECSRunOpts struct {
	ImageURI    string
	Command     []string
	Env         map[string]string
	EndpointURL string
}

// ValidateECSRunOpts checks required fields without talking to Docker.
func ValidateECSRunOpts(opts ECSRunOpts) error {
	if strings.TrimSpace(opts.ImageURI) == "" {
		return fmt.Errorf("compute: ImageURI is required")
	}
	return AllowImagePull(opts.ImageURI)
}

// EnsureECSNetwork creates (or reuses) an Internal Docker network for ECS tasks.
// Existing networks that are not Internal are refused (fail closed).
func (c *Client) EnsureECSNetwork(ctx context.Context) (string, error) {
	return c.ensureInternalNetwork(ctx, ECSNetworkName)
}

// RunECSTask starts a detached ECS task container and returns its Docker ID.
func (c *Client) RunECSTask(ctx context.Context, opts ECSRunOpts) (string, error) {
	if err := ValidateECSRunOpts(opts); err != nil {
		return "", err
	}
	endpoint := strings.TrimSpace(opts.EndpointURL)
	if endpoint == "" {
		endpoint = defaultEndpointURL
	}

	if _, err := c.EnsureECSNetwork(ctx); err != nil {
		return "", err
	}
	if err := c.pullImage(ctx, opts.ImageURI); err != nil {
		return "", fmt.Errorf("compute: pull image %s: %w", opts.ImageURI, err)
	}

	env := []string{
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_ENDPOINT_URL=" + endpoint,
		"AWS_ENDPOINT_URL_STS=" + endpoint,
		"AWS_ENDPOINT_URL_S3=" + endpoint,
		"AWS_ENDPOINT_URL_DYNAMODB=" + endpoint,
		"AWS_ENDPOINT_URL_SQS=" + endpoint,
		"AWS_ENDPOINT_URL_LAMBDA=" + endpoint,
		"AWS_ENDPOINT_URL_IAM=" + endpoint,
		"AWS_ENDPOINT_URL_KMS=" + endpoint,
		"AWS_ENDPOINT_URL_ECR=" + endpoint,
		"AWS_ENDPOINT_URL_ECS=" + endpoint,
	}
	for k, v := range opts.Env {
		if k == "" {
			continue
		}
		env = append(env, k+"="+v)
	}

	name := "noctaxris-ecs-" + uuid.NewString()
	hostConfig := &container.HostConfig{
		AutoRemove:  false,
		NetworkMode: container.NetworkMode(ECSNetworkName),
		ExtraHosts:  hostGatewayExtraHosts(),
	}
	cfg := &container.Config{
		Image: opts.ImageURI,
		Env:   env,
		Cmd:   opts.Command,
	}
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			ECSNetworkName: {},
		},
	}

	create, err := c.cli.ContainerCreate(ctx, cfg, hostConfig, netCfg, nil, name)
	if err != nil {
		return "", fmt.Errorf("compute: ecs container create: %w", err)
	}
	cid := create.ID
	if err := c.cli.ContainerStart(ctx, cid, container.StartOptions{}); err != nil {
		_ = c.cli.ContainerRemove(context.Background(), cid, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("compute: ecs container start: %w", err)
	}
	return cid, nil
}

// StopECSTask stops and removes an ECS task container by Docker ID.
func (c *Client) StopECSTask(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	timeout := 10
	if err := c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("compute: ecs container stop: %w", err)
	}
	if err := c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("compute: ecs container remove: %w", err)
	}
	return nil
}

// WaitECSTaskExit blocks until the container is not running or ctx ends.
// Returns the container exit code (0 on success).
func (c *Client) WaitECSTaskExit(ctx context.Context, containerID string) (int64, error) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return -1, fmt.Errorf("compute: container ID is required")
	}
	statusCh, errCh := c.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return -1, fmt.Errorf("compute: ecs container wait: %w", err)
		}
		return 0, nil
	case st := <-statusCh:
		if st.Error != nil && st.Error.Message != "" {
			return st.StatusCode, fmt.Errorf("compute: ecs container wait: %s", st.Error.Message)
		}
		return st.StatusCode, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

// ContainerRunning reports whether the DinD container is still running.
// Missing containers are treated as not running.
func (c *Client) ContainerRunning(ctx context.Context, containerID string) (bool, error) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return false, fmt.Errorf("compute: container ID is required")
	}
	insp, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("compute: ecs container inspect: %w", err)
	}
	return insp.State != nil && insp.State.Running, nil
}
