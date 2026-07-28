package compute

import (
	"strings"
	"testing"
)

func TestDataPlaneHostConfigNeverPublishesPorts(t *testing.T) {
	t.Setenv(EnvNestedPortPublish, "")
	t.Setenv(EnvBrokerPortPublish, "")
	hc := dataPlaneHostConfig(5432, DataKindRDS)
	if hc.PublishAllPorts {
		t.Fatal("PublishAllPorts must be false")
	}
	if hc.PortBindings != nil && len(hc.PortBindings) > 0 {
		t.Fatalf("PortBindings must be empty, got %#v", hc.PortBindings)
	}
	if string(hc.NetworkMode) != DataPlaneNetworkName {
		t.Fatalf("NetworkMode=%q want %q", hc.NetworkMode, DataPlaneNetworkName)
	}
	if hc.Privileged {
		t.Fatal("Privileged must be false")
	}
	if len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" {
		t.Fatalf("CapDrop=%#v want [ALL]", hc.CapDrop)
	}
	wantCaps := map[string]struct{}{}
	for _, c := range dataPlaneBootstrapCaps {
		wantCaps[c] = struct{}{}
	}
	if len(hc.CapAdd) != len(wantCaps) {
		t.Fatalf("CapAdd=%#v want %v", hc.CapAdd, dataPlaneBootstrapCaps)
	}
	for _, c := range hc.CapAdd {
		if _, ok := wantCaps[c]; !ok {
			t.Fatalf("unexpected CapAdd %q in %#v", c, hc.CapAdd)
		}
	}
}

