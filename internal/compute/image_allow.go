package compute

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// EnvImagePullAllowlist extends the default image pull allowlist with comma-separated prefixes.
const EnvImagePullAllowlist = "NOCTAXRIS_IMAGE_PULL_ALLOWLIST"

// AllowImagePull fails closed unless imageRef is the lab registry or a pinned lab base image.
// Attacker-controlled registry hosts are rejected.
func AllowImagePull(imageRef string) error {
	ref := strings.TrimSpace(imageRef)
	if ref == "" {
		return fmt.Errorf("compute: image reference is empty")
	}
	if isLabRegistryRef(ref) {
		return nil
	}
	if isPinnedLabImage(ref) {
		return nil
	}
	for _, prefix := range extraImageAllowPrefixes() {
		if prefix == "" {
			continue
		}
		if ref == prefix || strings.HasPrefix(ref, prefix) {
			if imageAllowPrefixNeedsDigest(prefix) && !strings.Contains(ref, "@sha256:") {
				return fmt.Errorf("compute: allowlisted image %q must be pinned by digest (@sha256:...)", ref)
			}
			return nil
		}
	}
	return fmt.Errorf("compute: image pull host not allowlisted: %q (lab registry or pinned bases only)", ref)
}

func isLabRegistryRef(ref string) bool {
	if strings.HasPrefix(ref, store.LabRegistryHost+"/") {
		return true
	}
	// DinD rewrite: host.docker.internal:PORT/ACCOUNT/REPO:tag
	const dindHost = "host.docker.internal:"
	if !strings.HasPrefix(ref, dindHost) {
		return false
	}
	rest := strings.TrimPrefix(ref, dindHost)
	portAndPath := strings.SplitN(rest, "/", 2)
	if len(portAndPath) != 2 || portAndPath[0] == "" || portAndPath[1] == "" {
		return false
	}
	if _, err := net.LookupPort("tcp", portAndPath[0]); err != nil {
		return false
	}
	return true
}

func isPinnedLabImage(ref string) bool {
	lower := strings.ToLower(ref)
	pinnedExact := map[string]struct{}{
		"public.ecr.aws/lambda/python:3.12":             {},
		"public.ecr.aws/lambda/python:3.11":             {},
		"public.ecr.aws/lambda/nodejs:20":               {},
		"python:3.12-slim":                              {},
		"python:3.11-slim":                              {},
		"node:20-slim":                                  {},
		"alpine:3.20":                                   {},
		"public.ecr.aws/docker/library/alpine:3.20":     {},
		"postgres:16-alpine":                            {},
		"valkey/valkey:8-alpine":                        {},
		"mongo:7":                                       {},
	}
	if _, ok := pinnedExact[lower]; ok {
		return true
	}
	// Documented Lambda public ECR path, including tag variants used in labs/tests.
	if strings.HasPrefix(lower, "public.ecr.aws/lambda/") {
		rest := strings.TrimPrefix(lower, "public.ecr.aws/lambda/")
		return rest != "" && !strings.Contains(rest, "..")
	}
	return false
}

func imageAllowPrefixNeedsDigest(prefix string) bool {
	// Registry hosts (contain a dot or port) outside the built-in pin list need digests.
	host := prefix
	if i := strings.IndexAny(prefix, "/@"); i >= 0 {
		host = prefix[:i]
	}
	return strings.Contains(host, ".") || strings.Contains(host, ":")
}

func extraImageAllowPrefixes() []string {
	raw := strings.TrimSpace(os.Getenv(EnvImagePullAllowlist))
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
