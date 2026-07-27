package compute

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"
)

const (
	// DataPlaneNetworkName is the DinD-internal network for nested data engines.
	// Containers on this network are not published to the operator host.
	DataPlaneNetworkName = "noctaxris-data"

	// LabelDataKind marks nested data containers (values: rds|elasticache|memorydb|docdb|mq|opensearch|neptune|msk).
	LabelDataKind = "noctaxris.data"

	// LabelManaged marks Noctaxris-managed Docker objects.
	LabelManaged = "noctaxris.managed"

	defaultPostgresImage   = "postgres:16-alpine"
	defaultMySQLImage      = "mysql:8.0"
	defaultMariaDBImage    = "mariadb:11"
	defaultValkeyImage     = "valkey/valkey:8-alpine"
	defaultMongoImage      = "mongo:7"
	defaultRabbitMQImage   = "rabbitmq:3.13-alpine"
	// apache/activemq-classic exposes AMQP on 5672 (plus OpenWire 61616 nested-only).
	defaultActiveMQImage   = "apache/activemq-classic:5.18.3"
	defaultOpenSearchImage = "opensearchproject/opensearch:2.11.1"
	defaultGremlinImage    = "tinkerpop/gremlin-server:3.7.3"
	defaultRedpandaImage   = "redpandadata/redpanda:v24.2.4"

	defaultPostgresPort   = 5432
	defaultMySQLPort      = 3306
	defaultRedisPort      = 6379
	defaultMongoPort      = 27017
	defaultAMQPPort       = 5672
	defaultOpenSearchPort = 9200
	defaultGremlinPort    = 8182
	defaultKafkaPort      = 9092
)

// DataKind identifies which nested data engine family a container belongs to.
type DataKind string

const (
	DataKindRDS         DataKind = "rds"
	DataKindElastiCache DataKind = "elasticache"
	DataKindMemoryDB    DataKind = "memorydb"
	DataKindDocDB       DataKind = "docdb"
	DataKindMQ          DataKind = "mq"
	DataKindOpenSearch  DataKind = "opensearch"
	DataKindNeptune     DataKind = "neptune"
	DataKindMSK         DataKind = "msk"
)

// DataPlaneOpts configures a nested data-engine container inside DinD.
//
// Host port publish is off by default (no PortBindings). Opt in with
// NOCTAXRIS_NESTED_PORT_PUBLISH=1 so the container port is bound on the DinD
// engine host; pair with docker/compose.lab-nested-ports.yaml for operator
// loopback publish. Callers normally use nested-network endpoints or HTTP
// facades on :4566 (for example RDS Data API).
type DataPlaneOpts struct {
	Kind  DataKind
	Image string
	// Name is an optional container name. Empty selects a generated name.
	Name string
	Env  map[string]string
	// Cmd overrides the image CMD when non-empty (used for Redpanda start args).
	Cmd []string
	// Labels are merged onto the container (noctaxris.data and noctaxris.managed
	// are always set and cannot be overridden away).
	Labels map[string]string
	// ContainerPort is the engine listen port inside the nested network.
	// Zero selects the default for Kind (5432 / 6379 / 27017 / 5672 / 9200 / 8182 / 9092).
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
	case DataKindRDS, DataKindElastiCache, DataKindMemoryDB, DataKindDocDB, DataKindMQ, DataKindOpenSearch, DataKindNeptune, DataKindMSK:
	default:
		return fmt.Errorf("compute: data-plane Kind must be rds, elasticache, memorydb, docdb, mq, opensearch, neptune, or msk")
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
// omit Image. Empty kind returns empty. DataKindMQ defaults to RabbitMQ;
// use DefaultDataPlaneImageForMQ for EngineType-specific pins.
// DataKindRDS defaults to Postgres; use DefaultDataPlaneImageForRDS for Engine-specific pins.
func DefaultDataPlaneImage(kind DataKind) string {
	switch kind {
	case DataKindRDS:
		return defaultPostgresImage
	case DataKindElastiCache, DataKindMemoryDB:
		return defaultValkeyImage
	case DataKindDocDB:
		return defaultMongoImage
	case DataKindMQ:
		return defaultRabbitMQImage
	case DataKindOpenSearch:
		return defaultOpenSearchImage
	case DataKindNeptune:
		return defaultGremlinImage
	case DataKindMSK:
		return defaultRedpandaImage
	default:
		return ""
	}
}

// DefaultDataPlaneImageForRDS returns the pinned nested image for RDS Engine.
// mysql → mysql:8.0; mariadb → mariadb:11; anything else → postgres:16-alpine.
func DefaultDataPlaneImageForRDS(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "mysql":
		return defaultMySQLImage
	case "mariadb":
		return defaultMariaDBImage
	default:
		return defaultPostgresImage
	}
}

// DefaultDataPlanePortForRDS returns the nested listen port for an RDS Engine.
func DefaultDataPlanePortForRDS(engine string) int {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "mysql", "mariadb":
		return defaultMySQLPort
	default:
		return defaultPostgresPort
	}
}

// DefaultDataPlaneImageForMQ returns the pinned nested image for Amazon MQ EngineType.
// ACTIVEMQ → apache/activemq-classic (AMQP 5672); anything else → RabbitMQ.
func DefaultDataPlaneImageForMQ(engineType string) string {
	switch strings.ToUpper(strings.TrimSpace(engineType)) {
	case "ACTIVEMQ":
		return defaultActiveMQImage
	default:
		return defaultRabbitMQImage
	}
}

