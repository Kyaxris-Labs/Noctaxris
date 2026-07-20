package compute

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestNewFunctionInvokerDefaultDinD(t *testing.T) {
	inv, err := NewFunctionInvoker(InvokerConfig{
		Runtime:    "",
		DockerHost: "tcp://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	cli, ok := inv.(*Client)
	if !ok || cli == nil {
		t.Fatalf("got %T, want *Client", inv)
	}
	_ = cli.Close()
}

func TestNewFunctionInvokerUnknownFailsClosed(t *testing.T) {
	_, err := NewFunctionInvoker(InvokerConfig{Runtime: "host-docker"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("err=%v", err)
	}
}

func TestNewFunctionInvokerMicroVMWSL2FailsClosed(t *testing.T) {
	_, err := NewFunctionInvoker(InvokerConfig{
		Runtime: RuntimeMicroVM,
		ProbeOverrides: &MicroVMProbeOpts{
			GOOS:        "linux",
			ProcVersion: "Linux version 6.6.0-microsoft-standard-WSL2",
			Stat: func(string) (os.FileInfo, error) {
				return fakeFileInfo{}, nil
			},
			LookPath: func(string) (string, error) {
				return "/usr/bin/firecracker", nil
			},
		},
	})
	if err == nil {
		t.Fatal("expected microVM WSL2 failure")
	}
	if !strings.Contains(err.Error(), "WSL2") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "docker.sock") {
		t.Fatalf("must not mention docker.sock fallthrough: %v", err)
	}
}

func TestMicroVMRunnerRejectsInvokeAfterForcedConstruct(t *testing.T) {
	// Simulate a runner that somehow skipped NewMicroVMRunner: probe still fails closed.
	r := &MicroVMRunner{ProbeOpts: MicroVMProbeOpts{
		GOOS:        "linux",
		ProcVersion: "Linux version 6.6.0-microsoft-standard-WSL2",
	}}
	_, err := r.RunInvoke(context.Background(), RunOpts{
		CodeHostPath: "/var/lib/noctaxris/lambda/code",
		Runtime:      "python3.12",
		Handler:      "handler.handler",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "WSL2") {
		t.Fatalf("err=%v", err)
	}
}

func TestMicroVMRunnerNotImplementedWhenProbeOK(t *testing.T) {
	r, err := NewMicroVMRunner(MicroVMProbeOpts{
		GOOS:            "linux",
		ProcVersion:     "Linux version 6.1.0-generic",
		FirecrackerPath: "/opt/firecracker/firecracker",
		Stat: func(string) (os.FileInfo, error) {
			return fakeFileInfo{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.RunInvoke(context.Background(), RunOpts{
		CodeHostPath: "/var/lib/noctaxris/lambda/code",
		Runtime:      "python3.12",
		Handler:      "handler.handler",
	})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("want not implemented, got %v", err)
	}
	_, err = r.RunImageInvoke(context.Background(), ImageRunOpts{
		ImageURI:      "public.ecr.aws/lambda/python:3.12",
		Handler:       "handler.handler",
		EventHostPath: "/var/lib/noctaxris/lambda/events",
	})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("want not implemented image path, got %v", err)
	}
	_, err = r.RunECSTask(context.Background(), ECSRunOpts{ImageURI: "alpine:3.20"})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("want not implemented ECS path, got %v", err)
	}
}
