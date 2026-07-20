package compute

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Compute runtime names for NOCTAXRIS_COMPUTE_RUNTIME.
const (
	RuntimeDinD    = "dind"
	RuntimeMicroVM = "microvm"
)

// ParseComputeRuntime maps an env value to a runtime name.
// Empty or whitespace defaults to DinD. Unknown values fail closed.
func ParseComputeRuntime(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", RuntimeDinD:
		return RuntimeDinD, nil
	case RuntimeMicroVM:
		return RuntimeMicroVM, nil
	default:
		return "", fmt.Errorf("compute: unknown NOCTAXRIS_COMPUTE_RUNTIME %q (want dind or microvm)", raw)
	}
}

// MicroVMProbeResult is the outcome of a platform probe for Firecracker-class isolation.
type MicroVMProbeResult struct {
	OK      bool
	Reason  string
	WSL2    bool
	HasKVM  bool
	HasBin  bool
	BinPath string
}

// MicroVMProbeOpts controls probe lookups. Empty fields use process defaults.
type MicroVMProbeOpts struct {
	// GOOS overrides runtime.GOOS (tests).
	GOOS string
	// ProcVersion is /proc/version contents (tests). Empty reads from disk on Linux.
	ProcVersion string
	// KVMPath defaults to /dev/kvm.
	KVMPath string
	// FirecrackerPath is an explicit binary path (NOCTAXRIS_FIRECRACKER_BIN).
	FirecrackerPath string
	// LookPath finds a binary on PATH. Defaults to execLookPath.
	LookPath func(file string) (string, error)
	// Stat is os.Stat. Defaults to os.Stat.
	Stat func(name string) (os.FileInfo, error)
	// ReadFile defaults to os.ReadFile.
	ReadFile func(name string) ([]byte, error)
}

// ProbeMicroVM checks whether the opt-in microVM path can run on this host.
// WSL2 is always unsupported (product policy). Missing KVM or Firecracker binary fails closed.
// Never opens host docker.sock.
func ProbeMicroVM(opts MicroVMProbeOpts) MicroVMProbeResult {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	stat := opts.Stat
	if stat == nil {
		stat = os.Stat
	}
	readFile := opts.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = execLookPath
	}
	kvmPath := opts.KVMPath
	if kvmPath == "" {
		kvmPath = "/dev/kvm"
	}

	res := MicroVMProbeResult{}

	if goos != "linux" {
		res.Reason = "microVM compute requires Linux with KVM (this GOOS is unsupported); use DinD (NOCTAXRIS_COMPUTE_RUNTIME=dind or unset)"
		return res
	}

	procVersion := opts.ProcVersion
	if procVersion == "" {
		if b, err := readFile("/proc/version"); err == nil {
			procVersion = string(b)
		}
	}
	if isWSL2(procVersion, os.Getenv("WSL_DISTRO_NAME")) {
		res.WSL2 = true
		res.Reason = "microVM compute is unsupported on WSL2; use DinD (NOCTAXRIS_COMPUTE_RUNTIME=dind or unset)"
		return res
	}

	if fi, err := stat(kvmPath); err == nil && !fi.IsDir() {
		res.HasKVM = true
	}
	if !res.HasKVM {
		res.Reason = "microVM compute requires /dev/kvm; KVM is missing or unusable; use DinD (NOCTAXRIS_COMPUTE_RUNTIME=dind or unset)"
		return res
	}

	bin := strings.TrimSpace(opts.FirecrackerPath)
	if bin != "" {
		if fi, err := stat(bin); err == nil && !fi.IsDir() {
			res.HasBin = true
			res.BinPath = bin
		}
	} else if p, err := lookPath("firecracker"); err == nil && p != "" {
		res.HasBin = true
		res.BinPath = p
	}
	if !res.HasBin {
		res.Reason = "microVM compute requires the firecracker binary (set NOCTAXRIS_FIRECRACKER_BIN or install on PATH); use DinD (NOCTAXRIS_COMPUTE_RUNTIME=dind or unset)"
		return res
	}

	res.OK = true
	return res
}

func isWSL2(procVersion, wslDistroName string) bool {
	if strings.TrimSpace(wslDistroName) != "" {
		return true
	}
	lower := strings.ToLower(procVersion)
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

// RequireMicroVMProbe returns a clear error when the microVM path cannot run.
func RequireMicroVMProbe(opts MicroVMProbeOpts) error {
	res := ProbeMicroVM(opts)
	if res.OK {
		return nil
	}
	if res.Reason == "" {
		return fmt.Errorf("compute: microVM probe failed")
	}
	return fmt.Errorf("compute: %s", res.Reason)
}
