package store

import (
	"errors"
	"strings"
)

// Lab-supported Lambda runtimes (not the full AWS matrix).
const (
	LambdaRuntimePython311 = "python3.11"
	LambdaRuntimePython312 = "python3.12"
	LambdaRuntimeNodejs20x = "nodejs20.x"
)

var (
	// ErrInvalidRuntime is returned when a zip function uses an unsupported runtime.
	ErrInvalidRuntime = errors.New("ValidationException: invalid runtime")

	supportedLambdaRuntimes = map[string]struct{}{
		LambdaRuntimePython311: {},
		LambdaRuntimePython312: {},
		LambdaRuntimeNodejs20x: {},
	}
)

// SupportedLambdaRuntimes returns the lab runtime allowlist in stable order.
func SupportedLambdaRuntimes() []string {
	return []string{
		LambdaRuntimePython311,
		LambdaRuntimePython312,
		LambdaRuntimeNodejs20x,
	}
}

// ValidateLambdaRuntime checks that runtime is in the lab allowlist.
func ValidateLambdaRuntime(runtime string) error {
	if _, ok := supportedLambdaRuntimes[strings.TrimSpace(runtime)]; !ok {
		return ErrInvalidRuntime
	}
	return nil
}

// LambdaRuntimeValidationMessage is the ValidationException detail for handlers.
func LambdaRuntimeValidationMessage() string {
	return "Runtime must be one of: python3.11, python3.12, nodejs20.x."
}

// IsPythonLambdaRuntime reports whether runtime uses the Python zip/image runner.
func IsPythonLambdaRuntime(runtime string) bool {
	switch strings.TrimSpace(runtime) {
	case LambdaRuntimePython311, LambdaRuntimePython312:
		return true
	default:
		return false
	}
}

// IsNodeLambdaRuntime reports whether runtime uses the Node.js zip runner.
func IsNodeLambdaRuntime(runtime string) bool {
	return strings.TrimSpace(runtime) == LambdaRuntimeNodejs20x
}
