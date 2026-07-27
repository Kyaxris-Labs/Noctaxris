package compute

import (
	"strconv"
	"testing"

	"github.com/docker/go-connections/nat"
)

func TestNestedPortPublishDefaultOff(t *testing.T) {
	t.Setenv(EnvNestedPortPublish, "")
	if nestedPortPublishEnabled() {
		t.Fatal("default nested port publish must be off")
	}
	t.Setenv(EnvNestedPortPublish, "0")
	if nestedPortPublishEnabled() {
		t.Fatal("NOCTAXRIS_NESTED_PORT_PUBLISH=0 must stay off")
	}
	if dataPlaneExposedPorts(5432) != nil {
		t.Fatal("ExposedPorts must be nil when publish is off")
	}
	hc := dataPlaneHostConfig(5432)
	if hc.PortBindings != nil && len(hc.PortBindings) > 0 {
		t.Fatalf("PortBindings must be empty when publish is off, got %#v", hc.PortBindings)
	}
}

func TestNestedPortPublishOptInBindings(t *testing.T) {
	t.Setenv(EnvNestedPortPublish, "1")
	if !nestedPortPublishEnabled() {
		t.Fatal("NOCTAXRIS_NESTED_PORT_PUBLISH=1 must enable publish")
	}
	exposed := dataPlaneExposedPorts(5432)
	p, err := nat.NewPort("tcp", "5432")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := exposed[p]; !ok || len(exposed) != 1 {
		t.Fatalf("ExposedPorts=%#v want {%v}", exposed, p)
	}
	hc := dataPlaneHostConfig(5432)
	bindings := hc.PortBindings[p]
	if len(bindings) != 1 {
		t.Fatalf("PortBindings=%#v", hc.PortBindings)
	}
	if bindings[0].HostPort != "5432" {
		t.Fatalf("HostPort=%q want 5432", bindings[0].HostPort)
	}
	// Empty HostIP: bind on DinD engine eth0 so Compose can forward; operator
	// loopback is the Compose overlay (127.0.0.1), not this DinD-side map.
	if bindings[0].HostIP != "" {
		t.Fatalf("HostIP=%q want empty (engine-side eth0)", bindings[0].HostIP)
	}

	t.Setenv(EnvNestedPortPublish, "true")
	hc = dataPlaneHostConfig(6379)
	p6379, err := nat.NewPort("tcp", strconv.Itoa(6379))
	if err != nil {
		t.Fatal(err)
	}
	if len(hc.PortBindings[p6379]) != 1 || hc.PortBindings[p6379][0].HostPort != "6379" {
		t.Fatalf("true gate PortBindings=%#v", hc.PortBindings)
	}

	t.Setenv(EnvNestedPortPublish, "1")
	if dataPlaneExposedPorts(0) != nil {
		t.Fatal("zero containerPort must not expose ports")
	}
	hc0 := dataPlaneHostConfig(0)
	if hc0.PortBindings != nil && len(hc0.PortBindings) > 0 {
		t.Fatalf("zero containerPort PortBindings=%#v", hc0.PortBindings)
	}
}
