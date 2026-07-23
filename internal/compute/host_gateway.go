package compute

import (
	"os"
	"strings"
)

// EnvInjectHostGateway controls ExtraHosts host.docker.internal:host-gateway
// injection into nested Lambda function containers. Default is off; set to "1" for
// in-function SDK labs that call the host-published API.
const EnvInjectHostGateway = "NOCTAXRIS_INJECT_HOST_GATEWAY"

// EnvInjectECSHostGateway controls ExtraHosts injection for nested ECS / CodeBuild /
// Batch containers on Internal network noctaxris-ecs. Default is off; set to "1" to
// enable host-gateway reachability from those tasks.
const EnvInjectECSHostGateway = "NOCTAXRIS_INJECT_ECS_HOST_GATEWAY"

// hostGatewayExtraHosts returns ExtraHosts for Lambda DinD children, or nil when disabled.
// Default is nil; opt in with NOCTAXRIS_INJECT_HOST_GATEWAY=1.
func hostGatewayExtraHosts() []string {
	v := strings.TrimSpace(os.Getenv(EnvInjectHostGateway))
	if v == "1" || strings.EqualFold(v, "true") {
		return []string{"host.docker.internal:host-gateway"}
	}
	return nil
}

// ecsHostGatewayExtraHosts returns ExtraHosts for ECS-path DinD children.
// Default is nil (Internal network honesty). Opt in with NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1.
func ecsHostGatewayExtraHosts() []string {
	v := strings.TrimSpace(os.Getenv(EnvInjectECSHostGateway))
	if v == "1" || strings.EqualFold(v, "true") {
		return []string{"host.docker.internal:host-gateway"}
	}
	return nil
}
