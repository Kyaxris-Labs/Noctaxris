package compute

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
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

	// LabelDataKind marks nested data containers (values: rds|elasticache|memorydb|docdb|mq|opensearch|neptune|msk|mqtt|duckdb).
	LabelDataKind = "noctaxris.data"

	// LabKafkaContainerName is the shared DinD Redpanda singleton when shared Kafka is on.
	LabKafkaContainerName = "noctaxris-lab-kafka"
	// LabMQTTContainerName is the shared DinD Mosquitto singleton when shared MQTT is on.
	LabMQTTContainerName = "noctaxris-lab-mqtt"

	// LabelManaged marks Noctaxris-managed Docker objects.
	LabelManaged = "noctaxris.managed"
	// LabelBindsHash fingerprints HostConfig.Binds so EnsureDataPlaneByName can recreate
	// when TLS/config mounts change (e.g. Mosquitto material written after first start).
	LabelBindsHash = "noctaxris.binds_hash"

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
	defaultNeo4jImage      = "neo4j:5-community"
	defaultRedpandaImage   = "redpandadata/redpanda:v24.2.4"
	defaultMosquittoImage  = "eclipse-mosquitto:2.0.20"

	defaultPostgresPort   = 5432
	defaultMySQLPort      = 3306
	defaultRedisPort      = 6379
	defaultMongoPort      = 27017
	defaultAMQPPort       = 5672
	defaultOpenSearchPort = 9200
	defaultGremlinPort    = 8182
	defaultNeo4jBoltPort  = 7687
	defaultKafkaPort      = 9092
	defaultMQTTPort       = 1883
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
	DataKindMQTT        DataKind = "mqtt"
	DataKindDuckDB      DataKind = "duckdb"
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
	// Binds are optional host bind mounts (host:container:mode).
	Binds []string
	// ExtraHosts are optional container ExtraHosts entries (e.g. host.docker.internal:host-gateway).
	ExtraHosts []string
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
	case DataKindRDS, DataKindElastiCache, DataKindMemoryDB, DataKindDocDB, DataKindMQ, DataKindOpenSearch, DataKindNeptune, DataKindMSK, DataKindMQTT, DataKindDuckDB:
	default:
		return fmt.Errorf("compute: data-plane Kind must be rds, elasticache, memorydb, docdb, mq, opensearch, neptune, msk, mqtt, or duckdb")
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
	case DataKindMQTT:
		return defaultMosquittoImage
	case DataKindDuckDB:
		return DefaultDuckImage
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

// DefaultDataPlaneImageForNeptune returns the pinned nested image for a Neptune graph backend.
// neo4j → neo4j:5-community; anything else → tinkerpop/gremlin-server.
func DefaultDataPlaneImageForNeptune(graphEngine string) string {
	switch strings.ToLower(strings.TrimSpace(graphEngine)) {
	case "neo4j", "opencypher", "cypher", "bolt":
		return defaultNeo4jImage
	default:
		return defaultGremlinImage
	}
}

// DefaultDataPlanePortForNeptune returns the nested listen port for a Neptune graph backend.
// neo4j → Bolt 7687; gremlin (default) → 8182.
func DefaultDataPlanePortForNeptune(graphEngine string) int {
	switch strings.ToLower(strings.TrimSpace(graphEngine)) {
	case "neo4j", "opencypher", "cypher", "bolt":
		return defaultNeo4jBoltPort
	default:
		return defaultGremlinPort
	}
}