func TestValidateDataPlaneOpts(t *testing.T) {
	t.Run("rejects empty kind", func(t *testing.T) {
		err := ValidateDataPlaneOpts(DataPlaneOpts{Image: "postgres:16-alpine"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects unknown kind", func(t *testing.T) {
		err := ValidateDataPlaneOpts(DataPlaneOpts{Kind: "mysql", Image: "mysql:8"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects empty image", func(t *testing.T) {
		err := ValidateDataPlaneOpts(DataPlaneOpts{Kind: DataKindRDS})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects negative port", func(t *testing.T) {
		err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindRDS, Image: "postgres:16-alpine", ContainerPort: -1,
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("accepts rds", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindRDS, Image: "postgres:16-alpine",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts elasticache", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindElastiCache, Image: "valkey/valkey:8-alpine",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts memorydb", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindMemoryDB, Image: "valkey/valkey:8-alpine",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts docdb", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindDocDB, Image: "mongo:7",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts mq", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindMQ, Image: "rabbitmq:3.13-alpine",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts opensearch", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindOpenSearch, Image: "opensearchproject/opensearch:2.11.1",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts neptune", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindNeptune, Image: "tinkerpop/gremlin-server:3.7.3",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts msk", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindMSK, Image: "redpandadata/redpanda:v24.2.4",
			Cmd:  RedpandaStartCmd("noctaxris-msk-lab"),
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts mqtt", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindMQTT, Image: "eclipse-mosquitto:2.0.20",
			Cmd:  MosquittoStartCmd("/etc/noctaxris/mqtt/mosquitto.conf"),
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStartDataPlaneNilClient(t *testing.T) {
	var c *Client
	_, err := c.StartDataPlane(t.Context(), DataPlaneOpts{
		Kind: DataKindRDS, Image: "postgres:16-alpine",
	})
	if err == nil {
		t.Fatal("expected unavailable error")
	}
	if err := c.StopDataPlane(t.Context(), "x"); err == nil {
		t.Fatal("expected unavailable error")
	}
	if _, err := c.InspectDataPlane(t.Context(), "x"); err == nil {
		t.Fatal("expected unavailable error")
	}
	if err := c.WaitDataPlaneHealthy(t.Context(), "x"); err == nil {
		t.Fatal("expected unavailable error")
	}
	if _, err := c.EnsureDataPlaneNetwork(t.Context()); err == nil {
		t.Fatal("expected unavailable error")
	}
}

func TestNestedDataEndpointAndDefaults(t *testing.T) {
	if got := NestedDataEndpoint("noctaxris-data-rds-1", 5432); got != "noctaxris-data-rds-1:5432" {
		t.Fatalf("endpoint=%q", got)
	}
	if DefaultDataPlaneImage(DataKindRDS) != "postgres:16-alpine" {
		t.Fatalf("rds image=%q", DefaultDataPlaneImage(DataKindRDS))
	}
	if DefaultDataPlaneImageForRDS("mysql") != "mysql:8.0" {
		t.Fatalf("mysql image=%q", DefaultDataPlaneImageForRDS("mysql"))
	}
	if DefaultDataPlaneImageForRDS("mariadb") != "mariadb:11" {
		t.Fatalf("mariadb image=%q", DefaultDataPlaneImageForRDS("mariadb"))
	}
	if DefaultDataPlaneImageForRDS("postgres") != "postgres:16-alpine" {
		t.Fatalf("postgres image=%q", DefaultDataPlaneImageForRDS("postgres"))
	}
	if DefaultDataPlanePortForRDS("mysql") != 3306 || DefaultDataPlanePortForRDS("mariadb") != 3306 {
		t.Fatalf("mysql/mariadb port=%d/%d", DefaultDataPlanePortForRDS("mysql"), DefaultDataPlanePortForRDS("mariadb"))
	}
	if DefaultDataPlanePortForRDS("postgres") != 5432 {
		t.Fatalf("postgres port=%d", DefaultDataPlanePortForRDS("postgres"))
	}
	if DefaultDataPlanePort(DataKindElastiCache) != 6379 {
		t.Fatalf("redis port=%d", DefaultDataPlanePort(DataKindElastiCache))
	}
	if DefaultDataPlanePort(DataKindMemoryDB) != 6379 {
		t.Fatalf("memorydb port=%d", DefaultDataPlanePort(DataKindMemoryDB))
	}
	if DefaultDataPlaneImage(DataKindMemoryDB) != "valkey/valkey:8-alpine" {
		t.Fatalf("memorydb image=%q", DefaultDataPlaneImage(DataKindMemoryDB))
	}
	if DefaultDataPlanePort(DataKindDocDB) != 27017 {
		t.Fatalf("mongo port=%d", DefaultDataPlanePort(DataKindDocDB))
	}
	if DefaultDataPlanePort(DataKindMQ) != 5672 {
		t.Fatalf("amqp port=%d", DefaultDataPlanePort(DataKindMQ))
	}
	if DefaultDataPlaneImage(DataKindMQ) != "rabbitmq:3.13-alpine" {
		t.Fatalf("mq image=%q", DefaultDataPlaneImage(DataKindMQ))
	}
	if DefaultDataPlanePort(DataKindOpenSearch) != 9200 {
		t.Fatalf("opensearch port=%d", DefaultDataPlanePort(DataKindOpenSearch))
	}
	if DefaultDataPlaneImage(DataKindOpenSearch) != "opensearchproject/opensearch:2.11.1" {
		t.Fatalf("opensearch image=%q", DefaultDataPlaneImage(DataKindOpenSearch))
	}
	if DefaultDataPlanePort(DataKindNeptune) != 8182 {
		t.Fatalf("neptune port=%d", DefaultDataPlanePort(DataKindNeptune))
	}
	if DefaultDataPlaneImage(DataKindNeptune) != "tinkerpop/gremlin-server:3.7.3" {
		t.Fatalf("neptune image=%q", DefaultDataPlaneImage(DataKindNeptune))
	}
	if DefaultDataPlanePort(DataKindMSK) != 9092 {
		t.Fatalf("msk port=%d", DefaultDataPlanePort(DataKindMSK))
	}
	if DefaultDataPlaneImage(DataKindMSK) != "redpandadata/redpanda:v24.2.4" {
		t.Fatalf("msk image=%q", DefaultDataPlaneImage(DataKindMSK))
	}
	cmd := RedpandaStartCmd("noctaxris-msk-x")
	if len(cmd) < 3 || cmd[0] != "redpanda" || !strings.Contains(strings.Join(cmd, " "), "noctaxris-msk-x:9092") {
		t.Fatalf("redpanda cmd=%v", cmd)
	}
	if DefaultDataPlanePort(DataKindMQTT) != 1883 {
		t.Fatalf("mqtt port=%d", DefaultDataPlanePort(DataKindMQTT))
	}
	if DefaultDataPlaneImage(DataKindMQTT) != defaultMosquittoImage {
		t.Fatalf("mqtt image=%q", DefaultDataPlaneImage(DataKindMQTT))
	}
	mqttCmd := MosquittoStartCmd("")
	if len(mqttCmd) != 3 || mqttCmd[0] != "/usr/sbin/mosquitto" || mqttCmd[2] != "/etc/noctaxris/mqtt/mosquitto.conf" {
		t.Fatalf("mosquitto cmd=%v", mqttCmd)
	}
}

func TestDataPlaneBindsHashStable(t *testing.T) {
	a := dataPlaneBindsHash([]string{"/b:/x:ro", "/a:/y:ro"})
	b := dataPlaneBindsHash([]string{"/a:/y:ro", "/b:/x:ro"})
	if a != b || a == "none" {
		t.Fatalf("hash a=%q b=%q", a, b)
	}
	if dataPlaneBindsHash(nil) != "none" {
		t.Fatal("empty binds want none")
	}
	if dataPlaneBindsHash([]string{"/a:/x"}) == dataPlaneBindsHash([]string{"/a:/y"}) {
		t.Fatal("different binds must differ")
	}
}

func TestDefaultDataPlaneImageForMQActiveMQ(t *testing.T) {
	if DefaultDataPlaneImageForMQ("RABBITMQ") != defaultRabbitMQImage {
		t.Fatalf("rabbit image=%q", DefaultDataPlaneImageForMQ("RABBITMQ"))
	}
	if DefaultDataPlaneImageForMQ("ACTIVEMQ") != defaultActiveMQImage {
		t.Fatalf("activemq image=%q", DefaultDataPlaneImageForMQ("ACTIVEMQ"))
	}
	if DefaultDataPlaneImageForMQ("activemq") != defaultActiveMQImage {
		t.Fatalf("activemq case fold image=%q", DefaultDataPlaneImageForMQ("activemq"))
	}
	if DefaultDataPlaneImageForMQ("") != defaultRabbitMQImage {
		t.Fatalf("empty engine default image=%q", DefaultDataPlaneImageForMQ(""))
	}
	if defaultActiveMQImage != "apache/activemq-classic:5.18.3" {
		t.Fatalf("pinned ActiveMQ image=%q", defaultActiveMQImage)
	}
}
