// Package compute runs Lambda function code in nested containers via DinD.
//
// Networking (Compose + DinD):
//   - noctaxris talks to dockerd at NOCTAXRIS_DOCKER_HOST (tcp://noctaxris-engine:2375).
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
//     policy hardening beyond Internal networks is deferred.
package compute

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const (
	// FunctionNetworkName is the DinD-internal network for one-shot invokes.
	FunctionNetworkName = "noctaxris-fn"

	// PreferredFunctionImage is the AWS Lambda Python base (one-shot override, not RIE).
	PreferredFunctionImage = "public.ecr.aws/lambda/python:3.12"
	// FallbackFunctionImage is used when the preferred image cannot be pulled.
	FallbackFunctionImage = "python:3.12-slim"

	defaultEndpointURL = "http://host.docker.internal:4566"
)

// Client talks to a nested Docker engine (never the host docker.sock by default).
type Client struct {
	cli *client.Client
}

// NewClient connects to dockerHost (e.g. tcp://noctaxris-engine:2375).
// An empty dockerHost returns an error so callers can treat compute as disabled.
func NewClient(dockerHost string) (*Client, error) {
	if strings.TrimSpace(dockerHost) == "" {
		return nil, fmt.Errorf("compute: NOCTAXRIS_DOCKER_HOST is empty (compute disabled)")
	}
	cli, err := client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
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

// EnsureImage pulls the preferred Lambda Python image, falling back to python:3.12-slim.
func (c *Client) EnsureImage(ctx context.Context) (string, error) {
	if err := c.pullImage(ctx, PreferredFunctionImage); err == nil {
		return PreferredFunctionImage, nil
	} else {
		prefErr := err
		if err := c.pullImage(ctx, FallbackFunctionImage); err != nil {
			return "", fmt.Errorf("compute: pull %s: %v; fallback %s: %w", PreferredFunctionImage, prefErr, FallbackFunctionImage, err)
		}
		return FallbackFunctionImage, nil
	}
}

func (c *Client) pullImage(ctx context.Context, ref string) error {
	rc, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(io.Discard, rc)
	return err
}
