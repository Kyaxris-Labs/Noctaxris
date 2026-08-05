package compute

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const (
	// bridgeNoMasquerade disables SNAT so containers can reach the Docker host
	// gateway (lab API via host.docker.internal) without a public-internet route.
	bridgeNoMasquerade = "com.docker.network.bridge.enable_ip_masquerade"
)

// ensureInternalNetwork returns an existing Internal managed network by name,
// or creates one. Reuse refuses networks that are not Internal (CI-06).
// Used for nested data-plane engines (no host-gateway reachability required).
func (c *Client) ensureInternalNetwork(ctx context.Context, name string) (string, error) {
	if c == nil || c.cli == nil {
		return "", fmt.Errorf("compute: client unavailable")
	}
	networks, err := c.cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return "", fmt.Errorf("compute: list networks: %w", err)
	}
	for _, n := range networks.Items {
		if n.Name != name {
			continue
		}
		inspRes, err := c.cli.NetworkInspect(ctx, n.ID, client.NetworkInspectOptions{})
		if err != nil {
			return "", fmt.Errorf("compute: inspect network %s: %w", name, err)
		}
		insp := inspRes.Network
		if !insp.Internal {
			return "", fmt.Errorf("compute: network %s exists but is not Internal (refuse reuse)", name)
		}
		return n.ID, nil
	}
	resp, err := c.cli.NetworkCreate(ctx, name, client.NetworkCreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels: map[string]string{
			LabelManaged: "true",
		},
	})
	if err != nil {
		return "", fmt.Errorf("compute: create network %s: %w", name, err)
	}
	return resp.ID, nil
}

// ensureFunctionNetwork returns (or creates) the lab function network.
// Posture: not Internal, IP masquerade disabled. That keeps WAN egress closed
// while still allowing routes to the Docker host gateway used by
// host.docker.internal → published Noctaxris API (4566).
// Legacy Internal:true networks with the same name are removed when unused, else refused.
func (c *Client) ensureFunctionNetwork(ctx context.Context, name string) (string, error) {
	if c == nil || c.cli == nil {
		return "", fmt.Errorf("compute: client unavailable")
	}
	networks, err := c.cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return "", fmt.Errorf("compute: list networks: %w", err)
	}
	for _, n := range networks.Items {
		if n.Name != name {
			continue
		}
		inspRes, err := c.cli.NetworkInspect(ctx, n.ID, client.NetworkInspectOptions{})
		if err != nil {
			return "", fmt.Errorf("compute: inspect network %s: %w", name, err)
		}
		insp := inspRes.Network
		if functionNetworkPostureOK(insp) {
			return n.ID, nil
		}
		if len(insp.Containers) > 0 {
			return "", fmt.Errorf("compute: network %s exists with incompatible egress posture and has active containers (remove or recreate)", name)
		}
		if _, err := c.cli.NetworkRemove(ctx, n.ID, client.NetworkRemoveOptions{}); err != nil {
			return "", fmt.Errorf("compute: remove incompatible network %s: %w", name, err)
		}
		break
	}
	resp, err := c.cli.NetworkCreate(ctx, name, client.NetworkCreateOptions{
		Driver:   "bridge",
		Internal: false,
		Options: map[string]string{
			bridgeNoMasquerade: "false",
		},
		Labels: map[string]string{
			LabelManaged: "true",
		},
	})
	if err != nil {
		return "", fmt.Errorf("compute: create function network %s: %w", name, err)
	}
	return resp.ID, nil
}

func functionNetworkPostureOK(insp network.Inspect) bool {
	if insp.Internal {
		return false
	}
	if insp.Driver != "" && !strings.EqualFold(insp.Driver, "bridge") {
		return false
	}
	masq, ok := insp.Options[bridgeNoMasquerade]
	if !ok {
		// Missing option defaults to masquerade enabled on bridge → refuse (WAN open).
		return false
	}
	return strings.EqualFold(masq, "false") || masq == "0"
}
