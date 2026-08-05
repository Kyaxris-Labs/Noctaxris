package compute

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

// ExecOpts configures a one-shot command inside a running nested container.
type ExecOpts struct {
	ContainerID string
	Cmd         []string
	Env         []string
	// Stdin is optional process stdin (for nested HTTP shims that POST JSON).
	Stdin string
}

// ExecResult is stdout/stderr and the process exit code from ContainerExec.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Exec runs Cmd inside ContainerID via the nested Docker engine.
// It never opens host docker.sock; the client must already target DinD TLS.
func (c *Client) Exec(ctx context.Context, opts ExecOpts) (ExecResult, error) {
	if c == nil || c.cli == nil {
		return ExecResult{}, fmt.Errorf("compute: exec client unavailable")
	}
	cid := strings.TrimSpace(opts.ContainerID)
	if cid == "" {
		return ExecResult{}, fmt.Errorf("compute: exec container ID is required")
	}
	if len(opts.Cmd) == 0 {
		return ExecResult{}, fmt.Errorf("compute: exec Cmd is required")
	}
	attachStdin := opts.Stdin != ""
	create, err := c.cli.ExecCreate(ctx, cid, client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  attachStdin,
		Env:          opts.Env,
		Cmd:          opts.Cmd,
	})
	if err != nil {
		return ExecResult{}, fmt.Errorf("compute: exec create: %w", err)
	}
	attach, err := c.cli.ExecAttach(ctx, create.ID, client.ExecAttachOptions{})
	if err != nil {
		return ExecResult{}, fmt.Errorf("compute: exec attach: %w", err)
	}
	defer attach.Close()

	if attachStdin {
		if _, err := io.WriteString(attach.Conn, opts.Stdin); err != nil {
			return ExecResult{}, fmt.Errorf("compute: exec stdin: %w", err)
		}
		_ = attach.CloseWrite()
	}

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attach.Reader); err != nil && err != io.EOF {
		return ExecResult{}, fmt.Errorf("compute: exec copy: %w", err)
	}
	insp, err := c.cli.ExecInspect(ctx, create.ID, client.ExecInspectOptions{})
	if err != nil {
		return ExecResult{}, fmt.Errorf("compute: exec inspect: %w", err)
	}
	return ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: insp.ExitCode,
	}, nil
}
