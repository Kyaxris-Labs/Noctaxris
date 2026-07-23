package opensearch

import "strings"

// Operator-facing hints for nested OpenSearch CreateFailed (fail-closed).
// Noctaxris never applies host sysctl from inside the API container.
const (
	HintMaxMapCount = "Nested OpenSearch failed host mmap limits (vm.max_map_count). " +
		"Raise the DinD/host VM to at least 262144 before CreateDomain " +
		"(Linux: sysctl -w vm.max_map_count=262144; Docker Desktop/WSL: set inside the Desktop/WSL VM, not only Windows). " +
		"See docs/services/opensearch.md. Domain stays CreateFailed (fail-closed)."

	HintMemoryLock = "Nested OpenSearch failed memory lock (bootstrap.memory_lock / mlockall). " +
		"Noctaxris sets bootstrap.memory_lock=false; if logs still mention lock, check memlock ulimits on the DinD host. " +
		"See docs/services/opensearch.md. Domain stays CreateFailed (fail-closed)."

	HintNestedGeneric = "Nested OpenSearch did not become healthy. Domain is CreateFailed (fail-closed). " +
		"If nested logs mention vm.max_map_count or memory lock, raise the DinD/host VM sysctl/ulimit " +
		"(see docs/services/opensearch.md). Noctaxris does not change host sysctl from the container."
)

// ClassifyOpenSearchNestedFailure returns an operator hint when evidence
// (nested logs, wait/start error text) matches known bootstrap failures.
// Unknown evidence yields HintNestedGeneric when non-empty, else "".
func ClassifyOpenSearchNestedFailure(evidence string) string {
	text := strings.TrimSpace(evidence)
	if text == "" {
		return ""
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "max_map_count"),
		strings.Contains(lower, "max virtual memory areas"):
		return HintMaxMapCount
	case strings.Contains(lower, "memory lock"),
		strings.Contains(lower, "bootstrap.memory_lock"),
		strings.Contains(lower, "unable to lock jvm memory"),
		strings.Contains(lower, "mlockall"):
		return HintMemoryLock
	default:
		return HintNestedGeneric
	}
}
