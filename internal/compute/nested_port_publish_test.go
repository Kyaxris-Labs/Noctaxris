package compute

import (
	"strconv"
	"testing"

	"github.com/docker/go-connections/nat"
)

func TestNestedPortPublishDefaultOff(t *testing.T) {
	t.Setenv(EnvNestedPortPublish, "")
	t.Setenv(EnvBrokerPortPublish, "")
	if nestedPortPublishEnabled() {
		t.Fatal("default nested port publish must be off")
	}
	if brokerPortPublishEnabled() {
		t.Fatal("default broker port publish must be off")
	}
	t.Setenv(EnvNestedPortPublish, "0")
	if nestedPortPublishEnabled() {
		t.Fatal("NOCTAXRIS_NESTED_PORT_PUBLISH=0 must stay off")
	}
	if dataPlaneExposedPorts(5432, DataKindRDS) != nil {
		t.Fatal("ExposedPorts must be nil when publish is off")
	}
	hc := dataPlaneHostConfig(5432, DataKindRDS)
	if hc.PortBindings != nil && len(hc.PortBindings) > 0 {
		t.Fatalf("PortBindings must be empty when publish is off, got %#v", hc.PortBindings)
	}
}

func TestNestedPortPublishOptInBindings(t *testing.T) {
	t.Setenv(EnvNestedPortPublish, "1")
	t.Setenv(EnvBrokerPortPublish, "")
	if !nestedPortPublishEnabled() {
		t.Fatal("NOCTAXRIS_NESTED_PORT_PUBLISH=1 must enable publish")
	}
	exposed := dataPlaneExposedPorts(5432, DataKindRDS)
	p, err := nat.NewPort("tcp", "5432")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := exposed[p]; !ok || len(exposed) != 1 {
		t.Fatalf("ExposedPorts=%#v want {%v}", exposed, p)
	}
	hc := dataPlaneHostConfig(5432, DataKindRDS)
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
	hc = dataPlaneHostConfig(6379, DataKindElastiCache)
	p6379, err := nat.NewPort("tcp", strconv.Itoa(6379))
	if err != nil {
		t.Fatal(err)
	}
	if len(hc.PortBindings[p6379]) != 1 || hc.PortBindings[p6379][0].HostPort != "6379" {
		t.Fatalf("true gate PortBindings=%#v", hc.PortBindings)
	}

	t.Setenv(EnvNestedPortPublish, "1")
	if dataPlaneExposedPorts(0, DataKindRDS) != nil {
		t.Fatal("zero containerPort must not expose ports")
	}
	hc0 := dataPlaneHostConfig(0, DataKindRDS)
	if hc0.PortBindings != nil && len(hc0.PortBindings) > 0 {
		t.Fatalf("zero containerPort PortBindings=%#v", hc0.PortBindings)
	}
}

func TestBrokerPortPublishNarrowGate(t *testing.T) {
	t.Setenv(EnvNestedPortPublish, "")
	t.Setenv(EnvBrokerPortPublish, "1")
	if brokerPortPublishEnabled() != true {
		t.Fatal("broker publish gate must be on")
	}

	for _, kind := range []DataKind{DataKindRDS, DataKindElastiCache, DataKindMQ} {
		if dataPlaneExposedPorts(5432, kind) != nil {
			t.Fatalf("kind %q must not expose ports with broker-only gate", kind)
		}
		hc := dataPlaneHostConfig(5432, kind)
		if hc.PortBindings != nil && len(hc.PortBindings) > 0 {
			t.Fatalf("kind %q PortBindings=%#v want empty", kind, hc.PortBindings)
		}
	}

	p9092, err := nat.NewPort("tcp", "9092")
	if err != nil {
		t.Fatal(err)
	}
	exposed := dataPlaneExposedPorts(9092, DataKindMSK)
	if _, ok := exposed[p9092]; !ok {
		t.Fatalf("MSK ExposedPorts=%#v", exposed)
	}
	hcMSK := dataPlaneHostConfig(9092, DataKindMSK)
	if len(hcMSK.PortBindings[p9092]) != 1 || hcMSK.PortBindings[p9092][0].HostPort != "9092" {
		t.Fatalf("MSK PortBindings=%#v", hcMSK.PortBindings)
	}

	p1883, err := nat.NewPort("tcp", "1883")
	if err != nil {
		t.Fatal(err)
	}
	exposedMQTT := dataPlaneExposedPorts(1883, DataKindMQTT)
	if _, ok := exposedMQTT[p1883]; !ok {
		t.Fatalf("MQTT ExposedPorts=%#v", exposedMQTT)
	}
	hcMQTT := dataPlaneHostConfig(1883, DataKindMQTT)
	if len(hcMQTT.PortBindings[p1883]) != 1 || hcMQTT.PortBindings[p1883][0].HostPort != "1883" {
		t.Fatalf("MQTT PortBindings=%#v", hcMQTT.PortBindings)
	}
}

func TestBrokerPortPublishDefaultOffWithoutNested(t *testing.T) {
	t.Setenv(EnvNestedPortPublish, "")
	t.Setenv(EnvBrokerPortPublish, "")
	hc := dataPlaneHostConfig(9092, DataKindMSK)
	if hc.PortBindings != nil && len(hc.PortBindings) > 0 {
		t.Fatalf("MSK PortBindings must be empty by default, got %#v", hc.PortBindings)
	}
}
