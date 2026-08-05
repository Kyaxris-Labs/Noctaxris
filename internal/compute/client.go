// Package compute runs Lambda function code in nested containers via DinD.
//
// Networking (Compose + DinD):
//   - noctaxris talks to dockerd at NOCTAXRIS_DOCKER_HOST (tcp://noctaxris-engine:2376)
//     with TLS client PEMs under NOCTAXRIS_DOCKER_CERT_PATH (/certs/client in Compose).
//   - Function containers are created inside DinD, on DinD's bridge, not on the
//     outer Compose network, so they cannot resolve the "noctaxris" service name.
//   - EndpointURL for invokes should be http://host.docker.internal:4566 under
//     Docker Desktop (host publishes 127.0.0.1:4566). Override with
//     NOCTAXRIS_LAMBDA_ENDPOINT_URL when host.docker.internal is wrong (e.g. some
//     Linux setups). Function ExtraHosts host.docker.internal:host-gateway is opt-in
//     (NOCTAXRIS_INJECT_HOST_GATEWAY=1). ECS/CodeBuild/Batch (Internal noctaxris-ecs)
//     omit ExtraHosts by default; set NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1 to opt in.
//
// Egress deny + lab API allow:
//   - EnsureNetwork creates noctaxris-fn as a bridge with IP masquerade disabled
//     (not Internal). Functions can reach the Docker host gateway
//     (host.docker.internal → published API) without SNAT to the public internet.
//   - Data-plane networks stay Internal:true (see EnsureDataPlaneNetwork).
//     Nested PortBindings stay off unless NOCTAXRIS_NESTED_PORT_PUBLISH=1
//     (pair with docker/compose.lab-nested-ports.yaml for 127.0.0.1 publish).
package compute

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/moby/moby/client"
)

const (
	// FunctionNetworkName is the DinD-internal network for one-shot invokes.
	FunctionNetworkName = "noctaxris-fn"

	defaultEndpointURL = "http://host.docker.internal:4566"
)

// Client talks to a nested Docker engine (never the host docker.sock by default).
type Client struct {
	cli        *client.Client
	listenAddr string
}

// NewClient connects to dockerHost (e.g. tcp://noctaxris-engine:2376).
// When tlsCertPath is non-empty, client TLS is enabled using ca.pem, cert.pem,
// and key.pem under that directory (Compose: NOCTAXRIS_DOCKER_CERT_PATH=/certs/client).
// An empty dockerHost returns an error so callers can treat compute as disabled.
// Host must be allowlisted (default tcp://noctaxris-engine:2376); unix://, npipe://,
// and docker.sock are always rejected. TLS client PEMs are required when host is set.
// listenAddr is the API listen address used to pin DinD lab registry image pulls.
func NewClient(dockerHost, tlsCertPath, listenAddr string) (*Client, error) {
	if strings.TrimSpace(dockerHost) == "" {
		return nil, fmt.Errorf("compute: NOCTAXRIS_DOCKER_HOST is empty (compute disabled)")
	}
	if err := ValidateDockerHost(dockerHost, tlsCertPath); err != nil {
		return nil, err
	}
	opts := []client.Opt{
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	}
	p := strings.TrimSpace(tlsCertPath)
	opts = append(opts, client.WithTLSClientConfig(
		filepath.Join(p, "ca.pem"),
		filepath.Join(p, "cert.pem"),
		filepath.Join(p, "key.pem"),
	))
	cli, err := client.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("compute: docker client: %w", err)
	}
	return &Client{cli: cli, listenAddr: strings.TrimSpace(listenAddr)}, nil
}

// Close releases the underlying Docker HTTP client.
func (c *Client) Close() error {
	if c == nil || c.cli == nil {
		return nil
	}
	return c.cli.Close()
}

// Ping checks that the nested engine is reachable.
func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.cli.Ping(ctx, client.PingOptions{}); err != nil {
		return fmt.Errorf("compute: ping engine: %w", err)
	}
	return nil
}

// EnsureNetwork creates (or reuses) the function Docker network with lab-API
// host-gateway reachability and WAN egress deny (masquerade off).
// Legacy Internal:true networks are replaced when unused; otherwise fail closed.
func (c *Client) EnsureNetwork(ctx context.Context) (string, error) {
	return c.ensureFunctionNetwork(ctx, FunctionNetworkName)
}

// EnsureImage pulls the preferred Lambda runtime image, falling back to a slim variant.
func (c *Client) EnsureImage(ctx context.Context, runtime string) (string, error) {
	imgs, err := FunctionImagesForRuntime(runtime)
	if err != nil {
		return "", err
	}
	if err := c.pullImage(ctx, imgs.Preferred); err == nil {
		return imgs.Preferred, nil
	} else {
		prefErr := err
		if err := c.pullImage(ctx, imgs.Fallback); err != nil {
			return "", fmt.Errorf("compute: pull %s: %v; fallback %s: %w", imgs.Preferred, prefErr, imgs.Fallback, err)
		}
		return imgs.Fallback, nil
	}
}

func (c *Client) pullImage(ctx context.Context, ref string) error {
	listen := ""
	if c != nil {
		listen = c.listenAddr
	}
	if err := AllowImagePull(ref, listen); err != nil {
		return err
	}
	rc, err := c.cli.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(io.Discard, rc)
	return err
}
