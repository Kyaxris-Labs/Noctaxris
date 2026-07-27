package compute

import (
	"fmt"
	"os"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// EnvImagePullAllowlist extends the default image pull allowlist with comma-separated prefixes.
const EnvImagePullAllowlist = "NOCTAXRIS_IMAGE_PULL_ALLOWLIST"

// AllowImagePull fails closed unless imageRef is the lab registry or a pinned lab base image.
// Attacker-controlled registry hosts are rejected.
//
// listenAddr pins host.docker.internal refs to DinDPullHost(listenAddr) only.
// When listenAddr is empty, DinD host.docker.internal refs are denied (no silent :4566 pin).
// Lab registry host refs (store.LabRegistryHost) still validate without listenAddr.
func AllowImagePull(imageRef, listenAddr string) error {
	ref := strings.TrimSpace(imageRef)
	if ref == "" {
		return fmt.Errorf("compute: image reference is empty")
	}
	if isLabRegistryRef(ref, listenAddr) {
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

func isLabRegistryRef(ref, listenAddr string) bool {
	labPrefix := store.LabRegistryHost + "/"
	if strings.HasPrefix(ref, labPrefix) {
		return isLabECRPath(strings.TrimPrefix(ref, labPrefix))
	}
	// Refuse DinD host checks without an explicit listen pin (no silent :4566 fallback).
	if strings.TrimSpace(listenAddr) == "" {
		return false
	}
	dindPrefix := DinDPullHost(listenAddr) + "/"
	if !strings.HasPrefix(ref, dindPrefix) {
		return false
	}
	return isLabECRPath(strings.TrimPrefix(ref, dindPrefix))
}

// isLabECRPath reports whether path is ACCOUNT/REPO with optional :tag or @digest.
func isLabECRPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "..") {
		return false
	}
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	if !isLabAccountID(parts[0]) {
		return false
	}
	repo := parts[1]
	name := repo
	if i := strings.IndexAny(repo, "@:"); i >= 0 {
		name = repo[:i]
	}
	if name == "" || strings.Contains(name, "/") {
		return false
	}
	return true
}

func isLabAccountID(s string) bool {
	if len(s) != 12 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isPinnedLabImage(ref string) bool {
	lower := strings.ToLower(ref)
	// Digest-pinned public Lambda bases (exact tags also allowed for lab smoke).
	if strings.HasPrefix(lower, "public.ecr.aws/lambda/") && strings.Contains(lower, "@sha256:") {
		rest := strings.TrimPrefix(lower, "public.ecr.aws/lambda/")
		return rest != "" && !strings.Contains(rest, "..")
	}
	pinnedExact := map[string]struct{}{
		"public.ecr.aws/lambda/python:3.12":         {},
		"public.ecr.aws/lambda/python:3.11":         {},
		"public.ecr.aws/lambda/python:3.13":         {},
		"public.ecr.aws/lambda/python:3.14":         {},
		"public.ecr.aws/lambda/nodejs:20":           {},
		"public.ecr.aws/lambda/nodejs:22":           {},
		"public.ecr.aws/lambda/nodejs:24":           {},
		"public.ecr.aws/lambda/java:21":             {},
		"public.ecr.aws/lambda/java:25":             {},
		"python:3.12-slim":                          {},
		"python:3.11-slim":                          {},
		"python:3.13-slim":                          {},
		"python:3.14-slim":                          {},
		"node:20-slim":                              {},
		"node:22-slim":                              {},
		"node:24-slim":                              {},
		"eclipse-temurin:21-jdk":                    {},
		"eclipse-temurin:25-jdk":                    {},
		"eclipse-temurin:21-jre":                    {},
		"eclipse-temurin:25-jre":                    {},
		"alpine:3.20":                               {},
		"public.ecr.aws/docker/library/alpine:3.20": {},
		"postgres:16-alpine":                        {},
		"valkey/valkey:8-alpine":                    {},
		"mongo:7":                                   {},
		"rabbitmq:3.13-alpine":                      {},
		"apache/activemq-classic:5.18.3":            {},
		"opensearchproject/opensearch:2.11.1":       {},
	}
	_, ok := pinnedExact[lower]
	return ok
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
