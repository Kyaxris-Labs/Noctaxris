package compute

import (
	"context"
	"fmt"
)

// MicroVMRunner is the opt-in Firecracker-class Lambda/ECS path.
// ProbeMicroVM gates platform readiness (Linux, non-WSL2, KVM, firecracker binary).
// Even when the probe succeeds, guest zip/Image Invoke and ECS RunTask fail closed
// until kernel/rootfs assets and a real guest runner ship. There is no fake boot
// and no fallthrough to host docker.sock or silent DinD.
type MicroVMRunner struct {
	ProbeOpts MicroVMProbeOpts
}

// errMicroVMGuestNotPackaged is returned when the host probe passes but this
// build has no live Firecracker guest path (kernel/rootfs + runner).
const errMicroVMGuestNotPackaged = "compute: microVM guest boot is not packaged in this build (Linux+KVM with Firecracker binary is required, and kernel/rootfs guest assets are not shipped); use DinD (NOCTAXRIS_COMPUTE_RUNTIME=dind or unset)"

// NewMicroVMRunner builds a fail-closed microVM invoker.
// Probe runs at construction so opt-in misconfiguration fails early.
// A successful construct only means the host can select microVM; it does not
// mean live guest Invoke/RunTask will succeed.
func NewMicroVMRunner(opts MicroVMProbeOpts) (*MicroVMRunner, error) {
	if err := RequireMicroVMProbe(opts); err != nil {
		return nil, err
	}
	return &MicroVMRunner{ProbeOpts: opts}, nil
}

// RunInvoke rejects zip Invoke: no live Firecracker guest is packaged.
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
	return InvokeResult{}, fmt.Errorf("%s", errMicroVMGuestNotPackaged)
}

// RunImageInvoke rejects Image Invoke: no live Firecracker guest is packaged.
func (r *MicroVMRunner) RunImageInvoke(ctx context.Context, opts ImageRunOpts) (InvokeResult, error) {
	_ = ctx
	if err := ValidateImageRunOpts(opts); err != nil {
		return InvokeResult{}, err
	}
	if err := RequireMicroVMProbe(r.probeOpts()); err != nil {
		return InvokeResult{}, err
	}
	return InvokeResult{}, fmt.Errorf("%s", errMicroVMGuestNotPackaged)
}

// RunECSTask rejects ECS RunTask: no live Firecracker guest is packaged.
func (r *MicroVMRunner) RunECSTask(ctx context.Context, opts ECSRunOpts) (string, error) {
	_ = ctx
	if err := ValidateECSRunOpts(opts); err != nil {
		return "", err
	}
	if err := RequireMicroVMProbe(r.probeOpts()); err != nil {
		return "", err
	}
	return "", fmt.Errorf("%s", errMicroVMGuestNotPackaged)
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
