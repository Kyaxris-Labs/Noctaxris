package compute

import (
	"testing"

	"github.com/docker/docker/api/types/network"
)

func TestFunctionNetworkPostureOK(t *testing.T) {
	cases := []struct {
		name string
		insp network.Inspect
		ok   bool
	}{
		{
			name: "lab posture",
			insp: network.Inspect{
				Internal: false,
				Driver:   "bridge",
				Options:  map[string]string{bridgeNoMasquerade: "false"},
			},
			ok: true,
		},
		{
			name: "legacy internal",
			insp: network.Inspect{Internal: true, Driver: "bridge"},
			ok:   false,
		},
		{
			name: "masquerade enabled",
			insp: network.Inspect{
				Internal: false,
				Driver:   "bridge",
				Options:  map[string]string{bridgeNoMasquerade: "true"},
			},
			ok: false,
		},
		{
			name: "missing masquerade option",
			insp: network.Inspect{Internal: false, Driver: "bridge", Options: map[string]string{}},
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := functionNetworkPostureOK(tc.insp); got != tc.ok {
				t.Fatalf("got %v want %v", got, tc.ok)
			}
		})
	}
}