// NeptuneNestedBootstrapEnv returns env for nested Neptune backends.
// Neo4j disables auth (NEO4J_AUTH=none) so nested peers can use Bolt without brokered credentials,
// matching Neptune's edge IAM auth model. Never log returned values.
func NeptuneNestedBootstrapEnv(graphEngine string) map[string]string {
	switch strings.ToLower(strings.TrimSpace(graphEngine)) {
	case "neo4j", "opencypher", "cypher", "bolt":
		return map[string]string{"NEO4J_AUTH": "none"}
	default:
		return nil
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
	case DataKindMQTT:
		return defaultMQTTPort
	case DataKindDuckDB:
		return DefaultDuckPort
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

// MosquittoStartCmd returns Mosquitto start args using the mounted lab TLS config path.
func MosquittoStartCmd(configPath string) []string {
	path := strings.TrimSpace(configPath)
	if path == "" {
		path = "/etc/noctaxris/mqtt/mosquitto.conf"
	}
	return []string{"/usr/sbin/mosquitto", "-c", path}
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
func dataPlaneHostConfig(containerPort int, kind DataKind) *container.HostConfig {
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
	applyDataPlanePortPublish(hc, containerPort, kind)
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
		LabelManaged:   "true",
		LabelDataKind:  string(opts.Kind),
		LabelBindsHash: dataPlaneBindsHash(opts.Binds),
	}
	for k, v := range opts.Labels {
		if k == "" || k == LabelManaged || k == LabelDataKind || k == LabelBindsHash {
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
		ExposedPorts: dataPlaneExposedPorts(port, opts.Kind),
	}
	if len(opts.Cmd) > 0 {
		cfg.Cmd = append([]string(nil), opts.Cmd...)
	}
	hostConfig := dataPlaneHostConfig(port, opts.Kind)
	if len(opts.Binds) > 0 {
		hostConfig.Binds = append([]string(nil), opts.Binds...)
	}
	if len(opts.ExtraHosts) > 0 {
		hostConfig.ExtraHosts = append([]string(nil), opts.ExtraHosts...)
	}
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

// EnsureDataPlaneByName starts or reuses a named nested data container (idempotent singleton ensure).
// If a running container exists with a different binds fingerprint (LabelBindsHash), it is removed
// and recreated so new TLS/config mounts take effect.
func (c *Client) EnsureDataPlaneByName(ctx context.Context, opts DataPlaneOpts) (DataPlaneInstance, error) {
	if c == nil || c.cli == nil {
		return DataPlaneInstance{}, fmt.Errorf("compute: data-plane client unavailable")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return DataPlaneInstance{}, fmt.Errorf("compute: data-plane Name is required for ensure")
	}
	wantHash := dataPlaneBindsHash(opts.Binds)
	insp, err := c.cli.ContainerInspect(ctx, name)
	if err == nil {
		port := opts.ContainerPort
		if port == 0 {
			port = DefaultDataPlanePort(opts.Kind)
		}
		gotHash := ""
		if insp.Config != nil && insp.Config.Labels != nil {
			gotHash = insp.Config.Labels[LabelBindsHash]
		}
		if gotHash != wantHash {
			if err := c.StopDataPlane(ctx, insp.ID); err != nil {
				return DataPlaneInstance{}, fmt.Errorf("compute: recreate data-plane (binds changed): %w", err)
			}
			return c.StartDataPlane(ctx, opts)
		}
		if insp.State != nil && insp.State.Running {
			return DataPlaneInstance{
				ContainerID: insp.ID,
				Name:        name,
				Kind:        opts.Kind,
				Endpoint:    NestedDataEndpoint(name, port),
				Image:       insp.Config.Image,
				Running:     true,
			}, nil
		}
		if err := c.cli.ContainerStart(ctx, insp.ID, container.StartOptions{}); err != nil {
			return DataPlaneInstance{}, fmt.Errorf("compute: data-plane container start: %w", err)
		}
		return DataPlaneInstance{
			ContainerID: insp.ID,
			Name:        name,
			Kind:        opts.Kind,
			Endpoint:    NestedDataEndpoint(name, port),
			Image:       insp.Config.Image,
			Running:     true,
		}, nil
	}
	if !errdefs.IsNotFound(err) {
		return DataPlaneInstance{}, fmt.Errorf("compute: data-plane inspect: %w", err)
	}
	return c.StartDataPlane(ctx, opts)
}

func dataPlaneBindsHash(binds []string) string {
	if len(binds) == 0 {
		return "none"
	}
	sorted := append([]string(nil), binds...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:8])
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
