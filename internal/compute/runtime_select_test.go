package compute

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseComputeRuntime(t *testing.T) {
	cases := []struct {
		in   string
		want string
		err  bool
	}{
		{"", RuntimeDinD, false},
		{"  ", RuntimeDinD, false},
		{"dind", RuntimeDinD, false},
		{"DinD", RuntimeDinD, false},
		{"microvm", RuntimeMicroVM, false},
		{"MICROVM", RuntimeMicroVM, false},
		{"firecracker", "", true},
		{"host", "", true},
		{"docker.sock", "", true},
	}
	for _, tc := range cases {
		got, err := ParseComputeRuntime(tc.in)
		if tc.err {
			if err == nil {
				t.Fatalf("ParseComputeRuntime(%q): want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseComputeRuntime(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseComputeRuntime(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestProbeMicroVMWSL2FailClosed(t *testing.T) {
	res := ProbeMicroVM(MicroVMProbeOpts{
		GOOS:        "linux",
		ProcVersion: "Linux version 6.6.87.2-microsoft-standard-WSL2",
		KVMPath:     "/dev/kvm",
		Stat: func(name string) (os.FileInfo, error) {
			return fakeFileInfo{}, nil
		},
		LookPath: func(string) (string, error) {
			return "/usr/bin/firecracker", nil
		},
	})
	if res.OK {
		t.Fatal("expected WSL2 probe to fail")
	}
	if !res.WSL2 {
		t.Fatal("expected WSL2 flag")
	}
	if !strings.Contains(res.Reason, "WSL2") {
		t.Fatalf("reason=%q", res.Reason)
	}
	if err := RequireMicroVMProbe(MicroVMProbeOpts{
		GOOS:        "linux",
		ProcVersion: "Linux version 6.6.87.2-microsoft-standard-WSL2",
	}); err == nil {
		t.Fatal("expected RequireMicroVMProbe error on WSL2")
	}
}

func TestProbeMicroVMMissingKVM(t *testing.T) {
	res := ProbeMicroVM(MicroVMProbeOpts{
		GOOS:        "linux",
		ProcVersion: "Linux version 6.1.0-generic",
		KVMPath:     "/dev/kvm",
		Stat: func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
		LookPath: func(string) (string, error) {
			return "/usr/bin/firecracker", nil
		},
	})
	if res.OK || res.HasKVM {
		t.Fatalf("expected missing KVM failure: %+v", res)
	}
	if !strings.Contains(res.Reason, "/dev/kvm") {
		t.Fatalf("reason=%q", res.Reason)
	}
}

func TestProbeMicroVMMissingBinary(t *testing.T) {
	res := ProbeMicroVM(MicroVMProbeOpts{
		GOOS:        "linux",
		ProcVersion: "Linux version 6.1.0-generic",
		KVMPath:     "/dev/kvm",
		Stat: func(name string) (os.FileInfo, error) {
			if name == "/dev/kvm" {
				return fakeFileInfo{}, nil
			}
			return nil, os.ErrNotExist
		},
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	})
	if res.OK || res.HasBin {
		t.Fatalf("expected missing binary failure: %+v", res)
	}
	if !strings.Contains(strings.ToLower(res.Reason), "firecracker") {
		t.Fatalf("reason=%q", res.Reason)
	}
}

func TestProbeMicroVMOK(t *testing.T) {
	res := ProbeMicroVM(MicroVMProbeOpts{
		GOOS:            "linux",
		ProcVersion:     "Linux version 6.1.0-generic (debian)",
		KVMPath:         "/dev/kvm",
		FirecrackerPath: "/opt/firecracker/firecracker",
		Stat: func(name string) (os.FileInfo, error) {
			return fakeFileInfo{}, nil
		},
	})
	if !res.OK {
		t.Fatalf("expected OK: %+v", res)
	}
	if res.BinPath != "/opt/firecracker/firecracker" {
		t.Fatalf("BinPath=%q", res.BinPath)
	}
}

func TestProbeMicroVMNonLinux(t *testing.T) {
	res := ProbeMicroVM(MicroVMProbeOpts{GOOS: "windows"})
	if res.OK {
		t.Fatal("expected non-linux failure")
	}
	if !strings.Contains(res.Reason, "Linux") {
		t.Fatalf("reason=%q", res.Reason)
	}
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "kvm" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }
