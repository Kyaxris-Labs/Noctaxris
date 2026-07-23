package compute

import (
	"testing"
)

func TestZipInvokeHostConfigSecurity(t *testing.T) {
	t.Setenv(EnvInjectHostGateway, "0")
	hc := zipInvokeHostConfig([]string{"/code:/var/task:ro"}, 256)
	if !hostConfigSecurityOK(hc) {
		t.Fatalf("zip HostConfig not hardened: Privileged=%v CapAdd=%#v CapDrop=%#v", hc.Privileged, hc.CapAdd, hc.CapDrop)
	}
	if hc.Memory != 256*1024*1024 {
		t.Fatalf("Memory=%d", hc.Memory)
	}
	if len(hc.Binds) != 1 || hc.Binds[0] != "/code:/var/task:ro" {
		t.Fatalf("Binds=%#v", hc.Binds)
	}
	if string(hc.NetworkMode) != FunctionNetworkName {
		t.Fatalf("NetworkMode=%q", hc.NetworkMode)
	}
}

func TestImageInvokeHostConfigSecurity(t *testing.T) {
	t.Setenv(EnvInjectHostGateway, "0")
	hc := imageInvokeHostConfig([]string{"/evt:/tmp/noctaxris:ro"}, 128)
	if !hostConfigSecurityOK(hc) {
		t.Fatalf("image HostConfig not hardened: Privileged=%v CapAdd=%#v CapDrop=%#v", hc.Privileged, hc.CapAdd, hc.CapDrop)
	}
}

func TestECSTaskHostConfigSecurity(t *testing.T) {
	t.Setenv(EnvInjectECSHostGateway, "")
	hc := ecsTaskHostConfig(512)
	if !hostConfigSecurityOK(hc) {
		t.Fatalf("ecs HostConfig not hardened: Privileged=%v CapAdd=%#v CapDrop=%#v", hc.Privileged, hc.CapAdd, hc.CapDrop)
	}
	if hc.Memory != 512*1024*1024 {
		t.Fatalf("Memory=%d", hc.Memory)
	}
	if string(hc.NetworkMode) != ECSNetworkName {
		t.Fatalf("NetworkMode=%q", hc.NetworkMode)
	}
	if hc.ExtraHosts != nil {
		t.Fatalf("default ECS ExtraHosts=%#v want nil", hc.ExtraHosts)
	}
}

func TestNestedTaskSecurityNoPrivilege(t *testing.T) {
	hc := nestedTaskSecurity(64)
	if hc.Privileged {
		t.Fatal("Privileged must be false")
	}
	if len(hc.CapAdd) != 0 {
		t.Fatalf("CapAdd=%#v want empty", hc.CapAdd)
	}
	if len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" {
		t.Fatalf("CapDrop=%#v", hc.CapDrop)
	}
	found := false
	for _, opt := range hc.SecurityOpt {
		if opt == "no-new-privileges:true" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("SecurityOpt=%#v missing no-new-privileges", hc.SecurityOpt)
	}
}
