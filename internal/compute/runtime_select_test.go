package compute

import "testing"

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
		{"microvm", "", true},
		{"MICROVM", "", true},
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
