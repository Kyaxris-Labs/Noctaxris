package sdk_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestProwlerAWSEnumerateSmoke(t *testing.T) {
	if _, err := exec.LookPath("prowler"); err != nil {
		t.Skip("prowler not installed")
	}
	endpointURL := strings.TrimSpace(os.Getenv("AWS_ENDPOINT_URL"))
	if endpointURL == "" {
		endpointURL = strings.TrimSpace(os.Getenv("NOCTAXRIS_ENDPOINT"))
	}
	if endpointURL == "" {
		t.Skip("AWS_ENDPOINT_URL unset")
	}
	requireReady(t)

	cmd := exec.Command("prowler", "aws", "--service", "iam", "--region", "us-east-1")
	cmd.Env = append(os.Environ(),
		"AWS_ENDPOINT_URL="+endpointURL,
		"AWS_EC2_METADATA_DISABLED=true",
		"AWS_DEFAULT_REGION="+envOr("AWS_DEFAULT_REGION", "us-east-1"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err == nil {
			return
		}
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 3 {
			return
		}
		t.Fatalf("prowler aws --service iam: %v", err)
	case <-time.After(3 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatal("prowler aws --service iam timed out")
	}
}
