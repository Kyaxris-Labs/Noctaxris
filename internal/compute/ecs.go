package compute

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
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
	return nil
}

// EnsureECSNetwork creates (or reuses) an Internal Docker network for ECS tasks.
func (c *Client) EnsureECSNetwork(ctx context.Context) (string, error) {
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("compute: list networks: %w", err)
	}
	for _, n := range networks {
		if n.Name == ECSNetworkName {
			return n.ID, nil
		}
	}
	resp, err := c.cli.NetworkCreate(ctx, ECSNetworkName, network.CreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels: map[string]string{
			"noctaxris.managed": "true",
		},
	})
	if err != nil {
		return "", fmt.Errorf("compute: create network %s: %w", ECSNetworkName, err)
	}
	return resp.ID, nil
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
		ExtraHosts:  []string{"host.docker.internal:host-gateway"},
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
