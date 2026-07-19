package compute

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"
)

// RunOpts configures a one-shot lab Lambda invoke inside DinD.
type RunOpts struct {
	// CodeHostPath is the unzipped function directory as seen by the DinD daemon
	// (shared noctaxris-data volume, e.g. /var/lib/noctaxris/...).
	CodeHostPath string
	// Handler is "module.function" (python3.12).
	Handler string
	// TimeoutSec is the container wall-clock limit (default 30).
	TimeoutSec int
	// Env is merged into the container environment (execution-role AWS_* keys, etc.).
	Env map[string]string
	// EventJSON is the Invoke payload written to /var/task/.noctaxris-event.json.
	EventJSON string
	// EndpointURL is injected as AWS_ENDPOINT_URL* for the function SDK.
	// Empty defaults to http://host.docker.internal:4566 (Compose Desktop path).
	EndpointURL string
}

// InvokeResult is the one-shot wrapper stdout (JSON) plus raw logs.
type InvokeResult struct {
	Payload []byte
	Logs    string
}

// oneShotPython loads HANDLER (module.function), reads the event file, calls the
// handler once, and prints JSON. Prefer this over keeping aws-lambda-rie up for
// sync lab Invoke.
const oneShotPython = `
import json, os, importlib, sys
handler = os.environ.get("AWS_LAMBDA_FUNCTION_HANDLER") or os.environ["HANDLER"]
if "." not in handler:
    raise SystemExit("handler must be module.function")
mod_name, fn_name = handler.rsplit(".", 1)
sys.path.insert(0, "/var/task")
mod = importlib.import_module(mod_name)
fn = getattr(mod, fn_name)
event_path = os.environ.get("NOCTAXRIS_EVENT_PATH", "/var/task/.noctaxris-event.json")
with open(event_path, "r", encoding="utf-8") as f:
    event = json.load(f)
result = fn(event, None)
print(json.dumps(result))
`

// ValidateRunOpts checks required fields without talking to Docker.
func ValidateRunOpts(opts RunOpts) error {
	if strings.TrimSpace(opts.CodeHostPath) == "" {
		return fmt.Errorf("compute: CodeHostPath is required")
	}
	// DinD is Linux: require a POSIX absolute path (filepath.IsAbs is OS-specific).
	if !strings.HasPrefix(opts.CodeHostPath, "/") {
		return fmt.Errorf("compute: CodeHostPath must be absolute (DinD path)")
	}
	h := strings.TrimSpace(opts.Handler)
	if h == "" || !strings.Contains(h, ".") || strings.HasPrefix(h, ".") || strings.HasSuffix(h, ".") {
		return fmt.Errorf("compute: Handler must be module.function")
	}
	if opts.TimeoutSec < 0 {
		return fmt.Errorf("compute: TimeoutSec must be >= 0")
	}
	return nil
}

// RunInvoke starts a one-shot function container, waits for exit, and returns
// the JSON printed by the Python wrapper.
func (c *Client) RunInvoke(ctx context.Context, opts RunOpts) (InvokeResult, error) {
	if err := ValidateRunOpts(opts); err != nil {
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
	img, err := c.EnsureImage(ctx)
	if err != nil {
		return InvokeResult{}, err
	}

	eventPath := filepath.Join(opts.CodeHostPath, ".noctaxris-event.json")
	if err := os.WriteFile(eventPath, []byte(opts.EventJSON), 0o600); err != nil {
		return InvokeResult{}, fmt.Errorf("compute: write event: %w", err)
	}
	defer os.Remove(eventPath)

	env := []string{
		"AWS_LAMBDA_FUNCTION_HANDLER=" + opts.Handler,
		"HANDLER=" + opts.Handler,
		"NOCTAXRIS_EVENT_PATH=/var/task/.noctaxris-event.json",
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
	}
	for k, v := range opts.Env {
		if k == "" {
			continue
		}
		env = append(env, k+"="+v)
	}

	name := "noctaxris-fn-" + uuid.NewString()
	stopTimeout := timeout
	hostConfig := &container.HostConfig{
		Binds: []string{
			opts.CodeHostPath + ":/var/task:ro",
		},
		AutoRemove:    false,
		NetworkMode:   container.NetworkMode(FunctionNetworkName),
		ExtraHosts:    []string{"host.docker.internal:host-gateway"},
		ReadonlyRootfs: false,
	}

	cfg := &container.Config{
		Image:      img,
		Env:        env,
		WorkingDir: "/var/task",
		// Override Lambda image ENTRYPOINT/CMD: one-shot import, not long-running RIE.
		Entrypoint: []string{"python", "-c", oneShotPython},
		Cmd:        nil,
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

func (c *Client) collectLogs(ctx context.Context, cid string) (logs string, payload []byte, err error) {
	rc, err := c.cli.ContainerLogs(ctx, cid, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return "", nil, err
	}
	defer rc.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, rc); err != nil && err != io.EOF {
		all, _ := io.ReadAll(rc)
		return string(all), bytes.TrimSpace(all), nil
	}
	logs = stdout.String() + stderr.String()
	payload = bytes.TrimSpace(stdout.Bytes())
	return logs, payload, nil
}
