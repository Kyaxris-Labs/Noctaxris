package compute

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/google/uuid"
)

// ImageRunOpts configures a one-shot lab Lambda invoke from a container image.
type ImageRunOpts struct {
	// ImageURI is the pullable container image reference (e.g. public ECR Lambda base).
	ImageURI string
	// Handler is "module.function" for Lambda Python base images.
	Handler string
	// TimeoutSec is the container wall-clock limit (default 30).
	TimeoutSec int
	// MemoryMB is the optional Docker memory limit in megabytes (0 = engine default).
	MemoryMB int
	// Env is merged into the container environment (execution-role AWS_* keys, etc.).
	Env map[string]string
	// EventJSON is the Invoke payload written under EventHostPath.
	EventJSON string
	// EndpointURL is injected as AWS_ENDPOINT_URL* for the function SDK.
	EndpointURL string
	// EventHostPath is a DinD-visible writable directory for the event file.
	EventHostPath string
	// LayerHostPaths are unpacked layer directories as seen by DinD (mounted at /opt).
	LayerHostPaths []string
	// LabRegistryPull requests an authenticated pull via PullLabRegistryImage (lab ECR).
	LabRegistryPull bool
	// RegistryUsername is the Docker registry username (lab: "AWS").
	RegistryUsername string
	// RegistryPassword is the lab ECR authorization token password.
	RegistryPassword string
	// AllowDefaultEntrypoint permits running the image ENTRYPOINT/CMD when no
	// one-shot runtime match exists (lab ECR full-container exec). Fail-closed otherwise.
	AllowDefaultEntrypoint bool
}

// ValidateImageRunOpts checks required fields without talking to Docker.
func ValidateImageRunOpts(opts ImageRunOpts) error {
	if strings.TrimSpace(opts.ImageURI) == "" {
		return fmt.Errorf("compute: ImageURI is required")
	}
	if err := AllowImagePull(opts.ImageURI); err != nil {
		return err
	}
	if strings.TrimSpace(opts.EventHostPath) == "" {
		return fmt.Errorf("compute: EventHostPath is required")
	}
	if !strings.HasPrefix(opts.EventHostPath, "/") {
		return fmt.Errorf("compute: EventHostPath must be absolute (DinD path)")
	}
	h := strings.TrimSpace(opts.Handler)
	if h == "" || !strings.Contains(h, ".") || strings.HasPrefix(h, ".") || strings.HasSuffix(h, ".") {
		return fmt.Errorf("compute: Handler must be module.function")
	}
	if opts.TimeoutSec < 0 {
		return fmt.Errorf("compute: TimeoutSec must be >= 0")
	}
	if err := RequireLabRegistryCreds(opts.LabRegistryPull, LabRegistryPullCreds{
		Username: opts.RegistryUsername,
		Password: opts.RegistryPassword,
	}); err != nil {
		return err
	}
	return nil
}

