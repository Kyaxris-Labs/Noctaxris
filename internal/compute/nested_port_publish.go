package compute

import (
	"os"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

// EnvNestedPortPublish gates engine-side PortBindings for nested data containers.
// Default is off. Set to "1" (or use docker/compose.lab-nested-ports.yaml) so
// StartDataPlane maps the container listen port onto the DinD host (noctaxris-engine).
// Operator loopback reachability still requires the Compose overlay publishing those
// engine ports to 127.0.0.1; a plain ports: map on the API service cannot reach DinD.
const EnvNestedPortPublish = "NOCTAXRIS_NESTED_PORT_PUBLISH"

// EnvBrokerPortPublish gates engine-side PortBindings for shared broker containers
// (MSK/Kafka and MQTT) when blanket NOCTAXRIS_NESTED_PORT_PUBLISH is off.
const EnvBrokerPortPublish = "NOCTAXRIS_BROKER_PORT_PUBLISH"

// nestedPortPublishEnabled reports whether nested data containers may bind ports
// on the DinD engine host. Default false.
func nestedPortPublishEnabled() bool {
	v := strings.TrimSpace(os.Getenv(EnvNestedPortPublish))
	return v == "1" || strings.EqualFold(v, "true")
}

// BrokerPortPublishReady reports whether shared MSK/MQTT engine PortBindings are allowed.
func BrokerPortPublishReady() bool {
	return nestedPortPublishEnabled() || brokerPortPublishEnabled()
}

// brokerPortPublishEnabled reports whether shared MSK/MQTT containers may bind
// broker ports on the DinD engine when blanket nested publish is off. Default false.
func brokerPortPublishEnabled() bool {
	v := strings.TrimSpace(os.Getenv(EnvBrokerPortPublish))
	return v == "1" || strings.EqualFold(v, "true")
}

// dataPlanePortPublishEnabled reports whether StartDataPlane should apply engine
// PortBindings for the given kind and container port.
func dataPlanePortPublishEnabled(kind DataKind, containerPort int) bool {
	if containerPort <= 0 {
		return false
	}
	if nestedPortPublishEnabled() {
		return true
	}
	if !brokerPortPublishEnabled() {
		return false
	}
	return kind == DataKindMSK || kind == DataKindMQTT
}

// dataPlaneExposedPorts returns ExposedPorts when port publish is enabled for kind.
// Empty when disabled or containerPort is non-positive.
func dataPlaneExposedPorts(containerPort int, kind DataKind) network.PortSet {
	if !dataPlanePortPublishEnabled(kind, containerPort) {
		return nil
	}
	p, err := network.ParsePort(strconv.Itoa(containerPort) + "/tcp")
	if err != nil {
		return nil
	}
	return network.PortSet{p: struct{}{}}
}

// applyDataPlanePortPublish sets PortBindings on hc when the opt-in gate is on.
// HostIP is left empty so the binding is reachable on the DinD engine's eth0
// (required for Compose to forward noctaxris-engine published ports). Operator
// loopback restriction is enforced by the Compose overlay (127.0.0.1:…), not here.
func applyDataPlanePortPublish(hc *container.HostConfig, containerPort int, kind DataKind) {
	if hc == nil || !dataPlanePortPublishEnabled(kind, containerPort) {
		return
	}
	p, err := network.ParsePort(strconv.Itoa(containerPort) + "/tcp")
	if err != nil {
		return
	}
	hc.PortBindings = network.PortMap{
		p: []network.PortBinding{{
			HostPort: strconv.Itoa(containerPort),
		}},
	}
}