// DefaultDataPlanePort returns the nested listen port for a kind.
func DefaultDataPlanePort(kind DataKind) int {
	switch kind {
	case DataKindRDS:
		return defaultPostgresPort
	case DataKindElastiCache, DataKindMemoryDB:
		return defaultRedisPort
	case DataKindDocDB:
		return defaultMongoPort
	case DataKindMQ:
		return defaultAMQPPort
	case DataKindOpenSearch:
		return defaultOpenSearchPort
	case DataKindNeptune:
		return defaultGremlinPort
	case DataKindMSK:
		return defaultKafkaPort
	default:
		return 0
	}
}

// RedpandaStartCmd returns pinned Redpanda start args for nested MSK.
// advertiseHost is the nested-network hostname clients use (container name).
func RedpandaStartCmd(advertiseHost string) []string {
	host := strings.TrimSpace(advertiseHost)
	if host == "" {
		host = "noctaxris-msk"
	}
	return []string{
		"redpanda", "start",
		"--overprovisioned",
		"--smp", "1",
		"--memory", "512M",
		"--reserve-memory", "0M",
		"--node-id", "0",
		"--check=false",
		"--kafka-addr", "internal://0.0.0.0:9092",
		"--advertise-kafka-addr", "internal://" + host + ":9092",
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

// dataPlaneBootstrapCaps are CapAdd entries required for official engine images
// (postgres/mongo/…) that bootstrap as root then drop to an unprivileged user.
// CapDrop remains ALL; without these CapAdds, entrypoints fail with
// "Operation not permitted" on chmod/chown/setuid (nested DinD smoke).
var dataPlaneBootstrapCaps = []string{
	"CHOWN", "DAC_OVERRIDE", "FOWNER", "SETGID", "SETUID",
}

// dataPlaneHostConfig returns HostConfig with CapDrop ALL and the minimal CapAdd
// set for nested data-engine bootstrap. PortBindings stay empty unless
// NOCTAXRIS_NESTED_PORT_PUBLISH is enabled (see applyDataPlanePortPublish).
// Exported via tests in this package to lock the secure-default invariant.
func dataPlaneHostConfig(containerPort int) *container.HostConfig {
	sec := nestedTaskSecurity(0)
	hc := &container.HostConfig{
		AutoRemove:      false,
		NetworkMode:     container.NetworkMode(DataPlaneNetworkName),
		PublishAllPorts: false,
		Privileged:      false,
		CapDrop:         append([]string(nil), sec.CapDrop...),
		CapAdd:          append([]string(nil), dataPlaneBootstrapCaps...),
		SecurityOpt:     append([]string(nil), sec.SecurityOpt...),
	}
	applyDataPlanePortPublish(hc, containerPort)
	return hc
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
// Host/engine port publish stays off unless NOCTAXRIS_NESTED_PORT_PUBLISH=1.
// Pull failure fails closed.
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
		Image:        opts.Image,
		Env:          env,
		Labels:       labels,
		ExposedPorts: dataPlaneExposedPorts(port),
	}
	if len(opts.Cmd) > 0 {
		cfg.Cmd = append([]string(nil), opts.Cmd...)
	}
	hostConfig := dataPlaneHostConfig(port)
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

// WaitDataPlaneHealthy best-effort waits until the container stays running briefly.
// It does not speak engine wire protocols (no pgx/redis). Timeout uses ctx.
// Exited containers fail immediately (includes inspect Error when present).
// The short stability window catches OpenSearch bootstrap exits (vm.max_map_count).
func (c *Client) WaitDataPlaneHealthy(ctx context.Context, containerID string) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("compute: data-plane client unavailable")
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("compute: container ID is required")
	}
	const stableFor = 2 * time.Second
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var runningSince time.Time
	for {
		insp, err := c.cli.ContainerInspect(ctx, containerID)
		if err != nil {
			if errdefs.IsNotFound(err) {
				return fmt.Errorf("compute: data-plane container not found")
			}
			return fmt.Errorf("compute: data-plane container inspect: %w", err)
		}
		if insp.State != nil && insp.State.Running {
			if runningSince.IsZero() {
				runningSince = time.Now()
			}
			if time.Since(runningSince) >= stableFor {
				return nil
			}
		} else {
			runningSince = time.Time{}
			if insp.State != nil && (insp.State.Status == "exited" || insp.State.Status == "dead") {
				msg := strings.TrimSpace(insp.State.Error)
				if msg == "" {
					msg = fmt.Sprintf("exit code %d", insp.State.ExitCode)
				}
				return fmt.Errorf("compute: data-plane container %s: %s", insp.State.Status, msg)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("compute: data-plane healthy wait: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// DataPlaneLogs returns combined stdout/stderr for a nested data container.
// Used to classify OpenSearch bootstrap failures (vm.max_map_count, memory lock).
func (c *Client) DataPlaneLogs(ctx context.Context, containerID string) (string, error) {
	if c == nil || c.cli == nil {
		return "", fmt.Errorf("compute: data-plane client unavailable")
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return "", fmt.Errorf("compute: container ID is required")
	}
	rc, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "200",
	})
	if err != nil {
		return "", fmt.Errorf("compute: data-plane logs: %w", err)
	}
	defer rc.Close()
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, rc); err != nil && err != io.EOF {
		all, _ := io.ReadAll(rc)
		return string(all), nil
	}
	return stdout.String() + stderr.String(), nil
}
