package compute

import (
	"testing"

	"github.com/moby/moby/api/types/network"
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
				Network: network.Network{
					Internal: false,
					Driver:   "bridge",
					Options:  map[string]string{bridgeNoMasquerade: "false"},
				},
			},
			ok: true,
		},
		{
			name: "legacy internal",
			insp: network.Inspect{
				Network: network.Network{Internal: true, Driver: "bridge"},
			},
			ok: false,
		},
		{
			name: "masquerade enabled",
			insp: network.Inspect{
				Network: network.Network{
					Internal: false,
					Driver:   "bridge",
					Options:  map[string]string{bridgeNoMasquerade: "true"},
				},
			},
			ok: false,
		},
		{
			name: "missing masquerade option",
			insp: network.Inspect{
				Network: network.Network{Internal: false, Driver: "bridge", Options: map[string]string{}},
			},
			ok: false,
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
