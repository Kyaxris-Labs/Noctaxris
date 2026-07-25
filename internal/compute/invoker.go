package compute

import (
	"context"
	"fmt"
)

// FunctionInvoker is the Lambda one-shot invoke surface (nested DinD Client).
type FunctionInvoker interface {
	RunInvoke(ctx context.Context, opts RunOpts) (InvokeResult, error)
	RunImageInvoke(ctx context.Context, opts ImageRunOpts) (InvokeResult, error)
}

// InvokerConfig selects the Lambda compute path. Only DinD is supported.
type InvokerConfig struct {
	Runtime           string // dind (default) or empty
	DockerHost        string
	DockerTLSCertPath string
	ListenAddr        string
}

// NewFunctionInvoker returns a DinD Client for Lambda zip/Image Invoke.
// Unknown runtime values must be rejected by ParseComputeRuntime before call.
func NewFunctionInvoker(cfg InvokerConfig) (FunctionInvoker, error) {
	kind, err := ParseComputeRuntime(cfg.Runtime)
	if err != nil {
		return nil, err
	}
	switch kind {
	case RuntimeDinD:
		return NewClient(cfg.DockerHost, cfg.DockerTLSCertPath, cfg.ListenAddr)
	default:
		return nil, fmt.Errorf("compute: unknown runtime %q", kind)
	}
}
