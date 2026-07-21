package server

import (
	"fmt"
	"strings"
)

const (
	maxLambdaTimeoutSec = 900
	minLambdaTimeoutSec = 1
	minLambdaMemoryMB   = 128
	maxLambdaMemoryMB   = 10240
)

// reservedLambdaEnvExact are keys AWS (and this lab) reject in function Environment.
// See https://docs.aws.amazon.com/lambda/latest/dg/configuration-envvars.md
var reservedLambdaEnvExact = map[string]struct{}{
	"AWS_ACCESS_KEY":        {},
	"AWS_ACCESS_KEY_ID":     {},
	"AWS_SECRET_ACCESS_KEY": {},
	"AWS_SESSION_TOKEN":     {},
	"AWS_SECURITY_TOKEN":    {},
	"AWS_REGION":            {},
	"AWS_DEFAULT_REGION":    {},
	"_HANDLER":             {},
	"_X_AMZN_TRACE_ID":     {},
	"LAMBDA_TASK_ROOT":     {},
	"LAMBDA_RUNTIME_DIR":   {},
}

func isReservedLambdaEnvKey(key string) bool {
	k := strings.TrimSpace(key)
	if k == "" {
		return false
	}
	if _, ok := reservedLambdaEnvExact[k]; ok {
		return true
	}
	if strings.HasPrefix(k, "AWS_LAMBDA_") {
		return true
	}
	if strings.HasPrefix(k, "AWS_ENDPOINT_URL") {
		return true
	}
	return false
}

func validateLambdaEnvKeys(env map[string]string) error {
	for k := range env {
		if isReservedLambdaEnvKey(k) {
			return fmt.Errorf("Lambda was unable to configure your environment variables because the environment variables you have provided contained reserved keys that are currently not supported for modification. Reserved keys used in this request: %s", k)
		}
	}
	return nil
}

func clampLambdaTimeout(n int) int {
	if n < minLambdaTimeoutSec {
		return minLambdaTimeoutSec
	}
	if n > maxLambdaTimeoutSec {
		return maxLambdaTimeoutSec
	}
	return n
}

func clampLambdaMemory(n int) int {
	if n < minLambdaMemoryMB {
		return minLambdaMemoryMB
	}
	if n > maxLambdaMemoryMB {
		return maxLambdaMemoryMB
	}
	return n
}

// mergeLambdaInvokeEnv applies function Environment first, then overlays minted
// execution-role credentials and lab endpoint URLs so Configure cannot clobber them.
func mergeLambdaInvokeEnv(fnEnv map[string]string, minted map[string]string) map[string]string {
	out := make(map[string]string, len(fnEnv)+len(minted))
	for k, v := range fnEnv {
		if k == "" {
			continue
		}
		out[k] = v
	}
	for k, v := range minted {
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}
