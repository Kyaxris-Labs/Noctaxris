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
//     Linux setups). Function containers get ExtraHosts host.docker.internal:host-gateway.
//
// Egress deny:
//   - EnsureNetwork creates noctaxris-fn with Internal:true so functions have no
//     default route to the public internet. Reaching the Noctaxris API still depends
//     on host.docker.internal / host-gateway working on the platform. Full egress
//     policy hardening beyond Internal networks is out of scope for the lab core (see docs/services/lambda.md).
package compute

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const (
	// FunctionNetworkName is the DinD-internal network for one-shot invokes.
	FunctionNetworkName = "noctaxris-fn"

	defaultEndpointURL = "http://host.docker.internal:4566"
)

// Client talks to a nested Docker engine (never the host docker.sock by default).
type Client struct {
	cli *client.Client
}

// NewClient connects to dockerHost (e.g. tcp://noctaxris-engine:2376).
// When tlsCertPath is non-empty, client TLS is enabled using ca.pem, cert.pem,
// and key.pem under that directory (Compose: NOCTAXRIS_DOCKER_CERT_PATH=/certs/client).
// An empty dockerHost returns an error so callers can treat compute as disabled.
// Host must be allowlisted (default tcp://noctaxris-engine:2376); unix://, npipe://,
// and docker.sock are always rejected. TLS client PEMs are required when host is set.
func NewClient(dockerHost, tlsCertPath string) (*Client, error) {
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
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("compute: docker client: %w", err)
	}
	return &Client{cli: cli}, nil
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
	if _, err := c.cli.Ping(ctx); err != nil {
		return fmt.Errorf("compute: ping engine: %w", err)
	}
	return nil
}

// EnsureNetwork creates (or reuses) an Internal Docker network for function
// containers. Internal:true is best-effort egress deny (no public internet route).
func (c *Client) EnsureNetwork(ctx context.Context) (string, error) {
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("compute: list networks: %w", err)
	}
	for _, n := range networks {
		if n.Name == FunctionNetworkName {
			return n.ID, nil
		}
	}
	resp, err := c.cli.NetworkCreate(ctx, FunctionNetworkName, network.CreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels: map[string]string{
			"noctaxris.managed": "true",
		},
	})
	if err != nil {
		return "", fmt.Errorf("compute: create network %s: %w", FunctionNetworkName, err)
	}
	return resp.ID, nil
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
	if err := AllowImagePull(ref); err != nil {
		return err
	}
	rc, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(io.Discard, rc)
	return err
}
