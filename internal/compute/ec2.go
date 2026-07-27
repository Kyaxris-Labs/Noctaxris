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
	// EC2NetworkName is the DinD-internal network for lab EC2 instance containers.
	EC2NetworkName = "noctaxris-ec2"

	// LabelEC2 marks nested EC2 lab containers.
	LabelEC2 = "noctaxris.ec2"
)

// EC2 keep-alive command so any base image stays running regardless of default CMD.
var ec2KeepAliveCmd = []string{"tail", "-f", "/dev/null"}

// EC2RunOpts configures a lab EC2 instance container inside DinD.
type EC2RunOpts struct {
	ImageURI    string
	Env         map[string]string
	EndpointURL string
	// ListenAddr pins DinD lab registry pulls.
	ListenAddr string
	MemoryMB   int
	// LabRegistryPull requests an authenticated pull via PullLabRegistryImage.
	LabRegistryPull  bool
	RegistryUsername string
	RegistryPassword string
}

// ValidateEC2RunOpts checks required fields without talking to Docker.
func ValidateEC2RunOpts(opts EC2RunOpts) error {
	if strings.TrimSpace(opts.ImageURI) == "" {
		return fmt.Errorf("compute: ImageURI is required")
	}
	if err := AllowImagePull(opts.ImageURI, opts.ListenAddr); err != nil {
		return err
	}
	return RequireLabRegistryCreds(opts.LabRegistryPull, LabRegistryPullCreds{
		Username: opts.RegistryUsername,
		Password: opts.RegistryPassword,
	})
}

// EnsureEC2Network creates (or reuses) an Internal Docker network for EC2 instances.
func (c *Client) EnsureEC2Network(ctx context.Context) (string, error) {
	return c.ensureInternalNetwork(ctx, EC2NetworkName)
}

func ec2TaskHostConfig(memoryMB int) *container.HostConfig {
	sec := nestedTaskSecurity(memoryMB)
	return &container.HostConfig{
		AutoRemove:  false,
		NetworkMode: container.NetworkMode(EC2NetworkName),
		ExtraHosts:  ecsHostGatewayExtraHosts(),
		Privileged:  false,
		CapDrop:     append([]string(nil), sec.CapDrop...),
		SecurityOpt: append([]string(nil), sec.SecurityOpt...),
		Resources:   nestedTaskResources(sec),
	}
}

// StartEC2Instance creates and starts a keep-alive EC2 lab container. Returns Docker ID.
func (c *Client) StartEC2Instance(ctx context.Context, opts EC2RunOpts) (string, error) {
	if err := ValidateEC2RunOpts(opts); err != nil {
		return "", err
	}
	endpoint := strings.TrimSpace(opts.EndpointURL)
	if endpoint == "" {
		endpoint = defaultEndpointURL
	}
	if _, err := c.EnsureEC2Network(ctx); err != nil {
		return "", err
	}
	if opts.LabRegistryPull {
		if err := c.PullLabRegistryImage(ctx, opts.ImageURI, opts.RegistryUsername, opts.RegistryPassword); err != nil {
			return "", fmt.Errorf("compute: pull lab registry image %s: %w", opts.ImageURI, err)
		}
	} else if err := c.pullImage(ctx, opts.ImageURI); err != nil {
		return "", fmt.Errorf("compute: pull image %s: %w", opts.ImageURI, err)
	}

	env := []string{
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_ENDPOINT_URL=" + endpoint,
		"AWS_ENDPOINT_URL_EC2=" + endpoint,
		"AWS_ENDPOINT_URL_S3=" + endpoint,
		"AWS_ENDPOINT_URL_STS=" + endpoint,
		"AWS_ENDPOINT_URL_IAM=" + endpoint,
	}
	for k, v := range opts.Env {
		if k == "" {
			continue
		}
		env = append(env, k+"="+v)
	}

	name := "noctaxris-ec2-" + uuid.NewString()
	cfg := &container.Config{
		Image: opts.ImageURI,
		Env:   env,
		Cmd:   append([]string(nil), ec2KeepAliveCmd...),
		Labels: map[string]string{
			LabelManaged: "1",
			LabelEC2:     "instance",
		},
	}
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			EC2NetworkName: {},
		},
	}
	create, err := c.cli.ContainerCreate(ctx, cfg, ec2TaskHostConfig(opts.MemoryMB), netCfg, nil, name)
	if err != nil {
		return "", fmt.Errorf("compute: ec2 container create: %w", err)
	}
	cid := create.ID
	if err := c.cli.ContainerStart(ctx, cid, container.StartOptions{}); err != nil {
		_ = c.cli.ContainerRemove(context.Background(), cid, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("compute: ec2 container start: %w", err)
	}
	return cid, nil
}

// StopEC2Instance stops a running EC2 lab container (kept for StartInstances).
func (c *Client) StopEC2Instance(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	timeout := 30
	if err := c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("compute: ec2 container stop: %w", err)
	}
	return nil
}

// StartStoppedEC2Instance starts a previously stopped EC2 lab container.
func (c *Client) StartStoppedEC2Instance(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	if err := c.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("compute: ec2 container start: %w", err)
	}
	return nil
}

// TerminateEC2Instance force-removes an EC2 lab container.
func (c *Client) TerminateEC2Instance(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	if err := c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("compute: ec2 container remove: %w", err)
	}
	return nil
}
