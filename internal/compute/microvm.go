package compute

import (
	"context"
	"fmt"
)

// MicroVMRunner is the opt-in Firecracker-class Lambda/ECS path.
// Until a Linux+KVM spike lands a real guest runner, all invokes and RunTask
// calls fail closed after ProbeMicroVM (no host docker.sock, no silent DinD fallthrough).
type MicroVMRunner struct {
	ProbeOpts MicroVMProbeOpts
}

// NewMicroVMRunner builds a fail-closed microVM invoker.
// Probe runs at construction so opt-in misconfiguration fails early.
func NewMicroVMRunner(opts MicroVMProbeOpts) (*MicroVMRunner, error) {
	if err := RequireMicroVMProbe(opts); err != nil {
		return nil, err
	}
	return &MicroVMRunner{ProbeOpts: opts}, nil
}

// RunInvoke rejects zip Invoke until a real Firecracker guest path ships.
// Callers that skip NewMicroVMRunner and construct MicroVMRunner directly still
// re-probe so unsupported platforms cannot invoke.
func (r *MicroVMRunner) RunInvoke(ctx context.Context, opts RunOpts) (InvokeResult, error) {
	_ = ctx
	if err := ValidateRunOpts(opts); err != nil {
		return InvokeResult{}, err
	}
	if err := RequireMicroVMProbe(r.probeOpts()); err != nil {
		return InvokeResult{}, err
	}
	return InvokeResult{}, fmt.Errorf("compute: microVM zip Invoke is not implemented on this build (Firecracker guest path deferred); use DinD (NOCTAXRIS_COMPUTE_RUNTIME=dind or unset)")
}

// RunImageInvoke rejects Image Invoke until a real Firecracker guest path ships.
func (r *MicroVMRunner) RunImageInvoke(ctx context.Context, opts ImageRunOpts) (InvokeResult, error) {
	_ = ctx
	if err := ValidateImageRunOpts(opts); err != nil {
		return InvokeResult{}, err
	}
	if err := RequireMicroVMProbe(r.probeOpts()); err != nil {
		return InvokeResult{}, err
	}
	return InvokeResult{}, fmt.Errorf("compute: microVM Image Invoke is not implemented on this build (Firecracker guest path deferred); use DinD (NOCTAXRIS_COMPUTE_RUNTIME=dind or unset)")
}

// RunECSTask rejects ECS RunTask until a real Firecracker guest path ships.
func (r *MicroVMRunner) RunECSTask(ctx context.Context, opts ECSRunOpts) (string, error) {
	_ = ctx
	if err := ValidateECSRunOpts(opts); err != nil {
		return "", err
	}
	if err := RequireMicroVMProbe(r.probeOpts()); err != nil {
		return "", err
	}
	return "", fmt.Errorf("compute: microVM ECS RunTask is not implemented on this build (Firecracker guest path deferred); use DinD (NOCTAXRIS_COMPUTE_RUNTIME=dind or unset)")
}

func (r *MicroVMRunner) probeOpts() MicroVMProbeOpts {
	if r == nil {
		return MicroVMProbeOpts{}
	}
	return r.ProbeOpts
}

// FunctionInvoker is the Lambda one-shot invoke surface shared by DinD and microVM.
type FunctionInvoker interface {
	RunInvoke(ctx context.Context, opts RunOpts) (InvokeResult, error)
	RunImageInvoke(ctx context.Context, opts ImageRunOpts) (InvokeResult, error)
}

// InvokerConfig selects the Lambda compute runtime.
type InvokerConfig struct {
	Runtime           string // dind (default) or microvm
	DockerHost        string
	DockerTLSCertPath string
	FirecrackerBin    string
	// ProbeOverrides is optional (tests). Nil uses process defaults.
	ProbeOverrides *MicroVMProbeOpts
}

// NewFunctionInvoker returns a DinD Client or a microVM runner.
// Unknown runtime values must be rejected by ParseComputeRuntime before call.
// microVM fails closed on unsupported platforms and never opens host docker.sock.
func NewFunctionInvoker(cfg InvokerConfig) (FunctionInvoker, error) {
	kind, err := ParseComputeRuntime(cfg.Runtime)
	if err != nil {
		return nil, err
	}
	switch kind {
	case RuntimeDinD:
		return NewClient(cfg.DockerHost, cfg.DockerTLSCertPath)
	case RuntimeMicroVM:
		opts := MicroVMProbeOpts{FirecrackerPath: cfg.FirecrackerBin}
		if cfg.ProbeOverrides != nil {
			opts = *cfg.ProbeOverrides
			if opts.FirecrackerPath == "" {
				opts.FirecrackerPath = cfg.FirecrackerBin
			}
		}
		return NewMicroVMRunner(opts)
	default:
		return nil, fmt.Errorf("compute: unknown runtime %q", kind)
	}
}
