package compute

import (
	"context"
	"fmt"
	"io"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
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
	// ListenAddr is the API listen address used to pin DinD lab registry pulls.
	ListenAddr string
	// MemoryMB is the optional Docker memory limit in megabytes (0 = engine default).
	MemoryMB int
	// LabRegistryPull requests an authenticated pull via PullLabRegistryImage (lab ECR).
	// When true, RunECSTask does not fall back to an unauthenticated pullImage.
	LabRegistryPull bool
	// RegistryUsername is the Docker registry username (lab: "AWS").
	RegistryUsername string
	// RegistryPassword is the lab ECR authorization token password.
	RegistryPassword string
	// PreStartCopyDest, with PreStartCopyTar, copies a tar archive into the container after
	// create and before start (CodeBuild CODECOMMIT workspace inject).
	PreStartCopyDest string
	PreStartCopyTar  io.Reader
}

// ValidateECSRunOpts checks required fields without talking to Docker.
func ValidateECSRunOpts(opts ECSRunOpts) error {
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
	if opts.LabRegistryPull {
		if err := c.PullLabRegistryImage(ctx, opts.ImageURI, opts.RegistryUsername, opts.RegistryPassword); err != nil {
			return "", fmt.Errorf("compute: pull lab registry image %s: %w", opts.ImageURI, err)
		}
	} else if err := c.pullImage(ctx, opts.ImageURI); err != nil {
		return "", fmt.Errorf("compute: pull image %s: %w", opts.ImageURI, err)
	}

	// When AWS_* role creds are present, mirror them on Internal noctaxris-ecs via
	// link-local ExtraHosts → sidecar (not the host API listen). Prefer
	// AWS_CONTAINER_CREDENTIALS_FULL_URI (port 9254); relative URI alone assumes :80.
	taskEnv := opts.Env
	imdsHosts, uriEnv, err := c.attachECSIMDSEnv(ctx, taskEnv)
	if err != nil {
		return "", err
	}
	if len(uriEnv) > 0 {
		taskEnv = cloneStringMap(taskEnv)
		for k, v := range uriEnv {
			taskEnv[k] = v
		}
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
		"AWS_ENDPOINT_URL_SNS=" + endpoint,
		"AWS_ENDPOINT_URL_LOGS=" + endpoint,
		"AWS_ENDPOINT_URL_CODEBUILD=" + endpoint,
		"AWS_ENDPOINT_URL_SECRETSMANAGER=" + endpoint,
		"AWS_ENDPOINT_URL_SECRETS_MANAGER=" + endpoint,
	}
	for k, v := range taskEnv {
		if k == "" {
			continue
		}
		env = append(env, k+"="+v)
	}

	name := "noctaxris-ecs-" + uuid.NewString()
	hostConfig := ecsTaskHostConfig(opts.MemoryMB, imdsHosts)
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

	create, err := c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           cfg,
		HostConfig:       hostConfig,
		NetworkingConfig: netCfg,
		Name:             name,
	})
	if err != nil {
		return "", fmt.Errorf("compute: ecs container create: %w", err)
	}
	cid := create.ID
	if opts.PreStartCopyTar != nil {
		dest := strings.TrimSpace(opts.PreStartCopyDest)
		if dest == "" {
			dest = CodeBuildWorkspaceDir
		}
		if err := ValidateCodeBuildContainerPath(dest); err != nil {
			_, _ = c.cli.ContainerRemove(context.Background(), cid, client.ContainerRemoveOptions{Force: true})
			return "", err
		}
		if err := c.CopyToContainer(ctx, cid, dest, opts.PreStartCopyTar); err != nil {
			_, _ = c.cli.ContainerRemove(context.Background(), cid, client.ContainerRemoveOptions{Force: true})
			return "", err
		}
	} else if strings.TrimSpace(opts.PreStartCopyDest) != "" {
		_, _ = c.cli.ContainerRemove(context.Background(), cid, client.ContainerRemoveOptions{Force: true})
		return "", fmt.Errorf("compute: PreStartCopyDest set without PreStartCopyTar")
	}
	if _, err := c.cli.ContainerStart(ctx, cid, client.ContainerStartOptions{}); err != nil {
		_, _ = c.cli.ContainerRemove(context.Background(), cid, client.ContainerRemoveOptions{Force: true})
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
	if _, err := c.cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("compute: ecs container stop: %w", err)
	}
	if _, err := c.cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true}); err != nil {
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
	wait := c.cli.ContainerWait(ctx, containerID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-wait.Error:
		if err != nil {
			return -1, fmt.Errorf("compute: ecs container wait: %w", err)
		}
		return 0, nil
	case st := <-wait.Result:
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
	inspRes, err := c.cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("compute: ecs container inspect: %w", err)
	}
	insp := inspRes.Container
	return insp.State != nil && insp.State.Running, nil
}
