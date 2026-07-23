package compute

import (
	"fmt"
	"strings"
)

// Compute runtime names for NOCTAXRIS_COMPUTE_RUNTIME.
const (
	RuntimeDinD = "dind"
)

// ParseComputeRuntime maps an env value to a runtime name.
// Empty or whitespace defaults to DinD. Unknown values (including removed
// microVM/Firecracker selectors) fail closed.
func ParseComputeRuntime(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", RuntimeDinD:
		return RuntimeDinD, nil
	default:
		return "", fmt.Errorf("compute: unknown NOCTAXRIS_COMPUTE_RUNTIME %q (want dind or unset)", raw)
	}
}
