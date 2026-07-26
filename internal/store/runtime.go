package store

import (
	"errors"
	"strings"
)

// Lab-supported Lambda runtimes (not the full AWS matrix).
const (
	LambdaRuntimePython311 = "python3.11"
	LambdaRuntimePython312 = "python3.12"
	LambdaRuntimePython313 = "python3.13"
	LambdaRuntimePython314 = "python3.14"
	LambdaRuntimeNodejs20x = "nodejs20.x"
	LambdaRuntimeNodejs22x = "nodejs22.x"
	LambdaRuntimeNodejs24x = "nodejs24.x"
	LambdaRuntimeJava21    = "java21"
	LambdaRuntimeJava25    = "java25"
)

var (
	// ErrInvalidRuntime is returned when a zip function uses an unsupported runtime.
	ErrInvalidRuntime = errors.New("ValidationException: invalid runtime")

	supportedLambdaRuntimes = map[string]struct{}{
		LambdaRuntimePython311: {},
		LambdaRuntimePython312: {},
		LambdaRuntimePython313: {},
		LambdaRuntimePython314: {},
		LambdaRuntimeNodejs20x: {},
		LambdaRuntimeNodejs22x: {},
		LambdaRuntimeNodejs24x: {},
		LambdaRuntimeJava21:    {},
		LambdaRuntimeJava25:    {},
	}
)

// SupportedLambdaRuntimes returns the lab runtime allowlist in stable order.
func SupportedLambdaRuntimes() []string {
	return []string{
		LambdaRuntimePython311,
		LambdaRuntimePython312,
		LambdaRuntimePython313,
		LambdaRuntimePython314,
		LambdaRuntimeNodejs20x,
		LambdaRuntimeNodejs22x,
		LambdaRuntimeNodejs24x,
		LambdaRuntimeJava21,
		LambdaRuntimeJava25,
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
	return "Runtime must be one of: python3.11, python3.12, python3.13, python3.14, nodejs20.x, nodejs22.x, nodejs24.x, java21, java25."
}

// IsPythonLambdaRuntime reports whether runtime uses the Python zip/image runner.
func IsPythonLambdaRuntime(runtime string) bool {
	switch strings.TrimSpace(runtime) {
	case LambdaRuntimePython311, LambdaRuntimePython312, LambdaRuntimePython313, LambdaRuntimePython314:
		return true
	default:
		return false
	}
}

// IsNodeLambdaRuntime reports whether runtime uses the Node.js zip runner.
func IsNodeLambdaRuntime(runtime string) bool {
	switch strings.TrimSpace(runtime) {
	case LambdaRuntimeNodejs20x, LambdaRuntimeNodejs22x, LambdaRuntimeNodejs24x:
		return true
	default:
		return false
	}
}

// IsJavaLambdaRuntime reports whether runtime uses the Java zip/image runner.
func IsJavaLambdaRuntime(runtime string) bool {
	switch strings.TrimSpace(runtime) {
	case LambdaRuntimeJava21, LambdaRuntimeJava25:
		return true
	default:
		return false
	}
}
