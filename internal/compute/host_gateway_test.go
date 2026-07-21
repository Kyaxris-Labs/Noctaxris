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
