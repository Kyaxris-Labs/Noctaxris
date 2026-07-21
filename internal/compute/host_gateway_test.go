package compute

import (
	"testing"
)

func TestHostGatewayExtraHosts(t *testing.T) {
	t.Setenv(EnvInjectHostGateway, "")
	got := hostGatewayExtraHosts()
	if len(got) != 1 || got[0] != "host.docker.internal:host-gateway" {
		t.Fatalf("default ExtraHosts = %#v", got)
	}
	t.Setenv(EnvInjectHostGateway, "0")
	if got := hostGatewayExtraHosts(); got != nil {
		t.Fatalf("disabled ExtraHosts = %#v want nil", got)
	}
}

func TestECSHostGatewayExtraHostsDefaultOff(t *testing.T) {
	t.Setenv(EnvInjectECSHostGateway, "")
	if got := ecsHostGatewayExtraHosts(); got != nil {
		t.Fatalf("default ECS ExtraHosts = %#v want nil", got)
	}
	t.Setenv(EnvInjectECSHostGateway, "0")
	if got := ecsHostGatewayExtraHosts(); got != nil {
		t.Fatalf("disabled ECS ExtraHosts = %#v want nil", got)
	}
	t.Setenv(EnvInjectECSHostGateway, "1")
	got := ecsHostGatewayExtraHosts()
	if len(got) != 1 || got[0] != "host.docker.internal:host-gateway" {
		t.Fatalf("opt-in ECS ExtraHosts = %#v", got)
	}
}

func TestNestedTaskSecurity(t *testing.T) {
	hc := nestedTaskSecurity(256)
	if len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" {
		t.Fatalf("CapDrop=%#v", hc.CapDrop)
	}
	wantMem := int64(256) * 1024 * 1024
	if hc.Memory != wantMem {
		t.Fatalf("Memory=%d want %d", hc.Memory, wantMem)
	}
	hc0 := nestedTaskSecurity(0)
	if hc0.Memory != 0 {
		t.Fatalf("Memory with 0 MB = %d want 0", hc0.Memory)
	}
}
