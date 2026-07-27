package compute

import (
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

// EnvNestedPortPublish gates engine-side PortBindings for nested data containers.
// Default is off. Set to "1" (or use docker/compose.lab-nested-ports.yaml) so
// StartDataPlane maps the container listen port onto the DinD host (noctaxris-engine).
// Operator loopback reachability still requires the Compose overlay publishing those
// engine ports to 127.0.0.1; a plain ports: map on the API service cannot reach DinD.
const EnvNestedPortPublish = "NOCTAXRIS_NESTED_PORT_PUBLISH"

// nestedPortPublishEnabled reports whether nested data containers may bind ports
// on the DinD engine host. Default false.
func nestedPortPublishEnabled() bool {
	v := strings.TrimSpace(os.Getenv(EnvNestedPortPublish))
	return v == "1" || strings.EqualFold(v, "true")
}

// dataPlaneExposedPorts returns ExposedPorts when nested port publish is on.
// Empty when disabled or containerPort is non-positive.
func dataPlaneExposedPorts(containerPort int) nat.PortSet {
	if !nestedPortPublishEnabled() || containerPort <= 0 {
		return nil
	}
	p, err := nat.NewPort("tcp", strconv.Itoa(containerPort))
	if err != nil {
		return nil
	}
	return nat.PortSet{p: struct{}{}}
}

// applyDataPlanePortPublish sets PortBindings on hc when the opt-in gate is on.
// HostIP is left empty so the binding is reachable on the DinD engine's eth0
// (required for Compose to forward noctaxris-engine published ports). Operator
// loopback restriction is enforced by the Compose overlay (127.0.0.1:…), not here.
func applyDataPlanePortPublish(hc *container.HostConfig, containerPort int) {
	if hc == nil || !nestedPortPublishEnabled() || containerPort <= 0 {
		return
	}
	p, err := nat.NewPort("tcp", strconv.Itoa(containerPort))
	if err != nil {
		return
	}
	hc.PortBindings = nat.PortMap{
		p: []nat.PortBinding{{
			HostPort: strconv.Itoa(containerPort),
		}},
	}
}