// RunImageInvoke starts a one-shot container from ImageURI and returns stdout JSON.
func (c *Client) RunImageInvoke(ctx context.Context, opts ImageRunOpts) (InvokeResult, error) {
	if err := ValidateImageRunOpts(opts); err != nil {
		return InvokeResult{}, err
	}
	timeout := opts.TimeoutSec
	if timeout == 0 {
		timeout = 30
	}
	endpoint := strings.TrimSpace(opts.EndpointURL)
	if endpoint == "" {
		endpoint = defaultEndpointURL
	}

	if _, err := c.EnsureNetwork(ctx); err != nil {
		return InvokeResult{}, err
	}
	if opts.LabRegistryPull {
		if err := c.PullLabRegistryImage(ctx, opts.ImageURI, opts.RegistryUsername, opts.RegistryPassword); err != nil {
			return InvokeResult{}, fmt.Errorf("compute: pull lab registry image %s: %w", opts.ImageURI, err)
		}
	} else if err := c.pullImage(ctx, opts.ImageURI); err != nil {
		return InvokeResult{}, fmt.Errorf("compute: pull image %s: %w", opts.ImageURI, err)
	}

	if err := os.MkdirAll(opts.EventHostPath, store.LabSharedDirMode); err != nil {
		return InvokeResult{}, fmt.Errorf("compute: mkdir event dir: %w", err)
	}
	if err := os.Chmod(opts.EventHostPath, store.LabSharedDirMode); err != nil {
		return InvokeResult{}, fmt.Errorf("compute: chmod event dir: %w", err)
	}
	eventPath := filepath.Join(opts.EventHostPath, ".noctaxris-event.json")
	if err := os.WriteFile(eventPath, []byte(opts.EventJSON), store.LabSharedFileMode); err != nil {
		return InvokeResult{}, fmt.Errorf("compute: write event: %w", err)
	}
	if err := os.Chmod(eventPath, store.LabSharedFileMode); err != nil {
		return InvokeResult{}, fmt.Errorf("compute: chmod event: %w", err)
	}
	defer os.RemoveAll(opts.EventHostPath)

	mergedOptDir := ""
	if len(opts.LayerHostPaths) > 0 {
		mergedOptDir = filepath.Join(opts.EventHostPath, ".noctaxris-opt")
		if err := MergeLayerDirs(opts.LayerHostPaths, mergedOptDir); err != nil {
			return InvokeResult{}, err
		}
	}

	env := []string{
		"AWS_LAMBDA_FUNCTION_HANDLER=" + opts.Handler,
		"HANDLER=" + opts.Handler,
		"NOCTAXRIS_EVENT_PATH=/tmp/noctaxris/.noctaxris-event.json",
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_ENDPOINT_URL=" + endpoint,
		"AWS_ENDPOINT_URL_STS=" + endpoint,
		"AWS_ENDPOINT_URL_S3=" + endpoint,
		"AWS_ENDPOINT_URL_DYNAMODB=" + endpoint,
		"AWS_ENDPOINT_URL_SQS=" + endpoint,
		"AWS_ENDPOINT_URL_LAMBDA=" + endpoint,
		"AWS_ENDPOINT_URL_IAM=" + endpoint,
		"AWS_ENDPOINT_URL_KMS=" + endpoint,
		"PYTHONPATH=/var/task",
		"LAMBDA_TASK_ROOT=/var/task",
		"LAMBDA_RUNTIME_DIR=/var/runtime",
	}
	for k, v := range opts.Env {
		if k == "" {
			continue
		}
		env = append(env, k+"="+v)
	}

	name := "noctaxris-img-" + uuid.NewString()
	stopTimeout := timeout
	binds := []string{
		opts.EventHostPath + ":/tmp/noctaxris:ro",
	}
	if mergedOptDir != "" {
		binds = append(binds, mergedOptDir+":/opt:ro")
	}
	hostConfig := imageInvokeHostConfig(binds, opts.MemoryMB)

	cfg := &container.Config{
		Image:      opts.ImageURI,
		Env:        env,
		WorkingDir: "/var/task",
	}
	if cmd, ok := imageOneShotCommand(opts.ImageURI); ok {
		cfg.Entrypoint = []string{cmd.Exe, cmd.Flag, cmd.Script}
		cfg.Cmd = nil
	} else if !opts.AllowDefaultEntrypoint {
		return InvokeResult{}, fmt.Errorf("compute: image %q has no recognized one-shot entrypoint (set lab ImageConfig/AllowDefaultEntrypoint for full-container exec)", opts.ImageURI)
	}

	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			FunctionNetworkName: {},
		},
	}

	create, err := c.cli.ContainerCreate(ctx, cfg, hostConfig, netCfg, nil, name)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("compute: container create: %w", err)
	}
	cid := create.ID
	defer func() {
		_ = c.cli.ContainerRemove(context.Background(), cid, container.RemoveOptions{Force: true})
	}()

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	if err := c.cli.ContainerStart(runCtx, cid, container.StartOptions{}); err != nil {
		return InvokeResult{}, fmt.Errorf("compute: container start: %w", err)
	}

	statusCh, errCh := c.cli.ContainerWait(runCtx, cid, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		_ = c.cli.ContainerStop(context.Background(), cid, container.StopOptions{Timeout: &stopTimeout})
		return InvokeResult{}, fmt.Errorf("compute: container wait: %w", err)
	case st := <-statusCh:
		if st.Error != nil && st.Error.Message != "" {
			return InvokeResult{}, fmt.Errorf("compute: container wait: %s", st.Error.Message)
		}
		logs, payload, logErr := c.collectLogs(runCtx, cid)
		if logErr != nil {
			return InvokeResult{Logs: logs}, fmt.Errorf("compute: logs: %w", logErr)
		}
		if st.StatusCode != 0 {
			return InvokeResult{Payload: payload, Logs: logs}, fmt.Errorf("compute: container exit %d", st.StatusCode)
		}
		return InvokeResult{Payload: payload, Logs: logs}, nil
	case <-runCtx.Done():
		_ = c.cli.ContainerStop(context.Background(), cid, container.StopOptions{Timeout: &stopTimeout})
		logs, _, _ := c.collectLogs(context.Background(), cid)
		return InvokeResult{Logs: logs}, fmt.Errorf("compute: invoke timeout after %ds: %w", timeout, runCtx.Err())
	}
}
