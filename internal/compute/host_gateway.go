package compute

import (
	"os"
	"strings"
)

// EnvInjectHostGateway controls ExtraHosts host.docker.internal:host-gateway
// injection into nested function/ECS containers. Set to "0" to omit (labs that
// do not need in-function SDK calls to the host-published API).
const EnvInjectHostGateway = "NOCTAXRIS_INJECT_HOST_GATEWAY"

// hostGatewayExtraHosts returns ExtraHosts for DinD children, or nil when disabled.
func hostGatewayExtraHosts() []string {
	v := strings.TrimSpace(os.Getenv(EnvInjectHostGateway))
	if v == "0" || strings.EqualFold(v, "false") {
		return nil
	}
	return []string{"host.docker.internal:host-gateway"}
}
