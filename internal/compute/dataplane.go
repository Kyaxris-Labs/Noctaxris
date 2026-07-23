package compute

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/google/uuid"
)

const (
	// DataPlaneNetworkName is the DinD-internal network for nested data engines.
	// Containers on this network are not published to the operator host.
	DataPlaneNetworkName = "noctaxris-data"

	// LabelDataKind marks nested data containers (values: rds|elasticache|docdb).
	LabelDataKind = "noctaxris.data"

	// LabelManaged marks Noctaxris-managed Docker objects.
	LabelManaged = "noctaxris.managed"

	defaultPostgresImage = "postgres:16-alpine"
	defaultValkeyImage   = "valkey/valkey:8-alpine"
	defaultMongoImage    = "mongo:7"

	defaultPostgresPort = 5432
	defaultRedisPort    = 6379
	defaultMongoPort    = 27017
)

// DataKind identifies which nested data engine family a container belongs to.
type DataKind string

const (
	DataKindRDS         DataKind = "rds"
	DataKindElastiCache DataKind = "elasticache"
	DataKindDocDB       DataKind = "docdb"
)

// DataPlaneOpts configures a nested data-engine container inside DinD.
//
// Host port publish is intentionally unsupported. There is no PortBindings /
// PublishPorts field. Callers must reach engines via nested-network endpoints
// or HTTP facades on :4566 (for example RDS Data API).
type DataPlaneOpts struct {
	Kind  DataKind
	Image string
	// Name is an optional container name. Empty selects a generated name.
	Name string
	Env  map[string]string
	// Labels are merged onto the container (noctaxris.data and noctaxris.managed
	// are always set and cannot be overridden away).
	Labels map[string]string
	// ContainerPort is the engine listen port inside the nested network.
	// Zero selects the default for Kind (5432 / 6379 / 27017).
	ContainerPort int
}

// DataPlaneInstance describes a started (or inspected) nested data container.
type DataPlaneInstance struct {
	ContainerID string
	Name        string
	Kind        DataKind
	// Endpoint is host:port on the nested data network (not published on the host).
	Endpoint string
	Image    string
	Running  bool
}

// ValidateDataPlaneOpts checks required fields without talking to Docker.
func ValidateDataPlaneOpts(opts DataPlaneOpts) error {
	switch opts.Kind {
	case DataKindRDS, DataKindElastiCache, DataKindDocDB:
	default:
		return fmt.Errorf("compute: data-plane Kind must be rds, elasticache, or docdb")
	}
	if strings.TrimSpace(opts.Image) == "" {
		return fmt.Errorf("compute: data-plane Image is required")
	}
	if opts.ContainerPort < 0 {
		return fmt.Errorf("compute: data-plane ContainerPort must be >= 0")
	}
	return nil
}

// DefaultDataPlaneImage returns the pinned lab image for a kind when callers
// omit Image. Empty kind returns empty.
func DefaultDataPlaneImage(kind DataKind) string {
	switch kind {
	case DataKindRDS:
		return defaultPostgresImage
	case DataKindElastiCache:
		return defaultValkeyImage
	case DataKindDocDB:
		return defaultMongoImage
	default:
		return ""
	}
}

// DefaultDataPlanePort returns the nested listen port for a kind.
func DefaultDataPlanePort(kind DataKind) int {
	switch kind {
	case DataKindRDS:
		return defaultPostgresPort
	case DataKindElastiCache:
		return defaultRedisPort
	case DataKindDocDB:
		return defaultMongoPort
	default:
		return 0
	}
}

// NestedDataEndpoint builds the documented nested-network endpoint string.
func NestedDataEndpoint(containerName string, port int) string {
	containerName = strings.TrimSpace(containerName)
	if containerName == "" {
		containerName = "unknown"
	}
	if port <= 0 {
		port = defaultPostgresPort
	}
	return fmt.Sprintf("%s:%d", containerName, port)
}

// dataPlaneHostConfig returns HostConfig with no host port publish and CapDrop ALL.
// Exported via tests in this package to lock the secure-default invariant.
func dataPlaneHostConfig() *container.HostConfig {
	sec := nestedTaskSecurity(0)
	return &container.HostConfig{
		AutoRemove:      false,
		NetworkMode:     container.NetworkMode(DataPlaneNetworkName),
		PublishAllPorts: false,
		// PortBindings intentionally nil/empty: never map DB ports to the host.
		Privileged:  false,
		CapDrop:     append([]string(nil), sec.CapDrop...),
		SecurityOpt: append([]string(nil), sec.SecurityOpt...),
	}
}

// EnsureDataPlaneNetwork creates (or reuses) an Internal Docker network for
// nested data engines. Internal:true is best-effort egress deny.
// Existing networks that are not Internal are refused (fail closed).
func (c *Client) EnsureDataPlaneNetwork(ctx context.Context) (string, error) {
	if c == nil || c.cli == nil {
		return "", fmt.Errorf("compute: data-plane client unavailable")
	}
	return c.ensureInternalNetwork(ctx, DataPlaneNetworkName)
}

// StartDataPlane pulls (once), creates, and starts a nested data container.
// It never publishes host ports. Pull failure fails closed.
func (c *Client) StartDataPlane(ctx context.Context, opts DataPlaneOpts) (DataPlaneInstance, error) {
	if c == nil || c.cli == nil {
		return DataPlaneInstance{}, fmt.Errorf("compute: data-plane client unavailable")
	}
	if strings.TrimSpace(opts.Image) == "" {
		opts.Image = DefaultDataPlaneImage(opts.Kind)
	}
	if err := ValidateDataPlaneOpts(opts); err != nil {
		return DataPlaneInstance{}, err
	}
	port := opts.ContainerPort
	if port == 0 {
		port = DefaultDataPlanePort(opts.Kind)
	}

	if _, err := c.EnsureDataPlaneNetwork(ctx); err != nil {
		return DataPlaneInstance{}, err
	}
	if err := c.pullImage(ctx, opts.Image); err != nil {
		return DataPlaneInstance{}, fmt.Errorf("compute: pull image %s: %w", opts.Image, err)
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = fmt.Sprintf("noctaxris-data-%s-%s", opts.Kind, uuid.NewString())
	}

	labels := map[string]string{
		LabelManaged:  "true",
		LabelDataKind: string(opts.Kind),
	}
	for k, v := range opts.Labels {
		if k == "" || k == LabelManaged || k == LabelDataKind {
			continue
		}
		labels[k] = v
	}

	env := make([]string, 0, len(opts.Env))
	for k, v := range opts.Env {
		if k == "" {
			continue
		}
		env = append(env, k+"="+v)
	}

	cfg := &container.Config{
		Image:  opts.Image,
		Env:    env,
		Labels: labels,
	}
	hostConfig := dataPlaneHostConfig()
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			DataPlaneNetworkName: {},
		},
	}

	create, err := c.cli.ContainerCreate(ctx, cfg, hostConfig, netCfg, nil, name)
	if err != nil {
		return DataPlaneInstance{}, fmt.Errorf("compute: data-plane container create: %w", err)
	}
	cid := create.ID
	if err := c.cli.ContainerStart(ctx, cid, container.StartOptions{}); err != nil {
		_ = c.cli.ContainerRemove(context.Background(), cid, container.RemoveOptions{Force: true})
		return DataPlaneInstance{}, fmt.Errorf("compute: data-plane container start: %w", err)
	}

	return DataPlaneInstance{
		ContainerID: cid,
		Name:        name,
		Kind:        opts.Kind,
		Endpoint:    NestedDataEndpoint(name, port),
		Image:       opts.Image,
		Running:     true,
	}, nil
}

// StopDataPlane stops and removes a nested data container by Docker ID.
func (c *Client) StopDataPlane(ctx context.Context, containerID string) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("compute: data-plane client unavailable")
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	timeout := 10
	if err := c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("compute: data-plane container stop: %w", err)
		}
	}
	if err := c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("compute: data-plane container remove: %w", err)
	}
	return nil
}

// InspectDataPlane returns nested endpoint metadata for a container ID.
func (c *Client) InspectDataPlane(ctx context.Context, containerID string) (DataPlaneInstance, error) {
	if c == nil || c.cli == nil {
		return DataPlaneInstance{}, fmt.Errorf("compute: data-plane client unavailable")
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return DataPlaneInstance{}, fmt.Errorf("compute: container ID is required")
	}
	insp, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return DataPlaneInstance{}, fmt.Errorf("compute: data-plane container not found")
		}
		return DataPlaneInstance{}, fmt.Errorf("compute: data-plane container inspect: %w", err)
	}
	kind := DataKind(insp.Config.Labels[LabelDataKind])
	port := DefaultDataPlanePort(kind)
	name := strings.TrimPrefix(insp.Name, "/")
	return DataPlaneInstance{
		ContainerID: insp.ID,
		Name:        name,
		Kind:        kind,
		Endpoint:    NestedDataEndpoint(name, port),
		Image:       insp.Config.Image,
		Running:     insp.State != nil && insp.State.Running,
	}, nil
}

// WaitDataPlaneHealthy best-effort waits until the container is running.
// It does not speak engine wire protocols (no pgx/redis). Timeout uses ctx.
func (c *Client) WaitDataPlaneHealthy(ctx context.Context, containerID string) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("compute: data-plane client unavailable")
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		running, err := c.ContainerRunning(ctx, containerID)
		if err != nil {
			return err
		}
		if running {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("compute: data-plane healthy wait: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
